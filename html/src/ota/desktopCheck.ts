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

/** Minimal shape of the Tauri IPC bridge injected on `window` inside the WebView. */
interface TauriApi {
    core: {
        invoke: <T = unknown>(cmd: string, args?: Record<string, unknown>) => Promise<T>;
    };
}

function getTauri(): TauriApi | undefined {
    return (window as unknown as { __TAURI__?: TauriApi }).__TAURI__;
}

export function isTauri(): boolean {
    return typeof getTauri() !== 'undefined';
}

export async function checkDesktopUpdate(): Promise<DesktopUpdateInfo | null> {
    const tauri = getTauri();
    if (!tauri) return null;
    try {
        const info = await tauri.core.invoke<DesktopUpdateInfo>('check_desktop_update');
        return info;
    } catch (err) {
        if (typeof console !== 'undefined') {
            console.warn('[ota/desktop] check failed:', err);
        }
        return null;
    }
}

export async function downloadAndInstallDesktop(version: string): Promise<void> {
    const tauri = getTauri();
    if (!tauri) return;
    try {
        await tauri.core.invoke('open_in_external_browser', {
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
