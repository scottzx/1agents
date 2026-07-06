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
    state: 'synced' | 'modified' | 'local' | 'update-available';
    // Store package's version counter, bumped on every content-changing push;
    // 0 when the skill isn't in the store yet.
    version: number;
    primaryTag?: string | null;
    secondaryTag?: string | null;
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
 * Details of a concurrent-edit conflict detected by the store on push: the
 * workspace's copy was based on an older store version than what's currently
 * there. Nothing is written until the caller resolves via resolvePush.
 */
export interface SkillPushConflict {
    id: string;
    name: string;
    storeVersion: number;
    baseVersion: number;
    sourcePath: string;
}

/** Response shape for `pushSkill` (Issue #379: conflict is optional, only set on status === 'conflict'). */
export interface PushSkillResponse {
    ok: boolean;
    status: 'updated' | 'exists' | 'created' | 'conflict';
    changed: boolean;
    created: boolean;
    version: number;
    id: string;
    conflict?: SkillPushConflict;
}

/**
 * Push a workspace's own edited copy of a skill back to the 1skills shared store
 * (母体) as the new baseline — the reverse of the create-time weak-copy. The Go
 * host resolves `<ws>/.claude/skills/<dir>` and forwards to the store, which
 * no-ops when the copy is unchanged. When the store has moved on since the
 * workspace's copy was based (concurrent edit), nothing is written and the
 * response carries `status: 'conflict'` + `conflict` details for the caller to
 * resolve via `resolvePush`.
 */
async function pushSkill(workspaceId: string, skillRef: string): Promise<PushSkillResponse> {
    const res = await apiFetch('/workspace/push-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as PushSkillResponse;
}

/** Response shape for `pullSkill`: fast-forwards the workspace copy to 母体's
 * current version. `dirty` means the workspace copy has local edits, so the
 * store refused to overwrite it (nothing written). */
export interface PullSkillResponse {
    status: 'pulled' | 'uptodate' | 'dirty';
    version: number;
}

/**
 * Pull the shared store's (母体) current version of a skill into a workspace's
 * own copy — the reverse of `pushSkill`. The Go host resolves
 * `<ws>/.claude/skills/<dir>` and forwards to the store's pull-to-path
 * endpoint, which refuses (status: 'dirty') when the workspace copy has local
 * edits rather than overwriting them.
 */
async function pullSkill(workspaceId: string, skillRef: string): Promise<PullSkillResponse> {
    const res = await apiFetch('/workspace/pull-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as PullSkillResponse;
}

/** One file's change within a push preview diff (issue #379 follow-up). */
export interface SkillDiffFile {
    path: string;
    status: 'added' | 'removed' | 'modified';
    diff: string;
}

/**
 * Read-only preview of what a push would do, fetched before the user commits.
 * `isNew` means this workspace copy isn't in 母体 yet (first-time add) — `target`
 * and `files` are absent/empty. Otherwise `target` is the 母体 package this would
 * push against and `files` is the per-file unified diff; `diverged` flags that
 * 母体 moved past this copy's base version since it was last synced.
 */
export interface SkillPushPreview {
    exists: boolean;
    isNew: boolean;
    sourcePath?: string;
    target?: { id: string; name: string; storeVersion: number; baseVersion: number };
    diverged?: boolean;
    files?: SkillDiffFile[];
}

/**
 * Preview a skill push without writing anything (issue #379 follow-up): lets the
 * UI show a diff of "workspace copy vs 母体" before the user commits to update
 * or fork.
 */
async function previewPush(workspaceId: string, skillRef: string): Promise<SkillPushPreview> {
    const res = await apiFetch('/workspace/push-skill?preview=1', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as SkillPushPreview;
}

/**
 * Resolve a concurrent-edit conflict surfaced by `pushSkill`. `resolution: 'fork'`
 * keeps the store's current version as main and lands the pushed content as a new
 * side fork; `resolution: 'main'` promotes the pushed content to main and demotes
 * the store's current version to a fork. `name` optionally renames the new fork.
 */
async function resolvePush(params: {
    sourcePath: string;
    baseId: string;
    resolution: 'main' | 'fork';
    name?: string;
}): Promise<{ ok: boolean; id: string; resolution: 'main' | 'fork'; version: number; packageDir: string }> {
    const res = await apiFetch('/skills/resolve-push', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(params),
    });
    if (!res.ok) throw new Error(await res.text());
    return (await res.json()) as {
        ok: boolean;
        id: string;
        resolution: 'main' | 'fork';
        version: number;
        packageDir: string;
    };
}

/** A 母体 (shared store) skill the project can add — one not already present
 * under the workspace's .claude/skills. Drives the project "add skill" picker. */
export interface AvailableSkill {
    skillRef: string;
    dir: string;
    name: string;
    description: string;
    version: number;
    primaryTag?: string | null;
    secondaryTag?: string | null;
}

/** List the 母体 store skills the project can add (those not yet in it). */
async function listAvailableSkills(workspaceId: string): Promise<AvailableSkill[]> {
    const res = await apiFetch(`/workspace/available-skills?id=${encodeURIComponent(workspaceId)}`);
    if (!res.ok) throw new Error(await res.text());
    const data = (await res.json()) as { skills?: AvailableSkill[] };
    return data.skills ?? [];
}

/** Add (materialize) a 母体 store skill into the project's .claude/skills. */
async function addSkill(workspaceId: string, skillRef: string): Promise<void> {
    const res = await apiFetch('/workspace/add-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
}

/** Remove a skill from the project's .claude/skills only (母体 untouched). */
async function removeSkill(workspaceId: string, skillRef: string): Promise<void> {
    const res = await apiFetch('/workspace/remove-skill', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ id: workspaceId, skillRef }),
    });
    if (!res.ok) throw new Error(await res.text());
}

export const skillService = {
    listSkills,
    listWorkspaceSkills,
    pushSkill,
    pullSkill,
    previewPush,
    resolvePush,
    listAvailableSkills,
    addSkill,
    removeSkill,
};
