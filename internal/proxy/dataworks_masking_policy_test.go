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

func newMaskingPolicyTestServer(t *testing.T) (*store.SQLStore, *httptest.Server) {
	t.Helper()
	db := openTestStore(t)
	t.Cleanup(func() { db.Close() })
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-masking.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(context.Background()) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)

	product := store.DataProduct{
		ID: "dprod_mask", ProductKey: "dw_credit_score", NameKO: "Credit Score API",
		SourceType: "api", SourceRef: "loan_history", Sensitivity: "personal", Status: "draft",
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	return db, srv
}

// applyMasking leaves unknown policies untouched, so storing one would advertise masking the
// runtime never performs.
func TestDataWorksContractScopeRejectsUnknownMaskingPolicy(t *testing.T) {
	_, srv := newMaskingPolicyTestServer(t)

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bad", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"masking_policy": "개인 단위 원천값 제외",
	})
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("free-form masking_policy status = %d: %s", resp.StatusCode, body)
	}
	if code := errorCodeOf(t, body); code != "invalid_masking_policy" {
		t.Fatalf("error code = %q: %s", code, body)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score"},
		"purpose": "credit risk monitoring", "masking_policy": "  REDACT  ",
	})
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redact masking_policy status = %d: %s", resp.StatusCode, body)
	}
	var saved struct {
		ContractScope store.ContractScope `json:"contract_scope"`
	}
	if err := json.Unmarshal(body, &saved); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
	// The runtime lowercases before matching, so store the canonical form the admin views read.
	if saved.ContractScope.MaskingPolicy != "redact" {
		t.Fatalf("stored masking_policy = %q, want redact", saved.ContractScope.MaskingPolicy)
	}
}

// Rows written before the validation existed must not satisfy the sensitive-product publish
// gate, because the runtime returns their responses unmasked.
func TestDataWorksPublishGateIgnoresUnenforcedMaskingPolicy(t *testing.T) {
	db, srv := newMaskingPolicyTestServer(t)
	ctx := context.Background()

	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		MaskingPolicy: "개인 단위 원천값 제외, 집계/가명/마스킹 응답 우선",
	}); err != nil {
		t.Fatal(err)
	}

	gate := fetchPublishGate(t, srv)
	if gate.MaskingConfigured {
		t.Fatalf("masking_configured = true for a policy the runtime ignores: %+v", gate)
	}
	if !containsString(gate.MissingEvidence, "masking_policy") {
		t.Fatalf("missing_evidence = %v, want masking_policy", gate.MissingEvidence)
	}

	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_legacy", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		MaskingPolicy: "hash",
	}); err != nil {
		t.Fatal(err)
	}
	gate = fetchPublishGate(t, srv)
	if !gate.MaskingConfigured {
		t.Fatalf("masking_configured = false for an enforced policy: %+v", gate)
	}
}

// Masking is applied per contract, so one masked customer says nothing about the customers
// whose contracts mask nothing.
func TestDataWorksPublishGateRequiresMaskingOnEveryServableContract(t *testing.T) {
	db, srv := newMaskingPolicyTestServer(t)
	ctx := context.Background()
	now := time.Now().UTC()

	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_masked", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		MaskingPolicy: "redact",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_raw", ProductKey: "dw_credit_score", CustomerKey: "cust_fintech",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		MaskingPolicy: "none",
	}); err != nil {
		t.Fatal(err)
	}

	gate := fetchPublishGate(t, srv)
	if gate.MaskingConfigured {
		t.Fatalf("masking_configured = true while cust_fintech still receives raw values: %+v", gate)
	}

	// Contracts that can never answer a query again are not evidence about the responses this
	// product returns, so they must not block the launch either.
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_raw", ProductKey: "dw_credit_score", CustomerKey: "cust_fintech",
		AllowedFields: []string{"score"}, Status: "revoked", Purpose: "credit risk monitoring",
		MaskingPolicy: "none",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_expired", ProductKey: "dw_credit_score", CustomerKey: "cust_old",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		ValidTo: now.Add(-24 * time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	gate = fetchPublishGate(t, srv)
	if !gate.MaskingConfigured {
		t.Fatalf("masking_configured = false although only closed contracts lack masking: %+v", gate)
	}

	// A window that has not opened yet will serve later, so it still has to mask.
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_future", ProductKey: "dw_credit_score", CustomerKey: "cust_next",
		AllowedFields: []string{"score"}, Status: "active", Purpose: "credit risk monitoring",
		ValidFrom: now.Add(24 * time.Hour).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatal(err)
	}
	gate = fetchPublishGate(t, srv)
	if gate.MaskingConfigured {
		t.Fatalf("masking_configured = true for an unmasked contract that opens tomorrow: %+v", gate)
	}
}

