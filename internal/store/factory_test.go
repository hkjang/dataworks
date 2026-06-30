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
