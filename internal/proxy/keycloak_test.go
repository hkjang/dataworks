package proxy

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"dataworks/internal/config"
)

func TestSSOStatusIncludesVersion(t *testing.T) {
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true, AllowLocalLogin: true}}}
	w := httptest.NewRecorder()
	s.handleSSOStatus(w, httptest.NewRequest(http.MethodGet, "/auth/sso/status", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != AppVersion {
		t.Fatalf("version=%v want %s", body["version"], AppVersion)
	}
}

func TestKeycloakCallbackErrorRedirectsToReactSPA(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{Enabled: true}}, db: db}
	w := httptest.NewRecorder()
	s.handleKeycloakCallback(w, httptest.NewRequest(http.MethodGet, "/auth/keycloak/callback?error=access_denied", nil))
	if w.Code != http.StatusFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	location := w.Header().Get("Location")
	if !strings.HasPrefix(location, "/dataworks/#kc_error=") {
		t.Fatalf("callback location=%q", location)
	}
}

func TestDataWorksSSOLandingPathFollowsAdminReadScope(t *testing.T) {
	if got := dataWorksSSOLandingPath(roleScopes["admin"]); got != "/dataworks/" {
		t.Fatalf("admin landing=%q, want /dataworks/", got)
	}
	if got := dataWorksSSOLandingPath(roleScopes["developer"]); got != "/dataworks/personal" {
		t.Fatalf("developer landing=%q, want /dataworks/personal", got)
	}
}

func TestActionCenterDistinguishesMissingScopeFromInvalidSession(t *testing.T) {
	_, proxy := newAuthTestServer(t, "http://example.invalid")
	defer proxy.Close()

	rootLogin := postJSON(t, proxy.URL+"/auth/login", "", map[string]string{
		"email": "root@example.com", "password": "correct-password",
	})
	var rootTokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(rootLogin.Body).Decode(&rootTokens); err != nil {
		t.Fatal(err)
	}
	rootLogin.Body.Close()

	created := postJSON(t, proxy.URL+"/admin/users", rootTokens.AccessToken, map[string]string{
		"email": "sso-developer@example.com", "password": "developer-password", "role": "developer",
	})
	created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create developer status=%d", created.StatusCode)
	}
	developerLogin := postJSON(t, proxy.URL+"/auth/login", "", map[string]string{
		"email": "sso-developer@example.com", "password": "developer-password",
	})
	var developerTokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(developerLogin.Body).Decode(&developerTokens); err != nil {
		t.Fatal(err)
	}
	developerLogin.Body.Close()

	actionRequest, _ := http.NewRequest(http.MethodGet, proxy.URL+"/admin/dataworks/action-center", nil)
	actionRequest.Header.Set("Authorization", "Bearer "+developerTokens.AccessToken)
	actionResponse, err := http.DefaultClient.Do(actionRequest)
	if err != nil {
		t.Fatal(err)
	}
	actionResponse.Body.Close()
	if actionResponse.StatusCode != http.StatusForbidden {
		t.Fatalf("valid developer session status=%d, want 403", actionResponse.StatusCode)
	}
	meRequest, _ := http.NewRequest(http.MethodGet, proxy.URL+"/auth/me", nil)
	meRequest.Header.Set("Authorization", "Bearer "+developerTokens.AccessToken)
	meResponse, err := http.DefaultClient.Do(meRequest)
	if err != nil {
		t.Fatal(err)
	}
	meResponse.Body.Close()
	if meResponse.StatusCode != http.StatusOK {
		t.Fatalf("scope denial must not invalidate the SSO session: /auth/me status=%d", meResponse.StatusCode)
	}

	created = postJSON(t, proxy.URL+"/admin/users", rootTokens.AccessToken, map[string]string{
		"email": "sso-viewer@example.com", "password": "viewer-password", "role": "viewer",
	})
	created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create viewer status=%d", created.StatusCode)
	}
	viewerLogin := postJSON(t, proxy.URL+"/auth/login", "", map[string]string{
		"email": "sso-viewer@example.com", "password": "viewer-password",
	})
	var viewerTokens struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(viewerLogin.Body).Decode(&viewerTokens); err != nil {
		t.Fatal(err)
	}
	viewerLogin.Body.Close()
	assetWrite := postJSON(t, proxy.URL+"/admin/dataworks/assets", viewerTokens.AccessToken, map[string]any{
		"asset_key": "readonly-must-not-write", "name": "Read-only asset",
	})
	assetWrite.Body.Close()
	if assetWrite.StatusCode != http.StatusForbidden {
		t.Fatalf("valid read-only session write status=%d, want 403", assetWrite.StatusCode)
	}
	viewerMeRequest, _ := http.NewRequest(http.MethodGet, proxy.URL+"/auth/me", nil)
	viewerMeRequest.Header.Set("Authorization", "Bearer "+viewerTokens.AccessToken)
	viewerMeResponse, err := http.DefaultClient.Do(viewerMeRequest)
	if err != nil {
		t.Fatal(err)
	}
	viewerMeResponse.Body.Close()
	if viewerMeResponse.StatusCode != http.StatusOK {
		t.Fatalf("write denial must not invalidate the read-only session: /auth/me status=%d", viewerMeResponse.StatusCode)
	}

	invalidRequest, _ := http.NewRequest(http.MethodGet, proxy.URL+"/admin/dataworks/action-center", nil)
	invalidRequest.Header.Set("Authorization", "Bearer invalid-session")
	invalidResponse, err := http.DefaultClient.Do(invalidRequest)
	if err != nil {
		t.Fatal(err)
	}
	invalidResponse.Body.Close()
	if invalidResponse.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invalid session status=%d, want 401", invalidResponse.StatusCode)
	}
}

