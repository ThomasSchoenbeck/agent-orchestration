// Package ui exposes the compiled Svelte UI as an embedded filesystem.
// Run `pnpm build` inside the ui/ directory before `go build` to populate dist/.
package ui

import "embed"

// FS holds the compiled Svelte app rooted at "dist/".
// Access files via fs.Sub(FS, "dist") or directly as FS.Open("dist/index.html").
//
//go:embed all:dist
var FS embed.FS
