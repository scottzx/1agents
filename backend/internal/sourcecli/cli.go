// Package sourcecli probes the lifecycle of the external command-line tools that
// back a data source (飞书 lark-cli today; other source CLIs plug in via CLITool).
// It answers the three questions the 数据源 UI needs but the ingestion path does
// not care about: is the CLI installed, is it authenticated (and until when), and
// is a newer version available. Detection shells out read-only — logging in and
// updating stay a manual user action; this package only reports state and hands
// back the command to run.
package sourcecli

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// Runner runs one CLI invocation and returns stdout. It is the test seam: unit
// tests inject canned JSON instead of shelling out to a real binary.
type Runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

// defaultRunner shells out to bin. stderr is discarded here (probes tolerate a
// non-zero exit — e.g. `auth status` fails when not logged in, which is itself a
// signal), so callers read only stdout and the returned error.
func defaultRunner(ctx context.Context, bin string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, bin, args...).Output()
}

// CLIStatus is the aggregate lifecycle snapshot for one source CLI, rendered by
// the frontend's CLI-lifecycle card. All optional fields are omitempty so a
// not-installed tool serializes to a minimal object.
type CLIStatus struct {
	Tool             string     `json:"tool"`
	Installed        bool       `json:"installed"`
	Path             string     `json:"path,omitempty"`
	Version          string     `json:"version,omitempty"`
	LatestVersion    string     `json:"latestVersion,omitempty"`
	UpdateAvailable  bool       `json:"updateAvailable"`
	Authenticated    bool       `json:"authenticated"`
	AuthAccount      string     `json:"authAccount,omitempty"`  // display name of the logged-in user
	AuthIdentity     string     `json:"authIdentity,omitempty"` // identity type, e.g. "user"
	TokenStatus      string     `json:"tokenStatus,omitempty"`  // valid|expired|...
	AuthExpiresAt    *time.Time `json:"authExpiresAt,omitempty"`
	RefreshExpiresAt *time.Time `json:"refreshExpiresAt,omitempty"`
	Scopes           []string   `json:"scopes,omitempty"`     // granted OAuth scopes (which domains are reachable)
	LoginHint        string     `json:"loginHint,omitempty"`  // command to (re)login, e.g. "lark-cli auth login"
	UpdateHint       string     `json:"updateHint,omitempty"` // command to update, e.g. "lark-cli update"
	InstallHint      string     `json:"installHint,omitempty"`
	Error            string     `json:"error,omitempty"` // non-fatal probe error (surfaced, not thrown)
	CheckedAt        time.Time  `json:"checkedAt"`
}

// CLITool is one probeable source CLI. Detect performs the full read-only probe
// (install + version + update + auth) and must never fail hard: transient errors
// land in CLIStatus.Error so the card degrades gracefully instead of 500-ing.
type CLITool interface {
	Name() string
	Detect(ctx context.Context) CLIStatus
}

// Manager registers source CLIs and caches their status behind a short TTL, so
// opening the data-source card doesn't shell out on every request.
type Manager struct {
	ttl   time.Duration
	mu    sync.Mutex
	tools map[string]CLITool
	cache map[string]cachedStatus
}

type cachedStatus struct {
	status CLIStatus
	at     time.Time
}

// NewManager builds a Manager with the given cache TTL (<=0 → 60s).
func NewManager(ttl time.Duration) *Manager {
	if ttl <= 0 {
		ttl = 60 * time.Second
	}
	return &Manager{ttl: ttl, tools: map[string]CLITool{}, cache: map[string]cachedStatus{}}
}

// Register adds a tool under its Name(). Last registration wins.
func (m *Manager) Register(t CLITool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tools[t.Name()] = t
}

// Status returns tool's status, using the cached value when still fresh.
// ok is false when the tool name is unknown.
func (m *Manager) Status(ctx context.Context, tool string) (CLIStatus, bool) {
	m.mu.Lock()
	t, known := m.tools[tool]
	if !known {
		m.mu.Unlock()
		return CLIStatus{}, false
	}
	if c, ok := m.cache[tool]; ok && time.Since(c.at) < m.ttl {
		m.mu.Unlock()
		return c.status, true
	}
	m.mu.Unlock()

	// Probe outside the lock (Detect shells out and can block on the network).
	st := t.Detect(ctx)
	m.mu.Lock()
	m.cache[tool] = cachedStatus{status: st, at: time.Now()}
	m.mu.Unlock()
	return st, true
}

// Recheck forces a fresh probe, bypassing (and refreshing) the cache.
func (m *Manager) Recheck(ctx context.Context, tool string) (CLIStatus, bool) {
	m.mu.Lock()
	delete(m.cache, tool)
	m.mu.Unlock()
	return m.Status(ctx, tool)
}
