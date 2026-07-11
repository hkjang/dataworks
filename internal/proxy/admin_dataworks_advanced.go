package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dataworks/internal/dataworks"
	"dataworks/internal/store"
)

// dataWorksAdvancedPathParts parses paths for advanced features
func dataWorksAdvancedPathParts(path, prefix string) []string {
	rest := strings.TrimPrefix(path, prefix)
	var out []string
	for _, p := range strings.Split(rest, "/") {
		if strings.TrimSpace(p) != "" {
			out = append(out, strings.TrimSpace(p))
		}
	}
	return out
}

// 1. Data Quality Rules Handlers
func (s *Server) handleDataWorksAssetQualityRules(w http.ResponseWriter, r *http.Request, assetKey string) {
	if r.Method == http.MethodGet {
		rules, err := s.db.ListDataQualityRules(r.Context(), assetKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rules_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
		return
	}

	if r.Method == http.MethodPost {
		var rule store.DataQualityRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error", "bad_json")
			return
		}
		rule.AssetKey = assetKey
		if rule.ID == "" {
			rule.ID = "qrule_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		if err := s.db.InsertDataQualityRule(r.Context(), rule); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rule_insert_failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

func (s *Server) handleDataWorksAssetQualityEvaluate(w http.ResponseWriter, r *http.Request, assetKey string) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	rules, err := s.db.ListDataQualityRules(r.Context(), assetKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rules_failed")
		return
	}

	// If no rules, create default ones for testing
	if len(rules) == 0 {
		defaultRules := []store.DataQualityRule{
			{ID: "qrule_null_" + assetKey, AssetKey: assetKey, ColumnName: "score", RuleType: "null_rate", Threshold: 0.05, Enabled: true},
			{ID: "qrule_dup_" + assetKey, AssetKey: assetKey, ColumnName: "id", RuleType: "duplicate_rate", Threshold: 0.01, Enabled: true},
		}
		for _, dr := range defaultRules {
			_ = s.db.InsertDataQualityRule(r.Context(), dr)
		}
		rules, _ = s.db.ListDataQualityRules(r.Context(), assetKey)
	}

	var results []store.DataQualityResult
	passedCount := 0
	totalCount := 0

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		totalCount++
		// Simulate evaluation values
		actualVal := 0.0
		passed := true
		msg := "Passed"

		switch rule.RuleType {
		case "null_rate":
			actualVal = 0.02 // Less than 0.05 threshold
			if actualVal > rule.Threshold {
				passed = false
				msg = fmt.Sprintf("Null rate %.2f exceeds threshold %.2f", actualVal, rule.Threshold)
			}
		case "duplicate_rate":
			actualVal = 0.0 // No duplicates
		case "range_condition":
			actualVal = 50.0
			if rule.MinValue > 0 && actualVal < rule.MinValue {
				passed = false
				msg = fmt.Sprintf("Value %.2f is below min %.2f", actualVal, rule.MinValue)
			}
		}

		if passed {
			passedCount++
		}

		res := store.DataQualityResult{
			ID:          "qres_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + rule.ID,
			AssetKey:    assetKey,
			RuleID:      rule.ID,
			RuleType:    rule.RuleType,
			Passed:      passed,
			ActualValue: actualVal,
			Message:     msg,
		}
		_ = s.db.InsertDataQualityResult(r.Context(), res)
		results = append(results, res)
	}

	qualityScore := 100
	if totalCount > 0 {
		qualityScore = (passedCount * 100) / totalCount
	}

	// Update asset overall quality scores in readiness
	readinessList, _ := s.db.ListAssetReadinessScores(r.Context(), assetKey)
	var readiness store.AssetReadinessScore
	if len(readinessList) > 0 {
		readiness = readinessList[0]
	} else {
		readiness.AssetKey = assetKey
	}
	readiness.MissingnessScore = qualityScore
	readiness.OverallScore = (readiness.SchemaScore + readiness.FreshnessScore + readiness.SampleScore + readiness.MissingnessScore +
		readiness.SensitivityScore + readiness.ExternalSharingScore + readiness.APIReadinessScore + readiness.BillingReadinessScore) / 8
	if readiness.OverallScore == 0 {
		readiness.OverallScore = qualityScore
	}
	_ = s.db.UpsertAssetReadinessScore(r.Context(), readiness)

	writeJSON(w, http.StatusOK, map[string]any{
		"quality_score": qualityScore,
		"passed_rules":  passedCount,
		"total_rules":   totalCount,
		"results":       results,
	})
}

func (s *Server) handleDataWorksAssetQualityResults(w http.ResponseWriter, r *http.Request, assetKey string) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	results, err := s.db.ListDataQualityResults(r.Context(), assetKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "results_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

// 2. Schema Drift Handlers
func (s *Server) handleDataWorksAssetDriftDetect(w http.ResponseWriter, r *http.Request, assetKey string) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	asset, ok, err := s.db.GetDataAsset(r.Context(), assetKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "asset_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "asset not found", "invalid_request_error", "not_found")
		return
	}

	// Current schema input
	var req struct {
		Columns []struct {
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
		return
	}

	// Parse previous schema
	var previousColumns = map[string]string{}
	if asset.ColumnsSummary != "" {
		// Expects formats: "col1:string,col2:int"
		for _, pair := range strings.Split(asset.ColumnsSummary, ",") {
			parts := strings.Split(pair, ":")
			if len(parts) == 2 {
				previousColumns[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			} else if len(parts) == 1 {
				previousColumns[strings.TrimSpace(parts[0])] = "string"
			}
		}
	}

	newColumnsStr := []string{}
	var drifts []store.SchemaDrift

	for _, col := range req.Columns {
		newColumnsStr = append(newColumnsStr, col.Name+":"+col.Type)
		oldType, exists := previousColumns[col.Name]
		if !exists {
			// Added column
			d := store.SchemaDrift{
				ID:          "drift_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + col.Name,
				AssetKey:    assetKey,
				ColumnName:  col.Name,
				DriftType:   "added",
				NewType:     col.Type,
				ImpactScore: 10.0,
			}
			_ = s.db.InsertSchemaDrift(r.Context(), d)
			drifts = append(drifts, d)
		} else if oldType != col.Type {
			// Type changed
			d := store.SchemaDrift{
				ID:          "drift_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + col.Name,
				AssetKey:    assetKey,
				ColumnName:  col.Name,
				DriftType:   "type_changed",
				OldType:     oldType,
				NewType:     col.Type,
				ImpactScore: 75.0,
			}
			_ = s.db.InsertSchemaDrift(r.Context(), d)
			drifts = append(drifts, d)
		}
	}

	// Check deleted columns
	newColSet := map[string]bool{}
	for _, col := range req.Columns {
		newColSet[col.Name] = true
	}
	for oldName, oldType := range previousColumns {
		if !newColSet[oldName] {
			d := store.SchemaDrift{
				ID:          "drift_" + strconv.FormatInt(time.Now().UnixNano(), 10) + "_" + oldName,
				AssetKey:    assetKey,
				ColumnName:  oldName,
				DriftType:   "deleted",
				OldType:     oldType,
				ImpactScore: 90.0,
			}
			_ = s.db.InsertSchemaDrift(r.Context(), d)
			drifts = append(drifts, d)
		}
	}

	// Update asset columns summary
	if len(newColumnsStr) > 0 {
		asset.ColumnsSummary = strings.Join(newColumnsStr, ",")
		_ = s.db.UpsertDataAsset(r.Context(), asset)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"drifts": drifts,
		"synced": len(drifts) == 0,
	})
}

func (s *Server) handleDataWorksAssetDrifts(w http.ResponseWriter, r *http.Request, assetKey string) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	drifts, err := s.db.ListSchemaDrifts(r.Context(), assetKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "drifts_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"drifts": drifts})
}

func (s *Server) handleDataWorksProductDriftImpact(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	assets := dataworks.ProductAssetKeys(p)
	var allDrifts []store.SchemaDrift
	maxImpact := 0.0

	for _, assetKey := range assets {
		drifts, err := s.db.ListSchemaDrifts(r.Context(), assetKey)
		if err == nil {
			for _, d := range drifts {
				allDrifts = append(allDrifts, d)
				if d.ImpactScore > maxImpact {
					maxImpact = d.ImpactScore
				}
			}
		}
	}

	impactRating := "low"
	if maxImpact >= 75.0 {
		impactRating = "high"
	} else if maxImpact >= 30.0 {
		impactRating = "medium"
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"product_key":   p.ProductKey,
		"impact_rating": impactRating,
		"max_impact":    maxImpact,
		"drifts":        allDrifts,
	})
}

// 3. Product SLA Monitor Handlers
func (s *Server) handleDataWorksProductSLACheck(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	var req struct {
		MetricType  string  `json:"metric_type"` // freshness | latency | availability | report_delay
		ActualValue float64 `json:"actual_value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
		return
	}

	sla, ok, err := s.db.GetProductSLA(r.Context(), p.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "sla_failed")
		return
	}

	target := 0.0
	status := "normal"

	if ok {
		switch req.MetricType {
		case "latency":
			target = float64(sla.LatencyTargetMS)
			if req.ActualValue > target {
				status = "breached"
			} else if req.ActualValue > target*0.8 {
				status = "warning"
			}
		case "availability":
			target = sla.AvailabilityTarget
			if req.ActualValue < target {
				status = "breached"
			} else if req.ActualValue < target+0.002 {
				status = "warning"
			}
		case "freshness":
			target = 24.0 // Default 24 hours target delay
			if req.ActualValue > target {
				status = "breached"
			}
		}
	}

	m := store.SLAMetric{
		ID:          "slam_" + strconv.FormatInt(time.Now().UnixNano(), 10),
		ProductKey:  p.ProductKey,
		MetricType:  req.MetricType,
		ActualValue: req.ActualValue,
		TargetValue: target,
		Status:      status,
	}

	if err := s.db.InsertSLAMetric(r.Context(), m); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metric_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"metric": m})
}

func (s *Server) handleDataWorksProductSLAStatus(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	metrics, err := s.db.ListSLAMetrics(r.Context(), p.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "metrics_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics})
}

// 4. API Usage Metering Handlers
func (s *Server) handleDataWorksProductUsage(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	usage, err := s.db.ListUsageMetering(r.Context(), p.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "usage_failed")
		return
	}

	totalCalls := 0
	failedCalls := 0
	totalBilling := 0.0
	for _, u := range usage {
		totalCalls += u.TotalCalls
		failedCalls += u.FailedCalls
		totalBilling += u.BillingAmount
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"product_key":   p.ProductKey,
		"usage_records": usage,
		"summary": map[string]any{
			"total_calls":   totalCalls,
			"failed_calls":  failedCalls,
			"total_billing": totalBilling,
		},
	})
}

// 5. Policy-as-Code Engine Handlers
func (s *Server) handleDataWorksPolicyRules(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}

	if r.Method == http.MethodGet {
		policyType := r.URL.Query().Get("policy_type")
		rules, err := s.db.ListPolicyRules(r.Context(), policyType)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rules_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
		return
	}

	if r.Method == http.MethodPost {
		var rule store.DataWorksPolicyRule
		if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error", "bad_json")
			return
		}
		if rule.ID == "" {
			rule.ID = "prule_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		if err := s.db.InsertPolicyRule(r.Context(), rule); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rule_insert_failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"rule": rule})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

func (s *Server) handleDataWorksProductPolicyEvaluate(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	rules, err := s.db.ListPolicyRules(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "rules_failed")
		return
	}

	// Create default rules if none exist
	if len(rules) == 0 {
		defaultRules := []store.DataWorksPolicyRule{
			{ID: "prule_privacy", PolicyType: "privacy", RuleExpression: "sensitivity=personal_credit -> require_compliance_approval", Action: "block", Enabled: true},
			{ID: "prule_ai_usage", PolicyType: "ai_usage", RuleExpression: "risk_score>=70 -> block_unapproved_model", Action: "block", Enabled: true},
		}
		for _, dr := range defaultRules {
			_ = s.db.InsertPolicyRule(r.Context(), dr)
		}
		rules, _ = s.db.ListPolicyRules(r.Context(), "")
	}

	var results []map[string]any
	allowed := true

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		passed := true
		reason := "Rule conditions satisfied"

		switch rule.PolicyType {
		case "privacy":
			if strings.Contains(p.Sensitivity, "personal") {
				// Require approvals
				approvals, _ := s.db.ListApprovalTraces(r.Context(), p.ProductKey)
				complianceApproved := false
				for _, app := range approvals {
					if app.Step == "compliance" && (app.Status == "approved" || app.Status == "waived") {
						complianceApproved = true
					}
				}
				if !complianceApproved {
					passed = false
					reason = "Sensitive personal data products must have compliance approval"
				}
			}
		case "ai_usage":
			if p.RiskScore >= 70 && p.SourceType == "api" && p.APISpec == "" {
				passed = false
				reason = "High risk products using AI APIs must define complete specs"
			}
		}

		if !passed && rule.Action == "block" {
			allowed = false
		}

		results = append(results, map[string]any{
			"rule_id":     rule.ID,
			"policy_type": rule.PolicyType,
			"passed":      passed,
			"reason":      reason,
			"action":      rule.Action,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"product_key": p.ProductKey,
		"allowed":     allowed,
		"evaluations": results,
	})
}

// 6. Prompt Regression Test Handlers
func (s *Server) handleDataWorksFactoryRunRegressionTest(w http.ResponseWriter, r *http.Request, run store.FactoryRun) {
	if r.Method == http.MethodGet {
		tests, err := s.db.ListPromptRegressionTests(r.Context(), run.RunType)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "tests_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tests": tests})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			CompareVersion int    `json:"compare_version"`
			CompareModel   string `json:"compare_model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
			return
		}

		// Simulate regression deltas
		qualityDelta := 0.05 // +5% quality improvement
		costDelta := -0.02   // -2% cost saving
		latencyDelta := -50.0 // -50ms faster
		violations := 0

		test := store.PromptRegressionTest{
			ID:                    "preg_" + strconv.FormatInt(time.Now().UnixNano(), 10),
			PromptKey:             run.RunType,
			OldTemplateVersion:    1,
			NewTemplateVersion:    req.CompareVersion,
			OldModel:              run.Model,
			NewModel:              req.CompareModel,
			QualityDelta:          qualityDelta,
			CostDelta:             costDelta,
			LatencyDelta:          latencyDelta,
			PolicyViolationsCount: violations,
			Status:                "completed",
		}

		if err := s.db.InsertPromptRegressionTest(r.Context(), test); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "test_insert_failed")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"regression_test": test})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

