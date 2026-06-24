// Web (browser) platform bridge — the default host.

import type { PlatformBridge, PlatformStorage, UploadResult } from './bridge';
import type { ConnectSocketOptions, PlatformSocket, SocketCloseInfo, SocketReadyState } from './socket';

/** localStorage-backed PlatformStorage (web/Tauri). */
const webStorage: PlatformStorage = {
    get: key => localStorage.getItem(key),
    set: (key, value) => localStorage.setItem(key, value),
    remove: key => localStorage.removeItem(key),
};

export class WebPlatformBridge implements PlatformBridge {
    readonly storage = webStorage;
    /**
     * Upload via the same multipart POST the workbench has always used.
     * Kept byte-for-byte identical to the original fsService.upload so the
     * web path is unchanged by the bridge indirection.
     */
    async uploadFile(file: File): Promise<UploadResult> {
        const fd = new FormData();
        fd.append('file', file);
        const res = await fetch('/api/fs/upload', { method: 'POST', body: fd });
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    }

    async openExternal(url: string): Promise<void> {
        window.open(url, '_blank', 'noopener');
    }

    connectSocket(url: string, opts?: ConnectSocketOptions): PlatformSocket {
        return new BrowserSocket(url, opts);
    }

    /** Global fetch — preserves same-origin relative URLs and any fetch wrapper. */
    httpFetch(url: string, init?: RequestInit): Promise<Response> {
        return fetch(url, init);
    }
}

// Re-export so the Tauri bridge can share the web storage impl.
export { webStorage };

/**
 * `PlatformSocket` over the browser `WebSocket`. Buffers `send` until the
 * connection opens so callers needn't await `onOpen`. The browser constructor
 * can't set headers, so `opts.headers` is ignored (web is same-origin anyway).
 */
export class BrowserSocket implements PlatformSocket {
    private ws: WebSocket;
    private outbox: string[] = [];

    constructor(url: string, opts?: ConnectSocketOptions) {
        this.ws = new WebSocket(url, opts?.protocols);
        this.ws.addEventListener('open', () => {
            for (const msg of this.outbox) this.ws.send(msg);
            this.outbox = [];
        });
    }

    get readyState(): SocketReadyState {
        switch (this.ws.readyState) {
            case WebSocket.CONNECTING:
                return 'connecting';
            case WebSocket.OPEN:
                return 'open';
            case WebSocket.CLOSING:
                return 'closing';
            default:
                return 'closed';
        }
    }

    send(data: string): void {
        if (this.ws.readyState === WebSocket.OPEN) {
            this.ws.send(data);
        } else {
            this.outbox.push(data);
        }
    }

    close(code?: number, reason?: string): void {
        this.ws.close(code, reason);
    }

    onOpen(cb: () => void): void {
        this.ws.addEventListener('open', () => cb());
    }

    onMessage(cb: (data: string) => void): void {
        this.ws.addEventListener('message', ev => cb(typeof ev.data === 'string' ? ev.data : String(ev.data)));
    }

    onClose(cb: (info: SocketCloseInfo) => void): void {
        this.ws.addEventListener('close', ev => cb({ code: ev.code, reason: ev.reason }));
    }

    onError(cb: (err: unknown) => void): void {
        this.ws.addEventListener('error', ev => cb(ev));
    }
}
