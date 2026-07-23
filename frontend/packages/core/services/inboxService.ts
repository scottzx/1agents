import { apiFetch } from './apiClient';

// Workspace Inbox (#202) — multi-source intake per Workspace.
// Mirrors backend/internal/meta (InboxItem + InboxHandler / Accept / Deliver).

export type InboxSource = 'manual' | 'agent' | 'function' | 'im' | 'email' | 'rss' | 'data_source' | 'misc';
export type InboxStatus = 'unread' | 'read' | 'archived';

export interface InboxItem {
    id: string;
    workspaceId?: string;
    source: InboxSource;
    fromWorkspaceId?: string;
    fromRef?: string;
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
    workspaceId?: string;
    title?: string;
    content?: string;
    url?: string;
    source?: InboxSource;
    tags?: string[];
}

export interface InboxAcceptInput {
    workspaceId?: string;
    title?: string;
    description?: string;
    priority?: string;
}

export interface InboxAcceptResult {
    requirement?: { id: string; title?: string; type?: string; labels?: string[] };
    project?: { id: string; name?: string };
    // backend may return flat DispatchResult shape
    [key: string]: unknown;
}

export interface InboxDeliverInput {
    workspaceId: string;
    source?: InboxSource;
    fromWorkspaceId?: string;
    fromRef?: string;
    title?: string;
    content?: string;
    url?: string;
    summary?: string;
    tags?: string[];
}

/** Workspace that can receive mail (GET /api/inbox/targets). */
export interface InboxTarget {
    projectId: string;
    name: string;
    workspacePath?: string;
}

function qs(params: Record<string, string | undefined>): string {
    const p = new URLSearchParams();
    for (const [k, v] of Object.entries(params)) {
        if (v) p.set(k, v);
    }
    const s = p.toString();
    return s ? `?${s}` : '';
}

export const inboxService = {
    /** GET /api/inbox?workspaceId= — list items for one Workspace (or global if omitted). */
    async list(includeArchived = false, workspaceId?: string): Promise<InboxListResult> {
        const res = await apiFetch(
            `/inbox${qs({
                workspaceId,
                archived: includeArchived ? '1' : undefined,
            })}`
        );
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxListResult;
    },

    /** POST /api/inbox — manual capture (prefer workspaceId of current shell). */
    async capture(input: InboxCaptureInput): Promise<InboxItem> {
        const res = await apiFetch('/inbox', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxItem;
    },

    /** POST /api/inbox/deliver — unified envelope write (function / agent / channel). */
    async deliver(input: InboxDeliverInput): Promise<InboxItem> {
        const res = await apiFetch('/inbox/deliver', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxItem;
    },

    /** GET /api/inbox/targets — workspaces that can receive send_mail / deliver. */
    async listTargets(): Promise<InboxTarget[]> {
        const res = await apiFetch('/inbox/targets');
        if (!res.ok) throw new Error(await res.text());
        const body = (await res.json()) as { targets?: InboxTarget[] };
        return body.targets || [];
    },

    /** POST /api/inbox/{id}/accept — adopt as requirement in workspace. */
    async accept(id: string, input: InboxAcceptInput = {}): Promise<InboxAcceptResult> {
        const res = await apiFetch(`/inbox/${id}/accept`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxAcceptResult;
    },

    /** POST /api/inbox/{id}/{action} — flip status (archive keeps the row). */
    async setStatus(id: string, action: 'archive' | 'read' | 'unread', workspaceId?: string): Promise<InboxItem> {
        const res = await apiFetch(`/inbox/${id}/${action}${qs({ workspaceId })}`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as InboxItem;
    },
};
