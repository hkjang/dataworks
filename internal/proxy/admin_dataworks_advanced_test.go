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

func TestDataWorksAdvancedOperations(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	defer db.Close()

	var resp *http.Response
	var err error

	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw-adv.ndjson"))
	logger.Start()
	defer logger.Stop(ctx)

	var server *Server
	server, err = NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	// Setup Initial Entities
	product := store.DataProduct{
		ID: "dprod_adv", ProductKey: "dw_credit_score_adv", NameKO: "Advanced Credit Score API",
		SourceType: "api", SourceRef: "loan_history_adv", Owner: "data",
		Sensitivity: "personal_credit", Status: "published", RiskScore: 82,
	}
	if err := db.UpsertDataProduct(ctx, product); err != nil {
		t.Fatal(err)
	}

	asset := store.DataAsset{
		ID: "asset_adv", AssetKey: "loan_history_adv", Name: "Loan History Adv", Domain: "finance",
		ColumnsSummary: "id:string,score:int,name:string",
	}
	if err := db.UpsertDataAsset(ctx, asset); err != nil {
		t.Fatal(err)
	}

	// 1. Data Quality Rule Builder & Evaluator
	t.Run("Data Quality Rule and Evaluation", func(t *testing.T) {
		ruleReq := map[string]any{
			"column_name": "score",
			"rule_type":   "null_rate",
			"threshold":   0.05,
			"enabled":     true,
		}
		resp := postJSON(t, srv.URL+"/admin/dataworks/assets/loan_history_adv/quality/rules", "admin-token", ruleReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create quality rule failed: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Get rules
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/assets/loan_history_adv/quality/rules", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get quality rules failed: status=%d err=%v", resp.StatusCode, err)
		}
		var rulesList struct {
			Rules []store.DataQualityRule `json:"rules"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&rulesList)
		resp.Body.Close()
		if len(rulesList.Rules) == 0 {
			t.Fatal("expected at least 1 quality rule")
		}

		// Evaluate Quality Rules
		evalResp := postJSON(t, srv.URL+"/admin/dataworks/assets/loan_history_adv/quality/evaluate", "admin-token", map[string]any{})
		if evalResp.StatusCode != http.StatusOK {
			t.Fatalf("evaluate quality failed: %d", evalResp.StatusCode)
		}
		var evalResult struct {
			QualityScore int                        `json:"quality_score"`
			Results      []store.DataQualityResult `json:"results"`
		}
		_ = json.NewDecoder(evalResp.Body).Decode(&evalResult)
		evalResp.Body.Close()
		if evalResult.QualityScore == 0 || len(evalResult.Results) == 0 {
			t.Fatalf("invalid evaluation results: %+v", evalResult)
		}

		// List Results
		req, _ = http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/assets/loan_history_adv/quality/results", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("list quality results failed: %d", resp.StatusCode)
		}
		var resultsList struct {
			Results []store.DataQualityResult `json:"results"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&resultsList)
		resp.Body.Close()
		if len(resultsList.Results) == 0 {
			t.Fatal("expected quality results in database")
		}
	})

	// 2. Schema Drift Detector
	t.Run("Schema Drift Detection", func(t *testing.T) {
		driftReq := map[string]any{
			"columns": []map[string]any{
				{"name": "id", "type": "string"},
				{"name": "score", "type": "float64"}, // type change from int to float64
				{"name": "new_column", "type": "string"}, // added column
				// deleted column: name
			},
		}
		driftResp := postJSON(t, srv.URL+"/admin/dataworks/assets/loan_history_adv/drift/detect", "admin-token", driftReq)
		if driftResp.StatusCode != http.StatusOK {
			t.Fatalf("drift detection failed: %d", driftResp.StatusCode)
		}
		var driftResult struct {
			Drifts []store.SchemaDrift `json:"drifts"`
			Synced bool                `json:"synced"`
		}
		_ = json.NewDecoder(driftResp.Body).Decode(&driftResult)
		driftResp.Body.Close()
		if len(driftResult.Drifts) == 0 {
			t.Fatal("expected schema drifts to be detected")
		}

		// Get drifts list
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/assets/loan_history_adv/drift", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get drifts failed: %d", resp.StatusCode)
		}
		var driftsList struct {
			Drifts []store.SchemaDrift `json:"drifts"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&driftsList)
		resp.Body.Close()
		if len(driftsList.Drifts) == 0 {
			t.Fatal("expected drifts in db")
		}

		// Product impact
		req, _ = http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/products/dw_credit_score_adv/drift-impact", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get product drift-impact failed: %d", resp.StatusCode)
		}
		var impact struct {
			ImpactRating string `json:"impact_rating"`
			MaxImpact    float64 `json:"max_impact"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&impact)
		resp.Body.Close()
		if impact.ImpactRating == "" {
			t.Fatal("missing impact rating")
		}
	})

	// 3. Product SLA Monitor
	t.Run("Product SLA Monitor", func(t *testing.T) {
		sla := store.ProductSLA{
			ProductKey: "dw_credit_score_adv", LatencyTargetMS: 150, AvailabilityTarget: 0.995,
		}
		if err := db.UpsertProductSLA(ctx, sla); err != nil {
			t.Fatal(err)
		}

		checkReq := map[string]any{
			"metric_type":  "latency",
			"actual_value": 180.0, // Breaches target
		}
		resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score_adv/sla/check", "admin-token", checkReq)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sla check failed: %d", resp.StatusCode)
		}
		var metricResult struct {
			Metric store.SLAMetric `json:"metric"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&metricResult)
		resp.Body.Close()
		if metricResult.Metric.Status != "breached" {
			t.Fatalf("expected breached status, got %s", metricResult.Metric.Status)
		}

		// Check SLA status listing
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/products/dw_credit_score_adv/sla/status", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get SLA status failed: %d", resp.StatusCode)
		}
		var statusList struct {
			Metrics []store.SLAMetric `json:"metrics"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&statusList)
		resp.Body.Close()
		if len(statusList.Metrics) == 0 {
			t.Fatal("expected SLA metrics in database")
		}
	})

	// 4. API Usage Metering
	t.Run("API Usage Metering Summary", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/products/dw_credit_score_adv/usage", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get usage failed: %d", resp.StatusCode)
		}
		var usageResult struct {
			Summary map[string]any `json:"summary"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&usageResult)
		resp.Body.Close()
		if _, ok := usageResult.Summary["total_calls"]; !ok {
			t.Fatal("missing total_calls in summary")
		}
	})

	// 5. Policy-as-Code Engine
	t.Run("Policy-as-Code Engine", func(t *testing.T) {
		policyReq := map[string]any{
			"policy_type":     "privacy",
			"rule_expression": "sensitivity=personal_credit -> require_compliance_approval",
			"action":          "block",
			"enabled":         true,
		}
		resp := postJSON(t, srv.URL+"/admin/dataworks/policy/rules", "admin-token", policyReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create policy rule failed: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Evaluate Policy
		evalResp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_credit_score_adv/policy/evaluate", "admin-token", map[string]any{})
		if evalResp.StatusCode != http.StatusOK {
			t.Fatalf("evaluate policies failed: %d", evalResp.StatusCode)
		}
		var evalResult struct {
			Allowed     bool             `json:"allowed"`
			Evaluations []map[string]any `json:"evaluations"`
		}
		_ = json.NewDecoder(evalResp.Body).Decode(&evalResult)
		evalResp.Body.Close()
		if len(evalResult.Evaluations) == 0 {
			t.Fatal("expected at least 1 policy evaluation result")
		}
	})

	// 6. Prompt Regression Test
	t.Run("Prompt Regression Test", func(t *testing.T) {
		run := store.FactoryRun{
			ID: "frun_regression", RunType: "ideas.generate", Model: "gpt-4o",
			Status: "completed", CreatedAt: "2026-07-11T00:00:00Z",
		}
		if err := db.InsertFactoryRun(ctx, run); err != nil {
			t.Fatal(err)
		}

		compareReq := map[string]any{
			"compare_version": 2,
			"compare_model":   "claude-3-5-sonnet",
		}
		resp := postJSON(t, srv.URL+"/admin/dataworks/factory/runs/frun_regression/regression-test", "admin-token", compareReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create regression test failed: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Get Regression History
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/factory/runs/frun_regression/regression-test", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get regression tests failed: %d", resp.StatusCode)
		}
		var testHistory struct {
			Tests []store.PromptRegressionTest `json:"tests"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&testHistory)
		resp.Body.Close()
		if len(testHistory.Tests) == 0 {
			t.Fatal("expected regression tests in history")
		}
	})

	// 7. Proposal Experiment Lab
	t.Run("Proposal Experiment Lab", func(t *testing.T) {
		expReq := map[string]any{
			"product_key":      "dw_credit_score_adv",
			"customer_segment": "finance",
			"headline_variant": "Premium Credit Score API",
			"pricing_variant":  99000.0,
			"package_variant":  "unlimited",
		}
		resp := postJSON(t, srv.URL+"/admin/dataworks/proposal-experiments", "admin-token", expReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("create proposal experiment failed: %d", resp.StatusCode)
		}
		var wrapper struct {
			Experiment store.ProposalExperiment `json:"experiment"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&wrapper)
		exp := wrapper.Experiment
		resp.Body.Close()

		// Feed response
		resp = postJSON(t, srv.URL+"/admin/dataworks/proposal-experiments/"+exp.ID+"/feedback", "admin-token", map[string]any{"positive": true})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("post experiment feedback failed: %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	// 8. Internal Data Product Marketplace
	t.Run("Data Product Marketplace and Bookmarks/Subscriptions", func(t *testing.T) {
		// List Marketplace
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/admin/dataworks/marketplace/products", nil)
		req.Header.Set("Authorization", "Bearer admin-token")
		resp, err = http.DefaultClient.Do(req)
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("get marketplace products failed: %d", resp.StatusCode)
		}
		var productsList struct {
			Products []store.DataProduct `json:"products"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&productsList)
		resp.Body.Close()
		if len(productsList.Products) == 0 {
			t.Fatal("expected at least 1 marketplace product")
		}

		// Bookmark Product
		resp = postJSON(t, srv.URL+"/admin/dataworks/marketplace/bookmarks", "admin-token", map[string]any{"product_key": "dw_credit_score_adv"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("bookmark failed: %d", resp.StatusCode)
		}
		resp.Body.Close()

		// Subscribe Product
		subReq := map[string]any{
			"product_key": "dw_credit_score_adv",
			"purpose":     "Internal analysis and dashboard reporting",
		}
		resp = postJSON(t, srv.URL+"/admin/dataworks/marketplace/subscriptions", "admin-token", subReq)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("subscribe failed: %d", resp.StatusCode)
		}
		resp.Body.Close()
	})

	// 9. Runtime Query Masking and Metering
	t.Run("Query Runtime Masking and Metering", func(t *testing.T) {
		// Add API Key
		apiKey := store.APIKeyRecord{
			ID: "apikey_adv", KeyHash: hashProxyKey("test-client-token"), Name: "adv-client",
			Team: "team_adv", Status: "active",
		}
		_ = db.UpsertAPIKey(ctx, apiKey)

		// Add Entitlement and Contract Scope with masking_policy = "redact"
		contract := store.ContractScope{
			ContractKey: "contract_adv", ProductKey: "dw_credit_score_adv", CustomerKey: "cust_adv",
			AllowedFields: []string{"product_key", "score", "risk_band"}, RateLimit: 100, Status: "active",
			MaskingPolicy: "redact", Purpose: "credit analysis",
		}
		_ = db.UpsertContractScope(ctx, contract)

		ent := store.APIEntitlement{
			ID: "ent_adv", APIKeyID: "apikey_adv", APIKeyHash: hashProxyKey("test-client-token"),
			ProductKey: "dw_credit_score_adv", CustomerKey: "cust_adv", ContractKey: "contract_adv",
			Status: "active", Scope: "*",
		}
		_ = db.UpsertAPIEntitlement(ctx, ent)

		// Post a query request
		queryReq := map[string]any{
			"fields": []string{"product_key", "score", "risk_band"},
		}
		resp := postJSON(t, srv.URL+"/v1/data-products/dw_credit_score_adv/query", "test-client-token", queryReq)
		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("runtime query failed: status=%d body=%s", resp.StatusCode, body)
		}
		var result struct {
			Data map[string]any `json:"data"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&result)
		resp.Body.Close()

		// Verify Masking (score is int, should be redacted to 0 or ****)
		scoreVal, ok := result.Data["score"]
		if !ok {
			t.Fatal("missing score in query response data")
		}
		// score should be 0 because policy was "redact" and score is numeric
		if val, ok := scoreVal.(float64); !ok || val != 0 {
			t.Fatalf("expected masked score value 0, got %+v", scoreVal)
		}

		// Verify Metering Incremented
		usage, err := db.ListUsageMetering(ctx, "dw_credit_score_adv")
		if err != nil || len(usage) == 0 {
			t.Fatalf("usage metering failed to record: %v", err)
		}
		if usage[0].TotalCalls != 1 {
			t.Fatalf("expected total calls 1, got %d", usage[0].TotalCalls)
		}
	})
}
