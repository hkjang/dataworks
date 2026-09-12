package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"dataworks/internal/store"
	"dataworks/internal/tracking"
)

// trackingServer boots a full server whose SPA shell is a tiny fixture, so the
// tests can read the injected markup instead of the real Vite build.
func trackingServer(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	t.Cleanup(upstream.Close)
	db := openTestStore(t)
	t.Cleanup(func() { db.Close() })
	logger := store.NewAsyncLogger(db, 32, t.TempDir()+"/fallback.ndjson")
	logger.Start()
	t.Cleanup(func() { logger.Stop(context.Background()) })
	server, err := NewServer(testConfig(upstream.URL, "secret"), db, logger, nil)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.Handle(dataWorksSPAPrefix, newSPAHandler(fstest.MapFS{
		"index.html":           &fstest.MapFile{Data: []byte("<!doctype html><html><head><title>Data Works</title></head><body><div id=\"root\"></div><script type=\"module\" src=\"/dataworks/assets/app.js\"></script></body></html>")},
		"assets/app-a1b2c3.js": &fstest.MapFile{Data: []byte("console.log('dataworks')")},
	}, dataWorksSPAPrefix).withPage(server.decorateSPAPage))
	full := server.Routes()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, dataWorksSPAPrefix) {
			mux.ServeHTTP(w, r)
			return
		}
		full.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return server, ts
}

func putTrackingSetting(t *testing.T, base, key, value string) {
	t.Helper()
	resp, body := req(t, http.MethodPut, base+"/admin/settings/by-key/"+key, `{"value":`+trackingJSONString(value)+`}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT %s = %d %v", key, resp.StatusCode, body)
	}
}

func trackingJSONString(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(value) + `"`
}

func fetchPage(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp, string(body)
}

var nonceInPolicy = regexp.MustCompile(`'nonce-([^']+)'`)

func TestTrackingOffLeavesPagesUntouchedWithStrictPolicy(t *testing.T) {
	_, ts := trackingServer(t)
	for _, path := range []string{"/dataworks/", "/dataworks/products/credit", "/dataworks/settings"} {
		resp, body := fetchPage(t, ts.URL+path)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s status = %d", path, resp.StatusCode)
		}
		policy := resp.Header.Get("Content-Security-Policy")
		if !strings.Contains(policy, "script-src 'self';") || strings.Contains(policy, "nonce-") || strings.Contains(policy, "report-uri") {
			t.Fatalf("%s policy = %q, want strict script-src 'self' without nonce or report-uri", path, policy)
		}
		if strings.Contains(policy, "'unsafe-inline'") && !strings.Contains(policy, "style-src 'self' 'unsafe-inline'") {
			t.Fatalf("policy must never relax scripts: %q", policy)
		}
		if strings.Count(body, "<script") != 1 || strings.Contains(body, "nonce=") {
			t.Fatalf("%s body changed while tracking is off: %s", path, body)
		}
	}
	// The legacy console keeps its original markup.
	resp, body := fetchPage(t, ts.URL+"/admin")
	if resp.StatusCode != http.StatusOK || strings.Contains(body, "tracker.js") || resp.Header.Get("Content-Security-Policy") != "" {
		t.Fatalf("legacy admin changed while tracking is off: status=%d csp=%q", resp.StatusCode, resp.Header.Get("Content-Security-Policy"))
	}
	// Non-page paths get the narrow policy.
	for _, path := range []string{"/healthz", "/admin/dataworks/home", "/v1/models", "/tracking/csp-report"} {
		r, _ := http.NewRequest(http.MethodGet, ts.URL+path, nil)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if got := response.Header.Get("Content-Security-Policy"); got != apiPolicy {
			t.Fatalf("%s policy = %q, want %q", path, got, apiPolicy)
		}
	}
	// The proxy stays closed and reports are dropped.
	proxyResp, _ := fetchPage(t, ts.URL+"/momento/tracker.js")
	if proxyResp.StatusCode != http.StatusNotFound {
		t.Fatalf("momento proxy while off = %d, want 404", proxyResp.StatusCode)
	}
}

