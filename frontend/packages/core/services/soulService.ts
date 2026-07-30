import { apiFetch } from './apiClient';

export interface SoulPreset {
    ref: string;
    title: string;
    summary: string;
    content: string;
}

async function listSouls(lang: string): Promise<SoulPreset[]> {
    const res = await apiFetch(`/assistant/souls?lang=${encodeURIComponent(lang)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { souls?: SoulPreset[] };
    return data.souls ?? [];
}

async function getWorkspaceSoul(workspaceId: string): Promise<string> {
    const res = await apiFetch(`/workspace/soul?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { content?: string };
    return data.content ?? '';
}

async function saveWorkspaceSoul(workspaceId: string, content: string): Promise<void> {
    const res = await apiFetch('/workspace/soul', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, content }),
    });
    if (!res.ok) throw new Error(await res.text());
}

export interface TeamMember {
    extensionId: string;
    file: string;
    name: string;
    description: string;
    state: 'active' | 'disabled';
    sourceAgent?: string;
    canUpdate: boolean;
    updateReason?: string;
}

export interface WorkspaceTeam {
    primary: string;
    members: TeamMember[];
}

async function getWorkspaceTeam(workspaceId: string): Promise<WorkspaceTeam> {
    const res = await apiFetch(`/workspace/team?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { primary?: string; members?: TeamMember[] };
    return { primary: data.primary ?? '', members: data.members ?? [] };
}

async function setWorkspacePrimary(workspaceId: string, primary: string): Promise<void> {
    const res = await apiFetch('/workspace/team', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, primary }),
    });
    if (!res.ok) throw new Error(await res.text());
}

export interface AvailableAgent {
    extensionId: string;
    name: string;
    description: string;
    sourceAgent?: string;
    installed: boolean;
    canInstall: boolean;
    unsupportedReason?: string;
}

async function listAvailableAgents(workspaceId: string): Promise<AvailableAgent[]> {
    const res = await apiFetch(`/workspace/available-agents?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { agents?: AvailableAgent[] };
    return data.agents ?? [];
}

async function addAgent(workspaceId: string, extensionId: string): Promise<void> {
    const res = await apiFetch('/workspace/add-agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
}

async function removeAgent(workspaceId: string, extensionId: string): Promise<void> {
    const res = await apiFetch('/workspace/remove-agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, extensionId }),
    });
    if (!res.ok) throw new Error(await res.text());
}

export const soulService = {
    listSouls,
    getWorkspaceSoul,
    saveWorkspaceSoul,
    getWorkspaceTeam,
    setWorkspacePrimary,
    listAvailableAgents,
    addAgent,
    removeAgent,
};
