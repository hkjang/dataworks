package proxy

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

func TestSafeReturnTo(t *testing.T) {
	cases := []struct {
		value string
		want  bool
	}{
		{"/dataworks/", true},
		{"/dataworks", true},
		{"/dataworks/products/orders?tab=contracts", true},
		{"/admin", true},
		{"/admin/", true},
		{"", false},
		{"/", false}, // the root is not a browser entry point (tokens would land on the API index)
		{"/v1/models", false},
		{"/auth/keycloak/callback", false},
		{"//evil.example/dataworks/", false},
		{"/\\evil.example", false},
		{"https://evil.example/dataworks/", false},
		{"dataworks/", false},
		{"/dataworks/\r\nLocation: https://evil", false},
		{"/dataworks/%0d%0a", true}, // percent-encoded stays inside the path
	}
	for _, c := range cases {
		if got := safeReturnTo(c.value); got != c.want {
			t.Errorf("safeReturnTo(%q) = %v, want %v", c.value, got, c.want)
		}
	}
}

func TestSilentRefusalLocation(t *testing.T) {
	cases := map[string]string{
		"":                                    "/dataworks/?sso=none",
		"/dataworks/":                         "/dataworks/?sso=none",
		"/dataworks/products/x":               "/dataworks/products/x?sso=none",
		"/dataworks/products/x?tab=contracts": "/dataworks/products/x?sso=none&tab=contracts",
		"/dataworks/x?sso=error":              "/dataworks/x?sso=none",
		"//evil.example/":                     "/dataworks/?sso=none",
		"/v1/models":                          "/dataworks/?sso=none",
	}
	for in, want := range cases {
		if got := silentRefusalLocation(in); got != want {
			t.Errorf("silentRefusalLocation(%q) = %q, want %q", in, got, want)
		}
	}
}

// seedKeycloakDiscovery points the process-wide discovery cache at fixed endpoints so the
// login/callback handlers never touch the network.
func seedKeycloakDiscovery(t *testing.T, issuer, authorizationEndpoint, tokenEndpoint string) {
	t.Helper()
	discMu.Lock()
	discCache = oidcDiscovery{Issuer: issuer, JWKSURI: "http://unused", AuthorizationEndpoint: authorizationEndpoint, TokenEndpoint: tokenEndpoint}
	discFetch = time.Now()
	discMu.Unlock()
}

// startKeycloakLogin calls the login handler and returns the query the browser would carry
// to the provider's authorization endpoint.
func startKeycloakLogin(t *testing.T, s *Server, target string) url.Values {
	t.Helper()
	w := httptest.NewRecorder()
	s.handleKeycloakLogin(w, httptest.NewRequest(http.MethodGet, target, nil))
	if w.Code != http.StatusFound {
		t.Fatalf("login status=%d body=%s", w.Code, w.Body.String())
	}
	loc, err := url.Parse(w.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return loc.Query()
}

func TestSSOStatusPublishesAutoLoginOnlyWithSSOEnabled(t *testing.T) {
	for name, kc := range map[string]config.KeycloakConfig{
		"off by default": {Enabled: true},
		"on":             {Enabled: true, AutoLogin: true},
		"sso disabled":   {Enabled: false, AutoLogin: true},
	} {
		t.Run(name, func(t *testing.T) {
			s := &Server{cfg: config.Config{Keycloak: kc}}
			w := httptest.NewRecorder()
			s.handleSSOStatus(w, httptest.NewRequest(http.MethodGet, "/auth/sso/status", nil))
			var body map[string]any
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if got := body["auto_login"]; got != (kc.Enabled && kc.AutoLogin) {
				t.Fatalf("auto_login=%v, want %v", got, kc.Enabled && kc.AutoLogin)
			}
		})
	}
}

// With auto_login off, ?prompt=none must be dropped: the provider gets an ordinary
// interactive request and the stored flow is not marked silent. This is what ties the
// redirect surface to the administrator setting.
func TestKeycloakLoginIgnoresPromptNoneWhenAutoLoginOff(t *testing.T) {
	const issuer = "https://kc.example.com/realms/vibe"
	seedKeycloakDiscovery(t, issuer, "https://kc.example.com/auth", "https://kc.example.com/token")
	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true, IssuerURL: issuer, ClientID: "dw"}}, db: db}

	q := startKeycloakLogin(t, s, "/auth/keycloak/login?prompt=none&return_to=%2Fdataworks%2Fproducts%2Fx")
	if q.Has("prompt") {
		t.Fatalf("prompt must not be forwarded while auto_login is off: %v", q)
	}
	fs, ok := s.takeOIDCFlow(context.Background(), q.Get("state"))
	if !ok || fs.silent {
		t.Fatalf("flow state = %+v ok=%v; want a non-silent flow", fs, ok)
	}
	if fs.returnTo != "/dataworks/products/x" {
		t.Fatalf("return_to should still be carried for an interactive login: %q", fs.returnTo)
	}
}

