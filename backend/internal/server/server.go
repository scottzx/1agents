package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/auth"
	"github.com/scottzx/1Agents/backend/internal/ccconnect"
	"github.com/scottzx/1Agents/backend/internal/config"
	ctxt "github.com/scottzx/1Agents/backend/internal/context"
	"github.com/scottzx/1Agents/backend/internal/fs"
	"github.com/scottzx/1Agents/backend/internal/gateway"
	"github.com/scottzx/1Agents/backend/internal/git"
	"github.com/scottzx/1Agents/backend/internal/meta"
	"github.com/scottzx/1Agents/backend/internal/system"
	"github.com/scottzx/1Agents/backend/internal/terminal"
	"github.com/scottzx/1Agents/backend/internal/tunnel"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/api/workspace/*  → Workspace CRUD handlers (Go, JSON file storage)
//	/api/agent/*      → 1agents-side chat session index (Go, JSON file storage)
//	/api/terminal/*   → Tmux terminal session management (create/list/kill/switch)
//	/api/system/*     → System management: version info, OTA self-update
//	/ws               → Reverse-proxy to ttyd WebSocket endpoint
//	/token            → Reverse-proxy to ttyd auth-token endpoint
//	/                 → Static file server (compiled frontend assets)
func NewRouter(cfg *config.Config) http.Handler {
	mux := http.NewServeMux()

	// ── File system API ──────────────────────────────────────────────────────
	fsHandler := fs.NewHandler(cfg.WorkDir)
	mux.HandleFunc("/api/fs/list", fsHandler.List)     // GET  ?path=.
	mux.HandleFunc("/api/fs/search", fsHandler.Search) // GET  ?query=xxx&tag=all/doc/img/code
	mux.HandleFunc("/api/fs/read", fsHandler.Read)     // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/view", fsHandler.View)     // GET  ?path=./page.html (serves with correct content-type)
	mux.HandleFunc("/api/fs/view/", fsHandler.View)    // GET  /api/fs/view/relative/path (prefix route for relative assets support)
	mux.HandleFunc("/api/fs/image", fsHandler.Image)     // GET  ?path=./image.png (returns base64 data URL, deprecated)
	mux.HandleFunc("/api/fs/image/", fsHandler.ImageStream) // GET  /api/fs/image/relative/path (streams raw bytes; preferred)
	mux.HandleFunc("/api/fs/write", fsHandler.Write)   // POST ?path=./main.go
	mux.HandleFunc("/api/fs/mkdir", fsHandler.Mkdir)   // POST ?path=./newdir
	mux.HandleFunc("/api/fs/delete", fsHandler.Delete) // DELETE ?path=./main.go

	// ── Workspace API ────────────────────────────────────────────────────────
	wsHandler := workspace.NewHandler(cfg.TmuxSession)
	mux.HandleFunc("/api/workspace/list", wsHandler.List)     // GET
	mux.HandleFunc("/api/workspace/create", wsHandler.Create) // POST
	mux.HandleFunc("/api/workspace/update", wsHandler.Update) // POST
	mux.HandleFunc("/api/workspace/reorder", wsHandler.Reorder) // POST
	mux.HandleFunc("/api/workspace/delete", wsHandler.Delete)           // DELETE ?id=xxx
	mux.HandleFunc("/api/workspace/pick-directory", wsHandler.PickDirectory) // POST — opens native folder picker
	mux.HandleFunc("/api/workspace/list-directories", wsHandler.ListDirectories) // GET ?path=...

	// ── Agent (chat session) index API ─────────────────────────────────────
	// 1agents-side metadata store. The actual conversation lives in
	// cc-connect; these endpoints only index a session created via the
	// cc-connect REST/WS so the sidebar can list it like a terminal session.
	agentStore, err := agent.NewStore()
	if err != nil {
		log.Printf("[server] agent store init failed: %v", err)
	} else {
		// One-time import of the legacy JSON stores into ~/.1agents/meta.db
		// (renames the source files to *.migrated; no-op afterwards).
		if db, dbErr := meta.OpenDefault(); dbErr == nil {
			if wsCfg, wsErr := wsHandler.LoadWorkspacesConfig(); wsErr == nil {
				refs := make([]meta.WorkspaceRef, len(wsCfg.Workspaces))
				for i, ws := range wsCfg.Workspaces {
					refs[i] = meta.WorkspaceRef{ID: ws.ID, Name: ws.Name, Path: ws.Path}
				}
				if migErr := db.MigrateLegacy(refs); migErr != nil {
					log.Printf("[server] legacy metadata migration: %v", migErr)
				}
			}
			mux.HandleFunc("/api/projects", meta.ProjectsHandler(db)) // GET, POST
		}

		tasksStore, tsErr := agent.NewTasksStore()
		if tsErr != nil {
			log.Printf("[server] tasks store init failed: %v", tsErr)
		} else {
			acpxClient := agent.NewAcpxClient(38082)

			scheduler := agent.NewScheduler(tasksStore, func() ([]agent.WorkspaceRef, error) {
				wsHandler := workspace.NewHandler()
				wsCfg, err := wsHandler.LoadWorkspacesConfig()
				if err != nil {
					return nil, err
				}
				refs := make([]agent.WorkspaceRef, len(wsCfg.Workspaces))
				for i, ws := range wsCfg.Workspaces {
					refs[i] = agent.WorkspaceRef{ID: ws.ID, Name: ws.Name, Path: ws.Path}
				}
				return refs, nil
			})
			// Headless executor: scheduler-triggered tasks run through the
			// 1acp bridge with no frontend involved (automation-first).
			scheduler.SetRunner(agent.NewTaskRunner(38082, tasksStore, agentStore, scheduler))
			scheduler.Start(context.Background())

			agentHandler := agent.NewHandler(agentStore, tasksStore, acpxClient, scheduler)
			mux.HandleFunc("/api/agent/agent-types", agentHandler.HandleAgentTypes)  // GET
			mux.HandleFunc("/api/agent/sessions", agentHandler.HandleSessionsRoot)   // GET, POST
			mux.HandleFunc("/api/agent/sessions/", agentHandler.HandleSessionsItem)  // GET, DELETE /{id}
			mux.HandleFunc("/api/agent/tasks", agentHandler.HandleTasksRoot)         // GET, POST
			mux.HandleFunc("/api/agent/tasks/", agentHandler.HandleTasksItem)        // DELETE /{id}
			mux.HandleFunc("/api/agent/chat/ws", agentHandler.HandleChatWs)          // WebSocket upgrade & bridge
		}
	}

	mux.HandleFunc("/api/cc-connect/url", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var body struct {
			Workspace string `json:"workspace"`
			Theme     string `json:"theme"`
			Lang      string `json:"lang"`
			Path      string `json:"path"`
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
		if body.Path != "" {
			redirectPath = body.Path
		} else if foundWS.ChatChannel != "" {
			redirectPath = "/chat/" + foundWS.ChatChannel
		} else {
			nameOrID := foundWS.Name
			if nameOrID == "" {
				nameOrID = foundWS.ID
			}
			projName := getCCProjectName(nameOrID, "claudecode")
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
	mux.HandleFunc("/api/git/discard", gitHandler.Discard)   // POST ?file=<path>
	mux.HandleFunc("/api/git/ai-commit", gitHandler.AICommit) // POST
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
	mux.HandleFunc("/api/terminal/rename", termHandler.Rename) // POST {windowName, name}
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

	// ── Bridge WebSocket proxy ──────────────────────────────────────────────
	// Proxies /bridge/ws to the CC-Connect bridge server (dynamic port).
	mux.Handle("/bridge/", gateway.NewBridgeProxy(ccconnect.BridgePort, ccconnect.BridgeToken))

	// ── 1skills reverse proxy ────────────────────────────────────────────────
	var skillsPort int
	if _, portStr, err := net.SplitHostPort(cfg.SkillsAddr); err == nil {
		fmt.Sscanf(portStr, "%d", &skillsPort)
	} else {
		skillsPort = 38085
	}
	mux.Handle("/1skills/", gateway.NewSkillsProxy(skillsPort))

	// ── Module embed scripts (custom elements) ──────────────────────────────
	// Self-contained ESM bundles produced by the submodule embed pipelines
	// (1skills: `yarn build:embed`, cc-connect: `npm run build:embed`).
	// The 1agents frontend loads them as ESM modules to register
	// <skills-panel> and <cc-connect-panel> custom elements, replacing
	// the iframe approach for non-terminal panels.
	//
	// The path inside dist-embed is fixed by the submodule's vite
	// library-mode config. We resolve the file at startup and 404 if the
	// submodule has not been built yet — a friendlier failure mode than
	// the route silently shadowing the static catch-all.
	mux.HandleFunc("/api/embed/skills-embed.js", serveEmbedScript([]string{
		"modules/1skills/dist-embed/skills-embed.js",
		"../modules/1skills/dist-embed/skills-embed.js",
	}))
	mux.HandleFunc("/api/embed/cc-connect-embed.js", serveEmbedScript([]string{
		"modules/cc-connect/web/dist-embed/cc-connect-embed.js",
		"../modules/cc-connect/web/dist-embed/cc-connect-embed.js",
	}))

	// ── 1skills API pass-through routes ──────────────────────────────────────
	// The 1skills frontend is built with VITE_API_BASE=/api, so its JS makes
	// requests to /api/skills, /api/mcp, /api/slash-commands, etc. directly on
	// the gateway host. These routes forward those calls to the Python backend
	// without stripping any prefix (the Python FastAPI handles /api/* natively).
	skillsAPIProxy := gateway.NewSkillsProxy(skillsPort)
	mux.Handle("/api/skills", skillsAPIProxy)
	mux.Handle("/api/skills/", skillsAPIProxy)
	mux.Handle("/api/mcp/", skillsAPIProxy)
	mux.Handle("/api/slash-commands", skillsAPIProxy)
	mux.Handle("/api/slash-commands/", skillsAPIProxy)
	mux.Handle("/api/marketplace/", skillsAPIProxy)
	mux.Handle("/api/scan/", skillsAPIProxy)
	mux.Handle("/api/settings", skillsAPIProxy)
	mux.Handle("/api/settings/", skillsAPIProxy)
	mux.Handle("/api/health", skillsAPIProxy)

	// ── Tunnel API (on-demand multi-port tunnel control) ─────────────────────
	tunnelAuth := func(r *http.Request) bool {
		authHeader := r.Header.Get("Authorization")
		expectedAuth := "Bearer " + ccconnect.ManagementToken
		return authHeader == expectedAuth || r.URL.Query().Get("token") == ccconnect.ManagementToken
	}

	resolvePort := func(r *http.Request) string {
		if p := r.URL.Query().Get("port"); p != "" {
			return p
		}
		return tunnel.PortFrom(cfg.ListenAddr)
	}

	resolveTimeout := func(r *http.Request) int {
		t := r.URL.Query().Get("timeout")
		if t == "" {
			return 0
		}
		var mins int
		fmt.Sscanf(t, "%d", &mins)
		return mins
	}

	mux.HandleFunc("/api/tunnel/start", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := resolvePort(r)
		timeout := resolveTimeout(r)
		publicURL, token, err := tunnel.DefaultSupervisor.Start(port, timeout)
		if err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"port":  port,
			"url":   publicURL,
			"token": token,
			"link":  fmt.Sprintf("%s/?token=%s", publicURL, token),
		})
	})

	mux.HandleFunc("/api/tunnel/stop", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		port := r.URL.Query().Get("port")
		if port == "" {
			http.Error(w, "port parameter is required to stop a specific tunnel", http.StatusBadRequest)
			return
		}

		if err := tunnel.DefaultSupervisor.Stop(port); err != nil {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "stopped", "port": port})
	})

	mux.HandleFunc("/api/tunnel/stop-all", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !tunnelAuth(r) {
			http.Error(w, "unauthorized control command", http.StatusUnauthorized)
			return
		}

		stopped := tunnel.DefaultSupervisor.StopAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":        "all_stopped",
			"stopped_ports": stopped,
		})
	})

	mux.HandleFunc("/api/tunnel/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tunnels := tunnel.DefaultSupervisor.ListAll()

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"active":  len(tunnels) > 0,
			"tunnels": tunnels,
		})
	})

	// ── System management API (version check + OTA update) ──────────────────
	sysHandler := system.NewHandler()
	mux.HandleFunc("/api/system/version", sysHandler.Version)             // GET  — current & latest version, has_update flag
	mux.HandleFunc("/api/system/update", sysHandler.Update)               // POST — trigger OTA update (non-blocking, returns 202)
	mux.HandleFunc("/api/system/update/status", sysHandler.UpdateStatus)  // GET  — real-time update progress log

	// ── Access Token API ─────────────────────────────────────────────────────
	mux.HandleFunc("/api/access/status", handleAccessStatus)
	mux.HandleFunc("/api/access/generate", handleAccessGenerate)
	mux.HandleFunc("/api/access/verify", handleAccessVerify)
	mux.HandleFunc("/api/access/revoke", handleAccessRevoke)

	// ── Proxy API ────────────────────────────────────────────────────────────
	mux.HandleFunc("/api/proxy", handleProxy)

	// ── Static frontend assets ───────────────────────────────────────────────
	// This catch-all must be registered last so it does not shadow the routes
	// above. html/dist must contain an index.html for SPA-style navigation.
	staticFS := http.FileServer(http.Dir(cfg.StaticDir))
	mux.Handle("/", staticFS)

	return authMiddleware(mux, cfg)
}

// authMiddleware enforces authentication in two layers:
//
//  1. Tunnel auth — when any Cloudflare tunnel is active, the ephemeral session
//     token is required.
//  2. Access token auth — when the user has generated a persistent access token
//     file, all non-localhost requests must present it. Localhost always bypasses.
func authMiddleware(next http.Handler, cfg *config.Config) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ── Layer 1: Tunnel session auth ────────────────────────────────────
		if tunnel.DefaultSupervisor.HasAnyActive() {
			// Bypass tunnel auth for tunnel control APIs
			if !strings.HasPrefix(r.URL.Path, "/api/tunnel/") {
				authenticated := false
				var matchedToken string

				checkToken := func(tok string) bool {
					if tok != "" && tunnel.DefaultSupervisor.ValidateToken(tok) {
						matchedToken = tok
						return true
					}
					return false
				}

				if tokenParam := r.URL.Query().Get("token"); tokenParam != "" {
					if checkToken(tokenParam) {
						authenticated = true
						http.SetCookie(w, &http.Cookie{
							Name:     "ra_session_token",
							Value:    matchedToken,
							Path:     "/",
							HttpOnly: true,
							Secure:   true,
							SameSite: http.SameSiteLaxMode,
						})
					}
				}

				if !authenticated {
					authHeader := r.Header.Get("Authorization")
					if strings.HasPrefix(authHeader, "Bearer ") {
						if checkToken(strings.TrimPrefix(authHeader, "Bearer ")) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					if cookie, err := r.Cookie("ra_session_token"); err == nil {
						if checkToken(cookie.Value) {
							authenticated = true
						}
					}
				}

				if !authenticated {
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(`{"error": "Unauthorized: Ephemeral session token required. Please scan the authorized QR code or click the secure link."}`))
					return
				}
			}
		}

		// ── Layer 2: Access token auth ─────────────────────────────────────
		if !auth.TokenExists() {
			next.ServeHTTP(w, r)
			return
		}

		// Access token API endpoints manage their own auth
		if strings.HasPrefix(r.URL.Path, "/api/access/") {
			next.ServeHTTP(w, r)
			return
		}

		// Localhost always bypasses
		if isLocalhost(r) {
			next.ServeHTTP(w, r)
			return
		}

		storedToken, _ := auth.LoadToken()
		if storedToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		accessAuthenticated := false

		// Mechanism A: ?access_token= query param
		if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
			accessAuthenticated = true
		}

		// Mechanism B: Authorization: Bearer <token> (also checks access token)
		if !accessAuthenticated {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					accessAuthenticated = true
				}
			}
		}

		// Mechanism C: ra_access_token cookie
		if !accessAuthenticated {
			if cookie, err := r.Cookie("ra_access_token"); err == nil {
				if cookie.Value == storedToken {
					accessAuthenticated = true
				}
			}
		}

		if accessAuthenticated {
			// Refresh long-lived cookie
			http.SetCookie(w, &http.Cookie{
				Name:     "ra_access_token",
				Value:    storedToken,
				Path:     "/",
				HttpOnly: true,
				Secure:   r.TLS != nil,
				SameSite: http.SameSiteLaxMode,
				MaxAge:   365 * 24 * 3600,
			})
			next.ServeHTTP(w, r)
			return
		}

		// Not authenticated — reject API calls, let page requests through
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"access_token_required","message":"An access token is required for non-localhost access."}`))
			return
		}

		// Page request: let SPA load; it will call /api/access/status and show gate
		next.ServeHTTP(w, r)
	})
}

func isLocalhost(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host == "127.0.0.1" || host == "::1"
}

// ── Access Token Handlers ───────────────────────────────────────────────────────

func handleAccessStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	required := auth.TokenExists()
	authenticated := true

	if required && !isLocalhost(r) {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			authenticated = false

			if t := r.URL.Query().Get("access_token"); t != "" && t == storedToken {
				authenticated = true
			}
			if !authenticated {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					if strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
						authenticated = true
					}
				}
			}
			if !authenticated {
				if cookie, err := r.Cookie("ra_access_token"); err == nil {
					if cookie.Value == storedToken {
						authenticated = true
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]bool{
		"required":      required,
		"authenticated": authenticated,
	})
}

func handleAccessGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !isLocalhost(r) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token generation is only allowed from localhost."})
		return
	}

	token := tunnel.GenerateRandomToken()
	if err := auth.SaveToken(token); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]string{
		"token":   token,
		"message": "Access token generated. Save it now — it will not be shown again.",
	})
}

func handleAccessVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Invalid request body."})
		return
	}

	if body.Token == "" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "Token is required."})
		return
	}

	storedToken, err := auth.LoadToken()
	if err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": err.Error()})
		return
	}

	if body.Token != storedToken {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": false, "error": "无效的访问令牌。"})
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "ra_access_token",
		Value:    storedToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 3600,
	})

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func handleAccessRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Allow localhost or already-authenticated callers
	allowed := isLocalhost(r)
	if !allowed {
		storedToken, _ := auth.LoadToken()
		if storedToken != "" {
			if cookie, err := r.Cookie("ra_access_token"); err == nil && cookie.Value == storedToken {
				allowed = true
			}
			if !allowed {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == storedToken {
					allowed = true
				}
			}
		}
	}

	if !allowed {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]string{"error": "Token revocation requires localhost or authenticated access."})
		return
	}

	if err := auth.DeleteToken(); err != nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "message": "Access token revoked."})
}

// handleProxy acts as a reverse proxy that fetches external websites, strips
// X-Frame-Options & Content-Security-Policy headers, and injects `<base>` and link-rewriting scripts.
func handleProxy(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		http.Error(w, "Missing url parameter", http.StatusBadRequest)
		return
	}

	req, err := http.NewRequestWithContext(r.Context(), "GET", targetURL, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Forward standard headers
	req.Header.Set("User-Agent", r.Header.Get("User-Agent"))
	req.Header.Set("Accept", r.Header.Get("Accept"))
	req.Header.Set("Accept-Language", r.Header.Get("Accept-Language"))

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	// If it's HTML, inject our base href and click interceptor scripts!
	if strings.Contains(strings.ToLower(contentType), "text/html") {
		htmlStr := string(bodyBytes)
		
		// 1. Inject <base href="..."> right after the opening <head> tag
		headIdx := strings.Index(strings.ToLower(htmlStr), "<head>")
		if headIdx != -1 {
			insertPos := headIdx + len("<head>")
			
			// Inject `<base>` tag and click interceptor script
			actualURL := resp.Request.URL.String()
			baseTag := `<base href="` + actualURL + `">`
			scriptTag := `
<script>
(function() {
  function getOriginalUrl(url) {
    try {
      var urlObj = new URL(url || window.location.href);
      if (urlObj.pathname === '/api/proxy') {
        var target = urlObj.searchParams.get('url');
        if (target) return target;
      }
      return urlObj.href;
    } catch(e) {
      return url || window.location.href;
    }
  }

  function notifyParent(url) {
    try {
      var orig = getOriginalUrl(url);
      window.parent.postMessage({ type: 'iframe_navigate', url: orig }, '*');
    } catch(e) {}
  }

  // Notify parent of initial load
  notifyParent();

  // Notify parent on history popstate (e.g. back/forward)
  window.addEventListener('popstate', function() {
    notifyParent();
  });

  // Prevent links from redirecting the frame to non-proxied addresses
  document.addEventListener('click', function(e) {
    var target = e.target.closest('a');
    if (target && target.href) {
      e.preventDefault();
      // Route the absolute URL back through our proxy!
      window.location.href = window.location.origin + '/api/proxy?url=' + encodeURIComponent(target.href);
    }
  }, true);

  // Prevent form actions from escaping the proxy
  document.addEventListener('submit', function(e) {
    var target = e.target;
    if (target && target.action) {
      if (target.method.toLowerCase() === 'get') {
        e.preventDefault();
        try {
          var url = new URL(target.action);
          var formData = new FormData(target);
          for (var pair of formData.entries()) {
            url.searchParams.set(pair[0], pair[1]);
          }
          window.location.href = window.location.origin + '/api/proxy?url=' + encodeURIComponent(url.href);
        } catch(err) {
          // Fallback if URL parsing fails
        }
      }
    }
  }, true);

  // Rewrite History API state changes to same-origin to prevent SecurityError
  if (window.history) {
    var originalPushState = window.history.pushState;
    window.history.pushState = function(state, title, url) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          originalPushState.apply(window.history, [state, title, proxiedUrl]);
          notifyParent(resolvedUrl);
        } else {
          originalPushState.apply(window.history, arguments);
          notifyParent();
        }
      } catch (e) {
        console.warn('Blocked pushState rewrite:', e);
      }
    };

    var originalReplaceState = window.history.replaceState;
    window.history.replaceState = function(state, title, url) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          originalReplaceState.apply(window.history, [state, title, proxiedUrl]);
          notifyParent(resolvedUrl);
        } else {
          originalReplaceState.apply(window.history, arguments);
          notifyParent();
        }
      } catch (e) {
        console.warn('Blocked replaceState rewrite:', e);
      }
    };
  }

  // Intercept window.fetch to route relative/external data requests through proxy
  if (window.fetch) {
    var originalFetch = window.fetch;
    window.fetch = function(input, init) {
      try {
        var url;
        if (typeof input === 'string') {
          url = input;
        } else if (input instanceof URL) {
          url = input.href;
        } else if (input && input.url) {
          url = input.url;
        }

        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxyHost = window.location.host;
          var resolvedObj = new URL(resolvedUrl);
          if (resolvedObj.host !== proxyHost) {
            var proxiedUrl = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
            if (typeof input === 'string') {
              input = proxiedUrl;
            } else if (input instanceof URL) {
              input = new URL(proxiedUrl);
            } else if (input instanceof Request) {
              input = new Request(proxiedUrl, input);
            } else if (input && input.url) {
              input = new Request(proxiedUrl, input);
            }
          }
        }
      } catch (e) {
        console.warn('Blocked fetch rewrite:', e);
      }
      return originalFetch.apply(this, arguments);
    };
  }

  // Intercept XMLHttpRequest to route relative/external data requests through proxy
  if (window.XMLHttpRequest) {
    var originalOpen = window.XMLHttpRequest.prototype.open;
    window.XMLHttpRequest.prototype.open = function(method, url, async, user, password) {
      try {
        if (url) {
          var resolvedUrl = new URL(url, document.baseURI).href;
          var proxyHost = window.location.host;
          var resolvedObj = new URL(resolvedUrl);
          if (resolvedObj.host !== proxyHost) {
            arguments[1] = window.location.origin + '/api/proxy?url=' + encodeURIComponent(resolvedUrl);
          }
        }
      } catch (e) {
        console.warn('Blocked XHR rewrite:', e);
      }
      return originalOpen.apply(this, arguments);
    };
  }
})();
</script>
`
			htmlStr = htmlStr[:insertPos] + baseTag + scriptTag + htmlStr[insertPos:]
			bodyBytes = []byte(htmlStr)
		}
	}

	// Copy headers, stripping security controls
	for k, v := range resp.Header {
		lowerK := strings.ToLower(k)
		if lowerK == "x-frame-options" || lowerK == "content-security-policy" || lowerK == "csp" {
			continue
		}
		for _, val := range v {
			w.Header().Add(k, val)
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(bodyBytes)
}

// serveEmbedScript returns an http.HandlerFunc that serves a single
// submodule embed bundle. The handler resolves the file lazily on each
// request, so a submodule that is built *after* 1agents has started
// becomes available without a restart. The first existing path wins.
//
// If none of the candidates exist the handler returns 404 with a hint
// telling the operator how to produce the bundle. This is intentional:
// silently shadowing the static catch-all would make "iframe doesn't
// load" look like "module registration failed", which is much harder
// to diagnose.
func serveEmbedScript(candidates []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Only allow GET — these are static assets; anything else is a bug.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		for _, c := range candidates {
			abs, err := filepath.Abs(c)
			if err != nil {
				continue
			}
			if info, err := os.Stat(abs); err == nil && !info.IsDir() {
				w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
				w.Header().Set("Cache-Control", "no-cache")
				http.ServeFile(w, r, abs)
				return
			}
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w,
			"embed bundle not found; tried: %s\nbuild it with `yarn build:embed` (1skills) or `npm run build:embed` (cc-connect) inside the submodule",
			strings.Join(candidates, ", "),
		)
	}
}

func getCCProjectName(workspaceName string, agentType string) string {
	var sb strings.Builder
	inInvalidSeq := false
	for _, r := range workspaceName {
		isValid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
		if isValid {
			sb.WriteRune(r)
			inInvalidSeq = false
		} else {
			if !inInvalidSeq {
				sb.WriteRune('_')
				inInvalidSeq = true
			}
		}
	}
	slug := sb.String()
	if len(slug) > 32 {
		slug = slug[:32]
	}
	if slug == "" {
		slug = "ws"
	}
	return fmt.Sprintf("%s__%s", slug, agentType)
}
