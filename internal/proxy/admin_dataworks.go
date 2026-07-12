package proxy

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	dw "dataworks/internal/dataworks"
	"dataworks/internal/store"
)

// handleDataWorksHome returns the Data Works dashboard KPIs.
func (s *Server) handleDataWorksHome(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}

	// Aggregate dashboard KPIs from existing factory dashboard + dw tables
	dash, _ := s.db.FactoryDashboard(r.Context())
	assets, _ := s.db.ListDataAssets(r.Context())
	products, _ := s.db.ListDataProducts(r.Context(), "")

	// Count products by status
	var published, review, riskHigh int
	for _, p := range products {
		switch p.Status {
		case "published":
			published++
		case "review", "risk_review":
			review++
		}
		if p.RiskScore >= 70 {
			riskHigh++
		}
	}

	// Top 10 products by revenue score
	top10 := products
	if len(top10) > 10 {
		top10 = top10[:10]
	}
	topProducts := make([]map[string]any, 0, len(top10))
	for _, p := range top10 {
		topProducts = append(topProducts, map[string]any{
			"product_key": p.ProductKey, "name": p.NameKO,
			"revenue_score": p.RevenueScore, "risk_score": p.RiskScore, "status": p.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dashboard": map[string]any{
			"total_assets":       len(assets),
			"total_products":     len(products),
			"published_products": published,
			"review_pending":     review,
			"high_risk":          riskHigh,
			"poc_pending":        dash.PendingPOCPlans,
			"ideas_total":        dash.IdeasTotal,
			"avg_revenue_score":  dash.AverageRevenue,
		},
		"top_products": topProducts,
	})
}

