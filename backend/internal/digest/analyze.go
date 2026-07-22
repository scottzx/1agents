package digest

import (
	"fmt"
	"time"

	"github.com/scottzx/1Agents/backend/internal/feishu"
	"github.com/scottzx/1Agents/backend/internal/meta"
)

// PrepareBatch assembles the single-batch analysis prompt for a chat: it pulls
// the session's messages (create_time >= since, epoch ms; since<=0 = all),
// resolves the chat's value-extraction templates (bound, or the global
// default), and renders both into the instruction text. n is the message count.
// This is the deterministic half — no LLM — so it is fully testable.
func PrepareBatch(fs *feishu.Store, ds *meta.DigestStore, channel, sessionID, chatName string, since int64) (prompt string, n int, err error) {
	msgs, err := fs.ListMessages(channel, sessionID, since, 0)
	if err != nil {
		return "", 0, err
	}
	tpls, err := ds.TemplatesForSession(sessionID)
	if err != nil {
		return "", 0, err
	}
	return BuildAnalysisPrompt(chatName, tpls, msgs), len(msgs), nil
}

// CreateAnalysisTask creates a scheduler-eligible task whose Description is the
// analysis prompt, in the given workspace. The existing Scheduler+TaskRunner
// then runs it (Execute uses Description as the agent instruction) and the
// agent's value extraction lands as a Reply on the task timeline — the L3 card.
//
// The task is a plain immediate 'task' with no assignee, so it passes the
// scheduler's eligibility checks (not discussion/requirement/bug/AI-suggested,
// assignee != user).
func CreateAnalysisTask(ts *meta.TaskStore, workspacePath, chatName, prompt string) (meta.Task, error) {
	now := time.Now().UTC()
	task := meta.Task{
		ID:           meta.NewID(),
		Title:        fmt.Sprintf("群「%s」聊天价值提取", chatName),
		Description:  prompt,
		IssueState:   meta.IssueOpen,
		Status:       meta.TaskStatusPending,
		Type:         meta.ItemTypeTask,
		Priority:     meta.PriorityMedium,
		ScheduleType: meta.ScheduleTypeImmediate,
		MaxRetries:   1,
		CreatedAt:    now,
		UpdatedAt:    now,
		Replies:      []meta.Reply{},
		Sessions:     []meta.SessionMetadata{},
	}
	if err := ts.Mutate(workspacePath, func(cfg *meta.TasksConfig) bool {
		cfg.Tasks = append(cfg.Tasks, task)
		return true
	}); err != nil {
		return meta.Task{}, err
	}
	// Save assigns the short number (#N); re-fetch the stored row.
	saved, _, err := ts.GetTask(task.ID)
	if err != nil {
		return meta.Task{}, err
	}
	return saved, nil
}
