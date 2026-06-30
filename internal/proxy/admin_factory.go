package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"clustara/internal/store"
)

type factoryIdeaRequest struct {
	Industry       string   `json:"industry"`
	CustomerType   string   `json:"customer_type"`
	CustomerTypes  []string `json:"customer_types"`
	MarketNeed     string   `json:"market_need"`
	Purpose        string   `json:"purpose"`
	DataAssets     []string `json:"data_assets"`
	Count          int      `json:"count"`
	CreatedBy      string   `json:"created_by"`
	SourceQuestion string   `json:"source_question"`
}

type factoryDefineRequest struct {
	IdeaID          string   `json:"idea_id"`
	Title           string   `json:"title"`
	TargetIndustry  string   `json:"target_industry"`
	TargetCustomers []string `json:"target_customers"`
	CustomerNeed    string   `json:"customer_need"`
	DataAssets      []string `json:"data_assets"`
	DeliveryMethod  string   `json:"delivery_method"`
	ExpectedImpact  string   `json:"expected_impact"`
	ProductKey      string   `json:"product_key"`
}

type factoryRiskRequest struct {
	ProductKey      string   `json:"product_key"`
	Title           string   `json:"title"`
	Description     string   `json:"description"`
	DataAssets      []string `json:"data_assets"`
	DeliveryMethod  string   `json:"delivery_method"`
	TargetCustomers []string `json:"target_customers"`
}

type factoryPOCRequest struct {
	ProductKey     string `json:"product_key"`
	Title          string `json:"title"`
	DataScope      string `json:"data_scope"`
	SuccessMetric  string `json:"success_metric"`
	Timeline       string `json:"timeline"`
	Owner          string `json:"owner"`
	CustomerType   string `json:"customer_type"`
	ApprovalStatus string `json:"approval_status"`
}

type factoryScoringRequest struct {
	ProductKey        string   `json:"product_key"`
	Title             string   `json:"title"`
	TargetCustomers   []string `json:"target_customers"`
	DataAssets        []string `json:"data_assets"`
	DeliveryMethod    string   `json:"delivery_method"`
	MarketNeed        string   `json:"market_need"`
	RiskScore         int      `json:"risk_score"`
	ImplementationFit int      `json:"implementation_fit"`
}

type factoryProposalRequest struct {
	ProductKey         string `json:"product_key"`
	TargetCustomerType string `json:"target_customer_type"`
	CustomerName       string `json:"customer_name"`
	UseCase            string `json:"use_case"`
}

func (s *Server) handleFactoryDashboard(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	d, err := s.db.FactoryDashboard(r.Context())
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "dashboard_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"dashboard": d})
}

func (s *Server) handleFactoryProducts(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	products, err := s.db.ListDataProducts(r.Context(), strings.TrimSpace(r.URL.Query().Get("status")))
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "products_failed")
		return
	}
	ideas, _ := s.db.ListProductIdeas(r.Context(), "", 50)
	dashboard, _ := s.db.FactoryDashboard(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"products": products, "ideas": ideas, "dashboard": dashboard})
}

func (s *Server) handleFactoryProductByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/admin/factory/products/"), "/")
	if rest == "" {
		writeOpenAIError(w, http.StatusNotFound, "product id required", "invalid_request_error", "not_found")
		return
	}
	parts := strings.Split(rest, "/")
	key := strings.TrimSpace(parts[0])
	action := ""
	if len(parts) > 1 {
		action = strings.TrimSpace(parts[1])
	}
	switch r.Method {
	case http.MethodGet:
		p, ok, err := s.db.GetDataProduct(r.Context(), key)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "get_failed")
			return
		}
		if !ok {
			writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
			return
		}
		def, defOK, _ := s.db.LatestProductDefinition(r.Context(), p.ProductKey)
		risk, riskOK, _ := s.db.LatestProductRiskReview(r.Context(), p.ProductKey)
		poc, pocOK, _ := s.db.LatestProductPOCPlan(r.Context(), p.ProductKey)
		writeJSON(w, http.StatusOK, map[string]any{
			"product": p, "definition": optional(def, defOK), "risk_review": optional(risk, riskOK), "poc_plan": optional(poc, pocOK),
		})
	case http.MethodPost:
		if action == "" {
			writeOpenAIError(w, http.StatusBadRequest, "action is required", "invalid_request_error", "missing_action")
			return
		}
		p, ok, err := s.db.GetDataProduct(r.Context(), key)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "get_failed")
			return
		}
		if !ok {
			writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
			return
		}
		next := map[string]string{"approve": "approved", "publish": "published", "archive": "archived"}[action]
		if next == "" {
			writeOpenAIError(w, http.StatusBadRequest, "action must be approve|publish|archive", "invalid_request_error", "bad_action")
			return
		}
		before := auditJSON(p)
		p.Status = next
		p.UpdatedBy = adminID(r)
		if err := s.db.UpsertDataProduct(r.Context(), p); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "status_failed")
			return
		}
		s.auditAdmin(r, "factory.product."+action, before, auditJSON(map[string]any{"product_key": p.ProductKey, "status": next}))
		writeJSON(w, http.StatusOK, map[string]any{"product_key": p.ProductKey, "status": next, "ok": true})
	default:
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
	}
}

