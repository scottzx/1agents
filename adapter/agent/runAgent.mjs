/**
 * runAgent(占位 —— 文档先行,M2 实现)。
 *
 * 这是 happy `src/agent/acp/runAcp.ts` 的 **DI 重写**。happy 的 runAcp.ts 是唯一耦合的
 * Tier-1 文件(import `@/api`、`@/daemon`、`@/persistence`),**绝不直接 import**;
 * 改为把 session 客户端 / 权限处理器 / envelope 发送器作为依赖注入,由 1Agents 侧提供等价物
 * (Go 后端 HTTP API + relay session)。
 *
 * 流向(M2 首迁 claude,收益最大 —— 原生 stream-json 不丢特性):
 *   AgentBackend(happy Tier-1,native Claude Code)── onMessage(AgentMessage)
 *     → happy MessageAdapter 归一成 ACP 形 `ACPMessageData`(thinking/reasoning 一等公民)
 *     → wire/envelope.mjs 映射 ACPMessageData → Go WsMessage
 *     → 经 relay session new-message 扇出给 H5(复用 chat 路径)
 *
 *   ⚠️ M2 消费点订正:tap happy `MessageAdapter` 之后的 **ACP 输出**(ACPMessageData),
 *      **不可**直接消费内部 `AgentMessage` union —— 那层 model-output 是纯文本,会把 thinking
 *      降级(详见 docs/agent-convergence-roadmap.md「Wire 源以 ACP 为准」)。
 *
 * 验收(M2 闸):claude 聊天时间线与现有 acpx 路径逐字节一致(golden-file 契约测试)。
 *
 * 依赖边界:happy-cli Tier-1(agent/core、agent/transport、agent/acp/{AcpBackend,AcpSessionManager})
 *           当库 + wire/;禁止 happy runAcp.ts / @/api / @/daemon / @/persistence。
 *
 * 开放问题(见 docs/happy-integration-skeleton.md 风险段 #2/#3):
 *   - Tier-1 引入机制:file: 依赖 vs 引 happy-cli dist。
 *   - runAcp DI 面:究竟要注入什么,Go HTTP API 能否满足还是要 Node 薄 shim。
 */

export const PLACEHOLDER = true; // M1 占位,无实现。
