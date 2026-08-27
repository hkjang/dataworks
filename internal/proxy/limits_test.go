package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"dataworks/internal/config"
	"dataworks/internal/store"
)

func TestClampMaxOutputTokens(t *testing.T) {
	// Over the cap → reduced.
	out, from, to, changed := clampMaxOutputTokens([]byte(`{"model":"m","max_tokens":9000}`), 4096)
	if !changed || from != 9000 || to != 4096 {
		t.Fatalf("over-cap clamp = %v from=%d to=%d", changed, from, to)
	}
	var root map[string]any
	_ = json.Unmarshal(out, &root)
	if int(root["max_tokens"].(float64)) != 4096 {
		t.Fatalf("max_tokens not clamped: %v", root["max_tokens"])
	}

	// Both Chat Completions token fields are independently clamped. A client cannot
	// bypass the ceiling by sending the legacy and newer field together.
	out, from, to, changed = clampMaxOutputTokens([]byte(`{"model":"m","max_tokens":9000,"max_completion_tokens":8000}`), 4096)
	if !changed || from != 9000 || to != 4096 {
		t.Fatalf("dual-field clamp = %v from=%d to=%d", changed, from, to)
	}
	_ = json.Unmarshal(out, &root)
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		if got := int(root[field].(float64)); got != 4096 {
			t.Fatalf("%s=%d want 4096", field, got)
		}
	}

	// Absent → injected (from = -1).
	_, from, to, changed = clampMaxOutputTokens([]byte(`{"model":"m"}`), 4096)
	if !changed || from != -1 || to != 4096 {
		t.Fatalf("inject = %v from=%d to=%d", changed, from, to)
	}

	// Under the cap → unchanged.
	if _, _, _, changed = clampMaxOutputTokens([]byte(`{"model":"m","max_tokens":100}`), 4096); changed {
		t.Fatal("under-cap should not change")
	}

	// max_completion_tokens variant.
	if _, from, _, changed = clampMaxOutputTokens([]byte(`{"model":"m","max_completion_tokens":8000}`), 2048); !changed || from != 8000 {
		t.Fatalf("max_completion_tokens clamp = %v from=%d", changed, from)
	}

	// Disabled (cap 0) → no-op.
	if _, _, _, changed = clampMaxOutputTokens([]byte(`{"model":"m","max_tokens":9000}`), 0); changed {
		t.Fatal("cap 0 should be a no-op")
	}

	// Absolute-cap mode clamps only explicit values and does not inject a disabled runtime cap.
	if _, from, to, changed = clampExplicitMaxOutputTokens([]byte(`{"model":"m","max_tokens":300000}`), config.MaxSupportedOutputTokens); !changed || from != 300000 || to != config.MaxSupportedOutputTokens {
		t.Fatalf("absolute explicit clamp = %v from=%d to=%d", changed, from, to)
	}
	if _, _, _, changed = clampExplicitMaxOutputTokens([]byte(`{"model":"m"}`), config.MaxSupportedOutputTokens); changed {
		t.Fatal("absolute cap must not inject when the runtime limit is disabled")
	}

	// Responses API uses its native field and follows the same inject/absolute modes.
	out, from, to, changed = clampResponsesMaxOutputTokens([]byte(`{"model":"m","max_output_tokens":9000}`), 2048)
	if !changed || from != 9000 || to != 2048 {
		t.Fatalf("responses clamp = %v from=%d to=%d", changed, from, to)
	}
	_ = json.Unmarshal(out, &root)
	if got := int(root["max_output_tokens"].(float64)); got != 2048 {
		t.Fatalf("max_output_tokens=%d want 2048", got)
	}
	if _, _, _, changed = clampExplicitResponsesMaxOutputTokens([]byte(`{"model":"m"}`), config.MaxSupportedOutputTokens); changed {
		t.Fatal("Responses absolute cap must not inject when the runtime limit is disabled")
	}
}

