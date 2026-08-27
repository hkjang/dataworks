package proxy

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	dataworksweb "dataworks/web"
)

const dataWorksSPAPrefix = "/dataworks/"

func dataWorksSPAAssets() fs.FS {
	return dataworksweb.Dist()
}

// spaHandler serves a built SPA without exposing directory listings. Files
// produced by Vite are served directly and clean client-side routes fall back
// to index.html.
type spaHandler struct {
	files  fs.FS
	prefix string
}

func newSPAHandler(files fs.FS, prefix string) http.Handler {
	return &spaHandler{files: files, prefix: prefix}
}

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h.files == nil || h.prefix == "" || !strings.HasPrefix(r.URL.Path, h.prefix) {
		http.NotFound(w, r)
		return
	}

	requested := strings.TrimPrefix(r.URL.Path, h.prefix)
	if !validSPAPath(requested) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}
	if requested == "" {
		h.serveIndex(w, r)
		return
	}

	// A trailing slash can be a valid React Router location, but never names a
	// regular static file in a Vite build.
	hasTrailingSlash := strings.HasSuffix(requested, "/")
	fileName := strings.TrimSuffix(requested, "/")
	if !hasTrailingSlash {
		info, err := fs.Stat(h.files, fileName)
		if err == nil {
			if info.IsDir() {
				if fileName == "assets" {
					h.serveIndex(w, r)
					return
				}
				http.NotFound(w, r)
				return
			}
			h.serveFile(w, r, fileName, fileName == "index.html")
			return
		}
		if !errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "Data Works UI asset is unavailable", http.StatusInternalServerError)
			return
		}
	}

	// Missing files and Vite's assets namespace are real 404s. Returning HTML
	// for them would turn a deployment error into a confusing script/style MIME
	// failure in the browser. Extensionless locations are React Router routes.
	// `/dataworks/assets` is also the React asset-catalog route. Only the
	// trailing-slash directory form and descendants belong to Vite's static
	// namespace; the exact extensionless route must fall back to the SPA.
	if (fileName == "assets" && hasTrailingSlash) || strings.HasPrefix(fileName, "assets/") || path.Ext(fileName) != "" {
		http.NotFound(w, r)
		return
	}
	h.serveIndex(w, r)
}

func validSPAPath(name string) bool {
	if name == "" {
		return true
	}
	if strings.ContainsRune(name, '\x00') || strings.ContainsRune(name, '\\') {
		return false
	}
	name = strings.TrimSuffix(name, "/")
	if !fs.ValidPath(name) {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if strings.HasPrefix(segment, ".") {
			return false
		}
	}
	return true
}

func (h *spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	if _, err := fs.Stat(h.files, "index.html"); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			http.Error(w, "Data Works UI has not been built", http.StatusServiceUnavailable)
			return
		}
		http.Error(w, "Data Works UI is unavailable", http.StatusInternalServerError)
		return
	}
	h.serveFile(w, r, "index.html", true)
}

func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string, index bool) {
	file, err := h.files.Open(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("X-Content-Type-Options", "nosniff")
	if index {
		w.Header().Set("Cache-Control", "no-cache")
	} else if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}

	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(w, r, name, info.ModTime(), seeker)
		return
	}
	content, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "Data Works UI asset is unavailable", http.StatusInternalServerError)
		return
	}
	http.ServeContent(w, r, name, info.ModTime(), bytes.NewReader(content))
}

func handleDataWorksSPARoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != strings.TrimSuffix(dataWorksSPAPrefix, "/") {
		http.NotFound(w, r)
		return
	}
	target := dataWorksSPAPrefix
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	http.Redirect(w, r, target, http.StatusPermanentRedirect)
}
