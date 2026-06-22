/**
 * RelayTerminalSocket — 把终端流(ttyd)载到中转上,替代同源 `wss://…/ws`(issue #17 终端那一路)。
 *
 * 与 RelayChatSocket 同思路,但**镜像 xterm 实际用到的 WebSocket EventTarget 面**
 * (`addEventListener` + `binaryType` + `send(Uint8Array)` + `readyState`),而非聊天的
 * `onmessage=` 属性式接口——所以它 `extends EventTarget`,xterm 的 ttyd 协议逻辑一行不改。
 *
 *   Down(node → H5):节点桥把本机 ttyd 的二进制帧批量镜像成 Happy session 消息
 *                    `{ frames: [base64, ...] }`;本类订阅 `socket.on('update')`,按 happySessionId
 *                    过滤、用 machine key 解密、**逐帧** dispatch `message`(data=ArrayBuffer)给 xterm。
 *   Up(H5 → node):send() 把 xterm 发的原始 ttyd 帧(输入/resize/pause/resume/心跳)经 RPC 透传。
 *
 * 节点侧对接:adapter/terminal/terminalBridge.mjs(桥接本机 ttyd,不引入 node-pty)。
 */
import type { Socket } from 'socket.io-client';
import { decrypt, encodeBase64, decodeBase64 } from './crypto';
import { openTerminal, inputTerminal, closeTerminal, type RelayMachine, type RelayTerminalParams } from './relayClient';

// 对齐 WebSocket.{CONNECTING,OPEN,CLOSED} 数值常量。
const CONNECTING = 0;
const OPEN = 1;
const CLOSED = 3;

export class RelayTerminalSocket extends EventTarget {
    // xterm 会设置/读取这两个,保持 WebSocket 形状。
    binaryType: 'arraybuffer' | 'blob' = 'arraybuffer';
    readyState = CONNECTING;

    private happySessionId: string | null = null;
    private updateHandler: ((data: unknown) => void) | null = null;
    private disconnectHandler: (() => void) | null = null;

    constructor(
        private socket: Socket,
        private machine: RelayMachine,
        private params: RelayTerminalParams
    ) {
        super();
        // start() 在稍后的 tick 完成:此时 xterm 已同步注册好 open/message/close 监听。
        void this.start();
    }

    private async start(): Promise<void> {
        try {
            const { happySessionId } = await openTerminal(this.socket, this.machine, this.params);
            if (this.readyState === CLOSED) return; // open 期间已被关闭
            this.happySessionId = happySessionId;

            this.updateHandler = (data: unknown) => {
                // new-message payload(happy-server eventRouter.buildNewMessageUpdate):
                //   { body: { t: 'new-message', sid, message: { content: { t, c } } } }
                const body = (data as { body?: Record<string, unknown> } | undefined)?.body;
                if (!body || body.t !== 'new-message') return;
                if (body.sid !== happySessionId) return;
                const message = body.message as { content?: { t?: string; c?: string } } | undefined;
                const content = message?.content;
                if (!content || content.t !== 'encrypted' || !content.c) return;
                void this.deliver(content.c);
            };
            this.socket.on('update', this.updateHandler);

            this.disconnectHandler = () => this.fail();
            this.socket.on('disconnect', this.disconnectHandler);

            this.readyState = OPEN;
            this.dispatchEvent(new Event('open'));
        } catch {
            this.fail();
        }
    }

    private async deliver(c: string): Promise<void> {
        const obj = await decrypt(this.machine.encryptionKey, this.machine.variant, decodeBase64(c));
        if (obj === null || obj === undefined) return;
        // 桥哨兵:节点侧 ttyd WS 关了 → 结束本传输(xterm 触发重连)。
        if (typeof obj === 'object' && (obj as { event?: string }).event === '__relay_closed') {
            this.fail();
            return;
        }
        const frames = (obj as { frames?: string[] }).frames;
        if (!Array.isArray(frames)) return;
        for (const f of frames) {
            const u8 = decodeBase64(f);
            // xterm 的 onSocketData 读 event.data 当 ArrayBuffer;给独立 buffer。
            const ab =
                u8.byteOffset === 0 && u8.byteLength === u8.buffer.byteLength
                    ? u8.buffer
                    : u8.slice().buffer;
            this.dispatchEvent(new MessageEvent('message', { data: ab }));
        }
    }

    /** xterm 用 socket.send(Uint8Array|string) 发原始 ttyd 帧;透传给节点桥。 */
    send(data: string | ArrayBuffer | ArrayBufferView): void {
        if (this.readyState !== OPEN) return;
        let u8: Uint8Array;
        if (typeof data === 'string') u8 = new TextEncoder().encode(data);
        else if (data instanceof ArrayBuffer) u8 = new Uint8Array(data);
        else u8 = new Uint8Array(data.buffer, data.byteOffset, data.byteLength);
        // 不阻塞 xterm:fire-and-forget(RPC 内部有 30s ack 超时兜底)。
        void inputTerminal(this.socket, this.machine, this.params.termId, encodeBase64(u8));
    }

    close(): void {
        if (this.readyState === CLOSED) return;
        this.readyState = CLOSED;
        this.cleanup();
        void closeTerminal(this.socket, this.machine, this.params.termId);
        // 正常关闭:code 1000,xterm 不重连。
        this.dispatchEvent(new CloseEvent('close', { code: 1000 }));
    }

    /** 异常结束(open 失败 / relay 掉线 / 桥哨兵):code≠1000 让 xterm 重连。 */
    private fail(): void {
        if (this.readyState === CLOSED) return;
        this.readyState = CLOSED;
        this.cleanup();
        this.dispatchEvent(new CloseEvent('close', { code: 1006 }));
    }

    private cleanup(): void {
        if (this.updateHandler) {
            this.socket.off('update', this.updateHandler);
            this.updateHandler = null;
        }
        if (this.disconnectHandler) {
            this.socket.off('disconnect', this.disconnectHandler);
            this.disconnectHandler = null;
        }
    }
}
