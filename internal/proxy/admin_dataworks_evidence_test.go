package proxy

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"testing"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

// evidenceTypesOf returns the evidence types the endpoint answered with, sorted so a
// missing one is reported by name rather than by position.
func evidenceTypesOf(t *testing.T, body []byte) []string {
	t.Helper()
	var payload struct {
		Evidence []store.ProductEvidence `json:"evidence"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode evidence body %s: %v", body, err)
	}
	types := []string{}
	for _, e := range payload.Evidence {
		types = append(types, e.EvidenceType)
	}
	sort.Strings(types)
	return types
}

// evidenceRequest runs one request against the evidence endpoint and hands back the
// status and body so each case can judge both.
func evidenceRequest(t *testing.T, method string, url string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
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

// A failed read of the sources an evidence pack is assembled from is not evidence that the
// product has no definition version, risk basis, or PoC success metric. The refresh replaces
// the stored pack with a DELETE followed by INSERTs, so a dropped source silently removes the
// governance record a launch decision rests on while the endpoint still answers 200.
// buildEvidencePackJSON reads the same three sources and already reports their errors, so the
// two paths must agree. Exercised against real SQLite failures through the production store
// and router.
func TestDataWorksEvidenceRejectsUnavailableSources(t *testing.T) {
	ctx := context.Background()
	dsn := filepath.Join(t.TempDir(), "evidence.db")
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

	// Walk the production admin routes until dw_evidence_sources holds a definition, a risk
	// review, and a PoC plan, so every optional evidence row below really has a source.
	for _, step := range []struct {
		path string
		body map[string]any
	}{
		{"/admin/dataworks/assets", map[string]any{
			"id": "asset_evidence", "asset_key": "evidence_ledger", "name": "Evidence Ledger", "domain": "credit",
			"owner": "risk-data", "columns_summary": "loan_id, overdue_days, balance",
			"sensitivity": "internal", "refresh_cycle": "daily",
		}},
		{"/admin/dataworks/assets/evidence_ledger/readiness/check", map[string]any{}},
		{"/admin/dataworks/factory/definitions", map[string]any{
			"product_key": "dw_evidence_sources", "title": "증거 출처 상품", "target_industry": "금융",
			"target_customers": []string{"은행"}, "customer_need": "출시 근거 추적",
			"data_assets": []string{"evidence_ledger"}, "delivery_method": "API",
		}},
		{"/admin/dataworks/risk/check", map[string]any{"product_key": "dw_evidence_sources"}},
		{"/admin/dataworks/poc/plans", map[string]any{"product_key": "dw_evidence_sources"}},
	} {
		resp := postJSON(t, srv.URL+step.path, "", step.body)
		requireStatus(t, resp, http.StatusOK)
		resp.Body.Close()
	}

	evidenceURL := srv.URL + "/admin/dataworks/products/dw_evidence_sources/evidence"
	complete := []string{"customer_need", "data_assets", "definition_version", "differentiation", "poc_success_metric", "risk_basis"}
	assertCompletePack := func(t *testing.T, method string) {
		t.Helper()
		status, body := evidenceRequest(t, method, evidenceURL)
		if status != http.StatusOK {
			t.Fatalf("%s evidence: status = %d, want 200: %s", method, status, body)
		}
		got := evidenceTypesOf(t, body)
		if len(got) != len(complete) {
			t.Fatalf("%s evidence types = %v, want %v", method, got, complete)
		}
		for i, want := range complete {
			if got[i] != want {
				t.Fatalf("%s evidence types = %v, want %v", method, got, complete)
			}
		}
	}
	assertCompletePack(t, http.MethodPost)

	for _, tc := range []struct{ table, missing string }{
		{"product_definitions", "definition_version"},
		{"product_risk_reviews", "risk_basis"},
		{"product_poc_plans", "poc_success_metric"},
	} {
		t.Run(tc.table, func(t *testing.T) {
			// Table names come exclusively from the fixed cases above.
			if _, err := schema.ExecContext(ctx, "ALTER TABLE "+tc.table+" RENAME TO unavailable_source"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_source RENAME TO "+tc.table); err != nil {
					t.Fatal(err)
				}
				// The stored pack must still carry every row: a refresh that could not read a
				// source must not have deleted the evidence it was unable to rebuild.
				assertCompletePack(t, http.MethodGet)
			})
			status, body := evidenceRequest(t, http.MethodPost, evidenceURL)
			if status != http.StatusInternalServerError {
				t.Fatalf("unavailable %s: status = %d, want 500 (%s would be dropped silently): %s", tc.table, status, tc.missing, body)
			}
			if code := errorCodeOf(t, body); code != "evidence_refresh_failed" {
				t.Errorf("unavailable %s: error code = %q, want %q", tc.table, code, "evidence_refresh_failed")
			}
			var parsed map[string]json.RawMessage
			if err := json.Unmarshal(body, &parsed); err != nil {
				t.Fatal(err)
			}
			if _, ok := parsed["evidence"]; ok {
				t.Errorf("unavailable %s returned a misleading evidence list: %s", tc.table, body)
			}
		})
	}

	// The read path builds the same pack on the fly for a product that has none stored yet,
	// so it has to refuse the same way instead of answering with a short pack.
	resp := postJSON(t, srv.URL+"/admin/dataworks/products", "", map[string]any{
		"product_key": "dw_evidence_unseeded", "name_ko": "증거 미생성 상품", "source_type": "api", "owner": "data-business",
	})
	requireStatus(t, resp, http.StatusOK)
	resp.Body.Close()
	t.Run("get_builds_without_source", func(t *testing.T) {
		if _, err := schema.ExecContext(ctx, "ALTER TABLE product_risk_reviews RENAME TO unavailable_source"); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := schema.ExecContext(ctx, "ALTER TABLE unavailable_source RENAME TO product_risk_reviews"); err != nil {
				t.Fatal(err)
			}
		})
		status, body := evidenceRequest(t, http.MethodGet, srv.URL+"/admin/dataworks/products/dw_evidence_unseeded/evidence")
		if status != http.StatusInternalServerError {
			t.Fatalf("unseeded product with unavailable risk reviews: status = %d, want 500: %s", status, body)
		}
		if code := errorCodeOf(t, body); code != "evidence_failed" {
			t.Errorf("error code = %q, want %q", code, "evidence_failed")
		}
	})
}
