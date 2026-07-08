package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"dataworks/internal/store"
)

func (s *Server) handleDataWorksAssetByKey(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	parts := dataWorksPathParts(r.URL.Path, "/admin/dataworks/assets/")
	if len(parts) < 2 {
		writeOpenAIError(w, http.StatusNotFound, "asset action required", "invalid_request_error", "not_found")
		return
	}
	assetKey := parts[0]
	switch {
	case len(parts) == 3 && parts[1] == "readiness" && parts[2] == "check":
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
		score := buildDataAssetReadinessScore(asset, adminID(r))
		if err := s.db.UpsertDataAssetReadinessScore(r.Context(), score); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "readiness_failed")
			return
		}
		_ = s.db.UpsertAssetReadinessScore(r.Context(), bridgeAssetReadinessScore(score))
		s.auditAdmin(r, "dataworks.asset.readiness.check", "", auditJSON(map[string]any{"asset_key": assetKey, "overall_score": score.OverallScore}))
		writeJSON(w, http.StatusOK, map[string]any{"readiness": score})
	case len(parts) == 2 && parts[1] == "lineage":
		if r.Method != http.MethodGet {
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
		graph, err := s.buildDataWorksGraph(r.Context(), assetKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "lineage_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"asset": asset, "lineage": graph})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown asset action", "invalid_request_error", "not_found")
	}
}

func (s *Server) handleDataWorksProductByKey(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	parts := dataWorksPathParts(r.URL.Path, "/admin/dataworks/products/")
	if len(parts) < 2 {
		writeOpenAIError(w, http.StatusNotFound, "product action required", "invalid_request_error", "not_found")
		return
	}
	productKey := parts[0]
	product, ok, err := s.db.GetDataProduct(r.Context(), productKey)
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "product_failed")
		return
	}
	if !ok {
		writeOpenAIError(w, http.StatusNotFound, "product not found", "invalid_request_error", "not_found")
		return
	}

	switch {
	case len(parts) == 3 && parts[1] == "canvas" && parts[2] == "generate":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		canvas := buildProductCanvas(product, adminID(r))
		if err := s.db.UpsertProductCanvas(r.Context(), canvas); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "canvas_failed")
			return
		}
		s.auditAdmin(r, "dataworks.product.canvas.generate", "", auditJSON(map[string]any{"product_key": productKey}))
		writeJSON(w, http.StatusOK, map[string]any{"canvas": canvas})
	case len(parts) == 2 && parts[1] == "evidence":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		evidence, err := s.db.ListProductEvidence(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "evidence_failed")
			return
		}
		persisted := true
		if len(evidence) == 0 {
			persisted = false
			evidence = s.buildProductEvidencePack(r.Context(), product, adminID(r))
			_ = s.db.ReplaceProductEvidencePack(r.Context(), product.ProductKey, evidence)
		}
		_ = s.ensureDataWorksEvidencePack(r.Context(), product, adminID(r))
		writeJSON(w, http.StatusOK, map[string]any{"evidence": evidence, "persisted": persisted})
	case len(parts) == 2 && parts[1] == "regulatory-trace":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		rows, err := s.regulatoryTraceFromRequest(r, product)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_body")
			return
		}
		if err := s.db.ReplaceRegulatoryTrace(r.Context(), product.ProductKey, rows); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "trace_failed")
			return
		}
		s.syncApprovalTracesFromRegulatoryTrace(r.Context(), product, rows, adminID(r))
		s.auditAdmin(r, "dataworks.product.regulatory_trace", "", auditJSON(map[string]any{"product_key": productKey, "rows": len(rows)}))
		writeJSON(w, http.StatusOK, map[string]any{"trace": rows, "summary": regulatoryTraceSummary(rows)})
	case len(parts) == 2 && parts[1] == "api-contract":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		contract := buildProductAPIContract(product, adminID(r))
		version, err := s.db.InsertProductAPIContract(r.Context(), contract)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "api_contract_failed")
			return
		}
		contract.Version = version
		s.auditAdmin(r, "dataworks.product.api_contract", "", auditJSON(map[string]any{"product_key": productKey, "version": version}))
		writeJSON(w, http.StatusOK, map[string]any{"api_contract": contract})
	case len(parts) == 2 && parts[1] == "mock":
		if r.Method != http.MethodPost {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		mock, logRow, err := buildMockAPIResponse(r, product)
		if err != nil {
			writeOpenAIError(w, http.StatusBadRequest, err.Error(), "invalid_request_error", "invalid_body")
			return
		}
		logRow.ID = newID("mock")
		if err := s.db.InsertMockAPILog(r.Context(), logRow); err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "mock_log_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"mock_response": mock})
	case len(parts) == 2 && parts[1] == "funnel":
		if r.Method != http.MethodGet {
			writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
			return
		}
		funnel, err := s.db.DataWorksFunnel(r.Context(), product.ProductKey)
		if err != nil {
			writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "funnel_failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"product_key": product.ProductKey, "funnel": funnel, "rates": funnelRates(funnel)})
	default:
		writeOpenAIError(w, http.StatusNotFound, "unknown product action", "invalid_request_error", "not_found")
	}
}

