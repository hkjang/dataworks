package dataworks

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"dataworks/internal/store"
)

const DefaultReadinessThreshold = 70

var RequiredPublishApprovals = []string{"data_owner", "legal", "compliance"}

type PublishGateResult struct {
	ProductKey             string                      `json:"product_key"`
	StrictGate             bool                        `json:"strict_gate"`
	Allowed                bool                        `json:"allowed"`
	MinimumReadiness       int                         `json:"minimum_readiness"`
	RequiredApprovals      []string                    `json:"required_approvals"`
	ApprovalStatus         map[string]string           `json:"approval_status"`
	MissingApprovals       []string                    `json:"missing_approvals"`
	MissingEvidence        []string                    `json:"missing_evidence"`
	BlockedReasons         []string                    `json:"blocked_reasons"`
	Warnings               []string                    `json:"warnings"`
	AssetReadiness         []store.AssetReadinessScore `json:"asset_readiness"`
	CheckedAt              string                      `json:"checked_at"`
	Metadata               map[string]any              `json:"metadata,omitempty"`
	QualityPassed          bool                        `json:"quality_passed"`
	RiskReviewed           bool                        `json:"risk_reviewed"`
	APIContractConfigured  bool                        `json:"api_contract_configured"`
	SLAConfigured          bool                        `json:"sla_configured"`
	PricingModelConfigured bool                        `json:"pricing_model_configured"`
	MaskingConfigured      bool                        `json:"masking_configured"`
}

func NormalizeReadinessScore(score store.AssetReadinessScore) store.AssetReadinessScore {
	score.SchemaScore = clamp(score.SchemaScore)
	score.FreshnessScore = clamp(score.FreshnessScore)
	score.SampleScore = clamp(score.SampleScore)
	score.MissingnessScore = clamp(score.MissingnessScore)
	score.SensitivityScore = clamp(score.SensitivityScore)
	score.ExternalSharingScore = clamp(score.ExternalSharingScore)
	score.APIReadinessScore = clamp(score.APIReadinessScore)
	score.BillingReadinessScore = clamp(score.BillingReadinessScore)
	if score.OverallScore <= 0 {
		score.OverallScore = (score.SchemaScore + score.FreshnessScore + score.SampleScore + score.MissingnessScore +
			score.SensitivityScore + score.ExternalSharingScore + score.APIReadinessScore + score.BillingReadinessScore) / 8
	}
	score.OverallScore = clamp(score.OverallScore)
	return score
}

func RequiresStrictPublishGate(p store.DataProduct) bool {
	sensitivity := strings.ToLower(strings.TrimSpace(p.Sensitivity))
	sourceType := strings.ToLower(strings.TrimSpace(p.SourceType))
	if p.RiskScore >= 70 {
		return true
	}
	if containsAny(sensitivity, "restricted", "personal", "credit", "pseudonym", "sensitive") {
		return true
	}
	return sourceType == "api" && p.RiskScore >= 55
}

