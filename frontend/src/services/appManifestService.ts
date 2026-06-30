/**
 * App Manifest Service — typed client for GET /api/apps, POST /api/apps/{id}/enable|disable.
 *
 * The backend (Wave 2a Go agent) serves these endpoints. If the endpoint is
 * unreachable, all functions degrade gracefully (empty list / no-op).
 */

// ── Types ────────────────────────────────────────────────────────────────────

export type MountPoint =
    | { type: 'project-tab'; id: string; label: string; view: string }
    | { type: 'l1-page'; id: string; label: string; view: string; icon?: string }
    | { type: 'lens'; id: string; label: string; view: string; scope: 'project' | 'home' };

export interface AppManifest {
    id: string;
    name: string;
    version: string;
    enabled: boolean;
    mountPoints: MountPoint[];
    taskTypes: string[];
    domainTables: string[];
}

interface AppsResponse {
    apps: AppManifest[];
}

// ── Service ──────────────────────────────────────────────────────────────────

/**
 * Fetch all registered app manifests from the backend.
 * Returns an empty array on any error (graceful degradation).
 */
export async function getApps(): Promise<AppManifest[]> {
    try {
        const res = await fetch('/api/apps', { credentials: 'same-origin' });
        if (!res.ok) return [];
        const data: AppsResponse = await res.json();
        return Array.isArray(data?.apps) ? data.apps : [];
    } catch {
        return [];
    }
}

/**
 * Enable an app by id. Fire-and-forget; caller should reload manifests after.
 */
export async function enableApp(id: string): Promise<boolean> {
    try {
        const res = await fetch(`/api/apps/${id}/enable`, {
            method: 'POST',
            credentials: 'same-origin',
        });
        return res.ok;
    } catch {
        return false;
    }
}

/**
 * Disable an app by id. Fire-and-forget; caller should reload manifests after.
 */
export async function disableApp(id: string): Promise<boolean> {
    try {
        const res = await fetch(`/api/apps/${id}/disable`, {
            method: 'POST',
            credentials: 'same-origin',
        });
        return res.ok;
    } catch {
        return false;
    }
}

/**
 * Fetch per-project configuration (GET /api/project/config).
 * Returns null on error.
 */
export async function getProjectConfig(workspaceId: string): Promise<ProjectConfig | null> {
    try {
        const res = await fetch(`/api/project/${workspaceId}/config`, { credentials: 'same-origin' });
        if (!res.ok) return null;
        return await res.json();
    } catch {
        return null;
    }
}

/**
 * Update per-project configuration (PUT /api/project/config).
 */
export async function putProjectConfig(workspaceId: string, config: Partial<ProjectConfig>): Promise<boolean> {
    try {
        const res = await fetch(`/api/project/${workspaceId}/config`, {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(config),
        });
        return res.ok;
    } catch {
        return false;
    }
}

export interface ProjectConfig {
    workspaceId: string;
    /** System prompt / instructions for the project's co-pilot. */
    instructions: string;
    /** IDs of connected MCP connectors. Wave 3 apps register theirs here. */
    connectors: string[];
    /** Expert/role definitions for the project. */
    experts: Expert[];
    /** Skill IDs available within this project. */
    skills: string[];
    /** Automation rules (JSON-serialized). */
    automation: string;
}

export interface Expert {
    id: string;
    name: string;
    role: string;
    instructions: string;
}
