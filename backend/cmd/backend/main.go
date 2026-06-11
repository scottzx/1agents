package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/scottzx/1Agents/backend/internal/cert"
	"github.com/scottzx/1Agents/backend/internal/ccconnect"
	"github.com/scottzx/1Agents/backend/internal/config"
	"github.com/scottzx/1Agents/backend/internal/server"
	"github.com/scottzx/1Agents/backend/internal/supervisor"
	"github.com/scottzx/1Agents/backend/internal/tunnel"
)

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func get1AgentsHome() string {
	if val := os.Getenv("ONEAGENTS_HOME"); val != "" {
		return val
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return home
}

func main() {
	cfg := config.Default()

	// ── CLI flags ─────────────────────────────────────────────────────────────
	var noTtyd bool
	flag.BoolVar(&noTtyd, "no-ttyd", false,
		"Skip launching ttyd (useful in dev when ttyd is already running separately)")
	flag.StringVar(&cfg.ListenAddr, "listen", cfg.ListenAddr,
		"External listen address (e.g. :8080 or 0.0.0.0:8080)")
	flag.StringVar(&cfg.TtydAddr, "ttyd-addr", cfg.TtydAddr,
		"Internal ttyd listen address (must stay on 127.0.0.1)")
	flag.StringVar(&cfg.TtydBinaryPath, "ttyd-bin", cfg.TtydBinaryPath,
		"Path to the ttyd executable")
	flag.StringVar(&cfg.SkillsAddr, "skills-addr", cfg.SkillsAddr,
		"Internal 1skills listen address (must stay on 127.0.0.1)")
	flag.StringVar(&cfg.SkillsBinaryPath, "skills-bin", cfg.SkillsBinaryPath,
		"Path to the python executable to run 1skills")
	flag.StringVar(&cfg.WorkDir, "workdir", cfg.WorkDir,
		"Root directory exposed by the file-system API")
	flag.StringVar(&cfg.StaticDir, "static", cfg.StaticDir,
		"Directory containing compiled frontend assets (html/dist)")
	flag.DurationVar(&cfg.RestartDelay, "restart-delay", cfg.RestartDelay,
		"How long to wait before restarting ttyd after an unexpected exit")
	flag.StringVar(&cfg.TmuxSession, "tmux-session", cfg.TmuxSession,
		"tmux session name for terminal persistence")
	flag.IntVar(&cfg.MaxRestarts, "max-restarts", cfg.MaxRestarts,
		"Maximum number of consecutive ttyd restarts before giving up")
	var sslCert, sslKey string
	var enableSSL bool
	flag.BoolVar(&enableSSL, "ssl", false, "Enable HTTPS/SSL with auto-generated certificates if none exist")
	flag.StringVar(&sslCert, "ssl-cert", "", "Path to the SSL certificate for HTTPS")
	flag.StringVar(&sslKey, "ssl-key", "", "Path to the SSL private key for HTTPS")
	flag.BoolVar(&cfg.EnableTunnel, "tunnel", false, "Enable on-demand public Web Tunnel via Cloudflare on startup")
	var tunnelIdleTimeout int
	flag.IntVar(&tunnelIdleTimeout, "tunnel-idle-timeout", 15, "Auto-stop tunnel after N minutes of inactivity (0 to disable)")

	var isDesktop bool
	var resourcesDir string
	flag.BoolVar(&isDesktop, "desktop", false, "Indicates if the daemon is running in desktop mode")
	flag.StringVar(&resourcesDir, "resources-dir", "", "Path to the Tauri resources directory")

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")

	flag.Parse()

	// If a custom tmux session is specified via flags and TtydArgs is the default tmux command,
	// keep TtydArgs in sync with the new session name.
	if cfg.TmuxSession != "" && runtime.GOOS != "windows" {
		if len(cfg.TtydArgs) >= 5 && cfg.TtydArgs[0] == "tmux" && cfg.TtydArgs[1] == "new-session" && cfg.TtydArgs[2] == "-A" && cfg.TtydArgs[3] == "-s" && cfg.TtydArgs[4] == "1agents" {
			cfg.TtydArgs[4] = cfg.TmuxSession
		}
	}

	if showVersion {
		fmt.Printf("1agents %s\ncommit:  %s\nbuilt:   %s\n", version, commit, buildTime)
		return
	}

	// ── check if daemon is already running ─────────────────────────────────────
	// Subcommands like 'tunnel' should not prevent execution since they are clients.
	isSubcommand := flag.NArg() > 0 && flag.Arg(0) == "tunnel"
	if !isSubcommand {
		if activeAddr, activePid, isRunning := checkDaemonRunning(); isRunning {
			log.Printf("[main] 1Agents daemon is already running at http://%s (PID %d).", activeAddr, activePid)
			log.Printf("[main] Starting Gateway Reverse Proxy on %s -> http://%s...", cfg.ListenAddr, activeAddr)
			startReverseProxy(cfg.ListenAddr, activeAddr)
			return
		}
	}

	if isDesktop {
		if resourcesDir == "" {
			log.Fatalf("[main] FATAL: -desktop mode requires -resources-dir to be set")
		}
		// Resolve ttyd binary path inside resources/bin/ttyd
		cfg.TtydBinaryPath = filepath.Join(resourcesDir, "resources", "bin", "ttyd")
		// Resolve static files dir inside resources/dist
		cfg.StaticDir = filepath.Join(resourcesDir, "resources", "dist")

		// Retrieve the login shell path to inherit host environment variables (like brew, git, etc.)
		userPath := getLoginShellPath()
		bundledBin := filepath.Join(resourcesDir, "resources", "bundled_tools", "bin")
		bundledNode := filepath.Join(resourcesDir, "resources", "runtime", "node", "bin")
		bundledStdBin := filepath.Join(resourcesDir, "resources", "bin")

		var paths []string
		if runtime.GOOS == "windows" {
			bundledBinRoot := filepath.Join(resourcesDir, "resources", "bundled_tools")
			paths = []string{bundledBin, bundledBinRoot, bundledNode, bundledStdBin, userPath}
		} else {
			paths = []string{bundledBin, bundledNode, bundledStdBin, userPath}
		}
		newPath := strings.Join(paths, string(os.PathListSeparator))
		os.Setenv("PATH", newPath)
		log.Printf("[main] Desktop Mode Enabled.")
		log.Printf("[main] Set ttyd path to: %s", cfg.TtydBinaryPath)
		log.Printf("[main] Set static dir to: %s", cfg.StaticDir)
		log.Printf("[main] Set PATH to: %s", newPath)
	}

	// Configure tunnel idle timeout (applies to both --tunnel and API-started tunnels)
	if tunnelIdleTimeout > 0 {
		tunnel.DefaultSupervisor.SetIdleTimeout(time.Duration(tunnelIdleTimeout) * time.Minute)
	}

	// ── tunnel subcommand (CLI client mode: talks to a running daemon) ─────────
	if flag.NArg() > 0 && flag.Arg(0) == "tunnel" {
		cmd := ""
		port := ""
		timeout := ""
		if flag.NArg() >= 2 {
			cmd = flag.Arg(1)
		}
		if flag.NArg() >= 3 {
			port = flag.Arg(2)
		}
		if flag.NArg() >= 4 {
			timeout = flag.Arg(3)
		}
		handleTunnelCommand(cmd, port, timeout)
		return
	}

	// Remaining positional arguments are passed verbatim to ttyd.
	if flag.NArg() > 0 {
		cfg.TtydArgs = flag.Args()
	}

	// ── Graceful-shutdown context ─────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── 1. Optionally start ttyd supervisor ───────────────────────────────────
	if !noTtyd {
		host, portStr, err := net.SplitHostPort(cfg.TtydAddr)
		if err != nil {
			host = "127.0.0.1"
			portStr = "7681"
		}
		var basePort int
		fmt.Sscanf(portStr, "%d", &basePort)

		freePort, err := findAvailablePort(host, basePort)
		if err != nil {
			log.Printf("[main] WARNING: Failed to find free port starting from %d: %v. Using default.", basePort, err)
		} else if freePort != basePort {
			log.Printf("[main] Port %d is busy. Automatically selected free port %d for internal ttyd.", basePort, freePort)
			cfg.TtydAddr = net.JoinHostPort(host, fmt.Sprintf("%d", freePort))
		}
	}

	sup := supervisor.New(cfg)
	if noTtyd {
		log.Println("[main] --no-ttyd: skipping ttyd launch (dev mode, ttyd runs separately)")
	} else {
		sup.Start(ctx)
		log.Printf("[main] Waiting for ttyd to start on %s ...", cfg.TtydAddr)
		time.Sleep(600 * time.Millisecond)
	}

	// ── 1.2. Start skills supervisor ─────────────────────────────────────────
	skillsHost, skillsPortStr, err := net.SplitHostPort(cfg.SkillsAddr)
	if err != nil {
		skillsHost = "127.0.0.1"
		skillsPortStr = "38085"
	}
	var skillsBasePort int
	fmt.Sscanf(skillsPortStr, "%d", &skillsBasePort)
	skillsFreePort, err := findAvailablePort(skillsHost, skillsBasePort)
	if err != nil {
		log.Printf("[main] WARNING: Failed to find free port starting from %d for 1skills: %v. Using default.", skillsBasePort, err)
	} else if skillsFreePort != skillsBasePort {
		log.Printf("[main] Port %d is busy. Automatically selected free port %d for internal 1skills.", skillsBasePort, skillsFreePort)
		cfg.SkillsAddr = net.JoinHostPort(skillsHost, fmt.Sprintf("%d", skillsFreePort))
	}

	skillsSup := supervisor.NewSkills(cfg)
	skillsSup.Start(ctx)

	acpxSup := supervisor.NewAcpx(cfg)
	acpxSup.Start(ctx)

	// ── 2. Start cc-connect Supervisor & engines ──────────────────────────────

	ccconnect.Start(ctx, isDesktop)

	// ── 3. Start HTTP gateway ─────────────────────────────────────────────────
	router := server.NewRouter(cfg)
	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (required for long-lived WebSocket streams)
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[main] 1Agent listening on %s", cfg.ListenAddr)
		writeDaemonFile(cfg.ListenAddr)
		log.Printf("[main] Version            : %s", version)
		log.Printf("[main] Commit             : %s", commit)
		log.Printf("[main] Build Time         : %s", buildTime)
		log.Printf("[main] Working directory  : %s", cfg.WorkDir)
		log.Printf("[main] Dev mode (no-ttyd) : %v", noTtyd)

		// Print a beautiful unified port list for the user
		fmt.Println("\n==================================================================")
		fmt.Println("🚀 1AGENTS DEPLOYMENT STATUS:")
		fmt.Printf("   🌐 HTTP Gateway (Listen)  : %s\n", cfg.ListenAddr)
		if !noTtyd {
			fmt.Printf("   📺 Internal Web Terminal  : %s\n", cfg.TtydAddr)
		}
		fmt.Printf("   🔌 CC-Connect Bridge Port : :%d (Dynamic)\n", ccconnect.BridgePort)
		fmt.Printf("   ⚙️  CC-Connect Mgmt Port   : :%d (Dynamic)\n", ccconnect.ManagementPort)
		fmt.Printf("   🛠️  1skills Microservice   : %s\n", cfg.SkillsAddr)
		fmt.Println("==================================================================")
		
		var err error
		if enableSSL {
			var tsDomain string
			var tsIPs []net.IP

			// Try to query Tailscale details
			if domain, ips, err := cert.GetTailscaleInfo(); err == nil {
				tsDomain = domain
				tsIPs = ips
				log.Printf("[main] Tailscale network detected: domain=%s, ips=%v", tsDomain, tsIPs)
			} else {
				log.Printf("[main] Tailscale network not detected or tailscale CLI not available (%v)", err)
			}

			// Try to auto-discover official Tailscale certs first
			if sslCert == "" && sslKey == "" {
				if c, k, found := cert.DiscoverTailscaleCerts(tsDomain); found {
					sslCert = c
					sslKey = k
					log.Printf("[main] Discovered official Tailscale certificate files. Using: %s", sslCert)
				}
			}

			// Fallback to default user home directory paths for self-signed certs
			if sslCert == "" || sslKey == "" {
				home := get1AgentsHome()
				defaultCertDir := filepath.Join(home, ".1agents", "certs")
				if sslCert == "" {
					sslCert = filepath.Join(defaultCertDir, "cert.pem")
				}
				if sslKey == "" {
					sslKey = filepath.Join(defaultCertDir, "key.pem")
				}
			}

			// Generate if not present
			if _, err := os.Stat(sslCert); os.IsNotExist(err) {
				log.Printf("[main] SSL certificate files not found. Generating secure self-signed cert on-the-fly...")
				if err := cert.GenerateSelfSignedCert(sslCert, sslKey, tsDomain, tsIPs); err != nil {
					log.Fatalf("[main] FATAL: failed to auto-generate certificate: %v", err)
				}
				log.Printf("[main] Successfully generated TLS certificate at %s", sslCert)
			} else {
				log.Printf("[main] Using active SSL certificate: %s", sslCert)
			}
		}

		if cfg.EnableTunnel {
			go func() {
				time.Sleep(500 * time.Millisecond) // Let the server bind to the port first
				log.Println("[main] --tunnel flag passed on boot. Initializing secure public Web Tunnel...")
				port := tunnel.PortFrom(cfg.ListenAddr)
				
				publicURL, token, err := tunnel.DefaultSupervisor.Start(port, 0)
				if err != nil {
					log.Printf("[main] ERROR: Failed to start public Web Tunnel: %v", err)
					return
				}

				fmt.Println("\n==================================================================")
				fmt.Println("🚀 1AGENT PUBLIC TUNNEL IS ACTIVE!")
				fmt.Printf("🔗 Secure Link: %s/?token=%s\n", publicURL, token)
				fmt.Println("==================================================================")
				fmt.Println("[main] Scan the high-contrast QR code below to connect instantly:")
				
				tunnel.RenderTerminalQR(fmt.Sprintf("%s/?token=%s", publicURL, token))
			}()
		}

		if sslCert != "" && sslKey != "" {
			log.Printf("[main] HTTPS / SSL enabled (using cert: %s)", sslCert)
			err = httpServer.ListenAndServeTLS(sslCert, sslKey)
		} else {
			err = httpServer.ListenAndServe()
		}
		
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] HTTP server fatal error: %v", err)
		}
	}()

	// ── 3. Wait for OS shutdown signal ───────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[main] Received signal %s, shutting down gracefully...", sig)

	// Stop public tunnel if active
	_ = tunnel.DefaultSupervisor.StopAll()

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[main] HTTP shutdown error: %v", err)
	}

	<-sup.Done()
	<-skillsSup.Done()
	<-acpxSup.Done()
	log.Println("[main] Shutdown complete. Goodbye.")
}