func EvaluatePublishGate(p store.DataProduct, readiness []store.AssetReadinessScore, approvals []store.ApprovalTrace, pack *store.EvidencePack, now time.Time) PublishGateResult {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	result := PublishGateResult{
		ProductKey:        p.ProductKey,
		StrictGate:        RequiresStrictPublishGate(p),
		Allowed:           true,
		MinimumReadiness:  DefaultReadinessThreshold,
		RequiredApprovals: append([]string(nil), RequiredPublishApprovals...),
		ApprovalStatus:    map[string]string{},
		AssetReadiness:    readiness,
		CheckedAt:         now.UTC().Format(time.RFC3339Nano),
		Metadata: map[string]any{
			"risk_score":  p.RiskScore,
			"sensitivity": p.Sensitivity,
			"source_type": p.SourceType,
		},
	}

	if !result.StrictGate {
		result.Warnings = append(result.Warnings, "strict gate is not required for this product risk/sensitivity profile")
		return result
	}

	assetKeys := ProductAssetKeys(p)
	readinessByAsset := map[string]store.AssetReadinessScore{}
	for _, score := range readiness {
		normalized := NormalizeReadinessScore(score)
		readinessByAsset[strings.ToLower(normalized.AssetKey)] = normalized
	}
	if len(assetKeys) == 0 {
		result.MissingEvidence = append(result.MissingEvidence, "source_assets")
		result.BlockedReasons = append(result.BlockedReasons, "at least one source data asset must be linked before publishing a strict-gated product")
	}
	for _, key := range assetKeys {
		score, ok := readinessByAsset[strings.ToLower(key)]
		if !ok {
			result.MissingEvidence = append(result.MissingEvidence, "asset_readiness:"+key)
			result.BlockedReasons = append(result.BlockedReasons, "missing asset readiness score for "+key)
			continue
		}
		if score.OverallScore < result.MinimumReadiness {
			result.BlockedReasons = append(result.BlockedReasons, fmt.Sprintf("asset %s readiness score %d is below %d", key, score.OverallScore, result.MinimumReadiness))
		}
	}

	for _, step := range result.RequiredApprovals {
		status := bestApprovalStatus(step, approvals, now)
		result.ApprovalStatus[step] = status
		if status != "approved" && status != "waived" {
			result.MissingApprovals = append(result.MissingApprovals, step)
			result.BlockedReasons = append(result.BlockedReasons, "missing required approval: "+step)
		}
	}

	if pack == nil || strings.TrimSpace(pack.PackJSON) == "" || strings.TrimSpace(pack.PackJSON) == "{}" {
		result.MissingEvidence = append(result.MissingEvidence, "evidence_pack")
		result.BlockedReasons = append(result.BlockedReasons, "evidence pack must be generated before publishing")
	}

	result.Allowed = len(result.BlockedReasons) == 0
	return result
}

func EvaluatePublishGateV2(
	p store.DataProduct,
	readiness []store.AssetReadinessScore,
	approvals []store.ApprovalTrace,
	pack *store.EvidencePack,
	sla *store.ProductSLA,
	cost *store.ProductCost,
	qualityResults []store.DataQualityResult,
	hasRiskReview bool,
	maskingConfigured bool,
	now time.Time,
) PublishGateResult {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	res := EvaluatePublishGate(p, readiness, approvals, pack, now)
	if !res.StrictGate {
		res.QualityPassed = true
		res.RiskReviewed = true
		res.APIContractConfigured = p.APISpec != ""
		res.SLAConfigured = sla != nil
		res.PricingModelConfigured = p.PricingModel != "" || (cost != nil && cost.QueryCost > 0)
		res.MaskingConfigured = true
		return res
	}

	res.QualityPassed = true
	if len(qualityResults) == 0 {
		res.QualityPassed = false
		res.Warnings = append(res.Warnings, "no data quality execution results found for assets")
		res.MissingEvidence = append(res.MissingEvidence, "quality_results")
	} else {
		for _, q := range qualityResults {
			if !q.Passed {
				res.QualityPassed = false
				res.BlockedReasons = append(res.BlockedReasons, "data quality rule failed: "+q.Message)
			}
		}
	}

	res.RiskReviewed = hasRiskReview
	if !hasRiskReview {
		res.Warnings = append(res.Warnings, "risk review check must be completed before publishing")
		res.MissingEvidence = append(res.MissingEvidence, "risk_review")
	}

	res.APIContractConfigured = p.APISpec != ""
	if !res.APIContractConfigured {
		res.Warnings = append(res.Warnings, "API contract (OpenAPI Spec) is not configured")
		res.MissingEvidence = append(res.MissingEvidence, "api_contract")
	}

	res.SLAConfigured = sla != nil
	if !res.SLAConfigured {
		res.Warnings = append(res.Warnings, "SLA targets are not configured")
		res.MissingEvidence = append(res.MissingEvidence, "product_sla")
	}

	res.PricingModelConfigured = p.PricingModel != "" || (cost != nil && (cost.QueryCost > 0 || cost.OpsCost > 0 || cost.DataProcessingCost > 0))
	if !res.PricingModelConfigured {
		res.Warnings = append(res.Warnings, "pricing model or operational cost parameters are not configured")
		res.MissingEvidence = append(res.MissingEvidence, "pricing_model")
	}

	sensitivity := strings.ToLower(strings.TrimSpace(p.Sensitivity))
	if containsAny(sensitivity, "restricted", "personal", "credit", "pseudonym", "sensitive") {
		res.MaskingConfigured = maskingConfigured
		if !maskingConfigured {
			res.BlockedReasons = append(res.BlockedReasons, "sample response masking policy is not configured for sensitive product")
			res.MissingEvidence = append(res.MissingEvidence, "masking_policy")
		}
	} else {
		res.MaskingConfigured = true
	}

	res.Allowed = len(res.BlockedReasons) == 0
	return res
}

