package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
)

func TestDataWorksFactoryOperationsRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "factory-ops.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	run := FactoryRun{
		ID: "frun_source", RunType: "products.define", Model: "rules:v1", PromptVersion: "1",
		InputHash: "input-abc", OutputRef: "dw_credit_score", PolicyDecision: "approved",
		TokenCost: 0.12, Status: "completed", CreatedBy: "tester",
	}
	if err := db.InsertFactoryRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	gotRun, ok, err := db.GetFactoryRun(ctx, run.ID)
	if err != nil || !ok || gotRun.PromptVersion != "1" || gotRun.TokenCost != 0.12 || gotRun.Status != "completed" {
		t.Fatalf("factory run mismatch: %+v ok=%v err=%v", gotRun, ok, err)
	}

	v1, err := db.InsertDataWorksPromptTemplate(ctx, DataWorksPromptTemplate{
		TemplateKey: "products.define", RunType: "products.define", TemplateBody: "Create {{product}}", Status: "active", CreatedBy: "tester",
	})
	if err != nil || v1.Version != 1 {
		t.Fatalf("prompt v1 mismatch: %+v err=%v", v1, err)
	}
	v2, err := db.InsertDataWorksPromptTemplate(ctx, DataWorksPromptTemplate{
		TemplateKey: "products.define", RunType: "products.define", TemplateBody: "Create evidence-backed {{product}}", Status: "active", CreatedBy: "tester",
	})
	if err != nil || v2.Version != 2 {
		t.Fatalf("prompt v2 mismatch: %+v err=%v", v2, err)
	}
	templates, err := db.ListDataWorksPromptTemplates(ctx, "products.define", "", "", 10)
	if err != nil || len(templates) != 2 || templates[0].Version != 2 || templates[0].Status != "active" || templates[1].Status != "retired" {
		t.Fatalf("prompt versions mismatch: %+v err=%v", templates, err)
	}
	active, ok, err := db.GetDataWorksPromptTemplate(ctx, "products.define", 0)
	if err != nil || !ok || active.Version != 2 {
		t.Fatalf("active prompt mismatch: %+v ok=%v err=%v", active, ok, err)
	}
	if _, err := db.InsertDataWorksPromptTemplate(ctx, DataWorksPromptTemplate{
		TemplateKey: "products.define", RunType: "ideas.generate", TemplateBody: "wrong run type", Status: "active",
	}); err == nil {
		t.Fatal("expected run_type change for an existing template_key to fail")
	}

	if err := db.InsertFactoryEvalScore(ctx, FactoryEvalScore{
		ID: "feval_1", RunID: run.ID, AccuracyScore: 90, UsefulnessScore: 85, RiskScore: 95,
		OutputQualityScore: 90, ReviewComment: "ready", Reviewer: "reviewer",
	}); err != nil {
		t.Fatal(err)
	}
	evaluations, err := db.ListFactoryEvalScores(ctx, run.ID, 10)
	if err != nil || len(evaluations) != 1 || evaluations[0].OutputQualityScore != 90 {
		t.Fatalf("factory evaluation mismatch: %+v err=%v", evaluations, err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	if err := db.UpsertProductFunnelDaily(ctx, ProductFunnelDaily{
		Date: today, Ideas: 10, Definitions: 8, Reviews: 6, Proposals: 4, POCs: 2, Published: 1,
	}); err != nil {
		t.Fatal(err)
	}
	history, err := db.ListProductFunnelDaily(ctx, today, 30)
	if err != nil || len(history) != 1 || history[0].Proposals != 4 {
		t.Fatalf("funnel history mismatch: %+v err=%v", history, err)
	}

	rel := ProductRelationship{
		FromType: "asset", FromKey: "loan_history", ToType: "product", ToKey: "dw_credit_score", RelationType: "feeds", Weight: 0.9,
	}
	if err := db.UpsertProductRelationship(ctx, rel); err != nil {
		t.Fatal(err)
	}
	rel.Weight = 1
	if err := db.UpsertProductRelationship(ctx, rel); err != nil {
		t.Fatal(err)
	}
	relationships, err := db.ListProductRelationships(ctx, "product", "dw_credit_score", 10)
	if err != nil || len(relationships) != 1 || relationships[0].Weight != 1 {
		t.Fatalf("product relationship mismatch: %+v err=%v", relationships, err)
	}
}

func TestDataWorksFactoryOperationsMigrateFromVersion70(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "factory-v70.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.db.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (70, '2026-07-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `CREATE TABLE factory_runs (
		id TEXT PRIMARY KEY, run_type TEXT NOT NULL DEFAULT '', model TEXT NOT NULL DEFAULT '',
		input_hash TEXT NOT NULL DEFAULT '', output_ref TEXT NOT NULL DEFAULT '', latency_ms INTEGER NOT NULL DEFAULT 0,
		created_by TEXT NOT NULL DEFAULT '', created_at TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `CREATE TABLE data_assets (
		asset_key TEXT PRIMARY KEY, name TEXT NOT NULL, owner TEXT NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `INSERT INTO data_assets (asset_key, name, owner) VALUES
		('card_transaction_signals', '?? ???? ??', '?????'),
		('company_risk_features', '?? ??? ??', '?????'),
		('credit_bureau_history', '???? ??', 'CB????')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	upgraded := FactoryRun{
		ID: "frun_upgraded", RunType: "ideas.generate", Model: "rules:v2", PromptVersion: "3",
		InputHash: "hash", ParentRunID: "frun_old", PolicyDecision: "approved_for_replay",
		TokenCost: 1.25, Status: "replayed", CreatedBy: "tester",
	}
	if err := db.InsertFactoryRun(ctx, upgraded); err != nil {
		t.Fatal(err)
	}
	got, ok, err := db.GetFactoryRun(ctx, upgraded.ID)
	if err != nil || !ok || got.ParentRunID != "frun_old" || got.Status != "replayed" {
		t.Fatalf("upgraded factory run mismatch: %+v ok=%v err=%v", got, ok, err)
	}
	var version int
	if err := db.db.QueryRowContext(ctx, `SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 127 {
		t.Fatalf("schema version=%d err=%v", version, err)
	}
	for _, tc := range []struct {
		key, name, owner string
	}{
		{key: "card_transaction_signals", name: "카드 트랜잭션 신호", owner: "카드사업팀"},
		{key: "company_risk_features", name: "기업 리스크 특성", owner: "기업정보팀"},
		{key: "credit_bureau_history", name: "신용평가 이력", owner: "CB사업본부"},
	} {
		var name, owner string
		if err := db.db.QueryRowContext(ctx, `SELECT name, owner FROM data_assets WHERE asset_key = ?`, tc.key).Scan(&name, &owner); err != nil {
			t.Fatal(err)
		}
		if name != tc.name || owner != tc.owner {
			t.Fatalf("repaired asset %s = (%q, %q), want (%q, %q)", tc.key, name, owner, tc.name, tc.owner)
		}
	}
}
