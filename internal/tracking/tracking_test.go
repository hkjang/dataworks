package tracking

import (
	"strings"
	"testing"
	"time"
)

func TestReadConfigDefaultsToOff(t *testing.T) {
	config := ReadConfig(nil)
	if config.Enabled || config.Provider != ProviderNone || config.Placement != PlacementHead || config.IncludeAdmin {
		t.Fatalf("defaults = %+v, want disabled/none/head/no-admin", config)
	}
	if !config.MomentoProxy {
		t.Fatal("momento proxy should default to on")
	}
	if config.Active(false) || config.Active(true) {
		t.Fatal("a default configuration must not be active on any page")
	}
	if config.MomentoTarget() != nil {
		t.Fatal("proxy must stay closed while tracking is off")
	}
}

func TestReadConfigNormalizes(t *testing.T) {
	config := ReadConfig(map[string]string{
		"tracking.enabled":       " true ",
		"tracking.provider":      "Momento",
		"tracking.placement":     "BODY",
		"tracking.momento_proxy": "not-a-bool",
	})
	if !config.Enabled || config.Provider != ProviderMomento || config.Placement != PlacementBody {
		t.Fatalf("config = %+v", config)
	}
	if !config.MomentoProxy {
		t.Fatal("unparseable momento_proxy should fall back to the default (on)")
	}
	if got := ReadConfig(map[string]string{"tracking.placement": "sidebar"}).Placement; got != PlacementHead {
		t.Fatalf("unknown placement = %q, want head", got)
	}
}

func TestMomentoProxySnippetNamesNoExternalOrigin(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://momento.internal/", MomentoSiteID: "dw-1", MomentoProxy: true, MomentoEnvironment: "stg"}
	snippet := config.Snippet("abc")
	for _, want := range []string{`src="/momento/tracker.js"`, `data-endpoint="/momento"`, `data-site-id="dw-1"`, `data-environment="stg"`, `data-contract-version="1"`, `nonce="abc"`} {
		if !strings.Contains(snippet, want) {
			t.Fatalf("snippet %q lacks %q", snippet, want)
		}
	}
	if strings.Contains(snippet, "momento.internal") {
		t.Fatalf("proxy snippet must not name the collector: %q", snippet)
	}
	scripts, connects, images := config.PolicySources()
	if len(scripts)+len(connects)+len(images) != 0 {
		t.Fatalf("proxy mode needs no policy sources, got %v %v %v", scripts, connects, images)
	}
	target := config.MomentoTarget()
	if target == nil || target.String() != "https://momento.internal" {
		t.Fatalf("proxy target = %v", target)
	}
	if !config.Active(false) || config.Active(true) {
		t.Fatal("should be active on visitor pages and not on admin pages by default")
	}
}

func TestMomentoDirectSnippetAddsCollectorOrigin(t *testing.T) {
	config := Config{Enabled: true, Provider: ProviderMomento, MomentoURL: "https://Momento.Internal:8443/base/", MomentoSiteID: "dw-1", MomentoProxy: false}
	snippet := config.Snippet("n1")
	if !strings.Contains(snippet, `src="https://Momento.Internal:8443/base/tracker.js"`) || strings.Contains(snippet, "data-endpoint") {
		t.Fatalf("snippet = %q", snippet)
	}
	scripts, connects, images := config.PolicySources()
	for _, group := range [][]string{scripts, connects, images} {
		if len(group) != 1 || group[0] != "https://momento.internal:8443" {
			t.Fatalf("sources = %v %v %v", scripts, connects, images)
		}
	}
	if config.MomentoTarget() != nil {
		t.Fatal("direct mode must not open the proxy")
	}
	if got := config.Snippet(""); strings.Contains(got, "nonce") {
		t.Fatalf("empty nonce must not be written: %q", got)
	}
}

