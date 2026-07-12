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

func TestDataWorksOperationalAPIs(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dataworks.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/dataworks/assets", "", map[string]any{
		"id": "asset_loan", "asset_key": "loan_history", "name": "Loan History", "domain": "credit",
		"owner": "risk-data", "columns_summary": "loan_id, overdue_days, balance", "sensitivity": "personal_credit", "refresh_cycle": "daily",
	})
	requireStatus(t, resp, http.StatusOK)

	resp = postJSON(t, srv.URL+"/admin/dataworks/assets/loan_history/readiness/check", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	var readinessBody struct {
		Readiness store.DataAssetReadinessScore `json:"readiness"`
	}
	decodeAndClose(t, resp, &readinessBody)
	if readinessBody.Readiness.OverallScore == 0 || readinessBody.Readiness.Status == "" {
		t.Fatalf("readiness missing score/status: %+v", readinessBody.Readiness)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/factory/definitions", "", map[string]any{
		"product_key": "dw_credit_risk_api", "title": "신용 리스크 API", "target_industry": "금융",
		"target_customers": []string{"은행"}, "customer_need": "여신 위험 조기탐지",
		"data_assets": []string{"loan_history"}, "delivery_method": "API",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp, err = http.Get(srv.URL + "/admin/dataworks/products/dw_credit_risk_api/evidence")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, resp, http.StatusOK)
	var evidenceBody struct {
		Evidence []store.ProductEvidence `json:"evidence"`
	}
	decodeAndClose(t, resp, &evidenceBody)
	if len(evidenceBody.Evidence) == 0 {
		t.Fatal("expected generated evidence pack")
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_risk_api/canvas/generate", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/risk/check", "", map[string]any{"product_key": "dw_credit_risk_api"})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_risk_api/regulatory-trace", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/factory/products/dw_credit_risk_api/publish", "", map[string]any{})
	requireStatus(t, resp, http.StatusConflict)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_risk_api/regulatory-trace", "", map[string]any{
		"trace": []map[string]any{
			{"risk_domain": "legal_review", "question": "법무 승인", "answer": "approved", "decision": "approved", "reviewer": "legal"},
			{"risk_domain": "compliance_review", "question": "준법 승인", "answer": "approved", "decision": "approved", "reviewer": "compliance"},
			{"risk_domain": "data_owner_approval", "question": "데이터오너 승인", "answer": "approved", "decision": "approved", "reviewer": "owner"},
		},
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/factory/products/dw_credit_risk_api/publish", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_risk_api/api-contract", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_risk_api/mock", "", map[string]any{"customer_type": "bank", "period": "2026-06"})
	requireStatus(t, resp, http.StatusOK)
	var mockBody struct {
		MockResponse map[string]any `json:"mock_response"`
	}
	decodeAndClose(t, resp, &mockBody)
	if mockBody.MockResponse["product_key"] != "dw_credit_risk_api" {
		t.Fatalf("mock product mismatch: %+v", mockBody.MockResponse)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/proposals", "", map[string]any{"product_key": "dw_credit_risk_api", "target_customer_type": "bank"})
	requireStatus(t, resp, http.StatusOK)
	var proposalBody struct {
		Package store.ProposalPackage `json:"package"`
	}
	decodeAndClose(t, resp, &proposalBody)
	if proposalBody.Package.ID == "" {
		t.Fatal("proposal package id missing")
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/proposals/"+proposalBody.Package.ID+"/feedback", "", map[string]any{
		"result": "poc", "reason": "interested", "next_action": "schedule_poc",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/poc/plans", "", map[string]any{"product_key": "dw_credit_risk_api"})
	requireStatus(t, resp, http.StatusOK)
	var pocBody struct {
		POCPlan store.ProductPOCPlan `json:"poc_plan"`
	}
	decodeAndClose(t, resp, &pocBody)
	if pocBody.POCPlan.ID == "" {
		t.Fatal("poc plan id missing")
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/poc/"+pocBody.POCPlan.ID+"/outcome", "", map[string]any{
		"success": true, "metric_result": "AUC 0.82", "conversion_status": "contract_candidate",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	for _, path := range []string{
		"/admin/dataworks/assets/loan_history/lineage",
		"/admin/dataworks/portfolio/graph",
		"/admin/dataworks/analytics/funnel",
		"/admin/dataworks/products/dw_credit_risk_api/funnel",
	} {
		resp, err = http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, resp, http.StatusOK)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}
}

func TestDataWorksProductLifecycleEvidenceAndActions(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dataworks-lifecycle.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	product := store.DataProduct{
		ID: "dprod_lifecycle", ProductKey: "dw_lifecycle", NameKO: "Lifecycle Product",
		SourceType: "api", SourceRef: "asset_lifecycle", Owner: "data-business",
		Sensitivity: "internal", Status: "draft", RevenueScore: 75, RiskScore: 25,
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}

	assertTransition := func(action, wantStatus string) {
		t.Helper()
		resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_lifecycle/"+action, "", map[string]any{})
		requireStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		stored, ok, err := db.GetDataProduct(context.Background(), product.ProductKey)
		if err != nil || !ok || stored.Status != wantStatus {
			t.Fatalf("transition %s: status=%q ok=%v err=%v", action, stored.Status, ok, err)
		}
	}

	assertTransition("submit", "review")
	invalid := postJSON(t, srv.URL+"/admin/dataworks/products/dw_lifecycle/submit", "", map[string]any{})
	requireStatus(t, invalid, http.StatusConflict)
	invalid.Body.Close()
	assertTransition("approve", "approved")
	assertTransition("reject", "draft")

	product.Status = "published"
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	assertTransition("archive", "archived")

	evidenceResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_lifecycle/evidence", "", map[string]any{})
	requireStatus(t, evidenceResp, http.StatusOK)
	var evidenceBody struct {
		Evidence []store.ProductEvidence `json:"evidence"`
	}
	decodeAndClose(t, evidenceResp, &evidenceBody)
	if len(evidenceBody.Evidence) < 3 {
		t.Fatalf("expected refreshed evidence rows, got %+v", evidenceBody.Evidence)
	}

	actionsResp, err := http.Get(srv.URL + "/admin/dataworks/products/dw_lifecycle/actions")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, actionsResp, http.StatusOK)
	var actionsBody struct {
		Actions []struct {
			ActionType string `json:"action_type"`
		} `json:"actions"`
	}
	decodeAndClose(t, actionsResp, &actionsBody)
	foundEvidenceRefresh := false
	for _, action := range actionsBody.Actions {
		if action.ActionType == "dataworks.product.evidence.refresh" {
			foundEvidenceRefresh = true
			break
		}
	}
	if !foundEvidenceRefresh {
		t.Fatalf("product actions missing evidence refresh: %+v", actionsBody.Actions)
	}
}

func requireStatus(t *testing.T, resp *http.Response, want int) {
	t.Helper()
	if resp.StatusCode == want {
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	t.Fatalf("status = %d want %d body=%s", resp.StatusCode, want, string(body))
}

func decodeAndClose(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}
