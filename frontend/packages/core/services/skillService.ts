import { apiFetch } from './apiClient';

export interface SkillRow {
    extensionId: string;
    name: string;
    description: string;
}

interface HarnessKitExtension {
    id: string;
    name: string;
    description?: string;
}

/** Global HarnessKit skills available to the assistant-create picker. */
async function listSkills(): Promise<SkillRow[]> {
    const res = await apiFetch('/harnesskit/list_extensions', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ kind: 'skill', scope_type: 'global' }),
    });
    if (!res.ok) throw new Error(await res.text());
    const rows = (await res.json()) as HarnessKitExtension[];
    return rows.map(row => ({
        extensionId: row.id,
        name: row.name,
        description: row.description ?? '',
    }));
}

export interface WorkspaceSkillStatus {
    extensionId: string;
    name: string;
    description: string;
    /** Workspace-relative native Agent path, when HarnessKit exposes one. */
    path?: string;
    state: 'active' | 'disabled';
    sourceAgent?: string;
    canUpdate: boolean;
    updateReason?: string;
}

async function listWorkspaceSkills(workspaceId: string): Promise<WorkspaceSkillStatus[]> {
    const res = await apiFetch(`/workspace/skills?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { skills?: WorkspaceSkillStatus[] };
    return data.skills ?? [];
}

export interface SkillReindexPreview {
    mode: 'in-place';
    extensionId: string;
    name: string;
    message: string;
}

export interface ReindexSkillResponse {
    ok: boolean;
    status: 'indexed';
    extensionId: string;
}

async function previewReindex(workspaceId: string, extensionId: string): Promise<SkillReindexPreview> {
    const res = await apiFetch('/workspace/push-skill?preview=1', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as SkillReindexPreview;
}

async function reindexSkill(workspaceId: string, extensionId: string): Promise<ReindexSkillResponse> {
    const res = await apiFetch('/workspace/push-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as ReindexSkillResponse;
}

export interface UpdateSkillResponse {
    ok: boolean;
    status: 'updated';
    extensionId: string;
}

async function updateSkill(workspaceId: string, extensionId: string): Promise<UpdateSkillResponse> {
    const res = await apiFetch('/workspace/pull-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as UpdateSkillResponse;
}

export interface AvailableSkill {
    extensionId: string;
    name: string;
    description: string;
    sourceAgent?: string;
    installed: boolean;
    canInstall: boolean;
    unsupportedReason?: string;
}

async function listAvailableSkills(workspaceId: string): Promise<AvailableSkill[]> {
    const res = await apiFetch(`/workspace/available-skills?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { skills?: AvailableSkill[] };
    return data.skills ?? [];
}

async function addSkill(workspaceId: string, extensionId: string): Promise<void> {
    const res = await apiFetch('/workspace/add-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
}

async function removeSkill(workspaceId: string, extensionId: string): Promise<void> {
    const res = await apiFetch('/workspace/remove-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
}

export const skillService = {
    listSkills,
    listWorkspaceSkills,
    previewReindex,
    reindexSkill,
    updateSkill,
    listAvailableSkills,
    addSkill,
    removeSkill,
};
