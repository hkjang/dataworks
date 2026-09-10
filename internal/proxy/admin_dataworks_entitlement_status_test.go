package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	"dataworks/internal/store"
)

func TestEntitlementStatusKnown(t *testing.T) {
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
		{"expired", false},
	}
	for _, tc := range cases {
		if got := entitlementStatusKnown(tc.status); got != tc.want {
			t.Fatalf("entitlementStatusKnown(%q) = %v, want %v", tc.status, got, tc.want)
		}
	}
}

// The runtime only serves an "active" entitlement, so an unrecognised status hands the customer
// 403 inactive_entitlement while the admin views still list the key as entitled. The write path
// has to refuse it the same way the contract scope path refuses an unknown contract status.
func TestDataWorksEntitlementRejectsUnknownStatus(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

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
		"id": "ent_typo", "api_key_id": "key_bank", "contract_key": "ct_bank", "status": "enabled",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown entitlement status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_entitlement_status" {
		t.Fatalf("unknown entitlement status error code = %q: %s", code, body)
	}
	ents, err := db.ListAPIEntitlements(ctx, "dw_credit_score", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("entitlement with unknown status was stored: %+v", ents)
	}

	// A recognised status is normalised so the response matches the row the runtime reads.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "contract_key": "ct_bank", "status": " ACTIVE ",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("valid entitlement status = %d: %s", resp.StatusCode, body)
	}
	var payload struct {
		Entitlement store.APIEntitlement `json:"entitlement"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Entitlement.Status != "active" {
		t.Fatalf("stored entitlement status = %q, want active", payload.Entitlement.Status)
	}

	// Revoked grants stay writable: the platform keeps them as rows for audit.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_old", "api_key_id": "key_bank", "contract_key": "ct_bank", "status": "revoked",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoked entitlement status = %d: %s", resp.StatusCode, body)
	}

	// An omitted status is stored as active, so the response must say so too.
	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_default", "api_key_id": "key_ops", "contract_key": "ct_bank",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("default entitlement status = %d: %s", resp.StatusCode, body)
	}
	payload.Entitlement = store.APIEntitlement{}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Entitlement.Status != "active" {
		t.Fatalf("default entitlement status = %q, want active", payload.Entitlement.Status)
	}
	stored, ok, err := db.FindAPIEntitlement(ctx, "dw_credit_score", "key_ops", "")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || stored.Status != "active" {
		t.Fatalf("stored default entitlement = %+v (found %v), want active", stored, ok)
	}
}
