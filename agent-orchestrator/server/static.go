package server

import (
	"io/fs"
	"net/http"

	uiEmbed "agent-orchestrator/ui"
)

// registerStaticHandler serves the compiled Svelte SPA from the embedded
// filesystem. Any path that does not match a real file is served as
// index.html so that the client-side hash router handles it.
func (s *Server) registerStaticHandler() {
	distFS, err := fs.Sub(uiEmbed.FS, "dist")
	if err != nil {
		// UI not built yet — skip static serving.
		return
	}

	fileServer := http.FileServer(http.FS(distFS))

	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Let API and WebSocket requests fall through to their own handlers.
		// The ServeMux routing means this "/" catch-all only fires when no
		// more-specific pattern matched, so /api/* and /ws/* are already
		// handled before we get here.

		// SPA fallback: if the requested file does not exist inside dist/,
		// serve index.html and let the hash router take over.
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else {
			// Strip leading slash.
			if len(path) > 0 && path[0] == '/' {
				path = path[1:]
			}
		}

		if _, err := distFS.Open(path); err != nil {
			// File not found — serve index.html for SPA routing.
			r2 := *r
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, &r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}
