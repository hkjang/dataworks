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

// Exercise aggregate failures through the production store and routes. In particular,
// keep product_ideas readable: its error was already propagated before this regression.
func TestFactoryDashboardRejectsUnavailableAggregates(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "factory-aggregates.db")
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

	endpoints := []struct {
		path, code, section string
		fields              []string
	}{
		{"/admin/factory/dashboard", "dashboard_failed", "dashboard", []string{
			"ideas_total", "draft_products", "review_products", "risk_review_products", "approved_products",
			"published_products", "archived_products", "high_risk_reviews", "pending_poc_plans", "average_revenue_score",
		}},
		{"/admin/dataworks/funnel", "funnel_failed", "funnel", nil},
		{"/admin/dataworks/home", "dashboard_failed", "dashboard", []string{
			"total_assets", "total_products", "published_products", "review_pending", "high_risk", "poc_pending", "ideas_total", "avg_revenue_score",
		}},
		{"/admin/dataworks/analytics", "analytics_failed", "analytics", []string{
			"total_products", "total_ideas", "avg_revenue", "avg_risk", "published", "archived",
		}},
	}
	assertBaseline := func(t *testing.T) {
		t.Helper()
		for _, endpoint := range endpoints {
			status, body := getAdminJSON(t, srv.URL+endpoint.path)
			if status != http.StatusOK {
				t.Fatalf("baseline %s: status = %d, want 200: %s", endpoint.path, status, body)
			}
			for _, field := range endpoint.fields {
				if got := dashboardCountOf(t, body, endpoint.section, field); got != 0 {
					t.Errorf("baseline %s: %s = %d, want 0", endpoint.path, field, got)
				}
			}
			if endpoint.section == "funnel" {
				var parsed struct {
					Funnel []struct {
						Count *int `json:"count"`
					} `json:"funnel"`
				}
				if err := json.Unmarshal(body, &parsed); err != nil {
					t.Fatal(err)
				}
				if len(parsed.Funnel) != 5 {
					t.Fatalf("baseline funnel: want five stages: %s", body)
				}
				for _, stage := range parsed.Funnel {
					if stage.Count == nil || *stage.Count != 0 {
						t.Errorf("baseline funnel: want zero count: %s", body)
					}
				}
			}
		}
	}
	assertBaseline(t)

	for _, tc := range []struct {
		name, table string
		averageOnly bool
		allRoutes   bool
	}{
		{"ideas", "product_ideas", false, true},
		{"products", "data_products", false, false},
		{"risk_reviews", "product_risk_reviews", false, true},
		{"poc_plans", "product_poc_plans", false, true},
		{"average_revenue", "data_products", true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Identifiers are fixed test cases; each subtest restores the same isolated DB.
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable_aggregate"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if tc.averageOnly {
					if _, err := schema.ExecContext(ctx, "DROP VIEW IF EXISTS data_products"); err != nil {
						t.Error(err)
					}
				}
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_aggregate RENAME TO "+tc.table); err != nil {
					t.Fatal(err)
				}
				assertBaseline(t)
			})
			if tc.table != "product_ideas" {
				var n int64
				if err := schema.QueryRowContext(ctx, "SELECT COUNT(*) FROM product_ideas").Scan(&n); err != nil {
					t.Fatalf("ideas must remain readable: %v", err)
				}
			}
			if tc.averageOnly {
				if _, err := schema.ExecContext(ctx, "CREATE VIEW data_products AS SELECT status FROM unavailable_aggregate"); err != nil {
					t.Fatal(err)
				}
				// Guard the lever using the aggregate SQL: every status count still works,
				// and only the last query needs the deliberately absent revenue_score.
				for _, status := range []string{"draft", "review", "risk_review", "approved", "published", "archived"} {
					var n int64
					if err := schema.QueryRowContext(ctx, "SELECT COUNT(*) FROM data_products WHERE status = ?", status).Scan(&n); err != nil || n != 0 {
						t.Fatalf("status count %s: count = %d, err = %v", status, n, err)
					}
				}
				var avg float64
				if err := schema.QueryRowContext(ctx, "SELECT COALESCE(AVG(revenue_score), 0) FROM data_products WHERE revenue_score > 0").Scan(&avg); err == nil {
					t.Fatal("lever did not break the average query")
				}
			}
			for i, endpoint := range endpoints {
				if i >= 2 && !tc.allRoutes {
					continue
				}
				t.Run(endpoint.path, func(t *testing.T) {
					status, body := getAdminJSON(t, srv.URL+endpoint.path)
					if status != http.StatusInternalServerError {
						t.Fatalf("unavailable %s: status = %d, want 500: %s", tc.name, status, body)
					}
					if code := errorCodeOf(t, body); code != endpoint.code {
						t.Errorf("error code = %q, want %q: %s", code, endpoint.code, body)
					}
					var parsed map[string]json.RawMessage
					if err := json.Unmarshal(body, &parsed); err != nil {
						t.Fatal(err)
					}
					if len(parsed) != 1 || parsed["error"] == nil {
						t.Errorf("failed response must contain only the error, without KPI data: %s", body)
					}
				})
			}
		})
	}
}
