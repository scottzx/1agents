/**
 * Wire 信封 —— happy `ACPMessageData`(ACP 形 App 线协议)↔ Go `WsMessage` 的字段级映射。
 *
 * 叶子模块(零依赖)。这是「Go 当大脑、Node 说 ACP」的契约层:M2 的 runAgent 把 happy
 * `MessageAdapter` 归一后的 **ACP 输出**(`sendAgentMessage(provider, body: ACPMessageData)`
 * 携带的 `ACPMessageData`)经 `toWsMessage` 翻成现网前端 reducer 期望的精确 `WsMessage` JSON,
 * 复用 chat 扇出路径;反向 `fromWsMessage` 把 agent 输出事件翻回 `ACPMessageData`(可逆子集)。
 *
 * ⭐ 为什么 FROM 源是 ACPMessageData 而非 happy 内部 `AgentMessage` union:
 *    happy 内部 `AgentMessage` 的 `model-output` 只有 textDelta/fullText —— **没有 thinking 通道**,
 *    thinking 在那层被降级成 generic `EventMessage{name:'thinking'}`。而 happy 真正的 App 线协议
 *    `ACPMessageData` 是 **ACP 形、thinking/reasoning 一等公民**(`src/api/apiSession.ts` 注释:
 *    "the unified format for all agent messages - CLI adapts each provider's format to ACP")。
 *    thinking 是重要字段,以 ACP 为准 —— 所以映射的 FROM 源取 ACPMessageData,thinking 保真直达
 *    `text_delta type:'thought'`,绝不降级。(详见 docs/agent-convergence-roadmap.md「Wire 源以 ACP 为准」。)
 *
 * 两端权威来源(都已落地,本映射逐字段对齐,非结构重写):
 *   FROM  happy `src/api/apiSession.ts` 的 `ACPMessageData` union(submodule pin 504890d)。
 *   TO    Go `backend/internal/agent/acpx_client.go` 的 `WsMessage` 结构 + 前端消费方
 *         `html/src/core/protocol/wireProtocol.ts`(BridgeEvent / BridgeEventPayload)、
 *         `reducer.ts`(每事件实际读取的字段;`applyTextDelta` 见 type==='thought' 渲染 ThinkingBubble)、
 *         `components/chat/hooks.ts`(事件分发)。
 *
 * ⚠️ 验收边界(诚实声明):本文件的 golden fixture 编码的是**从上述两份类型定义推导的契约**,
 *    **不是**从现网 acpx 链路抓取的录制。#180 的 M2 闸(以现网 acpx 路径产出为基线、逐字节
 *    对拍)仍未完成 —— 那需要跑起后端录基线后,把这里的 golden 替换/校验为真实抓包。
 *    见 docs/agent-convergence-roadmap.md「迁移重点」。
 */

// ── helpers ─────────────────────────────────────────────────────────────────

/** tool-result.output 是 unknown:字符串原样,其余 JSON 串化(前端 reducer 读 `text`)。 */
function stringifyResult(result) {
    if (typeof result === 'string') return result;
    if (result == null) return '';
    return JSON.stringify(result);
}

/** 丢弃 undefined 字段,得到与 Go `omitempty` 序列化一致的紧凑对象(golden 对拍前先归一)。 */
function compact(obj) {
    const out = {};
    for (const [k, v] of Object.entries(obj)) {
        if (v !== undefined) out[k] = v;
    }
    return out;
}

// ── happy ACPMessageData → Go WsMessage ───────────────────────────────────────

/**
 * 把一条 happy `ACPMessageData` 映射成前端 reducer 消费的 `WsMessage`。
 * 无对应前端事件的变体(task_started、file-edit、terminal-output、token_count 等)
 * 返回 `null` —— 调用方据此跳过扇出。
 *
 * @param {object} msg ACPMessageData(带 `type` 判别符)
 * @returns {object|null} WsMessage(已 compact)或 null
 */
export function toWsMessage(msg) {
    if (!msg || typeof msg !== 'object') return null;

    switch (msg.type) {
        case 'message': {
            // 普通模型输出:不带 type(前端 applyTextDelta 默认按 'output' 处理)。
            if (!msg.message) return null;
            return compact({ event: 'text_delta', text: msg.message });
        }

        // ⭐ thinking / reasoning 都是 ACP 形的「思考」通道 —— 一律 text_delta type:'thought',
        //    前端据 type==='thought' 渲染独立的 ThinkingBubble(reducer.applyTextDelta)。
        case 'reasoning': {
            if (!msg.message) return null;
            return compact({ event: 'text_delta', text: msg.message, type: 'thought' });
        }
        case 'thinking': {
            if (!msg.text) return null;
            return compact({ event: 'text_delta', text: msg.text, type: 'thought' });
        }

        case 'tool-call':
            return compact({
                event: 'tool_call',
                toolName: msg.name,
                toolCallId: msg.callId,
                arguments: msg.input,
            });

        case 'tool-result':
            return compact({
                event: 'tool_result',
                toolCallId: msg.callId,
                text: stringifyResult(msg.output),
                isError: !!msg.isError,
            });

        case 'permission-request':
            // reducer.applyPermissionRequest 读 arguments / requestId / toolName / toolCallId。
            // ACPMessageData 无 toolCallId(权限按 permissionId 归一);options 即可渲染的输入。
            return compact({
                event: 'permission_request',
                requestId: msg.permissionId,
                toolName: msg.toolName,
                message: msg.description,
                arguments: msg.options,
            });

        // 回合生命周期:complete / aborted 都翻成前端的回合结束信号 'done'。
        case 'task_complete':
        case 'turn_aborted':
            return compact({ event: 'done' });

        // 无 acpx 链路对应前端事件的变体:前端 turnStarted 客户端置位 / 暂不渲染。
        case 'task_started':
        case 'file-edit':
        case 'terminal-output':
        case 'token_count':
        default:
            return null;
    }
}

// ── Go WsMessage → happy ACPMessageData ───────────────────────────────────────

/**
 * 反向映射:把一条 agent 输出事件 `WsMessage` 翻回 happy `ACPMessageData`。仅覆盖**可逆子集**
 * (供历史归一 / round-trip)。入站控制动作(prompt / respond_permission)与无对应的事件返回
 * `null` —— ACPMessageData 是 agent→app 的**输出**格式,无 permission-response 变体(权限应答
 * 走 RPC,不在此层)。
 *
 * 注:`text_delta type:'thought'` 逆向取 `thinking`(thinking/reasoning 正向都到 thought,逆向不 1:1)。
 *
 * @param {object} ws WsMessage(含 `event`)
 * @returns {object|null} ACPMessageData 或 null
 */
export function fromWsMessage(ws) {
    if (!ws || typeof ws !== 'object') return null;

    switch (ws.event) {
        case 'text_delta':
            return ws.type === 'thought'
                ? { type: 'thinking', text: ws.text }
                : { type: 'message', message: ws.text };
        case 'tool_call':
            return { type: 'tool-call', name: ws.toolName, input: ws.arguments, callId: ws.toolCallId };
        case 'tool_result':
            return { type: 'tool-result', output: ws.text, callId: ws.toolCallId, isError: !!ws.isError };
        case 'permission_request':
            return {
                type: 'permission-request',
                permissionId: ws.requestId,
                toolName: ws.toolName,
                description: ws.message,
                options: ws.arguments,
            };
        case 'done':
            return { type: 'task_complete' };
        default:
            return null;
    }
}
