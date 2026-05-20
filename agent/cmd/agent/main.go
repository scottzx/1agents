package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/scottzx/remote-agents/agent/internal/config"
	"github.com/scottzx/remote-agents/agent/internal/server"
	"github.com/scottzx/remote-agents/agent/internal/supervisor"
)

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
	flag.StringVar(&cfg.WorkDir, "workdir", cfg.WorkDir,
		"Root directory exposed by the file-system API")
	flag.StringVar(&cfg.StaticDir, "static", cfg.StaticDir,
		"Directory containing compiled frontend assets (html/dist)")
	flag.DurationVar(&cfg.RestartDelay, "restart-delay", cfg.RestartDelay,
		"How long to wait before restarting ttyd after an unexpected exit")
	flag.IntVar(&cfg.MaxRestarts, "max-restarts", cfg.MaxRestarts,
		"Maximum number of consecutive ttyd restarts before giving up")

	flag.Parse()

	// Remaining positional arguments are passed verbatim to ttyd.
	if flag.NArg() > 0 {
		cfg.TtydArgs = flag.Args()
	}

	// ── Graceful-shutdown context ─────────────────────────────────────────────
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// ── 1. Optionally start ttyd supervisor ───────────────────────────────────
	sup := supervisor.New(cfg)
	if noTtyd {
		log.Println("[main] --no-ttyd: skipping ttyd launch (dev mode, ttyd runs separately)")
	} else {
		sup.Start(ctx)
		log.Printf("[main] Waiting for ttyd to start on %s ...", cfg.TtydAddr)
		time.Sleep(600 * time.Millisecond)
	}

	// ── 2. Start HTTP gateway ─────────────────────────────────────────────────
	router := server.NewRouter(cfg)
	httpServer := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // 0 = no timeout (required for long-lived WebSocket streams)
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("[main] Remote Agent listening on %s", cfg.ListenAddr)
		log.Printf("[main] Working directory  : %s", cfg.WorkDir)
		log.Printf("[main] Dev mode (no-ttyd) : %v", noTtyd)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("[main] HTTP server fatal error: %v", err)
		}
	}()

	// ── 3. Wait for OS shutdown signal ───────────────────────────────────────
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("[main] Received signal %s, shutting down gracefully...", sig)

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("[main] HTTP shutdown error: %v", err)
	}

	<-sup.Done()
	log.Println("[main] Shutdown complete. Goodbye.")
}
