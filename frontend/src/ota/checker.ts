// Over-the-air update checker.
//
// Calls /api/ota/manifest and decides whether the running frontend is
// behind the latest published version. Results are throttled (default
// 6h) and per-version "dismissed" state is persisted in localStorage
// so we don't pester the user about the same release twice.

import { APP_VERSION, isNewer } from '../version';

// ── Throttle + dismissal keys ───────────────────────────────────────────────
const LAST_CHECK_KEY = '1agents-ota-last-check';
const DISMISS_KEY = '1agents-ota-dismissed-version';
const THROTTLE_MS = 6 * 60 * 60 * 1000; // 6 hours

// ── Manifest schema (mirrors backend/internal/server ota response) ───────────
export interface PlatformBinary {
    url: string;
    size: number;
    sha256: string;
}

export interface RootManifest {
    channel: string;
    released_at: string;
    min_supported: string;
    components: {
        frontend: {
            version: string;
            entry: string;
            integrity: string;
        };
        backend: {
            version: string;
            platforms: Record<string, PlatformBinary>;
        };
    };
    previous: Array<{ version: string; url: string }>;
}

export interface UpdateInfo {
    hasUpdate: boolean;
    current: string;
    latest: string;
    manifest: RootManifest | null;
}

// ── Storage helpers ─────────────────────────────────────────────────────────
function readLastCheck(): number {
    if (typeof localStorage === 'undefined') return 0;
    const raw = localStorage.getItem(LAST_CHECK_KEY);
    const n = raw ? parseInt(raw, 10) : 0;
    return Number.isFinite(n) ? n : 0;
}

function writeLastCheck(ts: number): void {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(LAST_CHECK_KEY, String(ts));
}

export function isDismissed(latest: string): boolean {
    if (typeof localStorage === 'undefined') return false;
    return localStorage.getItem(DISMISS_KEY) === latest;
}

export function dismiss(latest: string): void {
    if (typeof localStorage === 'undefined') return;
    localStorage.setItem(DISMISS_KEY, latest);
}

// ── Public API ──────────────────────────────────────────────────────────────
export function isThrottled(): boolean {
    return Date.now() - readLastCheck() < THROTTLE_MS;
}

export async function fetchManifest(): Promise<RootManifest> {
    const res = await fetch('/api/ota/manifest', {
        // Don't let the browser cache manifests aggressively; we rely on
        // our own throttle + the server's Cache-Control header instead.
        cache: 'no-store',
    });
    if (!res.ok) throw new Error(`OTA manifest fetch failed: ${res.status}`);
    return res.json();
}

export async function check(): Promise<UpdateInfo> {
    if (isThrottled()) {
        return { hasUpdate: false, current: APP_VERSION, latest: APP_VERSION, manifest: null };
    }
    writeLastCheck(Date.now());

    let manifest: RootManifest;
    try {
        manifest = await fetchManifest();
    } catch (err) {
        // Soft-fail: missing manifest endpoint (e.g. older backend) is
        // not a user-facing error. We log and return no-update so the
        // rest of the app keeps working.
        if (typeof console !== 'undefined') {
            console.warn('[ota] manifest check failed:', err);
        }
        return { hasUpdate: false, current: APP_VERSION, latest: APP_VERSION, manifest: null };
    }

    const latest = manifest.components?.frontend?.version ?? '';
    const hasUpdate = !!latest && isNewer(latest, APP_VERSION) && !isDismissed(latest);

    return {
        hasUpdate,
        current: APP_VERSION,
        latest,
        manifest,
    };
}