func (s *Server) handleDataWorksProposalByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	parts := dataWorksPathParts(r.URL.Path, "/admin/dataworks/proposals/")
	if len(parts) != 2 || parts[1] != "feedback" {
		writeOpenAIError(w, http.StatusNotFound, "proposal feedback action required", "invalid_request_error", "not_found")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	proposalID := parts[0]
	var req struct {
		ProductKey         string `json:"product_key"`
		CustomerType       string `json:"customer_type"`
		CustomerNameMasked string `json:"customer_name_masked"`
		Result             string `json:"result"`
		Reason             string `json:"reason"`
		NextAction         string `json:"next_action"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if strings.TrimSpace(req.Result) == "" {
		writeOpenAIError(w, http.StatusBadRequest, "result is required", "invalid_request_error", "missing_fields")
		return
	}
	if proposal, ok, err := s.db.GetProposalPackage(r.Context(), proposalID); err == nil && ok {
		req.ProductKey = firstNonEmpty(req.ProductKey, proposal.ProductKey)
		req.CustomerType = firstNonEmpty(req.CustomerType, proposal.TargetCustomerType)
	}
	fb := store.ProposalFeedback{
		ID: newID("pfb"), ProposalID: proposalID, ProductKey: strings.TrimSpace(req.ProductKey),
		CustomerType: strings.TrimSpace(req.CustomerType), CustomerNameMasked: strings.TrimSpace(req.CustomerNameMasked),
		Result: strings.TrimSpace(req.Result), Reason: strings.TrimSpace(req.Reason), NextAction: strings.TrimSpace(req.NextAction),
		CreatedBy: adminID(r),
	}
	if err := s.db.InsertProposalFeedback(r.Context(), fb); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "feedback_failed")
		return
	}
	s.applyProposalFeedbackScore(r.Context(), fb.ProductKey, fb.Result, adminID(r))
	s.auditAdmin(r, "dataworks.proposal.feedback", "", auditJSON(map[string]any{"proposal_id": proposalID, "result": fb.Result}))
	writeJSON(w, http.StatusOK, map[string]any{"feedback": fb})
}

func (s *Server) handleDataWorksPOCByID(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	parts := dataWorksPathParts(r.URL.Path, "/admin/dataworks/poc/")
	if len(parts) != 2 || parts[1] != "outcome" {
		writeOpenAIError(w, http.StatusNotFound, "poc outcome action required", "invalid_request_error", "not_found")
		return
	}
	if r.Method != http.MethodPost {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	pocID := parts[0]
	var req struct {
		ProductKey       string `json:"product_key"`
		Success          bool   `json:"success"`
		MetricResult     string `json:"metric_result"`
		CustomerFeedback string `json:"customer_feedback"`
		ConversionStatus string `json:"conversion_status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeOpenAIError(w, http.StatusBadRequest, "invalid JSON body", "invalid_request_error", "invalid_body")
		return
	}
	if poc, ok, err := s.db.GetProductPOCPlan(r.Context(), pocID); err == nil && ok {
		req.ProductKey = firstNonEmpty(req.ProductKey, poc.ProductKey)
	}
	outcome := store.POCOutcome{
		ID: newID("pocout"), POCID: pocID, ProductKey: strings.TrimSpace(req.ProductKey), Success: req.Success,
		MetricResult: strings.TrimSpace(req.MetricResult), CustomerFeedback: strings.TrimSpace(req.CustomerFeedback),
		ConversionStatus: firstNonEmpty(strings.TrimSpace(req.ConversionStatus), "recorded"), CreatedBy: adminID(r),
	}
	if err := s.db.InsertPOCOutcome(r.Context(), outcome); err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "poc_outcome_failed")
		return
	}
	s.applyPOCOutcomeScore(r.Context(), outcome, adminID(r))
	s.auditAdmin(r, "dataworks.poc.outcome", "", auditJSON(map[string]any{"poc_id": pocID, "success": outcome.Success, "conversion_status": outcome.ConversionStatus}))
	writeJSON(w, http.StatusOK, map[string]any{"outcome": outcome})
}