func TestResolveKeycloakRole(t *testing.T) {
	cases := []struct {
		roles []string
		def   string
		want  string
	}{
		{[]string{"vibe-developer"}, "developer", "developer"},
		{[]string{"vibe-admin", "vibe-developer"}, "developer", "admin"}, // highest rank wins
		{[]string{"vibe-team-admin", "vibe-auditor"}, "developer", "team_admin"},
		{[]string{"unknown-role"}, "developer", "developer"}, // fallback default
		{[]string{"unknown-role"}, "", ""},                   // no default → block
		{[]string{"vibe-auditor"}, "developer", "readonly_admin"},
	}
	for i, c := range cases {
		if got := resolveKeycloakRole(c.roles, c.def); got != c.want {
			t.Errorf("case %d: resolveKeycloakRole(%v, %q) = %q, want %q", i, c.roles, c.def, got, c.want)
		}
	}
}

// resolveKeycloakRoleExplicit must distinguish an explicit claim→role match from a default
// fallback, so SSO login never silently demotes an existing user (e.g. super_admin) whose IdP
// carries no mapped role.
func TestResolveKeycloakRoleExplicit(t *testing.T) {
	cases := []struct {
		roles        []string
		def          string
		wantRole     string
		wantExplicit bool
	}{
		{[]string{"vibe-admin"}, "developer", "admin", true},        // explicit match
		{[]string{"unknown-role"}, "developer", "developer", false}, // default fallback (must not overwrite role)
		{[]string{}, "developer", "developer", false},               // no roles → fallback
		{[]string{"unknown-role"}, "", "", false},                   // no default → block
	}
	for i, c := range cases {
		role, explicit := resolveKeycloakRoleExplicit(nil, c.roles, c.def)
		if role != c.wantRole || explicit != c.wantExplicit {
			t.Errorf("case %d: got (%q,%v), want (%q,%v)", i, role, explicit, c.wantRole, c.wantExplicit)
		}
	}
}

func TestKeycloakTeamFromGroups(t *testing.T) {
	if got := keycloakTeamFromGroups([]string{"/other", "/teams/ai-platform"}); got != "ai-platform" {
		t.Errorf("team = %q, want ai-platform", got)
	}
	if got := keycloakTeamFromGroups([]string{"/teams/data-platform/sub"}); got != "data-platform" {
		t.Errorf("nested team = %q, want data-platform", got)
	}
	if got := keycloakTeamFromGroups([]string{"/nope"}); got != "" {
		t.Errorf("no team group should be empty, got %q", got)
	}
}

