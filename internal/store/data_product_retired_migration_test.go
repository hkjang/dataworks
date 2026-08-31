package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"dataworks/internal/config"
)

func TestDataProductRetiredKeysMigrationBackfillsOnlyOrphans(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "retired-migration.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	if _, err := db.db.ExecContext(ctx, `DELETE FROM data_product_retired_keys`); err != nil {
		t.Fatal(err)
	}
	currentKey := "current_catalog_product"
	if err := db.UpsertDataProduct(ctx, DataProduct{
		ID: "dprod_current", ProductKey: currentKey, NameKO: "현재 상품", Status: "draft",
	}); err != nil {
		t.Fatal(err)
	}

	now := "2026-08-31T00:00:00Z"
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO dw_product_canvases (product_key, customer_problem, created_at, updated_at) VALUES (?, 'old canvas', ?, ?)`, []any{"orphan_canvas", now, now}},
		{`INSERT INTO dw_product_canvases (product_key, customer_problem, created_at, updated_at) VALUES (?, 'current canvas', ?, ?)`, []any{currentKey, now, now}},
		{`INSERT INTO dw_approval_traces (id, product_key, step, created_at, updated_at) VALUES ('approval_orphan', ?, 'legal', ?, ?)`, []any{"orphan_approval", now, now}},
		{`INSERT INTO dw_contract_versions (id, product_key, contract_json, created_at) VALUES ('contract_orphan', ?, '{}', ?)`, []any{"orphan_contract", now}},
		{`INSERT INTO dw_evidence_packs (product_key, pack_json, created_at, updated_at) VALUES (?, '{}', ?, ?)`, []any{"orphan_evidence", now, now}},
		{`INSERT INTO dw_product_evidence (id, product_key, summary, created_at) VALUES ('evidence_orphan', ?, 'retained evidence', ?)`, []any{"orphan_product_evidence", now}},
		{`INSERT INTO dw_api_contracts (id, product_key, openapi_json, created_at, updated_at) VALUES ('api_contract_orphan', ?, '{}', ?, ?)`, []any{"orphan_api_contract", now, now}},
		{`INSERT INTO dw_product_versions (product_key, version, snapshot_json, changed_at) VALUES (?, 1, '{}', ?)`, []any{"orphan_product_version", now}},
		{`INSERT INTO data_product_access_requests (id, product_key, created_at) VALUES ('request_orphan', ?, ?)`, []any{"orphan_access_request", now}},
		{`INSERT INTO dw_unit_economics (id, product_key, updated_at) VALUES ('economics_orphan', ?, ?)`, []any{"orphan_unit_economics", now}},
		{`INSERT INTO dw_product_relationships (from_type, from_key, to_type, to_key, relation_type, created_at, updated_at)
			VALUES ('asset', 'source_asset', 'product', ?, 'feeds', ?, ?)`, []any{"orphan_relationship", now, now}},
		{`INSERT INTO dw_metadata_entities (id, urn, entity_type, name, source_ref, created_at, updated_at)
			VALUES ('metadata_orphan', 'urn:dw:product:orphan_metadata', 'product', 'Old Product', ?, ?, ?)`, []any{"orphan_metadata", now, now}},
	}
	for _, statement := range statements {
		if _, err := db.db.ExecContext(ctx, statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.ExecContext(ctx, `INSERT INTO data_product_retired_keys
		(product_key, product_id, retired_at) VALUES ('already_retired', 'dprod_original', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 129`); err != nil {
		t.Fatal(err)
	}

	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for _, productKey := range []string{
		"orphan_canvas",
		"orphan_approval",
		"orphan_contract",
		"orphan_evidence",
		"orphan_product_evidence",
		"orphan_api_contract",
		"orphan_product_version",
		"orphan_access_request",
		"orphan_unit_economics",
		"orphan_relationship",
		"orphan_metadata",
	} {
		var count int
		if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_product_retired_keys WHERE product_key = ?`, productKey).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("backfilled tombstones for %q = %d, want 1", productKey, count)
		}
	}

	var currentRetired int
	if err := db.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_product_retired_keys WHERE product_key = ?`, currentKey).Scan(&currentRetired); err != nil {
		t.Fatal(err)
	}
	if currentRetired != 0 {
		t.Fatalf("current catalog key was incorrectly retired")
	}
	if _, ok, err := db.GetDataProduct(ctx, currentKey); err != nil || !ok {
		t.Fatalf("current catalog product missing after migration: ok=%v err=%v", ok, err)
	}
	reuseErr := db.UpsertDataProduct(ctx, DataProduct{
		ID: "dprod_reused_orphan", ProductKey: "orphan_canvas", NameKO: "재사용 시도", Status: "draft",
	})
	var retiredErr *DataProductKeyRetiredError
	if !errors.As(reuseErr, &retiredErr) || retiredErr.ProductKey != "orphan_canvas" {
		t.Fatalf("backfilled key reuse error = %v, want retired-key error", reuseErr)
	}

	var productID, retiredAt string
	if err := db.db.QueryRowContext(ctx, `SELECT product_id, retired_at FROM data_product_retired_keys WHERE product_key = 'already_retired'`).Scan(&productID, &retiredAt); err != nil {
		t.Fatal(err)
	}
	if productID != "dprod_original" || retiredAt != "2026-01-01T00:00:00Z" {
		t.Fatalf("existing tombstone overwritten: product_id=%q retired_at=%q", productID, retiredAt)
	}
}
