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
