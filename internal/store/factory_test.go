package store

import (
	"context"
	"path/filepath"
	"testing"

	"clustara/internal/config"
)

func TestFactoryRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "factory.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if err := db.UpsertDataAsset(ctx, DataAsset{ID: "asset_1", AssetKey: "loan_history", Name: "대출 이력", Domain: "credit", Owner: "data"}); err != nil {
		t.Fatal(err)
	}
	assets, err := db.ListDataAssets(ctx)
	if err != nil || len(assets) != 1 || assets[0].AssetKey != "loan_history" {
		t.Fatalf("assets mismatch: %+v err=%v", assets, err)
	}

	idea := ProductIdea{
		ID: "idea_1", Title: "소상공인 리스크 API", TargetIndustry: "금융",
		TargetCustomers: []string{"은행"}, CustomerNeed: "조기 위험 탐지",
		DataAssets: []string{"loan_history"}, DeliveryMethod: "API", RevenueScore: 80, RiskScore: 65,
	}
	if err := db.InsertProductIdea(ctx, idea); err != nil {
		t.Fatal(err)
	}
	gotIdea, ok, err := db.GetProductIdea(ctx, "idea_1")
	if err != nil || !ok || gotIdea.TargetCustomers[0] != "은행" {
		t.Fatalf("idea roundtrip: %+v ok=%v err=%v", gotIdea, ok, err)
	}

	version, err := db.InsertProductDefinition(ctx, ProductDefinition{
		ID: "pdef_1", IdeaID: "idea_1", ProductKey: "dw_risk_api", DefinitionJSON: `{"name":"소상공인 리스크 API"}`,
	})
	if err != nil || version != 1 {
		t.Fatalf("definition version=%d err=%v", version, err)
	}
	if _, err := db.InsertProductDefinition(ctx, ProductDefinition{
		ID: "pdef_2", IdeaID: "idea_1", ProductKey: "dw_risk_api", DefinitionJSON: `{"name":"v2"}`,
	}); err != nil {
		t.Fatal(err)
	}
	def, ok, err := db.LatestProductDefinition(ctx, "dw_risk_api")
	if err != nil || !ok || def.Version != 2 {
		t.Fatalf("latest definition: %+v ok=%v err=%v", def, ok, err)
	}

	if err := db.InsertProductRiskReview(ctx, ProductRiskReview{
		ID: "risk_1", ProductKey: "dw_risk_api", PrivacyScore: 60, CreditScore: 80, AIScore: 50, SecurityScore: 40,
	}); err != nil {
		t.Fatal(err)
	}
	risk, ok, err := db.LatestProductRiskReview(ctx, "dw_risk_api")
	if err != nil || !ok || risk.OverallScore != 57 {
		t.Fatalf("risk review: %+v ok=%v err=%v", risk, ok, err)
	}

	if err := db.InsertProductPOCPlan(ctx, ProductPOCPlan{
		ID: "poc_1", ProductKey: "dw_risk_api", DataScope: "24개월", SuccessMetric: "AUC", Timeline: "4주",
	}); err != nil {
		t.Fatal(err)
	}
	poc, ok, err := db.LatestProductPOCPlan(ctx, "dw_risk_api")
	if err != nil || !ok || poc.ApprovalStatus != "pending" {
		t.Fatalf("poc plan: %+v ok=%v err=%v", poc, ok, err)
	}

	dash, err := db.FactoryDashboard(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if dash.IdeasTotal != 1 || dash.HighRiskReviews != 0 || dash.PendingPOCPlans != 1 {
		t.Fatalf("dashboard mismatch: %+v", dash)
	}
}

