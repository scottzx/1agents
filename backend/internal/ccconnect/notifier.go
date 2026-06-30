package ccconnect

import (
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/chenhg5/cc-connect/core"

	"github.com/scottzx/1Agents/backend/internal/agent"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// notifier.go implements the IM side of #129: it turns task-state
// notifications from the scheduler/runner into interactive cards pushed over
// the cc-connect bridge, and routes the user's approve/reject tap back into
// the task store via the card-action interceptor.
//
// The bridge is local and in-process; cards are delivered to whichever IM
// platform (Feishu/Slack) the project's active session is bound to. When no
// session is bound yet (nobody has chatted with the project's bot), the push
// is a no-op — the board remains the source of truth.

// cardActionPrefix marks button values this interceptor owns. Layout:
//
//	task:<decision>:<base64url(workspacePath)>:<taskID>
//
// taskID is colon-free; the workspace path is base64url-encoded so paths with
// separators round-trip cleanly through the single-string callback channel.
const cardActionPrefix = "task:"

// taskNotifier pushes task-state cards and handles their callbacks.
type taskNotifier struct {
	bridge *core.BridgeServer
	store  *agent.TasksStore
}

// newTaskNotifier wires a notifier over the running bridge server and the
// shared task store, and registers the card-action interceptor so approve/
// reject taps write back. Returns nil when either dependency is missing.
func newTaskNotifier(bridge *core.BridgeServer, store *agent.TasksStore) *taskNotifier {
	if bridge == nil || store == nil {
		return nil
	}
	n := &taskNotifier{bridge: bridge, store: store}
	bridge.SetCardActionInterceptor(n.handleCardAction)
	return n
}

// Notify implements agent.TaskNotifier. It builds an approve/reject card and
// pushes it to the task's project session asynchronously (the scheduler holds
// a workspace lock while calling, so this must not block).
func (n *taskNotifier) Notify(notif agent.TaskNotification) {
	project := n.resolveProject(notif.WorkspacePath, notif.WorkspaceID)
	if project == "" {
		return
	}
	card := buildDecisionCard(notif)
	go func() {
		if err := n.bridge.SendCardToProject(project, "", card); err != nil {
			// A missing session (nobody bound the bot yet) is expected and
			// not worth an error log.
			log.Printf("[notify] IM card not delivered for task %s: %v", notif.TaskID, err)
		}
	}()
}

// resolveProject maps a workspace path/ID to the cc-connect claudecode project
// name the bridge registered for it (see runner.go's workspace→project sync).
func (n *taskNotifier) resolveProject(wsPath, wsID string) string {
	wsHandler := workspace.NewHandler()
	cfg, err := wsHandler.LoadWorkspacesConfig()
	if err != nil {
		return ""
	}
	for _, ws := range cfg.Workspaces {
		if ws.Path == wsPath || (wsID != "" && ws.ID == wsID) {
			nameOrID := ws.Name
			if nameOrID == "" {
				nameOrID = ws.ID
			}
			return CCProjectSlug(nameOrID)
		}
	}
	return ""
}

// handleCardAction is the BridgeServer card-action interceptor. It claims
// "task:" actions, applies the decision, and returns a refreshed card.
func (n *taskNotifier) handleCardAction(action, sessionKey, project string) (bool, *core.Card) {
	if !strings.HasPrefix(action, cardActionPrefix) {
		return false, nil // not ours — let the engine's built-in dispatch run
	}
	decision, wsPath, taskID, ok := parseCardAction(action)
	if !ok {
		return true, errorCard("无法解析按钮指令")
	}

	title, newStatus, applied, err := agent.ApplyIMDecision(n.store, wsPath, taskID, decision)
	if err != nil {
		log.Printf("[notify] apply IM decision failed (task=%s): %v", taskID, err)
		return true, errorCard("写回失败:" + err.Error())
	}
	if !applied {
		return true, core.NewCard().
			Title("操作已失效", "grey").
			Markdownf("**%s**", fallbackTitle(title)).
			Note("该任务状态已变化,本次决策未生效。").
			Build()
	}
	return true, resultCard(decision, title, newStatus)
}

// buildDecisionCard renders the approve/reject prompt for a notification.
func buildDecisionCard(n agent.TaskNotification) *core.Card {
	color, kindZh := decisionPresentation(n.Kind)
	approveVal := encodeCardAction(agent.IMApprove, n.WorkspacePath, n.TaskID)
	rejectVal := encodeCardAction(agent.IMReject, n.WorkspacePath, n.TaskID)

	approveLabel := "✅ 批准重排"
	if n.Kind == agent.NotifyPendingReview {
		approveLabel = "✅ 批准验收"
	}

	b := core.NewCard().
		Title(fmt.Sprintf("任务%s — 待你决策", kindZh), color).
		Markdownf("**%s** %s", taskRef(n.Number), fallbackTitle(n.Title))
	if strings.TrimSpace(n.Summary) != "" {
		b = b.Markdownf("> %s", n.Summary)
	}
	return b.Buttons(
		core.PrimaryBtn(approveLabel, approveVal),
		core.DangerBtn("⛔ 打回取消", rejectVal),
	).Build()
}

func resultCard(decision agent.IMDecision, title, newStatus string) *core.Card {
	verb := "已批准"
	color := "green"
	if decision == agent.IMReject {
		verb, color = "已打回", "red"
	}
	return core.NewCard().
		Title(verb, color).
		Markdownf("**%s**", fallbackTitle(title)).
		Note("新状态:" + newStatus).
		Build()
}

func errorCard(msg string) *core.Card {
	return core.NewCard().Title("操作失败", "red").Markdown(msg).Build()
}

func decisionPresentation(k agent.TaskNotifyKind) (color, zh string) {
	switch k {
	case agent.NotifyBlocked:
		return "orange", "受阻"
	case agent.NotifyFailed:
		return "red", "失败"
	case agent.NotifyPendingReview:
		return "blue", "待验收"
	default:
		return "grey", "状态变化"
	}
}

func taskRef(number int) string {
	if number > 0 {
		return fmt.Sprintf("#%d", number)
	}
	return "任务"
}

func fallbackTitle(title string) string {
	if strings.TrimSpace(title) == "" {
		return "(无标题任务)"
	}
	return title
}

// encodeCardAction builds a "task:" button value.
func encodeCardAction(decision agent.IMDecision, wsPath, taskID string) string {
	return fmt.Sprintf("%s%s:%s:%s", cardActionPrefix, decision,
		base64.RawURLEncoding.EncodeToString([]byte(wsPath)), taskID)
}

// parseCardAction reverses encodeCardAction.
func parseCardAction(action string) (decision agent.IMDecision, wsPath, taskID string, ok bool) {
	rest := strings.TrimPrefix(action, cardActionPrefix)
	parts := strings.SplitN(rest, ":", 3)
	if len(parts) != 3 {
		return "", "", "", false
	}
	switch agent.IMDecision(parts[0]) {
	case agent.IMApprove, agent.IMReject:
		decision = agent.IMDecision(parts[0])
	default:
		return "", "", "", false
	}
	pathBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", "", "", false
	}
	if parts[2] == "" {
		return "", "", "", false
	}
	return decision, string(pathBytes), parts[2], true
}
