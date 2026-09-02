package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/analyzer"
	"dataworks/internal/store"
)

// The classified gap fields must survive JSON encoding. They used to be promoted from an embedded
// analyzer.CollectGap that collided with store.K8sCollectRun on the `category` tag, which made
// encoding/json drop the category from every recent-failure row.
func TestK8sCollectSLOReportsClassifiedFailureCategory(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "fallback.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())

	cfg := testConfig("http://upstream.invalid", "secret")
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(server.Routes())
	defer proxy.Close()

	if err := db.RecordK8sCollectRun(context.Background(), store.K8sCollectRun{
		ID: "run-fail-1", ClusterID: "prod-a", Trigger: "scheduled", Stage: "collect",
		OK: false, ErrorText: "namespaces is forbidden: User cannot list resource",
		LatencyMS: 120, ResourceCount: 0,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RecordK8sCollectRun(context.Background(), store.K8sCollectRun{
		ID: "run-ok-1", ClusterID: "prod-a", Trigger: "scheduled", Stage: "ok",
		OK: true, LatencyMS: 90, ResourceCount: 12,
	}); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(proxy.URL + "/admin/k8s/collect-slo?cluster_id=prod-a")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("collect-slo status=%d body=%s", resp.StatusCode, body)
	}
	var payload struct {
		RecentFailures []map[string]any `json:"recent_failures"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.RecentFailures) != 1 {
		t.Fatalf("expected exactly the one failed run, got %+v", payload.RecentFailures)
	}
	row := payload.RecentFailures[0]
	if row["category"] != analyzer.CollectGapRBAC {
		t.Fatalf("category should be the classified RBAC gap, got %+v", row["category"])
	}
	for _, key := range []string{"title", "likely", "remediation", "confidence"} {
		if s, _ := row[key].(string); s == "" {
			t.Fatalf("%s should be populated in the failure row: %+v", key, row)
		}
	}
	if _, ok := row["cluster_issue"].(bool); !ok {
		t.Fatalf("cluster_issue should be present in the failure row: %+v", row)
	}
	if row["id"] != "run-fail-1" || row["cluster_id"] != "prod-a" {
		t.Fatalf("run fields should stay flattened: %+v", row)
	}
}
