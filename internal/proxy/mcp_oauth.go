package proxy

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"dataworks/internal/store"
)

// MCP 를 SSO 로 — 개인 키 없이, Keycloak 이 발급한 액세스 토큰으로.
//
// The MCP authorization specification (2025-06-18 and later) is OAuth 2.1: this server
// is a *resource server* that publishes where its authorization server is (RFC 9728,
// /.well-known/oauth-protected-resource) and a client refused with 401 reads that
// document, sends the person through Keycloak with PKCE, and returns with an access
// token whose audience (RFC 8707) is this server. Nothing about issuing tokens happens
// here — Keycloak does that — so this file answers two questions only: where is the
// authorization server, and is this token one it issued for us.
//
// The API key stays. A token from SSO is a second door into the same room: it
// authenticates an *existing, active* Data Works account, carries the scopes the
// administrator chose (never wider than the person's role), and is accepted on the MCP
// endpoints only. It never creates an account, never revives a suspended one, and never
// reads a role out of the token.

const (
	// mcpOAuthDefaultScopes is what an SSO subject may do unless the administrator
	// narrows it: exactly the scope the MCP endpoints require of an API key.
	mcpOAuthDefaultScopes = "mcp:use"
	// mcpOAuthMetadataPath is the RFC 9728 well-known prefix. The bare path and the
	// path-suffixed forms (…/mcp, …/mcp/gateway) all serve the document.
	mcpOAuthMetadataPath = "/.well-known/oauth-protected-resource"
	mcpOAuthRealm        = "Data Works"
	// mcpOAuthClockSkew tolerates small clock differences between Keycloak and this host.
	mcpOAuthClockSkew = 60 * time.Second
	// mcpOAuthMaxTokenBytes bounds what the verifier will even look at.
	mcpOAuthMaxTokenBytes = 32 * 1024
)

// mcpOAuthEndpoints are the MCP paths a token may open, with the resource suffix each
// one adds to the base identifier (<origin>/mcp).
var mcpOAuthEndpoints = []string{"/mcp", "/mcp/gateway"}

// mcpOAuthConfig is the admin-settings slice (mcp.oauth.*).
type mcpOAuthConfig struct {
	Enabled bool
	// Resource is the identifier this deployment claims for /mcp (RFC 8707). Empty
	// means it is derived: from the Keycloak redirect URI's origin, else the request.
	Resource string
	// Audiences are the values an administrator accepts in aud or azp, on top of the
	// resource identifier itself.
	Audiences []string
	// Scopes are what a valid token may do, before the intersection with the account's
	// role. The administrator states the ceiling once instead of teaching Keycloak this
	// application's vocabulary.
	Scopes []string
}

// mcpOAuthState is the effective state: configuration plus the pieces reused from the
// Keycloak SSO settings, and why it is inactive when it is.
type mcpOAuthState struct {
	mcpOAuthConfig
	Issuer  string
	Problem string
}

// Active reports whether tokens are accepted and metadata is served.
func (st mcpOAuthState) Active() bool { return st.Enabled && st.Problem == "" }

// mcpOAuthConf returns the stored configuration. A server without a snapshot, such as
// one built directly in tests, has OAuth off.
func (s *Server) mcpOAuthConf() mcpOAuthConfig {
	if s == nil {
		return mcpOAuthConfig{}
	}
	if p := s.oauthRuntime.Load(); p != nil {
		return *p
	}
	return mcpOAuthConfig{}
}

// mcpOAuthConfigFrom builds the configuration from the stored settings, applying the
// registry defaults for anything unset.
func (s *Server) mcpOAuthConfigFrom(stored map[string]store.AdminSetting) mcpOAuthConfig {
	values := map[string]string{}
	for _, d := range settingRegistry {
		if !strings.HasPrefix(d.Key, "mcp.oauth.") {
			continue
		}
		value, _ := s.effectiveSettingValue(stored, d)
		values[d.Key] = value
	}
	return mcpOAuthConfig{
		Enabled:   strings.EqualFold(strings.TrimSpace(values["mcp.oauth.enabled"]), "true"),
		Resource:  strings.TrimRight(strings.TrimSpace(values["mcp.oauth.resource"]), "/"),
		Audiences: strings.Fields(values["mcp.oauth.audience"]),
		Scopes:    strings.Fields(values["mcp.oauth.scopes"]),
	}
}

