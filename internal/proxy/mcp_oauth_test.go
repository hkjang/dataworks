package proxy

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

// fakeIdP is a stand-in Keycloak realm: it serves discovery and a JWKS with one RSA and
// one EC key, and signs tokens with either.
type fakeIdP struct {
	issuer string
	rsaKey *rsa.PrivateKey
	ecKey  *ecdsa.PrivateKey
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{rsaKey: rsaKey, ecKey: ecKey}
	mux := http.NewServeMux()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	idp.issuer = ts.URL + "/realms/dw"
	mux.HandleFunc("/realms/dw/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": idp.issuer, "authorization_endpoint": idp.issuer + "/protocol/openid-connect/auth",
			"token_endpoint": idp.issuer + "/protocol/openid-connect/token", "jwks_uri": idp.issuer + "/protocol/openid-connect/certs",
		})
	})
	mux.HandleFunc("/realms/dw/protocol/openid-connect/certs", func(w http.ResponseWriter, r *http.Request) {
		b64 := func(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
		size := (idp.ecKey.Curve.Params().BitSize + 7) / 8
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]any{
			{"kty": "RSA", "kid": "rsa1", "use": "sig", "alg": "RS256", "n": b64(idp.rsaKey.N.Bytes()), "e": b64(big.NewInt(int64(idp.rsaKey.E)).Bytes())},
			{"kty": "EC", "kid": "ec1", "use": "sig", "alg": "ES256", "crv": "P-256", "x": b64(idp.ecKey.X.FillBytes(make([]byte, size))), "y": b64(idp.ecKey.Y.FillBytes(make([]byte, size)))},
		}})
	})
	// The process-wide caches must not hand this test the previous test's keys.
	discMu.Lock()
	discCache, discFetch = oidcDiscovery{}, time.Time{}
	discMu.Unlock()
	jwksMu.Lock()
	jwksKeys, jwksFetch, jwksMiss = nil, time.Time{}, time.Time{}
	jwksMu.Unlock()
	return idp
}

// claims returns a Keycloak-shaped access token payload for a client, one hour long.
func (idp *fakeIdP) claims(sub, azp string, extra map[string]any) map[string]any {
	claims := map[string]any{
		"iss": idp.issuer, "sub": sub, "azp": azp, "aud": []any{"account"}, "typ": "Bearer",
		"exp": float64(time.Now().Add(time.Hour).Unix()), "iat": float64(time.Now().Unix()),
		"scope": "openid profile email",
	}
	for k, v := range extra {
		if v == nil {
			delete(claims, k)
			continue
		}
		claims[k] = v
	}
	return claims
}

