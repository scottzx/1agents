// Platform-agnostic WebSocket transport seam.
//
// The web/Tauri hosts use the browser `WebSocket`; the 小程序 (Taro) host has
// no `WebSocket` and must use `Taro.connectSocket`. Core code that needs a
// realtime channel (relay, chat, terminal) talks to this minimal interface and
// lets the active PlatformBridge supply the concrete socket — so the transport
// "switches to Taro.connectSocket" on weapp without core importing Taro.

export type SocketReadyState = 'connecting' | 'open' | 'closing' | 'closed';

/** Connection-close details, normalized across browser WebSocket and Taro. */
export interface SocketCloseInfo {
    code?: number;
    reason?: string;
}

/**
 * Minimal realtime socket. Implementations buffer `send` until the connection
 * is open, so callers may send immediately after `connectSocket` without
 * waiting for `onOpen`. Text frames only (JSON) for now.
 */
export interface PlatformSocket {
    readonly readyState: SocketReadyState;
    send(data: string): void;
    close(code?: number, reason?: string): void;
    onOpen(cb: () => void): void;
    onMessage(cb: (data: string) => void): void;
    onClose(cb: (info: SocketCloseInfo) => void): void;
    onError(cb: (err: unknown) => void): void;
}

export interface ConnectSocketOptions {
    /** Sub-protocols, passed through to the underlying socket. */
    protocols?: string[];
    /**
     * Extra request headers. Honored by `Taro.connectSocket`; the browser
     * `WebSocket` constructor cannot set headers, so the web impl ignores them.
     */
    headers?: Record<string, string>;
}
