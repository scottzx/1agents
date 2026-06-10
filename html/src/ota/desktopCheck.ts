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

export function isTauri(): boolean {
    return typeof (window as any).__TAURI__ !== 'undefined';
}

export async function checkDesktopUpdate(): Promise<DesktopUpdateInfo | null> {
    if (!isTauri()) return null;
    try {
        const { invoke } = (window as any).__TAURI__.core;
        const info: DesktopUpdateInfo = await invoke('check_desktop_update');
        return info;
    } catch (err) {
        if (typeof console !== 'undefined') {
            console.warn('[ota/desktop] check failed:', err);
        }
        return null;
    }
}

export async function downloadAndInstallDesktop(version: string): Promise<void> {
    if (!isTauri()) return;
    try {
        const { invoke } = (window as any).__TAURI__.core;
        await invoke('open_in_external_browser', {
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
