package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/meta"
)

// createTmpWorkspace allocates a disposable cwd, seeds lightweight agent
// guidance, and registers a real projects row (kind=tmp) so resolveWorkspacePath
// and the side pane work like any other workspace. Returns workspace id, path.
func createTmpWorkspace(sessionID, displayName string) (workspaceID, cwd string, err error) {
	cwd, err = createOneshotCwd()
	if err != nil {
		return "", "", err
	}
	workspaceID = "tmp-" + sessionID
	name := strings.TrimSpace(displayName)
	if name == "" {
		name = "单次对话"
	}
	db, err := meta.OpenDefault()
	if err != nil {
		_ = os.RemoveAll(cwd)
		return "", "", err
	}
	p := meta.Project{
		ID:            workspaceID,
		Name:          name,
		WorkspacePath: cwd,
		Kind:          meta.KindTmp,
		Status:        meta.ProjectStatusActive,
	}
	if err := db.EnsureWorkspaceProject(p); err != nil {
		_ = os.RemoveAll(cwd)
		return "", "", fmt.Errorf("register tmp workspace: %w", err)
	}
	return workspaceID, cwd, nil
}

// createOneshotCwd makes /tmp/1agents-chat/<random> for ephemeral sessions and
// seeds lightweight project config so agents treat the dir as a pure-chat
// workspace (no project skills / no project MCP by default).
func createOneshotCwd() (string, error) {
	root := filepath.Join(os.TempDir(), "1agents-chat")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, "chat-")
	if err != nil {
		return "", err
	}
	if err := seedOneshotCwd(dir); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// seedOneshotCwd writes project-level hints for lightweight dialogue:
//   - .grok/config.toml  — empty project MCP + conservative permission notes
//   - AGENTS.md          — agent-agnostic instruction file
//   - Claude.md          — Claude Code convention (same intent)
//
// The directory intentionally has no .grok/skills or .agents/skills so the
// project layer does not add skills. User-level skills/MCP may still load
// from the agent harness (~/.grok, …); project config cannot fully disable them.
func seedOneshotCwd(dir string) error {
	grokDir := filepath.Join(dir, ".grok")
	if err := os.MkdirAll(grokDir, 0o755); err != nil {
		return fmt.Errorf("mkdir .grok: %w", err)
	}
	if err := os.WriteFile(filepath.Join(grokDir, "config.toml"), []byte(oneshotGrokConfigTOML), 0o644); err != nil {
		return fmt.Errorf("write .grok/config.toml: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte(oneshotAgentsMD), 0o644); err != nil {
		return fmt.Errorf("write AGENTS.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Claude.md"), []byte(oneshotClaudeMD), 0o644); err != nil {
		return fmt.Errorf("write Claude.md: %w", err)
	}
	return nil
}

// oneshotGrokConfigTOML is seeded into disposable oneshot cwds.
// Project [mcp_servers] entries would replace user/global MCP for this tree;
// we ship none so no project MCP is registered. Skills cannot be disabled here
// ([skills] only loads from ~/.grok/config.toml).
const oneshotGrokConfigTOML = `# 1agents tmp workspace (单次对话, kind=tmp) — lightweight session seed
# Disposable cwd under /tmp/1agents-chat/. Prefer pure conversation.
#
# Project-scoped: [mcp_servers], [plugins], [permission], [mcp].
# There are intentionally NO [mcp_servers.*] tables → no project MCP servers.
# User-level MCP in ~/.grok/config.toml may still apply depending on harness merge.
#
# Skills: cannot be disabled from project config. This tree has no .grok/skills
# and no .agents/skills so the project layer adds none.

[mcp]
# Cap tool/MCP payloads if any user-level MCP still attaches.
max_output_bytes = 16384

[permission]
# Prefer confirming before tool use; lightweight chat should stay conversational.
# Actual mode may also be driven by the 1agents session permission_mode.
`

// oneshotAgentsMD is the agent-agnostic project instruction for oneshot chats.
// Use plain paths (no markdown backticks) so the Go raw string stays valid.
const oneshotAgentsMD = `# 单次对话（轻量会话）

这是 **1agents 单次对话** 会话的临时工作目录（不绑定真实项目）。

## 目标

- 头脑风暴、澄清想法、纯文字对话
- **默认不依赖项目技能与 MCP 工具**
- 不要假设这里有代码仓库、任务看板或文件资产

## 行为约定

1. **优先直接回答**，用文字讨论；不要主动跑命令、读盘、装依赖或调用工具。
2. 用户没有明确要求改文件 / 执行操作时，**不要**使用工具。
3. 若必须使用工具才能回答，先说明原因并征得同意。
4. 不要创建大型脚手架或把对话写成实现任务，除非用户明确要求。
5. 会话结束后本目录可能被清理；**不要**依赖这里的持久状态。

## 配置说明

- .grok/config.toml：项目层不注册 MCP 服务器
- 本目录无 .grok/skills / .agents/skills
- 用户级（~/.grok 等）技能仍可能被 agent 发现；请仍以「轻量对话」为准
`

// oneshotClaudeMD mirrors AGENTS.md for Claude Code / claude-agent-acp discovery.
const oneshotClaudeMD = `# 单次对话（轻量会话）

这是 **1agents 单次对话** 的临时工作目录（Claude.md / Claude Code 约定）。

## 目标

- 头脑风暴、澄清想法、纯文字对话
- **默认不加载项目技能与 MCP**
- 无真实项目上下文、无仓库假设

## 行为约定

1. 优先直接用文字回答；不要主动使用工具或终端。
2. 用户未明确要求时，不要读写文件或执行命令。
3. 需要工具时先说明并征得同意。
4. 不要把对话默认升级为实现任务。
5. 本目录为一次性 cwd，勿依赖持久化。

## 配置

- 见 .grok/config.toml（无项目 MCP）
- 无项目级 skills 目录
`