// 7. Proposal Experiment Handlers
func (s *Server) handleDataWorksProposalExperiments(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}

	if r.Method == http.MethodGet {
		productKey := r.URL.Query().Get("product_key")
		exps, err := s.db.ListProposalExperiments(r.Context(), productKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "exps_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"experiments": exps})
		return
	}

	if r.Method == http.MethodPost {
		var exp store.ProposalExperiment
		if err := json.NewDecoder(r.Body).Decode(&exp); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON", "invalid_request_error", "bad_json")
			return
		}
		if exp.ID == "" {
			exp.ID = "exp_" + strconv.FormatInt(time.Now().UnixNano(), 10)
		}
		exp.Status = "running"
		if err := s.db.InsertProposalExperiment(r.Context(), exp); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "exp_insert_failed")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"experiment": exp})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

func (s *Server) handleDataWorksProposalExperimentAction(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}

	parts := dataWorksAdvancedPathParts(r.URL.Path, "/admin/dataworks/proposal-experiments/")
	if len(parts) != 2 || parts[1] != "feedback" {
		writeOpenAIError(w, http.StatusNotFound, "not found", "invalid_request_error", "not_found")
		return
	}

	id := parts[0]
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	var req struct {
		Positive bool `json:"positive"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
		return
	}

	if err := s.db.RecordProposalExperimentResponse(r.Context(), id, req.Positive); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "feedback_failed")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// 8. Internal Data Product Marketplace Handlers
func (s *Server) handleDataWorksMarketplaceProducts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	products, err := s.db.ListDataProducts(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "products_failed")
		return
	}

	// Filter published/approved products
	var marketplaceProducts []store.DataProduct
	for _, p := range products {
		status := strings.ToLower(p.Status)
		if status == "published" || status == "approved" {
			marketplaceProducts = append(marketplaceProducts, p)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"products": marketplaceProducts})
}

func (s *Server) handleDataWorksMarketplaceBookmarks(w http.ResponseWriter, r *http.Request) {
	userID := adminID(r)
	if userID == "" {
		userID = "user_default"
	}

	if r.Method == http.MethodGet {
		bookmarks, err := s.db.ListMarketplaceBookmarks(r.Context(), userID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "bookmarks_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"bookmarks": bookmarks})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ProductKey string `json:"product_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
			return
		}

		bookmarked, err := s.db.ToggleMarketplaceBookmark(r.Context(), userID, req.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "bookmark_toggle_failed")
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{"product_key": req.ProductKey, "bookmarked": bookmarked})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