func optional(v any, ok bool) any {
	if !ok {
		return nil
	}
	return v
}

func (s *Server) handleFactoryGenerateIdeas(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryIdeaRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if strings.TrimSpace(req.Industry) == "" && strings.TrimSpace(req.MarketNeed) == "" && strings.TrimSpace(req.SourceQuestion) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "industry, market_need, or source_question is required", "invalid_request_error", "missing_fields")
		return
	}
	ideas := buildFactoryIdeas(req, adminID(r))
	for _, idea := range ideas {
		if err := s.db.InsertProductIdea(r.Context(), idea); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "idea_insert_failed")
			return
		}
	}
	_ = s.db.InsertFactoryRun(r.Context(), store.FactoryRun{
		ID: newID("frun"), RunType: "ideas.generate", Model: "rules:dataworks-mvp",
		InputHash: factoryShortHash(req.Industry + "|" + req.MarketNeed + "|" + strings.Join(req.DataAssets, ",")),
		OutputRef: strings.Join(productIdeaIDs(ideas), ","), LatencyMS: 0, CreatedBy: adminID(r),
	})
	s.auditAdmin(r, "factory.ideas.generate", "", auditJSON(map[string]any{"count": len(ideas), "industry": req.Industry}))
	writeJSON(w, http.StatusOK, map[string]any{"ideas": ideas})
}

func (s *Server) handleFactoryDefineProduct(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryDefineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.IdeaID != "" {
		idea, ok, err := s.db.GetProductIdea(r.Context(), req.IdeaID)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "idea_failed")
			return
		}
		if !ok {
			writeOpenAIError(w, http.StatusNotFound, "idea not found", "invalid_request_error", "idea_not_found")
			return
		}
		req.Title = firstNonEmpty(req.Title, idea.Title)
		req.TargetIndustry = firstNonEmpty(req.TargetIndustry, idea.TargetIndustry)
		if len(req.TargetCustomers) == 0 {
			req.TargetCustomers = idea.TargetCustomers
		}
		req.CustomerNeed = firstNonEmpty(req.CustomerNeed, idea.CustomerNeed)
		if len(req.DataAssets) == 0 {
			req.DataAssets = idea.DataAssets
		}
		req.DeliveryMethod = firstNonEmpty(req.DeliveryMethod, idea.DeliveryMethod)
		req.ExpectedImpact = firstNonEmpty(req.ExpectedImpact, idea.ExpectedImpact)
	}
	if strings.TrimSpace(req.Title) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "title is required", "invalid_request_error", "missing_title")
		return
	}
	key := strings.TrimSpace(req.ProductKey)
	if key == "" {
		key = productKeyFromTitle(req.Title)
	}
	definition := buildProductDefinition(key, req)
	defJSON, _ := json.Marshal(definition)
	version, err := s.db.InsertProductDefinition(r.Context(), store.ProductDefinition{
		ID: newID("pdef"), IdeaID: req.IdeaID, ProductKey: key, DefinitionJSON: string(defJSON), Status: "review", CreatedBy: adminID(r),
	})
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "definition_failed")
		return
	}
	apiSpec, _ := json.Marshal(definition["api_spec"])
	pricing, _ := json.Marshal(definition["pricing_model"])
	product := store.DataProduct{
		ID: newID("dprod"), ProductKey: key, NameKO: req.Title, ShortName: shortProductName(req.Title), Description: stringValue(definition["one_line"]),
		ExecutiveSummary: stringValue(definition["executive_summary"]), SalesPitch: stringValue(definition["sales_pitch"]),
		SourceType: inferProductSourceType(req.DeliveryMethod), SourceRef: strings.Join(req.DataAssets, ","), Owner: "data-business",
		AllowedTeams: []string{"data-business", "strategy", "sales", "legal"}, Sensitivity: inferSensitivity(req), Status: "review",
		TargetIndustries: []string{req.TargetIndustry}, TargetCustomers: req.TargetCustomers, PricingModel: string(pricing), APISpec: string(apiSpec),
		RiskScore:       riskScoreFromText(req.Title + " " + req.CustomerNeed + " " + strings.Join(req.DataAssets, " ")),
		RevenueScore:    revenueScore(req.TargetCustomers, req.DataAssets, req.CustomerNeed),
		Differentiation: stringValue(definition["differentiation"]), UpdatedBy: adminID(r),
	}
	if err := s.db.UpsertDataProduct(r.Context(), product); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_failed")
		return
	}
	_ = s.db.InsertFactoryRun(r.Context(), store.FactoryRun{ID: newID("frun"), RunType: "products.define", Model: "rules:dataworks-mvp", InputHash: factoryShortHash(req.Title + req.CustomerNeed), OutputRef: key, CreatedBy: adminID(r)})
	s.auditAdmin(r, "factory.products.define", "", auditJSON(map[string]any{"product_key": key, "version": version}))
	writeJSON(w, http.StatusOK, map[string]any{"product_key": key, "version": version, "definition": definition, "product": product})
}