func TestTrackingMomentoProxyInjectsNonceAndForwards(t *testing.T) {
	server, ts := trackingServer(t)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Errorf("credentials forwarded to collector: cookie=%q auth=%q", r.Header.Get("Cookie"), r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte("// tracker " + r.Method + " " + r.URL.Path + " host=" + r.Host))
	}))
	t.Cleanup(collector.Close)

	putTrackingSetting(t, ts.URL, "tracking.provider", "momento")
	putTrackingSetting(t, ts.URL, "tracking.momento_url", collector.URL+"/base/")
	putTrackingSetting(t, ts.URL, "tracking.momento_site_id", "dw-42")
	putTrackingSetting(t, ts.URL, "tracking.enabled", "true")

	resp, body := fetchPage(t, ts.URL+"/dataworks/products/credit")
	policy := resp.Header.Get("Content-Security-Policy")
	match := nonceInPolicy.FindStringSubmatch(policy)
	if match == nil {
		t.Fatalf("policy lacks nonce: %q", policy)
	}
	nonce := match[1]
	if !strings.Contains(body, `<script nonce="`+nonce+`" async src="/momento/tracker.js" data-site-id="dw-42" data-endpoint="/momento"`) {
		t.Fatalf("body lacks nonced momento snippet for nonce %s: %s", nonce, body)
	}
	if !strings.Contains(body, "</script>\n</head>") {
		t.Fatalf("snippet should sit in head by default: %s", body)
	}
	if strings.Contains(policy, collector.URL) || strings.Contains(policy, "127.0.0.1") {
		t.Fatalf("proxy mode must not name the collector in the policy: %q", policy)
	}
	if !strings.Contains(policy, "report-uri "+trackingReportPath) {
		t.Fatalf("policy lacks report-uri while tracking is on: %q", policy)
	}
	// Every request gets its own nonce.
	second, _ := fetchPage(t, ts.URL+"/dataworks/")
	if other := nonceInPolicy.FindStringSubmatch(second.Header.Get("Content-Security-Policy")); other == nil || other[1] == nonce {
		t.Fatalf("nonce reused across requests: %v", other)
	}
	// Admin pages are skipped by default and included on request.
	adminResp, adminBody := fetchPage(t, ts.URL+"/dataworks/settings")
	if strings.Contains(adminBody, "tracker.js") || strings.Contains(adminResp.Header.Get("Content-Security-Policy"), "nonce-") {
		t.Fatalf("admin page tracked without include_admin: %s", adminBody)
	}
	legacyResp, legacyBody := fetchPage(t, ts.URL+"/admin")
	if legacyResp.StatusCode != http.StatusOK || strings.Contains(legacyBody, "tracker.js") {
		t.Fatal("legacy console tracked without include_admin")
	}
	putTrackingSetting(t, ts.URL, "tracking.include_admin", "true")
	putTrackingSetting(t, ts.URL, "tracking.placement", "body")
	_, adminBody = fetchPage(t, ts.URL+"/dataworks/settings")
	if !strings.Contains(adminBody, "tracker.js") || !strings.Contains(adminBody, "</script>\n</body>") {
		t.Fatalf("admin page not tracked in body with include_admin: %s", adminBody)
	}
	_, legacyBody = fetchPage(t, ts.URL+"/admin")
	if !strings.Contains(legacyBody, `src="/momento/tracker.js"`) {
		t.Fatal("legacy console should carry the snippet with include_admin")
	}

	// The same-origin proxy forwards to the collector without credentials.
	r, _ := http.NewRequest(http.MethodGet, ts.URL+"/momento/tracker.js?v=1", nil)
	r.Header.Set("Cookie", "dw_session=secret")
	r.Header.Set("Authorization", "Bearer secret")
	proxied, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatal(err)
	}
	proxiedBody, _ := io.ReadAll(proxied.Body)
	proxied.Body.Close()
	if proxied.StatusCode != http.StatusOK || !strings.Contains(string(proxiedBody), "GET /base/tracker.js") {
		t.Fatalf("proxied = %d %s", proxied.StatusCode, proxiedBody)
	}
	if proxied.Header.Get("Content-Security-Policy") != "" {
		t.Fatalf("proxy response should not carry the API policy: %q", proxied.Header.Get("Content-Security-Policy"))
	}
	beacon, _ := req(t, http.MethodPost, ts.URL+"/momento/api/event", `{"e":"pageview"}`)
	if beacon.StatusCode != http.StatusOK {
		t.Fatalf("beacon = %d", beacon.StatusCode)
	}
	// Status reflects the effective configuration.
	statusResp, status := req(t, http.MethodGet, ts.URL+"/admin/tracking/status", "")
	if statusResp.StatusCode != http.StatusOK || status["active"] != true || status["momento_proxy"] != true || status["problem"] != "" {
		t.Fatalf("status = %d %v", statusResp.StatusCode, status)
	}

	// Turning tracking off restores the strict policy and closes the proxy.
	putTrackingSetting(t, ts.URL, "tracking.enabled", "false")
	offResp, offBody := fetchPage(t, ts.URL+"/dataworks/")
	if strings.Contains(offBody, "tracker.js") || strings.Contains(offResp.Header.Get("Content-Security-Policy"), "nonce-") {
		t.Fatalf("tracking still on after disable: %q", offResp.Header.Get("Content-Security-Policy"))
	}
	if closed, _ := fetchPage(t, ts.URL+"/momento/tracker.js"); closed.StatusCode != http.StatusNotFound {
		t.Fatalf("proxy still open after disable: %d", closed.StatusCode)
	}
	if server.trackingConf().Enabled {
		t.Fatal("runtime snapshot not refreshed")
	}
}

