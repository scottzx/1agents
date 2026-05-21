package git

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Handler exposes Git operations for a single workdir repository.
type Handler struct {
	root string // absolute path to the repository root (cfg.WorkDir)
}

// NewHandler creates a Handler for the given working directory.
func NewHandler(root string) *Handler {
	abs, err := filepath.Abs(root)
	if err != nil {
		log.Fatalf("[git] cannot resolve root %q: %v", root, err)
	}
	return &Handler{root: abs}
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
