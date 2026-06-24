package agent

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/scottzx/1Agents/backend/internal/localtoken"
	"github.com/scottzx/1Agents/backend/internal/workspace"
)

// SessionRolePM marks the in-app AI Project Manager session. See
// meta.ChatSessionRecord.Role.
const SessionRolePM = "pm"

// SessionRolePMO marks the PM session spawned from the built-in default ("对话")
// workspace — the frontend maps role 'pm' → 'pmo' there. It behaves like a PM
// but its task tools are locked to the default project. Both roles get the
// PM system prompt + tasks MCP; see isProjectManagerRole.
const SessionRolePMO = "pmo"

// isProjectManagerRole reports whether a session role should receive the PM
// system prompt and the project-locked tasks MCP server.
func isProjectManagerRole(role string) bool {
	return role == SessionRolePM || role == SessionRolePMO
}

// SessionRoleAuto marks a session created by the headless task runner for an
// AI-executed task. These run silently in the backend, so handler.list hides
// them from the sidebar; they remain resolvable by id for "查看详情" resume.
const SessionRoleAuto = "auto"

// SessionRoleExecutor and SessionRoleVerifier are the two task-bound roles of
// the #50 permission model. Both are locked to a single task_id (the tasks MCP
// confines reads/writes to it); they differ in persona and in whether they
// execute: the executor runs its assigned task, the verifier only reviews the
// submitted output and does not execute.
const (
	SessionRoleExecutor = "executor"
	SessionRoleVerifier = "verifier"
)

// isExecutorRole / isVerifierRole report the task-bound roles.
func isExecutorRole(role string) bool { return role == SessionRoleExecutor }
func isVerifierRole(role string) bool { return role == SessionRoleVerifier }

// roleTemplateName maps a session role code to its builtin role-template name.
// pm and pmo share the single "pm" template; executor/verifier map 1:1. Returns
// "" for roles with no template (general / auto).
func roleTemplateName(role string) string {
	switch {
	case isProjectManagerRole(role):
		return "pm"
	case isExecutorRole(role):
		return SessionRoleExecutor
	case isVerifierRole(role):
		return SessionRoleVerifier
	default:
		return ""
	}
}

// workspaceName resolves a workspace id to its display name, falling back to
// the id when it can't be looked up.
func (h *Handler) workspaceName(workspaceID string) string {
	wsHandler := workspace.NewHandler()
	cfg, err := wsHandler.LoadWorkspacesConfig()
	if err != nil {
		return workspaceID
	}
	for _, ws := range cfg.Workspaces {
		if ws.ID == workspaceID {
			if ws.Name != "" {
				return ws.Name
			}
			return workspaceID
		}
	}
	return workspaceID
}

// buildPMSystemPrompt returns the appended system prompt that turns a normal
// chat agent into the project's AI Project Manager. It is appended to the
// agent's own system prompt (never replaces it).
func buildPMSystemPrompt(projectName, workspaceID string) string {
	return fmt.Sprintf(`# 角色：AI 项目经理

你现在是项目「%s」的 AI 项目经理（PM）。你通过 MCP 工具「tasks」读写本项目的任务看板，所有工具都已**锁定在当前项目**——你无法、也不要尝试操作其他项目。

## 你的职责
- 与用户对话式地澄清需求：当需求模糊时，先提 1-3 个关键问题，不要凭空假设。
- 把 PRD / Epic / 一段口头需求，拆解成**粒度合适、可独立执行**的子任务，并用依赖关系表达执行顺序。
- 为每个任务写清楚 description（给执行 agent 的工作说明）和 acceptanceCriteria（可检验的验收标准）。
- 合理使用 type 区分：常规开发用 'task'，产品需求用 'requirement'，缺陷用 'bug'；概念性、方向性、暂无明确交付物的内容用 'discussion'（通过 create_discussion 落到讨论区）。
- 工作漏斗（从松到紧）：讨论（自由记录，可能不转化）→ 需求/缺陷（目标清晰、有明确交付物）→ 任务（基于需求/缺陷的可执行单元）。
- 用里程碑（milestone）规划路线图：先用 create_milestone 建好阶段（可设 targetDate 目标日期），再在 create_task / update_task 里用同名 milestone 字段把任务归入对应阶段；用 list_milestones 查看各阶段进度。

## 工具使用约定
- 拆解时按依赖顺序创建：先建被依赖的任务，拿到返回的 id，再用 dependsOn 把后续任务挂上去。
- 创建/修改后，用 list_tasks 复述你刚落库的结果，让用户确认。
- 目标清晰、有明确交付物的才用 requirement/bug/task 写进看板；纯讨论、概念性方向用 create_discussion 落到讨论区，不要硬塞成任务。
- 不要在 description / acceptanceCriteria 里编造用户没提供的细节；不确定就先问。

## 引用其它任务（GitHub 风格永久链接）
- description / acceptanceCriteria / 回复都支持 Markdown，引用任务请直接写引用记号，前端会自动渲染成可跳转链接：
  - 同一项目内：写 #编号，例如 #90。
  - 跨项目：用反引号包裹 项目名#编号，例如 `+"`项目名#90`"+`。
  - 也可以直接写完整 URL：/项目名/tasks/编号。
- 引用记号只认 #数字；想写普通的 # 文本（如版本号）请用反引号转义，例如 `+"`#2`"+`。

## 风格
简洁、务实、以终为始。先给结论和方案，再落库。中文回复（除非用户用其它语言）。

（当前项目 workspace_id=%s，仅供你理解上下文；工具已自动锁定，无需也无法手动指定项目。）`, projectName, workspaceID)
}

