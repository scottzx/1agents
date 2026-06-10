package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// configDir is the per-user config directory; same as workspace/handler.go.
var configDir string

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

func init() {
	configDir = filepath.Join(get1AgentsHome(), ".1agents")
}

const configFile = "agent-sessions.json"

// Store is a JSON-file-backed CRUD store for ChatSessionRecord.
//
// Modeled on internal/workspace/handler.go's WorkspacesConfig:
//   - single file under ~/.1agents
//   - sync.RWMutex to serialize readers/writers
//   - atomic write (temp file + rename) so a crash mid-write can't corrupt
//
// The whole file is loaded into memory; the list is small (tens of
// sessions per workspace) and lookups are by id or workspace id, so a
// linear scan is fine.
type Store struct {
	mu   sync.RWMutex
	path string
}

// NewStore returns a Store backed by ~/.1agents/agent-sessions.json.
func NewStore() (*Store, error) {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return nil, fmt.Errorf("agent: ensure config dir: %w", err)
	}
	return &Store{path: filepath.Join(configDir, configFile)}, nil
}

func (s *Store) load() (*fileConfig, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return &fileConfig{Sessions: []ChatSessionRecord{}}, nil
		}
		return nil, err
	}
	var cfg fileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("agent: parse %s: %w", s.path, err)
	}
	if cfg.Sessions == nil {
		cfg.Sessions = []ChatSessionRecord{}
	}
	return &cfg, nil
}

// saveLocked writes cfg atomically. Caller must hold s.mu for writing.
func (s *Store) saveLocked(cfg *fileConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// ListByWorkspace returns all chat sessions belonging to a workspace,
// sorted newest-first by CreatedAt.
func (s *Store) ListByWorkspace(workspaceID string) ([]ChatSessionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.load()
	if err != nil {
		return nil, err
	}
	out := make([]ChatSessionRecord, 0)
	for _, rec := range cfg.Sessions {
		if rec.WorkspaceID == workspaceID {
			out = append(out, rec)
		}
	}
	// newest first
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out, nil
}

// Get returns a single record by id, or (zero, false) if not found.
func (s *Store) Get(id string) (ChatSessionRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	cfg, err := s.load()
	if err != nil {
		return ChatSessionRecord{}, false, err
	}
	for _, rec := range cfg.Sessions {
		if rec.ID == id {
			return rec, true, nil
		}
	}
	return ChatSessionRecord{}, false, nil
}

// Add inserts a new record. Returns ErrDuplicate if id already exists.
func (s *Store) Add(rec ChatSessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	for _, existing := range cfg.Sessions {
		if existing.ID == rec.ID {
			return ErrDuplicate
		}
	}
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now().UTC()
	}
	cfg.Sessions = append(cfg.Sessions, rec)
	return s.saveLocked(cfg)
}

// Delete removes the record with the given id. Returns ErrNotFound if no match.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	for i := 0; i < len(cfg.Sessions); i++ {
		if cfg.Sessions[i].ID == id {
			cfg.Sessions = append(cfg.Sessions[:i], cfg.Sessions[i+1:]...)
			return s.saveLocked(cfg)
		}
	}
	return ErrNotFound
}

// Touch updates the LastEventAt timestamp on a record.
func (s *Store) Touch(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	for i := range cfg.Sessions {
		if cfg.Sessions[i].ID == id {
			cfg.Sessions[i].LastEventAt = time.Now().UTC()
			return s.saveLocked(cfg)
		}
	}
	return ErrNotFound
}

// UpdateName updates the name/title of the session with the given id.
func (s *Store) UpdateName(id, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	for i := range cfg.Sessions {
		if cfg.Sessions[i].ID == id {
			cfg.Sessions[i].Name = name
			return s.saveLocked(cfg)
		}
	}
	return ErrNotFound
}