// mcpOAuth returns the effective state. The issuer is the web sign-in's: one Keycloak
// realm signs both the browser session and the MCP token.
func (s *Server) mcpOAuth() mcpOAuthState {
	st := mcpOAuthState{mcpOAuthConfig: s.mcpOAuthConf()}
	st.Issuer = strings.TrimRight(strings.TrimSpace(s.keycloakConfig().IssuerURL), "/")
	if len(st.Scopes) == 0 {
		st.Scopes = strings.Fields(mcpOAuthDefaultScopes)
	}
	if st.Enabled && st.Issuer == "" {
		st.Problem = "Keycloak Issuer URL(oidc.issuer_url)이 비어 있어 SSO 토큰을 검사할 수 없습니다. Keycloak SSO 설정에서 Issuer URL 을 먼저 저장하세요."
	}
	return st
}

// mcpOAuthResourceBase returns the base resource identifier (<origin>/mcp) and where
// it came from: the setting, the Keycloak redirect URI's origin (a public address the
// administrator already had to write for the web sign-in), or — last resort, anyone can
// set a Host header — the request.
func (s *Server) mcpOAuthResourceBase(r *http.Request) (string, string) {
	if resource := s.mcpOAuthConf().Resource; resource != "" {
		return resource, "setting"
	}
	if redirect, err := url.Parse(strings.TrimSpace(s.keycloakConfig().RedirectURI)); err == nil && redirect.Host != "" && (redirect.Scheme == "http" || redirect.Scheme == "https") {
		return redirect.Scheme + "://" + redirect.Host + "/mcp", "redirect_uri"
	}
	if r != nil {
		return requestOrigin(r) + "/mcp", "request"
	}
	return "", ""
}

// mcpOAuthResourceFor maps an MCP endpoint path onto the base identifier: /mcp is the
// base itself, /mcp/gateway is base + /gateway.
func mcpOAuthResourceFor(base, endpoint string) string {
	return base + strings.TrimPrefix(endpoint, "/mcp")
}

// mcpOAuthMetadataURL is where a refused client is sent to learn the above, for the
// endpoint it was refused on (RFC 9728 path-suffixed form).
func mcpOAuthMetadataURL(base, endpoint string) string {
	return strings.TrimSuffix(base, "/mcp") + mcpOAuthMetadataPath + endpoint
}

// mcpOAuthEndpoint reports whether a request path is one of the MCP endpoints a token
// may open. Everything else (REST, admin, web) keeps keys and sessions only.
func mcpOAuthEndpoint(path string) bool {
	for _, endpoint := range mcpOAuthEndpoints {
		if path == endpoint {
			return true
		}
	}
	return false
}

// looksLikeJWT is the cheap shape test that separates "not a key" from "not a token of
// any kind we accept": three non-empty dot-separated parts.
func looksLikeJWT(token string) bool {
	if len(token) > mcpOAuthMaxTokenBytes {
		return false
	}
	parts := strings.Split(token, ".")
	return len(parts) == 3 && parts[0] != "" && parts[1] != "" && parts[2] != ""
}

// mcpOAuthResource validates mcp.oauth.resource: empty, or an absolute http(s) URL whose
// path is exactly /mcp — the public address plus the MCP path, nothing else.
func mcpOAuthResource(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	parsed, err := url.Parse(v)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.ContainsAny(v, " \t\r\n\"'<>\\") {
		return fmt.Errorf("must be an absolute http(s) URL such as https://dataworks.example.com/mcp")
	}
	if strings.TrimRight(parsed.Path, "/") != "/mcp" {
		return fmt.Errorf("must be the public address plus /mcp, e.g. https://dataworks.example.com/mcp")
	}
	return nil
}

// mcpOAuthAudience validates mcp.oauth.audience: space-separated client ids or URLs
// with nothing that could confuse a claim comparison.
func mcpOAuthAudience(v string) error {
	for _, item := range strings.Fields(v) {
		if len(item) > 256 || strings.ContainsAny(item, "\"'<>\\") {
			return fmt.Errorf("%q must be a client id or resource URL without quotes", item)
		}
	}
	return nil
}

// mcpOAuthScopes validates mcp.oauth.scopes against this application's scope vocabulary.
func mcpOAuthScopes(v string) error {
	for _, scope := range strings.Fields(v) {
		if !hasScope(allScopes, scope) {
			return fmt.Errorf("unknown scope %q (allowed: %s)", scope, strings.Join(allScopes, " "))
		}
	}
	return nil
}

// mcpOAuthRefusal says exactly why a token will not do, in words an operator can act on.
type mcpOAuthRefusal struct {
	code    string
	message string
}

func (e *mcpOAuthRefusal) Error() string { return e.code + ": " + e.message }

