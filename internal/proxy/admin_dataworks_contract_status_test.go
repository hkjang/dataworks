package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"dataworks/internal/store"
)

func TestContractScopeStatusKnown(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"", true},
		{"active", true},
		{" ACTIVE ", true},
		{"draft", true},
		{"suspended", true},
		{"revoked", true},
		{"actve", false},
		{"enabled", false},
		{"inactive", false},
	}
	for _, tc := range cases {
		if got := contractScopeStatusKnown(tc.status); got != tc.want {
			t.Fatalf("contractScopeStatusKnown(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

func TestContractScopeStatusActive(t *testing.T) {
	cases := []struct {
		status string
		want   bool
	}{
		{"active", true},
		{" ACTIVE ", true},
		{"Active", true},
		{"", false},
		{"draft", false},
		{"suspended", false},
		{"revoked", false},
	}
	for _, tc := range cases {
		if got := contractScopeStatusActive(tc.status); got != tc.want {
			t.Fatalf("contractScopeStatusActive(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// An inverted window and an unrecognised status both leave a contract that answers every query
// with 403 while the admin views still list it, so the write path has to refuse them the same way
// it refuses an unparseable timestamp.
func TestDataWorksContractScopeRejectsDeadContract(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_inverted", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"valid_from": "2030-01-01T00:00:00Z", "valid_to": "2029-01-01T00:00:00Z",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("inverted access window status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_access_window" {
		t.Fatalf("inverted access window error code = %q: %s", code, body)
	}
	if _, ok, err := db.GetContractScope(ctx, "ct_inverted"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("inverted access window was stored")
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_typo", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"status": "actve",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown status status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_contract_status" {
		t.Fatalf("unknown status error code = %q: %s", code, body)
	}
	if _, ok, err := db.GetContractScope(ctx, "ct_typo"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Fatal("contract scope with unknown status was stored")
	}

	// An ordered window passes, and the status is normalised so the response matches the row the
	// runtime reads.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"valid_from": "2029-01-01T00:00:00Z", "valid_to": "2030-01-01T00:00:00Z", "status": " ACTIVE ",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid contract scope status = %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		ContractScope store.ContractScope `json:"contract_scope"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ContractScope.Status != "active" {
		t.Fatalf("stored status = %q, want active", payload.ContractScope.Status)
	}

	// An omitted status is stored as active, so the response must say so too.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_default", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("default status contract scope status = %d: %s", resp.StatusCode, body)
	}
	payload.ContractScope = store.ContractScope{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ContractScope.Status != "active" {
		t.Fatalf("default status = %q, want active", payload.ContractScope.Status)
	}
}