func (idp *fakeIdP) sign(t *testing.T, alg, kid string, claims map[string]any) string {
	t.Helper()
	hb, _ := json.Marshal(map[string]any{"alg": alg, "typ": "JWT", "kid": kid})
	cb, _ := json.Marshal(claims)
	signing := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(cb)
	digest := sha256.Sum256([]byte(signing))
	var sig []byte
	switch alg {
	case "RS256":
		var err error
		if sig, err = rsa.SignPKCS1v15(rand.Reader, idp.rsaKey, crypto.SHA256, digest[:]); err != nil {
			t.Fatal(err)
		}
	case "PS256":
		var err error
		if sig, err = rsa.SignPSS(rand.Reader, idp.rsaKey, crypto.SHA256, digest[:], nil); err != nil {
			t.Fatal(err)
		}
	case "ES256":
		r, s, err := ecdsa.Sign(rand.Reader, idp.ecKey, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		sig = append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	case "HS256":
		sig = []byte("not-a-real-mac-but-well-formed")
	default:
		t.Fatalf("unsupported test alg %s", alg)
	}
	return signing + "." + base64.RawURLEncoding.EncodeToString(sig)
}

// token is the common case: an RS256 access token for the MCP client "claude-mcp".
func (idp *fakeIdP) token(t *testing.T, sub string, extra map[string]any) string {
	t.Helper()
	return idp.sign(t, "RS256", "rsa1", idp.claims(sub, "claude-mcp", extra))
}

// mcpOAuthServer boots a full server with auth on and Keycloak SSO pointed at the fake
// realm, plus the accounts the tests need. OAuth itself is left off (the default).
func mcpOAuthServer(t *testing.T, idp *fakeIdP) (*Server, *store.SQLStore, *httptest.Server) {
	t.Helper()
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
	cfg.Keycloak = config.KeycloakConfig{Enabled: true, IssuerURL: idp.issuer, ClientID: "dataworks-web", RedirectURI: "https://dw.example.com/auth/keycloak/callback", DefaultRole: "developer", RoleClaim: "realm_access.roles", GroupClaim: "groups"}
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Routes())
	t.Cleanup(ts.Close)
	ctx := context.Background()
	for _, u := range []struct{ id, email, role, status, sub string }{
		{"usr_alice", "alice@example.com", "developer", "active", "alice-sub"},
		{"usr_bob", "bob@example.com", "developer", "suspended", "bob-sub"},
		{"usr_carol", "carol@example.com", "developer", "active", ""},
		{"usr_dave", "dave@example.com", "viewer", "active", "dave-sub"},
	} {
		if err := db.CreateAuthUser(ctx, store.AuthUser{ID: u.id, Email: u.email, Name: u.id, Role: u.role, Status: u.status}); err != nil {
			t.Fatal(err)
		}
		if u.sub != "" {
			if err := db.UpsertAuthIdentity(ctx, store.AuthIdentity{ID: "authid_" + u.id, UserID: u.id, Provider: "keycloak", Issuer: idp.issuer, Subject: u.sub, Email: u.email}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return server, db, ts
}

// setMCPOAuth writes mcp.oauth.* the way the settings API stores them and reloads.
func setMCPOAuth(t *testing.T, server *Server, db *store.SQLStore, values map[string]string) {
	t.Helper()
	for key, value := range values {
		def, ok := settingDefByKey(key)
		if !ok {
			t.Fatalf("unknown setting %s", key)
		}
		if err := validateSettingValue(def, value); err != nil {
			t.Fatalf("%s=%q: %v", key, value, err)
		}
		encoded, _ := json.Marshal(value)
		if err := db.UpsertAdminSetting(context.Background(), store.AdminSetting{Key: key, Category: def.Category, ValueJSON: string(encoded), ValueType: string(def.Type), Source: "admin"}, "test", "test"); err != nil {
			t.Fatal(err)
		}
	}
	server.reloadRuntimeConfig(context.Background())
}

func mcpPost(t *testing.T, url, bearer string, body string) (*http.Response, map[string]any) {
	t.Helper()
	r, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return resp, out
}

func errorCode(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		code, _ := e["code"].(string)
		return code
	}
	return ""
}

func errorMessage(body map[string]any) string {
	if e, ok := body["error"].(map[string]any); ok {
		msg, _ := e["message"].(string)
		return msg
	}
	return ""
}

const toolsList = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func TestMCPOAuthOffByDefaultChangesNothing(t *testing.T) {
	idp := newFakeIdP(t)
	server, _, ts := mcpOAuthServer(t, idp)
	if st := server.mcpOAuth(); st.Enabled || st.Active() {
		t.Fatalf("oauth must be off by default: %+v", st)
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource", "/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource/mcp/gateway"} {
		resp, _ := http.Get(ts.URL + path)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("%s while off = %d, want 404", path, resp.StatusCode)
		}
	}
	// A perfectly good token is refused exactly like an unknown key, with no challenge
	// header pointing anywhere: a deployment that never turned this on says nothing new.
	for _, bearer := range []string{"", idp.token(t, "alice-sub", nil), "vc_sk_unknown"} {
		for _, path := range []string{"/mcp", "/mcp/gateway"} {
			resp, body := mcpPost(t, ts.URL+path, bearer, toolsList)
			if resp.StatusCode != http.StatusUnauthorized || errorCode(body) != "invalid_api_key" || resp.Header.Get("WWW-Authenticate") != "" {
				t.Fatalf("%s bearer=%q: status=%d code=%q www-authenticate=%q", path, bearer, resp.StatusCode, errorCode(body), resp.Header.Get("WWW-Authenticate"))
			}
		}
	}
}

func TestMCPOAuthMetadataAndChallengeOnMCPPathsOnly(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp"})

	// The resource identifier comes from the Keycloak redirect URI's origin — a public
	// address the administrator already wrote — not from the request's Host header.
	for path, wantResource := range map[string]string{
		"/.well-known/oauth-protected-resource":             "https://dw.example.com/mcp",
		"/.well-known/oauth-protected-resource/mcp":         "https://dw.example.com/mcp",
		"/.well-known/oauth-protected-resource/mcp/gateway": "https://dw.example.com/mcp/gateway",
	} {
		resp, err := http.Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&doc)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Access-Control-Allow-Origin") != "*" || !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("%s: status=%d cors=%q ct=%q", path, resp.StatusCode, resp.Header.Get("Access-Control-Allow-Origin"), resp.Header.Get("Content-Type"))
		}
		if doc["resource"] != wantResource {
			t.Fatalf("%s resource = %v, want %s", path, doc["resource"], wantResource)
		}
		if servers, _ := doc["authorization_servers"].([]any); len(servers) != 1 || servers[0] != idp.issuer {
			t.Fatalf("%s authorization_servers = %v", path, doc["authorization_servers"])
		}
		if methods, _ := doc["bearer_methods_supported"].([]any); len(methods) != 1 || methods[0] != "header" {
			t.Fatalf("%s bearer_methods_supported = %v", path, doc["bearer_methods_supported"])
		}
		if scopes, _ := doc["scopes_supported"].([]any); len(scopes) != 1 || scopes[0] != "mcp:use" {
			t.Fatalf("%s scopes_supported = %v", path, doc["scopes_supported"])
		}
		if _, envelope := doc["data"]; envelope {
			t.Fatalf("%s must be the bare RFC 9728 document, got %v", path, doc)
		}
	}
	if resp, _ := http.Get(ts.URL + "/.well-known/oauth-protected-resource/v1/models"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("metadata for a non-MCP path = %d, want 404", resp.StatusCode)
	}

	// 401 on the MCP paths carries the pointer; with a bearer it also says invalid_token.
	for path, want := range map[string]string{
		"/mcp":         `Bearer realm="Data Works", resource_metadata="https://dw.example.com/.well-known/oauth-protected-resource/mcp"`,
		"/mcp/gateway": `Bearer realm="Data Works", resource_metadata="https://dw.example.com/.well-known/oauth-protected-resource/mcp/gateway"`,
	} {
		resp, body := mcpPost(t, ts.URL+path, "", toolsList)
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != want {
			t.Fatalf("%s no token: status=%d www-authenticate=%q body=%v", path, resp.StatusCode, resp.Header.Get("WWW-Authenticate"), body)
		}
		resp, body = mcpPost(t, ts.URL+path, "vc_sk_unknown", toolsList)
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != want+`, error="invalid_token"` || errorCode(body) != "invalid_api_key" {
			t.Fatalf("%s bad key: status=%d www-authenticate=%q code=%q", path, resp.StatusCode, resp.Header.Get("WWW-Authenticate"), errorCode(body))
		}
	}
	// REST 401s never carry it: browsers and SDK clients would follow it somewhere wrong.
	resp, _ := mcpPost(t, ts.URL+"/v1/chat/completions", "", `{"model":"test-model","messages":[{"role":"user","content":"hi"}]}`)
	if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("/v1/chat/completions: status=%d www-authenticate=%q", resp.StatusCode, resp.Header.Get("WWW-Authenticate"))
	}
}