// mcpOAuthAlgs are the signature algorithms accepted from the realm's JWKS. Symmetric
// (HS*) and "none" never are: the realm publishes public keys only.
var mcpOAuthAlgs = map[string]crypto.Hash{
	"RS256": crypto.SHA256, "RS384": crypto.SHA384, "RS512": crypto.SHA512,
	"PS256": crypto.SHA256, "PS384": crypto.SHA384, "PS512": crypto.SHA512,
	"ES256": crypto.SHA256, "ES384": crypto.SHA384, "ES512": crypto.SHA512,
}

// mcpOAuthVerify checks signature (JWKS), issuer, exp, nbf, typ, cnf and sub. The
// audience is checked by the caller because more than one value is acceptable and the
// refusal must say what it saw.
func (s *Server) mcpOAuthVerify(ctx context.Context, st mcpOAuthState, token string) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed jwt")
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	hb, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(hb, &header) != nil {
		return nil, errors.New("bad jwt header")
	}
	hash, ok := mcpOAuthAlgs[header.Alg]
	if !ok {
		return nil, errors.New("unsupported jwt alg " + header.Alg)
	}
	disc, err := keycloakDiscover(ctx, st.Issuer)
	if err != nil {
		return nil, fmt.Errorf("discovery: %w", err)
	}
	key, err := keycloakJWKSPublicKey(ctx, disc.JWKSURI, header.Kid)
	if err != nil {
		return nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("bad jwt signature encoding")
	}
	h := hash.New()
	h.Write([]byte(parts[0] + "." + parts[1]))
	digest := h.Sum(nil)
	switch pub := key.(type) {
	case *rsa.PublicKey:
		if strings.HasPrefix(header.Alg, "PS") {
			err = rsa.VerifyPSS(pub, hash, digest, sig, nil)
		} else if strings.HasPrefix(header.Alg, "RS") {
			err = rsa.VerifyPKCS1v15(pub, hash, digest, sig)
		} else {
			err = errors.New("alg does not match key type")
		}
	case *ecdsa.PublicKey:
		if !strings.HasPrefix(header.Alg, "ES") {
			err = errors.New("alg does not match key type")
			break
		}
		size := (pub.Curve.Params().BitSize + 7) / 8
		if len(sig) != 2*size {
			err = errors.New("bad ecdsa signature length")
			break
		}
		if !ecdsa.Verify(pub, digest, new(big.Int).SetBytes(sig[:size]), new(big.Int).SetBytes(sig[size:])) {
			err = errors.New("ecdsa verification failed")
		}
	default:
		err = errors.New("unsupported key type")
	}
	if err != nil {
		return nil, errors.New("jwt signature verification failed")
	}
	pb, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, errors.New("bad jwt payload")
	}
	var claims map[string]any
	if err := json.Unmarshal(pb, &claims); err != nil {
		return nil, errors.New("bad jwt claims")
	}
	if iss, _ := claims["iss"].(string); strings.TrimRight(iss, "/") != st.Issuer {
		return nil, errors.New("jwt issuer mismatch")
	}
	now := time.Now()
	exp, ok := claims["exp"].(float64)
	if !ok {
		return nil, errors.New("jwt missing exp")
	}
	if now.Add(-mcpOAuthClockSkew).After(time.Unix(int64(exp), 0)) {
		return nil, errors.New("jwt expired")
	}
	if nbf, ok := claims["nbf"].(float64); ok && now.Add(mcpOAuthClockSkew).Before(time.Unix(int64(nbf), 0)) {
		return nil, errors.New("jwt not yet valid")
	}
	// Keycloak access tokens carry typ=Bearer and ID tokens typ=ID. An ID token proves a
	// login happened; it is not an API credential.
	if raw, exists := claims["typ"]; exists {
		typ, _ := raw.(string)
		if strings.EqualFold(strings.TrimSpace(typ), "ID") {
			return nil, errors.New("id token presented as access token")
		}
		if !strings.EqualFold(strings.TrimSpace(typ), "Bearer") {
			return nil, errors.New("unsupported token typ " + typ)
		}
	}
	// cnf binds the token to a proof of possession (DPoP, mTLS) this server cannot
	// verify; accepting it as a bare bearer would defeat that binding.
	if _, bound := claims["cnf"]; bound {
		return nil, errors.New("sender-constrained token (cnf) not supported")
	}
	if strings.TrimSpace(strClaim(claims, "sub")) == "" {
		return nil, errors.New("jwt missing sub")
	}
	return claims, nil
}

