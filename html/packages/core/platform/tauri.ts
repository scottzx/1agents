// Tauri (desktop + mobile) platform bridge.
//
// Uses the same `window.__TAURI__.core.invoke` access the rest of the app
// already relies on (see components/browser/BuiltinBrowser.tsx and
// ota/desktopCheck.ts) rather than the @tauri-apps/* npm packages, keeping the
// dependency surface unchanged. Only loaded under Tauri (dynamic import in
// bridge.ts), so it never enters the web bundle.

import type { PlatformBridge, UploadResult } from './bridge';

interface TauriCore {
    invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown>;
}

function tauriCore(): TauriCore | undefined {
    return (window as unknown as { __TAURI__?: { core?: TauriCore } }).__TAURI__?.core;
}

export class TauriPlatformBridge implements PlatformBridge {
    /**
     * Upload through the same backend endpoint as the web host — the desktop
     * shell serves the same 1agents backend, so the multipart POST is identical.
     */
    async uploadFile(file: File): Promise<UploadResult> {
        const fd = new FormData();
        fd.append('file', file);
        const res = await fetch('/api/fs/upload', { method: 'POST', body: fd });
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    }

    /** Reuse the host's `open_in_external_browser` command (see desktopCheck.ts). */
    async openExternal(url: string): Promise<void> {
        const core = tauriCore();
        if (!core) {
            window.open(url, '_blank', 'noopener');
            return;
        }
        await core.invoke('open_in_external_browser', { url });
    }
}
