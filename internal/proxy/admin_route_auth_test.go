package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"dataworks/internal/store"
)

// TestAdminRoutesRejectAnonymousRequests pins the admin routes that were reachable
// without a credential. handleLLMPromptCompare and the marketplace handlers are
// registered as their own exact mux patterns, so they never inherit a neighbouring
// handler's guard and must authorize themselves.
func TestAdminRoutesRejectAnonymousRequests(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 32, filepath.Join(t.TempDir(), "fallback.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())

	cfg := testConfig("http://example.invalid", "secret")
	cfg.Auth.AdminToken = "rw-secret"
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(server.Routes())
	defer proxy.Close()

	cases := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/admin/llm/prompts/compare?prompt_name=demo", ""},
		{http.MethodGet, "/admin/dataworks/marketplace/products", ""},
		{http.MethodGet, "/admin/dataworks/marketplace/bookmarks", ""},
		{http.MethodPost, "/admin/dataworks/marketplace/bookmarks", `{"product_key":"demo"}`},
		{http.MethodGet, "/admin/dataworks/marketplace/subscriptions", ""},
		{http.MethodPost, "/admin/dataworks/marketplace/subscriptions", `{"product_key":"demo","purpose":"test"}`},
	}

	for _, tc := range cases {
		var body *strings.Reader
		if tc.body == "" {
			body = strings.NewReader("")
		} else {
			body = strings.NewReader(tc.body)
		}
		req, err := http.NewRequest(tc.method, proxy.URL+tc.path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s without a token should be 401, got %d", tc.method, tc.path, resp.StatusCode)
		}

		req, err = http.NewRequest(tc.method, proxy.URL+tc.path, strings.NewReader(tc.body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer rw-secret")
		resp, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", tc.method, tc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			t.Errorf("%s %s with the admin token should not be 401", tc.method, tc.path)
		}
	}
}