func (s *Server) handleFactoryRiskCheck(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryRiskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.ProductKey != "" {
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			req.Title = firstNonEmpty(req.Title, p.NameKO)
			req.Description = firstNonEmpty(req.Description, p.Description, p.ExecutiveSummary)
			if len(req.DataAssets) == 0 {
				req.DataAssets = splitLoose(p.SourceRef)
			}
			if len(req.TargetCustomers) == 0 {
				req.TargetCustomers = p.TargetCustomers
			}
			req.DeliveryMethod = firstNonEmpty(req.DeliveryMethod, p.SourceType)
		}
	}
	review := buildRiskReview(req)
	checklistJSON, _ := json.Marshal(review["checklist"])
	rr := store.ProductRiskReview{
		ID: newID("risk"), ProductKey: req.ProductKey, PrivacyScore: intValue(review["privacy_score"]), CreditScore: intValue(review["credit_score"]),
		AIScore: intValue(review["ai_score"]), SecurityScore: intValue(review["security_score"]), OverallScore: intValue(review["overall_score"]),
		ChecklistJSON: string(checklistJSON), ReviewNotes: stringValue(review["review_notes"]), CreatedBy: adminID(r),
	}
	if req.ProductKey != "" {
		if err := s.db.InsertProductRiskReview(r.Context(), rr); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "risk_failed")
			return
		}
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			p.RiskScore = rr.OverallScore
			p.Status = "risk_review"
			p.UpdatedBy = adminID(r)
			_ = s.db.UpsertDataProduct(r.Context(), p)
		}
	}
	s.auditAdmin(r, "factory.risk.check", "", auditJSON(map[string]any{"product_key": req.ProductKey, "overall_score": rr.OverallScore}))
	writeJSON(w, http.StatusOK, map[string]any{"risk_review": rr, "result": review})
}