func (s *Server) handleDataWorksPortfolioGraph(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	graph, err := s.buildDataWorksGraph(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "graph_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"graph": graph})
}

func (s *Server) handleDataWorksAnalyticsFunnel(w http.ResponseWriter, r *http.Request) {
	if !s.authorizeAdmin(r) {
		writeOpenAIError(w, http.StatusUnauthorized, "invalid admin token", "invalid_request_error", "invalid_api_key")
		return
	}
	if r.Method != http.MethodGet {
		writeOpenAIError(w, http.StatusMethodNotAllowed, "method not allowed", "invalid_request_error", "method_not_allowed")
		return
	}
	funnel, err := s.db.DataWorksFunnel(r.Context(), "")
	if err != nil {
		writeOpenAIError(w, http.StatusInternalServerError, err.Error(), "server_error", "funnel_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"funnel": funnel, "rates": funnelRates(funnel)})
}

func dataWorksPathParts(path string, prefix string) []string {
	rest := strings.Trim(strings.TrimPrefix(path, prefix), "/")
	if rest == "" {
		return nil
	}
	raw := strings.Split(rest, "/")
	parts := make([]string, 0, len(raw))
	for _, item := range raw {
		if item == "" {
			continue
		}
		decoded, err := url.PathUnescape(item)
		if err != nil {
			decoded = item
		}
		parts = append(parts, decoded)
	}
	return parts
}

func buildDataAssetReadinessScore(asset store.DataAsset, actor string) store.DataAssetReadinessScore {
	columns := splitLoose(asset.ColumnsSummary)
	quality := clampScore(45 + minInt(len(columns)*5, 30))
	if strings.TrimSpace(asset.ColumnsSummary) != "" {
		quality += 10
	}
	freshness := freshnessScore(asset.RefreshCycle)
	owner := 35
	if strings.TrimSpace(asset.Owner) != "" {
		owner = 90
	}
	metadata := 30
	if strings.TrimSpace(asset.Name) != "" {
		metadata += 15
	}
	if strings.TrimSpace(asset.Domain) != "" {
		metadata += 15
	}
	if strings.TrimSpace(asset.ColumnsSummary) != "" {
		metadata += 25
	}
	if strings.TrimSpace(asset.RefreshCycle) != "" {
		metadata += 10
	}
	metadata = clampScore(metadata)
	sensitivity := sensitivityReadinessScore(asset.Sensitivity)
	approval := approvalReadinessScore(asset)
	sample := sampleReadinessScore(asset.Sensitivity)
	overall := clampScore((quality*20 + freshness*15 + owner*15 + metadata*15 + sensitivity*10 + approval*15 + sample*10) / 100)
	blockers := []string{}
	if strings.TrimSpace(asset.Owner) == "" {
		blockers = append(blockers, "owner_required")
	}
	if strings.TrimSpace(asset.ColumnsSummary) == "" {
		blockers = append(blockers, "columns_summary_required")
	}
	if strings.TrimSpace(asset.RefreshCycle) == "" {
		blockers = append(blockers, "refresh_cycle_required")
	}
	if isHighSensitivity(asset.Sensitivity) {
		blockers = append(blockers, "external_review_required")
	}
	status := "Need Quality Check"
	switch {
	case strings.TrimSpace(asset.Owner) == "":
		status = "Need Owner"
	case isHighSensitivity(asset.Sensitivity):
		status = "External Review"
	case strings.EqualFold(asset.Sensitivity, "restricted"):
		status = "Restricted"
	case overall >= 80:
		status = "Ready"
	}
	return store.DataAssetReadinessScore{
		AssetKey: asset.AssetKey, QualityScore: clampScore(quality), FreshnessScore: freshness, OwnerScore: owner,
		MetadataScore: metadata, SensitivityScore: sensitivity, ApprovalScore: approval, SampleScore: sample,
		OverallScore: overall, Status: status, Blockers: blockers, CheckedBy: actor,
	}
}

func bridgeAssetReadinessScore(score store.DataAssetReadinessScore) store.AssetReadinessScore {
	overall := score.OverallScore
	if strings.EqualFold(score.Status, "External Review") && overall < 70 {
		overall = 70
	}
	return store.AssetReadinessScore{
		AssetKey:              score.AssetKey,
		SchemaScore:           score.MetadataScore,
		FreshnessScore:        score.FreshnessScore,
		SampleScore:           score.SampleScore,
		MissingnessScore:      score.QualityScore,
		SensitivityScore:      score.SensitivityScore,
		ExternalSharingScore:  score.ApprovalScore,
		APIReadinessScore:     score.QualityScore,
		BillingReadinessScore: score.OwnerScore,
		OverallScore:          overall,
		Notes:                 strings.Join(score.Blockers, ","),
		UpdatedBy:             score.CheckedBy,
	}
}

func freshnessScore(refresh string) int {
	lower := strings.ToLower(refresh)
	switch {
	case strings.Contains(lower, "hour") || strings.Contains(refresh, "시간") || strings.Contains(lower, "daily") || strings.Contains(refresh, "일"):
		return 95
	case strings.Contains(lower, "week") || strings.Contains(refresh, "주"):
		return 85
	case strings.Contains(lower, "month") || strings.Contains(refresh, "월"):
		return 70
	case strings.Contains(lower, "quarter") || strings.Contains(refresh, "분기"):
		return 55
	case strings.TrimSpace(refresh) == "":
		return 45
	default:
		return 65
	}
}

func sensitivityReadinessScore(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "public", "aggregated":
		return 92
	case "internal", "":
		return 82
	case "pseudonymized":
		return 70
	case "restricted":
		return 58
	case "personal_credit":
		return 45
	default:
		return 65
	}
}

func approvalReadinessScore(asset store.DataAsset) int {
	score := 45
	if strings.TrimSpace(asset.Owner) != "" {
		score += 25
	}
	if !isHighSensitivity(asset.Sensitivity) {
		score += 15
	}
	return clampScore(score)
}

func sampleReadinessScore(sensitivity string) int {
	switch strings.ToLower(strings.TrimSpace(sensitivity)) {
	case "personal_credit":
		return 35
	case "restricted":
		return 50
	case "pseudonymized":
		return 65
	default:
		return 82
	}
}

func isHighSensitivity(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return lower == "personal_credit" || lower == "restricted"
}

func buildProductCanvas(p store.DataProduct, actor string) store.ProductCanvas {
	return store.ProductCanvas{
		ProductKey:        p.ProductKey,
		CustomerProblem:   firstNonEmpty(p.Description, p.ExecutiveSummary, "고객 업무 문제를 데이터 상품으로 검증해야 합니다."),
		TargetSegment:     firstNonEmpty(strings.Join(p.TargetCustomers, ", "), strings.Join(p.TargetIndustries, ", "), "금융 고객군"),
		ValueProposition:  firstNonEmpty(p.SalesPitch, p.ExecutiveSummary, "내부 데이터 자산을 PoC 가능한 상품 패키지로 전환합니다."),
		DataInputs:        productSourceAssets(p),
		DeliveryModel:     firstNonEmpty(p.SourceType, "api"),
		PricingHypothesis: firstNonEmpty(p.PricingModel, "PoC 후 월 구독 + 사용량 기반 과금"),
		RiskPosture:       productRiskPosture(p),
		POCSuccessMetric:  firstNonEmpty(p.POCPlan, "4주 PoC에서 성능, 업무 적용성, 보안/법무 검토 통과 여부를 확인"),
		CreatedBy:         actor,
	}
}

func (s *Server) upsertGeneratedProductCanvas(ctx context.Context, p store.DataProduct, actor string) error {
	return s.db.UpsertProductCanvas(ctx, buildProductCanvas(p, actor))
}

func (s *Server) refreshProductEvidencePack(ctx context.Context, p store.DataProduct, actor string) error {
	return s.db.ReplaceProductEvidencePack(ctx, p.ProductKey, s.buildProductEvidencePack(ctx, p, actor))
}

func (s *Server) buildProductEvidencePack(ctx context.Context, p store.DataProduct, actor string) []store.ProductEvidence {
	def, defOK, _ := s.db.LatestProductDefinition(ctx, p.ProductKey)
	risk, riskOK, _ := s.db.LatestProductRiskReview(ctx, p.ProductKey)
	poc, pocOK, _ := s.db.LatestProductPOCPlan(ctx, p.ProductKey)
	evidence := []store.ProductEvidence{
		{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "data_assets", SourceRef: p.SourceRef, Summary: "상품 정의에 사용되는 데이터 자산: " + strings.Join(productSourceAssets(p), ", "), ConfidenceScore: 82, CreatedBy: actor},
		{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "customer_need", SourceRef: p.ProductKey, Summary: firstNonEmpty(p.Description, p.ExecutiveSummary), ConfidenceScore: 74, CreatedBy: actor},
		{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "differentiation", SourceRef: p.ProductKey, Summary: firstNonEmpty(p.Differentiation, "기존 상품과 대상 고객, 제공 방식, 리스크 가드레일 기준으로 차별화 필요"), ConfidenceScore: 68, CreatedBy: actor},
	}
	if defOK {
		evidence = append(evidence, store.ProductEvidence{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "definition_version", SourceRef: def.ID, Summary: "최신 상품 정의서 버전 " + itoaProxy(def.Version) + " 기준", ConfidenceScore: 88, CreatedBy: actor})
	}
	if riskOK {
		evidence = append(evidence, store.ProductEvidence{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "risk_basis", SourceRef: risk.ID, Summary: "리스크 점검 종합 점수 " + itoaProxy(risk.OverallScore) + "점: " + risk.ReviewNotes, ConfidenceScore: 78, CreatedBy: actor})
	}
	if pocOK {
		evidence = append(evidence, store.ProductEvidence{ID: newID("evid"), ProductKey: p.ProductKey, EvidenceType: "poc_success_metric", SourceRef: poc.ID, Summary: firstNonEmpty(poc.SuccessMetric, "PoC 성공지표 미정"), ConfidenceScore: 72, CreatedBy: actor})
	}
	return evidence
}

func (s *Server) ensureDataWorksEvidencePack(ctx context.Context, p store.DataProduct, actor string) error {
	if _, ok, err := s.db.GetEvidencePack(ctx, p.ProductKey); err != nil || ok {
		return err
	}
	packJSON, err := s.buildEvidencePackJSON(ctx, p)
	if err != nil {
		return err
	}
	return s.db.UpsertEvidencePack(ctx, store.EvidencePack{
		ProductKey: p.ProductKey,
		PackJSON:   packJSON,
		CreatedBy:  actor,
	})
}

func (s *Server) syncApprovalTracesFromRegulatoryTrace(ctx context.Context, p store.DataProduct, rows []store.RegulatoryTrace, actor string) {
	steps := map[string]string{
		"data_owner_approval": "data_owner",
		"legal_review":        "legal",
		"compliance_review":   "compliance",
	}
	for _, row := range rows {
		step, ok := steps[strings.ToLower(strings.TrimSpace(row.RiskDomain))]
		if !ok {
			continue
		}
		status := "pending"
		switch strings.ToLower(strings.TrimSpace(row.Decision)) {
		case "approved":
			status = "approved"
		case "waived", "not_required":
			status = "waived"
		case "rejected", "blocked", "denied":
			status = "rejected"
		}
		_ = s.db.UpsertApprovalTrace(ctx, store.ApprovalTrace{
			ID:          "appr_" + p.ProductKey + "_" + step,
			ProductKey:  p.ProductKey,
			Step:        step,
			Status:      status,
			Required:    true,
			EvidenceRef: row.ID,
			Notes:       row.Evidence,
			DecidedBy:   firstNonEmpty(strings.TrimSpace(row.Reviewer), actor),
		})
	}
}

func (s *Server) regulatoryTraceFromRequest(r *http.Request, p store.DataProduct) ([]store.RegulatoryTrace, error) {
	var req struct {
		Trace []store.RegulatoryTrace `json:"trace"`
		Rows  []store.RegulatoryTrace `json:"rows"`
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, err
		}
	}
	rows := req.Trace
	if len(rows) == 0 {
		rows = req.Rows
	}
	if len(rows) == 0 {
		return s.buildRegulatoryTrace(r.Context(), p, adminID(r)), nil
	}
	for i := range rows {
		rows[i].ID = firstNonEmpty(rows[i].ID, newID("rtrace"))
		rows[i].ProductKey = p.ProductKey
		rows[i].Reviewer = firstNonEmpty(strings.TrimSpace(rows[i].Reviewer), adminID(r))
		rows[i].RiskDomain = strings.TrimSpace(rows[i].RiskDomain)
		rows[i].Decision = firstNonEmpty(strings.TrimSpace(rows[i].Decision), "pending")
	}
	return rows, nil
}

func (s *Server) buildRegulatoryTrace(ctx context.Context, p store.DataProduct, actor string) []store.RegulatoryTrace {
	risk, ok, _ := s.db.LatestProductRiskReview(ctx, p.ProductKey)
	overall := p.RiskScore
	if ok && risk.OverallScore > overall {
		overall = risk.OverallScore
	}
	decision := func(score int) string {
		if score >= 70 {
			return "requires_review"
		}
		if score >= 45 {
			return "conditional"
		}
		return "approved"
	}
	baseEvidence := "product_sensitivity=" + p.Sensitivity + "; risk_score=" + itoaProxy(overall)
	rows := []store.RegulatoryTrace{
		traceRow(p.ProductKey, "privacy", "개인정보 또는 준식별자 포함 가능성이 있는가?", riskAnswer(ok, risk.PrivacyScore), baseEvidence, decision(scoreOrDefault(ok, risk.PrivacyScore, overall)), actor),
		traceRow(p.ProductKey, "credit_information", "개인신용정보 또는 신용 판단 정보에 해당하는가?", riskAnswer(ok, risk.CreditScore), baseEvidence, decision(scoreOrDefault(ok, risk.CreditScore, overall)), actor),
		traceRow(p.ProductKey, "pseudonymization", "가명/익명/집계 처리 설계가 필요한가?", yesNo(isHighSensitivity(p.Sensitivity) || strings.EqualFold(p.Sensitivity, "pseudonymized")), "source_ref="+p.SourceRef, decision(overall), actor),
		traceRow(p.ProductKey, "external_sharing", "외부 제공 또는 고객사 반출 가능성이 있는가?", yesNo(p.SourceType == "api" || strings.Contains(strings.ToLower(p.SourceType), "report")), "delivery_model="+p.SourceType, decision(scoreOrDefault(ok, risk.SecurityScore, overall)), actor),
		traceRow(p.ProductKey, "ai_usage", "AI 생성 산출물 또는 자동화 평가 보조에 쓰이는가?", riskAnswer(ok, risk.AIScore), "AI output is a draft; final owner approval required", decision(scoreOrDefault(ok, risk.AIScore, overall)), actor),
		traceRow(p.ProductKey, "security", "접근권한, 마스킹, 감사로그 통제가 필요한가?", riskAnswer(ok, risk.SecurityScore), "api_contract/mock/proposal exports must be audited", decision(scoreOrDefault(ok, risk.SecurityScore, overall)), actor),
	}
	approvalDecision := "not_required"
	if overall >= 70 || isHighSensitivity(p.Sensitivity) {
		approvalDecision = "required"
	}
	rows = append(rows,
		traceRow(p.ProductKey, "legal_review", "법무 승인이 필요한 고위험 상품인가?", yesNo(approvalDecision == "required"), baseEvidence, approvalDecision, actor),
		traceRow(p.ProductKey, "compliance_review", "준법 승인이 필요한 고위험 상품인가?", yesNo(approvalDecision == "required"), baseEvidence, approvalDecision, actor),
		traceRow(p.ProductKey, "data_owner_approval", "데이터오너 제공 승인 확인이 필요한가?", yesNo(true), "owner="+p.Owner+"; source_ref="+p.SourceRef, approvalDecision, actor),
	)
	return rows
}

func traceRow(productKey, domain, question, answer, evidence, decision, reviewer string) store.RegulatoryTrace {
	return store.RegulatoryTrace{ID: newID("rtrace"), ProductKey: productKey, RiskDomain: domain, Question: question, Answer: answer, Evidence: evidence, Decision: decision, Reviewer: reviewer}
}

func riskAnswer(ok bool, score int) string {
	if !ok {
		return "리스크 점검 전: 상품 메타데이터 기준 사전 판단"
	}
	if score >= 70 {
		return "높음"
	}
	if score >= 45 {
		return "중간"
	}
	return "낮음"
}

func scoreOrDefault(ok bool, score int, fallback int) int {
	if ok && score > 0 {
		return score
	}
	return fallback
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func regulatoryTraceSummary(rows []store.RegulatoryTrace) map[string]any {
	decisions := map[string]int{}
	blockers := []string{}
	for _, row := range rows {
		decision := strings.ToLower(strings.TrimSpace(row.Decision))
		decisions[decision]++
		if decision == "required" || decision == "requires_review" || decision == "pending" {
			blockers = append(blockers, row.RiskDomain)
		}
	}
	sort.Strings(blockers)
	return map[string]any{"decisions": decisions, "publish_blockers": blockers}
}

func buildProductAPIContract(p store.DataProduct, actor string) store.APIContract {
	path := "/v1/data-products/" + p.ProductKey + "/query"
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":   p.NameKO + " API",
			"version": "1.0.0",
		},
		"paths": map[string]any{
			path: map[string]any{
				"post": map[string]any{
					"summary":  p.Description,
					"security": []any{map[string]any{"bearerAuth": []any{}}},
					"requestBody": map[string]any{"required": true, "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"customer_segment": map[string]any{"type": "string"},
							"period":           map[string]any{"type": "string", "example": "2026-06"},
							"purpose":          map[string]any{"type": "string"},
						},
						"required": []string{"customer_segment", "period", "purpose"},
					}}}},
					"responses": map[string]any{"200": map[string]any{"description": "Mockable Data Works response", "content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"product_key":  map[string]any{"type": "string"},
							"score":        map[string]any{"type": "number"},
							"risk_band":    map[string]any{"type": "string"},
							"drivers":      map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"generated_at": map[string]any{"type": "string", "format": "date-time"},
						},
					}}}}},
					"x-dataworks": map[string]any{"source_assets": productSourceAssets(p), "sensitivity": p.Sensitivity, "owner": p.Owner},
				},
			},
		},
		"components": map[string]any{"securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}}},
	}
	encoded, _ := json.Marshal(spec)
	return store.APIContract{
		ID: newID("apic"), ProductKey: p.ProductKey, OpenAPIJSON: string(encoded),
		SLAPolicy:     "PoC 기본 SLA: 월 가용성 목표 99.0%, 상용 전 별도 계약",
		RateLimit:     "PoC token 기준 분당 60회, 일 10,000회",
		MaskingPolicy: "개인 단위 원천값 제외, 집계/가명/마스킹 응답 우선",
		CreatedBy:     actor,
	}
}