func fetchPublishGate(t *testing.T, srv *httptest.Server) struct {
	MaskingConfigured bool     `json:"masking_configured"`
	MissingEvidence   []string `json:"missing_evidence"`
} {
	t.Helper()
	resp, err := http.Get(srv.URL + "/admin/dataworks/products/dw_credit_score/publish-gate")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish-gate status = %d", resp.StatusCode)
	}
	var payload struct {
		PublishGate struct {
			MaskingConfigured bool     `json:"masking_configured"`
			MissingEvidence   []string `json:"missing_evidence"`
		} `json:"publish_gate"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	return payload.PublishGate
}

func TestMaskingPolicyClassification(t *testing.T) {
	cases := []struct {
		policy string
		known  bool
		masks  bool
	}{
		{"", true, false},
		{"none", true, false},
		{" NONE ", true, false},
		{"redact", true, true},
		{"Hash", true, true},
		{"pseudonymize", false, false},
		{"redact_pii", false, false},
		{"개인 단위 원천값 제외", false, false},
	}
	for _, tc := range cases {
		if got := maskingPolicyKnown(tc.policy); got != tc.known {
			t.Errorf("maskingPolicyKnown(%q) = %v, want %v", tc.policy, got, tc.known)
		}
		if got := maskingPolicyMasks(tc.policy); got != tc.masks {
			t.Errorf("maskingPolicyMasks(%q) = %v, want %v", tc.policy, got, tc.masks)
		}
	}
}

// maskingPolicies is the write-path allowlist, so every entry has to be a policy applyMasking
// actually rewrites values for.
func TestMaskingPoliciesMatchApplyMasking(t *testing.T) {
	for policy := range maskingPolicies {
		if got := applyMasking("sample_value", policy); got == any("sample_value") {
			t.Errorf("applyMasking with policy %q returned the raw value", policy)
		}
	}
}

func TestContractScopeCanServe(t *testing.T) {
	now := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Hour).Format(time.RFC3339Nano)
	future := now.Add(time.Hour).Format(time.RFC3339Nano)
	cases := []struct {
		name     string
		scope    store.ContractScope
		canServe bool
		active   bool
	}{
		{"open window", store.ContractScope{Status: "active", ValidFrom: past, ValidTo: future}, true, true},
		{"not opened yet", store.ContractScope{Status: "active", ValidFrom: future, ValidTo: future}, true, false},
		{"closed window", store.ContractScope{Status: "active", ValidFrom: past, ValidTo: past}, false, false},
		{"revoked", store.ContractScope{Status: "revoked", ValidTo: future}, false, false},
		{"missing status", store.ContractScope{ValidTo: future}, false, false},
		{"unparseable valid_to", store.ContractScope{Status: "active", ValidTo: "2026-12-31"}, false, false},
		{"unparseable valid_from", store.ContractScope{Status: "active", ValidFrom: "2026-01-01"}, true, false},
	}
	for _, tc := range cases {
		if got := contractScopeCanServe(tc.scope, now); got != tc.canServe {
			t.Errorf("%s: contractScopeCanServe = %v, want %v", tc.name, got, tc.canServe)
		}
		if got := contractScopeActive(tc.scope, now); got != tc.active {
			t.Errorf("%s: contractScopeActive = %v, want %v", tc.name, got, tc.active)
		}
	}
}