func TestLimitsTokenMutationIsEndpointSpecific(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "endpoint-limits.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}

	apply := func(path, raw string, configuredMax int) (map[string]any, string) {
		t.Helper()
		server.limitsRuntime.Store(&config.LimitsConfig{MaxOutputTokens: configuredMax})
		recorder := httptest.NewRecorder()
		rc := &requestPipeline{
			s:    server,
			w:    recorder,
			r:    httptest.NewRequest(http.MethodPost, path, nil),
			body: []byte(raw),
		}
		if !rc.stepLimits() {
			t.Fatalf("stepLimits halted for %s", path)
		}
		var body map[string]any
		if err := json.Unmarshal(rc.body, &body); err != nil {
			t.Fatalf("decode %s body: %v", path, err)
		}
		return body, recorder.Header().Get("X-Max-Tokens-Clamped")
	}

	chat, header := apply("/v1/chat/completions", `{"model":"m","max_tokens":9000,"max_completion_tokens":8000}`, 512)
	for _, field := range []string{"max_tokens", "max_completion_tokens"} {
		if got := int(chat[field].(float64)); got != 512 {
			t.Fatalf("Chat %s=%d want configured cap 512", field, got)
		}
	}
	if header != "9000->512" {
		t.Fatalf("Chat clamp header=%q", header)
	}

	responses, _ := apply("/v1/responses", `{"model":"m","max_output_tokens":300000}`, 0)
	if got := int(responses["max_output_tokens"].(float64)); got != config.MaxSupportedOutputTokens {
		t.Fatalf("Responses absolute max=%d want %d", got, config.MaxSupportedOutputTokens)
	}

	// Even a corrupted/out-of-band runtime value above the supported maximum cannot
	// raise the absolute service ceiling.
	responses, _ = apply("/v1/responses", `{"model":"m","max_output_tokens":300000}`, config.MaxSupportedOutputTokens+1000)
	if got := int(responses["max_output_tokens"].(float64)); got != config.MaxSupportedOutputTokens {
		t.Fatalf("Responses max with oversized runtime cap=%d want %d", got, config.MaxSupportedOutputTokens)
	}

	responses, header = apply("/v1/responses", `{"model":"m"}`, 1024)
	if got := int(responses["max_output_tokens"].(float64)); got != 1024 {
		t.Fatalf("Responses injected max=%d want 1024", got)
	}
	if header != "injected:1024" {
		t.Fatalf("Responses injection header=%q", header)
	}

	embeddings, header := apply("/v1/embeddings", `{"model":"embedding-model","input":"hello"}`, 512)
	for _, field := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens"} {
		if _, exists := embeddings[field]; exists {
			t.Fatalf("embeddings request unexpectedly received %s", field)
		}
	}
	if header != "" {
		t.Fatalf("embeddings clamp header=%q want empty", header)
	}
}

func TestLimitsMaxRequestBytes(t *testing.T) {
	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "lb.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.limitsRuntime.Store(&config.LimitsConfig{MaxRequestBytes: 200})
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	big := make([]map[string]string, 0)
	for i := 0; i < 50; i++ {
		big = append(big, map[string]string{"role": "user", "content": "this is a fairly long message used to exceed the byte limit"})
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", jsonReader(map[string]any{"model": "gpt-4o", "messages": big}))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized request = %d, want 413", resp.StatusCode)
	}
	if resp.Header.Get("X-Request-Bytes") == "" {
		t.Fatal("expected X-Request-Bytes header")
	}
}

func TestLimitsMaxMessages(t *testing.T) {
	if countMessages([]byte(`{"messages":[{"role":"user","content":"a"},{"role":"assistant","content":"b"}]}`)) != 2 {
		t.Fatal("countMessages should be 2")
	}
	if countMessages([]byte(`{"model":"m"}`)) != 0 {
		t.Fatal("absent messages → 0")
	}

	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "lm.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig("http://upstream.invalid", "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.limitsRuntime.Store(&config.LimitsConfig{MaxMessages: 3})
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	msgs := []map[string]string{}
	for i := 0; i < 5; i++ {
		msgs = append(msgs, map[string]string{"role": "user", "content": "x"})
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", jsonReader(map[string]any{"model": "gpt-4o", "messages": msgs}))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("too many messages = %d, want 400", resp.StatusCode)
	}
	if resp.Header.Get("X-Message-Count") != "5" {
		t.Fatalf("X-Message-Count = %q", resp.Header.Get("X-Message-Count"))
	}
}

func TestLimitsStepForwardsClamped(t *testing.T) {
	var seenMax float64 = -42
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var root map[string]any
		_ = json.Unmarshal(b, &root)
		if v, ok := root["max_tokens"].(float64); ok {
			seenMax = v
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`))
	}))
	defer upstream.Close()

	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "lim.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig(upstream.URL, "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.limitsRuntime.Store(&config.LimitsConfig{MaxOutputTokens: 512})
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", jsonReader(map[string]any{
		"model": "gpt-4o", "max_tokens": 100000, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("X-Max-Tokens-Clamped") != "100000->512" {
		t.Fatalf("clamp header = %q", resp.Header.Get("X-Max-Tokens-Clamped"))
	}
	if seenMax != 512 {
		t.Fatalf("upstream should receive clamped max_tokens=512, got %v", seenMax)
	}
}

func TestLimitsAbsoluteMaximumAppliesWhenRuntimeLimitDisabled(t *testing.T) {
	seen := map[string]any{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seen)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 8, filepath.Join(t.TempDir(), "absolute.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	server, err := NewServer(testConfig(upstream.URL, "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	server.limitsRuntime.Store(&config.LimitsConfig{MaxOutputTokens: 0})
	srv := httptest.NewServer(server.Routes())
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/chat/completions", jsonReader(map[string]any{
		"model": "gpt-4o", "max_tokens": 300000, "messages": []map[string]string{{"role": "user", "content": "hi"}},
	}))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if got := int(seen["max_tokens"].(float64)); got != config.MaxSupportedOutputTokens {
		t.Fatalf("absolute max forwarded=%d want=%d", got, config.MaxSupportedOutputTokens)
	}
}