// writeDaemonFile writes the daemon's listen address to a well-known location
// so CLI subcommands (tunnel, etc.) can discover the port without flags.
func writeDaemonFile(listenAddr string) {
	home := get1AgentsHome()
	daemonDir := filepath.Join(home, ".1agents")
	os.MkdirAll(daemonDir, 0700)

	info := struct {
		ListenAddr string `json:"listen_addr"`
		PID        int    `json:"pid"`
	}{
		ListenAddr: listenAddr,
		PID:        os.Getpid(),
	}
	data, _ := json.MarshalIndent(info, "", "  ")
	os.WriteFile(filepath.Join(daemonDir, "daemon.json"), data, 0644)
}

// findAvailablePort finds the first free TCP port starting from basePort.
func findAvailablePort(ip string, basePort int) (int, error) {
	for port := basePort; port < basePort+100; port++ {
		addr := fmt.Sprintf("%s:%d", ip, port)
		l, err := net.Listen("tcp", addr)
		if err == nil {
			l.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available port found in range %d-%d", basePort, basePort+100)
}

// getLoginShellPath runs the user's default login shell to read their full PATH environment variable.
func getLoginShellPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("PATH")
	}
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// -l loads login environment, -c executes the echo command
	cmd := exec.CommandContext(ctx, shell, "-l", "-c", "echo $PATH")
	output, err := cmd.Output()
	if err != nil {
		log.Printf("[main] Failed to get login shell PATH: %v. Using basic PATH.", err)
		return os.Getenv("PATH")
	}
	return strings.TrimSpace(string(output))
}

