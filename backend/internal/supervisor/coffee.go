package supervisor

import (
	"context"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
)

// CoffeeSupervisor manages the loopback-only Node.js payment module.
type CoffeeSupervisor struct {
	cfg *config.Config

	mu  sync.Mutex
	cmd *exec.Cmd
}

func NewCoffee(cfg *config.Config) *CoffeeSupervisor {
	return &CoffeeSupervisor{cfg: cfg}
}

func (s *CoffeeSupervisor) Start(ctx context.Context) {
	sourceDir, ok := s.resolveSourceDir()
	if !ok {
		log.Printf("[coffee-sup] payment module not found; embedded coffee page will be unavailable")
		return
	}

	go s.supervise(ctx, sourceDir)
}

func (s *CoffeeSupervisor) supervise(ctx context.Context, sourceDir string) {
	restarts := 0
	for {
		if ctx.Err() != nil {
			s.stop()
			return
		}
		if restarts >= s.cfg.MaxRestarts {
			log.Printf("[coffee-sup] payment service restarted %d times; giving up", restarts)
			return
		}

		err := s.run(ctx, sourceDir)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("[coffee-sup] payment service exited: %v", err)
		} else {
			log.Printf("[coffee-sup] payment service exited cleanly")
		}
		restarts++

		select {
		case <-ctx.Done():
			return
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

func (s *CoffeeSupervisor) run(ctx context.Context, sourceDir string) error {
	host, port, err := net.SplitHostPort(s.cfg.CoffeeAddr)
	if err != nil {
		return err
	}

	nodeBinary := s.cfg.CoffeeNodeBinaryPath
	if nodeBinary == "" {
		nodeBinary = "node"
	}
	cmd := exec.CommandContext(ctx, nodeBinary, "src/server.js")
	cmd.Dir = sourceDir
	cmd.Env = coffeeEnvironment(host, port)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[coffee-sup] starting %s/src/server.js on %s", sourceDir, s.cfg.CoffeeAddr)
	err = cmd.Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *CoffeeSupervisor) stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}

func (s *CoffeeSupervisor) resolveSourceDir() (string, bool) {
	var candidates []string
	if s.cfg.CoffeeSourceDir != "" {
		candidates = append(candidates, s.cfg.CoffeeSourceDir)
	}
	if envDir := os.Getenv("ALIPAY_COFFEE_DIR"); envDir != "" {
		candidates = append(candidates, envDir)
	}
	candidates = append(candidates,
		"modules/alipay-coffee",
		"../modules/alipay-coffee",
	)
	if executable, err := os.Executable(); err == nil {
		binDir := filepath.Dir(executable)
		candidates = append(candidates,
			filepath.Join(binDir, "alipay-coffee"),
			filepath.Join(binDir, "..", "alipay-coffee"),
			filepath.Join(binDir, "..", "resources", "alipay-coffee"),
		)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absolute = filepath.Clean(absolute)
		if _, exists := seen[absolute]; exists {
			continue
		}
		seen[absolute] = struct{}{}
		if coffeeSourceReady(absolute) {
			return absolute, true
		}
	}
	return "", false
}

func coffeeSourceReady(dir string) bool {
	required := []string{
		filepath.Join(dir, "package.json"),
		filepath.Join(dir, "src", "server.js"),
		filepath.Join(dir, "node_modules", "express", "package.json"),
		filepath.Join(dir, "node_modules", "alipay-sdk", "package.json"),
	}
	for _, file := range required {
		info, err := os.Stat(file)
		if err != nil || !info.Mode().IsRegular() {
			return false
		}
	}
	return true
}

func coffeeEnvironment(host, port string) []string {
	overrides := map[string]string{
		"ALIPAY_COFFEE_HOST": host,
		"ALIPAY_COFFEE_PORT": port,
	}
	if os.Getenv("ALIPAY_COFFEE_PROJECT_ROOT") == "" {
		if currentDir, err := os.Getwd(); err == nil {
			overrides["ALIPAY_COFFEE_PROJECT_ROOT"] = currentDir
		}
	}

	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		key := entry
		if index := strings.IndexByte(entry, '='); index >= 0 {
			key = entry[:index]
		}
		if _, replaced := overrides[key]; replaced {
			continue
		}
		filtered = append(filtered, entry)
	}
	for key, value := range overrides {
		filtered = append(filtered, key+"="+value)
	}
	return filtered
}
