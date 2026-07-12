package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dataworks/internal/store"
)

func newDataWorksCatalogHealthServer(t *testing.T) (*httptest.Server, *store.SQLStore) {
	t.Helper()
	db := openTestStore(t)
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "catalog-health.ndjson"))
	logger.Start()
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	t.Cleanup(func() {
		srv.Close()
		logger.Stop(context.Background())
		db.Close()
	})
	return srv, db
}

func TestDataWorksCatalogHealthDetectsStoredCorruption(t *testing.T) {
	srv, db := newDataWorksCatalogHealthServer(t)
	ctx := context.Background()
	if err := db.UpsertDataAsset(ctx, store.DataAsset{
		ID: "asset_good", AssetKey: "good_asset", Name: "정상 데이터 자산", Owner: "데이터플랫폼팀",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertDataAsset(ctx, store.DataAsset{
		ID: "asset_bad", AssetKey: "bad_asset", Name: "?? ??? ??", Owner: "?????",
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/admin/dataworks/catalog-health")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, resp, http.StatusOK)
	var health dataWorksCatalogHealth
	decodeAndClose(t, resp, &health)
	if health.Summary.Status != "critical" || health.Summary.CriticalCount != 2 || health.Summary.IssueCount != 2 {
		t.Fatalf("unexpected catalog health summary: %+v issues=%+v", health.Summary, health.Issues)
	}
	for _, issue := range health.Issues {
		if issue.EntityType != "asset" || issue.EntityKey != "bad_asset" || issue.Code != "corrupted_text" {
			t.Fatalf("unexpected catalog health issue: %+v", issue)
		}
	}
}

func TestDataWorksCatalogWritesRejectCorruptedText(t *testing.T) {
	srv, _ := newDataWorksCatalogHealthServer(t)
	tests := []struct {
		name string
		path string
		body map[string]any
	}{
		{name: "asset", path: "/admin/dataworks/assets", body: map[string]any{"asset_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "product", path: "/admin/dataworks/products", body: map[string]any{"product_key": "bad", "name_ko": "?? ???", "owner": "????"}},
		{name: "workspace", path: "/admin/dataworks/workspaces", body: map[string]any{"workspace_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "metadata", path: "/admin/dataworks/metadata/entities", body: map[string]any{"urn": "urn:dw:dataset:bad", "entity_type": "dataset", "name": "?? ???", "owner": "????"}},
		{name: "metric", path: "/admin/dataworks/semantic/metrics", body: map[string]any{"metric_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "glossary", path: "/admin/dataworks/semantic/glossary", body: map[string]any{"term": "?? ???", "mapping": "bad"}},
		{name: "flow", path: "/admin/dataworks/flows", body: map[string]any{"flow_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "agent", path: "/admin/dataworks/agents", body: map[string]any{"agent_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "tool", path: "/admin/dataworks/tools", body: map[string]any{"tool_key": "bad", "name": "?? ???", "owner": "????"}},
		{name: "customer segment", path: "/admin/dataworks/customer-segments", body: map[string]any{"segment_key": "bad", "industry": "?? ???", "buyer_type": "????"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp := postJSON(t, srv.URL+tc.path, "", tc.body)
			requireStatus(t, resp, http.StatusBadRequest)
			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "invalid_text_encoding") {
				t.Fatalf("response missing invalid_text_encoding: %s", body)
			}
		})
	}
}
