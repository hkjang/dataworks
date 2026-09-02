package proxy

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The OpenAPI catalog and the mux registrations drift apart silently: a new route simply never
// shows up in /openapi.json, and a deleted route lingers there forever. cmd/api-surface-audit
// checks this, but nothing ran it — these tests put the same invariant inside `go test ./...`.

var reMuxHandleFunc = regexp.MustCompile(`mux\.HandleFunc\(\s*"([^"]+)"`)

// registeredRoutes statically collects every path passed to mux.HandleFunc in the package
// sources. Routes() builds the mux at runtime but exposes no way to enumerate its patterns.
func registeredRoutes(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob package sources: %v", err)
	}
	seen := map[string]bool{}
	out := []string{}
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range reMuxHandleFunc.FindAllStringSubmatch(string(body), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				out = append(out, m[1])
			}
		}
	}
	if len(out) < 100 {
		t.Fatalf("found only %d registered routes — the extractor is broken, not the catalog", len(out))
	}
	sort.Strings(out)
	return out
}

// catalogCovers reports whether some apiEndpoints path documents this route. A route registered
// with a trailing slash is a prefix handler, so any documented path beneath it counts.
func catalogCovers(route string) bool {
	trimmed := strings.TrimRight(route, "/")
	for _, e := range apiEndpoints {
		if e.path == route || e.path == trimmed {
			return true
		}
		if strings.HasSuffix(route, "/") && strings.HasPrefix(e.path, route) {
			return true
		}
	}
	return false
}

// routeServes reports whether some registered route serves this documented path. Documented
// {param} segments resolve against the prefix handler they live under.
func routeServes(doc string, routes []string) bool {
	prefix := doc
	if i := strings.Index(doc, "{"); i >= 0 {
		prefix = doc[:i]
	}
	for _, r := range routes {
		if r == doc || r == strings.TrimRight(prefix, "/") {
			return true
		}
		if strings.HasSuffix(r, "/") && (strings.HasPrefix(doc, r) || strings.HasPrefix(prefix, r)) {
			return true
		}
	}
	return false
}

// TestOpenAPICatalogCoversEveryRoute fails when a route is registered but never documented, so a
// new endpoint cannot ship missing from /openapi.json.
func TestOpenAPICatalogCoversEveryRoute(t *testing.T) {
	missing := []string{}
	for _, r := range registeredRoutes(t) {
		if !catalogCovers(r) {
			missing = append(missing, r)
		}
	}
	if len(missing) > 0 {
		t.Errorf("%d registered route(s) missing from apiEndpoints — add them to admin_openapi.go: %v", len(missing), missing)
	}
}

// TestOpenAPICatalogHasNoStaleEntries fails when the catalog documents a path no route serves,
// so a renamed or deleted endpoint cannot linger in the published spec.
func TestOpenAPICatalogHasNoStaleEntries(t *testing.T) {
	routes := registeredRoutes(t)
	stale := []string{}
	for _, e := range apiEndpoints {
		if !routeServes(e.path, routes) {
			stale = append(stale, e.path)
		}
	}
	if len(stale) > 0 {
		t.Errorf("%d catalog entr(ies) serve no registered route — drop them from admin_openapi.go: %v", len(stale), stale)
	}
}

// TestOpenAPICatalogEntriesAreWellFormed guards the shape of new rows: an absolute path, at least
// one lowercase HTTP method, a tag, and a summary. buildOpenAPISpec silently emits a pathless or
// method-less entry as an empty operation object.
func TestOpenAPICatalogEntriesAreWellFormed(t *testing.T) {
	allowed := map[string]bool{"get": true, "post": true, "put": true, "patch": true, "delete": true}
	seen := map[string]bool{}
	for _, e := range apiEndpoints {
		if !strings.HasPrefix(e.path, "/") {
			t.Errorf("catalog path %q must be absolute", e.path)
		}
		if seen[e.path] {
			t.Errorf("catalog path %q is listed twice — merge the methods into one entry", e.path)
		}
		seen[e.path] = true
		if len(e.methods) == 0 {
			t.Errorf("catalog entry %q declares no methods", e.path)
		}
		for _, m := range e.methods {
			if !allowed[m] {
				t.Errorf("catalog entry %q has unsupported method %q (must be lowercase)", e.path, m)
			}
		}
		if strings.TrimSpace(e.tag) == "" || strings.TrimSpace(e.summary) == "" {
			t.Errorf("catalog entry %q needs both a tag and a summary", e.path)
		}
	}
}
