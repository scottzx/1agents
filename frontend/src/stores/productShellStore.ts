/**
 * Product Shell Store — signals state for the C0 Product Shell Registry
 * (design: docs/architecture/enterprise-foundation-v1.0.0.md §8, D7).
 *
 * Shells compose UX; they do not own data. All shells share the same
 * workspaces, tasks and sessions — switching shells only changes which
 * mount points the navigation and panes render. There is one frontend
 * bundle for every shell.
 *
 * Resolution rules mirror the backend:
 *   - a mount point with an empty `shells` list renders in every enabled
 *     shell (legacy mount points keep their existing rendering);
 *   - the user's preferred shell overrides the tenant default only while
 *     the tenant keeps that shell enabled;
 *   - disabling a shell never deletes data: preferences and defaults are
 *     kept and re-apply when the shell is enabled again.
 *
 * When the shell API is unreachable (older backend), `activeShellId`
 * stays '' and all mount filtering degrades to the legacy behavior.
 */

import { signal, computed } from '@preact/signals';
import * as svc from '../services/productShellService';
import type { ProductShell } from '../services/productShellService';

// ── State ────────────────────────────────────────────────────────────────────

/** All registered product shells with their effective enabled state. */
export const productShells = signal<ProductShell[]>([]);

/** True while a shell load is in flight. */
export const shellsLoading = signal(false);

/** Tenant product profile fields. */
export const tenantDefaultShell = signal('');
export const userPreferredShell = signal('');
export const effectiveDefaultShell = signal('');

/**
 * The shell the UI currently composes. '' = no shell info (legacy mode:
 * every mount renders as before). Persisted so a manual switch survives
 * reloads; reconciled against the backend state on load.
 */
const ACTIVE_SHELL_KEY = '1agents-active-shell';

const loadPersistedActiveShell = (): string => {
    try {
        return localStorage.getItem(ACTIVE_SHELL_KEY) ?? '';
    } catch {
        return '';
    }
};

export const activeShellId = signal<string>(loadPersistedActiveShell());

const persistActiveShell = (id: string): void => {
    try {
        localStorage.setItem(ACTIVE_SHELL_KEY, id);
    } catch {
        /* non-fatal (private mode / quota) */
    }
};

// ── Stable deep links (#328) ────────────────────────────────────────────────
//
// A shell is addressable via the URL fragment `#shell=<id>`. The fragment is
// used (not a query param) on purpose: `location.search` flows into the
// terminal WebSocket URL and xterm option parsing, so a `?shell=` param would
// leak into the terminal; the fragment never reaches the server or xterm.
//
// The deep link is captured at module load (before any router/task-permalink
// cleanup can rewrite the URL) and reconciled against the backend state in
// `loadShells`. `parseShellHash` / `buildShellHash` are pure so they can be
// unit-tested without a DOM.

