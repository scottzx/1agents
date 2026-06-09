package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Sentinel errors for store operations.
var (
	ErrDuplicate = fmt.Errorf("agent: duplicate record id")
	ErrNotFound  = fmt.Errorf("agent: record not found")
)
