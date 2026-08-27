package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"dataworks/internal/config"
	"dataworks/internal/secret"
	"dataworks/internal/store"
)

func TestKeycloakConfigDBOverlayAndSecretAtRest(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	ctx := context.Background()

	cipher, err := secret.New("unit-test-passphrase")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		cfg: config.Config{Keycloak: config.KeycloakConfig{
			Enabled: false, IssuerURL: "https://env-issuer/realms/x", ClientID: "env-client",
			ClientSecret: "env-secret", DefaultRole: "developer", Scopes: []string{"openid"},
		}},
		db: db,
	}
	s.secrets.Store(cipher)

	// No DB row yet → effective config equals env baseline.
	s.reloadKeycloakConfig(ctx)
	if got := s.keycloakConfig(); got.ClientID != "env-client" || got.ClientSecret != "env-secret" || got.Enabled {
		t.Fatalf("env baseline expected, got %+v", got)
	}

	// Persist a DB override with an encrypted client secret.
	enc, err := cipher.Encrypt("db-top-secret")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SaveSSOProviderConfig(ctx, store.SSOProviderConfig{
		Provider: "keycloak", Enabled: true, IssuerURL: "https://db-issuer/realms/y",
		ClientID: "db-client", ClientSecretEnc: enc, RedirectURI: "https://gw/cb",
		Scopes: []string{"openid", "profile"}, DefaultRole: "team_admin",
		RoleClaim: "realm_access.roles", GroupClaim: "groups", AllowLocalLogin: false,
		UpdatedBy: "admin@x.com",
	}); err != nil {
		t.Fatal(err)
	}

	// The raw row must NOT contain the plaintext secret.
	rec, found, err := db.GetSSOProviderConfig(ctx, "keycloak")
	if err != nil || !found {
		t.Fatalf("get config: found=%v err=%v", found, err)
	}
	if rec.ClientSecretEnc == "db-top-secret" || rec.ClientSecretEnc == "" {
		t.Fatalf("client secret must be stored encrypted, got %q", rec.ClientSecretEnc)
	}

	// After reload, the effective config reflects the DB row with the secret decrypted.
	s.reloadKeycloakConfig(ctx)
	got := s.keycloakConfig()
	if !got.Enabled || got.ClientID != "db-client" || got.IssuerURL != "https://db-issuer/realms/y" {
		t.Fatalf("db overlay not applied: %+v", got)
	}
	if got.ClientSecret != "db-top-secret" {
		t.Fatalf("client secret should decrypt to plaintext, got %q", got.ClientSecret)
	}
	if got.DefaultRole != "team_admin" || got.AllowLocalLogin {
		t.Fatalf("other fields not applied: %+v", got)
	}
}

func TestResolveKeycloakRoleWithCustomMap(t *testing.T) {
	// Empty/nil map falls back to the built-in defaults.
	if got := resolveKeycloakRoleWith(nil, []string{"vibe-admin"}, "developer"); got != "admin" {
		t.Errorf("nil map should use defaults, got %q", got)
	}
	// A custom map overrides the defaults (here vibe-admin is downgraded; a custom role wins).
	custom := map[string]string{"sso-superuser": "admin", "vibe-admin": "developer"}
	if got := resolveKeycloakRoleWith(custom, []string{"sso-superuser", "vibe-admin"}, "developer"); got != "admin" {
		t.Errorf("custom map: highest-rank mapped role should win, got %q", got)
	}
	// A role only present in the default map is unknown under a custom map → default role.
	if got := resolveKeycloakRoleWith(custom, []string{"vibe-auditor"}, "developer"); got != "developer" {
		t.Errorf("unmapped role should fall back to default, got %q", got)
	}
}