func buildMockAPIResponse(r *http.Request, p store.DataProduct) (map[string]any, store.MockAPILog, error) {
	start := time.Now()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, store.MockAPILog{}, err
	}
	var req map[string]any
	if len(strings.TrimSpace(string(body))) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, store.MockAPILog{}, err
		}
	}
	customerType, _ := req["customer_type"].(string)
	if customerType == "" {
		customerType, _ = req["customer_segment"].(string)
	}
	score := clampScore(firstPositiveInt(p.RevenueScore, 65) - maxInt(p.RiskScore-60, 0)/2)
	riskBand := "low"
	if p.RiskScore >= 70 {
		riskBand = "high"
	} else if p.RiskScore >= 45 {
		riskBand = "medium"
	}
	response := map[string]any{
		"product_key":   p.ProductKey,
		"customer_type": firstNonEmpty(customerType, strings.Join(p.TargetCustomers, ", ")),
		"score":         score,
		"risk_band":     riskBand,
		"drivers":       []string{"mock_response", "source_assets:" + strings.Join(productSourceAssets(p), ","), "risk_score:" + itoaProxy(p.RiskScore)},
		"generated_at":  time.Now().UTC().Format(time.RFC3339Nano),
		"disclaimer":    "PoC sandbox mock response; not production data.",
	}
	logRow := store.MockAPILog{ProductKey: p.ProductKey, CustomerType: firstNonEmpty(customerType, "unknown"), RequestHash: factoryShortHash(string(body)), LatencyMS: int(time.Since(start) / time.Millisecond)}
	return response, logRow, nil
}

