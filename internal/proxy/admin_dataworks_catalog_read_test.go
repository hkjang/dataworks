package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

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

// The legacy factory screen consumes all three sources in one response. Break the
// idea SELECT independently of its COUNT so a missing idea-error check cannot hide
// behind the dashboard-error check.
func TestFactoryProductsRejectsUnavailableSources(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "factory-products.db")
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

	type productsResponse struct {
		Products  []store.DataProduct `json:"products"`
		Ideas     []store.ProductIdea `json:"ideas"`
		Dashboard map[string]int      `json:"dashboard"`
	}
	readProducts := func(t *testing.T, query string) productsResponse {
		t.Helper()
		status, body := getAdminJSON(t, srv.URL+"/admin/factory/products"+query)
		if status != http.StatusOK {
			t.Fatalf("products%s: status = %d, want 200: %s", query, status, body)
		}
		var parsed productsResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(body, &fields); err != nil {
			t.Fatal(err)
		}
		if len(fields) != 3 || parsed.Products == nil || parsed.Ideas == nil || parsed.Dashboard == nil {
			t.Fatalf("want products/ideas arrays and dashboard only: %s", body)
		}
		return parsed
	}
	wantDashboard := map[string]int{
		"ideas_total": 0, "draft_products": 0, "review_products": 0,
		"risk_review_products": 0, "approved_products": 0, "published_products": 0,
		"archived_products": 0, "high_risk_reviews": 0, "pending_poc_plans": 0,
		"average_revenue_score": 0,
	}
	empty := readProducts(t, "")
	if len(empty.Products) != 0 || len(empty.Ideas) != 0 || !reflect.DeepEqual(empty.Dashboard, wantDashboard) {
		t.Fatalf("empty database response = %+v", empty)
	}

	for i, status := range []string{"draft", "review"} {
		resp := postJSON(t, srv.URL+"/admin/dataworks/products", "", map[string]any{
			"product_key": "factory_" + status, "name_ko": "Factory " + status,
			"source_type": "api", "owner": "data-business", "status": status,
		})
		requireStatus(t, resp, http.StatusOK)
		resp.Body.Close()
		// Make newest-first ordering deterministic without sleeping.
		if _, err := schema.ExecContext(ctx, "UPDATE data_products SET created_at = ? WHERE product_key = ?",
			fmt.Sprintf("2026-01-0%dT00:00:00Z", i+1), "factory_"+status); err != nil {
			t.Fatal(err)
		}
	}
	var generated []store.ProductIdea
	generateIdeas := func(count int) {
		t.Helper()
		resp := postJSON(t, srv.URL+"/admin/factory/ideas/generate", "", map[string]any{
			"industry": "금융", "market_need": "리스크 조기탐지", "data_assets": []string{"loan_history"}, "count": count,
		})
		requireStatus(t, resp, http.StatusOK)
		var parsed struct {
			Ideas []store.ProductIdea `json:"ideas"`
		}
		err := json.NewDecoder(resp.Body).Decode(&parsed)
		resp.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if len(parsed.Ideas) == 0 {
			t.Fatal("generation returned no ideas")
		}
		for _, idea := range parsed.Ideas {
			stamp := time.Date(2026, 1, 1, 0, len(generated), 0, 0, time.UTC).Format(time.RFC3339)
			if _, err := schema.ExecContext(ctx, "UPDATE product_ideas SET created_at = ?, updated_at = ? WHERE id = ?", stamp, stamp, idea.ID); err != nil {
				t.Fatal(err)
			}
			idea.CreatedAt, idea.UpdatedAt = stamp, stamp
			generated = append(generated, idea)
		}
	}
	generateIdeas(1) // Use the actual returned count; the generator may impose a minimum.
	wantIdeas := func() []store.ProductIdea {
		out := []store.ProductIdea{}
		for i := len(generated) - 1; i >= 0 && len(out) < 50; i-- {
			out = append(out, generated[i])
		}
		return out
	}
	baseline := readProducts(t, "")
	wantDashboard["ideas_total"] = len(generated)
	wantDashboard["draft_products"], wantDashboard["review_products"] = 1, 1
	if len(baseline.Products) != 2 || baseline.Products[0].ProductKey != "factory_review" || baseline.Products[1].ProductKey != "factory_draft" {
		t.Fatalf("products not returned newest first: %+v", baseline.Products)
	}
	if !reflect.DeepEqual(baseline.Ideas, wantIdeas()) || !reflect.DeepEqual(baseline.Dashboard, wantDashboard) {
		t.Fatalf("populated response = %+v, want ideas %+v and dashboard %+v", baseline, wantIdeas(), wantDashboard)
	}
	status, body := getAdminJSON(t, srv.URL+"/admin/dataworks/products")
	var catalog productsResponse
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK || !reflect.DeepEqual(baseline.Products, catalog.Products) {
		t.Fatalf("legacy product content differs from catalog: status = %d: %s", status, body)
	}
	filtered := baseline
	filtered.Products = baseline.Products[:1]
	assertBaseline := func(t *testing.T) {
		t.Helper()
		for query, want := range map[string]productsResponse{"": baseline, "?status=%20review%20": filtered} {
			if got := readProducts(t, query); !reflect.DeepEqual(got, want) {
				t.Errorf("products%s differs from complete baseline: got %+v, want %+v", query, got, want)
			}
		}
	}
	assertBaseline(t)

	for _, tc := range []struct {
		name, table string
		ideasOnly   bool
	}{
		{"ideas_only", "product_ideas", true},
		{"risk_reviews", "product_risk_reviews", false},
		{"poc_plans", "product_poc_plans", false},
		{"products", "data_products", false}, // Preserve the existing first-read error contract too.
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable_factory_source"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if tc.ideasOnly {
					if _, err := schema.ExecContext(ctx, "DROP VIEW IF EXISTS product_ideas"); err != nil {
						t.Error(err)
					}
				}
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_factory_source RENAME TO "+tc.table); err != nil {
					t.Fatal(err)
				}
				assertBaseline(t)
			})
			if tc.ideasOnly {
				if _, err := schema.ExecContext(ctx, "CREATE VIEW product_ideas AS SELECT id FROM unavailable_factory_source"); err != nil {
					t.Fatal(err)
				}
				status, body := getAdminJSON(t, srv.URL+"/admin/factory/dashboard")
				if status != http.StatusOK || dashboardCountOf(t, body, "dashboard", "ideas_total") != len(generated) {
					t.Fatalf("idea SELECT failure must leave dashboard counts readable: status = %d: %s", status, body)
				}
			} else if tc.table != "data_products" {
				// Guard that the dashboard, not either earlier list read, is broken.
				products, err := db.ListDataProducts(ctx, "")
				if err != nil || !reflect.DeepEqual(products, baseline.Products) {
					t.Fatalf("products unexpectedly unreadable: %v", err)
				}
				ideas, err := db.ListProductIdeas(ctx, "", 50)
				if err != nil || !reflect.DeepEqual(ideas, baseline.Ideas) {
					t.Fatalf("ideas unexpectedly unreadable: %v", err)
				}
				status, body := getAdminJSON(t, srv.URL+"/admin/factory/dashboard")
				if status != http.StatusInternalServerError || errorCodeOf(t, body) != "dashboard_failed" {
					t.Fatalf("lever did not break dashboard: status = %d: %s", status, body)
				}
			}
			status, body := getAdminJSON(t, srv.URL+"/admin/factory/products")
			if status != http.StatusInternalServerError {
				t.Fatalf("unavailable %s: status = %d, want 500: %s", tc.name, status, body)
			}
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatal(err)
			}
			if len(parsed) != 1 || parsed["error"] == nil {
				t.Fatalf("failed response must contain only error: %s", body)
			}
			var apiError struct{ Type, Code string }
			if err := json.Unmarshal(parsed["error"], &apiError); err != nil {
				t.Fatal(err)
			}
			if apiError.Type != "server_error" || apiError.Code != "products_failed" {
				t.Errorf("wrong failure contract: %s", body)
			}
		})
	}

	// Keep the legacy 50-idea cap while the KPI still counts the entire catalog.
	generateIdeas(20)
	generateIdeas(20)
	generateIdeas(20)
	limited := readProducts(t, "?status=%20review%20")
	wantDashboard["ideas_total"] = len(generated)
	if len(limited.Ideas) != 50 || !reflect.DeepEqual(limited.Ideas, wantIdeas()) ||
		!reflect.DeepEqual(limited.Products, filtered.Products) || !reflect.DeepEqual(limited.Dashboard, wantDashboard) {
		t.Fatalf("limited response lost content, order, or global counts: %+v", limited)
	}
}
