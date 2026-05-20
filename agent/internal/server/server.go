package server

import (
	"net/http"

	"github.com/scottzx/remote-agents/agent/internal/config"
	"github.com/scottzx/remote-agents/agent/internal/fs"
	"github.com/scottzx/remote-agents/agent/internal/gateway"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/ws               → Reverse-proxy to ttyd WebSocket endpoint
//	/token            → Reverse-proxy to ttyd auth-token endpoint
//	/                 → Static file server (compiled frontend assets)
func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// ── File system API ──────────────────────────────────────────────────────
	fsHandler := fs.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/fs/list", fsHandler.List)     // GET  ?path=.
	mux.HandleFunc("/api/fs/read", fsHandler.Read)     // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/write", fsHandler.Write)   // POST ?path=./main.go
	mux.HandleFunc("/api/fs/mkdir", fsHandler.Mkdir)   // POST ?path=./newdir
	mux.HandleFunc("/api/fs/delete", fsHandler.Delete) // DELETE ?path=./main.go

	// ── ttyd reverse proxy ───────────────────────────────────────────────────
	// All WebSocket and HTTP traffic destined for ttyd is forwarded here.
	// The frontend should connect to ws://<host>/ws (not directly to ttyd).
	ttydProxy := gateway.NewTtydProxy(cfg.TtydAddr)
	mux.Handle("/ws", ttydProxy)      // terminal WebSocket stream
	mux.Handle("/token", ttydProxy)   // ttyd auth token endpoint

	// ── Static frontend assets ───────────────────────────────────────────────
	// This catch-all must be registered last so it does not shadow the routes
	// above. html/dist must contain an index.html for SPA-style navigation.
	staticFS := http.FileServer(http.Dir(cfg.StaticDir))
	mux.Handle("/", staticFS)

	return mux
}