func firstPositiveInt(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func (s *Server) buildDataWorksGraph(ctx context.Context, focusAsset string) (map[string]any, error) {
	assets, err := s.db.ListDataAssets(ctx)
	if err != nil {
		return nil, err
	}
	products, err := s.db.ListDataProducts(ctx, "")
	if err != nil {
		return nil, err
	}
	feedback, _ := s.db.ListProposalFeedback(ctx, "")
	outcomes, _ := s.db.ListPOCOutcomes(ctx, "")
	nodes := []map[string]any{}
	edges := []map[string]any{}
	assetAllowed := map[string]bool{}
	for _, asset := range assets {
		if focusAsset != "" && asset.AssetKey != focusAsset {
			continue
		}
		assetAllowed[asset.AssetKey] = true
		nodes = append(nodes, map[string]any{"id": "asset:" + asset.AssetKey, "type": "asset", "label": firstNonEmpty(asset.Name, asset.AssetKey), "domain": asset.Domain, "sensitivity": asset.Sensitivity})
	}
	productAllowed := map[string]bool{}
	for _, product := range products {
		sourceAssets := productSourceAssets(product)
		include := focusAsset == ""
		for _, asset := range sourceAssets {
			if focusAsset != "" && asset == focusAsset {
				include = true
			}
		}
		if !include {
			continue
		}
		productAllowed[product.ProductKey] = true
		nodes = append(nodes, map[string]any{"id": "product:" + product.ProductKey, "type": "product", "label": firstNonEmpty(product.NameKO, product.ProductKey), "status": product.Status, "risk_score": product.RiskScore, "revenue_score": product.RevenueScore})
		for _, asset := range sourceAssets {
			if focusAsset == "" || assetAllowed[asset] {
				edges = append(edges, map[string]any{"from": "asset:" + asset, "to": "product:" + product.ProductKey, "relation_type": "feeds", "weight": 1})
			}
		}
	}
	for _, fb := range feedback {
		if !productAllowed[fb.ProductKey] {
			continue
		}
		id := "feedback:" + fb.ID
		nodes = append(nodes, map[string]any{"id": id, "type": "proposal_feedback", "label": fb.Result, "customer_type": fb.CustomerType})
		edges = append(edges, map[string]any{"from": "product:" + fb.ProductKey, "to": id, "relation_type": "proposal_feedback", "weight": 1})
	}
	for _, outcome := range outcomes {
		if !productAllowed[outcome.ProductKey] {
			continue
		}
		id := "poc:" + outcome.ID
		nodes = append(nodes, map[string]any{"id": id, "type": "poc_outcome", "label": outcome.ConversionStatus, "success": outcome.Success})
		edges = append(edges, map[string]any{"from": "product:" + outcome.ProductKey, "to": id, "relation_type": "poc_outcome", "weight": 1})
	}
	return map[string]any{"nodes": nodes, "edges": edges}, nil
}

func funnelRates(f store.DataWorksFunnel) map[string]any {
	rate := func(n, d int64) float64 {
		if d <= 0 {
			return 0
		}
		return float64(n) / float64(d)
	}
	return map[string]any{
		"definition_to_risk_review": rate(f.RiskReviews, f.Definitions),
		"definition_to_proposal":    rate(f.Proposals, f.Definitions),
		"proposal_feedback_rate":    rate(f.ProposalFeedback, f.Proposals),
		"poc_completion_rate":       rate(f.POCOutcomes, f.POCPlans),
		"published_rate":            rate(f.Published, f.Definitions),
	}
}

func productSourceAssets(p store.DataProduct) []string {
	assets := splitLoose(p.SourceRef)
	if len(assets) == 0 {
		return []string{}
	}
	return assets
}

func productRiskPosture(p store.DataProduct) string {
	if p.RiskScore >= 70 || isHighSensitivity(p.Sensitivity) {
		return "high: legal/compliance/data-owner approval required before publish"
	}
	if p.RiskScore >= 45 {
		return "medium: conditional approval and masking review required"
	}
	return "low: standard review"
}

func (s *Server) legacyDataWorksPublishGate(ctx context.Context, p store.DataProduct) []string {
	blockers := []string{}
	highRisk := p.RiskScore >= 70 || isHighSensitivity(p.Sensitivity)
	for _, assetKey := range productSourceAssets(p) {
		score, ok, _ := s.db.LatestDataAssetReadinessScore(ctx, assetKey)
		if highRisk && !ok {
			blockers = append(blockers, "asset_readiness_missing:"+assetKey)
			continue
		}
		if ok && (score.Status == "Need Owner" || score.OverallScore < 50) {
			blockers = append(blockers, "asset_not_ready:"+assetKey+":"+score.Status)
		}
	}
	if highRisk {
		trace, _ := s.db.ListRegulatoryTrace(ctx, p.ProductKey)
		approved := map[string]bool{}
		for _, row := range trace {
			if strings.EqualFold(row.Decision, "approved") {
				approved[row.RiskDomain] = true
			}
		}
		for _, domain := range []string{"legal_review", "compliance_review", "data_owner_approval"} {
			if !approved[domain] {
				blockers = append(blockers, "approval_required:"+domain)
			}
		}
	}
	sort.Strings(blockers)
	return blockers
}

func (s *Server) applyProposalFeedbackScore(ctx context.Context, productKey string, result string, actor string) {
	if strings.TrimSpace(productKey) == "" {
		return
	}
	p, ok, err := s.db.GetDataProduct(ctx, productKey)
	if err != nil || !ok {
		return
	}
	switch strings.ToLower(strings.TrimSpace(result)) {
	case "won", "accepted", "poc", "poc_converted", "interested":
		p.RevenueScore = clampScore(p.RevenueScore + 5)
	case "lost", "rejected", "no_interest":
		p.RevenueScore = clampScore(p.RevenueScore - 5)
	default:
		return
	}
	p.UpdatedBy = actor
	_ = s.db.UpsertDataProduct(ctx, p)
}

func (s *Server) applyPOCOutcomeScore(ctx context.Context, outcome store.POCOutcome, actor string) {
	if strings.TrimSpace(outcome.ProductKey) == "" {
		return
	}
	p, ok, err := s.db.GetDataProduct(ctx, outcome.ProductKey)
	if err != nil || !ok {
		return
	}
	if outcome.Success {
		p.RevenueScore = clampScore(p.RevenueScore + 8)
		if strings.EqualFold(outcome.ConversionStatus, "contract_candidate") && p.Status == "approved" {
			p.Status = "published"
		}
	} else {
		p.RevenueScore = clampScore(p.RevenueScore - 4)
	}
	p.UpdatedBy = actor
	_ = s.db.UpsertDataProduct(ctx, p)
}
