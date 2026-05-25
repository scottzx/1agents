package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/scottzx/remote-agents/agent/internal/ccconnect"
	"github.com/scottzx/remote-agents/agent/internal/config"
	ctxt "github.com/scottzx/remote-agents/agent/internal/context"
	"github.com/scottzx/remote-agents/agent/internal/fs"
	"github.com/scottzx/remote-agents/agent/internal/gateway"
	"github.com/scottzx/remote-agents/agent/internal/git"
	"github.com/scottzx/remote-agents/agent/internal/terminal"
	"github.com/scottzx/remote-agents/agent/internal/workspace"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/api/workspace/*  → Workspace CRUD handlers (Go, JSON file storage)
//	/api/terminal/*   → Tmux terminal session management (create/list/kill/switch)
//	/ws               → Reverse-proxy to ttyd WebSocket endpoint
//	/token            → Reverse-proxy to ttyd auth-token endpoint
//	/                 → Static file server (compiled frontend assets)
func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// ── File system API ──────────────────────────────────────────────────────
	fsHandler := fs.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/fs/list", fsHandler.List)     // GET  ?path=.
	mux.HandleFunc("/api/fs/read", fsHandler.Read)     // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/view", fsHandler.View)     // GET  ?path=./page.html (serves with correct content-type)
	mux.HandleFunc("/api/fs/image", fsHandler.Image)   // GET  ?path=./image.png (returns base64 data URL)
	mux.HandleFunc("/api/fs/write", fsHandler.Write)   // POST ?path=./main.go
	mux.HandleFunc("/api/fs/mkdir", fsHandler.Mkdir)   // POST ?path=./newdir
	mux.HandleFunc("/api/fs/delete", fsHandler.Delete) // DELETE ?path=./main.go

	// ── Workspace API ────────────────────────────────────────────────────────
	wsHandler := workspace.NewHandler()
	mux.HandleFunc("/api/workspace/list", wsHandler.List)     // GET
	mux.HandleFunc("/api/workspace/create", wsHandler.Create) // POST
	mux.HandleFunc("/api/workspace/update", wsHandler.Update) // POST
	mux.HandleFunc("/api/workspace/delete", wsHandler.Delete)           // DELETE ?id=xxx
	mux.HandleFunc("/api/workspace/pick-directory", wsHandler.PickDirectory) // POST — opens native folder picker
	mux.HandleFunc("/api/workspace/list-directories", wsHandler.ListDirectories) // GET ?path=...

	mux.HandleFunc("/api/cc-connect/url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Workspace string `json:"workspace"`
			Theme     string `json:"theme"`
			Lang      string `json:"lang"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		wsConfig, err := wsHandler.LoadWorkspacesConfig()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		var foundWS *workspace.Workspace
		for i := range wsConfig.Workspaces {
			if wsConfig.Workspaces[i].ID == body.Workspace {
				foundWS = &wsConfig.Workspaces[i]
				break
			}
		}

		if foundWS == nil {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}

		redirectPath := ""
		if foundWS.ChatChannel != "" {
			redirectPath = "/chat/" + foundWS.ChatChannel
		} else {
			projName := foundWS.Name
			if projName == "" {
				projName = foundWS.ID
			}
			redirectPath = "/projects/" + projName
		}

		// Normalize language codes from BCP-47 to CC-Connect codes
		normalLang := "zh"
		langLower := strings.ToLower(body.Lang)
		if strings.HasPrefix(langLower, "en") {
			normalLang = "en"
		} else if strings.HasPrefix(langLower, "zh-tw") || strings.HasPrefix(langLower, "zh-hk") {
			normalLang = "zh-TW"
		} else if strings.HasPrefix(langLower, "ja") {
			normalLang = "ja"
		} else if strings.HasPrefix(langLower, "es") {
			normalLang = "es"
		}

		url := fmt.Sprintf("/cc-connect/login?token=%s&redirect=%s&theme=%s&lang=%s",
			ccconnect.ManagementToken,
			url.QueryEscape(redirectPath),
			body.Theme,
			normalLang,
		)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]string{"url": url})
	})

	// ── Git API ───────────────────────────────────────────────────────────────
	gitHandler := git.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/git/status", gitHandler.Status)     // GET
	mux.HandleFunc("/api/git/diff", gitHandler.Diff)         // GET  ?file=<path>&staged=<bool>
	mux.HandleFunc("/api/git/stage", gitHandler.Stage)       // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/unstage", gitHandler.Unstage)   // POST ?file=<path> or ?all=true
	mux.HandleFunc("/api/git/commit", gitHandler.Commit)     // POST {message:"…"}
	mux.HandleFunc("/api/git/log", gitHandler.Log)           // GET  ?limit=20
	mux.HandleFunc("/api/git/branches", gitHandler.Branches) // GET
	mux.HandleFunc("/api/git/checkout", gitHandler.Checkout) // POST {branch:"…",create:bool}
	mux.HandleFunc("/api/git/push", gitHandler.Push)         // POST
	mux.HandleFunc("/api/git/pull", gitHandler.Pull)         // POST

	// ── Workspace context API (switches fs + git roots at runtime) ─────────
	ctxHandler := ctxt.NewHandler(fsHandler, gitHandler)
	mux.HandleFunc("/api/context/set", ctxHandler.Set) // POST {"path":"..."}
	mux.HandleFunc("/api/context/get", ctxHandler.Get) // GET
	// ── Terminal API (tmux session management) ────────────────────────────────
	termHandler := terminal.NewHandler(cfg)
	mux.HandleFunc("/api/terminal/create", termHandler.Create) // POST {workspaceId, cwd}
	mux.HandleFunc("/api/terminal/list", termHandler.List)     // GET
	mux.HandleFunc("/api/terminal/kill", termHandler.Kill)     // POST {windowIndex}
	mux.HandleFunc("/api/terminal/switch", termHandler.Switch) // POST {windowIndex}
	mux.HandleFunc("/api/terminal/mouse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			termHandler.GetMouse(w, r)
		} else if r.Method == http.MethodPost {
			termHandler.SetMouse(w, r)
		} else {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// ── ttyd reverse proxy ───────────────────────────────────────────────────
	// All WebSocket and HTTP traffic destined for ttyd is forwarded here.
	// The frontend should connect to ws://<host>/ws (not directly to ttyd).
	ttydProxy := gateway.NewTtydProxy(cfg.TtydAddr)
	mux.Handle("/ws", ttydProxy)      // terminal WebSocket stream
	mux.Handle("/token", ttydProxy)   // ttyd auth token endpoint

	// ── CC-Connect reverse proxy ─────────────────────────────────────────────
	// Transparently reverse-proxies requests to the local CC-Connect management server
	// under the main HTTPS gateway, resolving LAN protocol security and Mixed Content.
	mux.Handle("/cc-connect/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/assets/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))
	mux.Handle("/api/v1/", gateway.NewCCConnectProxy(ccconnect.ManagementPort))

	// ── Static frontend assets ───────────────────────────────────────────────
	// This catch-all must be registered last so it does not shadow the routes
	// above. html/dist must contain an index.html for SPA-style navigation.
	staticFS := http.FileServer(http.Dir(cfg.StaticDir))
	mux.Handle("/", staticFS)

	return mux
}
