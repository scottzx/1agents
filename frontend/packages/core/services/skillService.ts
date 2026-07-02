import { apiFetch } from './apiClient';

/**
 * Minimal skill row shape used by the assistant-create picker. Mirrors a subset
 * of the 1skills FastAPI `SkillsPageResponse.rows[*]` schema (skillRef / name /
 * description). Kept intentionally narrow — the picker only needs enough to
 * label a checkbox and send the ref back on create.
 */
export interface SkillRow {
    skillRef: string;
    name: string;
    description: string;
}

/**
 * List installed skills from the 1skills microservice (Go reverse-proxies
 * `/api/skills` → python skill-manager). Returns the flat rows array; callers
 * that don't care about harness columns/summary just consume names.
 */
async function listSkills(): Promise<SkillRow[]> {
    const res = await apiFetch('/skills');
    if (!res.ok) throw new Error(await res.text());
    const page = (await res.json()) as { rows?: SkillRow[] };
    return page.rows ?? [];
}

/**
 * One skill materialized in an assistant's workspace. `state` is its relationship
 * to the 母体 shared store: 'synced' (identical), 'modified' (drifted → push
 * overwrites), or 'local' (custom skill not in the store → push creates it).
 * name/description come from the copy's SKILL.md frontmatter. `dir` is the package
 * directory; `skillRef` is the store ref ("shared:<dir>").
 */
export interface WorkspaceSkillStatus {
    skillRef: string;
    dir: string;
    name: string;
    description: string;
    state: 'synced' | 'modified' | 'local';
}

/**
 * List the skills synced into an assistant's workspace (<ws>/.claude/skills) with
 * their drift status against the shared store. Drives the assistant detail page's
 * skills section.
 */
async function listWorkspaceSkills(workspaceId: string): Promise<WorkspaceSkillStatus[]> {
    const res = await apiFetch(`/workspace/skills?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { skills?: WorkspaceSkillStatus[] };
    return data.skills ?? [];
}

/**
 * Push a workspace's own edited copy of a skill back to the 1skills shared store
 * (母体) as the new baseline — the reverse of the create-time weak-copy. The Go
 * host resolves `<ws>/.claude/skills/<dir>` and forwards to the store, which
 * no-ops when the copy is unchanged. Returns whether the baseline actually moved.
 */
async function pushSkill(workspaceId: string, skillRef: string): Promise<{ changed: boolean; created: boolean }> {
    const res = await apiFetch('/workspace/push-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as { changed: boolean; created: boolean };
}

export const skillService = { listSkills, listWorkspaceSkills, pushSkill };