// mcpOAuthBound lists the parties a token names as its target: every aud value (a JWT
// aud is a string or an array of strings) plus azp.
func mcpOAuthBound(claims map[string]any) (aud []string, azp string) {
	if single, ok := claims["aud"].(string); ok && single != "" {
		aud = []string{single}
	} else {
		aud = toStringSlice(claims["aud"])
	}
	return aud, strings.TrimSpace(strClaim(claims, "azp"))
}

// mcpOAuthAuthenticate turns a bearer access token presented to an MCP endpoint into
// the same identity an API key of that person would carry, or says exactly why not.
func (s *Server) mcpOAuthAuthenticate(r *http.Request, token string) (string, *store.AuthContext, error) {
	ctx := r.Context()
	st := s.mcpOAuth()
	if !st.Active() {
		return "", nil, &mcpOAuthRefusal{"mcp_oauth_disabled", "이 서버의 MCP 는 SSO 액세스 토큰을 받지 않습니다. API 키를 쓰거나 관리자가 MCP SSO(OAuth) 인증을 켜야 합니다."}
	}
	claims, err := s.mcpOAuthVerify(ctx, st, token)
	if err != nil {
		if strings.HasPrefix(err.Error(), "discovery:") {
			slog.Warn("mcp oauth discovery failed", "issuer", st.Issuer, "error", err)
			return "", nil, &mcpOAuthRefusal{"mcp_oauth_unavailable", "Keycloak 발급자 정보를 읽지 못해 SSO 토큰을 확인할 수 없습니다. 잠시 후 다시 시도하거나 관리자에게 알리세요."}
		}
		return "", nil, &mcpOAuthRefusal{"invalid_token", "SSO 액세스 토큰이 유효하지 않습니다(" + err.Error() + "). 클라이언트에서 다시 로그인하세요."}
	}
	// Whom the token was minted for. A real Keycloak 26 puts the requesting client in
	// azp and only "account" in aud unless an Audience mapper adds more, so the binding
	// is "aud names this resource, or aud/azp is a party the administrator listed".
	base, _ := s.mcpOAuthResourceBase(r)
	resource := mcpOAuthResourceFor(base, r.URL.Path)
	accepted := append([]string{base, resource}, st.Audiences...)
	aud, azp := mcpOAuthBound(claims)
	bound := append(append([]string{}, aud...), azp)
	matched := false
	for _, value := range bound {
		if value != "" && hasScope(accepted, value) {
			matched = true
			break
		}
	}
	if !matched {
		return "", nil, &mcpOAuthRefusal{"invalid_audience", fmt.Sprintf(
			"SSO 토큰이 이 서버를 위해 발급된 것이 아닙니다(aud=%v, azp=%q). 관리자가 mcp.oauth.audience 에 %q 를 더하거나, Keycloak 클라이언트의 Audience 매퍼에 %q 를 넣어야 합니다.",
			aud, azp, azp, base)}
	}
	// The same lookup the web sign-in uses, without the provisioning half: the linked
	// subject first, then this application's username (the e-mail). Nothing is written.
	subject := strings.TrimSpace(strClaim(claims, "sub"))
	var user store.AuthUser
	found := false
	if id, ok, _ := s.db.AuthIdentityBySubject(ctx, "keycloak", s.keycloakConfig().IssuerURL, subject); ok {
		user, found, _ = s.db.AuthUserByID(ctx, id.UserID)
	}
	if !found {
		if email := strings.TrimSpace(strClaim(claims, "email")); email != "" {
			user, found, _ = s.db.AuthUserByEmail(ctx, email)
		}
	}
	if !found || user.Status != "active" {
		return "", nil, &mcpOAuthRefusal{"account_not_registered", "이 SSO 계정은 Data Works 에 등록되지 않았거나 비활성입니다. 먼저 웹으로 한 번 로그인하세요."}
	}
	// Never wider than a key: the administrator's ceiling, cut to the person's role, and
	// cut again when the token itself speaks this application's vocabulary.
	scopes := intersectScopes(st.Scopes, s.effectiveScopesForRole(ctx, user.Role))
	if spoken := intersectScopes(strings.Fields(strClaim(claims, "scope")), allScopes); len(spoken) > 0 {
		scopes = intersectScopes(scopes, spoken)
	}
	// Both MCP endpoints are MCP: the scope the gateway asks of a key on /mcp is what an
	// SSO subject needs on either (an empty intersection opens nothing).
	if s.cfg.Auth.Enabled && !hasScope(scopes, "mcp:use") {
		return "", nil, &mcpOAuthRefusal{"insufficient_scope", "이 계정의 SSO 토큰에는 mcp:use 범위가 없습니다. 관리자가 mcp.oauth.scopes 와 계정 역할을 확인해야 합니다."}
	}
	teamID, _ := s.db.PrimaryTeamForUser(ctx, user.ID)
	authCtx := store.AuthContext{UserID: user.ID, TeamID: teamID, Role: user.Role, Scopes: scopes}
	s.enrichAuthContextTeam(ctx, &authCtx)
	return "oauth_" + user.ID, &authCtx, nil
}

