package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

// A contract scope that cannot be read is not a contract scope that is missing or parked. The
// runtime gate walks every entitlement an API key holds for a product, so a failed read on one
// candidate used to be swallowed by the same `continue` that skips a genuinely unusable grant:
// the caller then got the first candidate's 403 reason instead of the 500 the single-candidate
// path already returns for the very same failure. Exercise both halves of the contract through
// real SQLite and the production router: the failure has to surface when nothing is usable, and
// it must not cost a caller the live grant that sits behind the broken one.
func TestDataProductQueryReportsUnreadableContractScope(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "runtime-entitlement.db")
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "audit.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(ctx) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)
	schema, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { schema.Close() })

	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_lookup", ProductKey: "dw_lookup_gate", NameKO: "Lookup Gate API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	for _, key := range []struct{ id, token, name string }{
		{"apikey_lookup_a", "lookup-a-token", "lookup-a"},
		{"apikey_lookup_b", "lookup-b-token", "lookup-b"},
	} {
		if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
			ID: key.id, KeyHash: hashProxyKey(key.token), Name: key.name,
			Team: "team_lookup", Status: "active",
		}); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339Nano)
	// ctr-broken is a perfectly ordinary contract until the view below makes its row unreadable.
	for _, scope := range []store.ContractScope{
		{ContractKey: "ctr-broken", ProductKey: "dw_lookup_gate", CustomerKey: "cust_lookup",
			AllowedFields: []string{"product_key", "score"}, Status: "active", Purpose: "risk monitoring"},
		{ContractKey: "ctr-closed", ProductKey: "dw_lookup_gate", CustomerKey: "cust_lookup",
			AllowedFields: []string{"product_key", "score"}, Status: "active", Purpose: "pilot", ValidTo: past},
		{ContractKey: "ctr-live", ProductKey: "dw_lookup_gate", CustomerKey: "cust_lookup",
			AllowedFields: []string{"product_key", "score"}, Status: "active", Purpose: "risk monitoring"},
	} {
		if err := db.UpsertContractScope(ctx, scope); err != nil {
			t.Fatal(err)
		}
	}
	// Key A holds only unusable grants once ctr-broken stops reading; key B keeps a live one
	// behind the broken candidate. ListAPIEntitlementCandidates orders by updated_at DESC, so
	// the stamps below are what fixes which candidate the handler falls back to.
	for _, ent := range []struct {
		id, apiKey, contract, updatedAt string
	}{
		{"ent_a_broken", "apikey_lookup_a", "ctr-broken", "2026-01-01T00:00:00Z"},
		{"ent_a_closed", "apikey_lookup_a", "ctr-closed", "2026-02-01T00:00:00Z"},
		{"ent_b_broken", "apikey_lookup_b", "ctr-broken", "2026-02-01T00:00:00Z"},
		{"ent_b_live", "apikey_lookup_b", "ctr-live", "2026-01-01T00:00:00Z"},
	} {
		token := "lookup-a-token"
		if ent.apiKey == "apikey_lookup_b" {
			token = "lookup-b-token"
		}
		if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
			ID: ent.id, APIKeyID: ent.apiKey, APIKeyHash: hashProxyKey(token),
			ProductKey: "dw_lookup_gate", CustomerKey: "cust_lookup", ContractKey: ent.contract,
			Status: "active", Scope: "data_product:query",
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := schema.ExecContext(ctx, "UPDATE dw_api_entitlements SET updated_at = ? WHERE id = ?", ent.updatedAt, ent.id); err != nil {
			t.Fatal(err)
		}
	}

	query := func(t *testing.T, token string) (int, []byte) {
		t.Helper()
		resp := postJSON(t, srv.URL+"/v1/data-products/dw_lookup_gate/query", token, map[string]any{"fields": []string{"score"}})
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, body
	}
	selection := func(t *testing.T, raw []byte) (string, string) {
		t.Helper()
		var payload struct {
			ContractKey   string `json:"contract_key"`
			EntitlementID string `json:"entitlement_id"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatal(err)
		}
		return payload.ContractKey, payload.EntitlementID
	}
	// Both keys serve while every row reads, which is what tells an unreadable row apart from
	// a contract that is genuinely closed.
	assertBaseline := func(t *testing.T) {
		t.Helper()
		for _, token := range []string{"lookup-a-token", "lookup-b-token"} {
			status, body := query(t, token)
			if status != http.StatusOK {
				t.Fatalf("baseline %s: status = %d, want 200: %s", token, status, body)
			}
			if contractKey, _ := selection(t, body); contractKey != "ctr-broken" {
				t.Fatalf("baseline %s selected %q, want ctr-broken", token, contractKey)
			}
		}
	}
	assertBaseline(t)

	// Swap the table for a view that returns NULL in the NOT NULL valid_from column of one row.
	// GetContractScope scans valid_from into a string, so only that contract_key fails to read;
	// renaming the whole table instead would make the handler's own lookup at :100 fail and the
	// candidate walk would never be reached.
	if _, err := schema.ExecContext(ctx, `ALTER TABLE dw_contract_scopes RENAME TO dw_contract_scopes_real`); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.ExecContext(ctx, `CREATE VIEW dw_contract_scopes AS
		SELECT contract_key, product_key, customer_key, allowed_fields, rate_limit,
			CASE WHEN contract_key = 'ctr-broken' THEN NULL ELSE valid_from END AS valid_from,
			valid_to, purpose, restrictions, status, created_by, created_at, updated_at, masking_policy
		FROM dw_contract_scopes_real`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := schema.ExecContext(ctx, `DROP VIEW dw_contract_scopes`); err != nil {
			t.Fatal(err)
		}
		if _, err := schema.ExecContext(ctx, `ALTER TABLE dw_contract_scopes_real RENAME TO dw_contract_scopes`); err != nil {
			t.Fatal(err)
		}
		assertBaseline(t)
	})

	t.Run("no_usable_candidate_surfaces_the_read_failure", func(t *testing.T) {
		status, body := query(t, "lookup-a-token")
		if status != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500 (a failed contract read is not an inactive contract): %s", status, body)
		}
		if code := errorCodeOf(t, body); code != "contract_lookup_failed" {
			t.Fatalf("error code = %q, want contract_lookup_failed: %s", code, body)
		}
	})

	t.Run("live_candidate_behind_the_failure_still_serves", func(t *testing.T) {
		status, body := query(t, "lookup-b-token")
		if status != http.StatusOK {
			t.Fatalf("status = %d, want 200 (a live grant must outrank another candidate's read failure): %s", status, body)
		}
		contractKey, entitlementID := selection(t, body)
		if contractKey != "ctr-live" || entitlementID != "ent_b_live" {
			t.Fatalf("selected %q/%q, want ctr-live/ent_b_live: %s", contractKey, entitlementID, body)
		}
	})
}
