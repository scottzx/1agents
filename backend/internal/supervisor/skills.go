package supervisor

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/scottzx/1Agents/backend/internal/config"
)

// SkillsSupervisor manages the lifecycle of the 1skills python FastAPI server.
//
// Launch priority (first match wins):
//  1. skill-manager binary next to 1agents / in ./bin (optional legacy PyInstaller)
//  2. @1agents/skills npm package source (node_modules/@1agents/skills)
//  3. modules/1skills (dev checkout) or release-layout 1skills/
//  4. Host Python venv under ~/.1agents/1skills/.venv
//
// Host-Python mode prefers `uv` for venv + deps, else python3 -m venv + pip.
// Requires Python >= 3.11 on the host for non-binary launches.
type SkillsSupervisor struct {
	cfg          *config.Config
	cmd          *exec.Cmd
	mu           sync.Mutex
	restartCount int
	done         chan struct{}
}

// NewSkills creates a new SkillsSupervisor with the given configuration.
func NewSkills(cfg *config.Config) *SkillsSupervisor {
	return &SkillsSupervisor{
		cfg:  cfg,
		done: make(chan struct{}),
	}
}

// Start launches the supervision loop in a background goroutine.
func (s *SkillsSupervisor) Start(ctx context.Context) {
	go s.supervisionLoop(ctx)
}

// Done returns a channel closed when the supervisor has fully stopped.
func (s *SkillsSupervisor) Done() <-chan struct{} {
	return s.done
}

// launchMode describes how the supervisor will run 1skills.
type launchMode int

const (
	launchModeBinary launchMode = iota // optional: standalone skill-manager binary
	launchModeVenv                     // host Python venv + skill_manager module
)

// resolveRuntime decides which launch mode to use and returns the executable
// path together with the working directory (skills source tree).
//
// Search order:
//  1. skill-manager binary next to the running 1agents executable
//  2. skill-manager binary in ./bin/ (relative to CWD)
//  3. Python source via -skills-dir / ONEAGENTS_SKILLS_DIR / auto-discovery
//     - prefer in-tree .venv if present
//     - else managed venv at ~/.1agents/1skills/.venv
func (s *SkillsSupervisor) resolveRuntime(cwd string) (mode launchMode, execPath string, skillsDir string) {
	skillsDir = s.resolveSkillsDir(cwd)

	// 1. Next to the running executable (legacy release layout)
	if selfExe, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(selfExe), "skill-manager")
		if isExecutable(candidate) {
			log.Printf("[skills-sup] Found skill-manager binary next to executable: %s", candidate)
			return launchModeBinary, candidate, skillsDir
		}
	}

	// 2. ./bin/skill-manager relative to CWD
	candidate := filepath.Join(cwd, "bin", "skill-manager")
	if isExecutable(candidate) {
		log.Printf("[skills-sup] Found skill-manager binary in bin/: %s", candidate)
		return launchModeBinary, candidate, skillsDir
	}

	// 3. Host Python + source tree
	if skillsDir == "" || !isSkillsSource(skillsDir) {
		log.Printf("[skills-sup] No 1skills Python source found")
		s.logSkillsDiscoveryHints(cwd)
		return launchModeVenv, "", skillsDir
	}

	// Prefer an in-tree .venv (dev / previously bootstrapped next to source)
	inTree := venvPython(filepath.Join(skillsDir, ".venv"))
	if isExecutable(inTree) {
		log.Printf("[skills-sup] Using in-tree venv: %s", inTree)
		return launchModeVenv, inTree, skillsDir
	}

	// Managed user venv (release/npm: source is read-only under the package)
	managed := venvPython(managedSkillsVenvDir())
	if isExecutable(managed) {
		log.Printf("[skills-sup] Using managed venv: %s", managed)
		return launchModeVenv, managed, skillsDir
	}

	// Bootstrap will create managed venv (or in-tree when source is writable)
	log.Printf("[skills-sup] Host Python mode; will bootstrap venv for source: %s", skillsDir)
	return launchModeVenv, managed, skillsDir
}