/** Parse a `#shell=<id>` fragment; returns '' when absent. Pure. */
export function parseShellHash(hash: string): string {
    const raw = hash.replace(/^#/, '');
    const m = raw.match(/(?:^|&)shell=([^&]*)/);
    if (!m) return '';
    try {
        return decodeURIComponent(m[1]);
    } catch {
        return m[1];
    }
}

/** Build the `#shell=<id>` fragment for an id ('' clears it). Pure. */
export function buildShellHash(id: string): string {
    return id ? `#shell=${encodeURIComponent(id)}` : '';
}

const readWindowShellHash = (): string => {
    return typeof window === 'undefined' ? '' : parseShellHash(window.location.hash);
};

/** Deep link present at boot — captured before any URL cleanup can drop it. */
const BOOT_SHELL_DEEP_LINK = readWindowShellHash();

/**
 * Mirror the active shell into the URL fragment (stable deep link) without a
 * reload. `replaceState` keeps the back button clean. No-op without a window.
 */
export const syncShellHash = (id: string): void => {
    if (typeof window === 'undefined' || typeof window.history?.replaceState !== 'function') return;
    try {
        const target = buildShellHash(id) || window.location.pathname + window.location.search;
        window.history.replaceState(null, '', target);
    } catch {
        /* non-fatal */
    }
};

// ── Derived ──────────────────────────────────────────────────────────────────

/** Shells the tenant has enabled. */
export const enabledShells = computed<ProductShell[]>(() => productShells.value.filter(s => s.enabled));

/** The active shell manifest, or null in legacy mode / unknown id. */
export const activeShell = computed<ProductShell | null>(() => {
    const id = activeShellId.value;
    if (!id) return null;
    return productShells.value.find(s => s.id === id) ?? null;
});

/**
 * Pure resolver used on load: keep the persisted active shell when it is
 * still registered and enabled; otherwise fall back to the backend's
 * effective default. Exposed for testing.
 */
export function resolveActiveShell(persisted: string, shells: ProductShell[], backendEffectiveDefault: string): string {
    if (persisted && shells.some(s => s.id === persisted && s.enabled)) {
        return persisted;
    }
    if (backendEffectiveDefault && shells.some(s => s.id === backendEffectiveDefault && s.enabled)) {
        return backendEffectiveDefault;
    }
    const firstEnabled = shells.find(s => s.enabled);
    return firstEnabled ? firstEnabled.id : '';
}

/**
 * Boot resolution with a deep link (#328): a `#shell=<id>` fragment wins when
 * it names a registered AND enabled shell; otherwise fall back to the normal
 * persisted/default resolution. Pure, exposed for testing.
 */
export function resolveBootShell(
    persisted: string,
    deepLink: string,
    shells: ProductShell[],
    backendEffectiveDefault: string
): string {
    if (deepLink && shells.some(s => s.id === deepLink && s.enabled)) {
        return deepLink;
    }
    return resolveActiveShell(persisted, shells, backendEffectiveDefault);
}

// ── Mount visibility (consumed by appManifestStore) ─────────────────────────

/**
 * Reports whether a mount point contributes to the active shell. Empty
 * `shells` (or no shell info loaded) keeps legacy rendering: visible
 * everywhere.
 */
export function mountVisibleInActiveShell(mountShells?: string[]): boolean {
    const active = activeShellId.value;
    if (!active) return true; // legacy mode / API unreachable
    if (!mountShells || mountShells.length === 0) return true; // legacy mount
    return mountShells.includes(active);
}

/**
 * Resolved placement zone of a mount: its declared slot, falling back to
 * the mount type — mirroring backend ComposeShell so legacy mounts keep
 * the zones the UI already renders.
 */
export function resolvedMountSlot(mount: { type: string; slot?: string }): string {
    return mount.slot && mount.slot.length > 0 ? mount.slot : mount.type;
}

/**
 * Stable comparator for shell-aware mount lists: by resolved slot, then
 * ascending `order` (missing order = 0) — the same (slot, order) key the
 * backend composition sorts by. Within type-filtered lists legacy mounts
 * share one slot and order 0, so their declaration order is preserved.
 */
export function compareMountPlacement<T extends { type: string; slot?: string; order?: number }>(a: T, b: T): number {
    const slotA = resolvedMountSlot(a);
    const slotB = resolvedMountSlot(b);
    if (slotA !== slotB) return slotA < slotB ? -1 : 1;
    return (a.order ?? 0) - (b.order ?? 0);
}

// ── Actions ──────────────────────────────────────────────────────────────────

/**
 * Load (or reload) shells + product profile from the backend and reconcile
 * the active shell. Graceful on failure: state is left untouched so the UI
 * degrades to legacy rendering.
 */
export const loadShells = async (): Promise<void> => {
    shellsLoading.value = true;
    try {
        const profile = await svc.getShells();
        if (!profile) return;
        productShells.value = profile.shells;
        tenantDefaultShell.value = profile.defaultShell ?? '';
        userPreferredShell.value = profile.userPreference ?? '';
        effectiveDefaultShell.value = profile.effectiveDefault ?? '';
        const resolved = resolveBootShell(
            activeShellId.value,
            BOOT_SHELL_DEEP_LINK,
            profile.shells,
            profile.effectiveDefault ?? ''
        );
        if (resolved !== activeShellId.value) {
            activeShellId.value = resolved;
            persistActiveShell(resolved);
        }
        // Keep the URL a stable deep link for the shell we actually landed on
        // (also repairs a stale/invalid `#shell=` fragment).
        syncShellHash(resolved);
    } finally {
        shellsLoading.value = false;
    }
};

/**
 * Switch the UI to another shell (client-side; the choice is persisted in
 * localStorage but does not alter the tenant profile).
 */
export const setActiveShell = (id: string): void => {
    activeShellId.value = id;
    persistActiveShell(id);
    syncShellHash(id);
};

/** Enable or disable a shell for the tenant, then refresh. */
export const toggleShell = async (id: string, enabled: boolean): Promise<boolean> => {
    const ok = enabled ? await svc.enableShell(id) : await svc.disableShell(id);
    if (ok) await loadShells();
    // loadShells reconciles activeShellId when the active shell got disabled.
    return ok;
};

/** Choose the tenant default shell, then refresh. */
export const chooseDefaultShell = async (id: string): Promise<boolean> => {
    const ok = await svc.setDefaultShell(id);
    if (ok) await loadShells();
    return ok;
};

/**
 * Choose (or clear, with '') the user's preferred shell, then refresh.
 * The preference overrides the tenant default only within the enabled
 * range — the backend enforces this at resolution time.
 */
export const choosePreferredShell = async (id: string): Promise<boolean> => {
    const ok = await svc.setPreferredShell(id);
    if (ok) await loadShells();
    return ok;
};
