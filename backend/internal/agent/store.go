package agent

import "github.com/scottzx/1Agents/backend/internal/meta"

// Store and TasksStore are the SQLite-backed metadata stores (moved to
// internal/meta as part of the project-model redesign; see
// docs/features/project-model/design.md). The aliases keep this package's
// handlers, scheduler, and acpx client unchanged.
type (
	Store      = meta.SessionStore
	TasksStore = meta.TaskStore
)

// Sentinel errors for store operations (aliased so existing errors.Is
// checks keep working).
var (
	ErrDuplicate = meta.ErrDuplicate
	ErrNotFound  = meta.ErrNotFound
)

// NewStore returns the chat-session store backed by ~/.1agents/meta.db.
func NewStore() (*Store, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	return meta.NewSessionStore(db), nil
}

// NewTasksStore returns the task store backed by ~/.1agents/meta.db.
func NewTasksStore() (*TasksStore, error) {
	db, err := meta.OpenDefault()
	if err != nil {
		return nil, err
	}
	return meta.NewTaskStore(db), nil
}
