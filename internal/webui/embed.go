// Package webui embeds the built React SPA (Vite outputs to dist/). A tracked
// placeholder index.html keeps this compiling before any frontend build.
package webui

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// DistFS returns the embedded frontend rooted at dist/.
func DistFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(err) // dist is always embedded
	}
	return sub
}