// checkDaemonRunning reads ~/.1agents/daemon.json and checks if the daemon is active
func checkDaemonRunning() (string, int, bool) {
	home := get1AgentsHome()
	data, err := os.ReadFile(filepath.Join(home, ".1agents", "daemon.json"))
	if err != nil {
		return "", 0, false
	}
	var info struct {
		ListenAddr string `json:"listen_addr"`
		PID        int    `json:"pid"`
	}
	if err := json.Unmarshal(data, &info); err != nil || info.ListenAddr == "" {
		return "", 0, false
	}

	// Verify if the daemon is actively listening on the port
	addr := info.ListenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	} else if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = strings.Replace(addr, "0.0.0.0:", "127.0.0.1:", 1)
	}

	conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return info.ListenAddr, info.PID, true
	}
	return "", 0, false
}

// startReverseProxy sets up a lightweight HTTP and WebSocket forwarding server
func startReverseProxy(listenAddr, targetAddr string) {
	if strings.HasPrefix(targetAddr, ":") {
		targetAddr = "127.0.0.1" + targetAddr
	} else if strings.HasPrefix(targetAddr, "0.0.0.0:") {
		targetAddr = strings.Replace(targetAddr, "0.0.0.0:", "127.0.0.1:", 1)
	}

	targetURL, err := url.Parse("http://" + targetAddr)
	if err != nil {
		log.Fatalf("[proxy] FATAL: failed to parse target URL: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Customize Director to handle Host headers correctly for routing
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Host = targetURL.Host
	}

	server := &http.Server{
		Addr:    listenAddr,
		Handler: proxy,
	}

	log.Printf("[proxy] Reverse proxy listening on %s", listenAddr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[proxy] FATAL: reverse proxy failed: %v", err)
	}
}