func TestDataWorksOperationalRecords(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dataworks_ops.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if err := db.UpsertDataAsset(ctx, DataAsset{
		ID: "asset_ops_1", AssetKey: "loan_history", Name: "Loan History", Domain: "credit",
		Owner: "risk-data", ColumnsSummary: "loan_id, overdue_days, balance", Sensitivity: "personal_credit", RefreshCycle: "daily",
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.GetDataAsset(ctx, "loan_history"); err != nil || !ok {
		t.Fatalf("asset lookup ok=%v err=%v", ok, err)
	}
	score := DataAssetReadinessScore{AssetKey: "loan_history", QualityScore: 80, FreshnessScore: 90, OwnerScore: 90, MetadataScore: 80, SensitivityScore: 45, ApprovalScore: 60, SampleScore: 35, OverallScore: 68, Status: "External Review", Blockers: []string{"external_review_required"}, CheckedBy: "tester"}
	if err := db.UpsertDataAssetReadinessScore(ctx, score); err != nil {
		t.Fatal(err)
	}
	gotScore, ok, err := db.LatestDataAssetReadinessScore(ctx, "loan_history")
	if err != nil || !ok || gotScore.Blockers[0] != "external_review_required" {
		t.Fatalf("readiness roundtrip: %+v ok=%v err=%v", gotScore, ok, err)
	}

	product := DataProduct{ID: "dprod_ops_1", ProductKey: "dw_credit_api", NameKO: "Credit API", SourceType: "api", SourceRef: "loan_history", Owner: "data-business", Status: "published", RiskScore: 72, RevenueScore: 80}
	if err := db.UpsertDataProduct(ctx, product); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertProductCanvas(ctx, ProductCanvas{ProductKey: product.ProductKey, CustomerProblem: "risk", DataInputs: []string{"loan_history"}, CreatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	canvas, ok, err := db.GetProductCanvas(ctx, product.ProductKey)
	if err != nil || !ok || len(canvas.DataInputs) != 1 {
		t.Fatalf("canvas roundtrip: %+v ok=%v err=%v", canvas, ok, err)
	}
	if err := db.ReplaceProductEvidencePack(ctx, product.ProductKey, []ProductEvidence{{ID: "evid_1", EvidenceType: "data_assets", SourceRef: "loan_history", Summary: "asset evidence", ConfidenceScore: 80}}); err != nil {
		t.Fatal(err)
	}
	evidence, err := db.ListProductEvidence(ctx, product.ProductKey)
	if err != nil || len(evidence) != 1 || evidence[0].ProductKey != product.ProductKey {
		t.Fatalf("evidence roundtrip: %+v err=%v", evidence, err)
	}
	if err := db.ReplaceRegulatoryTrace(ctx, product.ProductKey, []RegulatoryTrace{{ID: "trace_1", RiskDomain: "legal_review", Question: "approved?", Answer: "yes", Decision: "approved", Reviewer: "legal"}}); err != nil {
		t.Fatal(err)
	}
	trace, err := db.ListRegulatoryTrace(ctx, product.ProductKey)
	if err != nil || len(trace) != 1 || trace[0].Decision != "approved" {
		t.Fatalf("trace roundtrip: %+v err=%v", trace, err)
	}

	version, err := db.InsertProductAPIContract(ctx, APIContract{ID: "apic_1", ProductKey: product.ProductKey, OpenAPIJSON: `{"openapi":"3.0.3"}`, CreatedBy: "tester"})
	if err != nil || version != 1 {
		t.Fatalf("api contract version=%d err=%v", version, err)
	}
	latest, ok, err := db.LatestProductAPIContract(ctx, product.ProductKey)
	if err != nil || !ok || latest.Version != 1 {
		t.Fatalf("latest api contract: %+v ok=%v err=%v", latest, ok, err)
	}

	if _, err := db.InsertProductDefinition(ctx, ProductDefinition{ID: "pdef_ops_1", ProductKey: product.ProductKey, DefinitionJSON: `{}`}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertProductRiskReview(ctx, ProductRiskReview{ID: "risk_ops_1", ProductKey: product.ProductKey, OverallScore: 72}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertProposalPackage(ctx, ProposalPackage{ID: "prop_ops_1", ProductKey: product.ProductKey, TargetCustomerType: "bank"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.GetProposalPackage(ctx, "prop_ops_1"); err != nil || !ok {
		t.Fatalf("proposal lookup ok=%v err=%v", ok, err)
	}
	if err := db.InsertProposalFeedback(ctx, ProposalFeedback{ID: "pfb_1", ProposalID: "prop_ops_1", ProductKey: product.ProductKey, Result: "poc"}); err != nil {
		t.Fatal(err)
	}
	feedback, err := db.ListProposalFeedback(ctx, product.ProductKey)
	if err != nil || len(feedback) != 1 {
		t.Fatalf("feedback roundtrip: %+v err=%v", feedback, err)
	}
	if err := db.InsertProductPOCPlan(ctx, ProductPOCPlan{ID: "poc_ops_1", ProductKey: product.ProductKey, SuccessMetric: "AUC"}); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.GetProductPOCPlan(ctx, "poc_ops_1"); err != nil || !ok {
		t.Fatalf("poc lookup ok=%v err=%v", ok, err)
	}
	if err := db.InsertPOCOutcome(ctx, POCOutcome{ID: "pocout_1", POCID: "poc_ops_1", ProductKey: product.ProductKey, Success: true, ConversionStatus: "contract_candidate"}); err != nil {
		t.Fatal(err)
	}
	outcomes, err := db.ListPOCOutcomes(ctx, product.ProductKey)
	if err != nil || len(outcomes) != 1 || !outcomes[0].Success {
		t.Fatalf("outcome roundtrip: %+v err=%v", outcomes, err)
	}
	if err := db.InsertMockAPILog(ctx, MockAPILog{ID: "mock_1", ProductKey: product.ProductKey, RequestHash: "abc"}); err != nil {
		t.Fatal(err)
	}
	funnel, err := db.DataWorksFunnel(ctx, "")
	if err != nil || funnel.Definitions == 0 || funnel.Proposals != 1 || funnel.ProposalFeedback != 1 || funnel.POCOutcomes != 1 || funnel.Published != 1 {
		t.Fatalf("funnel mismatch: %+v err=%v", funnel, err)
	}
}
