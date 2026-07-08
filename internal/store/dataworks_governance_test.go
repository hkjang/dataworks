package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/config"
)

func TestDataWorksGovernanceRoundtrip(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dw.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if err := db.UpsertAssetReadinessScore(ctx, AssetReadinessScore{AssetKey: "loan_history", OverallScore: 91, UpdatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	readiness, err := db.ListAssetReadinessScores(ctx, "loan_history")
	if err != nil || len(readiness) != 1 || readiness[0].OverallScore != 91 {
		t.Fatalf("readiness mismatch: %+v err=%v", readiness, err)
	}

	canvas := ProductCanvasV2{ProductKey: "dw_credit_score", CustomerProblem: "approve safer loans", Buyer: "banks", UpdatedBy: "tester"}
	if err := db.UpsertProductCanvasV2(ctx, canvas); err != nil {
		t.Fatal(err)
	}
	gotCanvas, ok, err := db.GetProductCanvasV2(ctx, "dw_credit_score")
	if err != nil || !ok || gotCanvas.Buyer != "banks" {
		t.Fatalf("canvas mismatch: %+v ok=%v err=%v", gotCanvas, ok, err)
	}

	trace := ApprovalTrace{ID: "appr_1", ProductKey: "dw_credit_score", Step: "legal", Status: "approved", Required: true, EvidenceRef: "memo-1"}
	if err := db.UpsertApprovalTrace(ctx, trace); err != nil {
		t.Fatal(err)
	}
	traces, err := db.ListApprovalTraces(ctx, "dw_credit_score")
	if err != nil || len(traces) != 1 || !traces[0].Required || traces[0].Status != "approved" {
		t.Fatalf("trace mismatch: %+v err=%v", traces, err)
	}

	if err := db.UpsertEvidencePack(ctx, EvidencePack{ProductKey: "dw_credit_score", PackJSON: `{"ready":true}`, CreatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	pack, ok, err := db.GetEvidencePack(ctx, "dw_credit_score")
	if err != nil || !ok || pack.PackJSON == "" {
		t.Fatalf("pack mismatch: %+v ok=%v err=%v", pack, ok, err)
	}

	version, err := db.InsertContractVersion(ctx, ContractVersion{ID: "ctrt_1", ProductKey: "dw_credit_score", ContractJSON: `{"terms":"v1"}`, CreatedBy: "tester"})
	if err != nil || version != 1 {
		t.Fatalf("contract v1=%d err=%v", version, err)
	}
	version, err = db.InsertContractVersion(ctx, ContractVersion{ID: "ctrt_2", ProductKey: "dw_credit_score", ContractJSON: `{"terms":"v2"}`, CreatedBy: "tester"})
	if err != nil || version != 2 {
		t.Fatalf("contract v2=%d err=%v", version, err)
	}
	latest, ok, err := db.LatestContractVersion(ctx, "dw_credit_score")
	if err != nil || !ok || latest.Version != 2 {
		t.Fatalf("latest contract mismatch: %+v ok=%v err=%v", latest, ok, err)
	}

	segment := CustomerSegment{SegmentKey: "bank_enterprise", Industry: "banking", BuyerType: "risk team", PainPoints: []string{"loan approval", "credit risk"}, BudgetLevel: "enterprise"}
	if err := db.UpsertCustomerSegment(ctx, segment); err != nil {
		t.Fatal(err)
	}
	segments, err := db.ListCustomerSegments(ctx, "bank_enterprise")
	if err != nil || len(segments) != 1 || segments[0].PainPoints[0] != "loan approval" {
		t.Fatalf("segment mismatch: %+v err=%v", segments, err)
	}

	if err := db.UpsertProductFitScore(ctx, ProductFitScore{ProductKey: "dw_credit_score", CustomerSegment: "bank_enterprise", FitScore: 87, Reason: "matched", EvidenceRefs: []string{"target_industries"}}); err != nil {
		t.Fatal(err)
	}
	scores, err := db.ListProductFitScores(ctx, "dw_credit_score", "bank_enterprise")
	if err != nil || len(scores) != 1 || scores[0].FitScore != 87 || len(scores[0].EvidenceRefs) != 1 {
		t.Fatalf("fit score mismatch: %+v err=%v", scores, err)
	}

	productVersion, err := db.InsertProductVersion(ctx, ProductVersion{ProductKey: "dw_credit_score", SnapshotJSON: `{"product":{"name":"v1"}}`, DiffSummary: "initial", ChangedBy: "tester"})
	if err != nil || productVersion != 1 {
		t.Fatalf("product version v1=%d err=%v", productVersion, err)
	}
	productVersion, err = db.InsertProductVersion(ctx, ProductVersion{ProductKey: "dw_credit_score", SnapshotJSON: `{"product":{"name":"v2"}}`, DiffSummary: "name changed", ChangedBy: "tester"})
	if err != nil || productVersion != 2 {
		t.Fatalf("product version v2=%d err=%v", productVersion, err)
	}
	versions, err := db.ListProductVersions(ctx, "dw_credit_score")
	if err != nil || len(versions) != 2 || versions[0].Version != 2 {
		t.Fatalf("product versions mismatch: %+v err=%v", versions, err)
	}

	validTo := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)
	scope := ContractScope{
		ContractKey: "ct_scope_1", ProductKey: "dw_credit_score", CustomerKey: "cust_bank",
		AllowedFields: []string{"score", "risk_band"}, RateLimit: 100, ValidTo: validTo, Purpose: "risk monitoring",
	}
	if err := db.UpsertContractScope(ctx, scope); err != nil {
		t.Fatal(err)
	}
	gotScope, ok, err := db.GetContractScope(ctx, "ct_scope_1")
	if err != nil || !ok || len(gotScope.AllowedFields) != 2 || gotScope.RateLimit != 100 {
		t.Fatalf("contract scope mismatch: %+v ok=%v err=%v", gotScope, ok, err)
	}

	ent := APIEntitlement{ID: "ent_1", APIKeyID: "key_bank", CustomerKey: "cust_bank", ProductKey: "dw_credit_score", ContractKey: "ct_scope_1", Scope: "data_product:query", ExpiresAt: validTo}
	if err := db.UpsertAPIEntitlement(ctx, ent); err != nil {
		t.Fatal(err)
	}
	gotEnt, ok, err := db.FindAPIEntitlement(ctx, "dw_credit_score", "key_bank", "")
	if err != nil || !ok || gotEnt.ContractKey != "ct_scope_1" {
		t.Fatalf("entitlement mismatch: %+v ok=%v err=%v", gotEnt, ok, err)
	}

	if err := db.UpsertProductSLA(ctx, ProductSLA{ProductKey: "dw_credit_score", RefreshCycle: "daily", LatencyTargetMS: 250, AvailabilityTarget: 0.995, SupportLevel: "business"}); err != nil {
		t.Fatal(err)
	}
	sla, ok, err := db.GetProductSLA(ctx, "dw_credit_score")
	if err != nil || !ok || sla.LatencyTargetMS != 250 || sla.AvailabilityTarget != 0.995 {
		t.Fatalf("sla mismatch: %+v ok=%v err=%v", sla, ok, err)
	}

	if err := db.UpsertDataWatermark(ctx, DataWatermark{AssetKey: "loan_history", ProductKey: "dw_credit_score", DataAsOf: validTo, DelayStatus: "stale", UpdatedBy: "tester"}); err != nil {
		t.Fatal(err)
	}
	watermarks, err := db.ListDataWatermarks(ctx, "dw_credit_score", "")
	if err != nil || len(watermarks) != 1 || watermarks[0].DelayStatus != "stale" {
		t.Fatalf("watermark mismatch: %+v err=%v", watermarks, err)
	}

	if err := db.UpsertProductCost(ctx, ProductCost{ProductKey: "dw_credit_score", QueryCost: 100, LLMCost: 50, OpsCost: 25, DataProcessingCost: 10, EstimatedMargin: -5, Currency: "KRW"}); err != nil {
		t.Fatal(err)
	}
	cost, ok, err := db.GetProductCost(ctx, "dw_credit_score")
	if err != nil || !ok || cost.EstimatedMargin != -5 {
		t.Fatalf("cost mismatch: %+v ok=%v err=%v", cost, ok, err)
	}

	if err := db.InsertCustomerProposalEvent(ctx, CustomerProposalEvent{ID: "pevt_1", CustomerKey: "cust_bank", ProductKey: "dw_credit_score", ProposalID: "prop_1", Variant: "executive", EventType: "generated"}); err != nil {
		t.Fatal(err)
	}
	events, err := db.ListCustomerProposalEvents(ctx, "dw_credit_score", "cust_bank")
	if err != nil || len(events) != 1 || events[0].Variant != "executive" {
		t.Fatalf("proposal event mismatch: %+v err=%v", events, err)
	}

	if err := db.UpsertRetirementCandidate(ctx, RetirementCandidate{ProductKey: "dw_credit_score", Reason: "negative margin", RiskScore: 80, UsageCount: 0, Recommendation: "retire"}); err != nil {
		t.Fatal(err)
	}
	candidates, err := db.ListRetirementCandidates(ctx, "dw_credit_score")
	if err != nil || len(candidates) != 1 || candidates[0].Recommendation != "retire" {
		t.Fatalf("retirement candidate mismatch: %+v err=%v", candidates, err)
	}
}
