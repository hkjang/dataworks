package proxy

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dataworks/internal/store"
)

func (s *Server) handleDataWorksPolicySimulations(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		simulations, err := s.db.ListDataWorksPolicySimulations(r.Context(), r.URL.Query().Get("workspace_id"), r.URL.Query().Get("target_type"), intQuery(r, "limit", 100))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "policy_simulation_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"simulations": simulations})
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var raw map[string]any
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	contextMap, _ := raw["context"].(map[string]any)
	if contextMap == nil {
		contextMap = map[string]any{}
	}
	for key, value := range raw {
		if key != "context" && key != "workspace_id" && key != "target_type" && key != "target_id" {
			if _, exists := contextMap[key]; !exists {
				contextMap[key] = value
			}
		}
	}
	sim := evaluateDataWorksPolicy(
		stringMapValue(raw, "workspace_id"), stringMapValue(raw, "target_type"),
		stringMapValue(raw, "target_id"), contextMap, adminID(r),
	)
	if sim.TargetType == "" {
		writeOpenAIError(w, http.StatusBadRequest, "target_type is required", "invalid_request_error", "policy_target_required")
		return
	}
	sim.ID = newID("dwpolsim")
	if err := s.db.InsertDataWorksPolicySimulation(r.Context(), sim); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "policy_simulation_failed")
		return
	}
	s.auditAdmin(r, "dataworks.policy.simulate", "", auditJSON(map[string]any{"simulation_id": sim.ID, "target_type": sim.TargetType, "decision": sim.Decision, "risk_score": sim.RiskScore}))
	writeJSON(w, http.StatusOK, map[string]any{"simulation": sim})
}