func (s *Server) handleFactoryPOCPlan(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryPOCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.ProductKey != "" {
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			req.Title = firstNonEmpty(req.Title, p.NameKO)
			req.DataScope = firstNonEmpty(req.DataScope, p.SourceRef, "최근 24개월, 비식별 또는 집계 데이터")
		}
	}
	plan := buildPOCPlan(req)
	planJSON, _ := json.Marshal(plan)
	poc := store.ProductPOCPlan{
		ID: newID("poc"), ProductKey: req.ProductKey, DataScope: stringValue(plan["data_scope"]), SuccessMetric: stringValue(plan["success_metric"]),
		Timeline: stringValue(plan["timeline"]), Owner: firstNonEmpty(req.Owner, "data-business"), ApprovalStatus: firstNonEmpty(req.ApprovalStatus, "pending"),
		PlanJSON: string(planJSON), CreatedBy: adminID(r),
	}
	if req.ProductKey != "" {
		if err := s.db.InsertProductPOCPlan(r.Context(), poc); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "poc_failed")
			return
		}
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			p.POCPlan = string(planJSON)
			p.UpdatedBy = adminID(r)
			_ = s.db.UpsertDataProduct(r.Context(), p)
		}
	}
	s.auditAdmin(r, "factory.poc.plan", "", auditJSON(map[string]any{"product_key": req.ProductKey}))
	writeJSON(w, http.StatusOK, map[string]any{"poc_plan": poc, "plan": plan})
}

func (s *Server) handleFactoryScoringEvaluate(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryScoringRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.ProductKey != "" {
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			req.Title = firstNonEmpty(req.Title, p.NameKO)
			if len(req.TargetCustomers) == 0 {
				req.TargetCustomers = p.TargetCustomers
			}
			if len(req.DataAssets) == 0 {
				req.DataAssets = splitLoose(p.SourceRef)
			}
			req.DeliveryMethod = firstNonEmpty(req.DeliveryMethod, p.SourceType)
			req.RiskScore = factoryFirstPositive(req.RiskScore, p.RiskScore)
		}
	}
	score := buildPriorityScore(req)
	if req.ProductKey != "" {
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			p.RevenueScore = intValue(score["revenue_score"])
			p.RiskScore = intValue(score["risk_score"])
			p.UpdatedBy = adminID(r)
			_ = s.db.UpsertDataProduct(r.Context(), p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"score": score})
}

func (s *Server) handleFactoryProposalGenerate(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req factoryProposalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
		return
	}
	proposal := buildProposalPackage(p, req)
	proposalJSON, _ := json.Marshal(proposal)
	pkg := store.ProposalPackage{ID: newID("prop"), ProductKey: p.ProductKey, TargetCustomerType: req.TargetCustomerType, ProposalJSON: string(proposalJSON), CreatedBy: adminID(r)}
	if err := s.db.InsertProposalPackage(r.Context(), pkg); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "proposal_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"proposal": proposal, "package": pkg})
}