func intersectScopes(a, b []string) []string {
	out := []string{}
	for _, scope := range a {
		if hasScope(b, scope) && !hasScope(out, scope) {
			out = append(out, scope)
		}
	}
	return out
}

// writeMCPUnauthorized answers a refused MCP request. While OAuth is active the 401
// carries WWW-Authenticate with resource_metadata, which is what turns the refusal into
// an invitation: the client reads the document and starts the sign-in from there. The
// header is set on MCP paths only; a REST 401 with it would send browsers astray.
func (s *Server) writeMCPUnauthorized(w http.ResponseWriter, r *http.Request, reason error) {
	if st := s.mcpOAuth(); st.Active() && mcpOAuthEndpoint(r.URL.Path) {
		base, _ := s.mcpOAuthResourceBase(r)
		challenge := fmt.Sprintf(`Bearer realm=%q, resource_metadata=%q`, mcpOAuthRealm, mcpOAuthMetadataURL(base, r.URL.Path))
		if bearerToken(r.Header.Get("Authorization")) != "" {
			challenge += `, error="invalid_token"`
		}
		w.Header().Set("WWW-Authenticate", challenge)
	}
	var refusal *mcpOAuthRefusal
	if errors.As(reason, &refusal) {
		writeOpenAIError(w, http.StatusUnauthorized, refusal.message, "invalid_request_error", refusal.code)
		return
	}
	writeOpenAIError(w, http.StatusUnauthorized, "invalid proxy API key", "invalid_request_error", "invalid_api_key")
}

// handleMCPOAuthMetadata serves RFC 9728: the document a refused MCP client reads to find
// the authorization server. Public by design — it says where to sign in, not who is
// signed in — and bare JSON, because the reader is an OAuth client library. 404 while
// off: metadata that points at a server refusing every token is a login loop.
// GET /.well-known/oauth-protected-resource[/mcp[/gateway]]
func (s *Server) handleMCPOAuthMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	endpoint := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, mcpOAuthMetadataPath), "/")
	if endpoint == "" {
		endpoint = "/mcp"
	}
	if !mcpOAuthEndpoint(endpoint) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	st := s.mcpOAuth()
	if !st.Active() {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "mcp_oauth_disabled", "error_description": "이 서버의 MCP 는 SSO 토큰을 받지 않습니다. API 키를 사용하세요."})
		return
	}
	base, _ := s.mcpOAuthResourceBase(r)
	w.Header().Set("Cache-Control", "public, max-age=300")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"resource":                 mcpOAuthResourceFor(base, endpoint),
		"authorization_servers":    []string{st.Issuer},
		"bearer_methods_supported": []string{"header"},
		"scopes_supported":         st.Scopes,
		"resource_name":            "Data Works MCP",
	})
}

// handleMCPOAuthStatus tells the administrator what the configuration amounts to and
// hands over the values a client needs (MCP URL, metadata URL) ready to copy.
// GET /admin/mcp/oauth/status
func (s *Server) handleMCPOAuthStatus(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdminAuthorization(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	st := s.mcpOAuth()
	base, source := s.mcpOAuthResourceBase(r)
	endpoints := make([]map[string]string, 0, len(mcpOAuthEndpoints))
	for _, endpoint := range mcpOAuthEndpoints {
		endpoints = append(endpoints, map[string]string{
			"path":         endpoint,
			"url":          mcpOAuthResourceFor(base, endpoint),
			"resource":     mcpOAuthResourceFor(base, endpoint),
			"metadata_url": mcpOAuthMetadataURL(base, endpoint),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":         st.Enabled,
		"active":          st.Active(),
		"problem":         st.Problem,
		"issuer":          st.Issuer,
		"web_client_id":   strings.TrimSpace(s.keycloakConfig().ClientID),
		"resource":        base,
		"resource_source": source,
		"metadata_url":    mcpOAuthMetadataURL(base, "/mcp"),
		"endpoints":       endpoints,
		"audiences":       nonNilStrings(st.Audiences),
		"scopes":          nonNilStrings(st.Scopes),
	})
}