func evaluateDataWorksPolicy(workspaceID, targetType, targetID string, ctx map[string]any, createdBy string) store.DataWorksPolicySimulation {
	findings := []store.DataWorksPolicyFinding{}
	approvals := []store.DataWorksApprovalRequirement{}
	riskScore := 0
	blocked := false
	approvalRequired := false
	sensitivity := strings.ToLower(stringMapValue(ctx, "sensitivity"))
	externalSharing := boolMapValue(ctx, "external_sharing")
	pseudonymized := boolMapValue(ctx, "pseudonymized") || strings.Contains(sensitivity, "pseudonym") || strings.Contains(sensitivity, "aggregated")
	purpose := strings.TrimSpace(stringMapValue(ctx, "purpose"))
	aiUse := boolMapValue(ctx, "ai_use") || targetType == "agent"
	decisionImpact := strings.ToLower(stringMapValue(ctx, "decision_impact"))
	toolRisk := strings.ToLower(stringMapValue(ctx, "tool_risk"))
	personalCredit := strings.Contains(sensitivity, "personal_credit") || strings.Contains(sensitivity, "credit") || boolMapValue(ctx, "personal_credit")

	if personalCredit {
		riskScore += 30
		findings = append(findings, store.DataWorksPolicyFinding{
			Policy: "credit_data_classification", Severity: "high", Decision: "review",
			Reason: "The scope contains personal credit information.", Remediation: "Bind fields to an approved purpose and record data-owner approval.",
		})
		approvals = append(approvals, store.DataWorksApprovalRequirement{Role: "data_owner", Reason: "personal credit information", Required: true})
		approvalRequired = true
	}
	if externalSharing {
		riskScore += 20
		if personalCredit && !pseudonymized {
			blocked = true
			riskScore += 35
			findings = append(findings, store.DataWorksPolicyFinding{
				Policy: "external_sharing", Severity: "critical", Decision: "block",
				Reason: "Raw personal credit information cannot be externally shared.", Remediation: "Use aggregation, pseudonymization, field minimization, and an approved external-sharing pack.",
			})
		} else {
			approvalRequired = true
			findings = append(findings, store.DataWorksPolicyFinding{
				Policy: "external_sharing", Severity: "high", Decision: "approval_required",
				Reason: "External delivery requires legal and compliance review.", Remediation: "Attach purpose, retention, redistribution, masking, and recipient controls.",
			})
		}
		approvals = append(approvals,
			store.DataWorksApprovalRequirement{Role: "legal", Reason: "external sharing", Required: true},
			store.DataWorksApprovalRequirement{Role: "compliance", Reason: "external sharing", Required: true},
		)
	}
	if purpose == "" {
		riskScore += 15
		findings = append(findings, store.DataWorksPolicyFinding{
			Policy: "purpose_binding", Severity: "medium", Decision: "warn",
			Reason: "No explicit usage purpose was supplied.", Remediation: "Declare a contract purpose and map it to allowed fields and operations.",
		})
	}
	if aiUse && (decisionImpact == "high" || decisionImpact == "credit_decision") {
		riskScore += 20
		approvalRequired = true
		findings = append(findings, store.DataWorksPolicyFinding{
			Policy: "decision_impact", Severity: "high", Decision: "approval_required",
			Reason: "AI output may influence a credit or high-impact decision.", Remediation: "Add human review, explanation evidence, bias evaluation, and an adverse-use guard.",
		})
		approvals = append(approvals, store.DataWorksApprovalRequirement{Role: "model_risk", Reason: "high-impact AI use", Required: true})
	}
	if toolRisk == "high" || toolRisk == "critical" {
		riskScore += 20
		approvalRequired = true
		findings = append(findings, store.DataWorksPolicyFinding{
			Policy: "tool_governance", Severity: "high", Decision: "approval_required",
			Reason: "A high-risk tool is included in the execution plan.", Remediation: "Run in the sandbox and require human approval before live execution.",
		})
		approvals = append(approvals, store.DataWorksApprovalRequirement{Role: "security", Reason: "high-risk tool", Required: true})
	}
	if boolMapValue(ctx, "adverse_use") {
		blocked = true
		riskScore += 50
		findings = append(findings, store.DataWorksPolicyFinding{
			Policy: "adverse_use_guard", Severity: "critical", Decision: "block",
			Reason: "The declared use may enable discrimination, excessive inference, or out-of-purpose processing.", Remediation: "Redesign the use case and obtain independent compliance review.",
		})
	}
	if riskScore > 100 {
		riskScore = 100
	}
	decision := "allow"
	if blocked {
		decision = "block"
	} else if approvalRequired {
		decision = "approval_required"
	} else if len(findings) > 0 {
		decision = "allow_with_controls"
	}
	return store.DataWorksPolicySimulation{
		WorkspaceID: workspaceID, TargetType: targetType, TargetID: targetID, Context: ctx,
		Decision: decision, RiskScore: riskScore, Findings: findings, ApprovalMatrix: approvals,
		CreatedBy: createdBy, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func stringMapValue(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

func boolMapValue(m map[string]any, key string) bool {
	switch value := m[key].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(value, "true") || value == "1" || strings.EqualFold(value, "yes")
	case float64:
		return value != 0
	default:
		return false
	}
}

func (s *Server) handleDataWorksSynthetic(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method == http.MethodGet {
		datasets, err := s.db.ListSyntheticDatasets(r.Context(), r.URL.Query().Get("workspace_id"), intQuery(r, "limit", 50))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "synthetic_list_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"datasets": datasets})
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var in struct {
		WorkspaceID string                 `json:"workspace_id"`
		Name        string                 `json:"name"`
		Purpose     string                 `json:"purpose"`
		Strategy    string                 `json:"strategy"`
		Schema      []store.SyntheticField `json:"schema"`
		RowCount    int                    `json:"row_count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if in.Name == "" || len(in.Schema) == 0 {
		writeOpenAIError(w, http.StatusBadRequest, "name and schema are required", "invalid_request_error", "synthetic_schema_required")
		return
	}
	if in.RowCount <= 0 {
		in.RowCount = 10
	}
	if in.RowCount > 100 {
		in.RowCount = 100
	}
	if in.Strategy == "" {
		in.Strategy = "synthetic"
	}
	rows := make([]map[string]any, 0, in.RowCount)
	maskedFields := []string{}
	for i := 0; i < in.RowCount; i++ {
		row := map[string]any{}
		for _, field := range in.Schema {
			row[field.Name] = syntheticFieldValue(field, i)
			if field.Sensitivity != "" && field.Sensitivity != "public" && !platformContainsString(maskedFields, field.Name) {
				maskedFields = append(maskedFields, field.Name)
			}
		}
		rows = append(rows, row)
	}
	dataset := store.SyntheticDataset{
		ID: newID("dwsynth"), WorkspaceID: in.WorkspaceID, Name: in.Name, Purpose: in.Purpose,
		Strategy: in.Strategy, Schema: in.Schema, RowCount: in.RowCount, Sample: rows,
		SafetyReport: map[string]any{
			"real_person_records": 0, "direct_identifiers": 0, "masked_fields": maskedFields,
			"generated_only": true, "external_demo_safe": true,
		},
		Status: "ready", CreatedBy: adminID(r), CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.db.InsertSyntheticDataset(r.Context(), dataset); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "synthetic_generate_failed")
		return
	}
	s.auditAdmin(r, "dataworks.synthetic.generate", "", auditJSON(map[string]any{"dataset_id": dataset.ID, "rows": dataset.RowCount, "masked_fields": maskedFields}))
	writeJSON(w, http.StatusCreated, map[string]any{"dataset": dataset})
}

func syntheticFieldValue(field store.SyntheticField, index int) any {
	name := strings.ToLower(field.Name)
	typ := strings.ToLower(field.Type)
	sensitive := strings.ToLower(field.Sensitivity)
	if strings.Contains(name, "email") {
		return "user" + strconv.Itoa(index+1) + "@example.invalid"
	}
	if strings.Contains(name, "phone") || strings.Contains(name, "mobile") {
		return "010-****-" + fmt4(index+1)
	}
	if strings.Contains(sensitive, "personal") || strings.Contains(sensitive, "credit") || strings.Contains(sensitive, "identifier") {
		return "SYN-" + strings.ToUpper(strings.ReplaceAll(field.Name, " ", "_")) + "-" + fmt4(index+1)
	}
	switch typ {
	case "int", "integer", "number":
		return 1000 + index*17
	case "float", "decimal":
		return math.Round((0.25+float64(index)*0.037)*1000) / 1000
	case "bool", "boolean":
		return index%2 == 0
	case "date":
		return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, index).Format("2006-01-02")
	case "datetime", "timestamp":
		return time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC).Add(time.Duration(index) * time.Hour).Format(time.RFC3339)
	default:
		return "synthetic_" + strings.ToLower(strings.ReplaceAll(field.Name, " ", "_")) + "_" + strconv.Itoa(index+1)
	}
}

func fmt4(value int) string {
	return fmt.Sprintf("%04d", value)
}

func platformContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (s *Server) handleDataWorksProductUnitEconomics(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	usage, _ := s.db.ListUsageMetering(r.Context(), product.ProductKey)
	observedCalls := 0
	for _, row := range usage {
		observedCalls += row.TotalCalls
	}
	expectedCalls := intQuery(r, "expected_calls", observedCalls)
	if expectedCalls <= 0 {
		expectedCalls = 10000
	}
	cost, hasCost, err := s.db.GetProductCost(r.Context(), product.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "unit_economics_cost_failed")
		return
	}
	unitPrice := platformFloatQuery(r, "unit_price", 0)
	if unitPrice <= 0 && hasCost && cost.ExpectedRevenue > 0 {
		unitPrice = cost.ExpectedRevenue / float64(expectedCalls)
	}
	if unitPrice <= 0 {
		unitPrice = 100
	}
	queryPerCall := platformFloatQuery(r, "query_cost_per_call", 0)
	llmPerCall := platformFloatQuery(r, "llm_cost_per_call", 0)
	processingPerCall := platformFloatQuery(r, "processing_cost_per_call", 0)
	opsCost := platformFloatQuery(r, "ops_cost", 0)
	if hasCost {
		if queryPerCall == 0 {
			queryPerCall = cost.QueryCost / float64(expectedCalls)
		}
		if llmPerCall == 0 {
			llmPerCall = cost.LLMCost / float64(expectedCalls)
		}
		if processingPerCall == 0 {
			processingPerCall = cost.DataProcessingCost / float64(expectedCalls)
		}
		if opsCost == 0 {
			opsCost = cost.OpsCost
		}
	}
	queryCost := queryPerCall * float64(expectedCalls)
	llmCost := llmPerCall * float64(expectedCalls)
	processingCost := processingPerCall * float64(expectedCalls)
	revenue := unitPrice * float64(expectedCalls)
	totalCost := queryCost + llmCost + processingCost + opsCost
	margin := revenue - totalCost
	marginRate := 0.0
	if revenue > 0 {
		marginRate = margin / revenue
	}
	contribution := unitPrice - queryPerCall - llmPerCall - processingPerCall
	breakEven := 0
	if contribution > 0 {
		breakEven = int(math.Ceil(opsCost / contribution))
	}
	economics := store.DataWorksUnitEconomics{
		ID:         platformStableID("dwecon", product.ProductKey+"|"+firstNonEmpty(r.URL.Query().Get("scenario"), "base")+"|"+r.URL.Query().Get("customer_segment")),
		ProductKey: product.ProductKey, ScenarioKey: firstNonEmpty(r.URL.Query().Get("scenario"), "base"),
		CustomerSegment: r.URL.Query().Get("customer_segment"), ExpectedCalls: expectedCalls,
		UnitPrice: unitPrice, ExpectedRevenue: revenue, QueryCost: queryCost, LLMCost: llmCost,
		DataProcessingCost: processingCost, OpsCost: opsCost, TotalCost: totalCost, Margin: margin,
		MarginRate: marginRate, BreakEvenCalls: breakEven, Currency: "KRW",
	}
	if err := s.db.UpsertDataWorksUnitEconomics(r.Context(), economics); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "unit_economics_save_failed")
		return
	}
	history, _ := s.db.ListDataWorksUnitEconomics(r.Context(), product.ProductKey)
	writeJSON(w, http.StatusOK, map[string]any{
		"unit_economics": economics, "history": history,
		"assumptions": map[string]any{"observed_calls": observedCalls, "query_cost_per_call": queryPerCall, "llm_cost_per_call": llmPerCall, "processing_cost_per_call": processingPerCall},
	})
}

func platformFloatQuery(r *http.Request, key string, fallback float64) float64 {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func (s *Server) handleDataWorksMarketplaceItems(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	if err := s.syncDataWorksMarketplace(r); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "marketplace_sync_failed")
		return
	}
	items, err := s.db.ListDataWorksMarketplaceItems(r.Context(), r.URL.Query().Get("q"), r.URL.Query().Get("item_type"), r.URL.Query().Get("workspace_id"))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "marketplace_list_failed")
		return
	}
	types := map[string]int{}
	for _, item := range items {
		types[item.ItemType]++
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "facets": types, "count": len(items)})
}

func (s *Server) handleDataWorksMarketplaceSubscribe(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var in struct {
		ItemID  string `json:"item_id"`
		ItemKey string `json:"item_key"`
		Purpose string `json:"purpose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	item, ok, err := s.db.GetDataWorksMarketplaceItem(r.Context(), firstNonEmpty(in.ItemID, in.ItemKey))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "marketplace_item_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "marketplace item not found", "invalid_request_error", "not_found")
		return
	}
	sub := store.DataWorksMarketplaceSubscription{
		ID: newID("dwsub"), UserID: adminID(r), ItemID: item.ID, ItemKey: item.ItemKey,
		ItemType: item.ItemType, Status: "pending", Purpose: strings.TrimSpace(in.Purpose),
	}
	if err := s.db.InsertDataWorksMarketplaceSubscription(r.Context(), sub); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "marketplace_subscribe_failed")
		return
	}
	approval := store.DataWorksApprovalTask{
		ID: newID("dwappr"), TargetType: "marketplace_subscription", TargetID: sub.ID,
		Step: "purpose_and_owner_review", Status: "pending", RequestedBy: adminID(r),
	}
	_ = s.db.InsertDataWorksApprovalTask(r.Context(), approval)
	s.auditAdmin(r, "dataworks.marketplace.subscribe", "", auditJSON(map[string]any{"subscription_id": sub.ID, "item_key": item.ItemKey, "purpose": sub.Purpose}))
	writeJSON(w, http.StatusCreated, map[string]any{"subscription": sub, "approval_task": approval})
}