func (s *Server) handleFactoryCompareProducts(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	var req struct {
		ProductKey  string `json:"product_key"`
		Title       string `json:"title"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if req.ProductKey != "" {
		if p, ok, err := s.db.GetDataProduct(r.Context(), req.ProductKey); err == nil && ok {
			req.Title = firstNonEmpty(req.Title, p.NameKO)
			req.Description = firstNonEmpty(req.Description, p.Description, p.ExecutiveSummary)
		}
	}
	products, err := s.db.ListDataProducts(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "compare_failed")
		return
	}
	matches := compareProducts(req.ProductKey, req.Title+" "+req.Description, products)
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches, "note": "중복 가능성이 높은 상품은 리브랜딩보다 결합 또는 차별화 포인트 보강을 우선 검토하세요."})
}

func buildFactoryIdeas(req factoryIdeaRequest, actor string) []store.ProductIdea {
	count := req.Count
	if count <= 0 {
		count = 5
	}
	if count < 5 {
		count = 5
	}
	if count > 20 {
		count = 20
	}
	industry := firstNonEmpty(strings.TrimSpace(req.Industry), "금융")
	customers := req.CustomerTypes
	if len(customers) == 0 && strings.TrimSpace(req.CustomerType) != "" {
		customers = splitLoose(req.CustomerType)
	}
	if len(customers) == 0 {
		customers = []string{"은행", "카드", "보험", "핀테크"}
	}
	need := firstNonEmpty(strings.TrimSpace(req.MarketNeed), strings.TrimSpace(req.Purpose), strings.TrimSpace(req.SourceQuestion), "신규 데이터 상품 발굴")
	templates := []struct {
		title    string
		method   string
		impact   string
		diff     string
		customer string
	}{
		{"%s 리스크 조기탐지 API", "월 구독형 API", "고위험 고객군을 조기에 식별해 심사·영업 의사결정을 단축", "대체정보와 기존 신용정보를 결합한 실행형 스코어", customers[0]},
		{"%s 고객군 세그먼트 인사이트 리포트", "월간 분석 리포트", "고객군별 수요·이탈·교차판매 기회를 표준 리포트로 제공", "시장/고객군 메타와 내부 질의 패턴을 결합", customers[minInt(1, len(customers)-1)]},
		{"%s 이상징후 모니터링 대시보드", "대시보드형 상품", "운영자가 이벤트 변화를 상시 관찰하고 PoC 전환 후보를 추적", "반복 질의와 지표 카탈로그를 함께 활용", customers[minInt(2, len(customers)-1)]},
		{"%s 대체정보 기반 수요 예측 스코어", "스코어 API", "영업 우선순위와 캠페인 대상을 점수화", "비식별·집계 데이터 기반으로 외부 제공 리스크 완화", customers[0]},
		{"%s PoC 샘플 데이터 패키지", "비식별 샘플 데이터셋", "고객사가 본계약 전 데이터 효용을 빠르게 검증", "PoC 범위와 성공지표를 함께 제공", customers[len(customers)-1]},
		{"%s API 패키지 과금 모델", "건별 조회 API", "반복 판매 가능한 조회형 상품으로 전환", "Rate Limit·샘플 응답·오류코드까지 패키징", customers[0]},
		{"%s 규제 안전형 집계 리포트", "집계 리포트", "개인 단위 제공을 피하면서 시장 인사이트를 제공", "가명·집계 원칙을 제품 구조에 내장", customers[minInt(1, len(customers)-1)]},
	}
	out := []store.ProductIdea{}
	for i := 0; i < count; i++ {
		t := templates[i%len(templates)]
		title := strings.TrimSpace(t.customer + " 대상 " + fmtTitle(t.title, industry))
		text := title + " " + need + " " + strings.Join(req.DataAssets, " ")
		out = append(out, store.ProductIdea{
			ID: newID("idea"), Title: title, TargetIndustry: industry, TargetCustomers: []string{t.customer},
			CustomerNeed: need, DataAssets: req.DataAssets, DeliveryMethod: t.method, ExpectedImpact: t.impact,
			DifficultyScore: clampScore(38 + len(req.DataAssets)*5 + i*3), RiskScore: riskScoreFromText(text),
			RevenueScore:    revenueScore([]string{t.customer}, req.DataAssets, need) + (i % 3 * 2),
			Differentiation: t.diff, SourcePrompt: compactPromptSummary(req), CreatedBy: actor, Status: "draft",
		})
	}
	return out
}

func buildProductDefinition(key string, req factoryDefineRequest) map[string]any {
	endpoint := "/v1/data-products/" + key + "/query"
	return map[string]any{
		"product_key":       key,
		"name":              req.Title,
		"one_line":          firstNonEmpty(req.CustomerNeed, req.Title+" 상품화 후보"),
		"executive_summary": req.Title + "은 " + strings.Join(req.DataAssets, ", ") + " 자산을 활용해 " + strings.Join(req.TargetCustomers, ", ") + " 고객군의 의사결정을 돕는 데이터 상품입니다.",
		"target_customers":  req.TargetCustomers,
		"pain_point":        firstNonEmpty(req.CustomerNeed, "고객군별 수요를 표준화된 데이터 상품으로 빠르게 검증해야 합니다."),
		"provided_data":     req.DataAssets,
		"delivery_method":   firstNonEmpty(req.DeliveryMethod, "월 구독형 API"),
		"product_type":      inferProductSourceType(req.DeliveryMethod),
		"api_spec": map[string]any{
			"endpoint": endpoint, "method": "POST", "auth": "Bearer token",
			"rate_limit":      "계약별 분당 호출량 제한",
			"request_schema":  map[string]any{"customer_segment": "string", "period": "YYYY-MM", "purpose": "string"},
			"response_schema": map[string]any{"score": "number", "risk_band": "string", "drivers": []string{"string"}, "generated_at": "datetime"},
			"sample_error":    map[string]any{"code": "DW_LIMITED_DATA", "message": "requested scope needs additional approval"},
		},
		"pricing_model": map[string]any{
			"type": "subscription_plus_usage", "base_fee": "월 기본료", "unit": "API call 또는 report seat", "tiers": []string{"PoC", "Standard", "Enterprise"},
		},
		"expected_effect":  firstNonEmpty(req.ExpectedImpact, "PoC 기간 내 고객군별 활용성과 반복 판매 가능성을 검증합니다."),
		"sales_pitch":      "KCB 내부 데이터 자산을 고객 업무 문제에 맞춘 API/리포트 패키지로 전환해 도입 시간을 줄입니다.",
		"differentiation":  "반복 Text2SQL 수요, 내부 데이터 자산, 규제 체크리스트를 한 상품 정의 안에서 함께 관리합니다.",
		"risk_guardrails":  []string{"개인 단위 원천값 외부 제공 금지", "샘플 응답 비식별화", "목적 제한 및 승인 이력 보관"},
		"lifecycle_status": "review",
	}
}

func buildRiskReview(req factoryRiskRequest) map[string]any {
	text := strings.ToLower(req.Title + " " + req.Description + " " + strings.Join(req.DataAssets, " ") + " " + req.DeliveryMethod + " " + strings.Join(req.TargetCustomers, " "))
	privacy := scoreByKeywords(text, 20, map[string]int{"개인": 35, "고객": 15, "식별": 25, "주소": 20, "전화": 20, "device": 10})
	credit := scoreByKeywords(text, 15, map[string]int{"신용": 35, "대출": 25, "상환": 20, "연체": 25, "보험": 10, "score": 10})
	ai := scoreByKeywords(text, 20, map[string]int{"ai": 20, "스코어": 25, "score": 25, "추천": 15, "분류": 15, "자동": 15})
	security := scoreByKeywords(text, 20, map[string]int{"api": 20, "외부": 20, "파일": 10, "다운로드": 15, "샘플": 10, "구독": 10})
	overall := clampScore((privacy + credit + ai + security) / 4)
	status := func(score int) string {
		if score >= 70 {
			return "필수 검토"
		}
		if score >= 45 {
			return "주의"
		}
		return "낮음"
	}
	checklist := []map[string]any{
		{"item": "개인정보 포함 가능성", "status": status(privacy), "evidence": "식별자·준식별자·재식별 결합 가능성 확인"},
		{"item": "개인신용정보 해당성", "status": status(credit), "evidence": "신용도 판단, 상환능력, 거래/대출 이력 활용 여부 확인"},
		{"item": "가명정보 결합", "status": status(maxInt(privacy, credit)), "evidence": "외부 결합형 상품은 데이터전문기관 절차 검토"},
		{"item": "자동화평가 가능성", "status": status(ai), "evidence": "심사·평가 의사결정에 직접 쓰이면 설명 가능성과 인적 개입 필요"},
		{"item": "AI 활용 리스크", "status": status(ai), "evidence": "AI는 보조수단이며 최종 책임자를 지정"},
		{"item": "외부 제공 리스크", "status": status(security), "evidence": "API·파일·리포트 제공 범위와 목적 제한 필요"},
		{"item": "보안 리스크", "status": status(security), "evidence": "접근권한, 마스킹, 감사로그, 샘플 응답 비식별화 필요"},
	}
	notes := "법률 판단 자동화가 아니라 사전 체크리스트와 근거 수집 보조 결과입니다."
	if overall >= 70 {
		notes = "출시 전 법무·보안·신용정보 검토와 승인 워크플로우가 필요합니다."
	}
	return map[string]any{"privacy_score": privacy, "credit_score": credit, "ai_score": ai, "security_score": security, "overall_score": overall, "checklist": checklist, "review_notes": notes}
}

func buildPOCPlan(req factoryPOCRequest) map[string]any {
	title := firstNonEmpty(req.Title, req.ProductKey, "데이터 상품")
	dataScope := firstNonEmpty(req.DataScope, "최근 24개월 비식별 또는 집계 데이터, 목적 적합 최소 필드")
	successMetric := firstNonEmpty(req.SuccessMetric, "예측/분류 성능, 업무 적용성, 고객사 사용성, 규제 검토 통과")
	timeline := firstNonEmpty(req.Timeline, "4주: 범위 확정 1주, 샘플 적재 1주, 검증 1주, 결과 보고 1주")
	return map[string]any{
		"title":          title + " PoC 계획",
		"data_scope":     dataScope,
		"sample_period":  "최근 12~24개월",
		"method":         "비식별 샘플 생성 후 고객사 업무 시나리오 기준 오프라인 검증",
		"success_metric": successMetric,
		"timeline":       timeline,
		"participants":   []string{"데이터사업", "데이터오너", "법무/보안", "고객사 PoC 담당"},
		"approvals":      []string{"데이터 반출/열람 승인", "리스크 검토", "PoC 착수 승인"},
		"exit_criteria":  []string{"성공 지표 충족", "상용 과금 모델 합의", "보안/법무 이슈 해소"},
	}
}

func buildPriorityScore(req factoryScoringRequest) map[string]any {
	revenue := revenueScore(req.TargetCustomers, req.DataAssets, req.MarketNeed)
	risk := factoryFirstPositive(req.RiskScore, riskScoreFromText(req.Title+" "+strings.Join(req.DataAssets, " ")+" "+req.DeliveryMethod))
	fit := factoryFirstPositive(req.ImplementationFit, clampScore(75-len(req.DataAssets)*3))
	repeatability := clampScore(55 + len(req.TargetCustomers)*8)
	priority := clampScore((revenue*35 + fit*25 + repeatability*25 + (100-risk)*15) / 100)
	return map[string]any{
		"revenue_score": revenue, "implementation_fit": fit, "repeatability_score": repeatability,
		"risk_score": risk, "priority_score": priority,
		"decision": priorityDecision(priority, risk),
	}
}

func buildProposalPackage(p store.DataProduct, req factoryProposalRequest) map[string]any {
	target := firstNonEmpty(req.TargetCustomerType, strings.Join(p.TargetCustomers, ", "), "금융 고객사")
	name := firstNonEmpty(req.CustomerName, target)
	return map[string]any{
		"title":            name + " 대상 " + p.NameKO + " 제안 패키지",
		"product_key":      p.ProductKey,
		"target_customer":  target,
		"opening_message":  p.SalesPitch,
		"business_problem": firstNonEmpty(req.UseCase, p.Description),
		"expected_effect":  p.ExecutiveSummary,
		"required_data":    splitLoose(p.SourceRef),
		"poc_method":       "4주 PoC로 샘플 검증, 성공 지표 합의, 보안/법무 체크 병행",
		"security_terms":   []string{"목적 제한", "최소 데이터", "접근 로그", "샘플 응답 비식별화"},
		"commercial_model": firstNonEmpty(p.PricingModel, "PoC 후 월 구독 + 사용량 기반 과금"),
		"next_steps":       []string{"PoC 범위 워크숍", "데이터 항목 검토", "보안/법무 사전 질의", "견적/계약 모델 확정"},
	}
}

func compareProducts(productKey, text string, products []store.DataProduct) []map[string]any {
	type match struct {
		key, name, diff string
		score           int
	}
	target := factoryTokenSet(text)
	matches := []match{}
	for _, p := range products {
		if p.ProductKey == productKey {
			continue
		}
		candidate := factoryTokenSet(p.ProductKey + " " + p.NameKO + " " + p.Description + " " + p.ExecutiveSummary + " " + p.Differentiation)
		score := factoryOverlapScore(target, candidate)
		if score <= 0 && text != "" && strings.Contains(strings.ToLower(p.NameKO+p.Description), strings.ToLower(text)) {
			score = 35
		}
		if score > 0 {
			matches = append(matches, match{key: p.ProductKey, name: p.NameKO, score: score, diff: firstNonEmpty(p.Differentiation, "대상 고객, 제공 방식, 리스크 가드레일 기준으로 차별화 필요")})
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].score > matches[j].score })
	if len(matches) > 5 {
		matches = matches[:5]
	}
	out := []map[string]any{}
	for _, m := range matches {
		out = append(out, map[string]any{"product_key": m.key, "name": m.name, "similarity_score": m.score, "difference_summary": m.diff})
	}
	return out
}

func productIdeaIDs(ideas []store.ProductIdea) []string {
	ids := make([]string, 0, len(ideas))
	for _, idea := range ideas {
		ids = append(ids, idea.ID)
	}
	return ids
}

func compactPromptSummary(req factoryIdeaRequest) string {
	parts := []string{req.Industry, req.CustomerType, strings.Join(req.CustomerTypes, ","), req.MarketNeed, req.Purpose, strings.Join(req.DataAssets, ",")}
	return strings.TrimSpace(strings.Join(parts, " | "))
}

func fmtTitle(format, industry string) string {
	return strings.TrimSpace(strings.Replace(format, "%s", industry, 1))
}

var productKeyCleanRe = regexp.MustCompile(`[^a-z0-9]+`)

func productKeyFromTitle(title string) string {
	lower := strings.ToLower(title)
	base := productKeyCleanRe.ReplaceAllString(lower, "-")
	base = strings.Trim(base, "-")
	if base == "" || len([]rune(base)) < 3 {
		base = "data-product"
	}
	if len(base) > 48 {
		base = base[:48]
		base = strings.Trim(base, "-")
	}
	return "dw_" + base + "_" + factoryShortHash(title)[:8]
}

func shortProductName(title string) string {
	title = strings.TrimSpace(title)
	if len([]rune(title)) <= 24 {
		return title
	}
	r := []rune(title)
	return string(r[:24])
}

func factoryShortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func inferProductSourceType(delivery string) string {
	lower := strings.ToLower(delivery)
	switch {
	case strings.Contains(lower, "api"):
		return "api"
	case strings.Contains(lower, "리포트") || strings.Contains(lower, "report"):
		return "report"
	case strings.Contains(lower, "스코어") || strings.Contains(lower, "score"):
		return "score"
	case strings.Contains(lower, "세그먼트") || strings.Contains(lower, "segment"):
		return "segment"
	case strings.Contains(lower, "대시보드") || strings.Contains(lower, "dashboard"):
		return "dashboard"
	default:
		return "dataset"
	}
}

func inferSensitivity(req factoryDefineRequest) string {
	text := strings.ToLower(req.Title + " " + req.CustomerNeed + " " + strings.Join(req.DataAssets, " "))
	if strings.Contains(text, "신용") || strings.Contains(text, "대출") || strings.Contains(text, "연체") {
		return "personal_credit"
	}
	if strings.Contains(text, "개인") || strings.Contains(text, "고객") {
		return "pseudonymized"
	}
	if strings.Contains(text, "집계") || strings.Contains(text, "통계") {
		return "aggregated"
	}
	return "internal"
}

func riskScoreFromText(text string) int {
	return scoreByKeywords(strings.ToLower(text), 25, map[string]int{
		"개인": 15, "고객": 8, "신용": 25, "대출": 18, "연체": 20, "상환": 16, "보험": 10,
		"api": 8, "외부": 12, "ai": 8, "스코어": 12, "score": 12,
	})
}

func revenueScore(customers, assets []string, need string) int {
	score := 52 + len(customers)*8 + len(assets)*4
	if strings.TrimSpace(need) != "" {
		score += 8
	}
	return clampScore(score)
}

func scoreByKeywords(text string, base int, weights map[string]int) int {
	score := base
	for k, w := range weights {
		if strings.Contains(text, strings.ToLower(k)) {
			score += w
		}
	}
	return clampScore(score)
}

func clampScore(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func factoryFirstPositive(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func priorityDecision(priority, risk int) string {
	if risk >= 75 {
		return "리스크 검토 선행"
	}
	if priority >= 75 {
		return "우선 추진"
	}
	if priority >= 55 {
		return "PoC 검토"
	}
	return "보류 또는 재정의"
}

func splitLoose(value string) []string {
	out := []string{}
	for _, item := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '\n' || r == ';' || r == '|' }) {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func intValue(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}

func stringValue(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func factoryTokenSet(text string) map[string]bool {
	set := map[string]bool{}
	text = strings.ToLower(productKeyCleanRe.ReplaceAllString(text, " "))
	for _, tok := range strings.Fields(text) {
		if len(tok) >= 2 {
			set[tok] = true
		}
	}
	return set
}

func factoryOverlapScore(a, b map[string]bool) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	hits := 0
	for k := range a {
		if b[k] {
			hits++
		}
	}
	denom := len(a)
	if len(b) < denom {
		denom = len(b)
	}
	return clampScore(hits * 100 / denom)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func _factoryNow() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
