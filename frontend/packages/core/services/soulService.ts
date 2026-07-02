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

export const soulService = { listSouls, getWorkspaceSoul, saveWorkspaceSoul };
