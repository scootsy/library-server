// Package embedded provides access to the compiled Svelte SPA assets
// via Go's embed directive. The assets are baked into the binary at
// build time.
package embedded

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// WebFS returns a filesystem rooted at the dist/ directory containing
// the built Svelte SPA. Returns nil if the embedded assets are not available.
func WebFS() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		return nil
	}
	return sub
}
