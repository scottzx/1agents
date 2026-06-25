import { apiFetch } from './apiClient';

// Inbox 统一信息收口层 (#60) — multi-source intake list.
// Mirrors backend/internal/meta (InboxItem + InboxHandler).

export type InboxSource = 'manual' | 'im' | 'email' | 'rss' | 'misc';
export type InboxStatus = 'unread' | 'read' | 'archived';

export interface InboxItem {
    id: string;
    source: InboxSource;
    title: string;
    content?: string;
    url?: string;
    summary?: string;
    tags?: string[];
    status: InboxStatus;
    createdAt: string;
    updatedAt: string;
}

export interface InboxListResult {
    items: InboxItem[];
    unread: number;
}

export interface InboxCaptureInput {
    title?: string;
    content?: string;
    url?: string;
    source?: InboxSource;
    tags?: string[];
}

export const inboxService = {
    /** GET /api/inbox — list items (archived hidden unless includeArchived). */
    async list(includeArchived = false): Promise<InboxListResult> {
        const res = await apiFetch(`/inbox${includeArchived ? '?archived=1' : ''}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxListResult;
    },

    /** POST /api/inbox — manual capture of a paste/link/idea. */
    async capture(input: InboxCaptureInput): Promise<InboxItem> {
        const res = await apiFetch('/inbox', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxItem;
    },

    /** POST /api/inbox/{id}/{action} — flip status (archive keeps the row). */
    async setStatus(id: string, action: 'archive' | 'read' | 'unread'): Promise<InboxItem> {
        const res = await apiFetch(`/inbox/${id}/${action}`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxItem;
    },
};