func TestMCPOAuthTokenOpensRegisteredAccountOnly(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp"})
	before, _ := db.ListAuthUsers(context.Background())

	// azp listed by the administrator (the mapper-less path a real Keycloak 26 needs).
	for _, path := range []string{"/mcp", "/mcp/gateway"} {
		resp, body := mcpPost(t, ts.URL+path, idp.token(t, "alice-sub", nil), toolsList)
		if resp.StatusCode != http.StatusOK || body["result"] == nil {
			t.Fatalf("%s with alice's token: status=%d body=%v", path, resp.StatusCode, body)
		}
	}
	// aud naming the resource (the Audience-mapper path) with an unlisted azp.
	mapped := idp.token(t, "alice-sub", map[string]any{"aud": []any{"account", "https://dw.example.com/mcp"}, "azp": "some-other-client"})
	for _, path := range []string{"/mcp", "/mcp/gateway"} {
		if resp, body := mcpPost(t, ts.URL+path, mapped, toolsList); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s with mapped audience: status=%d body=%v", path, resp.StatusCode, body)
		}
	}
	// EC and PSS signatures from the realm's JWKS verify too.
	for _, alg := range []struct{ alg, kid string }{{"ES256", "ec1"}, {"PS256", "rsa1"}} {
		tok := idp.sign(t, alg.alg, alg.kid, idp.claims("alice-sub", "claude-mcp", nil))
		if resp, body := mcpPost(t, ts.URL+"/mcp/gateway", tok, toolsList); resp.StatusCode != http.StatusOK {
			t.Fatalf("%s token: status=%d body=%v", alg.alg, resp.StatusCode, body)
		}
	}
	// An account without a linked subject is found by this application's username (email).
	if resp, body := mcpPost(t, ts.URL+"/mcp/gateway", idp.token(t, "carol-sub", map[string]any{"email": "carol@example.com"}), toolsList); resp.StatusCode != http.StatusOK {
		t.Fatalf("carol by email: status=%d body=%v", resp.StatusCode, body)
	}

	// The identity is the person's own, with the administrator's scopes cut to the role —
	// never a role read from the token.
	r := httptest.NewRequest(http.MethodPost, "/mcp/gateway", nil)
	r.Header.Set("Authorization", "Bearer "+idp.token(t, "alice-sub", map[string]any{"realm_access": map[string]any{"roles": []any{"vibe-admin"}}}))
	id, authCtx, ok, reason := server.authenticateProxyContextReason(r)
	if !ok || id != "oauth_usr_alice" || authCtx == nil || authCtx.UserID != "usr_alice" || authCtx.Role != "developer" || strings.Join(authCtx.Scopes, " ") != "mcp:use" || authCtx.APIKeyID != "" {
		t.Fatalf("principal = %q %+v ok=%v reason=%v", id, authCtx, ok, reason)
	}
	// A token that speaks this application's vocabulary only narrows further.
	r.Header.Set("Authorization", "Bearer "+idp.token(t, "alice-sub", map[string]any{"scope": "openid models:read"}))
	if _, _, ok, reason := server.authenticateProxyContextReason(r); ok {
		t.Fatalf("token scope without mcp:use must not open /mcp: ok=%v reason=%v", ok, reason)
	} else if !strings.Contains(reason.Error(), "insufficient_scope") {
		t.Fatalf("reason = %v", reason)
	}

	// Nobody was provisioned, revived or promoted along the way.
	after, _ := db.ListAuthUsers(context.Background())
	if len(after) != len(before) {
		t.Fatalf("users changed: %d → %d", len(before), len(after))
	}
	for _, u := range after {
		if u.ID == "usr_bob" && u.Status != "suspended" || u.ID == "usr_alice" && u.Role != "developer" {
			t.Fatalf("account mutated by a token: %+v", u)
		}
	}
}