func TestSnippetIncompleteProviderIsInactive(t *testing.T) {
	cases := []Config{
		{Enabled: true, Provider: ProviderMomento, MomentoSiteID: "x"},
		{Enabled: true, Provider: ProviderMomento, MomentoURL: "momento.internal", MomentoSiteID: "x"},
		{Enabled: true, Provider: ProviderGA4},
		{Enabled: true, Provider: ProviderMatomo, MatomoURL: "https://m.internal"},
		{Enabled: true, Provider: ProviderCustom, CustomSnippet: "   "},
		{Enabled: true, Provider: ProviderCustom, CustomSnippet: "<script>" + strings.Repeat("x", MaxSnippetBytes) + "</script>"},
		{Enabled: true, Provider: "pixel"},
		{Enabled: false, Provider: ProviderGA4, MeasurementID: "G-1"},
	}
	for index, config := range cases {
		if config.Active(false) {
			t.Fatalf("case %d: %+v should be inactive", index, config)
		}
		if config.Enabled && config.Validate() == nil {
			t.Fatalf("case %d: Validate should report the missing value", index)
		}
	}
	if err := (Config{Enabled: false, Provider: "pixel"}).Validate(); err != nil {
		t.Fatalf("disabled tracking must validate regardless of provider: %v", err)
	}
}

func TestWithNonceCoversEveryScriptTag(t *testing.T) {
	snippet := `<SCRIPT async src="https://a.example/x.js"></SCRIPT><script>init()</script><script nonce="keep">x</script><div>no script</div>`
	got := WithNonce(snippet, `n"1`)
	if strings.Count(got, `nonce="n&#34;1"`) != 2 || !strings.Contains(got, `nonce="keep"`) {
		t.Fatalf("WithNonce = %q", got)
	}
	if got := WithNonce("<script>a</script>", ""); got != "<script>a</script>" {
		t.Fatalf("empty nonce changed snippet: %q", got)
	}
}

func TestInjectPlacement(t *testing.T) {
	page := []byte("<!doctype html><html><head><title>x</title></head><body><div id=root></div></body></html>")
	head := string(Inject(page, "<script>h</script>", PlacementHead))
	if !strings.Contains(head, "<script>h</script>\n</head>") {
		t.Fatalf("head placement = %q", head)
	}
	body := string(Inject(page, "<script>b</script>", PlacementBody))
	if !strings.Contains(body, "<script>b</script>\n</body>") {
		t.Fatalf("body placement = %q", body)
	}
	if got := string(Inject([]byte("no markup"), "<script>x</script>", PlacementHead)); !strings.HasSuffix(got, "<script>x</script>\n") {
		t.Fatalf("fallback = %q", got)
	}
	if got := string(Inject(page, "", PlacementHead)); got != string(page) {
		t.Fatal("empty snippet must leave the page alone")
	}
}

func TestSnippetOriginsAndCustomPolicySources(t *testing.T) {
	snippet := `<script async src="https://cdn.example/t.js"></script>
<script>fetch('https://collect.example/v1/hit?x=1');new Image().src="http://pixel.example/p.gif";var s="https://cdn.example/other.js";</script>`
	origins := SnippetOrigins(snippet)
	want := []string{"https://cdn.example", "https://collect.example", "http://pixel.example"}
	if strings.Join(origins, " ") != strings.Join(want, " ") {
		t.Fatalf("origins = %v, want %v", origins, want)
	}
	config := Config{Enabled: true, Provider: ProviderCustom, CustomSnippet: snippet, AllowedHosts: "https://extra.example, https://*.wild.example\nhttps://cdn.example"}
	scripts, connects, images := config.PolicySources()
	joined := strings.Join(scripts, " ")
	for _, origin := range append(want, "https://extra.example", "https://*.wild.example") {
		if !strings.Contains(joined, origin) {
			t.Fatalf("scripts %v lack %s", scripts, origin)
		}
	}
	if len(connects) != len(scripts) || len(images) != len(scripts) {
		t.Fatalf("groups differ: %v %v %v", scripts, connects, images)
	}
	if got := SnippetOrigins("<script>var x = 'httpish';</script>"); len(got) != 0 {
		t.Fatalf("non-url text produced origins %v", got)
	}
}

