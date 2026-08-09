/**
 * Product Shell Service — typed client for the C0 Product Shell Registry
 * (design: docs/architecture/enterprise-foundation-v1.0.0.md §8, D7).
 *
 * Shells are UX composition layers, not data layers: the three built-in
 * shells (personal / presales / commerce) share the same tasks, sessions
 * and permission facts. Endpoints:
 *
 *   GET  /api/shells                       → shells + product profile
 *   POST /api/shells/{id}/enable|disable   → tenant enable/disable (flag only)
 *   PUT  /api/shells/default               → tenant default shell
 *   PUT  /api/shells/preference            → user preferred shell (may be "")
 *   GET  /api/shells/composition?shell=<id>
 */

// ── Types ────────────────────────────────────────────────────────────────────

export const SHELL_IDS = {
    personal: 'personal',
    presales: 'presales',
    commerce: 'commerce',
} as const;

export type ShellId = (typeof SHELL_IDS)[keyof typeof SHELL_IDS];

export interface ProductShell {
    id: string;
    name: string;
    version: string;
    description?: string;
    icon?: string;
    /** Effective state: manifest default merged with the tenant's intent. */
    enabled: boolean;
}

export interface ShellProfileResponse {
    shells: ProductShell[];
    tenant: string;
    /** Tenant default shell ("" when unset). */
    defaultShell: string;
    /** The requesting user's stored preference ("" when unset). */
    userPreference: string;
    /** The shell the user lands on: preference (if allowed) → default → first enabled. */
    effectiveDefault: string;
}

export interface ComposedMount {
    appId: string;
    /** Resolved placement zone (declared slot, or the mount type). */
    slot: string;
    mount: {
        type: string;
        id: string;
        label: string;
        view: string;
        icon?: string;
        scope?: string;
        shells?: string[];
        slot?: string;
        order?: number;
        permission?: string;
    };
}

// ── Service ──────────────────────────────────────────────────────────────────

/**
 * Fetch shells + product profile. Returns null on error (graceful
 * degradation: callers fall back to the legacy single-shell rendering).
 */
export async function getShells(): Promise<ShellProfileResponse | null> {
    try {
        const res = await fetch('/api/shells', { credentials: 'same-origin' });
        if (!res.ok) return null;
        const data = await res.json();
        return Array.isArray(data?.shells) ? (data as ShellProfileResponse) : null;
    } catch {
        return null;
    }
}

/** Enable a shell for the tenant (POST /api/shells/{id}/enable). */
export async function enableShell(id: string): Promise<boolean> {
    try {
        const res = await fetch(`/api/shells/${id}/enable`, {
            method: 'POST',
            credentials: 'same-origin',
        });
        return res.ok;
    } catch {
        return false;
    }
}

/** Disable a shell for the tenant — flips a flag only, never deletes data. */
export async function disableShell(id: string): Promise<boolean> {
    try {
        const res = await fetch(`/api/shells/${id}/disable`, {
            method: 'POST',
            credentials: 'same-origin',
        });
        return res.ok;
    } catch {
        return false;
    }
}

/** Set the tenant default shell (must be registered and enabled). */
export async function setDefaultShell(id: string): Promise<boolean> {
    try {
        const res = await fetch('/api/shells/default', {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ shell: id }),
        });
        return res.ok;
    } catch {
        return false;
    }
}

/**
 * Set (or clear, with "") the user's preferred shell. The preference
 * overrides the tenant default only while the shell stays tenant-enabled.
 */
export async function setPreferredShell(id: string): Promise<boolean> {
    try {
        const res = await fetch('/api/shells/preference', {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ shell: id }),
        });
        return res.ok;
    } catch {
        return false;
    }
}

/** Fetch the composed mount points of one shell (shell/slot/order applied). */
export async function getShellComposition(shellId: string): Promise<ComposedMount[]> {
    try {
        const res = await fetch(`/api/shells/composition?shell=${encodeURIComponent(shellId)}`, {
            credentials: 'same-origin',
        });
        if (!res.ok) return [];
        const data = await res.json();
        return Array.isArray(data?.mounts) ? (data.mounts as ComposedMount[]) : [];
    } catch {
        return [];
    }
}