func TestMCPOAuthRefusesForeignAndBrokenTokens(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp"})
	before, _ := db.ListAuthUsers(context.Background())
	past := float64(time.Now().Add(-time.Hour).Unix())
	future := float64(time.Now().Add(time.Hour).Unix())

	cases := []struct {
		name, token, code string
		contains          []string
	}{
		{"other client, no mapper", idp.token(t, "alice-sub", map[string]any{"azp": "other-client"}), "invalid_audience",
			[]string{`azp="other-client"`, "aud=[account]", "mcp.oauth.audience", "https://dw.example.com/mcp"}},
		{"expired", idp.token(t, "alice-sub", map[string]any{"exp": past}), "invalid_token", []string{"expired"}},
		{"not yet valid", idp.token(t, "alice-sub", map[string]any{"nbf": future}), "invalid_token", []string{"not yet valid"}},
		{"other issuer", idp.token(t, "alice-sub", map[string]any{"iss": "https://other-idp.example.com/realms/x"}), "invalid_token", []string{"issuer"}},
		{"id token", idp.token(t, "alice-sub", map[string]any{"typ": "ID", "aud": []any{"claude-mcp"}}), "invalid_token", []string{"id token"}},
		{"hs256", idp.sign(t, "HS256", "rsa1", idp.claims("alice-sub", "claude-mcp", nil)), "invalid_token", []string{"alg"}},
		{"sender-constrained", idp.token(t, "alice-sub", map[string]any{"cnf": map[string]any{"jkt": "abc"}}), "invalid_token", []string{"cnf"}},
		{"unknown kid", idp.sign(t, "RS256", "rsa-rotated-away", idp.claims("alice-sub", "claude-mcp", nil)), "invalid_token", nil},
		{"missing sub", idp.token(t, "", nil), "invalid_token", []string{"sub"}},
		{"suspended account", idp.token(t, "bob-sub", nil), "account_not_registered", []string{"웹으로"}},
		{"unregistered subject", idp.token(t, "nobody-sub", map[string]any{"email": "nobody@example.com"}), "account_not_registered", []string{"웹으로"}},
		{"role without mcp:use", idp.token(t, "dave-sub", nil), "insufficient_scope", []string{"mcp:use"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, body := mcpPost(t, ts.URL+"/mcp/gateway", tc.token, toolsList)
			if resp.StatusCode != http.StatusUnauthorized || errorCode(body) != tc.code {
				t.Fatalf("status=%d code=%q message=%q", resp.StatusCode, errorCode(body), errorMessage(body))
			}
			if !strings.HasSuffix(resp.Header.Get("WWW-Authenticate"), `error="invalid_token"`) {
				t.Fatalf("WWW-Authenticate = %q", resp.Header.Get("WWW-Authenticate"))
			}
			for _, want := range tc.contains {
				if !strings.Contains(errorMessage(body), want) {
					t.Fatalf("message %q should mention %q", errorMessage(body), want)
				}
			}
		})
	}
	after, _ := db.ListAuthUsers(context.Background())
	if len(after) != len(before) {
		t.Fatalf("a refused token created an account: %d → %d", len(before), len(after))
	}

	// Refusals are recorded for the operator.
	events, _ := db.ListAuditEvents(context.Background(), 200)
	denied := 0
	for _, e := range events {
		if e.EventType == "mcp_oauth_denied" {
			denied++
		}
	}
	if denied != len(cases) {
		t.Fatalf("mcp_oauth_denied events = %d, want %d", denied, len(cases))
	}
}