func TestEffectiveKeycloakRoleMap(t *testing.T) {
	s := &Server{cfg: config.Config{Keycloak: config.KeycloakConfig{}}}
	// No overlay → built-in defaults.
	if s.effectiveKeycloakRoleMap()["vibe-admin"] != "admin" {
		t.Error("default map expected without overlay")
	}
	// Overlay with a custom map → custom map returned.
	s.keycloakCfg.Store(&config.KeycloakConfig{RoleMap: map[string]string{"x": "team_admin"}})
	m := s.effectiveKeycloakRoleMap()
	if m["x"] != "team_admin" || len(m) != 1 {
		t.Errorf("custom overlay map expected, got %v", m)
	}
}

func TestKeycloakConfigRoleGrantCeiling(t *testing.T) {
	_, proxy := newAuthTestServer(t, "http://example.invalid")
	defer proxy.Close()

	login := func(email, password string) string {
		t.Helper()
		resp := postJSON(t, proxy.URL+"/auth/login", "", map[string]string{"email": email, "password": password})
		defer resp.Body.Close()
		var out struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || out.AccessToken == "" {
			t.Fatalf("login %s status=%d token_set=%v", email, resp.StatusCode, out.AccessToken != "")
		}
		return out.AccessToken
	}
	rootToken := login("root@example.com", "correct-password")
	created := postJSON(t, proxy.URL+"/admin/users", rootToken, map[string]string{
		"email": "sso-admin@example.com", "password": "admin-password", "name": "SSO Admin", "role": "admin",
	})
	created.Body.Close()
	if created.StatusCode != http.StatusCreated {
		t.Fatalf("create admin status=%d", created.StatusCode)
	}
	adminToken := login("sso-admin@example.com", "admin-password")

	putConfig := func(token, defaultRole string, roleMap map[string]string) *http.Response {
		t.Helper()
		body, err := json.Marshal(map[string]any{
			"enabled": false, "issuer_url": "", "client_id": "", "redirect_uri": "",
			"scopes": []string{"openid", "profile", "email"}, "default_role": defaultRole,
			"role_claim": "realm_access.roles", "group_claim": "groups", "allow_local_login": true,
			"role_map": roleMap,
		})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequest(http.MethodPut, proxy.URL+"/admin/sso/keycloak/config", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	for name, tc := range map[string]struct {
		defaultRole string
		roleMap     map[string]string
		want        int
	}{
		"default super admin":  {defaultRole: "super_admin", roleMap: map[string]string{}, want: http.StatusForbidden},
		"mapped super admin":   {defaultRole: "developer", roleMap: map[string]string{"attacker-role": "super_admin"}, want: http.StatusForbidden},
		"new peer admin map":   {defaultRole: "developer", roleMap: map[string]string{"attacker-role": "admin"}, want: http.StatusForbidden},
		"unknown default role": {defaultRole: "not-a-role", roleMap: map[string]string{}, want: http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			resp := putConfig(adminToken, tc.defaultRole, tc.roleMap)
			resp.Body.Close()
			if resp.StatusCode != tc.want {
				t.Fatalf("status=%d want=%d", resp.StatusCode, tc.want)
			}
		})
	}

	// The settings UI posts the effective built-in map. An ordinary admin may
	// preserve its existing peer-admin entry while changing unrelated SSO fields.
	preserve := putConfig(adminToken, "developer", map[string]string{
		"vibe-admin": "admin", "vibe-team-admin": "team_admin",
		"vibe-developer": "developer", "vibe-auditor": "readonly_admin",
	})
	preserve.Body.Close()
	if preserve.StatusCode != http.StatusNoContent {
		t.Fatalf("preserving built-in role map status=%d want=%d", preserve.StatusCode, http.StatusNoContent)
	}

	// A super admin remains able to establish the top-level break-glass mapping.
	super := putConfig(rootToken, "super_admin", map[string]string{"break-glass": "super_admin"})
	super.Body.Close()
	if super.StatusCode != http.StatusNoContent {
		t.Fatalf("super-admin config status=%d want=%d", super.StatusCode, http.StatusNoContent)
	}
}
