package dataworks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"dataworks/internal/store"
)

func TestEvaluatePublishGateBlocksStrictProductUntilEvidenceIsComplete(t *testing.T) {
	product := store.DataProduct{
		ProductKey:  "dw_credit_score",
		SourceType:  "api",
		SourceRef:   "loan_history",
		Sensitivity: "personal_credit",
		RiskScore:   82,
	}

	gate := EvaluatePublishGate(product, nil, nil, nil, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if gate.Allowed {
		t.Fatalf("strict product should be blocked without evidence: %+v", gate)
	}
	if !hasString(gate.MissingApprovals, "legal") || !hasString(gate.MissingEvidence, "evidence_pack") {
		t.Fatalf("missing approval/evidence not reported: %+v", gate)
	}

	readiness := []store.AssetReadinessScore{NormalizeReadinessScore(store.AssetReadinessScore{
		AssetKey: "loan_history", SchemaScore: 92, FreshnessScore: 91, SampleScore: 90, MissingnessScore: 94,
		SensitivityScore: 88, ExternalSharingScore: 90, APIReadinessScore: 93, BillingReadinessScore: 87,
	})}
	approvals := []store.ApprovalTrace{
		{Step: "data_owner", Status: "approved", Required: true},
		{Step: "legal", Status: "approved", Required: true},
		{Step: "compliance", Status: "approved", Required: true},
	}
	pack := store.EvidencePack{ProductKey: product.ProductKey, PackJSON: `{"ok":true}`}

	gate = EvaluatePublishGate(product, readiness, approvals, &pack, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if !gate.Allowed {
		t.Fatalf("strict product should pass when evidence is complete: %+v", gate)
	}

	approvals = append(approvals, store.ApprovalTrace{Step: "security", Status: "pending", Required: true})
	gate = EvaluatePublishGate(product, readiness, approvals, &pack, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if gate.Allowed || !hasString(gate.RequiredApprovals, "security") || !hasString(gate.MissingApprovals, "security") {
		t.Fatalf("custom required approval must block publishing: %+v", gate)
	}
	approvals[len(approvals)-1].Status = "approved"
	gate = EvaluatePublishGate(product, readiness, approvals, &pack, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if !gate.Allowed {
		t.Fatalf("approved custom requirement should unblock publishing: %+v", gate)
	}
}

func TestEvaluatePublishGateHonorsCustomRequirementForStandardProduct(t *testing.T) {
	product := store.DataProduct{ProductKey: "standard", Sensitivity: "internal", RiskScore: 10}
	approvals := []store.ApprovalTrace{{Step: "security", Status: "pending", Required: true}}
	gate := EvaluatePublishGate(product, nil, approvals, nil, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if gate.Allowed || !hasString(gate.MissingApprovals, "security") {
		t.Fatalf("custom requirement must apply to a standard product: %+v", gate)
	}
	approvals[0].Required = false
	gate = EvaluatePublishGate(product, nil, approvals, nil, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if !gate.Allowed || len(gate.RequiredApprovals) != 0 {
		t.Fatalf("optional approval must not block a standard product: %+v", gate)
	}
	approvals[0] = store.ApprovalTrace{Step: "security", Status: "approved", Required: true, ExpiresAt: "invalid-time"}
	gate = EvaluatePublishGate(product, nil, approvals, nil, time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC))
	if gate.Allowed || gate.ApprovalStatus["security"] != "expired" {
		t.Fatalf("invalid legacy expiration must fail closed: %+v", gate)
	}
}

func TestCustomerFitScoreAndSnapshotDiff(t *testing.T) {
	product := store.DataProduct{
		ProductKey: "dw_credit_score", NameEN: "Credit Score API",
		Description:      "loan approval and credit risk score for banking risk teams",
		TargetIndustries: []string{"banking"}, TargetCustomers: []string{"risk team"},
		PricingModel: "enterprise subscription", RevenueScore: 80, RiskScore: 30,
	}
	segment := store.CustomerSegment{
		SegmentKey: "bank_enterprise", Industry: "banking", BuyerType: "risk team",
		PainPoints: []string{"loan approval", "credit risk"}, BudgetLevel: "enterprise",
	}
	score := ComputeCustomerFitScore(product, segment)
	if score.FitScore < 70 {
		t.Fatalf("expected strong customer fit, got %+v", score)
	}
	if !hasString(score.EvidenceRefs, "target_industries") {
		t.Fatalf("expected target industry evidence, got %+v", score)
	}

	from, err := MarshalProductSnapshot(product, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	product.RiskScore = 55
	to, err := MarshalProductSnapshot(product, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	diff := DiffProductSnapshots(from, to)
	if diff == "no material changes" || !strings.Contains(diff, "risk_score") {
		t.Fatalf("expected risk_score diff, got %q", diff)
	}
}

func TestCustomerFitScoreMatchesKoreanPositioningTerms(t *testing.T) {
	product := store.DataProduct{
		ProductKey: "dw_credit_score", NameKO: "여신 승인 신용 위험 스코어",
		Description:      "여신 승인 심사와 신용 위험 평가를 지원하는 스코어 데이터",
		TargetIndustries: []string{"은행"}, TargetCustomers: []string{"리스크 팀"},
		PricingModel: "엔터프라이즈 구독", RevenueScore: 80, RiskScore: 30,
	}
	segment := store.CustomerSegment{
		SegmentKey: "bank_enterprise", Industry: "은행", BuyerType: "리스크 팀",
		PainPoints: []string{"여신 승인", "신용 위험"}, BudgetLevel: "enterprise",
	}

	score := ComputeCustomerFitScore(product, segment)
	if !hasString(score.EvidenceRefs, "segment_pain_points") {
		t.Fatalf("korean pain points must count as positioning evidence: %+v", score)
	}
	if score.FitScore < 70 {
		t.Fatalf("expected strong korean customer fit, got %+v", score)
	}

	// An unrelated segment must still score materially lower, so the tokenizer
	// change does not simply inflate every pairing.
	other := ComputeCustomerFitScore(product, store.CustomerSegment{
		SegmentKey: "retail_smb", Industry: "유통", BuyerType: "매장 운영",
		PainPoints: []string{"재고 회전", "매장 방문객"}, BudgetLevel: "low",
	})
	if other.FitScore >= score.FitScore {
		t.Fatalf("unrelated segment should score lower: matched=%+v unrelated=%+v", score, other)
	}
	if hasString(other.EvidenceRefs, "segment_pain_points") {
		t.Fatalf("unrelated segment must not claim positioning evidence: %+v", other)
	}
}

func TestTokenSetSkipsPunctuationAndSingleRuneTerms(t *testing.T) {
	tokens := tokenSet("여신 및 승인, Credit-Score a 42 x_y")
	want := []string{"여신", "승인", "credit", "score", "42", "x_y"}
	for _, term := range want {
		if !tokens[term] {
			t.Fatalf("expected token %q in %v", term, tokens)
		}
	}
	for _, term := range []string{"및", "a", "-", ","} {
		if tokens[term] {
			t.Fatalf("unexpected token %q in %v", term, tokens)
		}
	}
}

func TestDynamicOpenAPIProposalVariantsAndRetirement(t *testing.T) {
	product := store.DataProduct{
		ProductKey: "dw_credit_score", NameEN: "Credit Score API", SourceType: "api", SourceRef: "loan_history",
		Sensitivity: "personal_credit", RiskScore: 65, RevenueScore: 20,
	}
	scope := store.ContractScope{
		ContractKey: "ct_bank", CustomerKey: "cust_bank", ProductKey: product.ProductKey,
		AllowedFields: []string{"score", "risk_band"}, RateLimit: 100, Purpose: "credit risk monitoring",
	}
	sla := store.ProductSLA{ProductKey: product.ProductKey, RefreshCycle: "daily", LatencyTargetMS: 250, AvailabilityTarget: 0.995}
	doc := BuildDynamicOpenAPIDocument(product, &scope, &sla)
	raw, _ := json.Marshal(doc)
	if !strings.Contains(string(raw), "x-dataworks-contract") || !strings.Contains(string(raw), "risk_band") {
		t.Fatalf("dynamic openapi missing contract fields: %s", raw)
	}

	variants := BuildProposalABVariants(product, "enterprise bank")
	if len(variants) != 3 || variants[0]["variant"] == "" {
		t.Fatalf("proposal variants mismatch: %+v", variants)
	}

	candidate := EvaluateRetirementCandidate(
		product,
		&store.ProductCost{ProductKey: product.ProductKey, EstimatedMargin: -100},
		[]store.DataWatermark{{AssetKey: "loan_history", ProductKey: product.ProductKey, DelayStatus: "stale"}},
		[]store.ProductFitScore{{ProductKey: product.ProductKey, CustomerSegment: "bank", FitScore: 30}},
		nil,
		time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC),
	)
	if candidate.Recommendation != "retire" || candidate.RiskScore < 75 {
		t.Fatalf("expected retirement recommendation, got %+v", candidate)
	}
}

func hasString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRetirementIgnoresEntitlementWithUnparseableExpiry(t *testing.T) {
	product := store.DataProduct{ProductKey: "dw_credit_score", RiskScore: 30, RevenueScore: 80}
	now := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)

	live := EvaluateRetirementCandidate(product, nil, nil, nil, []store.APIEntitlement{
		{ProductKey: product.ProductKey, Status: "active", ExpiresAt: "2030-01-01T00:00:00Z"},
	}, now)
	if live.UsageCount != 1 || strings.Contains(live.Reason, "no active API entitlements") {
		t.Fatalf("parseable future expiry should count as active usage: %+v", live)
	}

	// The runtime access gate denies a malformed expiry, so it must not be reported as live usage.
	broken := EvaluateRetirementCandidate(product, nil, nil, nil, []store.APIEntitlement{
		{ProductKey: product.ProductKey, Status: "active", ExpiresAt: "2026-12-31"},
	}, now)
	if broken.UsageCount != 0 || !strings.Contains(broken.Reason, "no active API entitlements") {
		t.Fatalf("unparseable expiry should not count as active usage: %+v", broken)
	}
}

// strictGateFixture is the complete V1 evidence set from
// TestEvaluatePublishGateBlocksStrictProductUntilEvidenceIsComplete, so V2 cases only exercise
// the V2 evidence checks layered on top of an already-passing V1 gate.
func strictGateFixture() (store.DataProduct, []store.AssetReadinessScore, []store.ApprovalTrace, *store.EvidencePack) {
	product := store.DataProduct{
		ProductKey:   "dw_credit_score",
		SourceType:   "api",
		SourceRef:    "loan_history",
		Sensitivity:  "personal_credit",
		RiskScore:    82,
		APISpec:      `{"openapi":"3.0.0"}`,
		PricingModel: "per_call",
	}
	readiness := []store.AssetReadinessScore{NormalizeReadinessScore(store.AssetReadinessScore{
		AssetKey: "loan_history", SchemaScore: 92, FreshnessScore: 91, SampleScore: 90, MissingnessScore: 94,
		SensitivityScore: 88, ExternalSharingScore: 90, APIReadinessScore: 93, BillingReadinessScore: 87,
	})}
	approvals := []store.ApprovalTrace{
		{Step: "data_owner", Status: "approved", Required: true},
		{Step: "legal", Status: "approved", Required: true},
		{Step: "compliance", Status: "approved", Required: true},
	}
	pack := &store.EvidencePack{ProductKey: product.ProductKey, PackJSON: `{"ok":true}`}
	return product, readiness, approvals, pack
}

func TestEvaluatePublishGateV2StrictEvidence(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	passing := []store.DataQualityResult{{AssetKey: "loan_history", RuleID: "not_null", Passed: true}}
	sla := &store.ProductSLA{ProductKey: "dw_credit_score", RefreshCycle: "daily"}

	cases := []struct {
		name            string
		mutate          func(p *store.DataProduct)
		quality         []store.DataQualityResult
		sla             *store.ProductSLA
		risk            bool
		masking         bool
		wantAllowed     bool
		wantQuality     bool
		wantRisk        bool
		wantAPI         bool
		wantSLA         bool
		wantPricing     bool
		wantMasking     bool
		wantMissing     []string
		wantBlockedPart string
	}{
		{
			name:    "complete evidence is allowed with nothing missing",
			quality: passing, sla: sla, risk: true, masking: true,
			wantAllowed: true, wantQuality: true, wantRisk: true, wantAPI: true, wantSLA: true, wantPricing: true, wantMasking: true,
		},
		{
			name:    "no quality results is a warning only",
			quality: []store.DataQualityResult{}, sla: sla, risk: true, masking: true,
			wantAllowed: true, wantQuality: false, wantRisk: true, wantAPI: true, wantSLA: true, wantPricing: true, wantMasking: true,
			wantMissing: []string{"quality_results"},
		},
		{
			name:    "failed quality rule blocks publishing",
			quality: []store.DataQualityResult{{AssetKey: "loan_history", RuleID: "range", Passed: false, Message: "score out of range"}},
			sla:     sla, risk: true, masking: true,
			wantAllowed: false, wantQuality: false, wantRisk: true, wantAPI: true, wantSLA: true, wantPricing: true, wantMasking: true,
			wantBlockedPart: "data quality rule failed: score out of range",
		},
		{
			name:    "sensitive product without masking is blocked",
			quality: passing, sla: sla, risk: true, masking: false,
			wantAllowed: false, wantQuality: true, wantRisk: true, wantAPI: true, wantSLA: true, wantPricing: true, wantMasking: false,
			wantMissing: []string{"masking_policy"}, wantBlockedPart: "masking policy",
		},
		{
			name: "missing risk review, api spec, sla and pricing are warnings only",
			mutate: func(p *store.DataProduct) {
				p.APISpec = ""
				p.PricingModel = ""
			},
			quality: passing, sla: nil, risk: false, masking: true,
			wantAllowed: true, wantQuality: true, wantRisk: false, wantAPI: false, wantSLA: false, wantPricing: false, wantMasking: true,
			wantMissing: []string{"risk_review", "api_contract", "product_sla", "pricing_model"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			product, readiness, approvals, pack := strictGateFixture()
			if tc.mutate != nil {
				tc.mutate(&product)
			}
			got := EvaluatePublishGateV2(product, readiness, approvals, pack, tc.sla, nil, tc.quality, tc.risk, tc.masking, now)
			if !got.StrictGate {
				t.Fatalf("fixture must be a strict product: %+v", got)
			}
			if got.Allowed != tc.wantAllowed {
				t.Fatalf("allowed=%v want %v: %+v", got.Allowed, tc.wantAllowed, got)
			}
			if got.QualityPassed != tc.wantQuality || got.RiskReviewed != tc.wantRisk || got.APIContractConfigured != tc.wantAPI ||
				got.SLAConfigured != tc.wantSLA || got.PricingModelConfigured != tc.wantPricing || got.MaskingConfigured != tc.wantMasking {
				t.Fatalf("evidence flags mismatch: quality=%v risk=%v api=%v sla=%v pricing=%v masking=%v (%+v)",
					got.QualityPassed, got.RiskReviewed, got.APIContractConfigured, got.SLAConfigured, got.PricingModelConfigured, got.MaskingConfigured, got)
			}
			if len(tc.wantMissing) == 0 && len(got.MissingEvidence) != 0 {
				t.Fatalf("expected no missing evidence, got %v", got.MissingEvidence)
			}
			for _, want := range tc.wantMissing {
				if !hasString(got.MissingEvidence, want) {
					t.Fatalf("missing evidence %q not reported: %v", want, got.MissingEvidence)
				}
			}
			if tc.wantBlockedPart == "" {
				if len(got.BlockedReasons) != 0 {
					t.Fatalf("expected no blocked reasons, got %v", got.BlockedReasons)
				}
				return
			}
			found := false
			for _, reason := range got.BlockedReasons {
				if strings.Contains(reason, tc.wantBlockedPart) {
					found = true
				}
			}
			if !found {
				t.Fatalf("blocked reason containing %q not found: %v", tc.wantBlockedPart, got.BlockedReasons)
			}
		})
	}
}

func TestEvaluatePublishGateV2PricingPredicateIsSameForStandardAndStrict(t *testing.T) {
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	strictProduct, readiness, approvals, pack := strictGateFixture()
	strictProduct.PricingModel = ""
	standardProduct := store.DataProduct{ProductKey: "dw_public_stats", SourceType: "batch", RiskScore: 10, Sensitivity: "public"}
	if RequiresStrictPublishGate(standardProduct) || !RequiresStrictPublishGate(strictProduct) {
		t.Fatal("fixture products must fall on opposite sides of the strict gate")
	}
	quality := []store.DataQualityResult{{AssetKey: "loan_history", Passed: true}}
	sla := &store.ProductSLA{RefreshCycle: "daily"}

	cases := []struct {
		name         string
		pricingModel string
		cost         *store.ProductCost
		want         bool
	}{
		{name: "ops cost only", cost: &store.ProductCost{OpsCost: 25}, want: true},
		{name: "data processing cost only", cost: &store.ProductCost{DataProcessingCost: 10}, want: true},
		{name: "query cost only", cost: &store.ProductCost{QueryCost: 100}, want: true},
		{name: "pricing model without cost row", pricingModel: "per_call", want: true},
		{name: "nothing configured", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			standard := standardProduct
			standard.PricingModel = tc.pricingModel
			strict := strictProduct
			strict.PricingModel = tc.pricingModel

			gotStandard := EvaluatePublishGateV2(standard, nil, nil, nil, sla, tc.cost, quality, true, true, now)
			gotStrict := EvaluatePublishGateV2(strict, readiness, approvals, pack, sla, tc.cost, quality, true, true, now)
			if gotStandard.StrictGate || !gotStrict.StrictGate {
				t.Fatalf("strict flag mismatch: standard=%+v strict=%+v", gotStandard, gotStrict)
			}
			if gotStandard.PricingModelConfigured != tc.want {
				t.Fatalf("standard product pricing_model_configured=%v want %v: %+v", gotStandard.PricingModelConfigured, tc.want, gotStandard)
			}
			if gotStrict.PricingModelConfigured != tc.want {
				t.Fatalf("strict product pricing_model_configured=%v want %v: %+v", gotStrict.PricingModelConfigured, tc.want, gotStrict)
			}
			// The non-strict branch reports the flag but never accumulates evidence warnings
			// (its only warning is the V1 "strict gate is not required" note).
			if hasString(gotStandard.MissingEvidence, "pricing_model") {
				t.Fatalf("standard product must not accumulate pricing warnings: %+v", gotStandard)
			}
			for _, warning := range gotStandard.Warnings {
				if strings.Contains(warning, "pricing") {
					t.Fatalf("standard product must not accumulate pricing warnings: %+v", gotStandard)
				}
			}
			if !gotStandard.MaskingConfigured || !gotStandard.QualityPassed || !gotStandard.Allowed {
				t.Fatalf("standard product defaults changed: %+v", gotStandard)
			}
			if tc.want == hasString(gotStrict.MissingEvidence, "pricing_model") {
				t.Fatalf("strict product missing_evidence must mirror the flag: %+v", gotStrict)
			}
		})
	}
}

func TestEvaluateRetirementCandidateThresholds(t *testing.T) {
	now := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)
	live := []store.APIEntitlement{{Status: "active", ExpiresAt: "", UpdatedAt: "2026-07-01T00:00:00Z"}}

	cases := []struct {
		name         string
		product      store.DataProduct
		cost         *store.ProductCost
		watermarks   []store.DataWatermark
		fitScores    []store.ProductFitScore
		entitlements []store.APIEntitlement
		wantRisk     int
		wantRec      string
		wantReason   string
		wantUsage    int
		wantLastUsed string
	}{
		{
			name:         "healthy product keeps its own risk score",
			product:      store.DataProduct{ProductKey: "p", RiskScore: 49, RevenueScore: 80},
			entitlements: live,
			wantRisk:     49, wantRec: "keep", wantReason: "healthy product signals", wantUsage: 1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:     "no entitlements pushes a 30 product into improve",
			product:  store.DataProduct{ProductKey: "p", RiskScore: 30, RevenueScore: 80},
			wantRisk: 50, wantRec: "improve", wantReason: "no active API entitlements", wantUsage: 0,
		},
		{
			name:         "low revenue and negative margin reach retire from 40",
			product:      store.DataProduct{ProductKey: "p", RiskScore: 40, RevenueScore: 20},
			cost:         &store.ProductCost{EstimatedMargin: -1},
			entitlements: live,
			wantRisk:     75, wantRec: "retire", wantReason: "low revenue score; negative estimated margin", wantUsage: 1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:         "revenue score zero is not treated as low revenue",
			product:      store.DataProduct{ProductKey: "p", RiskScore: 60, RevenueScore: 0},
			entitlements: live,
			wantRisk:     60, wantRec: "improve", wantReason: "healthy product signals", wantUsage: 1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:    "watermark states count case-insensitively and are deduplicated per asset",
			product: store.DataProduct{ProductKey: "p", RiskScore: 10, RevenueScore: 80},
			watermarks: []store.DataWatermark{
				{AssetKey: "a", DelayStatus: "Stale"},
				{AssetKey: "b", DelayStatus: "DELAYED"},
				{AssetKey: "c", DelayStatus: "failed"},
				{AssetKey: "c", DelayStatus: "failed"},
				{AssetKey: "d", DelayStatus: "ok"},
			},
			entitlements: live,
			wantRisk:     50, wantRec: "improve",
			wantReason: "stale or failed data watermark: a; stale or failed data watermark: b; stale or failed data watermark: c",
			wantUsage:  1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:         "low average fit adds 15 and average at 45 does not",
			product:      store.DataProduct{ProductKey: "p", RiskScore: 40, RevenueScore: 80},
			fitScores:    []store.ProductFitScore{{FitScore: 30}, {FitScore: 58}},
			entitlements: live,
			wantRisk:     55, wantRec: "improve", wantReason: "low average customer fit", wantUsage: 1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:         "average fit of exactly 45 is healthy",
			product:      store.DataProduct{ProductKey: "p", RiskScore: 40, RevenueScore: 80},
			fitScores:    []store.ProductFitScore{{FitScore: 40}, {FitScore: 50}},
			entitlements: live,
			wantRisk:     40, wantRec: "keep", wantReason: "healthy product signals", wantUsage: 1, wantLastUsed: "2026-07-01T00:00:00Z",
		},
		{
			name:       "every signal together is clamped to 100",
			product:    store.DataProduct{ProductKey: "p", RiskScore: 90, RevenueScore: 10},
			cost:       &store.ProductCost{EstimatedMargin: -5},
			watermarks: []store.DataWatermark{{AssetKey: "a", DelayStatus: "stale"}},
			fitScores:  []store.ProductFitScore{{FitScore: 10}},
			entitlements: []store.APIEntitlement{
				{Status: "revoked", UpdatedAt: "2026-07-05T00:00:00Z"},
				{Status: "revoked", UpdatedAt: "2026-07-02T00:00:00Z"},
			},
			wantRisk: 100, wantRec: "retire",
			wantReason: "no active API entitlements; low revenue score; negative estimated margin; stale or failed data watermark: a; low average customer fit",
			wantUsage:  0, wantLastUsed: "2026-07-05T00:00:00Z",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateRetirementCandidate(tc.product, tc.cost, tc.watermarks, tc.fitScores, tc.entitlements, now)
			if got.ProductKey != tc.product.ProductKey {
				t.Fatalf("product key mismatch: %+v", got)
			}
			if got.RiskScore != tc.wantRisk || got.Recommendation != tc.wantRec {
				t.Fatalf("risk=%d rec=%q want %d/%q: %+v", got.RiskScore, got.Recommendation, tc.wantRisk, tc.wantRec, got)
			}
			if got.Reason != tc.wantReason {
				t.Fatalf("reason=%q want %q", got.Reason, tc.wantReason)
			}
			if got.UsageCount != tc.wantUsage || got.LastUsedAt != tc.wantLastUsed {
				t.Fatalf("usage=%d last_used=%q want %d/%q", got.UsageCount, got.LastUsedAt, tc.wantUsage, tc.wantLastUsed)
			}
		})
	}
}

func TestRetirementCountsEntitlementLikeRuntimeGate(t *testing.T) {
	product := store.DataProduct{ProductKey: "dw_credit_score", RiskScore: 30, RevenueScore: 80}
	now := time.Date(2026, 7, 9, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name      string
		ent       store.APIEntitlement
		wantUsage int
	}{
		{name: "legacy row with padded status and expiry is live", ent: store.APIEntitlement{Status: " Active ", ExpiresAt: " 2030-01-01T00:00:00Z "}, wantUsage: 1},
		{name: "revoked row is not live", ent: store.APIEntitlement{Status: "revoked", ExpiresAt: "2030-01-01T00:00:00Z"}, wantUsage: 0},
		{name: "expiry equal to now is already inactive", ent: store.APIEntitlement{Status: "active", ExpiresAt: now.Format(time.RFC3339)}, wantUsage: 0},
		{name: "empty expiry never expires", ent: store.APIEntitlement{Status: "active", ExpiresAt: ""}, wantUsage: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent := tc.ent
			ent.ProductKey = product.ProductKey
			if store.EntitlementActive(ent, now) != (tc.wantUsage == 1) {
				t.Fatalf("fixture disagrees with the runtime gate: %+v", ent)
			}
			got := EvaluateRetirementCandidate(product, nil, nil, nil, []store.APIEntitlement{ent}, now)
			if got.UsageCount != tc.wantUsage {
				t.Fatalf("usage=%d want %d: %+v", got.UsageCount, tc.wantUsage, got)
			}
			if (tc.wantUsage == 0) != strings.Contains(got.Reason, "no active API entitlements") {
				t.Fatalf("reason must mirror usage count: %+v", got)
			}
		})
	}
}
