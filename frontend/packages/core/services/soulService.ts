import { apiFetch } from './apiClient';

/**
 * One curated persona preset surfaced in the assistant-create picker. Mirrors
 * the backend `SoulPreset` (workspace/souls.go): ref + localized title/summary +
 * full SOUL.md markdown for preview. The full library (4000+) is being ingested
 * separately — see issue #368.
 */
export interface SoulPreset {
    ref: string;
    title: string;
    summary: string;
    content: string;
}

/** List curated persona presets, localized to `lang` ("zh" | "en"). */
async function listSouls(lang: string): Promise<SoulPreset[]> {
    const res = await apiFetch(`/assistant/souls?lang=${encodeURIComponent(lang)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { souls?: SoulPreset[] };
    return data.souls ?? [];
}

/** Read an assistant's current persona (<ws>/SOUL.md); "" when blank. */
async function getWorkspaceSoul(workspaceId: string): Promise<string> {
    const res = await apiFetch(`/workspace/soul?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { content?: string };
    return data.content ?? '';
}

/** Write an assistant's persona. Empty content clears SOUL.md (blank persona). */
async function saveWorkspaceSoul(workspaceId: string, content: string): Promise<void> {
    const res = await apiFetch('/workspace/soul', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, content }),
    });
    if (!res.ok) throw new Error(await res.text());
}

/** One agent in a project's team (mirrors backend WorkspaceAgentStatus). */
export interface TeamMember {
    agentRef: string; // "shared:<name>.md" store ref
    file: string; // <name>.md
    name: string; // declared name (frontmatter) or file stem
    description: string;
    state: string; // synced | modified | local
}

export interface WorkspaceTeam {
    primary: string; // <name>.md that drives the default conversation; "" = none
    members: TeamMember[];
}

/** Read a project's agent-team manifest (primary + roster). */
async function getWorkspaceTeam(workspaceId: string): Promise<WorkspaceTeam> {
    const res = await apiFetch(`/workspace/team?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { primary?: string; members?: TeamMember[] };
    return { primary: data.primary ?? '', members: data.members ?? [] };
}

/** Set the project's primary agent (drives the default conversation). "" clears it. */
async function setWorkspacePrimary(workspaceId: string, primary: string): Promise<void> {
    const res = await apiFetch('/workspace/team', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, primary }),
    });
    if (!res.ok) throw new Error(await res.text());
}

/** One agent in the shared store (母体) offered in the project add-member picker. */
export interface AvailableAgent {
    agentRef: string; // "shared:<name>.md"
    file: string; // <name>.md
    name: string;
    description: string;
    installed: boolean; // already in this workspace's .claude/agents
}

/** List 母体 agents the project can add as team members. */
async function listAvailableAgents(workspaceId: string): Promise<AvailableAgent[]> {
    const res = await apiFetch(`/workspace/available-agents?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { agents?: AvailableAgent[] };
    return data.agents ?? [];
}

/** Materialize a 母体 agent into the workspace's .claude/agents. */
async function addAgent(workspaceId: string, agentRef: string): Promise<void> {
    const res = await apiFetch('/workspace/add-agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, agentRef }),
    });
    if (!res.ok) throw new Error(await res.text());
}

/** Delete an agent from the workspace's .claude/agents (母体 untouched). */
async function removeAgent(workspaceId: string, agentRef: string): Promise<void> {
    const res = await apiFetch('/workspace/remove-agent', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, agentRef }),
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