func DefaultProductCanvas(p store.DataProduct) store.ProductCanvasV2 {
	return store.ProductCanvasV2{
		ProductKey:         p.ProductKey,
		CustomerProblem:    firstNonEmpty(p.Description, p.ExecutiveSummary, p.NameKO),
		Buyer:              strings.Join(p.TargetCustomers, ", "),
		UseCases:           firstNonEmpty(p.SalesPitch, p.Description),
		ProvidedData:       p.SourceRef,
		Differentiation:    p.Differentiation,
		PricingModel:       p.PricingModel,
		RiskNotes:          fmt.Sprintf("risk_score=%d sensitivity=%s", p.RiskScore, p.Sensitivity),
		POCSuccessCriteria: "validated business metric, approved data scope, and customer acceptance",
		ExpectedRevenue:    fmt.Sprintf("revenue_score=%d", p.RevenueScore),
		Owner:              p.Owner,
	}
}

func BuildEvidencePack(p store.DataProduct, canvas *store.ProductCanvasV2, readiness []store.AssetReadinessScore, approvals []store.ApprovalTrace, definition any, risk any, poc any, contract any, gate PublishGateResult) map[string]any {
	return map[string]any{
		"product": map[string]any{
			"product_key": p.ProductKey,
			"name_ko":     p.NameKO,
			"name_en":     p.NameEN,
			"status":      p.Status,
			"owner":       p.Owner,
			"sensitivity": p.Sensitivity,
			"risk_score":  p.RiskScore,
		},
		"canvas":          optionalCanvas(canvas),
		"data_lineage":    map[string]any{"source_type": p.SourceType, "source_ref": p.SourceRef, "asset_keys": ProductAssetKeys(p)},
		"asset_readiness": readiness,
		"definition":      definition,
		"risk_review":     risk,
		"approval_trace":  approvals,
		"poc_plan":        poc,
		"api_contract":    p.APISpec,
		"contract":        contract,
		"publish_gate":    gate,
		"generated_at":    time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func BuildOpenAPIDocument(p store.DataProduct) map[string]any {
	return BuildDynamicOpenAPIDocument(p, nil, nil)
}

func BuildDynamicOpenAPIDocument(p store.DataProduct, scope *store.ContractScope, sla *store.ProductSLA) map[string]any {
	endpoint := "/v1/data-products/" + p.ProductKey + "/query"
	var apiSpec map[string]any
	if strings.TrimSpace(p.APISpec) != "" {
		_ = json.Unmarshal([]byte(p.APISpec), &apiSpec)
	}
	if v, ok := apiSpec["endpoint"].(string); ok && strings.TrimSpace(v) != "" {
		endpoint = v
	}
	title := firstNonEmpty(p.NameEN, p.NameKO, p.ProductKey)
	responseProperties := map[string]any{}
	requiredFields := []string{}
	fields := []string{"product_key", "score", "risk_band", "as_of"}
	if scope != nil && len(scope.AllowedFields) > 0 {
		fields = scope.AllowedFields
	}
	for _, field := range uniqueStrings(fields) {
		responseProperties[field] = map[string]any{"type": openAPIFieldType(field)}
		requiredFields = append(requiredFields, field)
	}
	extensions := map[string]any{}
	if scope != nil {
		extensions["x-dataworks-contract"] = map[string]any{
			"contract_key":   scope.ContractKey,
			"customer_key":   scope.CustomerKey,
			"allowed_fields": scope.AllowedFields,
			"rate_limit":     scope.RateLimit,
			"valid_from":     scope.ValidFrom,
			"valid_to":       scope.ValidTo,
			"purpose":        scope.Purpose,
			"restrictions":   scope.Restrictions,
		}
	}
	if sla != nil {
		extensions["x-dataworks-sla"] = map[string]any{
			"refresh_cycle":       sla.RefreshCycle,
			"latency_target_ms":   sla.LatencyTargetMS,
			"availability_target": sla.AvailabilityTarget,
			"support_level":       sla.SupportLevel,
		}
	}
	return map[string]any{
		"openapi": "3.1.0",
		"info": map[string]any{
			"title":       title,
			"version":     fmt.Sprintf("%d", max(1, p.Version)),
			"description": firstNonEmpty(p.ExecutiveSummary, p.Description),
		},
		"paths": map[string]any{
			endpoint: map[string]any{
				"post": map[string]any{
					"summary":     title,
					"description": firstNonEmpty(p.Description, "Data Works product API"),
					"security":    []map[string][]string{{"bearerAuth": []string{}}},
					"requestBody": map[string]any{
						"required": true,
						"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
							"type":                 "object",
							"additionalProperties": true,
						}}},
					},
					"responses": map[string]any{
						"200": map[string]any{
							"description": "Product response",
							"content": map[string]any{"application/json": map[string]any{"schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"product_key":    map[string]any{"type": "string"},
									"customer_key":   map[string]any{"type": "string"},
									"contract_key":   map[string]any{"type": "string"},
									"entitlement_id": map[string]any{"type": "string"},
									"mock":           map[string]any{"type": "boolean"},
									"as_of":          map[string]any{"type": "string", "format": "date-time"},
									"data": map[string]any{
										"type":       "object",
										"properties": responseProperties,
										"required":   requiredFields,
									},
								},
							}}},
						},
						"403": map[string]any{"description": "Data product access or approval gate denied"},
						"429": map[string]any{"description": "Rate limit exceeded"},
					},
				},
			},
		},
		"x-dataworks": extensions,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"},
			},
		},
	}
}

