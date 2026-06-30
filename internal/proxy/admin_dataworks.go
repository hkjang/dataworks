package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"clustara/internal/store"
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
		"product_key": p.ProductKey,
		"endpoint":    "/v1/data-products/" + p.ProductKey + "/query",
		"method":      "POST",
		"auth":        "Bearer token",
		"rate_limit":  "계약별 분당 호출량 제한",
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
			"total_products":  len(products),
			"total_ideas":     dash.IdeasTotal,
			"avg_revenue":     avgRevenue,
			"avg_risk":        avgRisk,
			"status_breakdown": statusCount,
			"published":       statusCount["published"],
			"archived":        statusCount["archived"],
		},
	})
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
	since := time.Now().UTC().AddDate(0, 0, -30)
	runs, err := s.db.ListFactoryRuns(r.Context(), since, 100)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "runs_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
}

// handleDataWorksProducts is a compatibility wrapper for /admin/dataworks/products.
func (s *Server) handleDataWorksProducts(w http.ResponseWriter, r *http.Request) {
	s.handleAdminDataProducts(w, r)
}
