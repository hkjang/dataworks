package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"dataworks/internal/store"
)

func TestDataWorksPlatformControlPlanes(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 16, filepath.Join(t.TempDir(), "platform.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/admin/dataworks/workspaces", "", map[string]any{
		"id": "dwws_credit", "workspace_key": "credit-platform", "name": "Credit Data Platform",
		"owner": "data-platform", "environment": "development", "tags": []string{"credit", "governed"},
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/workspaces/dwws_credit/members", "", map[string]any{
		"user_id": "alice", "role": "owner",
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/assets", "", map[string]any{
		"id": "asset_credit", "asset_key": "credit_features", "name": "Credit Features",
		"domain": "credit", "owner": "credit-data", "sensitivity": "personal_credit",
		"refresh_cycle": "daily", "columns_summary": "customer_id, score_band, balance",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	product := store.DataProduct{
		ID: "dprod_credit_api", ProductKey: "credit_api", NameKO: "Credit API", Description: "Approved credit feature API",
		SourceType: "dataset", SourceRef: "credit_features", Owner: "data-business", Sensitivity: "pseudonymized",
		Status: "published", RiskScore: 45, RevenueScore: 80,
	}
	if err := db.UpsertDataProduct(context.Background(), product); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProductCost(context.Background(), store.ProductCost{
		ProductKey: product.ProductKey, QueryCost: 10000, LLMCost: 5000, DataProcessingCost: 15000,
		OpsCost: 200000, ExpectedRevenue: 1000000, Currency: "KRW", UpdatedBy: "test",
	}); err != nil {
		t.Fatal(err)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/tools", "", map[string]any{
		"id": "dwtool_catalog", "tool_key": "catalog_lookup", "workspace_id": "dwws_credit",
		"name": "Catalog Lookup", "tool_type": "mcp", "server_label": "catalog",
		"owner": "platform", "risk_level": "low", "input_schema": map[string]any{"type": "object"},
		"output_schema": map[string]any{"type": "object"}, "allowed_parameters": map[string]any{"query": "string"},
		"masking_level": "partial", "enabled": true,
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/tools/dwtool_catalog/test", "", map[string]any{})
	requireStatus(t, resp, http.StatusOK)
	var toolTest struct {
		Status string `json:"status"`
	}
	decodeAndClose(t, resp, &toolTest)
	if toolTest.Status != "passed" {
		t.Fatalf("tool contract test status=%q", toolTest.Status)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/semantic/metrics", "", map[string]any{
		"id": "dwmetric_approval", "metric_key": "approval_rate", "workspace_id": "dwws_credit",
		"name": "Approval Rate", "expression": "approved_applications / total_applications",
		"aggregation": "ratio", "dimensions": []string{"month", "channel"}, "owner": "risk-analytics",
		"status": "approved", "source_urn": store.DataWorksURN("dataset", "credit_features"),
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/semantic/glossary", "", map[string]any{
		"id": "term_approval", "schema_name": "credit", "term": "approval rate",
		"mapping": "approved_applications / total_applications", "description": "Standard approval ratio",
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/semantic/assertions", "", map[string]any{
		"id": "assert_score", "entity_urn": store.DataWorksURN("dataset", "credit_features"),
		"assertion_type": "freshness", "operator": "lte_hours", "expected_value": "24",
		"severity": "high", "enabled": true,
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/flows", "", map[string]any{
		"id": "dwflow_credit", "flow_key": "credit-review-flow", "workspace_id": "dwws_credit",
		"name": "Credit Review Flow", "flow_type": "hybrid", "owner": "risk-platform",
		"nodes": []map[string]any{
			{"node_key": "input", "node_type": "input", "name": "Input"},
			{"node_key": "lookup", "node_type": "tool", "name": "Catalog Lookup", "ref_urn": store.DataWorksURN("tool", "catalog_lookup"), "config": map[string]any{"tool_id": "dwtool_catalog"}},
			{"node_key": "output", "node_type": "output", "name": "Output", "ref_urn": store.DataWorksURN("product", "credit_api")},
		},
		"edges": []map[string]any{
			{"source_node_key": "input", "target_node_key": "lookup"},
			{"source_node_key": "lookup", "target_node_key": "output"},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/flows/dwflow_credit/run", "", map[string]any{"input": "evaluate catalog metadata"})
	requireStatus(t, resp, http.StatusOK)
	var flowRun struct {
		Run store.DataWorksFlowRun `json:"run"`
	}
	decodeAndClose(t, resp, &flowRun)
	if flowRun.Run.Status != "succeeded" {
		t.Fatalf("flow run status=%q result=%+v", flowRun.Run.Status, flowRun.Run.ResultSummary)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/flows/dwflow_credit/promote", "", map[string]any{"target": "test"})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/agents", "", map[string]any{
		"id": "dwagent_credit", "agent_key": "credit-advisor", "workspace_id": "dwws_credit",
		"name": "Credit Advisor", "purpose": "Prepare governed credit product evidence", "owner": "risk-platform",
		"risk_level": "medium", "status": "active", "allowed_tools": []string{"dwtool_catalog"},
		"max_cost": 5, "max_steps": 6,
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	resp = postJSON(t, srv.URL+"/admin/dataworks/agents/dwagent_credit/run", "", map[string]any{
		"input": "find the governed credit dataset", "tool_ids": []string{"dwtool_catalog"}, "params": map[string]any{"query": "credit"},
	})
	requireStatus(t, resp, http.StatusOK)
	var agentRun struct {
		Session store.DataWorksAgentSession `json:"session"`
		Traces  []store.DataWorksAgentTrace `json:"traces"`
	}
	decodeAndClose(t, resp, &agentRun)
	if agentRun.Session.Status != "succeeded" || len(agentRun.Traces) < 3 {
		t.Fatalf("agent run=%+v traces=%+v", agentRun.Session, agentRun.Traces)
	}

	referenceResp, err := http.Get(srv.URL + "/admin/dataworks/reference-catalog")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, referenceResp, http.StatusOK)
	var referenceCatalog struct {
		Assets           []store.DataAsset          `json:"assets"`
		Products         []store.DataProduct        `json:"products"`
		Workspaces       []store.DataWorksWorkspace `json:"workspaces"`
		Flows            []store.DataWorksFlow      `json:"flows"`
		Agents           []store.DataWorksAgent     `json:"agents"`
		Tools            []store.DataWorksTool      `json:"tools"`
		CustomerSegments []store.CustomerSegment    `json:"customer_segments"`
		Owners           []string                   `json:"owners"`
	}
	decodeAndClose(t, referenceResp, &referenceCatalog)
	if len(referenceCatalog.Assets) == 0 || len(referenceCatalog.Products) == 0 || len(referenceCatalog.Workspaces) == 0 ||
		len(referenceCatalog.Flows) == 0 || len(referenceCatalog.Agents) == 0 || len(referenceCatalog.Tools) == 0 {
		t.Fatalf("reference catalog missing active objects: %+v", referenceCatalog)
	}
	if len(referenceCatalog.CustomerSegments) < 5 || len(referenceCatalog.Owners) < 4 {
		t.Fatalf("reference catalog missing standard choices: segments=%d owners=%v", len(referenceCatalog.CustomerSegments), referenceCatalog.Owners)
	}
	foundComplianceOwner := false
	for _, owner := range referenceCatalog.Owners {
		if owner == "compliance" {
			foundComplianceOwner = true
			break
		}
	}
	if !foundComplianceOwner {
		t.Fatalf("reference catalog missing compliance owner: %v", referenceCatalog.Owners)
	}

	traceResp, err := http.Get(srv.URL + "/admin/dataworks/agents/dwagent_credit/trace?session_id=" + url.QueryEscape(agentRun.Session.ID))
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, traceResp, http.StatusOK)
	traceResp.Body.Close()

	searchResp, err := http.Get(srv.URL + "/admin/dataworks/metadata/search?q=credit")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, searchResp, http.StatusOK)
	var search struct {
		Entities []store.MetadataEntity `json:"entities"`
	}
	decodeAndClose(t, searchResp, &search)
	if len(search.Entities) < 4 {
		t.Fatalf("expected synchronized metadata entities, got %d", len(search.Entities))
	}

	impactPath := "/admin/dataworks/metadata/" + url.PathEscape(store.DataWorksURN("tool", "catalog_lookup")) + "/impact"
	impactResp, err := http.Get(srv.URL + impactPath)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, impactResp, http.StatusOK)
	var impact struct {
		Impact store.MetadataImpact `json:"impact"`
	}
	decodeAndClose(t, impactResp, &impact)
	if impact.Impact.AffectedByType["agent"] == 0 {
		t.Fatalf("tool impact did not include agent: %+v", impact.Impact)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/policies/simulate", "", map[string]any{
		"workspace_id": "dwws_credit", "target_type": "product", "target_id": "credit_api",
		"context": map[string]any{"sensitivity": "personal_credit", "external_sharing": true, "pseudonymized": false},
	})
	requireStatus(t, resp, http.StatusOK)
	var policy struct {
		Simulation store.DataWorksPolicySimulation `json:"simulation"`
	}
	decodeAndClose(t, resp, &policy)
	if policy.Simulation.Decision != "block" || policy.Simulation.RiskScore < 50 {
		t.Fatalf("policy simulation=%+v", policy.Simulation)
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/synthetic/generate", "", map[string]any{
		"workspace_id": "dwws_credit", "name": "Credit API Demo", "purpose": "customer demo", "row_count": 5,
		"schema": []map[string]any{
			{"name": "customer_id", "type": "string", "sensitivity": "identifier"},
			{"name": "score", "type": "integer", "sensitivity": "internal"},
		},
	})
	requireStatus(t, resp, http.StatusCreated)
	var synthetic struct {
		Dataset store.SyntheticDataset `json:"dataset"`
	}
	decodeAndClose(t, resp, &synthetic)
	if synthetic.Dataset.RowCount != 5 || len(synthetic.Dataset.Sample) != 5 || synthetic.Dataset.Sample[0]["customer_id"] == "" {
		t.Fatalf("synthetic dataset=%+v", synthetic.Dataset)
	}

	econResp, err := http.Get(srv.URL + "/admin/dataworks/products/credit_api/unit-economics?expected_calls=20000&unit_price=80&scenario=growth")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, econResp, http.StatusOK)
	var economics struct {
		UnitEconomics store.DataWorksUnitEconomics `json:"unit_economics"`
	}
	decodeAndClose(t, econResp, &economics)
	if economics.UnitEconomics.ExpectedRevenue != 1600000 || economics.UnitEconomics.Margin == 0 {
		t.Fatalf("unit economics=%+v", economics.UnitEconomics)
	}

	marketResp, err := http.Get(srv.URL + "/admin/dataworks/marketplace/items?q=credit")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, marketResp, http.StatusOK)
	var marketplace struct {
		Items []store.DataWorksMarketplaceItem `json:"items"`
	}
	decodeAndClose(t, marketResp, &marketplace)
	if len(marketplace.Items) < 2 {
		t.Fatalf("marketplace items=%+v", marketplace.Items)
	}
	resp = postJSON(t, srv.URL+"/admin/dataworks/marketplace/subscribe", "", map[string]any{
		"item_id": marketplace.Items[0].ID, "purpose": "approved internal product evaluation",
	})
	requireStatus(t, resp, http.StatusCreated)
	resp.Body.Close()

	for _, path := range []string{"/admin/dataworks/agentops", "/admin/dataworks/platform/overview"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		requireStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	resp = postJSON(t, srv.URL+"/admin/dataworks/executive/simulate", "", map[string]any{
		"customer_segment": "banks", "product_keys": []string{"credit_api"}, "customers": 20,
		"calls_per_customer": 5000, "unit_price": 70, "poc_success_rate": 0.7, "risk_mitigation": 0.4,
	})
	requireStatus(t, resp, http.StatusOK)
	var executive map[string]any
	decodeAndClose(t, resp, &executive)
	simulation, _ := executive["simulation"].(map[string]any)
	scenarios, _ := simulation["scenarios"].([]any)
	if len(scenarios) != 3 {
		encoded, _ := json.Marshal(executive)
		t.Fatalf("executive scenarios missing: %s", encoded)
	}
}
