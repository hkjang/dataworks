package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

func getAdminJSON(t *testing.T, url string) (int, []byte) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, body
}

// dashboardCountOf reads one integer out of a nested object such as
// {"dashboard":{"total_products":2}} so a silent zero is visible to the assertions below.
func dashboardCountOf(t *testing.T, body []byte, section, field string) int {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode %s.%s: %v: %s", section, field, err, body)
	}
	nested, ok := parsed[section].(map[string]any)
	if !ok {
		t.Fatalf("%s missing or not an object: %s", section, body)
	}
	value, ok := nested[field].(float64)
	if !ok {
		t.Fatalf("%s.%s missing or not a number: %s", section, field, body)
	}
	return int(value)
}

func reviewQueueLen(t *testing.T, body []byte, field string) int {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("decode %s: %v: %s", field, err, body)
	}
	queue, ok := parsed[field].([]any)
	if !ok {
		t.Fatalf("%s missing or not a list: %s", field, body)
	}
	return len(queue)
}

// A read the server could not complete says nothing about how many assets, products, or pending
// risk reviews exist. These three admin GETs dropped the error and answered 200 with zeroes and
// empty queues, so an unreachable table looked exactly like an empty catalog on the screens an
// operator watches — while the sibling reads in the very same handlers (ListDataAssets in
// /assets, the first ListDataProducts in /reviews, FactoryDashboard in /funnel) already report
// the same failure as 500. Exercised against real SQLite failures through the production store
// and router.
func TestDataWorksCatalogReadsRejectUnavailableTables(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "catalog-read.db")
	db, err := store.Open(ctx, config.DatabaseConfig{Driver: "sqlite", DSN: dsn})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "audit.ndjson"))
	logger.Start()
	t.Cleanup(func() { logger.Stop(ctx) })
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(srv.Close)
	schema, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { schema.Close() })

	// Seed through the production admin routes so every row below is one the handlers really read.
	resp := postJSON(t, srv.URL+"/admin/dataworks/assets", "", map[string]any{
		"id": "asset_catalog", "asset_key": "catalog_ledger", "name": "Catalog Ledger", "domain": "credit",
		"owner": "risk-data", "columns_summary": "loan_id, balance", "sensitivity": "internal", "refresh_cycle": "daily",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	for _, product := range []struct{ key, name, status string }{
		{"dw_catalog_review", "검토 대기 상품", "review"},
		{"dw_catalog_risk", "리스크 검토 상품", "risk_review"},
	} {
		resp := postJSON(t, srv.URL+"/admin/dataworks/products", "", map[string]any{
			"product_key": product.key, "name_ko": product.name, "source_type": "api",
			"owner": "data-business", "status": product.status,
		})
		requireStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	assertBaseline := func(t *testing.T, when string) {
		t.Helper()
		status, body := getAdminJSON(t, srv.URL+"/admin/dataworks/home")
		if status != http.StatusOK {
			t.Fatalf("%s: home status = %d, want 200: %s", when, status, body)
		}
		if got := dashboardCountOf(t, body, "dashboard", "total_products"); got != 2 {
			t.Fatalf("%s: home total_products = %d, want 2: %s", when, got, body)
		}
		if got := dashboardCountOf(t, body, "dashboard", "total_assets"); got != 1 {
			t.Fatalf("%s: home total_assets = %d, want 1: %s", when, got, body)
		}
		status, body = getAdminJSON(t, srv.URL+"/admin/dataworks/analytics")
		if status != http.StatusOK {
			t.Fatalf("%s: analytics status = %d, want 200: %s", when, status, body)
		}
		if got := dashboardCountOf(t, body, "analytics", "total_products"); got != 2 {
			t.Fatalf("%s: analytics total_products = %d, want 2: %s", when, got, body)
		}
		status, body = getAdminJSON(t, srv.URL+"/admin/dataworks/reviews")
		if status != http.StatusOK {
			t.Fatalf("%s: reviews status = %d, want 200: %s", when, status, body)
		}
		if got := reviewQueueLen(t, body, "review_pending"); got != 1 {
			t.Fatalf("%s: review_pending = %d, want 1: %s", when, got, body)
		}
		if got := reviewQueueLen(t, body, "risk_review_pending"); got != 1 {
			t.Fatalf("%s: risk_review_pending = %d, want 1: %s", when, got, body)
		}
	}
	assertBaseline(t, "baseline")

	for _, tc := range []struct{ name, path, table, code, hides string }{
		{"home_products", "/admin/dataworks/home", "data_products", "dashboard_failed", "total_products, published_products and high_risk"},
		{"home_assets", "/admin/dataworks/home", "data_assets", "dashboard_failed", "total_assets"},
		{"home_factory_counts", "/admin/dataworks/home", "product_ideas", "dashboard_failed", "ideas_total and poc_pending"},
		{"analytics_products", "/admin/dataworks/analytics", "data_products", "analytics_failed", "total_products and status_breakdown"},
		{"analytics_factory_counts", "/admin/dataworks/analytics", "product_ideas", "analytics_failed", "total_ideas"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Table names come exclusively from the fixed cases above.
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable_catalog"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_catalog RENAME TO "+tc.table); err != nil {
					t.Fatal(err)
				}
				assertBaseline(t, "after restoring "+tc.table)
			})
			status, body := getAdminJSON(t, srv.URL+tc.path)
			if status != http.StatusInternalServerError {
				t.Fatalf("unavailable %s: status = %d, want 500 (%s would be reported as zero): %s", tc.table, status, tc.hides, body)
			}
			if code := errorCodeOf(t, body); code != tc.code {
				t.Errorf("unavailable %s: error code = %q, want %q: %s", tc.table, code, tc.code, body)
			}
		})
	}

	// /reviews reads data_products twice and only the second call carries the risk_review queue,
	// so a lever that breaks the whole table cannot tell the two reads apart: the first one
	// already answers 500. Swap in a view that makes exactly the risk_review rows unscannable
	// (description is NOT NULL in the schema and read without COALESCE) so the review queue is
	// still served normally while the risk queue cannot be read.
	t.Run("reviews_risk_queue_unreadable", func(t *testing.T) {
		columns := []string{}
		rows, err := schema.QueryContext(ctx, "PRAGMA table_info(data_products)")
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, pk int
			var name, colType string
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			if name == "description" {
				columns = append(columns, "CASE WHEN status = 'risk_review' THEN NULL ELSE description END AS description")
				continue
			}
			columns = append(columns, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		if len(columns) == 0 {
			t.Fatal("data_products has no columns")
		}
		if _, err := schema.ExecContext(ctx, "ALTER TABLE data_products RENAME TO data_products_real"); err != nil {
			t.Fatal(err)
		}
		if _, err := schema.ExecContext(ctx, "CREATE VIEW data_products AS SELECT "+strings.Join(columns, ", ")+" FROM data_products_real"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := schema.ExecContext(ctx, "DROP VIEW data_products"); err != nil {
				t.Fatal(err)
			}
			if _, err := schema.ExecContext(ctx, "ALTER TABLE data_products_real RENAME TO data_products"); err != nil {
				t.Fatal(err)
			}
			assertBaseline(t, "after restoring the data_products table")
		})

		// Guard the lever: the review queue must still read cleanly, otherwise this case would
		// pass on the sibling read's pre-existing 500 rather than on the risk queue's error.
		status, body := getAdminJSON(t, srv.URL+"/admin/dataworks/products?status=review")
		if status != http.StatusOK {
			t.Fatalf("review rows are unexpectedly unreadable: status = %d: %s", status, body)
		}
		status, body = getAdminJSON(t, srv.URL+"/admin/dataworks/products?status=risk_review")
		if status != http.StatusInternalServerError {
			t.Fatalf("lever did not break the risk_review read: status = %d: %s", status, body)
		}

		status, body = getAdminJSON(t, srv.URL+"/admin/dataworks/reviews")
		if status != http.StatusInternalServerError {
			t.Fatalf("unreadable risk_review rows: status = %d, want 500 (the risk review queue would look empty): %s", status, body)
		}
		if code := errorCodeOf(t, body); code != "reviews_failed" {
			t.Errorf("unreadable risk_review rows: error code = %q, want %q: %s", code, "reviews_failed", body)
		}
		var parsed map[string]json.RawMessage
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatal(err)
		}
		if _, ok := parsed["risk_review_pending"]; ok {
			t.Errorf("failed response still carries a risk_review_pending queue: %s", body)
		}
	})
}