// handleDataWorksAssets manages internal data assets.
func (s *Server) handleDataWorksAssets(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		assets, err := s.db.ListDataAssets(r.Context())
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "assets_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"assets": assets})
	case http.MethodPost:
		var a store.DataAsset
		if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if strings.TrimSpace(a.AssetKey) == "" {
			writeOpenAIError(w, http.StatusBadRequest, "asset_key is required", "invalid_request_error", "missing_fields")
			return
		}
		if looksCorruptedCatalogText(a.Name) || looksCorruptedCatalogText(a.Owner) {
			writeOpenAIError(w, http.StatusBadRequest, "asset name or owner contains corrupted text", "invalid_request_error", "invalid_text_encoding")
			return
		}
		if a.ID == "" {
			a.ID = newID("asset")
		}
		if err := s.db.UpsertDataAsset(r.Context(), a); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "asset_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.asset.upsert", "", auditJSON(map[string]any{"asset_key": a.AssetKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "asset_key": a.AssetKey})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func looksCorruptedCatalogText(value string) bool {
	compact := strings.Join(strings.Fields(value), "")
	questionMarks := strings.Count(compact, "?")
	return questionMarks >= 2 && questionMarks*2 >= utf8.RuneCountInString(compact)
}

// handleDataWorksAssetReadiness manages asset readiness scoring.
func (s *Server) handleDataWorksAssetReadiness(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		scores, err := s.db.ListAssetReadinessScores(r.Context(), strings.TrimSpace(r.URL.Query().Get("asset_key")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "readiness_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"readiness": scores})
	case http.MethodPost:
		var score store.AssetReadinessScore
		if err := json.NewDecoder(r.Body).Decode(&score); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		score = dw.NormalizeReadinessScore(score)
		score.UpdatedBy = adminID(r)
		if err := s.db.UpsertAssetReadinessScore(r.Context(), score); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "readiness_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.asset_readiness.upsert", "", auditJSON(map[string]any{"asset_key": score.AssetKey, "overall_score": score.OverallScore}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "readiness": score})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

// handleDataWorksActionCenter returns prioritized operational follow-ups.
func (s *Server) handleDataWorksActionCenter(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	products, err := s.db.ListDataProducts(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "products_failed")
		return
	}
	fitScores, _ := s.db.ListProductFitScores(r.Context(), "", "")
	contractScopes, _ := s.db.ListContractScopes(r.Context(), "", "")
	entitlements, _ := s.db.ListAPIEntitlements(r.Context(), "", "")
	watermarks, _ := s.db.ListDataWatermarks(r.Context(), "", "")
	costs, _ := s.db.ListProductCosts(r.Context(), "")
	retirements, _ := s.db.ListRetirementCandidates(r.Context(), "")

	actions := []map[string]any{}
	summary := map[string]int{
		"approval_pending":      0,
		"blocked_launches":      0,
		"low_fit_scores":        0,
		"expiring_contracts":    0,
		"inactive_access":       0,
		"stale_watermarks":      0,
		"negative_margin":       0,
		"retirement_candidates": 0,
	}
	for _, product := range products {
		if product.Status == "review" || product.Status == "risk_review" {
			summary["approval_pending"]++
			actions = append(actions, map[string]any{
				"type": "approval_pending", "severity": "medium", "product_key": product.ProductKey,
				"title":       firstNonEmpty(product.ShortName, product.NameKO, product.ProductKey),
				"next_action": "complete review approvals or return to draft with remediation notes",
			})
		}
		if product.Status == "approved" || product.Status == "risk_review" || product.Status == "review" {
			if gate, err := s.dataWorksPublishGate(r.Context(), product); err == nil && !gate.Allowed {
				summary["blocked_launches"]++
				actions = append(actions, map[string]any{
					"type": "launch_blocked", "severity": "high", "product_key": product.ProductKey,
					"blocked_reasons": gate.BlockedReasons, "missing_approvals": gate.MissingApprovals,
					"next_action": "resolve publish gate evidence before moving to published",
				})
			}
		}
	}
	for _, score := range fitScores {
		if score.FitScore >= 50 {
			continue
		}
		summary["low_fit_scores"]++
		actions = append(actions, map[string]any{
			"type": "low_customer_fit", "severity": "low", "product_key": score.ProductKey,
			"customer_segment": score.CustomerSegment, "fit_score": score.FitScore,
			"next_action": "adjust product positioning or target a better segment before proposal work",
		})
	}
	now := time.Now().UTC()
	for _, scope := range contractScopes {
		if scope.ValidTo == "" {
			continue
		}
		if expiresAt, err := time.Parse(time.RFC3339Nano, scope.ValidTo); err == nil && expiresAt.Before(now.Add(30*24*time.Hour)) {
			summary["expiring_contracts"]++
			severity := "medium"
			if expiresAt.Before(now) {
				severity = "high"
			}
			actions = append(actions, map[string]any{
				"type": "contract_expiring", "severity": severity, "product_key": scope.ProductKey,
				"contract_key": scope.ContractKey, "customer_key": scope.CustomerKey, "valid_to": scope.ValidTo,
				"next_action": "renew, narrow, or retire the customer contract scope",
			})
		}
	}
	for _, ent := range entitlements {
		if ent.Status != "active" || entitlementExpired(ent.ExpiresAt, now) {
			summary["inactive_access"]++
			actions = append(actions, map[string]any{
				"type": "entitlement_inactive", "severity": "medium", "product_key": ent.ProductKey,
				"entitlement_id": ent.ID, "api_key_id": ent.APIKeyID, "status": ent.Status, "expires_at": ent.ExpiresAt,
				"next_action": "rotate, reactivate, or remove stale API product access",
			})
		}
	}
	for _, wm := range watermarks {
		status := strings.ToLower(strings.TrimSpace(wm.DelayStatus))
		if status == "stale" || status == "delayed" || status == "failed" {
			summary["stale_watermarks"]++
			actions = append(actions, map[string]any{
				"type": "stale_watermark", "severity": "high", "product_key": wm.ProductKey,
				"asset_key": wm.AssetKey, "delay_status": wm.DelayStatus, "data_as_of": wm.DataAsOf,
				"next_action": "refresh source data or pause proposals that depend on stale data",
			})
		}
	}
	for _, cost := range costs {
		if cost.EstimatedMargin < 0 {
			summary["negative_margin"]++
			actions = append(actions, map[string]any{
				"type": "negative_margin", "severity": "medium", "product_key": cost.ProductKey,
				"estimated_margin": cost.EstimatedMargin, "currency": cost.Currency,
				"next_action": "adjust pricing, reduce query/LLM cost, or restrict low-value usage",
			})
		}
	}
	for _, candidate := range retirements {
		if candidate.Recommendation == "retire" || candidate.Recommendation == "improve" {
			summary["retirement_candidates"]++
			severity := "medium"
			if candidate.Recommendation == "retire" {
				severity = "high"
			}
			actions = append(actions, map[string]any{
				"type": "retirement_candidate", "severity": severity, "product_key": candidate.ProductKey,
				"risk_score": candidate.RiskScore, "recommendation": candidate.Recommendation, "reason": candidate.Reason,
				"next_action": "review product catalog status and decide improve or retire",
			})
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"summary": summary, "actions": actions})
}

func (s *Server) handleDataWorksCustomerSegments(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	switch r.Method {
	case http.MethodGet:
		segments, err := s.db.ListCustomerSegments(r.Context(), strings.TrimSpace(r.URL.Query().Get("segment_key")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "segments_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"segments": segments})
	case http.MethodPost:
		var seg store.CustomerSegment
		if err := json.NewDecoder(r.Body).Decode(&seg); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if err := s.db.UpsertCustomerSegment(r.Context(), seg); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "segment_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.customer_segment.upsert", "", auditJSON(map[string]any{"segment_key": seg.SegmentKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "segment_key": seg.SegmentKey})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

// handleDataWorksFactoryIdeas generates product ideas.
func (s *Server) handleDataWorksFactoryIdeas(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	// Delegate to existing factory handler
	s.handleFactoryGenerateIdeas(w, r)
}

// handleDataWorksFactoryDefinitions generates product definitions.
func (s *Server) handleDataWorksFactoryDefinitions(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryDefineProduct(w, r)
}

// handleDataWorksFactoryAPISpec generates API specifications.
func (s *Server) handleDataWorksFactoryAPISpec(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req struct {
		ProductKey string `json:"product_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey)
	if err != nil || !ok {
		writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
		return
	}
	apiSpec := map[string]any{
		"product_key":     p.ProductKey,
		"endpoint":        "/v1/data-products/" + p.ProductKey + "/query",
		"method":          "POST",
		"auth":            "Bearer token",
		"rate_limit":      "계약별 분당 호출량 제한",
		"request_schema":  map[string]any{"customer_segment": "string", "period": "YYYY-MM"},
		"response_schema": map[string]any{"score": "number", "risk_band": "string", "generated_at": "datetime"},
		"error_codes":     []map[string]string{{"code": "DW_LIMITED_DATA", "message": "requested scope needs additional approval"}},
	}
	s.auditAdmin(r, "dataworks.factory.api_spec", "", auditJSON(map[string]any{"product_key": req.ProductKey}))
	writeJSON(w, http.StatusOK, map[string]any{"api_spec": apiSpec})
}

// handleDataWorksFactoryReportSpec generates report product specifications.
func (s *Server) handleDataWorksFactoryReportSpec(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req struct {
		ProductKey string `json:"product_key"`
		Title      string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	title := firstNonEmpty(req.Title, "데이터 분석 리포트")
	reportSpec := map[string]any{
		"title":       title + " 리포트 상품 설계",
		"product_key": req.ProductKey,
		"report_type": "월간 분석 리포트",
		"sections":    []string{"요약", "핵심 지표", "트렌드 분석", "세그먼트 비교", "리스크 요인", "권고사항"},
		"key_metrics": []string{"전환율", "이탈률", "고객 세그먼트별 수요", "매출 기여도"},
		"charts":      []string{"시계열 트렌드", "세그먼트 분포", "리스크 히트맵"},
		"delivery":    "월간 PDF + 대시보드 링크",
		"use_case":    "고객사 의사결정 지원용 정기 분석 리포트",
	}
	s.auditAdmin(r, "dataworks.factory.report_spec", "", auditJSON(map[string]any{"product_key": req.ProductKey}))
	writeJSON(w, http.StatusOK, map[string]any{"report_spec": reportSpec})
}

// handleDataWorksProposals generates customer proposal packages.
func (s *Server) handleDataWorksProposals(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryProposalGenerate(w, r)
}

// handleDataWorksRiskCheck performs risk assessment.
func (s *Server) handleDataWorksRiskCheck(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryRiskCheck(w, r)
}

// handleDataWorksSimilarityCheck compares products.
func (s *Server) handleDataWorksSimilarityCheck(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryCompareProducts(w, r)
}

// handleDataWorksPOCPlans creates PoC plans.
func (s *Server) handleDataWorksPOCPlans(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryPOCPlan(w, r)
}

// handleDataWorksScoringEvaluate evaluates business priority.
func (s *Server) handleDataWorksScoringEvaluate(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	s.handleFactoryScoringEvaluate(w, r)
}

// handleDataWorksReviews manages approval tasks.
func (s *Server) handleDataWorksReviews(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	products, err := s.db.ListDataProducts(r.Context(), "review")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "reviews_failed")
		return
	}
	riskProducts, _ := s.db.ListDataProducts(r.Context(), "risk_review")
	writeJSON(w, http.StatusOK, map[string]any{
		"review_pending":      products,
		"risk_review_pending": riskProducts,
	})
}

// handleDataWorksReviewAction approves or rejects a product.
func (s *Server) handleDataWorksReviewAction(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/admin/dataworks/reviews/")
	parts := strings.Split(strings.Trim(rest, "/"), "/")
	if len(parts) < 2 {
		writeOpenAIError(w, http.StatusBadRequest, "id and action required", "invalid_request_error", "missing_fields")
		return
	}
	productKey := parts[0]
	action := parts[1]

	p, ok, err := s.db.GetDataProduct(r.Context(), productKey)
	if err != nil || !ok {
		writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
		return
	}

	switch action {
	case "approve":
		p.Status = "approved"
	case "reject":
		p.Status = "draft"
	default:
		writeOpenAIError(w, http.StatusBadRequest, "action must be approve or reject", "invalid_request_error", "bad_action")
		return
	}
	p.UpdatedBy = adminID(r)
	if err := s.db.UpsertDataProduct(r.Context(), p); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "update_failed")
		return
	}
	s.auditAdmin(r, "dataworks.review."+action, "", auditJSON(map[string]any{"product_key": productKey}))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "product_key": productKey, "status": p.Status})
}

// handleDataWorksPortfolio returns products grouped by lifecycle status.
func (s *Server) handleDataWorksPortfolio(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	all, err := s.db.ListDataProducts(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "portfolio_failed")
		return
	}
	grouped := map[string][]store.DataProduct{}
	for _, p := range all {
		grouped[p.Status] = append(grouped[p.Status], p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"portfolio": grouped, "total": len(all)})
}

// handleDataWorksAnalytics returns performance analytics.
func (s *Server) handleDataWorksAnalytics(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	dash, _ := s.db.FactoryDashboard(r.Context())
	products, _ := s.db.ListDataProducts(r.Context(), "")

	var totalRevenue, totalRisk int
	statusCount := map[string]int{}
	for _, p := range products {
		totalRevenue += p.RevenueScore
		totalRisk += p.RiskScore
		statusCount[p.Status]++
	}
	avgRevenue := 0
	avgRisk := 0
	if len(products) > 0 {
		avgRevenue = totalRevenue / len(products)
		avgRisk = totalRisk / len(products)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"analytics": map[string]any{
			"total_products":   len(products),
			"total_ideas":      dash.IdeasTotal,
			"avg_revenue":      avgRevenue,
			"avg_risk":         avgRisk,
			"status_breakdown": statusCount,
			"published":        statusCount["published"],
			"archived":         statusCount["archived"],
		},
	})
}

// handleDataWorksFunnel returns factory-to-revenue lifecycle counts.
func (s *Server) handleDataWorksFunnel(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	dash, err := s.db.FactoryDashboard(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "funnel_failed")
		return
	}
	stages := []map[string]any{
		{"stage": "ideas", "count": dash.IdeasTotal},
		{"stage": "definition", "count": dash.DraftProducts + dash.ReviewProducts},
		{"stage": "risk_review", "count": dash.RiskReviewProducts},
		{"stage": "approved", "count": dash.ApprovedProducts},
		{"stage": "published", "count": dash.PublishedProducts},
	}
	writeJSON(w, http.StatusOK, map[string]any{"funnel": stages})
}

// handleDataWorksFactoryRuns returns AI generation execution history.
func (s *Server) handleDataWorksFactoryRuns(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	days := queryInt(r, "days", 30, 365)
	limit := queryInt(r, "limit", 100, 500)
	since := time.Now().UTC().AddDate(0, 0, -days)
	runs, err := s.db.ListFactoryRuns(r.Context(), since, limit)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "runs_failed")
		return
	}
	evaluations, err := s.db.ListFactoryEvalScores(r.Context(), "", 500)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "run_evaluations_failed")
		return
	}
	byRun := map[string][]store.FactoryEvalScore{}
	for _, evaluation := range evaluations {
		byRun[evaluation.RunID] = append(byRun[evaluation.RunID], evaluation)
	}
	summaries := map[string]any{}
	for runID, scores := range byRun {
		summaries[runID] = factoryEvaluationSummary(scores)
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs, "evaluation_summaries": summaries})
}

// handleDataWorksProductActions manages product-scoped Data Works governance endpoints.
func (s *Server) handleDataWorksProductActions(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/dataworks/products/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "expected /admin/dataworks/products/{key}/{action}", "invalid_request_error", "bad_product_action")
		return
	}
	if dataWorksOpsProductAction(parts) {
		s.handleDataWorksProductByKey(w, r)
		return
	}
	productKey := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	product, ok, err := s.db.GetDataProduct(r.Context(), productKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
		return
	}

	switch action {
	case "canvas":
		s.handleDataWorksProductCanvas(w, r, product)
	case "approvals":
		s.handleDataWorksProductApprovals(w, r, product)
	case "evidence-pack":
		s.handleDataWorksEvidencePack(w, r, product)
	case "publish-gate":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		gate, err := s.dataWorksPublishGate(r.Context(), product)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "publish_gate_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"publish_gate": gate})
	case "publish":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		if !s.enforceDataWorksPublishGate(w, r, product) {
			return
		}
		before := auditJSON(product)
		product.Status = "published"
		product.UpdatedBy = adminID(r)
		if err := s.db.UpsertDataProduct(r.Context(), product); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "publish_failed")
			return
		}
		s.auditAdmin(r, "dataworks.product.publish", before, auditJSON(map[string]any{"product_key": product.ProductKey, "status": product.Status}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "product_key": product.ProductKey, "status": product.Status})
	case "submit", "approve", "reject", "archive":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		transition := struct {
			from map[string]bool
			to   string
		}{}
		switch action {
		case "submit":
			transition = struct {
				from map[string]bool
				to   string
			}{map[string]bool{"draft": true}, "review"}
		case "approve":
			transition = struct {
				from map[string]bool
				to   string
			}{map[string]bool{"review": true, "risk_review": true}, "approved"}
		case "reject":
			transition = struct {
				from map[string]bool
				to   string
			}{map[string]bool{"review": true, "risk_review": true, "approved": true}, "draft"}
		case "archive":
			transition = struct {
				from map[string]bool
				to   string
			}{map[string]bool{"published": true}, "archived"}
		}
		if !transition.from[product.Status] {
			writeOpenAIError(w, http.StatusConflict, "invalid lifecycle transition from "+product.Status+" via "+action, "invalid_request_error", "invalid_product_transition")
			return
		}
		before := auditJSON(product)
		product.Status = transition.to
		product.UpdatedBy = adminID(r)
		if err := s.db.UpsertDataProduct(r.Context(), product); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "transition_failed")
			return
		}
		s.auditAdmin(r, "dataworks.product."+action, before, auditJSON(map[string]any{"product_key": product.ProductKey, "status": product.Status}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "product_key": product.ProductKey, "status": product.Status})
	case "actions":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		logs, err := s.db.ListAdminAudit(r.Context(), 200)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_actions_failed")
			return
		}
		actions := make([]map[string]any, 0)
		for _, log := range logs {
			if !auditReferencesProduct(log.BeforeValue, product.ProductKey) && !auditReferencesProduct(log.AfterValue, product.ProductKey) {
				continue
			}
			actions = append(actions, map[string]any{
				"id": log.ID, "created_by": log.AdminID, "action_type": log.Action, "created_at": log.CreatedAt,
			})
		}
		writeJSON(w, http.StatusOK, map[string]any{"actions": actions})
	case "openapi":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		var scopePtr *store.ContractScope
		if contractKey := strings.TrimSpace(r.URL.Query().Get("contract_key")); contractKey != "" {
			scope, ok, err := s.db.GetContractScope(r.Context(), contractKey)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "contract_scope_failed")
				return
			}
			if !ok || scope.ProductKey != product.ProductKey {
				writeOpenAIError(w, http.StatusNotFound, "contract scope not found for product", "invalid_request_error", "contract_scope_not_found")
				return
			}
			scopePtr = &scope
		}
		var slaPtr *store.ProductSLA
		if sla, ok, err := s.db.GetProductSLA(r.Context(), product.ProductKey); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "sla_failed")
			return
		} else if ok {
			slaPtr = &sla
		}
		writeJSON(w, http.StatusOK, map[string]any{"openapi": dw.BuildDynamicOpenAPIDocument(product, scopePtr, slaPtr)})
	case "contract-versions":
		s.handleDataWorksContractVersions(w, r, product)
	case "fit-scores":
		s.handleDataWorksProductFitScores(w, r, product)
	case "versions":
		s.handleDataWorksProductVersions(w, r, product)
	case "version-diff":
		s.handleDataWorksProductVersionDiff(w, r, product)
	case "contract-scopes":
		s.handleDataWorksContractScopes(w, r, product)
	case "entitlements":
		s.handleDataWorksAPIEntitlements(w, r, product)
	case "sla":
		if len(parts) == 3 && parts[2] == "check" {
			s.handleDataWorksProductSLACheck(w, r, product)
		} else if len(parts) == 3 && parts[2] == "status" {
			s.handleDataWorksProductSLAStatus(w, r, product)
		} else {
			s.handleDataWorksProductSLA(w, r, product)
		}
	case "usage":
		s.handleDataWorksProductUsage(w, r, product)
	case "margin":
		s.handleDataWorksProductMargin(w, r, product)
	case "policy":
		if len(parts) == 3 && parts[2] == "evaluate" {
			s.handleDataWorksProductPolicyEvaluate(w, r, product)
		} else {
			writeOpenAIError(w, http.StatusNotFound, "unknown product action", "invalid_request_error", "not_found")
		}
	case "drift-impact":
		s.handleDataWorksProductDriftImpact(w, r, product)
	case "watermarks":
		s.handleDataWorksDataWatermarks(w, r, product)
	case "costs":
		s.handleDataWorksProductCosts(w, r, product)
	case "unit-economics":
		s.handleDataWorksProductUnitEconomics(w, r, product)
	case "proposal-ab":
		s.handleDataWorksProposalAB(w, r, product)
	case "retirement":
		s.handleDataWorksRetirement(w, r, product)
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown product action", "invalid_request_error", "not_found")
	}
}

func auditReferencesProduct(raw, productKey string) bool {
	if strings.TrimSpace(raw) == "" || strings.TrimSpace(productKey) == "" {
		return false
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return false
	}
	return jsonReferencesProduct(value, productKey)
}

func jsonReferencesProduct(value any, productKey string) bool {
	switch typed := value.(type) {
	case map[string]any:
		if key, ok := typed["product_key"].(string); ok && key == productKey {
			return true
		}
		for _, child := range typed {
			if jsonReferencesProduct(child, productKey) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if jsonReferencesProduct(child, productKey) {
				return true
			}
		}
	}
	return false
}

func dataWorksOpsProductAction(parts []string) bool {
	if len(parts) < 2 {
		return false
	}
	switch parts[1] {
	case "evidence", "regulatory-trace", "api-contract", "mock", "funnel":
		return len(parts) == 2
	case "canvas":
		return len(parts) == 3 && parts[2] == "generate"
	default:
		return false
	}
}

func (s *Server) handleDataWorksProductCanvas(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		canvas, ok, err := s.db.GetProductCanvasV2(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "canvas_failed")
			return
		}
		if !ok {
			draft := dw.DefaultProductCanvas(product)
			writeJSON(w, http.StatusOK, map[string]any{"canvas": draft, "draft": true})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"canvas": canvas, "draft": false})
	case http.MethodPost:
		var canvas store.ProductCanvasV2
		if err := json.NewDecoder(r.Body).Decode(&canvas); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if blankCanvas(canvas) {
			canvas = dw.DefaultProductCanvas(product)
		}
		canvas.ProductKey = product.ProductKey
		canvas.UpdatedBy = adminID(r)
		if err := s.db.UpsertProductCanvasV2(r.Context(), canvas); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "canvas_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.canvas.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "canvas": canvas})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductApprovals(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		traces, err := s.db.ListApprovalTraces(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "approvals_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"approvals": traces})
	case http.MethodPost:
		var payload struct {
			ID          string `json:"id"`
			Step        string `json:"step"`
			Status      string `json:"status"`
			Required    *bool  `json:"required"`
			EvidenceRef string `json:"evidence_ref"`
			Notes       string `json:"notes"`
			DecidedBy   string `json:"decided_by"`
			ExpiresAt   string `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		required := true
		if payload.Required != nil {
			required = *payload.Required
		}
		trace := store.ApprovalTrace{
			ID: firstNonEmpty(strings.TrimSpace(payload.ID), newID("appr")), ProductKey: product.ProductKey,
			Step: strings.TrimSpace(payload.Step), Status: firstNonEmpty(strings.TrimSpace(payload.Status), "pending"),
			Required: required, EvidenceRef: payload.EvidenceRef, Notes: payload.Notes,
			DecidedBy: firstNonEmpty(strings.TrimSpace(payload.DecidedBy), adminID(r)), ExpiresAt: strings.TrimSpace(payload.ExpiresAt),
		}
		if trace.Step == "" {
			writeOpenAIError(w, http.StatusBadRequest, "step is required", "invalid_request_error", "missing_step")
			return
		}
		if err := s.db.UpsertApprovalTrace(r.Context(), trace); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "approval_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.approval_trace.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "step": trace.Step, "status": trace.Status}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "approval": trace})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksEvidencePack(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		pack, ok, err := s.db.GetEvidencePack(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "evidence_pack_failed")
			return
		}
		if !ok {
			writeOpenAIError(w, http.StatusNotFound, "evidence pack not found", "invalid_request_error", "not_found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"evidence_pack": pack})
	case http.MethodPost:
		var payload struct {
			PackJSON    json.RawMessage `json:"pack_json"`
			ArtifactRef string          `json:"artifact_ref"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		packJSON := strings.TrimSpace(string(payload.PackJSON))
		if packJSON == "" {
			generated, err := s.buildEvidencePackJSON(r.Context(), product)
			if err != nil {
				writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "evidence_pack_build_failed")
				return
			}
			packJSON = generated
		}
		pack := store.EvidencePack{ProductKey: product.ProductKey, PackJSON: packJSON, ArtifactRef: payload.ArtifactRef, CreatedBy: adminID(r)}
		if err := s.db.UpsertEvidencePack(r.Context(), pack); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "evidence_pack_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.evidence_pack.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "evidence_pack": pack})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksContractVersions(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		version, ok, err := s.db.LatestContractVersion(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "contract_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"contract_version": optional(version, ok)})
	case http.MethodPost:
		var payload store.ContractVersion
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		payload.ID = firstNonEmpty(strings.TrimSpace(payload.ID), newID("ctrt"))
		payload.ProductKey = product.ProductKey
		payload.CreatedBy = adminID(r)
		version, err := s.db.InsertContractVersion(r.Context(), payload)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "contract_insert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.contract_version.insert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "version": version}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": version})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductFitScores(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		scores, err := s.db.ListProductFitScores(r.Context(), product.ProductKey, strings.TrimSpace(r.URL.Query().Get("customer_segment")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "fit_scores_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"fit_scores": scores})
	case http.MethodPost:
		var payload struct {
			CustomerSegment string   `json:"customer_segment"`
			FitScore        *int     `json:"fit_score"`
			Reason          string   `json:"reason"`
			EvidenceRefs    []string `json:"evidence_refs"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		segments, err := s.db.ListCustomerSegments(r.Context(), strings.TrimSpace(payload.CustomerSegment))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "segments_failed")
			return
		}
		if payload.FitScore != nil {
			if strings.TrimSpace(payload.CustomerSegment) == "" {
				writeOpenAIError(w, http.StatusBadRequest, "customer_segment is required for manual fit_score", "invalid_request_error", "missing_segment")
				return
			}
			score := store.ProductFitScore{
				ProductKey: product.ProductKey, CustomerSegment: strings.TrimSpace(payload.CustomerSegment),
				FitScore: *payload.FitScore, Reason: payload.Reason, EvidenceRefs: payload.EvidenceRefs,
			}
			if err := s.db.UpsertProductFitScore(r.Context(), score); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "fit_score_upsert_failed")
				return
			}
			s.auditAdmin(r, "dataworks.fit_score.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "customer_segment": score.CustomerSegment}))
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "fit_scores": []store.ProductFitScore{score}})
			return
		}
		if len(segments) == 0 {
			writeOpenAIError(w, http.StatusNotFound, "customer segment not found", "invalid_request_error", "segment_not_found")
			return
		}
		out := []store.ProductFitScore{}
		for _, seg := range segments {
			score := dw.ComputeCustomerFitScore(product, seg)
			if err := s.db.UpsertProductFitScore(r.Context(), score); err != nil {
				writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "fit_score_upsert_failed")
				return
			}
			out = append(out, score)
		}
		s.auditAdmin(r, "dataworks.fit_score.compute", "", auditJSON(map[string]any{"product_key": product.ProductKey, "segments": len(out)}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "fit_scores": out})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductVersions(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		versions, err := s.db.ListProductVersions(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "versions_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"versions": versions})
	case http.MethodPost:
		snapshot, err := s.productSnapshotJSON(r.Context(), product)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "snapshot_failed")
			return
		}
		diff := "initial snapshot"
		if latest, ok, err := s.db.LatestProductVersion(r.Context(), product.ProductKey); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "latest_version_failed")
			return
		} else if ok {
			diff = dw.DiffProductSnapshots(latest.SnapshotJSON, snapshot)
		}
		versionNo, err := s.db.InsertProductVersion(r.Context(), store.ProductVersion{
			ProductKey: product.ProductKey, SnapshotJSON: snapshot, DiffSummary: diff, ChangedBy: adminID(r),
		})
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "version_insert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.product_version.insert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "version": versionNo}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "version": versionNo, "diff_summary": diff})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductVersionDiff(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	fromNo, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("from")))
	toNo, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("to")))
	var from, to store.ProductVersion
	var fromOK, toOK bool
	var err error
	if fromNo <= 0 || toNo <= 0 {
		versions, err := s.db.ListProductVersions(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "versions_failed")
			return
		}
		if len(versions) < 2 {
			writeOpenAIError(w, http.StatusBadRequest, "at least two versions are required", "invalid_request_error", "not_enough_versions")
			return
		}
		to = versions[0]
		from = versions[1]
		fromOK, toOK = true, true
	} else {
		from, fromOK, err = s.db.GetProductVersion(r.Context(), product.ProductKey, fromNo)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "from_version_failed")
			return
		}
		to, toOK, err = s.db.GetProductVersion(r.Context(), product.ProductKey, toNo)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "to_version_failed")
			return
		}
	}
	if !fromOK || !toOK {
		writeOpenAIError(w, http.StatusNotFound, "version not found", "invalid_request_error", "version_not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"product_key": product.ProductKey, "from": from.Version, "to": to.Version,
		"diff_summary": dw.DiffProductSnapshots(from.SnapshotJSON, to.SnapshotJSON),
	})
}