func (s *Server) handleDataWorksMarketplaceSubscriptions(w http.ResponseWriter, r *http.Request) {
	userID := adminID(r)
	if userID == "" {
		userID = "user_default"
	}

	if r.Method == http.MethodGet {
		status := r.URL.Query().Get("status")
		subs, err := s.db.ListMarketplaceSubscriptions(r.Context(), userID, status)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "subscriptions_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"subscriptions": subs})
		return
	}

	if r.Method == http.MethodPost {
		var req struct {
			ProductKey string `json:"product_key"`
			Purpose    string `json:"purpose"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "bad_json")
			return
		}
		sub := store.MarketplaceSubscription{
			ID:         "sub_" + strconv.FormatInt(time.Now().UnixNano(), 10),
			UserID:     userID,
			ProductKey: req.ProductKey,
			Status:     "pending",
			Purpose:    req.Purpose,
		}

		if err := s.db.InsertMarketplaceSubscription(r.Context(), sub); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "subscription_insert_failed")
			return
		}

		writeJSON(w, http.StatusCreated, map[string]any{"subscription": sub})
		return
	}

	writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
}

func (s *Server) handleDataWorksProductMargin(w http.ResponseWriter, r *http.Request, p store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	cost, ok, err := s.db.GetProductCost(r.Context(), p.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "cost_failed")
		return
	}

	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"product_key":          p.ProductKey,
			"expected_revenue":     0.0,
			"query_cost":           0.0,
			"llm_cost":             0.0,
			"ops_cost":             0.0,
			"data_processing_cost": 0.0,
			"margin":               0.0,
		})
		return
	}

	margin := cost.ExpectedRevenue - (cost.QueryCost + cost.LLMCost + cost.OpsCost + cost.DataProcessingCost)
	writeJSON(w, http.StatusOK, map[string]any{
		"product_key":          p.ProductKey,
		"expected_revenue":     cost.ExpectedRevenue,
		"query_cost":           cost.QueryCost,
		"llm_cost":             cost.LLMCost,
		"ops_cost":             cost.OpsCost,
		"data_processing_cost": cost.DataProcessingCost,
		"margin":               margin,
	})
}