func BuildProposalABVariants(p store.DataProduct, customerSegment string) []map[string]any {
	name := firstNonEmpty(p.ShortName, p.NameEN, p.NameKO, p.ProductKey)
	segment := firstNonEmpty(customerSegment, strings.Join(p.TargetCustomers, ", "), "target customer")
	baseProof := firstNonEmpty(p.Differentiation, p.ExecutiveSummary, p.Description)
	return []map[string]any{
		{
			"variant":     "executive",
			"audience":    "business decision maker",
			"headline":    name + " for measurable growth",
			"positioning": "Focus on revenue upside, faster decisions, and low-friction adoption for " + segment + ".",
			"proof":       baseProof,
			"cta":         "Approve a scoped PoC with success metrics and commercial next steps.",
		},
		{
			"variant":     "technical",
			"audience":    "data and platform team",
			"headline":    name + " as a governed API product",
			"positioning": "Focus on API contract, allowed fields, freshness, observability, and integration effort.",
			"proof":       fmt.Sprintf("source_type=%s source_ref=%s", p.SourceType, p.SourceRef),
			"cta":         "Validate API schema, sample payloads, SLA targets, and entitlement setup.",
		},
		{
			"variant":     "compliance",
			"audience":    "risk, legal, and compliance",
			"headline":    name + " with controlled data scope",
			"positioning": "Focus on purpose limitation, field minimization, approval trace, and evidence pack readiness.",
			"proof":       fmt.Sprintf("sensitivity=%s risk_score=%d", p.Sensitivity, p.RiskScore),
			"cta":         "Review contract scope, retention, and evidence before external sharing.",
		},
	}
}

