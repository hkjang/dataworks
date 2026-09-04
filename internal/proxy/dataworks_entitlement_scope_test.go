package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func TestEntitlementAllowsQuery(t *testing.T) {
	cases := []struct {
		scope string
		want  bool
	}{
		{"", true},
		{"   ", true},
		{"*", true},
		{"query", true},
		{"data_product:query", true},
		{"DATA_PRODUCT:QUERY", true},
		{"data_product:*", true},
		{"data_product:export, data_product:query", true},
		{"catalog:read data_product:query", true},
		// A deny marker must not be read as a grant just because it spells "query".
		{"no-query", false},
		{"query:denied", false},
		{"data_product:subquery", false},
		{"data_product:export", false},
		{"catalog:read", false},
	}
	for _, tc := range cases {
		if got := entitlementAllowsQuery(tc.scope); got != tc.want {
			t.Errorf("entitlementAllowsQuery(%q) = %v, want %v", tc.scope, got, tc.want)
		}
	}
}

// TestDataProductQueryScopeGate covers the runtime gate end to end: a scope that only
// mentions "query" inside a deny marker must be refused, while the documented wildcard
// grant must be accepted.
func TestDataProductQueryScopeGate(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_scope", ProductKey: "dw_scope_gate", NameKO: "Scope Gate API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
		ID: "apikey_scope", KeyHash: hashProxyKey("scope-client-token"), Name: "scope-client",
		Team: "team_scope", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_scope", ProductKey: "dw_scope_gate", CustomerKey: "cust_scope",
		AllowedFields: []string{"product_key", "score"}, Status: "active",
		Purpose: "risk monitoring",
	}); err != nil {
		t.Fatal(err)
	}
	entitlement := store.APIEntitlement{
		ID: "ent_scope", APIKeyID: "apikey_scope", APIKeyHash: hashProxyKey("scope-client-token"),
		ProductKey: "dw_scope_gate", CustomerKey: "cust_scope", ContractKey: "ct_scope",
		Status: "active", Scope: "catalog:read,no-query",
	}
	if err := db.UpsertAPIEntitlement(ctx, entitlement); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"fields": []string{"score"}}
	denied := postJSON(t, srv.URL+"/v1/data-products/dw_scope_gate/query", "scope-client-token", body)
	raw, _ := io.ReadAll(denied.Body)
	denied.Body.Close()
	if denied.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 for a scope that does not grant query, got %d: %s", denied.StatusCode, raw)
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &errBody)
	if errBody.Error.Code != "scope_denied" {
		t.Fatalf("unexpected error code in %s", raw)
	}

	entitlement.Scope = "data_product:*"
	if err := db.UpsertAPIEntitlement(ctx, entitlement); err != nil {
		t.Fatal(err)
	}
	allowed := postJSON(t, srv.URL+"/v1/data-products/dw_scope_gate/query", "scope-client-token", body)
	raw, _ = io.ReadAll(allowed.Body)
	allowed.Body.Close()
	if allowed.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 for the data_product:* wildcard grant, got %d: %s", allowed.StatusCode, raw)
	}
}
