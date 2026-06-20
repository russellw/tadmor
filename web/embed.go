// Package web embeds the built front-end (web/dist) so the Go server can serve
// the SPA from a single self-contained binary.
//
// The dist tree is produced by `make web-build` (Vite). A committed placeholder
// (dist/.gitkeep) keeps this package compiling before any build has run; until
// `make web-build` is run, only the placeholder is embedded and the server
// reports the front-end as not built.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

// DistFS returns the embedded production build, rooted at web/dist.
func DistFS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist is always present (at least the .gitkeep placeholder)
	}
	return sub
}
