package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"dataworks/internal/store"
)

func TestContractRateLimiterWindows(t *testing.T) {
	limiter := &contractRateLimiter{}
	base := time.Date(2026, 9, 4, 10, 30, 20, 0, time.UTC)

	if allowed, used, reset := limiter.allow("ct_a", 2, base); !allowed || used != 1 || !reset.Equal(base.Truncate(time.Minute).Add(time.Minute)) {
		t.Fatalf("first call: allowed=%v used=%d reset=%s", allowed, used, reset)
	}
	if allowed, used, _ := limiter.allow("ct_a", 2, base.Add(5*time.Second)); !allowed || used != 2 {
		t.Fatalf("second call: allowed=%v used=%d", allowed, used)
	}
	if allowed, used, _ := limiter.allow("ct_a", 2, base.Add(10*time.Second)); allowed || used != 2 {
		t.Fatalf("third call should be rejected without consuming the window: allowed=%v used=%d", allowed, used)
	}
	// A different contract keeps its own window.
	if allowed, used, _ := limiter.allow("ct_b", 2, base.Add(10*time.Second)); !allowed || used != 1 {
		t.Fatalf("other contract: allowed=%v used=%d", allowed, used)
	}
	// The next minute starts a fresh window.
	if allowed, used, _ := limiter.allow("ct_a", 2, base.Add(time.Minute)); !allowed || used != 1 {
		t.Fatalf("next window: allowed=%v used=%d", allowed, used)
	}
	// A non-positive ceiling means unlimited.
	for i := 0; i < 5; i++ {
		if allowed, _, _ := limiter.allow("ct_c", 0, base); !allowed {
			t.Fatal("zero limit should not throttle")
		}
	}
}

func TestRetryAfterSeconds(t *testing.T) {
	base := time.Date(2026, 9, 4, 10, 30, 0, 0, time.UTC)
	cases := []struct {
		name  string
		now   time.Time
		reset time.Time
		want  int
	}{
		{"whole seconds", base, base.Add(30 * time.Second), 30},
		{"rounds up a partial second", base, base.Add(30*time.Second + 1), 31},
		{"never returns zero", base, base, 1},
		{"never returns negative", base, base.Add(-5 * time.Second), 1},
	}
	for _, tc := range cases {
		if got := retryAfterSeconds(tc.reset, tc.now); got != tc.want {
			t.Errorf("%s: retryAfterSeconds = %d, want %d", tc.name, got, tc.want)
		}
	}
}

// TestDataProductQueryEnforcesContractRateLimit covers the runtime path: the per-minute
// ceiling stored on the contract scope must actually block calls, report Retry-After, and
// land in the over-limit usage counter instead of the failed-call counter.
func TestDataProductQueryEnforcesContractRateLimit(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	ctx := context.Background()
	if err := db.UpsertDataProduct(ctx, store.DataProduct{
		ID: "dprod_rl", ProductKey: "dw_rate_limited", NameKO: "Rate Limited API",
		SourceType: "api", SourceRef: "loan_history", Owner: "data",
		Sensitivity: "internal", Status: "published",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIKey(ctx, store.APIKeyRecord{
		ID: "apikey_rl", KeyHash: hashProxyKey("rl-client-token"), Name: "rl-client",
		Team: "team_rl", Status: "active",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertContractScope(ctx, store.ContractScope{
		ContractKey: "ct_rl", ProductKey: "dw_rate_limited", CustomerKey: "cust_rl",
		AllowedFields: []string{"product_key", "score"}, RateLimit: 2, Status: "active",
		Purpose: "risk monitoring",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertAPIEntitlement(ctx, store.APIEntitlement{
		ID: "ent_rl", APIKeyID: "apikey_rl", APIKeyHash: hashProxyKey("rl-client-token"),
		ProductKey: "dw_rate_limited", CustomerKey: "cust_rl", ContractKey: "ct_rl",
		Status: "active", Scope: "data_product:query",
	}); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"fields": []string{"score"}}
	for i := 1; i <= 2; i++ {
		resp := postJSON(t, srv.URL+"/v1/data-products/dw_rate_limited/query", "rl-client-token", body)
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("call %d: expected 200 inside the rate limit, got %d: %s", i, resp.StatusCode, raw)
		}
		if got := resp.Header.Get("X-DataWorks-RateLimit-Limit"); got != "2" {
			t.Fatalf("call %d: rate limit header = %q", i, got)
		}
	}

	blocked := postJSON(t, srv.URL+"/v1/data-products/dw_rate_limited/query", "rl-client-token", body)
	raw, _ := io.ReadAll(blocked.Body)
	blocked.Body.Close()
	if blocked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("expected 429 past the contract rate limit, got %d: %s", blocked.StatusCode, raw)
	}
	var errBody struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(raw, &errBody)
	if errBody.Error.Code != "contract_rate_limited" {
		t.Fatalf("unexpected error code in %s", raw)
	}
	retryAfter := blocked.Header.Get("Retry-After")
	if retryAfter == "" || retryAfter == "0" {
		t.Fatalf("Retry-After = %q, want the seconds left in the window", retryAfter)
	}
	if got := blocked.Header.Get("X-DataWorks-RateLimit-Used"); got != "2" {
		t.Fatalf("rejected call should not consume the window, used header = %q", got)
	}

	usage, err := db.ListUsageMetering(ctx, "dw_rate_limited")
	if err != nil || len(usage) != 1 {
		t.Fatalf("usage metering = %+v (err %v)", usage, err)
	}
	if usage[0].TotalCalls != 3 || usage[0].OverLimitCalls != 1 || usage[0].FailedCalls != 0 {
		t.Fatalf("expected 3 calls with 1 over-limit and 0 failed, got %+v", usage[0])
	}
}

func TestContractScopeRejectsNegativeRateLimit(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "dw.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	if err := db.UpsertDataProduct(context.Background(), store.DataProduct{
		ID: "dprod_rl2", ProductKey: "dw_rate_limit_input", NameKO: "Rate Limit Input",
		SourceType: "api", SourceRef: "loan_history", Owner: "data", Status: "draft",
	}); err != nil {
		t.Fatal(err)
	}

	resp := postJSON(t, srv.URL+"/admin/dataworks/products/dw_rate_limit_input/contract-scopes", "", map[string]any{
		"contract_key": "ct_negative", "customer_key": "cust_rl", "allowed_fields": []string{"score"},
		"rate_limit": -1,
	})
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for a negative rate_limit, got %d: %s", resp.StatusCode, raw)
	}
	scopes, err := db.ListContractScopes(context.Background(), "dw_rate_limit_input", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(scopes) != 0 {
		t.Fatalf("rejected scope should not be stored, got %+v", scopes)
	}
}
