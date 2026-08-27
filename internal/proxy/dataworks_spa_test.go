package proxy

import (
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"

	"dataworks/internal/config"
)

func testSPAHandler() http.Handler {
	return newSPAHandler(fstest.MapFS{
		"index.html":              &fstest.MapFile{Data: []byte("<!doctype html><title>Data Works</title>")},
		"assets/app-a1b2c3.js":    &fstest.MapFile{Data: []byte("console.log('dataworks')")},
		"assets/app-a1b2c3.css":   &fstest.MapFile{Data: []byte("body { color: #111; }")},
		"product-manifest.json":   &fstest.MapFile{Data: []byte(`{"name":"Data Works"}`)},
		"assets/nested/asset.txt": &fstest.MapFile{Data: []byte("nested")},
	}, dataWorksSPAPrefix)
}

func TestSPAHandlerServesIndexAndClientRoutes(t *testing.T) {
	handler := testSPAHandler()
	for _, requestPath := range []string{
		"/dataworks/",
		"/dataworks/index.html",
		"/dataworks/assets",
		"/dataworks/products/credit-insight",
		"/dataworks/products/credit-insight/",
	} {
		t.Run(requestPath, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			response := recorder.Result()
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d; body=%s", response.StatusCode, http.StatusOK, body)
			}
			if !strings.Contains(string(body), "Data Works") {
				t.Fatalf("expected SPA index, got %q", body)
			}
			if got := response.Header.Get("Cache-Control"); got != "no-cache" {
				t.Fatalf("Cache-Control = %q, want no-cache", got)
			}
		})
	}
}

func TestSPAHandlerServesStaticAssets(t *testing.T) {
	handler := testSPAHandler()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dataworks/assets/app-a1b2c3.js", nil))

	response := recorder.Result()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := string(body); got != "console.log('dataworks')" {
		t.Fatalf("body = %q", got)
	}
	if got := response.Header.Get("Content-Type"); !strings.Contains(got, "javascript") {
		t.Fatalf("Content-Type = %q, want JavaScript", got)
	}
	if got := response.Header.Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("Cache-Control = %q", got)
	}
	if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

func TestSPAHandlerDoesNotFallbackForMissingAssets(t *testing.T) {
	handler := testSPAHandler()
	for _, requestPath := range []string{
		"/dataworks/assets/missing.js",
		"/dataworks/missing.ico",
		"/dataworks/assets/",
	} {
		t.Run(requestPath, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, requestPath, nil))
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusNotFound, recorder.Body.String())
			}
		})
	}
}

func TestSPAHandlerRejectsUnsafePaths(t *testing.T) {
	handler := testSPAHandler()
	unsafePaths := []string{
		"/dataworks/../secret",
		"/dataworks/assets/../index.html",
		"/dataworks//index.html",
		"/dataworks/.gitkeep",
		"/dataworks/assets\\app.js",
	}
	for _, requestPath := range unsafePaths {
		t.Run(requestPath, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := &http.Request{Method: http.MethodGet, URL: &url.URL{Path: requestPath}, Header: make(http.Header)}
			handler.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func TestSPAHandlerMethodAndHead(t *testing.T) {
	handler := testSPAHandler()

	post := httptest.NewRecorder()
	handler.ServeHTTP(post, httptest.NewRequest(http.MethodPost, "/dataworks/", nil))
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want %d", post.Code, http.StatusMethodNotAllowed)
	}
	if got := post.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("Allow = %q", got)
	}

	head := httptest.NewRecorder()
	handler.ServeHTTP(head, httptest.NewRequest(http.MethodHead, "/dataworks/assets/app-a1b2c3.js", nil))
	if head.Code != http.StatusOK {
		t.Fatalf("HEAD status = %d, want %d", head.Code, http.StatusOK)
	}
	if head.Body.Len() != 0 {
		t.Fatalf("HEAD body length = %d, want 0", head.Body.Len())
	}
}

func TestSPAHandlerMissingBuildReturnsServiceUnavailable(t *testing.T) {
	handler := newSPAHandler(fstest.MapFS{
		".gitkeep": &fstest.MapFile{Data: []byte("placeholder")},
	}, dataWorksSPAPrefix)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/dataworks/", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
}

func TestDataWorksSPARootRedirect(t *testing.T) {
	recorder := httptest.NewRecorder()
	handleDataWorksSPARoot(recorder, httptest.NewRequest(http.MethodGet, "/dataworks?from=test", nil))
	if recorder.Code != http.StatusPermanentRedirect {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPermanentRedirect)
	}
	if got := recorder.Header().Get("Location"); got != "/dataworks/?from=test" {
		t.Fatalf("Location = %q", got)
	}
}

func TestDataWorksSPARoutesDoNotShadowAdmin(t *testing.T) {
	server := &Server{cfg: config.Config{Auth: config.AuthConfig{AdminToken: "admin-token"}}}
	handler := server.Routes()

	api := httptest.NewRecorder()
	handler.ServeHTTP(api, httptest.NewRequest(http.MethodGet, "/admin/dataworks/home", nil))
	if api.Code != http.StatusUnauthorized {
		t.Fatalf("admin API status = %d, want %d; body=%s", api.Code, http.StatusUnauthorized, api.Body.String())
	}
	if got := api.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("admin API Content-Type = %q, want JSON", got)
	}

	legacyAdmin := httptest.NewRecorder()
	handler.ServeHTTP(legacyAdmin, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if legacyAdmin.Code != http.StatusOK {
		t.Fatalf("legacy admin status = %d, want %d", legacyAdmin.Code, http.StatusOK)
	}
	if got := legacyAdmin.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("legacy admin Content-Type = %q, want HTML", got)
	}
}

func TestEmbeddedDataWorksDistIsAvailable(t *testing.T) {
	// This protects the source-checkout placeholder while also passing after a
	// Vite build replaces it with production assets.
	entries, err := fs.ReadDir(dataWorksSPAAssets(), ".")
	if err != nil {
		t.Fatalf("read embedded dist: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("embedded dist is empty")
	}
}
