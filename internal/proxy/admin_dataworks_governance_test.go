package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dataworks/internal/store"
)

func TestDataWorksPublishGateFlow(t *testing.T) {
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

	product := store.DataProduct{
		ID: "dprod_1", ProductKey: "dw_credit_score", NameKO: "Credit Score API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "personal_credit", Status: "approved", RiskScore: 82,
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/publish", "", map[string]any{})
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("publish should be blocked before evidence, got %d", resp.StatusCode)
	}
	var blocked struct {
		PublishGate struct {
			Allowed bool `json:"allowed"`
		} `json:"publish_gate"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&blocked)
	resp.Body.Close()
	if blocked.PublishGate.Allowed {
		t.Fatal("blocked response reported allowed=true")
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/assets/readiness", "", map[string]any{
		"asset_key": "loan_history", "schema_score": 95, "freshness_score": 92, "sample_score": 90,
		"missingness_score": 93, "sensitivity_score": 90, "external_sharing_score": 91,
		"api_readiness_score": 92, "billing_readiness_score": 88,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readiness status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	for _, step := range []string{"data_owner", "legal", "compliance"} {
		resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/approvals", "", map[string]any{
			"step": step, "status": "approved", "evidence_ref": "memo-" + step,
		})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("approval %s status = %d", step, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/evidence-pack", "", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("evidence pack status = %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/publish", "", map[string]any{})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish should pass after evidence, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	published, ok, err := db.GetDataProduct(context.Background(), "dw_credit_score")
	if err != nil || !ok || published.Status != "published" {
		t.Fatalf("product not published: %+v ok=%v err=%v", published, ok, err)
	}
}

func TestDataWorksOperationsAndRuntimeEntitlement(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-runtime.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	product := store.DataProduct{
		ID: "dprod_runtime", ProductKey: "dw_credit_score", NameKO: "Credit Score API",
		Description: "loan approval credit risk score for banks", SourceType: "api", SourceRef: "loan_history",
		Sensitivity: "personal_credit", Status: "published", TargetIndustries: []string{"banking"},
		TargetCustomers: []string{"risk team"}, RevenueScore: 80, RiskScore: 35,
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(context.Background(), store.APIKeyRecord{
		ID: "key_bank", Name: "Bank API", KeyHash: hashProxyKey("bank-secret"), Status: "active",
	}); err != nil {
		t.Fatal(err)
	}

	blocked := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score"}})
	if blocked.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(blocked.Body)
		t.Fatalf("expected missing entitlement 403, got %d: %s", blocked.StatusCode, body)
	}
	blocked.Body.Close()

	segResp := postJSON(t, srv.URL+"/admin/dataworks/customer-segments", "", map[string]any{
		"segment_key": "bank_enterprise", "industry": "banking", "buyer_type": "risk team",
		"pain_points": []string{"loan approval", "credit risk"}, "budget_level": "enterprise",
	})
	if segResp.StatusCode != http.StatusOK {
		t.Fatalf("segment status = %d", segResp.StatusCode)
	}
	segResp.Body.Close()

	fitResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/fit-scores", "", map[string]any{"customer_segment": "bank_enterprise"})
	if fitResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(fitResp.Body)
		t.Fatalf("fit score status = %d: %s", fitResp.StatusCode, body)
	}
	var fitBody struct {
		FitScores []store.ProductFitScore `json:"fit_scores"`
	}
	_ = json.NewDecoder(fitResp.Body).Decode(&fitBody)
	fitResp.Body.Close()
	if len(fitBody.FitScores) != 1 || fitBody.FitScores[0].FitScore < 60 {
		t.Fatalf("unexpected fit score body: %+v", fitBody)
	}

	validTo := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	scopeResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/contract-scopes", "", map[string]any{
		"contract_key": "ct_bank", "customer_key": "cust_bank", "allowed_fields": []string{"score", "risk_band"},
		"rate_limit": 100, "valid_to": validTo, "purpose": "credit risk monitoring",
	})
	if scopeResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(scopeResp.Body)
		t.Fatalf("contract scope status = %d: %s", scopeResp.StatusCode, body)
	}
	scopeResp.Body.Close()

	entResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/entitlements", "", map[string]any{
		"id": "ent_bank", "api_key_id": "key_bank", "customer_key": "cust_bank", "contract_key": "ct_bank",
		"scope": "data_product:query", "expires_at": validTo,
	})
	if entResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(entResp.Body)
		t.Fatalf("entitlement status = %d: %s", entResp.StatusCode, body)
	}
	entResp.Body.Close()

	slaResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/sla", "", map[string]any{
		"refresh_cycle": "daily", "latency_target_ms": 250, "availability_target": 0.995, "support_level": "business",
	})
	if slaResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(slaResp.Body)
		t.Fatalf("sla status = %d: %s", slaResp.StatusCode, body)
	}
	slaResp.Body.Close()

	openAPIResp, err := http.Get(srv.URL + "/admin/dataworks/products/dw_credit_score/openapi?contract_key=ct_bank")
	if err != nil {
		t.Fatal(err)
	}
	openAPIRaw, _ := io.ReadAll(openAPIResp.Body)
	openAPIResp.Body.Close()
	if openAPIResp.StatusCode != http.StatusOK || !strings.Contains(string(openAPIRaw), "x-dataworks-contract") || !strings.Contains(string(openAPIRaw), "risk_band") {
		t.Fatalf("unexpected openapi response status=%d body=%s", openAPIResp.StatusCode, openAPIRaw)
	}

	allowed := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score", "risk_band"}})
	if allowed.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(allowed.Body)
		t.Fatalf("expected runtime query 200, got %d: %s", allowed.StatusCode, body)
	}
	var queryBody struct {
		Data map[string]any `json:"data"`
		Mock bool           `json:"mock"`
	}
	_ = json.NewDecoder(allowed.Body).Decode(&queryBody)
	allowed.Body.Close()
	if !queryBody.Mock || len(queryBody.Data) != 2 || queryBody.Data["score"] == nil || queryBody.Data["risk_band"] == nil {
		t.Fatalf("unexpected query body: %+v", queryBody)
	}

	forbidden := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score/query", "bank-secret", map[string]any{"fields": []string{"score", "raw_ssn"}})
	if forbidden.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(forbidden.Body)
		t.Fatalf("expected forbidden field 403, got %d: %s", forbidden.StatusCode, body)
	}
	forbidden.Body.Close()

	v1 := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/versions", "", map[string]any{})
	if v1.StatusCode != http.StatusOK {
		t.Fatalf("version v1 status = %d", v1.StatusCode)
	}
	v1.Body.Close()
	product.RiskScore = 58
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	v2 := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/versions", "", map[string]any{})
	if v2.StatusCode != http.StatusOK {
		t.Fatalf("version v2 status = %d", v2.StatusCode)
	}
	v2.Body.Close()

	diffResp, err := http.Get(srv.URL + "/admin/dataworks/products/dw_credit_score/version-diff?from=1&to=2")
	if err != nil {
		t.Fatal(err)
	}
	defer diffResp.Body.Close()
	diffRaw, _ := io.ReadAll(diffResp.Body)
	if diffResp.StatusCode != http.StatusOK || !strings.Contains(string(diffRaw), "risk_score") {
		t.Fatalf("unexpected diff response status=%d body=%s", diffResp.StatusCode, diffRaw)
	}

	watermarkResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/watermarks", "", map[string]any{
		"asset_key": "loan_history", "data_as_of": time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano), "delay_status": "stale",
	})
	if watermarkResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(watermarkResp.Body)
		t.Fatalf("watermark status = %d: %s", watermarkResp.StatusCode, body)
	}
	watermarkResp.Body.Close()

	costResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/costs", "", map[string]any{
		"query_cost": 100000, "llm_cost": 50000, "ops_cost": 40000, "data_processing_cost": 25000, "estimated_margin": -1000,
	})
	if costResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(costResp.Body)
		t.Fatalf("cost status = %d: %s", costResp.StatusCode, body)
	}
	costResp.Body.Close()

	proposalResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/proposal-ab", "", map[string]any{
		"customer_key": "cust_bank", "customer_segment": "bank_enterprise",
	})
	if proposalResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(proposalResp.Body)
		t.Fatalf("proposal-ab status = %d: %s", proposalResp.StatusCode, body)
	}
	var proposalBody struct {
		Variants []map[string]any `json:"variants"`
	}
	_ = json.NewDecoder(proposalResp.Body).Decode(&proposalBody)
	proposalResp.Body.Close()
	if len(proposalBody.Variants) != 3 {
		t.Fatalf("expected three proposal variants, got %+v", proposalBody)
	}

	retirementResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score/retirement", "", map[string]any{})
	if retirementResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(retirementResp.Body)
		t.Fatalf("retirement status = %d: %s", retirementResp.StatusCode, body)
	}
	var retirementBody struct {
		Candidate store.RetirementCandidate `json:"candidate"`
	}
	_ = json.NewDecoder(retirementResp.Body).Decode(&retirementBody)
	retirementResp.Body.Close()
	if retirementBody.Candidate.Recommendation == "" {
		t.Fatalf("missing retirement recommendation: %+v", retirementBody)
	}

	actionResp, err := http.Get(srv.URL + "/admin/dataworks/action-center")
	if err != nil {
		t.Fatal(err)
	}
	actionRaw, _ := io.ReadAll(actionResp.Body)
	actionResp.Body.Close()
	if actionResp.StatusCode != http.StatusOK || !strings.Contains(string(actionRaw), "stale_watermarks") || !strings.Contains(string(actionRaw), "negative_margin") {
		t.Fatalf("unexpected action center status=%d body=%s", actionResp.StatusCode, actionRaw)
	}
}
