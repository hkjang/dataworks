package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"dataworks/internal/store"
)

// contract_key and entitlement id are unique across the platform while the admin routes are
// scoped per product and the store upsert rewrites product_key on conflict. Posting a key that
// another product already holds must therefore be refused, not re-parented: the other product's
// entitlements keep pointing at the moved contract and every query answers 403 from then on.
func TestDataWorksContractScopeRefusesKeyOfAnotherProduct(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_other", ProductKey: "dw_fraud_signal", NameKO: "Fraud Signal API",
		SourceType: "api", SourceRef: "fraud_events", Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
		ID: "key_bank", Name: "Bank API", KeyHash: hashProxyKey("bank-secret"), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"valid_to": "2030-01-01T00:00:00Z",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("contract scope status = %d: %s", resp.StatusCode, body)
	}
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank", "scope": "data_product:query",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("entitlement status = %d: %s", resp.StatusCode, body)
	}
	assertCreditScoreQueryAllowed := func(step string) {
		t.Helper()
		resp := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score"}})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: runtime query status = %d: %s", step, resp.StatusCode, body)
		}
	}
	assertCreditScoreQueryAllowed("before conflict")

	// The other product reuses the same contract_key (operators name contracts after the
	// customer, so the collision is routine).
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_fraud_signal/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"fraud_score"},
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("contract_key reuse status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "contract_key_taken" {
		t.Fatalf("contract_key reuse error code = %q: %s", code, body)
	}
	scope, ok, err := db.GetContractScope(ctx, "ct_bank")
	if err != nil || !ok {
		t.Fatalf("contract scope lookup ok=%v err=%v", ok, err)
	}
	if scope.ProductKey != "dw_credit_score" || len(scope.AllowedFields) != 1 || scope.AllowedFields[0] != "score" {
		t.Fatalf("contract scope was re-parented or rewritten: %+v", scope)
	}
	assertCreditScoreQueryAllowed("after refused reuse")

	resp, err = http.Get(srv.URL + "/admin/dataworks/products/dw_fraud_signal/contract-scopes")
	if err != nil {
		t.Fatal(err)
	}
	var listed struct {
		ContractScopes []store.ContractScope `json:"contract_scopes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(listed.ContractScopes) != 0 {
		t.Fatalf("other product lists the contested contract: %+v", listed.ContractScopes)
	}

	// Re-posting under the owning product is still an update.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score", "risk_band"},
		"valid_to": "2031-01-01T00:00:00Z",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own contract update status = %d: %s", resp.StatusCode, body)
	}
	if scope, _, err := db.GetContractScope(ctx, "ct_bank"); err != nil || len(scope.AllowedFields) != 2 {
		t.Fatalf("own contract update not stored: %+v err=%v", scope, err)
	}
}

func TestDataWorksEntitlementRefusesIDOfAnotherProduct(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_other", ProductKey: "dw_fraud_signal", NameKO: "Fraud Signal API",
		SourceType: "api", SourceRef: "fraud_events", Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	for _, seed := range []struct{ product, contract string }{
		{"dw_credit_score", "ct_bank_credit"}, {"dw_fraud_signal", "ct_bank_fraud"},
	} {
		resp := postJSON(t, srv.URL+"/admin/dataworks/products/"+seed.product+"/contract-scopes", "", map[string]any{
			"contract_key": seed.contract, "customer_key": "cust_bank", "allowed_fields": []string{"*"},
		})
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("contract scope %s status = %d: %s", seed.contract, resp.StatusCode, body)
		}
	}
	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank_credit",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("entitlement status = %d: %s", resp.StatusCode, body)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_fraud_signal/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank_fraud",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("entitlement id reuse status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "entitlement_id_taken" {
		t.Fatalf("entitlement id reuse error code = %q: %s", code, body)
	}
	ent, ok, err := db.GetAPIEntitlement(ctx, "ent_bank")
	if err != nil || !ok {
		t.Fatalf("entitlement lookup ok=%v err=%v", ok, err)
	}
	if ent.ProductKey != "dw_credit_score" || ent.ContractKey != "ct_bank_credit" {
		t.Fatalf("entitlement was re-parented: %+v", ent)
	}

	// Updating the grant under its own product keeps working.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank_credit", "status": "suspended",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("own entitlement update status = %d: %s", resp.StatusCode, body)
	}
	if ent, _, err := db.GetAPIEntitlement(ctx, "ent_bank"); err != nil || ent.Status != "suspended" {
		t.Fatalf("own entitlement update not stored: %+v err=%v", ent, err)
	}
}