func TestClaimStringsAndRoles(t *testing.T) {
	claims := map[string]any{
		"realm_access": map[string]any{"roles": []any{"vibe-admin", "offline_access"}},
		"resource_access": map[string]any{
			"clustara": map[string]any{"roles": []any{"vibe-developer"}},
		},
		"groups": []any{"/teams/ai-platform"},
	}
	if got := claimStrings(claims, "realm_access.roles"); len(got) != 2 || got[0] != "vibe-admin" {
		t.Fatalf("realm roles = %v", got)
	}
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{ClientID: "clustara", RoleClaim: "realm_access.roles"}}}
	roles := s.keycloakRolesFromClaims(claims)
	// realm + client roles merged.
	hasAdmin, hasDev := false, false
	for _, r := range roles {
		if r == "vibe-admin" {
			hasAdmin = true
		}
		if r == "vibe-developer" {
			hasDev = true
		}
	}
	if !hasAdmin || !hasDev {
		t.Fatalf("expected realm+client roles, got %v", roles)
	}
}

// signRS256 builds a signed RS256 JWT for testing.
func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	hb, _ := json.Marshal(header)
	cb, _ := json.Marshal(claims)
	signing := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestVerifyKeycloakIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	// Seed the JWKS cache so verification doesn't hit the network.
	jwksMu.Lock()
	jwksKeys = map[string]*rsa.PublicKey{"test-kid": &key.PublicKey}
	jwksFetch = time.Now()
	jwksMu.Unlock()

	const issuer = "https://kc.example.com/realms/vibe"
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{ClientID: "clustara", IssuerURL: issuer}}}
	disc := oidcDiscovery{Issuer: issuer, JWKSURI: "http://unused"}

	base := map[string]any{
		"iss": issuer, "aud": "clustara", "sub": "u-123", "email": "dev@x.com",
		"nonce": "N1", "exp": float64(time.Now().Add(time.Hour).Unix()),
	}
	tok := signRS256(t, key, "test-kid", base)
	claims, err := s.verifyKeycloakIDToken(t.Context(), disc, tok, "N1")
	if err != nil || claims["sub"] != "u-123" {
		t.Fatalf("valid token should verify: claims=%v err=%v", claims, err)
	}

	// nonce mismatch.
	if _, err := s.verifyKeycloakIDToken(t.Context(), disc, tok, "WRONG"); err == nil {
		t.Error("nonce mismatch should fail")
	}
	// audience mismatch.
	badAud := signRS256(t, key, "test-kid", map[string]any{"iss": issuer, "aud": "other", "sub": "u", "nonce": "N1", "exp": float64(time.Now().Add(time.Hour).Unix())})
	if _, err := s.verifyKeycloakIDToken(t.Context(), disc, badAud, "N1"); err == nil {
		t.Error("audience mismatch should fail")
	}
	// expired.
	expired := signRS256(t, key, "test-kid", map[string]any{"iss": issuer, "aud": "clustara", "sub": "u", "nonce": "N1", "exp": float64(time.Now().Add(-time.Hour).Unix())})
	if _, err := s.verifyKeycloakIDToken(t.Context(), disc, expired, "N1"); err == nil {
		t.Error("expired token should fail")
	}
	// wrong issuer.
	badIss := signRS256(t, key, "test-kid", map[string]any{"iss": "https://evil", "aud": "clustara", "sub": "u", "nonce": "N1", "exp": float64(time.Now().Add(time.Hour).Unix())})
	if _, err := s.verifyKeycloakIDToken(t.Context(), disc, badIss, "N1"); err == nil {
		t.Error("issuer mismatch should fail")
	}
	// tampered signature (flip a key).
	otherKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := signRS256(t, otherKey, "test-kid", base)
	if _, err := s.verifyKeycloakIDToken(t.Context(), disc, forged, "N1"); err == nil {
		t.Error("token signed by wrong key must fail signature check")
	}
}

func TestVerifyKeycloakAccessToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwksMu.Lock()
	jwksKeys = map[string]*rsa.PublicKey{"at-kid": &key.PublicKey}
	jwksFetch = time.Now()
	jwksMu.Unlock()
	// Seed the discovery cache so verifyKeycloakAccessToken doesn't hit the network.
	const issuer = "https://kc.example.com/realms/vibe"
	discMu.Lock()
	discCache = oidcDiscovery{Issuer: issuer, JWKSURI: "http://unused", AuthorizationEndpoint: "x", TokenEndpoint: "y"}
	discFetch = time.Now()
	discMu.Unlock()

	db := openTestStore(t)
	defer db.Close()
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{
		Enabled: true, ClientID: "clustara", IssuerURL: issuer, DefaultRole: "developer",
		RoleClaim: "realm_access.roles", GroupClaim: "groups",
	}}, db: db}

	// Access token issued to this client (Keycloak commonly uses azp) with an admin
	// realm role → synthesized admin claims + scopes.
	tok := signRS256(t, key, "at-kid", map[string]any{
		"iss": issuer, "azp": "clustara", "typ": "Bearer", "sub": "svc-1", "email": "svc@x.com",
		"realm_access": map[string]any{"roles": []any{"vibe-admin"}},
		"groups":       []any{"/teams/ai-platform"},
		"exp":          float64(time.Now().Add(time.Hour).Unix()),
	})
	claims, ok := s.verifyKeycloakAccessToken(t.Context(), tok)
	if !ok || claims.Role != "admin" || claims.Subject != "svc-1" || claims.TeamID != "ai-platform" {
		t.Fatalf("access token claims = %+v ok=%v", claims, ok)
	}
	if !hasScope(claims.Scopes, "admin:read") {
		t.Errorf("admin role should carry admin:read scope, got %v", claims.Scopes)
	}
	// An aud claim is the other supported Keycloak representation.
	audToken := signRS256(t, key, "at-kid", map[string]any{
		"iss": issuer, "aud": []any{"account", "clustara"}, "azp": "another-client", "sub": "svc-aud",
		"realm_access": map[string]any{"roles": []any{"vibe-developer"}},
		"exp":          float64(time.Now().Add(time.Hour).Unix()),
	})
	if got, ok := s.verifyKeycloakAccessToken(t.Context(), audToken); !ok || got.Subject != "svc-aud" {
		t.Fatalf("configured audience should verify: claims=%+v ok=%v", got, ok)
	}
	// A same-realm token without this resource in aud/azp belongs to another client
	// and must not become a Data Works identity.
	for name, extra := range map[string]map[string]any{
		"missing client binding": {},
		"wrong authorized party": {"aud": "account", "azp": "other-client"},
	} {
		t.Run(name, func(t *testing.T) {
			foreignClaims := map[string]any{
				"iss": issuer, "sub": "foreign", "realm_access": map[string]any{"roles": []any{"vibe-admin"}},
				"exp": float64(time.Now().Add(time.Hour).Unix()),
			}
			for key, value := range extra {
				foreignClaims[key] = value
			}
			foreign := signRS256(t, key, "at-kid", foreignClaims)
			if _, ok := s.verifyKeycloakAccessToken(t.Context(), foreign); ok {
				t.Fatal("token minted for another client must be rejected")
			}
		})
	}
	// An ID token is also signed for this client, but it is not an API bearer token.
	// Reject it by payload type, and reject subject-less tokens so they cannot collapse
	// into an ambiguous shared identity.
	for name, claims := range map[string]map[string]any{
		"id token reuse": {
			"iss": issuer, "aud": "clustara", "typ": "ID", "sub": "browser-user",
			"realm_access": map[string]any{"roles": []any{"vibe-admin"}},
			"exp":          float64(time.Now().Add(time.Hour).Unix()),
		},
		"empty subject": {
			"iss": issuer, "azp": "clustara", "typ": "Bearer", "sub": "",
			"realm_access": map[string]any{"roles": []any{"vibe-admin"}},
			"exp":          float64(time.Now().Add(time.Hour).Unix()),
		},
	} {
		t.Run(name, func(t *testing.T) {
			token := signRS256(t, key, "at-kid", claims)
			if _, ok := s.verifyKeycloakAccessToken(t.Context(), token); ok {
				t.Fatal("non-access or subject-less token must be rejected")
			}
		})
	}
	// Expired access token rejected.
	expired := signRS256(t, key, "at-kid", map[string]any{
		"iss": issuer, "azp": "clustara", "typ": "Bearer", "sub": "x",
		"realm_access": map[string]any{"roles": []any{"vibe-admin"}},
		"exp":          float64(time.Now().Add(-time.Hour).Unix()),
	})
	if _, ok := s.verifyKeycloakAccessToken(t.Context(), expired); ok {
		t.Error("expired access token must be rejected")
	}
	// An HS256 token (our internal format) is ignored by the Keycloak verifier.
	if _, ok := s.verifyKeycloakAccessToken(t.Context(), "eyJhbGciOiJIUzI1NiJ9.e30.x"); ok {
		t.Error("HS256 token must not be accepted as a Keycloak access token")
	}
}