func TestMCPOAuthTokenRefusedOutsideMCPAndKeysUnchanged(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp"})
	token := idp.token(t, "alice-sub", nil)

	// The same token that opens /mcp opens nothing else.
	for _, path := range []string{"/v1/chat/completions", "/v1/embeddings"} {
		resp, body := mcpPost(t, ts.URL+path, token, `{"model":"test-model","input":"hi","messages":[{"role":"user","content":"hi"}]}`)
		if resp.StatusCode != http.StatusUnauthorized || resp.Header.Get("WWW-Authenticate") != "" {
			t.Fatalf("%s with an SSO token = %d body=%v www-authenticate=%q, want 401 without a challenge", path, resp.StatusCode, body, resp.Header.Get("WWW-Authenticate"))
		}
	}

	// An API key works exactly as before while OAuth is on.
	if err := db.UpsertAPIKey(context.Background(), store.APIKeyRecord{ID: "key_alice", Name: "alice", KeyHash: hashProxyKey("vc_sk_alice"), UserID: "usr_alice", Role: "developer", Status: "active", Scopes: scopesForRole("developer"), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/mcp", "/mcp/gateway"} {
		if resp, body := mcpPost(t, ts.URL+path, "vc_sk_alice", toolsList); resp.StatusCode != http.StatusOK || body["result"] == nil {
			t.Fatalf("%s with an API key: status=%d body=%v", path, resp.StatusCode, body)
		}
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer vc_sk_alice")
	if id, authCtx, ok := server.authenticateProxyContext(r); !ok || id != "key_alice" || authCtx == nil || authCtx.APIKeyID != "key_alice" {
		t.Fatalf("key principal = %q %+v ok=%v", id, authCtx, ok)
	}
}

func TestMCPOAuthEnabledWithoutIssuerStaysOff(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	server.keycloakCfg.Store(&config.KeycloakConfig{Enabled: false})
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp"})
	st := server.mcpOAuth()
	if !st.Enabled || st.Active() || st.Problem == "" {
		t.Fatalf("state = %+v", st)
	}
	if resp, _ := http.Get(ts.URL + "/.well-known/oauth-protected-resource"); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("metadata without issuer = %d, want 404", resp.StatusCode)
	}
	resp, body := mcpPost(t, ts.URL+"/mcp", idp.token(t, "alice-sub", nil), toolsList)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(body) != "invalid_api_key" || resp.Header.Get("WWW-Authenticate") != "" {
		t.Fatalf("token while inactive: status=%d code=%q www-authenticate=%q", resp.StatusCode, errorCode(body), resp.Header.Get("WWW-Authenticate"))
	}
}

func TestMCPOAuthResourceSettingOverridesDerivation(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.resource": "https://mcp.example.com/mcp/", "mcp.oauth.scopes": "mcp:use models:read"})
	resp, err := http.Get(ts.URL + "/.well-known/oauth-protected-resource/mcp/gateway")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&doc)
	resp.Body.Close()
	if doc["resource"] != "https://mcp.example.com/mcp/gateway" {
		t.Fatalf("resource = %v", doc["resource"])
	}
	if scopes, _ := doc["scopes_supported"].([]any); len(scopes) != 2 {
		t.Fatalf("scopes_supported = %v", doc["scopes_supported"])
	}
	// Without any listed audience, only a token whose aud names the resource gets in.
	if r, body := mcpPost(t, ts.URL+"/mcp/gateway", idp.token(t, "alice-sub", nil), toolsList); r.StatusCode != http.StatusUnauthorized || errorCode(body) != "invalid_audience" || !strings.Contains(errorMessage(body), "https://mcp.example.com/mcp") {
		t.Fatalf("unmapped token: status=%d code=%q message=%q", r.StatusCode, errorCode(body), errorMessage(body))
	}
	if r, body := mcpPost(t, ts.URL+"/mcp/gateway", idp.token(t, "alice-sub", map[string]any{"aud": "https://mcp.example.com/mcp"}), toolsList); r.StatusCode != http.StatusOK {
		t.Fatalf("mapped token: status=%d body=%v", r.StatusCode, body)
	}
	if r, _ := mcpPost(t, ts.URL+"/mcp", "", toolsList); r.Header.Get("WWW-Authenticate") != `Bearer realm="Data Works", resource_metadata="https://mcp.example.com/.well-known/oauth-protected-resource/mcp"` {
		t.Fatalf("WWW-Authenticate = %q", r.Header.Get("WWW-Authenticate"))
	}
}

