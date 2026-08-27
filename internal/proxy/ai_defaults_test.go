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

	"dataworks/internal/config"
	"dataworks/internal/store"
)

func TestApplyDefaultChatStreamOnlyWhenOmitted(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        string
		def         bool
		want        bool
		wantChanged bool
	}{
		{name: "missing defaults true", body: `{"model":"m"}`, def: true, want: true, wantChanged: true},
		{name: "missing defaults false", body: `{"model":"m"}`, def: false, want: false, wantChanged: true},
		{name: "explicit false respected", body: `{"model":"m","stream":false}`, def: true, want: false, wantChanged: false},
		{name: "explicit true respected", body: `{"model":"m","stream":true}`, def: false, want: true, wantChanged: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed := applyDefaultChatStream([]byte(tc.body), tc.def)
			if changed != tc.wantChanged {
				t.Fatalf("changed=%v want %v", changed, tc.wantChanged)
			}
			var root map[string]any
			if err := json.Unmarshal(got, &root); err != nil {
				t.Fatal(err)
			}
			if stream, _ := root["stream"].(bool); stream != tc.want {
				t.Fatalf("stream=%v want %v body=%s", stream, tc.want, got)
			}
		})
	}
}

func TestChatDefaultStreamRuntimeSettingAndAudit(t *testing.T) {
	streamSeen := make(chan bool, 3)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		stream, _ := body["stream"].(bool)
		streamSeen <- stream
		if stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`)
	}))
	defer upstream.Close()

	db := openTestStore(t)
	defer db.Close()
	logger := store.NewAsyncLogger(db, 32, filepath.Join(t.TempDir(), "fallback.ndjson"))
	logger.Start()
	defer logger.Stop(context.Background())
	cfg := testConfig(upstream.URL, "upstream-secret")
	cfg.AI.DefaultStream = true
	server, err := NewServer(cfg, db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httptest.NewServer(server.Routes())
	defer proxy.Close()

	call := func(body map[string]any) {
		resp := postJSON(t, proxy.URL+"/v1/chat/completions", "caller-secret", body)
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("chat status=%d", resp.StatusCode)
		}
	}
	base := map[string]any{"model": "test-model", "messages": []map[string]string{{"role": "user", "content": "hello"}}}
	call(base)
	if got := <-streamSeen; !got {
		t.Fatal("omitted stream should use runtime default true")
	}
	waitFor(t, time.Second, func() bool {
		recent, _ := db.RecentRequests(context.Background(), store.RequestFilter{Limit: 1})
		return len(recent) == 1 && recent[0].Stream
	})

	explicitFalse := map[string]any{"model": "test-model", "stream": false, "messages": []map[string]string{{"role": "user", "content": "hello"}}}
	call(explicitFalse)
	if got := <-streamSeen; got {
		t.Fatal("explicit stream=false must be preserved")
	}
	waitFor(t, time.Second, func() bool {
		recent, _ := db.RecentRequests(context.Background(), store.RequestFilter{Limit: 2})
		return len(recent) == 2 && !recent[0].Stream
	})

	resp, out := req(t, http.MethodPut, proxy.URL+"/admin/settings/by-key/ai.default_stream", `{"value":"false"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setting update status=%d body=%v", resp.StatusCode, out)
	}
	call(base)
	if got := <-streamSeen; got {
		t.Fatal("omitted stream should use updated runtime default false")
	}
}

func TestTokenBudgetSettingsAllowUpTo256Ki(t *testing.T) {
	for _, key := range []string{"mcp.max_tokens", "limits.max_output_tokens", "limits.agent_max_tokens"} {
		def, ok := settingDefByKey(key)
		if !ok {
			t.Fatalf("setting %s missing", key)
		}
		if err := validateSettingValue(def, "262144"); err != nil {
			t.Errorf("%s should accept 262144: %v", key, err)
		}
		if err := validateSettingValue(def, "262145"); err == nil {
			t.Errorf("%s should reject values above 262144", key)
		}
	}
	if config.MaxSupportedOutputTokens != 262144 {
		t.Fatalf("MaxSupportedOutputTokens=%d", config.MaxSupportedOutputTokens)
	}
}

func TestAdminChatConsoleCapsAt256Ki(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/admin/chat-test/run", nil)
	prep, ok := s.prepareChatTestRequest(w, r, chatTestRunRequest{
		Model:     "test-model",
		Prompt:    "hello",
		MaxTokens: config.MaxSupportedOutputTokens + 1,
	}, false)
	if !ok {
		t.Fatalf("prepare failed: status=%d body=%s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(prep.body, &body); err != nil {
		t.Fatal(err)
	}
	if got := int(body["max_tokens"].(float64)); got != config.MaxSupportedOutputTokens {
		t.Fatalf("max_tokens=%d want %d", got, config.MaxSupportedOutputTokens)
	}
}
