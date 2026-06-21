/**
 * Wire 信封(占位 —— 文档先行,M2 实现)。
 *
 * 叶子模块:持有 happy `AgentMessage`(union) ↔ Go `WsMessage` ↔ happy-wire `SessionEnvelope`
 * 的映射。这是"Go 当大脑、Node 说 happy/wire"的契约层 —— 字段级映射,非结构性重写。
 *
 * 锚点:backend/internal/agent/acpx_client.go 的 WsMessage 已含对位字段
 * (Action / Event / Text / ToolName / ToolCallID / Arguments / Summary / AcpSessionID …),
 * 所以映射是逐字段对齐。
 *
 * 映射草表(M2 实现时补全 + golden-file 测试):
 *   happy AgentMessage.type      →  Go WsMessage.Event
 *   ─────────────────────────────────────────────────
 *   'model-output' (textDelta)   →  'text_delta'   (Text)
 *   'tool-call'                  →  'tool_call'    (ToolName, ToolCallID, Arguments)
 *   'tool-result'                →  'tool_result'  (ToolCallID, Summary)
 *   'permission-request'         →  (映射到 1Agents 权限审批事件)
 *   'status' (idle/done)         →  'done'         (Summary)
 *   'event' (session started)    →  'session_ready'(AcpSessionID)
 *   ...(其余 AgentMessage 变体见 happy agent/core/AgentMessage.ts)
 *
 * 依赖边界:只依赖**已发布的** @1agents/wire 包(非 submodule 路径),版本锁定、可与前端共享;
 *           不 import adapter 其它任何模块。
 *
 * TODO(M2):
 *   1. 引入 @1agents/wire 的 SessionEnvelope / createEnvelope。
 *   2. toWsMessage(agentMessage) / fromWsMessage(wsMessage) 双向映射。
 *   3. golden-file 测试:样本 AgentMessage → 现有前端解析器期望的精确 WsMessage JSON。
 */

export const PLACEHOLDER = true; // M1 占位,无实现。
