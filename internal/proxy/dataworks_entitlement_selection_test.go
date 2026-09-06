package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/store"
)

// TestDataProductQueryPicksActiveEntitlement covers a customer whose API key holds a revoked
// entitlement written after the active one: the runtime gate must evaluate the grant that is
// still active instead of the row that happens to be newest.
func TestDataProductQueryPicksActiveEntitlement(t *testing.T) {
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
		ID: "dprod_pick", ProductKey: "dw_pick_gate", NameKO: "Selection Gate API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
		ID: "apikey_pick", KeyHash: hashProxyKey("pick-client-token"), Name: "pick-client",
		Team: "team_pick", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_pick_renewal", ProductKey: "dw_pick_gate", CustomerKey: "cust_pick",
		AllowedFields: []string{"product_key", "score"}, Status: "active", Purpose: "risk monitoring",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_pick_trial", ProductKey: "dw_pick_gate", CustomerKey: "cust_pick",
		AllowedFields: []string{"product_key"}, Status: "active", Purpose: "trial",
	}); err != nil {
		t.Fatal(err)
	}

	// The renewal is active; the trial that follows it has expired.
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_pick_renewal", APIKeyID: "apikey_pick", APIKeyHash: hashProxyKey("pick-client-token"),
		ProductKey: "dw_pick_gate", CustomerKey: "cust_pick", ContractKey: "ct_pick_renewal",
		Status: "active", Scope: "data_product:query",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_pick_trial", APIKeyID: "apikey_pick", APIKeyHash: hashProxyKey("pick-client-token"),
		ProductKey: "dw_pick_gate", CustomerKey: "cust_pick", ContractKey: "ct_pick_trial",
		Status: "active", Scope: "data_product:query",
		ExpiresAt: time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/v1/data-products/dw_pick_gate/query", "pick-client-token", map[string]any{"fields": []string{"score"}})
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 while an active entitlement exists, got %d: %s", resp.StatusCode, raw)
	}
	var body struct {
		ContractKey   string `json:"contract_key"`
		EntitlementID string `json:"entitlement_id"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.EntitlementID != "ent_pick_renewal" || body.ContractKey != "ct_pick_renewal" {
		t.Fatalf("expected the active renewal to be used, got %s", raw)
	}
}
