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

func newAccessWindowTestServer(t *testing.T) (*store.SQLStore, *httptest.Server) {
	t.Helper()
	db := openTestStore(t)
	t.Cleanup(func() { db.Close() })
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-access.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(context.Background()) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)

	product := store.DataProduct{
		ID: "dprod_window", ProductKey: "dw_credit_score", NameKO: "Credit Score API",
		SourceType: "api", SourceRef: "loan_history", Sensitivity: "internal", Status: "published",
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	return db, srv
}

// The runtime gate treats an unparseable access window as inactive, so the admin write path must
// refuse to store one instead of publishing a rule that silently denies every query.
func TestDataWorksAccessWindowRejectsNonRFC3339(t *testing.T) {
	_, srv := newAccessWindowTestServer(t)

	scopeResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bad", "customer_key": "cust_bank", "valid_to": "2026-12-31",
	})
	body, _ := io.ReadAll(scopeResp.Body)
	scopeResp.Body.Close()
	if scopeResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("contract scope with date-only valid_to status = %d: %s", scopeResp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_valid_to" {
		t.Fatalf("contract scope error code = %q: %s", code, body)
	}

	scopeResp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"valid_to": "2030-01-01T00:00:00Z", "purpose": "credit risk monitoring",
	})
	body, _ = io.ReadAll(scopeResp.Body)
	scopeResp.Body.Close()
	if scopeResp.StatusCode != http.StatusOK {
		t.Fatalf("valid contract scope status = %d: %s", scopeResp.StatusCode, body)
	}

	entResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bad", "api_key_id": "key_bank", "contract_key": "ct_bank", "expires_at": "2026-12-31",
	})
	body, _ = io.ReadAll(entResp.Body)
	entResp.Body.Close()
	if entResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("entitlement with date-only expires_at status = %d: %s", entResp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_expires_at" {
		t.Fatalf("entitlement error code = %q: %s", code, body)
	}

	entResp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank",
		"expires_at": "2030-01-01T00:00:00Z", "status": "active",
	})
	body, _ = io.ReadAll(entResp.Body)
	entResp.Body.Close()
	if entResp.StatusCode != http.StatusOK {
		t.Fatalf("valid entitlement status = %d: %s", entResp.StatusCode, body)
	}
}

// Rows written before the validation existed still have to show up in the action center, because
// the runtime already refuses to serve them.
func TestDataWorksActionCenterFlagsUnparseableAccessWindow(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		ValidTo: "2026-12-31", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_legacy", APIKeyID: "key_bank", ProductKey: "dw_credit_score",
		ContractKey: "ct_legacy", ExpiresAt: "2026-12-31", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/admin/dataworks/action-center")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Summary map[string]int   `json:"summary"`
		Actions []map[string]any `json:"actions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("action center status = %d", resp.StatusCode)
	}
	if payload.Summary["expiring_contracts"] != 1 {
		t.Fatalf("expiring_contracts = %d, want 1: %+v", payload.Summary["expiring_contracts"], payload.Actions)
	}
	if payload.Summary["inactive_access"] != 1 {
		t.Fatalf("inactive_access = %d, want 1: %+v", payload.Summary["inactive_access"], payload.Actions)
	}
	for _, action := range payload.Actions {
		if action["type"] == "contract_expiring" && action["severity"] != "high" {
			t.Fatalf("contract with unparseable valid_to severity = %v, want high", action["severity"])
		}
	}
}

func errorCodeOf(t *testing.T, body []byte) string {
	t.Helper()
	var parsed struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode error body %s: %v", body, err)
	}
	return parsed.Error.Code
}