func (s *Server) syncDataWorksMarketplace(r *http.Request) error {
	ctx := r.Context()
	products, err := s.db.ListDataProducts(ctx, "")
	if err != nil {
		return err
	}
	for _, product := range products {
		if product.Status != "published" && product.Status != "approved" {
			continue
		}
		risk := "low"
		if product.RiskScore >= 70 {
			risk = "high"
		} else if product.RiskScore >= 40 {
			risk = "medium"
		}
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "product:"+product.ProductKey), ItemKey: "product:" + product.ProductKey,
			ItemType: "product", Name: firstNonEmpty(product.NameKO, product.NameEN, product.ProductKey),
			Description: product.Description, Owner: product.Owner, Status: "published", RiskLevel: risk,
			SourceRef: product.ProductKey, Version: product.Version,
		})
	}
	flows, _ := s.db.ListDataWorksFlows(ctx, "", "")
	for _, flow := range flows {
		if flow.Status != "approved" && flow.Status != "production" {
			continue
		}
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "flow:"+flow.FlowKey), ItemKey: "flow:" + flow.FlowKey,
			WorkspaceID: flow.WorkspaceID, ItemType: "flow", Name: flow.Name, Description: flow.Description,
			Owner: flow.Owner, Status: "published", RiskLevel: "medium", SourceRef: flow.ID, Version: flow.Version,
		})
	}
	agents, _ := s.db.ListDataWorksAgents(ctx, "", "")
	for _, agent := range agents {
		if agent.Status != "approved" && agent.Status != "active" && agent.Status != "production" {
			continue
		}
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "agent:"+agent.AgentKey), ItemKey: "agent:" + agent.AgentKey,
			WorkspaceID: agent.WorkspaceID, ItemType: "agent", Name: agent.Name, Description: agent.Purpose,
			Owner: agent.Owner, Status: "published", RiskLevel: agent.RiskLevel, SourceRef: agent.ID, Version: agent.Version,
		})
	}
	tools, _ := s.db.ListDataWorksTools(ctx, "", "", true)
	for _, tool := range tools {
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "tool:"+tool.ToolKey), ItemKey: "tool:" + tool.ToolKey,
			WorkspaceID: tool.WorkspaceID, ItemType: "tool", Name: tool.Name, Description: tool.Description,
			Owner: tool.Owner, Status: "published", RiskLevel: tool.RiskLevel, SourceRef: tool.ID, Version: 1,
		})
	}
	templates, _ := s.db.ListPromptTemplates(ctx, true)
	for _, template := range templates {
		if template.Status != "standard" && template.Status != "approved" {
			continue
		}
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "prompt:"+template.ID), ItemKey: "prompt:" + template.ID,
			ItemType: "prompt", Name: template.Name, Description: template.Description, Status: "published",
			RiskLevel: "low", Tags: template.Tags, SourceRef: template.ID, Version: 1,
		})
	}
	reports, _ := s.db.ListText2SQLSavedReports(ctx)
	for _, report := range reports {
		if report.ApprovalStatus != "approved" && report.Visibility != "team" {
			continue
		}
		_ = s.db.UpsertDataWorksMarketplaceItem(ctx, store.DataWorksMarketplaceItem{
			ID: platformStableID("dwitem", "report:"+report.ID), ItemKey: "report:" + report.ID,
			ItemType: "report", Name: report.Name, Description: report.Question, Owner: report.CreatedBy,
			Status: "published", RiskLevel: "low", Tags: []string{report.SchemaName, report.Kind}, SourceRef: report.ID, Version: 1,
		})
	}
	return nil
}

