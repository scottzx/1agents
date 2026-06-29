import { apiFetch } from './apiClient';

// 联系人聚合 (Contacts aggregation) — a user-curated address book over channel
// identities auto-discovered from synced Feishu messages.
// Mirrors backend/internal/contacts (Handler) + backend/internal/meta (Contact,
// ContactChannel) + backend/internal/feishu (Message/SenderInfo/SessionSummary).
//
// NOTE on casing: Contact/ContactChannel are json-tagged on the Go side →
// camelCase. feishu.Message / SessionSummary have NO json tags → they serialize
// as Go PascalCase field names, so those TS types use PascalCase keys.

export interface ContactChannel {
    id: string;
    contactId: string;
    platform: string;
    channelId: string;
    nickname: string;
    sessionId: string;
    lastSeen: number;
    createdAt: string;
    updatedAt: string;
}

export interface Contact {
    id: string;
    phone: string;
    name: string;
    company: string;
    title: string;
    note: string;
    tags: string[];
    createdAt: string;
    updatedAt: string;
    channels?: ContactChannel[];
}

export interface ContactInput {
    phone?: string;
    name?: string;
    company?: string;
    title?: string;
    note?: string;
    tags?: string[];
}

// feishu.Message — PascalCase (no json tags on the Go struct).
export interface FeishuMessage {
    Channel: string;
    ChannelAccID: string;
    MessageID: string;
    SessionID: string;
    SenderID: string;
    SenderName: string;
    MsgType: string;
    Title: string;
    Content: string;
    CreateTime: number;
}

// feishu.SessionSummary — PascalCase (no json tags).
export interface SessionSummary {
    SessionID: string;
    SessionName: string;
    LastPreview: string;
    LastTime: number;
    Count: number;
}

export interface DiscoverResult {
    discovered: number;
    updated: number;
}

export const contactService = {
    /** GET /api/contacts — all contacts, each with bound channels. */
    async listContacts(): Promise<Contact[]> {
        const res = await apiFetch('/contacts');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as Contact[];
    },

    /** POST /api/contacts — create a contact (409 on duplicate phone). */
    async createContact(input: ContactInput): Promise<Contact> {
        const res = await apiFetch('/contacts', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as Contact;
    },

    /** PATCH /api/contacts/{id} — update (409 on duplicate phone). */
    async updateContact(id: string, input: ContactInput): Promise<Contact> {
        const res = await apiFetch(`/contacts/${id}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as Contact;
    },

    /** DELETE /api/contacts/{id} — delete (its channels are unbound, not deleted). */
    async deleteContact(id: string): Promise<void> {
        const res = await apiFetch(`/contacts/${id}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** GET /api/contacts/channels?contactId=&unlinked=1 — channel identities. */
    async listChannels(opts: { contactId?: string; unlinked?: boolean } = {}): Promise<ContactChannel[]> {
        const params = new URLSearchParams();
        if (opts.contactId) params.set('contactId', opts.contactId);
        if (opts.unlinked) params.set('unlinked', '1');
        const qs = params.toString();
        const res = await apiFetch(`/contacts/channels${qs ? `?${qs}` : ''}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ContactChannel[];
    },

    /** POST /api/contacts/discover — scan synced messages for channel identities. */
    async discover(): Promise<DiscoverResult> {
        const res = await apiFetch('/contacts/discover', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as DiscoverResult;
    },

    /** POST /api/contacts/channels/{id}/link — bind a channel to a contact. */
    async linkChannel(id: string, contactId: string): Promise<void> {
        const res = await apiFetch(`/contacts/channels/${id}/link`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ contactId }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    /** POST /api/contacts/channels/{id}/unlink — detach a channel. */
    async unlinkChannel(id: string): Promise<void> {
        const res = await apiFetch(`/contacts/channels/${id}/unlink`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** GET /api/contacts/messages?contactId=|sessionId=&limit= — chat messages. */
    async messages(opts: { contactId?: string; sessionId?: string; limit?: number }): Promise<FeishuMessage[]> {
        const params = new URLSearchParams();
        if (opts.contactId) params.set('contactId', opts.contactId);
        if (opts.sessionId) params.set('sessionId', opts.sessionId);
        if (opts.limit) params.set('limit', String(opts.limit));
        const res = await apiFetch(`/contacts/messages?${params.toString()}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as FeishuMessage[];
    },

    /** GET /api/contacts/sessions — session summaries (group list). */
    async sessions(): Promise<SessionSummary[]> {
        const res = await apiFetch('/contacts/sessions');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SessionSummary[];
    },
};
