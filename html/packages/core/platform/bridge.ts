// Platform bridge — the seam between platform-agnostic core code and the
// host capabilities that differ across the web workbench, the Tauri
// desktop/mobile shell, and (later) the 小程序 client.
//
// Phase 0 establishes the geometry: core/services route host-divergent
// operations (file upload, opening an external browser) through a bridge that
// `getPlatformBridge()` resolves at runtime. The web bridge is the static
// default; the Tauri bridge is lazily swapped in at boot (see
// `initPlatformBridge`) so its code + Tauri APIs never enter the web bundle.

import { WebPlatformBridge } from './web';

/** Result of saving an uploaded file on the backend. Mirrors POST /api/fs/upload. */
export interface UploadResult {
    /** Absolute path of the saved file on the host. */
    path: string;
    /** Original base name (with extension). */
    name: string;
}

export interface PlatformBridge {
    /**
     * Upload an arbitrary file to the backend, which saves it under a
     * randomized /tmp name and returns the absolute path the local agent can
     * read. The web and Tauri implementations hit the same POST endpoint.
     */
    uploadFile(file: File): Promise<UploadResult>;
    /**
     * Open a URL in the user's real external browser (not an in-app webview).
     * Web falls back to window.open; Tauri invokes the host command.
     */
    openExternal(url: string): Promise<void>;
}

let current: PlatformBridge = new WebPlatformBridge();

/** Runtime check for the Tauri (desktop/mobile) host. */
function isTauri(): boolean {
    return typeof window !== 'undefined' && !!(window as unknown as { __TAURI__?: object }).__TAURI__;
}

/**
 * Resolve the active platform bridge. Synchronous so call sites (e.g.
 * fsService.upload) stay simple. Returns the web bridge until
 * `initPlatformBridge()` has swapped in the Tauri bridge on desktop —
 * harmless for uploadFile, which is identical across hosts.
 */
export function getPlatformBridge(): PlatformBridge {
    return current;
}

/**
 * Swap in the host-specific bridge once, at app boot. Dynamically imports the
 * Tauri bridge only when running under Tauri so the desktop-only code path is
 * tree-shaken out of the web bundle. No-op on the web.
 */
export async function initPlatformBridge(): Promise<void> {
    if (!isTauri()) return;
    const { TauriPlatformBridge } = await import('./tauri');
    current = new TauriPlatformBridge();
}
