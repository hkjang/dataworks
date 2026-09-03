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
