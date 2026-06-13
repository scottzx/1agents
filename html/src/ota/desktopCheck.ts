// Desktop OTA check — only executes inside Tauri WebView (when
// window.__TAURI__ is available). Calls the Rust IPC command
// `check_desktop_update` (see src-tauri/src/updater.rs), downloads
// the installer, and opens the release page in the default browser.

import { APP_VERSION } from '../version';

interface DesktopUpdateInfo {
    available: boolean;
    current: string;
    latest: string;
    notes: string | null;
}

interface TauriGlobal {
    core: { invoke<T = unknown>(cmd: string, args?: Record<string, unknown>): Promise<T> };
}

function tauri(): TauriGlobal | undefined {
    return (window as unknown as { __TAURI__?: TauriGlobal }).__TAURI__;
}

export function isTauri(): boolean {
    return tauri() !== undefined;
}

export async function checkDesktopUpdate(): Promise<DesktopUpdateInfo | null> {
    const t = tauri();
    if (!t) return null;
    try {
        const info = await t.core.invoke<DesktopUpdateInfo>('check_desktop_update');
        return info;
    } catch (err) {
        if (typeof console !== 'undefined') {
            console.warn('[ota/desktop] check failed:', err);
        }
        return null;
    }
}

export async function downloadAndInstallDesktop(version: string): Promise<void> {
    const t = tauri();
    if (!t) return;
    try {
        await t.core.invoke('open_in_external_browser', {
            url: `https://github.com/scottzx/1Agents/releases/tag/${version}`,
        });
    } catch (err) {
        if (typeof console !== 'undefined') {
            console.error('[ota/desktop] open release page failed:', err);
        }
    }
}

export function currentVersion(): string {
    return APP_VERSION;
}