func (s *Server) handleDataWorksAgentOps(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	agents, err := s.db.ListDataWorksAgents(r.Context(), r.URL.Query().Get("workspace_id"), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agentops_agents_failed")
		return
	}
	sessions, err := s.db.ListDataWorksAgentSessions(r.Context(), "", intQuery(r, "limit", 1000))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "agentops_sessions_failed")
		return
	}
	byAgent := map[string][]store.DataWorksAgentSession{}
	for _, session := range sessions {
		byAgent[session.AgentID] = append(byAgent[session.AgentID], session)
	}
	rows := []map[string]any{}
	totals := map[string]any{"runs": len(sessions), "success": 0, "failed": 0, "blocked": 0, "pending_approval": 0, "cost": 0.0}
	for _, agent := range agents {
		items := byAgent[agent.ID]
		success, failed, blocked, pending := 0, 0, 0, 0
		latency, cost := int64(0), 0.0
		for _, session := range items {
			latency += session.LatencyMS
			cost += session.TotalCost
			switch session.Status {
			case "succeeded":
				success++
			case "blocked":
				blocked++
			case "pending_approval":
				pending++
			default:
				failed++
			}
		}
		rate := 0.0
		avgLatency := int64(0)
		if len(items) > 0 {
			rate = float64(success) / float64(len(items))
			avgLatency = latency / int64(len(items))
		}
		recommendation := "추가 조치가 필요하지 않습니다."
		if blocked > 0 {
			recommendation = "도구 허용 목록과 차단된 정책 판정을 검토하세요."
		} else if pending > 0 {
			recommendation = "승인 담당자를 지정하고 승인 처리 SLA를 정의하세요."
		} else if rate < 0.9 && len(items) > 0 {
			recommendation = "회귀 테스트를 실행하고 최근 실패 트레이스를 확인하세요."
		}
		rows = append(rows, map[string]any{
			"agent": agent, "runs": len(items), "success_rate": rate, "failed": failed,
			"blocked": blocked, "pending_approval": pending, "avg_latency_ms": avgLatency,
			"cost": cost, "recommendation": recommendation,
		})
		totals["success"] = totals["success"].(int) + success
		totals["failed"] = totals["failed"].(int) + failed
		totals["blocked"] = totals["blocked"].(int) + blocked
		totals["pending_approval"] = totals["pending_approval"].(int) + pending
		totals["cost"] = totals["cost"].(float64) + cost
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": rows, "totals": totals})
}