func EvaluateRetirementCandidate(p store.DataProduct, cost *store.ProductCost, watermarks []store.DataWatermark, fitScores []store.ProductFitScore, entitlements []store.APIEntitlement, now time.Time) store.RetirementCandidate {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	risk := p.RiskScore
	reasons := []string{}
	activeUsage := 0
	lastUsedAt := ""
	for _, ent := range entitlements {
		if strings.EqualFold(strings.TrimSpace(ent.Status), "active") && !expiredAt(ent.ExpiresAt, now) {
			activeUsage++
		}
		if ent.UpdatedAt > lastUsedAt {
			lastUsedAt = ent.UpdatedAt
		}
	}
	if activeUsage == 0 {
		risk += 20
		reasons = append(reasons, "no active API entitlements")
	}
	if p.RevenueScore > 0 && p.RevenueScore < 35 {
		risk += 15
		reasons = append(reasons, "low revenue score")
	}
	if cost != nil && cost.EstimatedMargin < 0 {
		risk += 20
		reasons = append(reasons, "negative estimated margin")
	}
	for _, wm := range watermarks {
		status := strings.ToLower(strings.TrimSpace(wm.DelayStatus))
		if status == "stale" || status == "delayed" || status == "failed" {
			risk += 10
			reasons = append(reasons, "stale or failed data watermark: "+wm.AssetKey)
		}
	}
	if len(fitScores) > 0 {
		total := 0
		for _, score := range fitScores {
			total += score.FitScore
		}
		if total/len(fitScores) < 45 {
			risk += 15
			reasons = append(reasons, "low average customer fit")
		}
	}
	risk = clamp(risk)
	recommendation := "keep"
	if risk >= 75 {
		recommendation = "retire"
	} else if risk >= 50 {
		recommendation = "improve"
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "healthy product signals")
	}
	return store.RetirementCandidate{
		ProductKey:     p.ProductKey,
		Reason:         strings.Join(uniqueStrings(reasons), "; "),
		RiskScore:      risk,
		UsageCount:     activeUsage,
		LastUsedAt:     lastUsedAt,
		Recommendation: recommendation,
	}
}

func ComputeCustomerFitScore(p store.DataProduct, segment store.CustomerSegment) store.ProductFitScore {
	productText := strings.Join([]string{
		p.NameKO, p.NameEN, p.Description, p.ExecutiveSummary, p.SalesPitch,
		strings.Join(p.TargetIndustries, " "), strings.Join(p.TargetCustomers, " "),
		p.Differentiation, p.PricingModel,
	}, " ")
	segmentText := strings.Join([]string{
		segment.SegmentKey, segment.Industry, segment.BuyerType,
		strings.Join(segment.PainPoints, " "), segment.BudgetLevel,
	}, " ")

	score := 20
	evidence := []string{}
	reasons := []string{}

	if containsFold(p.TargetIndustries, segment.Industry) {
		score += 25
		evidence = append(evidence, "target_industries")
		reasons = append(reasons, "industry match")
	} else if overlapCount(tokenSet(strings.Join(p.TargetIndustries, " ")), tokenSet(segment.Industry)) > 0 {
		score += 10
		evidence = append(evidence, "target_industries")
		reasons = append(reasons, "partial industry overlap")
	}

	overlap := overlapCount(tokenSet(productText), tokenSet(segmentText))
	if overlap > 0 {
		score += min(35, overlap*7)
		evidence = append(evidence, "product_positioning", "segment_pain_points")
		reasons = append(reasons, fmt.Sprintf("%d shared positioning terms", overlap))
	}

	if p.RevenueScore > 0 {
		score += min(20, p.RevenueScore/5)
		evidence = append(evidence, "revenue_score")
		reasons = append(reasons, "commercial upside included")
	}
	if strings.TrimSpace(p.PricingModel) != "" && containsAny(strings.ToLower(segment.BudgetLevel), "high", "enterprise", "premium") {
		score += 10
		evidence = append(evidence, "pricing_model")
		reasons = append(reasons, "budget level supports priced offer")
	}
	if p.RiskScore > 0 {
		penalty := min(15, p.RiskScore/6)
		score -= penalty
		reasons = append(reasons, fmt.Sprintf("risk penalty -%d", penalty))
	}
	score = clamp(score)
	if len(reasons) == 0 {
		reasons = append(reasons, "limited matching evidence")
	}
	return store.ProductFitScore{
		ProductKey:      p.ProductKey,
		CustomerSegment: segment.SegmentKey,
		FitScore:        score,
		Reason:          strings.Join(uniqueStrings(reasons), "; "),
		EvidenceRefs:    uniqueStrings(evidence),
	}
}

