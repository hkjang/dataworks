package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"dataworks/internal/store"
)

type actionCenterPayload struct {
	Summary        map[string]int   `json:"summary"`
	Actions        []map[string]any `json:"actions"`
	ExpiringWithin string           `json:"expiring_within"`
}

func getActionCenter(t *testing.T, baseURL string, query string) actionCenterPayload {
	t.Helper()
	resp, err := http.Get(baseURL + "/admin/dataworks/action-center" + query)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("action center%s status = %d: %s", query, resp.StatusCode, body)
	}
	var payload actionCenterPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func actionsOfType(payload actionCenterPayload, actionType string) []map[string]any {
	out := []map[string]any{}
	for _, action := range payload.Actions {
		if action["type"] == actionType {
			out = append(out, action)
		}
	}
	return out
}

// An entitlement that is still active but about to lapse has to be flagged before the expiry
// day: without a lookahead the customer key simply stops working and the only signal is the
// inactive_access entry that appears after access is already gone.
func TestDataWorksActionCenterWarnsBeforeEntitlementExpires(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_soon", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		ValidTo: now.Add(60 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
		AllowedFields: []string{"score"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_soon", APIKeyID: "key_bank", ProductKey: "dw_credit_score", ContractKey: "ct_soon",
		ExpiresAt: now.Add(20 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_stable", APIKeyID: "key_insurer", ProductKey: "dw_credit_score", ContractKey: "ct_soon",
		ExpiresAt: now.Add(200 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	payload := getActionCenter(t, srv.URL, "")
	if payload.Summary["expiring_access"] != 1 {
		t.Fatalf("expiring_access = %d, want 1: %+v", payload.Summary["expiring_access"], payload.Actions)
	}
	if payload.Summary["inactive_access"] != 0 {
		t.Fatalf("inactive_access = %d, want 0: %+v", payload.Summary["inactive_access"], payload.Actions)
	}
	expiring := actionsOfType(payload, "entitlement_expiring")
	if len(expiring) != 1 || expiring[0]["entitlement_id"] != "ent_soon" {
		t.Fatalf("entitlement_expiring actions = %+v", expiring)
	}
	if expiring[0]["contract_key"] != "ct_soon" {
		t.Fatalf("entitlement_expiring contract_key = %v, want ct_soon", expiring[0]["contract_key"])
	}
	// The contract outlives the default window, so only the entitlement is due.
	if payload.Summary["expiring_contracts"] != 0 {
		t.Fatalf("expiring_contracts = %d, want 0: %+v", payload.Summary["expiring_contracts"], payload.Actions)
	}
	if payload.ExpiringWithin != (30 * 24 * time.Hour).String() {
		t.Fatalf("expiring_within = %q, want %q", payload.ExpiringWithin, (30 * 24 * time.Hour).String())
	}

	// A quarterly renewal cycle needs a longer lookahead than the fixed 30 days.
	wide := getActionCenter(t, srv.URL, "?expiring_within=13w")
	if wide.Summary["expiring_contracts"] != 1 {
		t.Fatalf("expiring_contracts within 13w = %d, want 1: %+v", wide.Summary["expiring_contracts"], wide.Actions)
	}
	if wide.Summary["expiring_access"] != 1 {
		t.Fatalf("expiring_access within 13w = %d, want 1: %+v", wide.Summary["expiring_access"], wide.Actions)
	}

	// A shorter lookahead only reports what lapses inside it.
	narrow := getActionCenter(t, srv.URL, "?expiring_within=7d")
	if narrow.Summary["expiring_access"] != 0 {
		t.Fatalf("expiring_access within 7d = %d, want 0: %+v", narrow.Summary["expiring_access"], narrow.Actions)
	}
	if narrow.ExpiringWithin != (7 * 24 * time.Hour).String() {
		t.Fatalf("expiring_within = %q, want %q", narrow.ExpiringWithin, (7 * 24 * time.Hour).String())
	}
}

// A contract an operator parked or closed is not waiting on a renewal, and its valid_to only
// moves further into the past, so counting it as expiring would pin it to the screen forever
// at high severity and bury the live contracts that really are about to lapse.
func TestDataWorksActionCenterSkipsInactiveContractScopes(t *testing.T) {
	db, srv := newAccessWindowTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	past := now.Add(-90 * 24 * time.Hour).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		contractKey string
		status      string
	}{
		{"ct_revoked", "revoked"},
		{"ct_draft", "draft"},
		{"ct_suspended", "suspended"},
	} {
		if err := db.UpsertContractScope(ctx, store.ContractScope{
			ContractKey: tc.contractKey, ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
			ValidTo: past, Status: tc.status, AllowedFields: []string{"score"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_live", ProductKey: "dw_credit_score", CustomerKey: "cust_insurer",
		ValidTo: now.Add(10 * 24 * time.Hour).Format(time.RFC3339Nano), Status: "active",
		AllowedFields: []string{"score"},
	}); err != nil {
		t.Fatal(err)
	}

	payload := getActionCenter(t, srv.URL, "")
	if payload.Summary["expiring_contracts"] != 1 {
		t.Fatalf("expiring_contracts = %d, want 1: %+v", payload.Summary["expiring_contracts"], payload.Actions)
	}
	expiring := actionsOfType(payload, "contract_expiring")
	if len(expiring) != 1 || expiring[0]["contract_key"] != "ct_live" {
		t.Fatalf("contract_expiring actions = %+v", expiring)
	}
}

// Falling back to the default window for a malformed value would answer for a different
// horizon than the operator asked about, so the request is rejected instead.
func TestDataWorksActionCenterRejectsInvalidExpiringWindow(t *testing.T) {
	_, srv := newAccessWindowTestServer(t)

	for _, raw := range []string{"quarter", "0d", "-30d", "30x"} {
		resp, err := http.Get(srv.URL + "/admin/dataworks/action-center?expiring_within=" + raw)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expiring_within=%s status = %d: %s", raw, resp.StatusCode, body)
		}
		if code := errorCodeOf(t, body); code != "invalid_expiring_within" {
			t.Fatalf("expiring_within=%s error code = %q: %s", raw, code, body)
		}
	}
}

func TestParseExpiryHorizon(t *testing.T) {
	cases := []struct {
		raw  string
		want time.Duration
		ok   bool
	}{
		{"", 30 * 24 * time.Hour, true},
		{"  ", 30 * 24 * time.Hour, true},
		{"45d", 45 * 24 * time.Hour, true},
		{"13W", 13 * 7 * 24 * time.Hour, true},
		{"72h", 72 * time.Hour, true},
		{"90m", 90 * time.Minute, true},
		{"0", 0, false},
		{"0d", 0, false},
		{"-7d", 0, false},
		{"-24h", 0, false},
		{"90", 0, false},
		{"quarter", 0, false},
	}
	for _, tc := range cases {
		got, ok := parseExpiryHorizon(tc.raw)
		if ok != tc.ok || (ok && got != tc.want) {
			t.Fatalf("parseExpiryHorizon(%q) = (%v, %t), want (%v, %t)", tc.raw, got, ok, tc.want, tc.ok)
		}
	}
}