func (s *Server) handleDataWorksExecutiveSimulator(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var in struct {
		CustomerSegment  string   `json:"customer_segment"`
		ProductKeys      []string `json:"product_keys"`
		Customers        int      `json:"customers"`
		CallsPerCustomer int      `json:"calls_per_customer"`
		UnitPrice        float64  `json:"unit_price"`
		POCSuccessRate   float64  `json:"poc_success_rate"`
		RiskMitigation   float64  `json:"risk_mitigation"`
		DiscountRate     float64  `json:"discount_rate"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if in.Customers <= 0 {
		in.Customers = 10
	}
	if in.CallsPerCustomer <= 0 {
		in.CallsPerCustomer = 10000
	}
	if in.UnitPrice <= 0 {
		in.UnitPrice = 100
	}
	if in.POCSuccessRate <= 0 || in.POCSuccessRate > 1 {
		in.POCSuccessRate = 0.6
	}
	if len(in.ProductKeys) == 0 {
		in.ProductKeys = []string{"portfolio"}
	}
	baseRisk := 0.0
	knownProducts := 0
	for _, key := range in.ProductKeys {
		if product, ok, _ := s.db.GetDataProduct(r.Context(), key); ok {
			baseRisk += float64(product.RiskScore)
			knownProducts++
		}
	}
	if knownProducts > 0 {
		baseRisk /= float64(knownProducts)
	} else {
		baseRisk = 40
	}
	if in.RiskMitigation < 0 {
		in.RiskMitigation = 0
	}
	if in.RiskMitigation > 1 {
		in.RiskMitigation = 1
	}
	baseRevenue := float64(in.Customers*in.CallsPerCustomer*len(in.ProductKeys)) * in.UnitPrice * (1 - in.DiscountRate)
	scenarios := []map[string]any{}
	for _, scenario := range []struct {
		name       string
		successMul float64
		volumeMul  float64
		costRate   float64
	}{
		{"conservative", 0.75, 0.8, 0.45},
		{"base", 1.0, 1.0, 0.35},
		{"growth", 1.15, 1.35, 0.32},
	} {
		success := math.Min(1, in.POCSuccessRate*scenario.successMul)
		revenue := baseRevenue * scenario.volumeMul * success
		cost := revenue * scenario.costRate
		risk := math.Max(0, baseRisk*(1-in.RiskMitigation)+(1-success)*20)
		scenarios = append(scenarios, map[string]any{
			"scenario": scenario.name, "expected_revenue": revenue, "expected_cost": cost,
			"expected_margin": revenue - cost, "margin_rate": 1 - scenario.costRate,
			"poc_success_rate": success, "risk_score": math.Round(risk*10) / 10,
		})
	}
	result := map[string]any{
		"customer_segment": in.CustomerSegment, "product_keys": in.ProductKeys,
		"assumptions": map[string]any{"customers": in.Customers, "calls_per_customer": in.CallsPerCustomer, "unit_price": in.UnitPrice, "discount_rate": in.DiscountRate, "risk_mitigation": in.RiskMitigation},
		"scenarios":   scenarios,
	}
	s.auditAdmin(r, "dataworks.executive.simulate", "", auditJSON(map[string]any{"customer_segment": in.CustomerSegment, "products": len(in.ProductKeys)}))
	writeJSON(w, http.StatusOK, map[string]any{"simulation": result})
}

func (s *Server) handleDataWorksPlatformOverview(w http.ResponseWriter, r *http.Request) {
	if !s.requireDataWorksAdmin(w, r) {
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	_ = s.syncDataWorksMetadata(r.Context())
	workspaces, _ := s.db.ListDataWorksWorkspaces(r.Context(), "")
	entities, _ := s.db.SearchMetadataEntities(r.Context(), "", "", "", 500)
	flows, _ := s.db.ListDataWorksFlows(r.Context(), "", "")
	agents, _ := s.db.ListDataWorksAgents(r.Context(), "", "")
	tools, _ := s.db.ListDataWorksTools(r.Context(), "", "", false)
	sessions, _ := s.db.ListDataWorksAgentSessions(r.Context(), "", 1000)
	flowRuns, _ := s.db.ListDataWorksFlowRuns(r.Context(), "", 1000)
	policyRuns, _ := s.db.ListDataWorksPolicySimulations(r.Context(), "", "", 1000)
	blocked := 0
	for _, sim := range policyRuns {
		if sim.Decision == "block" {
			blocked++
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"kpis": map[string]any{
			"workspaces": len(workspaces), "metadata_entities": len(entities), "flows": len(flows),
			"flow_runs": len(flowRuns), "agents": len(agents), "agent_runs": len(sessions),
			"tools": len(tools), "policy_blocks": blocked,
		},
		"workspaces": workspaces, "recent_agent_runs": firstAgentSessions(sessions, 6), "recent_flow_runs": firstFlowRuns(flowRuns, 6),
	})
}

func firstAgentSessions(items []store.DataWorksAgentSession, limit int) []store.DataWorksAgentSession {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func firstFlowRuns(items []store.DataWorksFlowRun, limit int) []store.DataWorksFlowRun {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}
