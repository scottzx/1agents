package git

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/config"
)

// Handler exposes Git operations for a single workdir repository.
type Handler struct {
	root string // absolute path to the repository root (cfg.WorkDir)
}

// NewHandler creates a Handler for the given working directory.
func NewHandler(root string) *Handler {
	rootExpanded := expandTilde(root)
	abs, err := filepath.Abs(rootExpanded)
	if err != nil {
		log.Fatalf("[git] cannot resolve root %q: %v", root, err)
	}
	return &Handler{root: abs}
}

// SetRoot changes the working directory at runtime.
func (h *Handler) SetRoot(newRoot string) error {
	rootExpanded := expandTilde(newRoot)
	abs, err := filepath.Abs(rootExpanded)
	if err != nil {
		return err
	}
	h.root = abs
	log.Printf("[git] root changed to %q", abs)
	return nil
}

// expandTilde expands a ~ prefix to the user's home directory.
func expandTilde(path string) string {
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, "~"+string(os.PathSeparator)) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// --- Data types ---

// FileStatus represents a single changed file in the working tree.
type FileStatus struct {
	Path   string `json:"path"`
	Status string `json:"status"` // M, A, D, R, ?, etc.
}

// GitStatus is the full repository status returned by /api/git/status.
type GitStatus struct {
	Branch    string       `json:"branch"`
	Ahead     int          `json:"ahead"`
	Behind    int          `json:"behind"`
	Staged    []FileStatus `json:"staged"`
	Unstaged  []FileStatus `json:"unstaged"`
	Untracked []FileStatus `json:"untracked"`
	IsRepo    bool         `json:"isRepo"`
}

// CommitEntry is a single line from git log.
type CommitEntry struct {
	Hash    string `json:"hash"`
	Short   string `json:"short"`
	Message string `json:"message"`
	Author  string `json:"author"`
	Time    int64  `json:"time"`
}

// BranchEntry is a single branch name.
type BranchEntry struct {
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// --- HTTP handlers ---

// Status handles GET /api/git/status
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if this is a git repo
	if !h.isRepo() {
		writeJSON(w, GitStatus{IsRepo: false})
		return
	}

	status := GitStatus{IsRepo: true}
	status.Branch = h.currentBranch()
	status.Ahead, status.Behind = h.aheadBehind()
	status.Staged, status.Unstaged, status.Untracked = h.changedFiles()

	writeJSON(w, status)
}

// Diff handles GET /api/git/diff?file=<path>&staged=<bool>
func (h *Handler) Diff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file := r.URL.Query().Get("file")
	staged := r.URL.Query().Get("staged") == "true"

	var args []string
	if staged {
		args = []string{"diff", "--cached", "--", file}
	} else {
		args = []string{"diff", "--", file}
	}

	out, err := h.git(args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(out))
}

// Stage handles POST /api/git/stage?file=<path>  (or ?all=true)
func (h *Handler) Stage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	all := r.URL.Query().Get("all") == "true"
	file := r.URL.Query().Get("file")

	var err error
	if all {
		_, err = h.git("add", "-A")
	} else if file != "" {
		_, err = h.git("add", "--", file)
	} else {
		http.Error(w, "must specify file or all=true", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// Unstage handles POST /api/git/unstage?file=<path>  (or ?all=true)
func (h *Handler) Unstage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	all := r.URL.Query().Get("all") == "true"
	file := r.URL.Query().Get("file")

	var err error
	if all {
		_, err = h.git("reset", "HEAD", "--")
	} else if file != "" {
		_, err = h.git("restore", "--staged", "--", file)
	} else {
		http.Error(w, "must specify file or all=true", http.StatusBadRequest)
		return
	}

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// Commit handles POST /api/git/commit  (body: {"message":"…"})
func (h *Handler) Commit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Message == "" {
		http.Error(w, "missing message", http.StatusBadRequest)
		return
	}

	out, err := h.git("commit", "-m", body.Message)
	if err != nil {
		http.Error(w, out+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "output": out})
}

// Log handles GET /api/git/log?limit=20
func (h *Handler) Log(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	// Format: hash|short|unix-timestamp|author|message
	format := "--pretty=format:%H|%h|%at|%an|%s"
	out, err := h.git("log", format, "-n", strconv.Itoa(limit))
	if err != nil {
		// Could be an empty repo (no commits yet)
		writeJSON(w, []CommitEntry{})
		return
	}

	var entries []CommitEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}
		ts, _ := strconv.ParseInt(parts[2], 10, 64)
		entries = append(entries, CommitEntry{
			Hash:    parts[0],
			Short:   parts[1],
			Time:    ts,
			Author:  parts[3],
			Message: parts[4],
		})
	}
	if entries == nil {
		entries = []CommitEntry{}
	}
	writeJSON(w, entries)
}

// Branches handles GET /api/git/branches
func (h *Handler) Branches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	out, err := h.git("branch", "--format=%(refname:short)|%(HEAD)")
	if err != nil {
		writeJSON(w, []BranchEntry{})
		return
	}

	var branches []BranchEntry
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 2)
		if len(parts) != 2 {
			continue
		}
		branches = append(branches, BranchEntry{
			Name:    parts[0],
			Current: parts[1] == "*",
		})
	}
	if branches == nil {
		branches = []BranchEntry{}
	}
	writeJSON(w, branches)
}