// resolveSkillsDir picks the 1skills Python source tree.
// Priority: -skills-dir config → ONEAGENTS_SKILLS_DIR env → auto-discovery.
func (s *SkillsSupervisor) resolveSkillsDir(cwd string) string {
	if s.cfg != nil && s.cfg.SkillsSourceDir != "" {
		dir := s.cfg.SkillsSourceDir
		if isSkillsSource(dir) {
			if abs, err := filepath.Abs(dir); err == nil {
				log.Printf("[skills-sup] Using -skills-dir: %s", abs)
				return abs
			}
			log.Printf("[skills-sup] Using -skills-dir: %s", dir)
			return dir
		}
		log.Printf("[skills-sup] -skills-dir %q is not a valid 1skills source (need skill_manager/ + requirements.txt); falling back to discovery", dir)
	}

	if env := strings.TrimSpace(os.Getenv("ONEAGENTS_SKILLS_DIR")); env != "" {
		if isSkillsSource(env) {
			if abs, err := filepath.Abs(env); err == nil {
				log.Printf("[skills-sup] Using ONEAGENTS_SKILLS_DIR: %s", abs)
				return abs
			}
			log.Printf("[skills-sup] Using ONEAGENTS_SKILLS_DIR: %s", env)
			return env
		}
		log.Printf("[skills-sup] ONEAGENTS_SKILLS_DIR %q is not a valid 1skills source; falling back to discovery", env)
	}

	if found := findSkillsSource(cwd); found != "" {
		log.Printf("[skills-sup] Auto-discovered 1skills source: %s", found)
		return found
	}
	return ""
}

// logSkillsDiscoveryHints prints why source discovery failed (npm path vs cwd).
func (s *SkillsSupervisor) logSkillsDiscoveryHints(cwd string) {
	if s.cfg != nil && s.cfg.SkillsSourceDir != "" {
		log.Printf("[skills-sup] configured -skills-dir=%q (invalid or incomplete package layout)", s.cfg.SkillsSourceDir)
	}
	if env := strings.TrimSpace(os.Getenv("ONEAGENTS_SKILLS_DIR")); env != "" {
		log.Printf("[skills-sup] ONEAGENTS_SKILLS_DIR=%q (invalid or incomplete package layout)", env)
	}
	log.Printf("[skills-sup] cwd=%s", cwd)
	if selfExe, err := os.Executable(); err == nil {
		log.Printf("[skills-sup] executable=%s", selfExe)
	}
	log.Println("[skills-sup] Pass -skills-dir <path> (npm CLI resolves @1agents/skills), or set ONEAGENTS_SKILLS_DIR, or use modules/1skills in dev.")
}

