/**
 * App Manifest Service — typed client for GET /api/apps, POST /api/apps/{id}/enable|disable.
 *
 * The backend (Wave 2a Go agent) serves these endpoints. If the endpoint is
 * unreachable, all functions degrade gracefully (empty list / no-op).
 */

import * as wsStore from '../stores/workspaceStore';

// ── Types ────────────────────────────────────────────────────────────────────

/**
 * Product Shell placement fields (C0, design §8). All optional — mounts
 * without them keep the legacy behavior: visible in every enabled shell,
 * placed by type, declaration order, no permission gate.
 */
export interface MountPointShellFields {
    /** Product shell ids this mount contributes to; empty = every shell. */
    shells?: string[];
    /** Placement zone within a shell; empty = derived from type. */
    slot?: string;
    /** Sort order within a slot (ascending, 0 default). */
    order?: number;
    /** Permission key required to see this mount; empty = everyone. */
    permission?: string;
}

export type MountPoint =
    | ({ type: 'project-tab'; id: string; label: string; view: string } & MountPointShellFields)
    | ({ type: 'l1-page'; id: string; label: string; view: string; icon?: string } & MountPointShellFields)
    | ({ type: 'lens'; id: string; label: string; view: string; scope: 'project' | 'home' } & MountPointShellFields);

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

// Per-project config lives in the project-local file <project>/.1agents/
// project_config.json (via /api/project/local-config), sharing that file with
// other project-local prefs (e.g. tab visibility). Each writer PUTs only its own
// keys; the backend shallow-merges. Replaces the old meta.db /api/project/config
// round-trip (whose path/query styles never matched).
const localConfigUrl = (workspacePath: string) =>
    `/api/project/local-config?workspacePath=${encodeURIComponent(workspacePath)}`;

function resolveWorkspacePath(workspaceId: string): string {
    return wsStore.findWorkspaceAnyStatus(workspaceId)?.path ?? '';
}

/**
 * Fetch per-project configuration from the project-local file. Returns null on
 * error; fills defaults so the shape is always complete for consumers.
 */
export async function getProjectConfig(workspaceId: string): Promise<ProjectConfig | null> {
    const path = resolveWorkspacePath(workspaceId);
    if (!path) return null;
    try {
        const res = await fetch(localConfigUrl(path), { credentials: 'same-origin' });
        if (!res.ok) return null;
        const raw = (await res.json()) as Partial<ProjectConfig>;
        return {
            instructions: '',
            connectors: [],
            experts: [],
            skills: [],
            automation: '',
            ...raw,
            workspaceId,
        };
    } catch {
        return null;
    }
}

/**
 * Persist per-project configuration to the project-local file. Only the config
 * fields are sent (the backend merges, preserving sibling keys like hiddenTabs).
 */
export async function putProjectConfig(workspaceId: string, config: Partial<ProjectConfig>): Promise<boolean> {
    const path = resolveWorkspacePath(workspaceId);
    if (!path) return false;
    const { instructions, connectors, experts, skills, automation } = config;
    const payload = { instructions, connectors, experts, skills, automation };
    try {
        const res = await fetch(localConfigUrl(path), {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(payload),
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