func TestTrackingCustomSnippetPolicySourcesAndViolations(t *testing.T) {
	_, ts := trackingServer(t)
	snippet := `<script async src="https://cdn.example/t.js"></script><script>window.t=function(){navigator.sendBeacon('https://collect.example/hit')}</script>`
	putTrackingSetting(t, ts.URL, "tracking.provider", "custom")
	putTrackingSetting(t, ts.URL, "tracking.custom_snippet", snippet)
	putTrackingSetting(t, ts.URL, "tracking.enabled", "true")

	resp, body := fetchPage(t, ts.URL+"/dataworks/")
	policy := resp.Header.Get("Content-Security-Policy")
	nonce := nonceInPolicy.FindStringSubmatch(policy)[1]
	if strings.Count(body, `nonce="`+nonce+`"`) != 2 {
		t.Fatalf("both script tags need the nonce: %s", body)
	}
	for _, directive := range []string{"script-src", "connect-src", "img-src"} {
		segment := policy[strings.Index(policy, directive):]
		segment = segment[:strings.Index(segment, ";")]
		if !strings.Contains(segment, "https://cdn.example") || !strings.Contains(segment, "https://collect.example") {
			t.Fatalf("%s lacks snippet origins: %q", directive, segment)
		}
	}
	if strings.Contains(policy, "script-src 'self' 'unsafe-inline'") {
		t.Fatalf("unsafe-inline for scripts: %q", policy)
	}

	// A browser reports a blocked origin; it is listed once with its directive.
	report := `{"csp-report":{"document-uri":"` + ts.URL + `/dataworks/","blocked-uri":"https://pixel.example/p.gif?x=1","effective-directive":"img-src","violated-directive":"img-src 'self'"}}`
	for range 3 {
		if r, _ := req(t, http.MethodPost, ts.URL+"/tracking/csp-report", report); r.StatusCode != http.StatusNoContent {
			t.Fatalf("report = %d", r.StatusCode)
		}
	}
	if r, _ := req(t, http.MethodPost, ts.URL+"/tracking/csp-report", "not json"); r.StatusCode != http.StatusNoContent {
		t.Fatalf("broken report = %d, want 204", r.StatusCode)
	}
	listResp, list := req(t, http.MethodGet, ts.URL+"/admin/tracking/violations", "")
	items, _ := list["items"].([]any)
	if listResp.StatusCode != http.StatusOK || len(items) != 1 {
		t.Fatalf("violations = %d %v", listResp.StatusCode, list)
	}
	item := items[0].(map[string]any)
	if item["origin"] != "https://pixel.example" || item["directive"] != "img-src" || item["count"] != float64(3) || item["allowed"] != false {
		t.Fatalf("item = %v", item)
	}

	// One click adds it to the allow list and the policy picks it up.
	allowResp, allowed := req(t, http.MethodPost, ts.URL+"/admin/tracking/violations/allow", `{"origin":"https://pixel.example/"}`)
	if allowResp.StatusCode != http.StatusOK || allowed["value"] != "https://pixel.example" {
		t.Fatalf("allow = %d %v", allowResp.StatusCode, allowed)
	}
	if first := allowed["items"].([]any)[0].(map[string]any); first["allowed"] != true {
		t.Fatalf("allowed flag not updated: %v", first)
	}
	after, _ := fetchPage(t, ts.URL+"/dataworks/")
	if !strings.Contains(after.Header.Get("Content-Security-Policy"), "https://pixel.example") {
		t.Fatalf("policy lacks allowed host: %q", after.Header.Get("Content-Security-Policy"))
	}
	if bad, _ := req(t, http.MethodPost, ts.URL+"/admin/tracking/violations/allow", `{"origin":"javascript:alert(1)"}`); bad.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad origin = %d, want 400", bad.StatusCode)
	}
	if cleared, _ := req(t, http.MethodDelete, ts.URL+"/admin/tracking/violations", ""); cleared.StatusCode != http.StatusNoContent {
		t.Fatalf("clear = %d", cleared.StatusCode)
	}
	if _, list := req(t, http.MethodGet, ts.URL+"/admin/tracking/violations", ""); len(list["items"].([]any)) != 0 {
		t.Fatalf("violations not cleared: %v", list)
	}
}