func BuildProductSnapshot(p store.DataProduct, canvas *store.ProductCanvasV2, contract *store.ContractVersion) map[string]any {
	var apiSpec any = p.APISpec
	if strings.TrimSpace(p.APISpec) != "" {
		var parsed any
		if err := json.Unmarshal([]byte(p.APISpec), &parsed); err == nil {
			apiSpec = parsed
		}
	}
	return map[string]any{
		"product": map[string]any{
			"product_key":       p.ProductKey,
			"name_ko":           p.NameKO,
			"name_en":           p.NameEN,
			"description":       p.Description,
			"executive_summary": p.ExecutiveSummary,
			"sales_pitch":       p.SalesPitch,
			"source_type":       p.SourceType,
			"source_ref":        p.SourceRef,
			"sensitivity":       p.Sensitivity,
			"status":            p.Status,
			"target_industries": p.TargetIndustries,
			"target_customers":  p.TargetCustomers,
			"pricing_model":     p.PricingModel,
			"risk_score":        p.RiskScore,
			"revenue_score":     p.RevenueScore,
			"differentiation":   p.Differentiation,
			"similar_products":  p.SimilarProducts,
		},
		"canvas":              optionalCanvas(canvas),
		"api_spec":            apiSpec,
		"poc_plan":            p.POCPlan,
		"latest_contract":     optionalContract(contract),
		"snapshot_created_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func MarshalProductSnapshot(p store.DataProduct, canvas *store.ProductCanvasV2, contract *store.ContractVersion) (string, error) {
	raw, err := json.Marshal(BuildProductSnapshot(p, canvas, contract))
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func DiffProductSnapshots(fromJSON, toJSON string) string {
	from := map[string]any{}
	to := map[string]any{}
	_ = json.Unmarshal([]byte(fromJSON), &from)
	_ = json.Unmarshal([]byte(toJSON), &to)

	left := map[string]string{}
	right := map[string]string{}
	flattenJSON("", from, left)
	flattenJSON("", to, right)

	keys := map[string]bool{}
	for key := range left {
		keys[key] = true
	}
	for key := range right {
		keys[key] = true
	}
	ordered := make([]string, 0, len(keys))
	for key := range keys {
		if key == "snapshot_created_at" {
			continue
		}
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	changes := []string{}
	for _, key := range ordered {
		if left[key] == right[key] {
			continue
		}
		switch {
		case left[key] == "":
			changes = append(changes, "+ "+key+" = "+right[key])
		case right[key] == "":
			changes = append(changes, "- "+key+" was "+left[key])
		default:
			changes = append(changes, "~ "+key+" changed from "+left[key]+" to "+right[key])
		}
	}
	if len(changes) == 0 {
		return "no material changes"
	}
	return strings.Join(changes, "\n")
}

func BuildPortfolioGraph(products []store.DataProduct, assets []store.DataAsset, approvals []store.ApprovalTrace) map[string]any {
	nodes := []map[string]any{}
	edges := []map[string]any{}
	for _, p := range products {
		productNode := "product:" + p.ProductKey
		nodes = append(nodes, map[string]any{"id": productNode, "type": "product", "label": firstNonEmpty(p.ShortName, p.NameKO, p.ProductKey), "status": p.Status, "risk_score": p.RiskScore})
		for _, assetKey := range ProductAssetKeys(p) {
			assetNode := "asset:" + assetKey
			edges = append(edges, map[string]any{"from": productNode, "to": assetNode, "type": "uses_asset"})
		}
	}
	for _, a := range assets {
		nodes = append(nodes, map[string]any{"id": "asset:" + a.AssetKey, "type": "asset", "label": firstNonEmpty(a.Name, a.AssetKey), "domain": a.Domain, "sensitivity": a.Sensitivity})
	}
	for _, trace := range approvals {
		approvalNode := "approval:" + trace.ID
		nodes = append(nodes, map[string]any{"id": approvalNode, "type": "approval", "label": trace.Step, "status": trace.Status, "required": trace.Required})
		edges = append(edges, map[string]any{"from": "product:" + trace.ProductKey, "to": approvalNode, "type": "approval_trace"})
	}
	return map[string]any{"nodes": nodes, "edges": edges}
}

func ProductAssetKeys(p store.DataProduct) []string {
	return splitLoose(p.SourceRef)
}

func bestApprovalStatus(step string, approvals []store.ApprovalTrace, now time.Time) string {
	best := "missing"
	step = strings.ToLower(strings.TrimSpace(step))
	for _, trace := range approvals {
		if strings.ToLower(strings.TrimSpace(trace.Step)) != step {
			continue
		}
		status := strings.ToLower(strings.TrimSpace(trace.Status))
		if status == "" {
			status = "pending"
		}
		if trace.ExpiresAt != "" {
			if expiresAt, err := time.Parse(time.RFC3339Nano, trace.ExpiresAt); err == nil && expiresAt.Before(now) {
				status = "expired"
			}
		}
		if status == "approved" || status == "waived" {
			return status
		}
		best = status
	}
	return best
}

func optionalCanvas(canvas *store.ProductCanvasV2) any {
	if canvas == nil {
		return nil
	}
	return *canvas
}

func optionalContract(contract *store.ContractVersion) any {
	if contract == nil {
		return nil
	}
	return *contract
}

func flattenJSON(prefix string, value any, out map[string]string) {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			next := key
			if prefix != "" {
				next = prefix + "." + key
			}
			flattenJSON(next, typed[key], out)
		}
	case []any:
		raw, _ := json.Marshal(typed)
		out[prefix] = string(raw)
	default:
		raw, _ := json.Marshal(typed)
		out[prefix] = string(raw)
	}
}

func tokenSet(value string) map[string]bool {
	set := map[string]bool{}
	for _, tok := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') && r != '_'
	}) {
		if len(tok) >= 2 {
			set[tok] = true
		}
	}
	return set
}

func overlapCount(a, b map[string]bool) int {
	n := 0
	if len(a) > len(b) {
		a, b = b, a
	}
	for key := range a {
		if b[key] {
			n++
		}
	}
	return n
}

func containsFold(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func openAPIFieldType(field string) string {
	field = strings.ToLower(field)
	switch {
	case strings.Contains(field, "score"), strings.Contains(field, "amount"), strings.Contains(field, "cost"), strings.Contains(field, "rate"), strings.Contains(field, "count"):
		return "number"
	case strings.Contains(field, "at"), strings.Contains(field, "date"), strings.Contains(field, "time"):
		return "string"
	case strings.Contains(field, "active"), strings.Contains(field, "enabled"), strings.Contains(field, "flag"):
		return "boolean"
	default:
		return "string"
	}
}

func expiredAt(raw string, now time.Time) bool {
	if strings.TrimSpace(raw) == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	return err == nil && t.Before(now)
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func clamp(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
