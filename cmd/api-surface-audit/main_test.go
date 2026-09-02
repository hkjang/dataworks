package main

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// The audit used to point at cmd/vibe/main.go and sdk/typescript/vibe.ts, which no longer exist:
// the reads warned, returned "", and the CLI/SDK contract check passed on zero paths. Every source
// main() names must exist, and must actually yield client paths — otherwise the audit is vacuous.
func TestAuditSourcesExistAndYieldPaths(t *testing.T) {
	repo := filepath.Join("..", "..")
	reRead := regexp.MustCompile(`read\(filepath\.Join\((.+)\)\)`)
	reArg := regexp.MustCompile(`"([^"]+)"`)

	body, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatalf("read main.go: %v", err)
	}
	joins := reRead.FindAllStringSubmatch(string(body), -1)
	if len(joins) != 3 {
		t.Fatalf("expected main() to read 3 sources (openapi, cli, sdk), found %d", len(joins))
	}
	for _, j := range joins {
		parts := []string{repo}
		for _, a := range reArg.FindAllStringSubmatch(j[1], -1) {
			parts = append(parts, a[1])
		}
		path := filepath.Join(parts...)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("audit reads a source that does not exist: %s (%v)", path, err)
		}
	}

	cli, err := os.ReadFile(filepath.Join(repo, "cmd", "clustara-cli", "main.go"))
	if err != nil {
		t.Fatalf("read CLI source: %v", err)
	}
	if got := extractMatches(reClientGo, string(cli)); len(got) == 0 {
		t.Error("CLI source yields no client paths — the contract check would pass vacuously")
	}
	sdk, err := os.ReadFile(filepath.Join(repo, "sdk", "typescript", "clustara.ts"))
	if err != nil {
		t.Fatalf("read SDK source: %v", err)
	}
	if got := extractMatches(reClientTS, string(sdk)); len(got) == 0 {
		t.Error("SDK source yields no client paths — the contract check would pass vacuously")
	}
}

func TestPathCovered(t *testing.T) {
	routes := []string{"/v1/models", "/v1/apps/", "/mcp/gateway", "/me/connection-doctor", "/v1/"}
	cases := []struct {
		path string
		want bool
	}{
		{"/v1/models", true}, // exact
		{"/v1/apps/${encodeURIComponent(appId)}/run", true}, // prefix handler + template param
		{"/v1/apps/", true},                    // the prefix itself
		{"/mcp/gateway", true},                 // exact
		{"/me/connection-doctor", true},        // exact
		{"/v1/chat/completions", true},         // covered by "/v1/" prefix
		{"/v1/nonexistent-but-under-v1", true}, // also under "/v1/"
		{"/admin/secret", false},               // not served by any client route here
	}
	for _, c := range cases {
		if got := pathCovered(c.path, routes); got != c.want {
			t.Errorf("pathCovered(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestExtractors(t *testing.T) {
	server := `mux.HandleFunc("/v1/apps/", h)` + "\n" + `mux.HandleFunc( "/mcp/gateway" , h)`
	if r := extractMatches(reHandleFunc, server); len(r) != 2 || r[0] != "/v1/apps/" || r[1] != "/mcp/gateway" {
		t.Fatalf("server routes = %v", r)
	}
	openapi := `{"/v1/apps/{id}/run", []string{"post"}, "apps", "...", false},`
	if d := extractMatches(reOpenAPI, openapi); len(d) != 1 || d[0] != "/v1/apps/{id}/run" {
		t.Fatalf("openapi paths = %v", d)
	}
	cliGo := `cfg.do(http.MethodPost, "/v1/apps/"+appID+"/run", nil)` + "\n" + `cfg.do("GET", "/me/connection-doctor", nil)`
	if c := extractMatches(reClientGo, cliGo); len(c) != 2 {
		t.Fatalf("cli paths = %v", c)
	}
	sdkTS := "this.req(\"POST\", `/v1/apps/${id}/run`, {})\nthis.req(\"GET\", \"/v1/models\")"
	got := extractMatches(reClientTS, sdkTS)
	if len(got) != 2 {
		t.Fatalf("sdk paths = %v", got)
	}
}

// The real repo sources must keep the CLI/SDK contract intact (no cli_only/sdk_only).
func TestBuildReportContractIntact(t *testing.T) {
	server := []string{`
		mux.HandleFunc("/v1/chat/completions", h)
		mux.HandleFunc("/v1/models", h)
		mux.HandleFunc("/v1/apps/", h)
		mux.HandleFunc("/v1/workflows/", h)
		mux.HandleFunc("/mcp/gateway", h)
		mux.HandleFunc("/me/connection-doctor", h)
	`}
	openapi := `{"/v1/models", []string{"get"}, "x", "y", false},`
	cli := `do("GET","/v1/models", nil); do("POST","/mcp/gateway", nil); do("POST","/me/connection-doctor", nil)`
	sdk := "req(\"POST\", \"/v1/chat/completions\"); req(\"POST\", `/v1/apps/${id}/run`); req(\"POST\", `/v1/workflows/${id}/run`); req(\"POST\", \"/mcp/gateway\")"
	rep := buildReport(server, openapi, cli, sdk)
	if len(rep.CLIOnly) != 0 {
		t.Errorf("expected no cli_only, got %v", rep.CLIOnly)
	}
	if len(rep.SDKOnly) != 0 {
		t.Errorf("expected no sdk_only, got %v", rep.SDKOnly)
	}
	// A SDK path with no server route must be flagged.
	rep2 := buildReport([]string{`mux.HandleFunc("/v1/models", h)`}, "", "", "req(\"POST\", \"/v1/ghost\")")
	if len(rep2.SDKOnly) != 1 || rep2.SDKOnly[0] != "/v1/ghost" {
		t.Fatalf("expected /v1/ghost flagged as sdk_only, got %v", rep2.SDKOnly)
	}
}
