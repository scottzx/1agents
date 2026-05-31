package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/scottzx/remote-agents/agent/internal/auth"
	"github.com/scottzx/remote-agents/agent/internal/ccconnect"
	"github.com/scottzx/remote-agents/agent/internal/config"
	ctxt "github.com/scottzx/remote-agents/agent/internal/context"
	"github.com/scottzx/remote-agents/agent/internal/fs"
	"github.com/scottzx/remote-agents/agent/internal/gateway"
	"github.com/scottzx/remote-agents/agent/internal/git"
	"github.com/scottzx/remote-agents/agent/internal/system"
	"github.com/scottzx/remote-agents/agent/internal/terminal"
	"github.com/scottzx/remote-agents/agent/internal/tunnel"
	"github.com/scottzx/remote-agents/agent/internal/workspace"
)

// NewRouter builds and returns the main HTTP request multiplexer.
//
// Route hierarchy (evaluated top-to-bottom):
//
//	/api/fs/*         → File system CRUD handlers (Go, local I/O)
//	/api/workspace/*  → Workspace CRUD handlers (Go, JSON file storage)
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
	mux.HandleFunc("/api/fs/read", fsHandler.Read)     // GET  ?path=./main.go
	mux.HandleFunc("/api/fs/view", fsHandler.View)     // GET  ?path=./page.html (serves with correct content-type)
	mux.HandleFunc("/api/fs/view/", fsHandler.View)    // GET  /api/fs/view/relative/path (prefix route for relative assets support)
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
	mux.HandleFunc("/api/git/discard", gitHandler.Discard)   // POST ?file=<path>
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

	// ── Bridge WebSocket proxy ──────────────────────────────────────────────
	// Proxies /bridge/ws to the CC-Connect bridge server (dynamic port).
	mux.Handle("/bridge/", gateway.NewBridgeProxy(ccconnect.BridgePort))

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
