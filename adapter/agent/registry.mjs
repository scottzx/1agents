/**
 * Agent 注册表(占位 —— 文档先行,M3 实现)。
 *
 * 收敛目标:用 happy 的 `AgentRegistry`(每 agent 选最佳传输)替代 Go 后端
 * backend/internal/agent/catalog.go 的静态 AgentInfo 表 + acpx「一切皆 ACP」。
 *
 *   happy 混合传输(消费 modules/happy-cli Tier-1):
 *     claude   → 原生 Claude Code CLI(stream-json,功能最全,非 ACP)
 *     codex    → v2 JSON-RPC AppServer(非 ACP)
 *     gemini   → ACP(--experimental-acp,happy AcpBackend + Zed @agentclientprotocol/sdk)
 *     openclaw → 私有 WS
 *     devin    → ACP
 *
 * catalog.go 当前双角色(传输路由 + 安装元数据)须拆开:registry 接管传输选择,
 * catalog 缩成 label/安装命令等 UI 元数据。详见 docs/agent-convergence-roadmap.md(M3)。
 *
 * 依赖边界:消费 happy-cli Tier-1(agent/core/AgentRegistry 等)当库;不引 happy 的 runAcp.ts。
 *
 * TODO(M3):从 modules/happy-cli 引入 Tier-1 AgentRegistry,按 agentType 注册 factory。
 */

export const PLACEHOLDER = true; // M1 占位,无实现。