func TestTrackingSettingsValidation(t *testing.T) {
	_, ts := trackingServer(t)
	cases := []struct {
		key, value string
		want       int
	}{
		{"tracking.provider", "pixel", http.StatusBadRequest},
		{"tracking.provider", "ga4", http.StatusOK},
		{"tracking.custom_snippet", "<script>" + strings.Repeat("x", tracking.MaxSnippetBytes) + "</script>", http.StatusBadRequest},
		{"tracking.custom_snippet", "<script>ok()</script>", http.StatusOK},
		{"tracking.momento_url", "momento.internal", http.StatusBadRequest},
		{"tracking.momento_url", "https://momento.internal", http.StatusOK},
		{"tracking.allowed_hosts", "cdn.example", http.StatusBadRequest},
		{"tracking.allowed_hosts", "https://cdn.example, https://*.wild.example", http.StatusOK},
		{"tracking.placement", "sidebar", http.StatusBadRequest},
		{"tracking.placement", "body", http.StatusOK},
	}
	for _, tc := range cases {
		resp, body := req(t, http.MethodPut, ts.URL+"/admin/settings/by-key/"+tc.key, `{"value":`+trackingJSONString(tc.value)+`}`)
		if resp.StatusCode != tc.want {
			t.Fatalf("PUT %s=%q -> %d, want %d (%v)", tc.key, tc.value[:min(len(tc.value), 40)], resp.StatusCode, tc.want, body)
		}
	}
	// Reports are dropped while tracking is off, so the endpoint cannot be
	// used to fill the recorder from outside.
	if r, _ := req(t, http.MethodPost, ts.URL+"/tracking/csp-report", `{"csp-report":{"blocked-uri":"https://x.example","effective-directive":"img-src"}}`); r.StatusCode != http.StatusNoContent {
		t.Fatalf("report = %d", r.StatusCode)
	}
	if _, list := req(t, http.MethodGet, ts.URL+"/admin/tracking/violations", ""); len(list["items"].([]any)) != 0 {
		t.Fatalf("report recorded while tracking is off: %v", list)
	}
}

func TestPagePolicyDirectModeNamesCollector(t *testing.T) {
	config := tracking.Config{Enabled: true, Provider: tracking.ProviderMomento, MomentoURL: "https://momento.internal", MomentoSiteID: "1", MomentoProxy: false}
	policy := pagePolicy(config, false, "n1")
	if !strings.Contains(policy, "script-src 'self' 'nonce-n1' https://momento.internal;") || !strings.Contains(policy, "connect-src 'self' https://momento.internal;") {
		t.Fatalf("policy = %q", policy)
	}
	if strings.Contains(pagePolicy(config, true, "n1"), "nonce-") {
		t.Fatal("admin page should not get the nonce without include_admin")
	}
	if strings.Contains(pagePolicy(tracking.Config{}, false, "n1"), "report-uri") {
		t.Fatal("report-uri only while tracking is on")
	}
}

func TestTrackingAdminPage(t *testing.T) {
	for path, want := range map[string]bool{"/admin": true, "/admin/": true, "/dataworks/settings": true, "/dataworks/settings/": true, "/dataworks/": false, "/dataworks/products/x": false, "/dataworks/personal": false} {
		if got := trackingAdminPage(path); got != want {
			t.Fatalf("trackingAdminPage(%q) = %v, want %v", path, got, want)
		}
	}
}
