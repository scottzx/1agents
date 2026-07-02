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

export const skillService = { listSkills };
