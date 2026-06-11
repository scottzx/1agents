package agent

import (
	"context"
	"log"
	"sync"
	"time"
)

type WorkspaceLock struct {
	mu      sync.Mutex
	running map[string]string // workspacePath -> task ID
}

func NewWorkspaceLock() *WorkspaceLock {
	return &WorkspaceLock{
		running: make(map[string]string),
	}
}

func (wl *WorkspaceLock) TryAcquire(workspace, taskId string) bool {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	if _, occupied := wl.running[workspace]; occupied {
		return false // Workspace concurrency lock occupied
	}
	wl.running[workspace] = taskId
	return true
}

func (wl *WorkspaceLock) Release(workspace string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	delete(wl.running, workspace)
}

func (wl *WorkspaceLock) GetRunning(workspace string) (string, bool) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	id, occupied := wl.running[workspace]
	return id, occupied
}

type Scheduler struct {
	Lock         *WorkspaceLock
	tasksStore   *TasksStore
	workspacesFn func() ([]string, error)
	ticker       *time.Ticker
}

func NewScheduler(tasksStore *TasksStore, workspacesFn func() ([]string, error)) *Scheduler {
	return &Scheduler{
		Lock:         NewWorkspaceLock(),
		tasksStore:   tasksStore,
		workspacesFn: workspacesFn,
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	s.ticker = time.NewTicker(5 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.Tick()
			case <-ctx.Done():
				if s.ticker != nil {
					s.ticker.Stop()
				}
				log.Println("[scheduler] Tasks scheduler stopped.")
				return
			}
		}
	}()
	log.Println("[scheduler] Tasks scheduler started.")
}


func (s *Scheduler) Tick() {
	paths, err := s.workspacesFn()
	if err != nil {
		log.Printf("[scheduler] Failed to list workspaces: %v", err)
		return
	}

	for _, path := range paths {
		s.tickWorkspace(path)
	}
}

func (s *Scheduler) tickWorkspace(workspacePath string) {
	cfg, err := s.tasksStore.Load(workspacePath)
	if err != nil {
		return
	}

	modified := false
	now := time.Now().UTC()

	// Build map of tasks by ID for dependency checking
	taskMap := make(map[string]*Task)
	for i := range cfg.Tasks {
		taskMap[cfg.Tasks[i].ID] = &cfg.Tasks[i]
	}

	for i := range cfg.Tasks {
		task := &cfg.Tasks[i]

		// Release lock if task completed/failed/cancelled
		if task.Status == TaskStatusRunning {
			// Check if we need to release lock
			// The lock is released if task status changes from Running to a terminal state
			continue
		}

		if task.Status != TaskStatusPending && task.Status != TaskStatusQueued {
			continue
		}

		// 1. Time checks (ScheduledAt)
		if task.ScheduleType == ScheduleTypeScheduled && task.ScheduledAt != nil {
			if task.ScheduledAt.After(now) {
				continue // Time has not arrived yet
			}
		}

		// 2. Dependency checks
		depsMet := true
		for _, depId := range task.DependsOn {
			dep, exists := taskMap[depId]
			if !exists || dep.Status != TaskStatusCompleted {
				depsMet = false
				break
			}
		}

		if !depsMet {
			// Defer execution if dependencies are not met (顺延)
			continue
		}

		// 3. Concurrency Lock check
		if task.Status == TaskStatusPending {
			task.Status = TaskStatusQueued
			task.UpdatedAt = now
			modified = true
		}

		if s.Lock.TryAcquire(workspacePath, task.ID) {
			// Transition to running
			task.Status = TaskStatusRunning
			task.StartedAt = &now
			task.UpdatedAt = now
			modified = true

			log.Printf("[scheduler] Concurrency Lock acquired. Task %s is now running in workspace %s", task.ID, workspacePath)
		}
	}

	if modified {
		if err := s.tasksStore.Save(workspacePath, cfg); err != nil {
			log.Printf("[scheduler] Failed to save tasks config in %s: %v", workspacePath, err)
		}
	}
}
