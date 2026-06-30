import { apiFetch } from './apiClient';

// 复盘归档读取 (#271) — read-only access to the retrospectives the project
// archive hook (#144) ingests into the kwiki knowledge base. Mirrors
// backend/internal/retro (Item + Handler).

export interface RetroItem {
    slug: string;
    title: string;
    summary?: string;
    tags?: string[];
    body: string;
    created?: string;
    updated?: string;
}

export const retroService = {
    /** GET /api/retrospectives — list every retrospective page (metadata + body). */
    async list(): Promise<RetroItem[]> {
        const res = await apiFetch('/retrospectives');
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as { items?: RetroItem[] };
        return data.items || [];
    },

    /** GET /api/retrospectives/{slug} — one retrospective page. */
    async get(slug: string): Promise<RetroItem> {
        const res = await apiFetch(`/retrospectives/${encodeURIComponent(slug)}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as RetroItem;
    },
};