func (s *Server) handleDataWorksContractScopes(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		scopes, err := s.db.ListContractScopes(r.Context(), product.ProductKey, strings.TrimSpace(r.URL.Query().Get("customer_key")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "contract_scopes_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"contract_scopes": scopes})
	case http.MethodPost:
		var scope store.ContractScope
		if err := json.NewDecoder(r.Body).Decode(&scope); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		scope.ContractKey = firstNonEmpty(strings.TrimSpace(scope.ContractKey), newID("scope"))
		scope.ProductKey = product.ProductKey
		scope.CreatedBy = adminID(r)
		if err := s.db.UpsertContractScope(r.Context(), scope); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "contract_scope_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.contract_scope.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "contract_key": scope.ContractKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "contract_scope": scope})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksAPIEntitlements(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		ents, err := s.db.ListAPIEntitlements(r.Context(), product.ProductKey, strings.TrimSpace(r.URL.Query().Get("api_key_id")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "entitlements_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entitlements": ents})
	case http.MethodPost:
		var ent store.APIEntitlement
		if err := json.NewDecoder(r.Body).Decode(&ent); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		if strings.TrimSpace(ent.ContractKey) == "" {
			writeOpenAIError(w, http.StatusBadRequest, "contract_key is required", "invalid_request_error", "missing_contract")
			return
		}
		if scope, ok, err := s.db.GetContractScope(r.Context(), ent.ContractKey); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "contract_scope_failed")
			return
		} else if !ok || scope.ProductKey != product.ProductKey {
			writeOpenAIError(w, http.StatusBadRequest, "contract scope not found for product", "invalid_request_error", "contract_scope_not_found")
			return
		}
		ent.ID = firstNonEmpty(strings.TrimSpace(ent.ID), newID("ent"))
		ent.ProductKey = product.ProductKey
		ent.CreatedBy = adminID(r)
		if err := s.db.UpsertAPIEntitlement(r.Context(), ent); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "entitlement_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.api_entitlement.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "entitlement_id": ent.ID}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "entitlement": ent})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductSLA(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		sla, ok, err := s.db.GetProductSLA(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "sla_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"sla": optional(sla, ok)})
	case http.MethodPost:
		var sla store.ProductSLA
		if err := json.NewDecoder(r.Body).Decode(&sla); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		sla.ProductKey = product.ProductKey
		sla.UpdatedBy = adminID(r)
		if err := s.db.UpsertProductSLA(r.Context(), sla); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "sla_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.sla.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sla": sla})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksDataWatermarks(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		watermarks, err := s.db.ListDataWatermarks(r.Context(), product.ProductKey, strings.TrimSpace(r.URL.Query().Get("asset_key")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "watermarks_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"watermarks": watermarks})
	case http.MethodPost:
		var wm store.DataWatermark
		if err := json.NewDecoder(r.Body).Decode(&wm); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		wm.ProductKey = product.ProductKey
		if strings.TrimSpace(wm.AssetKey) == "" {
			assets := dw.ProductAssetKeys(product)
			if len(assets) > 0 {
				wm.AssetKey = assets[0]
			}
		}
		wm.UpdatedBy = adminID(r)
		if err := s.db.UpsertDataWatermark(r.Context(), wm); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "watermark_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.watermark.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "asset_key": wm.AssetKey, "delay_status": wm.DelayStatus}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "watermark": wm})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProductCosts(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		cost, ok, err := s.db.GetProductCost(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "cost_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"cost": optional(cost, ok)})
	case http.MethodPost:
		var cost store.ProductCost
		if err := json.NewDecoder(r.Body).Decode(&cost); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		cost.ProductKey = product.ProductKey
		cost.UpdatedBy = adminID(r)
		if cost.EstimatedMargin == 0 {
			cost.EstimatedMargin = float64(product.RevenueScore)*1000 - cost.QueryCost - cost.LLMCost - cost.OpsCost - cost.DataProcessingCost
		}
		if err := s.db.UpsertProductCost(r.Context(), cost); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "cost_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.cost.upsert", "", auditJSON(map[string]any{"product_key": product.ProductKey, "estimated_margin": cost.EstimatedMargin}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "cost": cost})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksProposalAB(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		events, err := s.db.ListCustomerProposalEvents(r.Context(), product.ProductKey, strings.TrimSpace(r.URL.Query().Get("customer_key")))
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "proposal_events_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"proposal_events": events})
	case http.MethodPost:
		var payload struct {
			CustomerKey     string `json:"customer_key"`
			CustomerSegment string `json:"customer_segment"`
			ProposalID      string `json:"proposal_id"`
			NextActionAt    string `json:"next_action_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
			return
		}
		variants := dw.BuildProposalABVariants(product, payload.CustomerSegment)
		proposalID := firstNonEmpty(strings.TrimSpace(payload.ProposalID), newID("propab"))
		for _, variant := range variants {
			name, _ := variant["variant"].(string)
			_ = s.db.InsertCustomerProposalEvent(r.Context(), store.CustomerProposalEvent{
				ID: newID("pevt"), CustomerKey: payload.CustomerKey, ProductKey: product.ProductKey,
				ProposalID: proposalID, Variant: name, EventType: "generated", Feedback: auditJSON(variant),
				NextActionAt: payload.NextActionAt, CreatedBy: adminID(r),
			})
		}
		s.auditAdmin(r, "dataworks.proposal_ab.generate", "", auditJSON(map[string]any{"product_key": product.ProductKey, "proposal_id": proposalID, "variants": len(variants)}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "proposal_id": proposalID, "variants": variants})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) handleDataWorksRetirement(w http.ResponseWriter, r *http.Request, product store.DataProduct) {
	switch r.Method {
	case http.MethodGet:
		candidate, err := s.evaluateDataWorksRetirementCandidate(r.Context(), product)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "retirement_failed")
			return
		}
		stored, _ := s.db.ListRetirementCandidates(r.Context(), product.ProductKey)
		writeJSON(w, http.StatusOK, map[string]any{"candidate": candidate, "stored": stored})
	case http.MethodPost:
		candidate, err := s.evaluateDataWorksRetirementCandidate(r.Context(), product)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "retirement_failed")
			return
		}
		if err := s.db.UpsertRetirementCandidate(r.Context(), candidate); err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "retirement_upsert_failed")
			return
		}
		s.auditAdmin(r, "dataworks.retirement.evaluate", "", auditJSON(map[string]any{"product_key": product.ProductKey, "recommendation": candidate.Recommendation, "risk_score": candidate.RiskScore}))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "candidate": candidate})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func (s *Server) evaluateDataWorksRetirementCandidate(ctx context.Context, product store.DataProduct) (store.RetirementCandidate, error) {
	var costPtr *store.ProductCost
	if cost, ok, err := s.db.GetProductCost(ctx, product.ProductKey); err != nil {
		return store.RetirementCandidate{}, err
	} else if ok {
		costPtr = &cost
	}
	watermarks, err := s.db.ListDataWatermarks(ctx, product.ProductKey, "")
	if err != nil {
		return store.RetirementCandidate{}, err
	}
	fitScores, err := s.db.ListProductFitScores(ctx, product.ProductKey, "")
	if err != nil {
		return store.RetirementCandidate{}, err
	}
	entitlements, err := s.db.ListAPIEntitlements(ctx, product.ProductKey, "")
	if err != nil {
		return store.RetirementCandidate{}, err
	}
	return dw.EvaluateRetirementCandidate(product, costPtr, watermarks, fitScores, entitlements, time.Now().UTC()), nil
}

func (s *Server) productSnapshotJSON(ctx context.Context, product store.DataProduct) (string, error) {
	var canvasPtr *store.ProductCanvasV2
	if canvas, ok, err := s.db.GetProductCanvasV2(ctx, product.ProductKey); err != nil {
		return "", err
	} else if ok {
		canvasPtr = &canvas
	}
	var contractPtr *store.ContractVersion
	if contract, ok, err := s.db.LatestContractVersion(ctx, product.ProductKey); err != nil {
		return "", err
	} else if ok {
		contractPtr = &contract
	}
	return dw.MarshalProductSnapshot(product, canvasPtr, contractPtr)
}

func (s *Server) dataWorksPublishGate(ctx context.Context, product store.DataProduct) (dw.PublishGateResult, error) {
	readiness, err := s.dataWorksProductReadiness(ctx, product)
	if err != nil {
		return dw.PublishGateResult{}, err
	}
	approvals, err := s.db.ListApprovalTraces(ctx, product.ProductKey)
	if err != nil {
		return dw.PublishGateResult{}, err
	}
	pack, ok, err := s.db.GetEvidencePack(ctx, product.ProductKey)
	if err != nil {
		return dw.PublishGateResult{}, err
	}
	var packPtr *store.EvidencePack
	if ok {
		packPtr = &pack
	}

	// Fetch SLA
	sla, _, _ := s.db.GetProductSLA(ctx, product.ProductKey)
	var slaPtr *store.ProductSLA
	if sla.ProductKey != "" {
		slaPtr = &sla
	}

	// Fetch Cost
	cost, _, _ := s.db.GetProductCost(ctx, product.ProductKey)
	var costPtr *store.ProductCost
	if cost.ProductKey != "" {
		costPtr = &cost
	}

	// Fetch Quality Results
	var qualityResults []store.DataQualityResult
	for _, assetKey := range dw.ProductAssetKeys(product) {
		qres, err := s.db.ListDataQualityResults(ctx, assetKey)
		if err == nil {
			qualityResults = append(qualityResults, qres...)
		}
	}

	// Fetch Risk Review
	hasRiskReview := false
	if rrev, ok, err := s.db.LatestProductRiskReview(ctx, product.ProductKey); err == nil && ok && rrev.ProductKey != "" {
		hasRiskReview = true
	}

	// Fetch Masking Configured
	maskingConfigured := true
	scopes, err := s.db.ListContractScopes(ctx, product.ProductKey, "")
	if err == nil && len(scopes) > 0 {
		hasMasking := false
		for _, sc := range scopes {
			if sc.MaskingPolicy != "" && sc.MaskingPolicy != "none" {
				hasMasking = true
				break
			}
		}
		maskingConfigured = hasMasking
	}

	return dw.EvaluatePublishGateV2(product, readiness, approvals, packPtr, slaPtr, costPtr, qualityResults, hasRiskReview, maskingConfigured, time.Now().UTC()), nil
}

func entitlementExpired(raw string, now time.Time) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, raw)
	return err == nil && expiresAt.Before(now)
}

func (s *Server) enforceDataWorksPublishGate(w http.ResponseWriter, r *http.Request, product store.DataProduct) bool {
	gate, err := s.dataWorksPublishGate(r.Context(), product)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "publish_gate_failed")
		return false
	}
	if !gate.Allowed {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "publish gate blocked", "publish_gate": gate})
		return false
	}
	return true
}

func (s *Server) dataWorksProductReadiness(ctx context.Context, product store.DataProduct) ([]store.AssetReadinessScore, error) {
	out := []store.AssetReadinessScore{}
	seen := map[string]bool{}
	for _, assetKey := range dw.ProductAssetKeys(product) {
		key := strings.ToLower(strings.TrimSpace(assetKey))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		scores, err := s.db.ListAssetReadinessScores(ctx, assetKey)
		if err != nil {
			return nil, err
		}
		out = append(out, scores...)
	}
	return out, nil
}

func (s *Server) buildEvidencePackJSON(ctx context.Context, product store.DataProduct) (string, error) {
	var canvasPtr *store.ProductCanvasV2
	if canvas, ok, err := s.db.GetProductCanvasV2(ctx, product.ProductKey); err != nil {
		return "", err
	} else if ok {
		canvasPtr = &canvas
	} else {
		canvas := dw.DefaultProductCanvas(product)
		canvasPtr = &canvas
	}
	readiness, err := s.dataWorksProductReadiness(ctx, product)
	if err != nil {
		return "", err
	}
	approvals, err := s.db.ListApprovalTraces(ctx, product.ProductKey)
	if err != nil {
		return "", err
	}
	placeholderPack := store.EvidencePack{ProductKey: product.ProductKey, PackJSON: "pending"}
	gate := dw.EvaluatePublishGate(product, readiness, approvals, &placeholderPack, time.Now().UTC())
	def, defOK, err := s.db.LatestProductDefinition(ctx, product.ProductKey)
	if err != nil {
		return "", err
	}
	risk, riskOK, err := s.db.LatestProductRiskReview(ctx, product.ProductKey)
	if err != nil {
		return "", err
	}
	poc, pocOK, err := s.db.LatestProductPOCPlan(ctx, product.ProductKey)
	if err != nil {
		return "", err
	}
	contract, contractOK, err := s.db.LatestContractVersion(ctx, product.ProductKey)
	if err != nil {
		return "", err
	}
	body := dw.BuildEvidencePack(product, canvasPtr, readiness, approvals, optional(def, defOK), optional(risk, riskOK), optional(poc, pocOK), optional(contract, contractOK), gate)
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func blankCanvas(canvas store.ProductCanvasV2) bool {
	return strings.TrimSpace(canvas.CustomerProblem) == "" &&
		strings.TrimSpace(canvas.Buyer) == "" &&
		strings.TrimSpace(canvas.UseCases) == "" &&
		strings.TrimSpace(canvas.ProvidedData) == "" &&
		strings.TrimSpace(canvas.Differentiation) == "" &&
		strings.TrimSpace(canvas.PricingModel) == "" &&
		strings.TrimSpace(canvas.RiskNotes) == "" &&
		strings.TrimSpace(canvas.POCSuccessCriteria) == "" &&
		strings.TrimSpace(canvas.ExpectedRevenue) == ""
}

// handleDataWorksProducts is a compatibility wrapper for /admin/dataworks/products.
func (s *Server) handleDataWorksProducts(w http.ResponseWriter, r *http.Request) {
	s.handleAdminDataProducts(w, r)
}