// findSkillsSource locates the skill-manager Python source tree via heuristics.
// Prefer an explicit -skills-dir from the npm CLI when available.
func findSkillsSource(cwd string) string {
	// Prefer npm package @1agents/skills (direct registry install, or nested under cli)
	for _, start := range skillsSearchRoots(cwd) {
		candidate := filepath.Join(start, "node_modules", "@1agents", "skills")
		if isSkillsSource(candidate) {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return candidate
		}
	}

	// Dev: walk up for modules/1skills
	dir := cwd
	for {
		candidate := filepath.Join(dir, "modules", "1skills")
		if isSkillsSource(candidate) {
			return candidate
		}
		// monorepo npm package path
		candidate = filepath.Join(dir, "npm", "packages", "skills")
		if isSkillsSource(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Release / npm layouts relative to the 1agents executable
	// e.g. .../node_modules/@1agents/core-linux-arm64/bin/1agents
	//   → .../node_modules/@1agents/1agents/node_modules/@1agents/skills
	//   → .../node_modules/@1agents/skills (hoisted)
	if selfExe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(selfExe)
		for _, candidate := range []string{
			filepath.Join(exeDir, "1skills"),
			filepath.Join(exeDir, "..", "1skills"),
			filepath.Join(exeDir, "..", "..", "1skills"),
			filepath.Join(exeDir, "..", "@1agents", "skills"),
			filepath.Join(exeDir, "..", "..", "@1agents", "skills"),
		} {
			if resolved, err := filepath.Abs(candidate); err == nil && isSkillsSource(resolved) {
				return resolved
			}
		}
		// Walk up from bin/ looking for node_modules/@1agents/skills at each level
		dir := exeDir
		for i := 0; i < 10; i++ {
			candidate := filepath.Join(dir, "node_modules", "@1agents", "skills")
			if resolved, err := filepath.Abs(candidate); err == nil && isSkillsSource(resolved) {
				return resolved
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	for _, candidate := range []string{
		filepath.Join(cwd, "1skills"),
		filepath.Join(cwd, "modules", "1skills"),
	} {
		if isSkillsSource(candidate) {
			return candidate
		}
	}
	return ""
}

func skillsSearchRoots(cwd string) []string {
	seen := map[string]bool{}
	var roots []string
	add := func(p string) {
		if p == "" || seen[p] {
			return
		}
		seen[p] = true
		roots = append(roots, p)
	}
	add(cwd)
	if selfExe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(selfExe)
		add(exeDir)
		// Walk parents of the binary (covers nested node_modules installs)
		dir := exeDir
		for i := 0; i < 10; i++ {
			add(dir)
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Walk a few parents of CWD for nested node_modules installs
	dir := cwd
	for i := 0; i < 6; i++ {
		add(dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return roots
}

func isSkillsSource(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, "skill_manager"))
	if err != nil || !info.IsDir() {
		return false
	}
	if _, err := os.Stat(filepath.Join(dir, "requirements.txt")); err != nil {
		return false
	}
	return true
}

func managedSkillsVenvDir() string {
	return filepath.Join(get1AgentsHome(), ".1agents", "1skills", ".venv")
}

// venvPython returns the platform-specific python path inside a venv directory.
func venvPython(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "python.exe")
	}
	return filepath.Join(venvDir, "bin", "python")
}

func venvPip(venvDir string) string {
	if runtime.GOOS == "windows" {
		return filepath.Join(venvDir, "Scripts", "pip.exe")
	}
	return filepath.Join(venvDir, "bin", "pip")
}

// skillsDataEnv returns the environment overrides that pin 1skills
// (skill-manager) storage under the shared 1agents home (~/.1agents),
// keeping its data alongside meta.db/sync.db instead of the platform default
// (~/Library/Application Support/skill-manager on macOS).
//
// skill-manager honors the XDG base-dir vars and appends its own "skill-manager"
// component, so the effective root becomes ~/.1agents/skill-manager.
func skillsDataEnv() []string {
	base := filepath.Join(get1AgentsHome(), ".1agents")
	return []string{
		"XDG_CONFIG_HOME=" + base,
		"XDG_DATA_HOME=" + base,
		"XDG_STATE_HOME=" + base,
	}
}

// get1AgentsHome resolves the parent of the .1agents data directory, honoring
// ONEAGENTS_HOME (matching internal/meta and the rest of the backend).
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

// isExecutable returns true if path exists and is a regular executable file.
func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	// On Windows, .exe need not have unix execute bits.
	if runtime.GOOS == "windows" {
		return info.Mode().IsRegular()
	}
	return info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

// supervisionLoop manages the check, bootstrap, and process watch cycle.
func (s *SkillsSupervisor) supervisionLoop(ctx context.Context) {
	defer close(s.done)

	cwd, err := os.Getwd()
	if err != nil {
		log.Printf("[skills-sup] Failed to determine working directory: %v", err)
		return
	}

	mode, execPath, skillsDir := s.resolveRuntime(cwd)
	log.Printf("[skills-sup] Launch mode: %v, exec: %s, skillsDir: %s", mode, execPath, skillsDir)

	if mode == launchModeVenv {
		if skillsDir == "" || !isSkillsSource(skillsDir) {
			log.Println("[skills-sup] 1skills source missing and no skill-manager binary; skills service will not start.")
			log.Println("[skills-sup] Install @1agents/skills (npm), ensure @1agents/1agents passes -skills-dir (or run: 1agents install skills), or use modules/1skills in dev; need Python >= 3.11 (prefer uv).")
			return
		}
		if !isExecutable(execPath) {
			log.Println("[skills-sup] Virtual environment not found. Bootstrapping 1skills with host Python...")
			venvDir, err := s.bootstrap(ctx, skillsDir)
			if err != nil {
				log.Printf("[skills-sup] Bootstrapping failed: %v. Server will not be started.", err)
				return
			}
			execPath = venvPython(venvDir)
			if !isExecutable(execPath) {
				log.Printf("[skills-sup] Bootstrap finished but python missing at %s", execPath)
				return
			}
			log.Println("[skills-sup] Bootstrapping completed successfully.")
		}
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("[skills-sup] Shutdown requested, stopping 1skills.")
			s.stopProcess()
			return
		default:
		}

		if s.restartCount >= s.cfg.MaxRestarts {
			log.Printf("[skills-sup] FATAL: 1skills has restarted %d times consecutively. Giving up.", s.restartCount)
			return
		}

		log.Printf("[skills-sup] Starting 1skills microservice (attempt %d)...", s.restartCount+1)
		if err := s.startProcess(ctx, mode, skillsDir, execPath); err != nil {
			log.Printf("[skills-sup] 1skills exited with error: %v", err)
		} else {
			log.Println("[skills-sup] 1skills exited cleanly.")
		}

		if ctx.Err() != nil {
			log.Println("[skills-sup] Context cancelled after process exit, stopping supervisor.")
			return
		}

		s.mu.Lock()
		s.restartCount++
		count := s.restartCount
		s.mu.Unlock()

		log.Printf("[skills-sup] Restarting 1skills in %v... (%d/%d)",
			s.cfg.RestartDelay, count, s.cfg.MaxRestarts)

		select {
		case <-ctx.Done():
			log.Println("[skills-sup] Shutdown during restart wait, stopping.")
			return
		case <-time.After(s.cfg.RestartDelay):
		}
	}
}

// bootstrap creates a venv and installs requirements for the skills source tree.
// Prefers a managed venv under ~/.1agents when the source tree is not writable
// (typical for npm global installs); otherwise uses <source>/.venv.
// Prefer `uv` when available; otherwise python3 -m venv + pip.
// Returns the venv directory path.
func (s *SkillsSupervisor) bootstrap(ctx context.Context, sourceDir string) (string, error) {
	venvDir := filepath.Join(sourceDir, ".venv")
	if !dirIsWritable(sourceDir) {
		venvDir = managedSkillsVenvDir()
		log.Printf("[skills-sup] Source not writable; using managed venv at %s", venvDir)
	}
	if err := os.MkdirAll(filepath.Dir(venvDir), 0o755); err != nil {
		return "", err
	}

	req := filepath.Join(sourceDir, "requirements.txt")

	// Prefer uv
	if uv, err := exec.LookPath("uv"); err == nil {
		log.Printf("[skills-sup] Bootstrapping with uv (%s) at %s", uv, venvDir)
		cmdVenv := exec.CommandContext(ctx, uv, "venv", venvDir)
		cmdVenv.Stdout = os.Stdout
		cmdVenv.Stderr = os.Stderr
		if err := cmdVenv.Run(); err != nil {
			return "", fmt.Errorf("uv venv: %w", err)
		}
		// uv pip install --python <venv-python> -r requirements.txt
		py := venvPython(venvDir)
		cmdPip := exec.CommandContext(ctx, uv, "pip", "install", "--python", py, "-r", req)
		cmdPip.Dir = sourceDir
		cmdPip.Stdout = os.Stdout
		cmdPip.Stderr = os.Stderr
		if err := cmdPip.Run(); err != nil {
			return "", fmt.Errorf("uv pip install: %w", err)
		}
		return venvDir, nil
	}

	// Fallback: python3 -m venv + pip
	pythonBin := s.cfg.SkillsBinaryPath
	if pythonBin == "" {
		pythonBin = "python3"
	}
	if _, err := exec.LookPath(pythonBin); err != nil {
		return "", fmt.Errorf("host Python not found (%s) and uv not installed: %w — install Python >= 3.11 or uv", pythonBin, err)
	}

	log.Printf("[skills-sup] uv not found; creating venv with %s at %s...", pythonBin, venvDir)
	cmdVenv := exec.CommandContext(ctx, pythonBin, "-m", "venv", venvDir)
	cmdVenv.Stdout = os.Stdout
	cmdVenv.Stderr = os.Stderr
	if err := cmdVenv.Run(); err != nil {
		return "", fmt.Errorf("python -m venv: %w", err)
	}

	pipPath := venvPip(venvDir)
	log.Printf("[skills-sup] Installing requirements via %s -r %s...", pipPath, req)
	cmdPip := exec.CommandContext(ctx, pipPath, "install", "-r", req)
	cmdPip.Dir = sourceDir
	cmdPip.Stdout = os.Stdout
	cmdPip.Stderr = os.Stderr
	if err := cmdPip.Run(); err != nil {
		return "", fmt.Errorf("pip install: %w", err)
	}
	return venvDir, nil
}

func dirIsWritable(dir string) bool {
	probe := filepath.Join(dir, ".1agents-write-test")
	f, err := os.Create(probe)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

// startProcess runs the 1skills service and blocks until it exits.
// In binary mode the skill-manager executable is invoked directly.
// In venv mode the venv python interpreter is invoked with -m skill_manager
// and Dir set to the source tree so the package resolves from cwd.
func (s *SkillsSupervisor) startProcess(ctx context.Context, mode launchMode, dir string, execPath string) error {
	port := s.portFrom(s.cfg.SkillsAddr)

	var cmd *exec.Cmd
	switch mode {
	case launchModeBinary:
		cmd = exec.CommandContext(ctx, execPath,
			"serve",
			"--host", "127.0.0.1",
			"--port", port,
			"--no-open-browser",
		)
	default: // launchModeVenv
		cmd = exec.CommandContext(ctx, execPath,
			"-m", "skill_manager", "serve",
			"--host", "127.0.0.1",
			"--port", port,
			"--no-open-browser",
		)
		cmd.Dir = dir
	}

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), skillsDataEnv()...)

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	log.Printf("[skills-sup] exec: %s %s (Dir: %q)", execPath, strings.Join(cmd.Args[1:], " "), cmd.Dir)
	err := cmd.Run()

	if ctx.Err() != nil {
		return nil
	}
	return err
}

// stopProcess sends SIGINT or SIGKILL to stop the python process.
func (s *SkillsSupervisor) stopProcess() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cmd != nil && s.cmd.Process != nil {
		log.Println("[skills-sup] Sending SIGINT to 1skills...")
		_ = s.cmd.Process.Signal(os.Interrupt)
	}
}

// portFrom extracts the port number from an address string (e.g. "127.0.0.1:8000" -> "8000")
func (s *SkillsSupervisor) portFrom(addr string) string {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			return addr[i+1:]
		}
	}
	return addr
}