// buildTasksMcpServer builds the ACP stdio McpServer entry for the task tools.
// The workspace lock is the ONEAGENTS_WORKSPACE_ID env var — the tools expose
// no project parameter, so the agent is confined to workspaceID. A non-empty
// taskID additionally injects ONEAGENTS_TASK_ID, narrowing the session to a
// single task (executor scope, #50): the server then withholds the PM-only
// create/milestone tools and confines reads/writes to that task. Credentials
// (the internal token) are injected here in Go, never from a role template's
// YAML. Returns nil if the executable can't be resolved.
func (h *Handler) buildTasksMcpServer(workspaceID, taskID string) map[string]any {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		log.Printf("[agent] tasks mcpServer: cannot resolve executable: %v", err)
		return nil
	}
	env := []map[string]string{
		{"name": "ONEAGENTS_BASE_URL", "value": h.selfBaseURL},
		{"name": "ONEAGENTS_WORKSPACE_ID", "value": workspaceID},
		{"name": "ONEAGENTS_INTERNAL_TOKEN", "value": localtoken.Token},
	}
	if taskID != "" {
		env = append(env, map[string]string{"name": "ONEAGENTS_TASK_ID", "value": taskID})
	}
	return map[string]any{
		"type":    "stdio",
		"name":    "tasks",
		"command": exe,
		"args":    []string{"mcp-tasks"},
		"env":     env,
	}
}

// buildPMMcpServers builds the per-session MCP server config (a JSON array of
// one ACP stdio McpServer) for the project-locked task tools. Retained as the
// hardcoded fallback for the PM/PMO path when the role template can't be
// resolved; the template-driven path uses buildMcpServersFromRole.
func (h *Handler) buildPMMcpServers(workspaceID string) json.RawMessage {
	srv := h.buildTasksMcpServer(workspaceID, "") // PM is project-wide; no task lock
	if srv == nil {
		return nil
	}
	b, err := json.Marshal([]map[string]any{srv})
	if err != nil {
		log.Printf("[agent] PM mcpServers: marshal failed: %v", err)
		return nil
	}
	return b
}

// resolvePMRole resolves the builtin "pm" role template for a PM/PMO session
// and returns its rendered system prompt and per-session MCP config. Both the
// pm and pmo role codes map to the single "pm" template. If the template can't
// be loaded or its engine isn't available, it falls back to the hardcoded PM
// builders so PM sessions never regress (#137).
func (h *Handler) resolvePMRole(workspacePath, workspaceID string) (prompt string, mcpServers json.RawMessage) {
	projectName := h.workspaceName(workspaceID)
	if tpl, ok := LoadRoles(workspacePath).Resolve("pm"); ok && tpl.Available {
		return renderRolePrompt(tpl, projectName, workspaceID), h.buildMcpServersFromRole(tpl, workspaceID, "")
	}
	return buildPMSystemPrompt(projectName, workspaceID), h.buildPMMcpServers(workspaceID)
}

// roleInjection resolves the builtin role template for a task-bound role
// (executor/verifier, #50) and returns its rendered persona plus a tasks MCP
// locked to taskID. ok is false when the role has no template, so the caller
// falls back to the default task background. Unlike resolvePMRole this does not
// gate on engine availability — the persona and task lock apply regardless of
// which agent drives the session.
func (h *Handler) roleInjection(role, workspacePath, workspaceID, taskID string) (prompt string, mcpServers json.RawMessage, ok bool) {
	name := roleTemplateName(role)
	if name == "" {
		return "", nil, false
	}
	tpl, found := LoadRoles(workspacePath).Resolve(name)
	if !found {
		return "", nil, false
	}
	return renderRolePrompt(tpl, h.workspaceName(workspaceID), workspaceID),
		h.buildMcpServersFromRole(tpl, workspaceID, taskID), true
}

// buildMcpServersFromRole turns a role template's mcp_servers list into the
// per-session MCP server config. Each named server maps to a Go-built entry
// (credentials injected server-side). A non-empty taskID locks the task tools
// to a single task (executor scope, #50); pass "" for project-wide roles.
// Unknown names are logged and skipped. Returns nil when no server resolves.
func (h *Handler) buildMcpServersFromRole(tpl *RoleTemplate, workspaceID, taskID string) json.RawMessage {
	var servers []map[string]any
	for _, name := range tpl.McpServers {
		switch name {
		case "tasks", "mcp-tasks":
			if srv := h.buildTasksMcpServer(workspaceID, taskID); srv != nil {
				servers = append(servers, srv)
			}
		default:
			log.Printf("[agent] role %s: unknown mcp server %q (skipped)", tpl.Name, name)
		}
	}
	if len(servers) == 0 {
		return nil
	}
	b, err := json.Marshal(servers)
	if err != nil {
		log.Printf("[agent] role mcpServers: marshal failed: %v", err)
		return nil
	}
	return b
}