// Checkout handles POST /api/git/checkout  (body: {"branch":"…","create":false})
func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body struct {
		Branch string `json:"branch"`
		Create bool   `json:"create"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Branch == "" {
		http.Error(w, "missing branch", http.StatusBadRequest)
		return
	}

	var args []string
	if body.Create {
		args = []string{"checkout", "-b", body.Branch}
	} else {
		args = []string{"checkout", body.Branch}
	}

	out, err := h.git(args...)
	if err != nil {
		http.Error(w, out+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "output": out})
}

// Push handles POST /api/git/push
func (h *Handler) Push(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out, err := h.git("push")
	if err != nil {
		http.Error(w, out+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "output": out})
}

// Pull handles POST /api/git/pull
func (h *Handler) Pull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	out, err := h.git("pull")
	if err != nil {
		http.Error(w, out+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "output": out})
}

// Discard handles POST /api/git/discard?file=<path>
func (h *Handler) Discard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	file := r.URL.Query().Get("file")
	if file == "" {
		http.Error(w, "must specify file", http.StatusBadRequest)
		return
	}

	// Revert uncommitted changes in working directory
	out, err := h.git("checkout", "--", file)
	if err != nil {
		// Fallback to git restore if checkout fails or is unsupported
		out, err = h.git("restore", "--", file)
	}

	if err != nil {
		http.Error(w, out+"\n"+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

// AICommit handles POST /api/git/ai-commit
func (h *Handler) AICommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Check if there are staged changes
	_, err := h.git("diff", "--cached", "--quiet")
	if err == nil {
		// Exit code 0 means no changes staged
		w.WriteHeader(http.StatusBadRequest)
		writeJSON(w, map[string]any{"ok": false, "error": "工作区没有已暂存的更改。请先暂存文件，然后再试。"})
		return
	}

	// 2. Fetch agent config from CC Connect
	ccEnv, claudeBin, cliExtraArgs, ccErr := h.getCCConnectAgentEnv()
	if ccErr != nil {
		log.Printf("[git/ai-commit] CC Connect config lookup warning: %v, falling back to default claude CLI", ccErr)
	}

	// 3. Execute Claude Code non-interactively with empty stdin
	prompt := "Write a concise, conventional git commit message in Chinese (中文) based on the staged changes in this git repository. The commit type prefix (e.g., feat, fix, docs, style, refactor) must be in English, but the actual message description MUST be written in clear, concise Chinese. Output ONLY the raw commit message itself, without quotes, code blocks, or explanations."
	
	var args []string
	if len(cliExtraArgs) > 0 {
		args = append(args, cliExtraArgs...)
	}
	args = append(args, "-p", prompt)

	cmd := exec.Command(claudeBin, args...)
	cmd.Dir = h.root
	cmd.Stdin = strings.NewReader("")

	// Filter out CLAUDECODE env var to avoid nesting checks
	var baseEnv []string
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "CLAUDECODE=") {
			baseEnv = append(baseEnv, e)
		}
	}
	
	// Merge CC Connect environment variables
	if len(ccEnv) > 0 {
		cmd.Env = mergeEnvironments(baseEnv, ccEnv)
	} else {
		cmd.Env = baseEnv
	}

	out, err := cmd.CombinedOutput()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		writeJSON(w, map[string]any{"ok": false, "error": fmt.Sprintf("AI 生成失败: %s\n%v", string(out), err)})
		return
	}

	// 4. Parse the output (trim the "Warning: no stdin..." text if present)
	outputStr := string(out)
	warningStr := "Warning: no stdin data received"
	if idx := strings.Index(outputStr, warningStr); idx >= 0 {
		if lineEnd := strings.Index(outputStr[idx:], "\n"); lineEnd >= 0 {
			outputStr = outputStr[idx+lineEnd+1:]
		}
	}

	outputStr = strings.TrimSpace(outputStr)
	writeJSON(w, map[string]any{"ok": true, "message": outputStr})
}

// --- Internal helpers ---

func (h *Handler) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = h.root
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (h *Handler) isRepo() bool {
	_, err := h.git("rev-parse", "--git-dir")
	return err == nil
}

func (h *Handler) currentBranch() string {
	out, err := h.git("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "HEAD"
	}
	return out
}

func (h *Handler) aheadBehind() (ahead, behind int) {
	out, err := h.git("rev-list", "--count", "--left-right", "@{upstream}...HEAD")
	if err != nil {
		return 0, 0
	}
	parts := strings.Fields(out)
	if len(parts) == 2 {
		behind, _ = strconv.Atoi(parts[0])
		ahead, _ = strconv.Atoi(parts[1])
	}
	return
}

// changedFiles parses `git status --porcelain=v1` into three buckets.
func (h *Handler) changedFiles() (staged, unstaged, untracked []FileStatus) {
	staged = []FileStatus{}
	unstaged = []FileStatus{}
	untracked = []FileStatus{}

	out, err := h.git("status", "--porcelain=v1")
	if err != nil || out == "" {
		return
	}

	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		x := string(line[0]) // staged status
		y := string(line[1]) // unstaged status
		path := strings.TrimSpace(line[3:])

		// Handle renames: "old -> new"
		if strings.Contains(path, " -> ") {
			parts := strings.SplitN(path, " -> ", 2)
			path = parts[1]
		}

		if x == "?" && y == "?" {
			untracked = append(untracked, FileStatus{Path: path, Status: "?"})
			continue
		}
		if x != " " && x != "?" {
			staged = append(staged, FileStatus{Path: path, Status: x})
		}
		if y != " " && y != "?" {
			unstaged = append(unstaged, FileStatus{Path: path, Status: y})
		}
	}
	return
}

// writeJSON serialises v to w with application/json content-type.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[git] json encode error: %v", err)
	}
}

// Ensure time package import is used (used implicitly via CommitEntry).
var _ = time.Now

// getCCConnectAgentEnv fetches project agent options and environment variables from ~/.cc-connect/config.toml
func (h *Handler) getCCConnectAgentEnv() ([]string, string, []string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, "claude", nil, err
	}
	configPath := filepath.Join(home, ".cc-connect", "config.toml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return nil, "claude", nil, fmt.Errorf("cc-connect config file not found at %s", configPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, "claude", nil, fmt.Errorf("failed to load cc-connect config: %w", err)
	}

	var matchedProj *config.ProjectConfig
	for i := range cfg.Projects {
		proj := &cfg.Projects[i]
		workDir, _ := proj.Agent.Options["work_dir"].(string)
		if workDir == "" {
			continue
		}
		projAbs, err := filepath.Abs(expandTilde(workDir))
		if err != nil {
			continue
		}
		hAbs, err := filepath.Abs(h.root)
		if err != nil {
			continue
		}
		if projAbs == hAbs {
			matchedProj = proj
			break
		}
	}

	if matchedProj == nil {
		return nil, "claude", nil, fmt.Errorf("no matching project found for workspace path %s", h.root)
	}

	// Extract binary path from agent options
	cliBin := "claude"
	var cliExtraArgs []string
	if cliPath, _ := matchedProj.Agent.Options["cli_path"].(string); cliPath != "" {
		parts := strings.Fields(cliPath)
		cliBin = parts[0]
		if len(parts) > 1 {
			cliExtraArgs = parts[1:]
		}
	} else if path, err := exec.LookPath("claude"); err == nil {
		cliBin = path
	} else {
		localPath := filepath.Join(home, ".local", "bin", "claude")
		if _, err := os.Stat(localPath); err == nil {
			cliBin = localPath
		}
	}

	// Build the environments list
	var env []string

	// 1. First, find active provider
	var activeProvider *config.ProviderConfig
	if len(matchedProj.Agent.Providers) > 0 {
		activeProvider = &matchedProj.Agent.Providers[0]
	} else if len(matchedProj.Agent.ProviderRefs) > 0 {
		refName := matchedProj.Agent.ProviderRefs[0]
		for i := range cfg.Providers {
			if cfg.Providers[i].Name == refName {
				activeProvider = &cfg.Providers[i]
				break
			}
		}
	}

	// 2. Add provider specific environments
	if activeProvider != nil {
		if activeProvider.BaseURL != "" {
			env = append(env, "ANTHROPIC_BASE_URL="+activeProvider.BaseURL)
			if activeProvider.APIKey != "" {
				env = append(env, "ANTHROPIC_AUTH_TOKEN="+activeProvider.APIKey)
				env = append(env, "ANTHROPIC_API_KEY=")
			}
			if activeProvider.Model != "" {
				env = append(env, "ANTHROPIC_MODEL="+activeProvider.Model)
			}
		} else {
			if activeProvider.APIKey != "" {
				env = append(env, "ANTHROPIC_API_KEY="+activeProvider.APIKey)
			}
		}
		for k, v := range activeProvider.Env {
			env = append(env, k+"="+v)
		}
	}

	// 3. Add agent option envs
	if envMap, ok := matchedProj.Agent.Options["env"].(map[string]string); ok {
		for k, v := range envMap {
			env = append(env, k+"="+v)
		}
	} else if envMap, ok := matchedProj.Agent.Options["env"].(map[string]any); ok {
		for k, v := range envMap {
			if s, ok := v.(string); ok {
				env = append(env, k+"="+s)
			}
		}
	}

	return env, cliBin, cliExtraArgs, nil
}

// mergeEnvironments merges override environments into base environment slice, ensuring uniqueness
func mergeEnvironments(base []string, overrides []string) []string {
	m := make(map[string]string)
	for _, entry := range base {
		if parts := strings.SplitN(entry, "=", 2); len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	for _, entry := range overrides {
		if parts := strings.SplitN(entry, "=", 2); len(parts) == 2 {
			m[parts[0]] = parts[1]
		}
	}
	var res []string
	for k, v := range m {
		res = append(res, k+"="+v)
	}
	return res
}