func TestKeycloakLoginForwardsPromptNoneWhenAutoLoginOn(t *testing.T) {
	const issuer = "https://kc.example.com/realms/vibe"
	seedKeycloakDiscovery(t, issuer, "https://kc.example.com/auth", "https://kc.example.com/token")
	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true, AutoLogin: true, IssuerURL: issuer, ClientID: "dw"}}, db: db}

	q := startKeycloakLogin(t, s, "/auth/keycloak/login?prompt=none&return_to=%2F%2Fevil.example%2F")
	if q.Get("prompt") != "none" {
		t.Fatalf("prompt=none should reach the provider when auto_login is on: %v", q)
	}
	fs, ok := s.takeOIDCFlow(context.Background(), q.Get("state"))
	if !ok || !fs.silent {
		t.Fatalf("flow state = %+v ok=%v; want a silent flow", fs, ok)
	}
	if fs.returnTo != "" {
		t.Fatalf("protocol-relative return_to must be dropped, got %q", fs.returnTo)
	}
	// An ordinary click on the SSO button (no prompt) stays interactive even with auto_login on.
	if q := startKeycloakLogin(t, s, "/auth/keycloak/login"); q.Has("prompt") {
		t.Fatalf("interactive login must not carry prompt: %v", q)
	}
}

// A refused silent attempt is not a failure: the browser is sent to the login screen at the
// deep link it started from, with ?sso=none so it does not try again, and without kc_error.
func TestKeycloakCallbackSilentRefusalLandsOnLoginWithMarker(t *testing.T) {
	const issuer = "https://kc.example.com/realms/vibe"
	seedKeycloakDiscovery(t, issuer, "https://kc.example.com/auth", "https://kc.example.com/token")
	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true, AutoLogin: true, IssuerURL: issuer, ClientID: "dw"}}, db: db}

	q := startKeycloakLogin(t, s, "/auth/keycloak/login?prompt=none&return_to=%2Fdataworks%2Fproducts%2Fx%3Ftab%3Dcontracts")
	state := q.Get("state")

	w := httptest.NewRecorder()
	s.handleKeycloakCallback(w, httptest.NewRequest(http.MethodGet, "/auth/keycloak/callback?error=login_required&error_description=Login+required&state="+url.QueryEscape(state), nil))
	if w.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if loc := w.Header().Get("Location"); loc != "/dataworks/products/x?sso=none&tab=contracts" {
		t.Fatalf("silent refusal location=%q", loc)
	}
	// The state was consumed: replaying the same refusal is no longer treated as silent.
	w = httptest.NewRecorder()
	s.handleKeycloakCallback(w, httptest.NewRequest(http.MethodGet, "/auth/keycloak/callback?error=login_required&state="+url.QueryEscape(state), nil))
	if loc := w.Header().Get("Location"); !strings.HasPrefix(loc, "/dataworks/#kc_error=") {
		t.Fatalf("replayed state should fall back to the failure path, got %q", loc)
	}
}

// An interactive login that the provider refuses (user cancelled, access denied) must keep
// reporting the failure through kc_error exactly as before.
func TestKeycloakCallbackInteractiveErrorStillReportsFailure(t *testing.T) {
	const issuer = "https://kc.example.com/realms/vibe"
	seedKeycloakDiscovery(t, issuer, "https://kc.example.com/auth", "https://kc.example.com/token")
	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true, AutoLogin: true, IssuerURL: issuer, ClientID: "dw"}}, db: db}

	state := startKeycloakLogin(t, s, "/auth/keycloak/login?return_to=%2Fdataworks%2Fproducts%2Fx").Get("state")
	w := httptest.NewRecorder()
	s.handleKeycloakCallback(w, httptest.NewRequest(http.MethodGet, "/auth/keycloak/callback?error=access_denied&state="+url.QueryEscape(state), nil))
	loc := w.Header().Get("Location")
	if !strings.HasPrefix(loc, "/dataworks/#kc_error=access_denied") || strings.Contains(loc, "sso=none") {
		t.Fatalf("interactive error location=%q", loc)
	}
}

