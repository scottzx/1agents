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

interface TauriWindow {
    __TAURI__?: {
        core: {
            invoke: <T = unknown>(cmd: string, args?: Record<string, unknown>) => Promise<T>;
        };
    };
}

const tauriCore = () => (window as unknown as TauriWindow).__TAURI__?.core;

export function isTauri(): boolean {
    return tauriCore() !== undefined;
}

export async function checkDesktopUpdate(): Promise<DesktopUpdateInfo | null> {
    const core = tauriCore();
    if (!core) return null;
    try {
        const info = await core.invoke<DesktopUpdateInfo>('check_desktop_update');
        return info;
    } catch (err) {
        if (typeof console !== 'undefined') {
            console.warn('[ota/desktop] check failed:', err);
        }
        return null;
    }
}

export async function downloadAndInstallDesktop(version: string): Promise<void> {
    const core = tauriCore();
    if (!core) return;
    try {
        await core.invoke('open_in_external_browser', {
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
