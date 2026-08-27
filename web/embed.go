// Package web contains the Data Works single-page application bundled with the
// Go server.
package web

import (
	"embed"
	"io/fs"
)

// distFiles always has at least dist/.gitkeep in a source checkout. The Vite
// build replaces the directory contents with the production application before
// the Go binary is built in Docker.
//
//go:embed all:dist
var distFiles embed.FS

// Dist returns the production SPA filesystem rooted at web/dist.
func Dist() fs.FS {
	dist, err := fs.Sub(distFiles, "dist")
	if err != nil {
		// The embedded tree is fixed at compile time, so this indicates a broken
		// build rather than a runtime condition callers can recover from.
		panic("dataworks web: embedded dist directory is unavailable: " + err.Error())
	}
	return dist
}