// End-to-end: a silent attempt that succeeds must land on the deep link it started from with
// the session tokens in the fragment, and the round trip must go through the real routes.
func TestKeycloakSilentLoginReturnsToDeepLink(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwksMu.Lock()
	jwksKeys = map[string]*rsa.PublicKey{"silent-kid": &key.PublicKey}
	jwksFetch = time.Now()
	jwksMu.Unlock()

	const issuer = "https://kc.example.com/realms/vibe"
	var nonce string
	tokenStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil || r.PostForm.Get("grant_type") != "authorization_code" || r.PostForm.Get("code") != "code-1" {
			http.Error(w, "bad token request", http.StatusBadRequest)
			return
		}
		idToken := signRS256(t, key, "silent-kid", map[string]any{
			"iss": issuer, "aud": "dw", "sub": "silent-user", "email": "silent@example.com", "typ": "ID",
			"nonce": nonce, "exp": float64(time.Now().Add(time.Hour).Unix()),
			"realm_access": map[string]any{"roles": []any{"vibe-admin"}},
		})
		writeJSON(w, http.StatusOK, map[string]any{"access_token": "kc-at", "id_token": idToken, "token_type": "Bearer"})
	}))
	defer tokenStub.Close()
	seedKeycloakDiscovery(t, issuer, "https://kc.example.com/auth", tokenStub.URL)

	db := openTestStore(t)
	t.Cleanup(func() { db.Close() })
	logger := store.NewAsyncLogger(db, 32, filepath.Join(t.TempDir(), "fallback.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(context.Background()) })
	cfg := testConfig("http://example.invalid", "secret")
	cfg.Auth.Enabled = true
	cfg.Auth.JWTSecret = "test-jwt-secret"
	cfg.Auth.AccessTokenTTL = 15 * time.Minute
	cfg.Auth.RefreshTokenTTL = time.Hour
	cfg.Auth.APIKeyPrefix = "vc_sk_"
	cfg.Auth.ServiceKeyPrefix = "vc_sa_"
	cfg.Keycloak = config.KeycloakConfig{
		Enabled: true, AutoLogin: true, IssuerURL: issuer, ClientID: "dw", RedirectURI: "http://app.example/auth/keycloak/callback",
		Scopes: []string{"openid"}, DefaultRole: "developer", RoleClaim: "realm_access.roles", GroupClaim: "groups",
	}
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(server.Routes())
	defer proxy.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}

	start, err := client.Get(proxy.URL + "/auth/keycloak/login?prompt=none&return_to=" + url.QueryEscape("/dataworks/products/orders?tab=contracts"))
	if err != nil {
		t.Fatal(err)
	}
	start.Body.Close()
	authz, err := url.Parse(start.Header.Get("Location"))
	if err != nil || authz.Query().Get("prompt") != "none" {
		t.Fatalf("login redirect=%q err=%v", start.Header.Get("Location"), err)
	}
	nonce = authz.Query().Get("nonce")

	done, err := client.Get(proxy.URL + "/auth/keycloak/callback?code=code-1&state=" + url.QueryEscape(authz.Query().Get("state")))
	if err != nil {
		t.Fatal(err)
	}
	done.Body.Close()
	loc := done.Header.Get("Location")
	if done.StatusCode != http.StatusFound || !strings.HasPrefix(loc, "/dataworks/products/orders?tab=contracts#") {
		t.Fatalf("callback status=%d location=%q", done.StatusCode, loc)
	}
	frag, _ := url.ParseQuery(loc[strings.Index(loc, "#")+1:])
	if frag.Get("kc_access") == "" || frag.Get("kc_refresh") == "" {
		t.Fatalf("session tokens missing from fragment: %q", loc)
	}
	me, _ := http.NewRequest(http.MethodGet, proxy.URL+"/auth/me", nil)
	me.Header.Set("Authorization", "Bearer "+frag.Get("kc_access"))
	res, err := http.DefaultClient.Do(me)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("/auth/me with the silently issued token = %d", res.StatusCode)
	}
}

// The admin screen must be able to switch auto_login on and the runtime overlay must see it;
// omitting the field in the payload keeps it off.
func TestKeycloakConfigSaveRoundTripsAutoLogin(t *testing.T) {
	_, proxy := newAuthTestServer(t, "http://example.invalid")
	defer proxy.Close()
	login := postJSON(t, proxy.URL+"/auth/login", "", map[string]string{"email": "root@example.com", "password": "correct-password"})
	var tokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(login.Body).Decode(&tokens); err != nil {
		t.Fatal(err)
	}
	login.Body.Close()

	readConfig := func() map[string]any {
		req, _ := http.NewRequest(http.MethodGet, proxy.URL+"/admin/sso/keycloak/config", nil)
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var body map[string]any
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body
	}
	save := func(payload map[string]any) {
		req, _ := http.NewRequest(http.MethodPut, proxy.URL+"/admin/sso/keycloak/config", strings.NewReader(mustJSON(payload)))
		req.Header.Set("Authorization", "Bearer "+tokens.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNoContent {
			t.Fatalf("save status=%d", res.StatusCode)
		}
	}
	base := map[string]any{"enabled": true, "issuer_url": "https://kc.example.com/realms/vibe", "client_id": "dw", "allow_local_login": true}
	save(base)
	if cfg := readConfig(); cfg["auto_login"] != false {
		t.Fatalf("auto_login must default to off, got %v", cfg["auto_login"])
	}
	status, _ := http.Get(proxy.URL + "/auth/sso/status")
	var st map[string]any
	_ = json.NewDecoder(status.Body).Decode(&st)
	status.Body.Close()
	if st["auto_login"] != false {
		t.Fatalf("public status auto_login=%v before enabling", st["auto_login"])
	}

	base["auto_login"] = true
	save(base)
	if cfg := readConfig(); cfg["auto_login"] != true {
		t.Fatalf("auto_login did not persist: %v", cfg["auto_login"])
	}
	status, _ = http.Get(proxy.URL + "/auth/sso/status")
	_ = json.NewDecoder(status.Body).Decode(&st)
	status.Body.Close()
	if st["auto_login"] != true {
		t.Fatalf("public status should publish auto_login after saving, got %v", st["auto_login"])
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
