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

// SessionRoleAuto marks a session created by the headless task runner for an
// AI-executed task. These run silently in the backend, so handler.list hides
// them from the sidebar; they remain resolvable by id for "查看详情" resume.
const SessionRoleAuto = "auto"

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
- 合理使用 type 区分：常规开发用 'task'，产品需求用 'requirement'，缺陷用 'bug'。
- 用里程碑（milestone）规划路线图：先用 create_milestone 建好阶段（可设 targetDate 目标日期），再在 create_task / update_task 里用同名 milestone 字段把任务归入对应阶段；用 list_milestones 查看各阶段进度。

## 工具使用约定
- 拆解时按依赖顺序创建：先建被依赖的任务，拿到返回的 id，再用 dependsOn 把后续任务挂上去。
- 创建/修改后，用 list_tasks 复述你刚落库的结果，让用户确认。
- 只把已确认要做的事写进看板；纯讨论不落库。
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

// buildPMMcpServers builds the per-session MCP server config (a JSON array of
// one ACP stdio McpServer) for the project-locked task tools. The lock is the
// ONEAGENTS_WORKSPACE_ID env var — the tools expose no project parameter, so
// the agent is confined to workspaceID.
func (h *Handler) buildPMMcpServers(workspaceID string) json.RawMessage {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		log.Printf("[agent] PM mcpServers: cannot resolve executable: %v", err)
		return nil
	}
	servers := []map[string]any{
		{
			"type":    "stdio",
			"name":    "tasks",
			"command": exe,
			"args":    []string{"mcp-tasks"},
			"env": []map[string]string{
				{"name": "ONEAGENTS_BASE_URL", "value": h.selfBaseURL},
				{"name": "ONEAGENTS_WORKSPACE_ID", "value": workspaceID},
				{"name": "ONEAGENTS_INTERNAL_TOKEN", "value": localtoken.Token},
			},
		},
	}
	b, err := json.Marshal(servers)
	if err != nil {
		log.Printf("[agent] PM mcpServers: marshal failed: %v", err)
		return nil
	}
	return b
}
