package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"dataworks/internal/config"
)

func TestDataProductRoundtripAndRequests(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dp.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	p := DataProduct{
		ProductKey: "team-cost-daily", NameKO: "팀 일별 비용", Description: "daily team cost",
		SourceType: "saved_report", SourceRef: "rep_123", Owner: "data-team",
		AllowedTeams: []string{"alpha", "beta"}, Sensitivity: "internal", Status: "published",
	}
	p.ID = "dp_1"
	if err := db.UpsertDataProduct(ctx, p); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.GetDataProduct(ctx, "team-cost-daily")
	if err != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, err)
	}
	if got.NameKO != "팀 일별 비용" || got.SourceType != "saved_report" || got.Status != "published" {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if len(got.AllowedTeams) != 2 || got.AllowedTeams[0] != "alpha" {
		t.Fatalf("allowed_teams not preserved: %+v", got.AllowedTeams)
	}

	// Published-only filter.
	pub, err := db.ListDataProducts(ctx, "published")
	if err != nil || len(pub) != 1 {
		t.Fatalf("published list: n=%d err=%v", len(pub), err)
	}
	drafts, _ := db.ListDataProducts(ctx, "draft")
	if len(drafts) != 0 {
		t.Fatalf("expected 0 drafts, got %d", len(drafts))
	}

	// Update bumps version.
	if err := db.UpsertDataProduct(ctx, got); err != nil {
		t.Fatal(err)
	}
	got2, _, _ := db.GetDataProduct(ctx, "team-cost-daily")
	if got2.Version != 2 {
		t.Fatalf("expected version 2 after update, got %d", got2.Version)
	}

	// Access request lifecycle.
	if err := db.AddDataProductAccessRequest(ctx, DataProductAccessRequest{ID: "dpreq_1", ProductKey: "team-cost-daily", UserID: "u1", Team: "gamma", Reason: "need it"}); err != nil {
		t.Fatal(err)
	}
	reqs, err := db.ListDataProductAccessRequests(ctx, "team-cost-daily")
	if err != nil || len(reqs) != 1 || reqs[0].Status != "pending" {
		t.Fatalf("requests: %+v err=%v", reqs, err)
	}
	if err := db.DecideDataProductAccessRequest(ctx, "dpreq_1", true, "admin_z"); err != nil {
		t.Fatal(err)
	}
	reqs, _ = db.ListDataProductAccessRequests(ctx, "")
	if reqs[0].Status != "approved" || reqs[0].DecidedBy != "admin_z" {
		t.Fatalf("decision not applied: %+v", reqs[0])
	}

	// Product-scoped governance history is retained when the catalog row is
	// deleted. The retired-key tombstone prevents a new product from inheriting it.
	if err := db.UpsertProductCanvasV2(ctx, ProductCanvasV2{
		ProductKey: p.ProductKey, CustomerProblem: "old canvas", UpdatedBy: "tester",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertApprovalTrace(ctx, ApprovalTrace{
		ID: "approval_old", ProductKey: p.ProductKey, Step: "legal", Status: "approved",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.InsertContractVersion(ctx, ContractVersion{
		ID: "contract_old", ProductKey: p.ProductKey, ContractJSON: `{"version":"old"}`,
	}); err != nil {
		t.Fatal(err)
	}

	if err := db.DeleteDataProduct(ctx, "team-cost-daily"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := db.GetDataProduct(ctx, "team-cost-daily"); ok {
		t.Fatal("expected product deleted")
	}
	if canvas, ok, err := db.GetProductCanvasV2(ctx, p.ProductKey); err != nil || !ok || canvas.CustomerProblem != "old canvas" {
		t.Fatalf("retained canvas mismatch: %+v ok=%v err=%v", canvas, ok, err)
	}
	if traces, err := db.ListApprovalTraces(ctx, p.ProductKey); err != nil || len(traces) != 1 {
		t.Fatalf("retained approval traces: %+v err=%v", traces, err)
	}
	if contract, ok, err := db.LatestContractVersion(ctx, p.ProductKey); err != nil || !ok || contract.ID != "contract_old" {
		t.Fatalf("retained contract mismatch: %+v ok=%v err=%v", contract, ok, err)
	}

	replacement := p
	replacement.ID = "dp_replacement"
	replacement.NameKO = "동일 키 신규 상품"
	err = db.UpsertDataProduct(ctx, replacement)
	var retiredErr *DataProductKeyRetiredError
	if !errors.As(err, &retiredErr) || retiredErr.ProductKey != p.ProductKey {
		t.Fatalf("reuse error = %v, want retired-key error for %q", err, p.ProductKey)
	}
	var productID string
	if err := db.db.QueryRowContext(ctx, `SELECT product_id FROM data_product_retired_keys WHERE product_key = ?`, p.ProductKey).Scan(&productID); err != nil {
		t.Fatal(err)
	}
	if productID != p.ID {
		t.Fatalf("retired product id = %q, want %q", productID, p.ID)
	}
}

func TestDeleteDataProductRollsBackTombstoneOnDeleteFailure(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dp-rollback.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	p := DataProduct{ID: "dp_atomic", ProductKey: "atomic-delete", NameKO: "원자 삭제", Status: "draft"}
	if err := db.UpsertDataProduct(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `CREATE TRIGGER fail_data_product_delete
		BEFORE DELETE ON data_products BEGIN SELECT RAISE(ABORT, 'forced delete failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteDataProduct(ctx, p.ProductKey); err == nil {
		t.Fatal("expected forced delete failure")
	}

	if _, ok, err := db.GetDataProduct(ctx, p.ProductKey); err != nil || !ok {
		t.Fatalf("product must survive rolled-back delete: ok=%v err=%v", ok, err)
	}
	var tombstones int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_product_retired_keys WHERE product_key = ?`, p.ProductKey).Scan(&tombstones); err != nil {
		t.Fatal(err)
	}
	if tombstones != 0 {
		t.Fatalf("rolled-back delete left %d tombstones", tombstones)
	}
	if _, err := db.db.ExecContext(ctx, `DROP TRIGGER fail_data_product_delete`); err != nil {
		t.Fatal(err)
	}
	p.NameKO = "원자 삭제 수정"
	if err := db.UpsertDataProduct(ctx, p); err != nil {
		t.Fatalf("rolled-back tombstone must not block update: %v", err)
	}
}

func TestDeleteDataProductByKeyDoesNotDeleteCollidingID(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "dp-exact-key.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	const collidingValue = "shared_identifier"
	target := DataProduct{ID: "dprod_exact_target", ProductKey: collidingValue, NameKO: "키로 삭제할 상품", Status: "draft"}
	collidingID := DataProduct{ID: collidingValue, ProductKey: "different_product_key", NameKO: "보존할 상품", Status: "draft"}
	if err := db.UpsertDataProduct(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertDataProduct(ctx, collidingID); err != nil {
		t.Fatal(err)
	}

	got, ok, err := db.GetDataProductByKey(ctx, collidingValue)
	if err != nil || !ok || got.ID != target.ID {
		t.Fatalf("exact-key lookup = %+v ok=%v err=%v", got, ok, err)
	}
	if err := db.DeleteDataProductByKey(ctx, collidingValue); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := db.GetDataProductByKey(ctx, target.ProductKey); err != nil || ok {
		t.Fatalf("target remains after exact-key delete: ok=%v err=%v", ok, err)
	}
	preserved, ok, err := db.GetDataProductByKey(ctx, collidingID.ProductKey)
	if err != nil || !ok || preserved.ID != collidingID.ID {
		t.Fatalf("colliding id row was not preserved: %+v ok=%v err=%v", preserved, ok, err)
	}
	var targetTombstones, preservedTombstones int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_product_retired_keys WHERE product_key = ?`, target.ProductKey).Scan(&targetTombstones); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_product_retired_keys WHERE product_key = ?`, collidingID.ProductKey).Scan(&preservedTombstones); err != nil {
		t.Fatal(err)
	}
	if targetTombstones != 1 || preservedTombstones != 0 {
		t.Fatalf("tombstones target=%d preserved=%d", targetTombstones, preservedTombstones)
	}
}