func TestAddAllowedHost(t *testing.T) {
	if got := AddAllowedHost("", "https://a.example/"); got != "https://a.example" {
		t.Fatalf("first host = %q", got)
	}
	if got := AddAllowedHost("https://a.example", "HTTPS://A.EXAMPLE"); got != "https://a.example" {
		t.Fatalf("duplicate appended: %q", got)
	}
	if got := AddAllowedHost("https://a.example", "https://b.example"); got != "https://a.example, https://b.example" {
		t.Fatalf("append = %q", got)
	}
	if got := AddAllowedHost("https://a.example", "  "); got != "https://a.example" {
		t.Fatalf("blank changed list: %q", got)
	}
}

func TestRecorderKeepsDistinctOrigins(t *testing.T) {
	recorder := NewRecorder()
	clock := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	recorder.now = func() time.Time { return clock }
	recorder.Record("https://collect.example/v1/hit", "connect-src 'self'", "/dataworks/")
	clock = clock.Add(time.Minute)
	recorder.Record("https://collect.example/v1/other", "connect-src", "/dataworks/products")
	recorder.Record("https://cdn.example/t.js", "script-src-elem", "/dataworks/")
	recorder.Record("chrome-extension://abc/x.js", "script-src", "/dataworks/")
	recorder.Record("data", "img-src", "/dataworks/")
	recorder.Record("", "img-src", "/dataworks/")
	recorder.Record("https://empty.example", "", "/dataworks/")

	items := recorder.List(Config{AllowedHosts: "https://cdn.example"})
	if len(items) != 3 {
		t.Fatalf("items = %+v, want 3 distinct origins", items)
	}
	byKey := map[string]Violation{}
	for _, item := range items {
		byKey[item.Directive+" "+item.Origin] = item
	}
	collect := byKey["connect-src https://collect.example"]
	if collect.Count != 2 || collect.Page != "/dataworks/products" || collect.Allowed || !collect.LastSeen.Equal(clock) {
		t.Fatalf("collect = %+v", collect)
	}
	if cdn := byKey["script-src-elem https://cdn.example"]; !cdn.Allowed {
		t.Fatalf("allowed host not marked: %+v", cdn)
	}
	if empty := byKey["connect-src https://empty.example"]; empty.Count != 1 {
		t.Fatalf("empty directive should default to connect-src: %+v", byKey)
	}
	if items[0].Origin != "https://cdn.example" && items[0].Origin != "https://collect.example" && items[0].Origin != "https://empty.example" {
		t.Fatalf("unexpected order %+v", items)
	}
	recorder.Forget()
	if len(recorder.List(Config{})) != 0 {
		t.Fatal("Forget should clear the recorder")
	}
	var nilRecorder *Recorder
	nilRecorder.Record("https://x.example", "script-src", "/")
	if len(nilRecorder.List(Config{})) != 0 {
		t.Fatal("nil recorder must be safe")
	}
}

func TestRecorderEvictsOldestBeyondCapacity(t *testing.T) {
	recorder := NewRecorder()
	clock := time.Date(2026, 9, 12, 9, 0, 0, 0, time.UTC)
	recorder.now = func() time.Time { return clock }
	for index := 0; index <= MaxViolations; index++ {
		clock = clock.Add(time.Second)
		recorder.Record("https://host-"+strings.Repeat("a", index%7)+"-"+string(rune('a'+index%26))+strings.Repeat("z", index/26)+".example", "script-src", "/")
	}
	items := recorder.List(Config{})
	if len(items) != MaxViolations {
		t.Fatalf("len = %d, want %d", len(items), MaxViolations)
	}
	for _, item := range items {
		if item.Origin == "https://host--a.example" {
			t.Fatal("oldest entry should have been evicted")
		}
	}
}

func TestWildcardAllowMarksSubdomains(t *testing.T) {
	recorder := NewRecorder()
	recorder.Record("https://region1.google-analytics.com/g/collect", "connect-src", "/")
	items := recorder.List(Config{Enabled: true, Provider: ProviderGA4, MeasurementID: "G-1"})
	if len(items) != 1 || !items[0].Allowed {
		t.Fatalf("items = %+v", items)
	}
}
