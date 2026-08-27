package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"dataworks/internal/store"
)

func meKeysServer(t *testing.T, selfService bool) (*httptest.Server, *store.SQLStore) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(upstream.Close)

	db := openTestStore(t)
	t.Cleanup(func() { db.Close() })
	logger := store.NewAsyncLogger(db, 32, filepath.Join(t.TempDir(), "fallback.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(context.Background()) })

	cfg := testConfig(upstream.URL, "secret")
	cfg.Auth.SelfServiceKeys = selfService
	cfg.Auth.APIKeyPrefix = "vc_sk_"
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Seed a primary key for user u1 (developer scopes) — its plaintext is "usersecret".
	if err := db.UpsertAPIKey(context.Background(), store.APIKeyRecord{
		ID: "key_primary_u1", Name: "u1 primary", KeyHash: hashProxyKey("usersecret"),
		UserID: "u1", Team: "t1", Role: "developer", Status: "active",
		Scopes: []string{"chat:completion", "models:read"},
	}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(server.Routes())
	t.Cleanup(ts.Close)
	return ts, db
}

func TestSelfServiceKeysDisabled(t *testing.T) {
	ts, _ := meKeysServer(t, false)
	resp := postJSON(t, ts.URL+"/me/keys", "usersecret", map[string]any{"name": "cli"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("disabled self-service should 404, got %d", resp.StatusCode)
	}
}

func TestSelfServiceKeysLifecycle(t *testing.T) {
	ts, _ := meKeysServer(t, true)

	// Create: inherits caller scopes when none requested.
	resp := postJSON(t, ts.URL+"/me/keys", "usersecret", map[string]any{"name": "cli key"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create should 201, got %d", resp.StatusCode)
	}
	var created struct {
		APIKey struct {
			ID     string   `json:"id"`
			UserID string   `json:"user_id"`
			Role   string   `json:"role"`
			Scopes []string `json:"scopes"`
		} `json:"api_key"`
		Secret string `json:"secret"`
	}
	json.NewDecoder(resp.Body).Decode(&created)
	resp.Body.Close()
	if created.Secret == "" || created.APIKey.UserID != "u1" || created.APIKey.Role != "developer" {
		t.Fatalf("unexpected created key: %+v", created.APIKey)
	}
	if len(created.APIKey.Scopes) != 2 {
		t.Errorf("new key should inherit caller's 2 scopes, got %v", created.APIKey.Scopes)
	}

	// Narrowing to a valid subset of the caller's scopes is allowed (role-appropriate).
	sub := postJSON(t, ts.URL+"/me/keys", "usersecret", map[string]any{"name": "narrow", "scopes": []string{"models:read"}})
	var narrowed struct {
		APIKey struct {
			Scopes []string `json:"scopes"`
		} `json:"api_key"`
	}
	json.NewDecoder(sub.Body).Decode(&narrowed)
	sub.Body.Close()
	if sub.StatusCode != http.StatusCreated || len(narrowed.APIKey.Scopes) != 1 || narrowed.APIKey.Scopes[0] != "models:read" {
		t.Fatalf("narrowing to a subset should 201 with that scope, got status %d %+v", sub.StatusCode, narrowed.APIKey.Scopes)
	}

	// Scope escalation is denied.
	esc := postJSON(t, ts.URL+"/me/keys", "usersecret", map[string]any{"name": "evil", "scopes": []string{"admin:write"}})
	defer esc.Body.Close()
	if esc.StatusCode != http.StatusForbidden {
		t.Fatalf("scope escalation should 403, got %d", esc.StatusCode)
	}

	// List: returns the caller's own keys, plus the caller's role and grantable scopes for the picker.
	listReq, _ := http.NewRequest(http.MethodGet, ts.URL+"/me/keys", nil)
	listReq.Header.Set("Authorization", "Bearer usersecret")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		APIKeys         []store.APIKeyPublic `json:"api_keys"`
		Role            string               `json:"role"`
		GrantableScopes []string             `json:"grantable_scopes"`
	}
	json.NewDecoder(listResp.Body).Decode(&listed)
	listResp.Body.Close()
	if listed.Role != "developer" {
		t.Errorf("expected role developer in list response, got %q", listed.Role)
	}
	if len(listed.GrantableScopes) != 2 {
		t.Errorf("expected 2 grantable scopes, got %v", listed.GrantableScopes)
	}
	// 3 own keys now: primary + created + narrowed.
	if len(listed.APIKeys) != 3 {
		t.Errorf("expected 3 own keys, got %d", len(listed.APIKeys))
	}
	for _, k := range listed.APIKeys {
		if k.UserID != "u1" {
			t.Errorf("listed a key not owned by caller: %+v", k)
		}
	}

	// PATCH scopes of an existing key to a valid subset → 200.
	patchOK := patchJSON(t, ts.URL+"/me/keys/"+created.APIKey.ID, "usersecret", map[string]any{"scopes": []string{"models:read"}})
	patchOK.Body.Close()
	if patchOK.StatusCode != http.StatusOK {
		t.Fatalf("scope edit to subset should 200, got %d", patchOK.StatusCode)
	}
	// PATCH with an escalated scope → 403.
	patchBad := patchJSON(t, ts.URL+"/me/keys/"+created.APIKey.ID, "usersecret", map[string]any{"scopes": []string{"admin:write"}})
	patchBad.Body.Close()
	if patchBad.StatusCode != http.StatusForbidden {
		t.Fatalf("scope edit escalation should 403, got %d", patchBad.StatusCode)
	}

	// Rotate the created key → new secret, old revoked.
	rot := postJSON(t, ts.URL+"/me/keys/"+created.APIKey.ID+"/rotate", "usersecret", map[string]any{})
	var rotated struct {
		RotatedFrom string `json:"rotated_from"`
		Secret      string `json:"secret"`
		APIKey      struct {
			ID string `json:"id"`
		} `json:"api_key"`
	}
	json.NewDecoder(rot.Body).Decode(&rotated)
	rot.Body.Close()
	if rot.StatusCode != http.StatusOK || rotated.Secret == "" || rotated.APIKey.ID == created.APIKey.ID {
		t.Fatalf("rotate should return a new key+secret, got status %d %+v", rot.StatusCode, rotated)
	}

	// Revoking a key not owned by the caller → 404 (ownership hidden).
	delReq, _ := http.NewRequest(http.MethodDelete, ts.URL+"/me/keys/key_does_not_exist", nil)
	delReq.Header.Set("Authorization", "Bearer usersecret")
	delResp, _ := http.DefaultClient.Do(delReq)
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusNotFound {
		t.Errorf("revoking unknown key should 404, got %d", delResp.StatusCode)
	}
}

func TestSelfServiceKeyPolicyPatchAndRotatePreserveConstraints(t *testing.T) {
	ts, db := meKeysServer(t, true)
	now := time.Now().UTC()
	parentExpiry := now.Add(24 * time.Hour)
	primary, found, err := db.GetAPIKey(context.Background(), "key_primary_u1")
	if err != nil || !found {
		t.Fatalf("primary lookup found=%v err=%v", found, err)
	}
	primary.AllowedIPs = []string{"127.0.0.0/8"}
	primary.AllowedModels = []string{"gpt-*"}
	primary.DeniedModels = []string{"gpt-danger"}
	primary.AllowedProviders = []string{"openai", "qwen"}
	primary.DeniedProviders = []string{"evil-provider"}
	primary.BudgetLimitKRW = 1000
	primary.ExpiresAt = parentExpiry
	if err := db.UpsertAPIKey(context.Background(), primary); err != nil {
		t.Fatal(err)
	}

	// Omitted policy fields inherit every constraint, not only scopes.
	createdResp := postJSON(t, ts.URL+"/me/keys", "usersecret", map[string]any{"name": "policy child"})
	var created struct {
		APIKey store.APIKeyPublic `json:"api_key"`
		Secret string             `json:"secret"`
	}
	if err := json.NewDecoder(createdResp.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	createdResp.Body.Close()
	if createdResp.StatusCode != http.StatusCreated || created.Secret == "" {
		t.Fatalf("create policy child = %d %+v", createdResp.StatusCode, created)
	}
	child, found, err := db.GetAPIKey(context.Background(), created.APIKey.ID)
	if err != nil || !found {
		t.Fatalf("child lookup found=%v err=%v", found, err)
	}
	if !reflect.DeepEqual(child.AllowedIPs, primary.AllowedIPs) || !reflect.DeepEqual(child.AllowedModels, primary.AllowedModels) ||
		!reflect.DeepEqual(child.DeniedModels, primary.DeniedModels) || child.BudgetLimitKRW != primary.BudgetLimitKRW ||
		!child.ExpiresAt.Equal(primary.ExpiresAt) {
		t.Fatalf("omitted policy did not inherit all constraints: child=%+v primary=%+v", child, primary)
	}

	childExpiry := now.Add(2 * time.Hour).UTC()
	patchResp := patchJSON(t, ts.URL+"/me/keys/"+child.ID, "usersecret", map[string]any{
		"scopes":            []string{}, // explicit empty means no permissions, never inheritance
		"allowed_ips":       []string{"127.0.0.1"},
		"allowed_models":    []string{"gpt-4.1"},
		"denied_models":     []string{"gpt-danger", "gpt-legacy"},
		"allowed_providers": []string{"openai"},
		"denied_providers":  []string{"evil-provider", "legacy-provider"},
		"budget_limit_krw":  250,
		"expires_at":        childExpiry.Format(time.RFC3339Nano),
	})
	if patchResp.StatusCode != http.StatusOK {
		defer patchResp.Body.Close()
		var body map[string]any
		_ = json.NewDecoder(patchResp.Body).Decode(&body)
		t.Fatalf("policy patch = %d body=%v", patchResp.StatusCode, body)
	}
	patchResp.Body.Close()
	child, found, err = db.GetAPIKey(context.Background(), child.ID)
	if err != nil || !found {
		t.Fatalf("patched child lookup found=%v err=%v", found, err)
	}
	if len(child.Scopes) != 0 || !reflect.DeepEqual(child.AllowedIPs, []string{"127.0.0.1"}) ||
		!reflect.DeepEqual(child.AllowedModels, []string{"gpt-4.1"}) ||
		!reflect.DeepEqual(child.DeniedModels, []string{"gpt-danger", "gpt-legacy"}) ||
		!reflect.DeepEqual(child.AllowedProviders, []string{"openai"}) ||
		!reflect.DeepEqual(child.DeniedProviders, []string{"evil-provider", "legacy-provider"}) ||
		child.BudgetLimitKRW != 250 || !child.ExpiresAt.Equal(childExpiry) {
		t.Fatalf("patched policy was not persisted exactly: %+v", child)
	}

	// Removing a caller allow/deny/budget/expiry bound would create a broader key and is denied.
	for name, body := range map[string]map[string]any{
		"remove allowed models": {"allowed_models": []string{}},
		"drop parent deny":      {"denied_models": []string{"gpt-legacy"}},
		"raise budget":          {"budget_limit_krw": 1001},
		"remove expiry":         {"expires_at": ""},
	} {
		t.Run(name, func(t *testing.T) {
			resp := patchJSON(t, ts.URL+"/me/keys/"+child.ID, "usersecret", body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Fatalf("policy escalation = %d, want 403", resp.StatusCode)
			}
		})
	}

	// A real other-user key is hidden from PATCH as 404.
	if err := db.UpsertAPIKey(context.Background(), store.APIKeyRecord{
		ID: "key_other_user", Name: "other", KeyHash: hashProxyKey("othersecret"), UserID: "u2", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	other := patchJSON(t, ts.URL+"/me/keys/key_other_user", "usersecret", map[string]any{"scopes": []string{}})
	other.Body.Close()
	if other.StatusCode != http.StatusNotFound {
		t.Fatalf("other user's key patch = %d, want 404", other.StatusCode)
	}
	if err := db.UpsertAPIKey(context.Background(), store.APIKeyRecord{
		ID: "key_higher_role", Name: "higher", KeyHash: hashProxyKey("highersecret"), UserID: "u1", Team: "t1",
		Role: "admin", Status: "active", Scopes: []string{},
	}); err != nil {
		t.Fatal(err)
	}
	higher := postJSON(t, ts.URL+"/me/keys/key_higher_role/rotate", "usersecret", map[string]any{})
	higher.Body.Close()
	if higher.StatusCode != http.StatusForbidden {
		t.Fatalf("lower-role key rotating higher-role sibling = %d, want 403", higher.StatusCode)
	}

	// Rotation preserves the complete key identity/policy and atomically revokes the old row.
	rotateResp := postJSON(t, ts.URL+"/me/keys/"+child.ID+"/rotate", "usersecret", map[string]any{})
	var rotated struct {
		RotatedFrom string             `json:"rotated_from"`
		APIKey      store.APIKeyPublic `json:"api_key"`
		Secret      string             `json:"secret"`
	}
	if err := json.NewDecoder(rotateResp.Body).Decode(&rotated); err != nil {
		t.Fatal(err)
	}
	rotateResp.Body.Close()
	if rotateResp.StatusCode != http.StatusOK || rotated.Secret == "" || rotated.RotatedFrom != child.ID {
		t.Fatalf("rotate = %d %+v", rotateResp.StatusCode, rotated)
	}
	old, found, err := db.GetAPIKey(context.Background(), child.ID)
	if err != nil || !found || old.Status != "revoked" || old.RevokedAt.IsZero() {
		t.Fatalf("old key not revoked: found=%v old=%+v err=%v", found, old, err)
	}
	replacement, found, err := db.GetAPIKey(context.Background(), rotated.APIKey.ID)
	if err != nil || !found {
		t.Fatalf("replacement lookup found=%v err=%v", found, err)
	}
	if replacement.UserID != child.UserID || replacement.Team != child.Team || replacement.Role != child.Role ||
		len(replacement.Scopes) != 0 || !reflect.DeepEqual(replacement.AllowedIPs, child.AllowedIPs) ||
		!reflect.DeepEqual(replacement.AllowedModels, child.AllowedModels) || !reflect.DeepEqual(replacement.DeniedModels, child.DeniedModels) ||
		!reflect.DeepEqual(replacement.AllowedProviders, child.AllowedProviders) || !reflect.DeepEqual(replacement.DeniedProviders, child.DeniedProviders) ||
		replacement.BudgetLimitKRW != child.BudgetLimitKRW || !replacement.ExpiresAt.Equal(child.ExpiresAt) {
		t.Fatalf("rotation widened or dropped policy: before=%+v after=%+v", child, replacement)
	}
}

func TestAdminAPIKeyPolicyCreateAndPatch(t *testing.T) {
	ts, db := meKeysServer(t, true)
	expires := time.Now().UTC().Add(3 * time.Hour)
	create := postJSON(t, ts.URL+"/admin/api-keys", "", map[string]any{
		"name": "admin policy", "key": "admin-policy-secret", "user_id": "managed-user",
		"scopes": []string{"models:read"}, "allowed_ips": []string{"10.0.0.0/24"},
		"allowed_models": []string{"gpt-*"}, "denied_models": []string{"gpt-danger"},
		"allowed_providers": []string{"openai"}, "denied_providers": []string{"evil"},
		"budget_limit_krw": 500, "expires_at": expires.Format(time.RFC3339Nano),
	})
	var created struct {
		APIKey store.APIKeyPublic `json:"api_key"`
	}
	_ = json.NewDecoder(create.Body).Decode(&created)
	create.Body.Close()
	if create.StatusCode != http.StatusCreated || created.APIKey.ID == "" {
		t.Fatalf("admin policy create = %d %+v", create.StatusCode, created)
	}

	nextExpiry := time.Now().UTC().Add(4 * time.Hour)
	patch := patchJSON(t, ts.URL+"/admin/api-keys/"+created.APIKey.ID, "", map[string]any{
		"scopes": []string{}, "allowed_ips": []string{"10.0.0.8"},
		"allowed_models": []string{"qwen-*"}, "denied_models": []string{"qwen-danger"},
		"allowed_providers": []string{"qwen"}, "denied_providers": []string{"legacy"},
		"budget_limit_krw": 125, "expires_at": nextExpiry.Format(time.RFC3339Nano),
	})
	patch.Body.Close()
	if patch.StatusCode != http.StatusOK {
		t.Fatalf("admin policy patch = %d", patch.StatusCode)
	}
	got, found, err := db.GetAPIKey(context.Background(), created.APIKey.ID)
	if err != nil || !found || len(got.Scopes) != 0 || !reflect.DeepEqual(got.AllowedIPs, []string{"10.0.0.8"}) ||
		!reflect.DeepEqual(got.AllowedModels, []string{"qwen-*"}) || got.BudgetLimitKRW != 125 || !got.ExpiresAt.Equal(nextExpiry) {
		t.Fatalf("admin patch not persisted found=%v got=%+v err=%v", found, got, err)
	}

	bad := patchJSON(t, ts.URL+"/admin/api-keys/"+created.APIKey.ID, "", map[string]any{"allowed_ips": []string{"not-an-ip"}})
	bad.Body.Close()
	if bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid admin allowed_ips = %d, want 400", bad.StatusCode)
	}
	unknown := patchJSON(t, ts.URL+"/admin/api-keys/"+created.APIKey.ID, "", map[string]any{"unknown_policy": true})
	unknown.Body.Close()
	if unknown.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown admin key policy field = %d, want 400", unknown.StatusCode)
	}
}