// UpdateACP persists the agent-managed session id for a chat record. Used
// when the bridge-server reports back the agent's session uuid via
// session_ready, so that subsequent opens can resume the same session
// (and find its native storage, e.g. Claude Code's <uuid>.jsonl).
// It also tries to resolve a descriptive session title from Claude's sessions index
// if the session currently has a default or empty name.
func (s *Store) UpdateACP(id, acpSessionID string) error {
	if acpSessionID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg, err := s.load()
	if err != nil {
		return err
	}
	for i := range cfg.Sessions {
		if cfg.Sessions[i].ID == id {
			updated := false
			if cfg.Sessions[i].AcpSessionID == "" {
				cfg.Sessions[i].AcpSessionID = acpSessionID
				updated = true
			}
			// If current name is empty or a generic default, try to resolve it from Claude index
			name := cfg.Sessions[i].Name
			if name == "" || name == "聊天会话" || name == "新建会话" || strings.HasPrefix(name, "Chat") || strings.HasSuffix(name, "会话") {
				if title, err := ResolveClaudeSessionName(acpSessionID); err == nil && title != "" {
					cfg.Sessions[i].Name = title
					updated = true
				}
			}
			if updated {
				return s.saveLocked(cfg)
			}
			return nil
		}
	}
	return ErrNotFound
}

// ResolveClaudeSessionName searches ~/.claude/projects/*/sessions-index.json
// for the given acpSessionID, and returns the firstPrompt as the session title.
func ResolveClaudeSessionName(acpSessionID string) (string, error) {
	if acpSessionID == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	projectsDir := filepath.Join(home, ".claude", "projects")

	// Read projects directory
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	type SessionEntry struct {
		SessionID   string `json:"sessionId"`
		FirstPrompt string `json:"firstPrompt"`
	}
	type SessionsIndex struct {
		Entries []SessionEntry `json:"entries"`
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		indexPath := filepath.Join(projectsDir, entry.Name(), "sessions-index.json")
		data, err := os.ReadFile(indexPath)
		if err != nil {
			continue
		}

		var idx SessionsIndex
		if err := json.Unmarshal(data, &idx); err != nil {
			continue
		}

		for _, item := range idx.Entries {
			if item.SessionID == acpSessionID {
				title := item.FirstPrompt
				title = cleanSessionTitle(title)
				return title, nil
			}
		}
	}

	return "", nil
}

func cleanSessionTitle(prompt string) string {
	// Strip HTML tags like <ide_opened_file>...</ide_opened_file> or <command-message>...</command-message>
	for {
		start := strings.Index(prompt, "<")
		if start == -1 {
			break
		}
		end := strings.Index(prompt[start:], ">")
		if end == -1 {
			break
		}
		prompt = prompt[:start] + prompt[start+end+1:]
	}

	prompt = strings.TrimSpace(prompt)
	// Replace newlines/multiple spaces with a single space
	prompt = strings.ReplaceAll(prompt, "\r", "")
	prompt = strings.ReplaceAll(prompt, "\n", " ")
	for strings.Contains(prompt, "  ") {
		prompt = strings.ReplaceAll(prompt, "  ", " ")
	}

	// Truncate to N characters
	const maxLen = 60
	runes := []rune(prompt)
	if len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return prompt
}

// Sentinel errors for store operations.
var (
	ErrDuplicate = fmt.Errorf("agent: duplicate record id")
	ErrNotFound  = fmt.Errorf("agent: record not found")
)

type TasksStore struct {
	mu sync.RWMutex
}

func NewTasksStore() *TasksStore {
	return &TasksStore{}
}

func (s *TasksStore) getTasksFilePath(workspacePath string) string {
	return filepath.Join(workspacePath, ".1agents", "tasks.json")
}

func (s *TasksStore) Load(workspacePath string) (*TasksConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filePath := s.getTasksFilePath(workspacePath)
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &TasksConfig{Tasks: []Task{}}, nil
		}
		return nil, err
	}

	var cfg TasksConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("agent: parse tasks %s: %w", filePath, err)
	}
	if cfg.Tasks == nil {
		cfg.Tasks = []Task{}
	}
	return &cfg, nil
}

func (s *TasksStore) Save(workspacePath string, cfg *TasksConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := s.getTasksFilePath(workspacePath)
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("agent: ensure workspace tasks config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmp := filePath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filePath)
}