func TestMCPOAuthSettingValidation(t *testing.T) {
	for value, ok := range map[string]bool{
		"": true, "https://dw.example.com/mcp": true, "https://dw.example.com/mcp/": true, "http://localhost:8080/mcp": true,
		"dw.example.com/mcp": false, "https://dw.example.com": false, "https://dw.example.com/mcp/gateway": false,
		"https://dw.example.com/mcp?x=1": false, "https://user:pw@dw.example.com/mcp": false, "ftp://dw.example.com/mcp": false,
	} {
		if err := mcpOAuthResource(value); (err == nil) != ok {
			t.Errorf("mcpOAuthResource(%q) err=%v, want ok=%v", value, err, ok)
		}
	}
	if err := mcpOAuthScopes("mcp:use models:read"); err != nil {
		t.Errorf("known scopes rejected: %v", err)
	}
	if err := mcpOAuthScopes("mcp:use everything"); err == nil {
		t.Error("unknown scope accepted")
	}
	if err := mcpOAuthAudience(`claude-mcp "x"`); err == nil {
		t.Error("quoted audience accepted")
	}
	def, _ := settingDefByKey("mcp.oauth.enabled")
	if settingPermissionGroup(def) != "security" {
		t.Errorf("mcp.oauth.* permission group = %s, want security", settingPermissionGroup(def))
	}
	for _, key := range []string{"mcp.oauth.enabled", "mcp.oauth.resource", "mcp.oauth.audience", "mcp.oauth.scopes"} {
		if settingDescriptions[key] == "" {
			t.Errorf("%s has no description", key)
		}
	}
}

func TestMCPOAuthStatusForAdmin(t *testing.T) {
	idp := newFakeIdP(t)
	server, db, ts := mcpOAuthServer(t, idp)
	setMCPOAuth(t, server, db, map[string]string{"mcp.oauth.enabled": "true", "mcp.oauth.audience": "claude-mcp cursor"})
	resp, err := http.Get(ts.URL + "/admin/mcp/oauth/status")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status without admin credentials = %d, want 401", resp.StatusCode)
	}
	r := httptest.NewRequest(http.MethodGet, "https://dw.example.com/admin/mcp/oauth/status", nil)
	w := httptest.NewRecorder()
	server.cfg.Auth.Enabled = false // the handler's own auth gate is exercised above; read the body directly
	server.handleMCPOAuthStatus(w, r)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["active"] != true || body["resource"] != "https://dw.example.com/mcp" || body["resource_source"] != "redirect_uri" || body["metadata_url"] != "https://dw.example.com/.well-known/oauth-protected-resource/mcp" || body["issuer"] != idp.issuer {
		t.Fatalf("status = %v", body)
	}
	if auds, _ := body["audiences"].([]any); len(auds) != 2 {
		t.Fatalf("audiences = %v", body["audiences"])
	}
	if endpoints, _ := body["endpoints"].([]any); len(endpoints) != 2 {
		t.Fatalf("endpoints = %v", body["endpoints"])
	}
}
