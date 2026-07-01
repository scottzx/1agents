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
    /** Feishu org of the member (free from chat.members on degree-2 ingestion). */
    tenantKey: string;
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
    /** 1 = first-degree (manual/好友), 2 = second-degree (group roster only). */
    degree: number;
    /** How many distinct tracked groups this contact appears in (所在群 column). */
    groupCount: number;
    createdAt: string;
    updatedAt: string;
    channels?: ContactChannel[];
}

// One feishu_group_members roster entry (degree-2 source).
export interface GroupMember {
    openId: string;
    nickname: string;
    tenantKey: string;
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

// meta.CompanyTenant — one company_tenants × companies join row: a mapped
// tenant_key + its company names. The frontend builds a tenantKey→shortName
// lookup from the list to label a contact's channel org (replaces the old
// hardcoded 飞书官方 constant; 飞书官方 is now seeded data).
export interface CompanyTenant {
    tenantKey: string;
    companyId: string;
    fullName: string;
    shortName: string;
}

// ── 飞书渠道配置 (Phase 2) ──────────────────────────────────────────────────
// feishu.ChatInfo — camelCase json tags.
export interface ChatInfo {
    chatId: string;
    name: string;
    avatar: string;
    description: string;
    chatMode: string;
    tenantKey: string;
    external: boolean;
}

// meta.TrackedChat — camelCase json tags. memberCount is the degree-2 roster
// size, attached additively by the tracked-chats endpoint (0 until first sync).
export interface TrackedChat {
    chatId: string;
    chatName: string;
    avatar: string;
    external: boolean;
    autoSync: boolean;
    lastSyncedAt: number;
    createdAt: string;
    // memberCount = degree-2 roster size actually ingested; memberTotal = the
    // chat's true size from the API. For large groups the API caps the roster,
    // so memberTotal can exceed memberCount.
    memberCount?: number;
    memberTotal?: number;
}

// meta.SyncConfig — camelCase json tags.
export interface SyncConfig {
    enabled: boolean;
    intervalMinutes: number;
}

// feishu.DoctorCheck — camelCase json tags.
export interface DoctorCheck {
    name: string;
    status: string;
    message: string;
    hint: string;
}

export interface ChannelStatus {
    connected: boolean;
    error?: string;
    checks: DoctorCheck[];
    config: SyncConfig;
}

export interface SyncResultItem {
    chatId: string;
    inserted: number;
    fetched: number;
    error?: string;
}

export interface TrackChatInput {
    chatId: string;
    chatName?: string;
    avatar?: string;
    external?: boolean;
}

export const contactService = {
    /** GET /api/contacts?degree= — contacts (each with bound channels); optional
     * degree filter (1 = first-degree, 2 = second-degree; omit for all). */
    async listContacts(degree?: number): Promise<Contact[]> {
        const qs = degree === 1 || degree === 2 ? `?degree=${degree}` : '';
        const res = await apiFetch(`/contacts${qs}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as Contact[];
    },

    /** GET /api/contacts/{id}/groups — the tracked-group sessionIds a contact
     * belongs to, from the roster (a person in N groups has 1 channel but N
     * roster rows, so membership comes from the roster not the channel). */
    async contactGroups(contactId: string): Promise<string[]> {
        const res = await apiFetch(`/contacts/${encodeURIComponent(contactId)}/groups`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as string[];
    },

    /** GET /api/contacts/groups/{sessionId}/members — a tracked group's roster. */
    async groupMembers(sessionId: string): Promise<GroupMember[]> {
        const res = await apiFetch(`/contacts/groups/${encodeURIComponent(sessionId)}/members`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as GroupMember[];
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

    /** GET /api/contacts/companies — tenant_key→company-name map rows. The caller
     * builds a tenantKey→shortName lookup to label a contact's channel org. */
    async companies(): Promise<CompanyTenant[]> {
        const res = await apiFetch('/contacts/companies');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CompanyTenant[];
    },
};

// 飞书渠道配置 (Phase 2) — browse Feishu groups, track/untrack, manual + auto
// sync. Mirrors backend/internal/digest (Handler) chat-config endpoints. All
// fetch/dedup reuses the existing SyncChat watermark loop server-side.
export const channelService = {
    /** GET /api/digest/chats/available — Feishu groups the user is in (lark-cli). */
    async availableChats(): Promise<ChatInfo[]> {
        const res = await apiFetch('/digest/chats/available');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ChatInfo[];
    },

    /** GET /api/digest/chats/tracked — tracked chats. */
    async trackedChats(): Promise<TrackedChat[]> {
        const res = await apiFetch('/digest/chats/tracked');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as TrackedChat[];
    },

    /** POST /api/digest/chats/tracked — start tracking a chat. */
    async trackChat(input: TrackChatInput): Promise<TrackedChat> {
        const res = await apiFetch('/digest/chats/tracked', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as TrackedChat;
    },

    /** DELETE /api/digest/chats/tracked/{chatId} — untrack a chat. */
    async untrackChat(chatId: string): Promise<void> {
        const res = await apiFetch(`/digest/chats/tracked/${encodeURIComponent(chatId)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** PATCH /api/digest/chats/tracked/{chatId} — toggle a chat's auto-sync. */
    async setChatAutoSync(chatId: string, on: boolean): Promise<TrackedChat> {
        const res = await apiFetch(`/digest/chats/tracked/${encodeURIComponent(chatId)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ autoSync: on }),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as TrackedChat;
    },

    /** POST /api/digest/sync {chatId} — sync one chat now. */
    async syncOne(chatId: string): Promise<void> {
        const res = await apiFetch('/digest/sync', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ chatId }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    /** POST /api/digest/sync/all — sync every tracked chat now. */
    async syncAll(): Promise<SyncResultItem[]> {
        const res = await apiFetch('/digest/sync/all', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        const data = (await res.json()) as { results: SyncResultItem[] };
        return data.results || [];
    },

    /** GET /api/digest/status — lark-cli connectivity + sync config. */
    async status(): Promise<ChannelStatus> {
        const res = await apiFetch('/digest/status');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ChannelStatus;
    },

    /** GET /api/digest/sync/config — global auto-sync config. */
    async getSyncConfig(): Promise<SyncConfig> {
        const res = await apiFetch('/digest/sync/config');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SyncConfig;
    },

    /** PUT /api/digest/sync/config — persist the global auto-sync config. */
    async setSyncConfig(enabled: boolean, intervalMinutes: number): Promise<SyncConfig> {
        const res = await apiFetch('/digest/sync/config', {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ enabled, intervalMinutes }),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SyncConfig;
    },
};

/** iCloud credential status (never carries the password). */
export interface ICloudStatus {
    configured: boolean;
    appleId: string;
}

/** Result of an iCloud contacts CardDAV pull. */
export interface ICloudSyncResult {
    created: number;
    updated: number;
}

// iCloud 通讯录 (CardDAV) — the user provides their Apple ID + an app-specific
// password (stored locally in the Keychain server-side); contacts are pulled
// over CardDAV. Mirrors backend/internal/contacts (icloud.go) endpoints.
export const icloudService = {
    /** GET /api/contacts/icloud/credentials — {configured, appleId} (no password). */
    async status(): Promise<ICloudStatus> {
        const res = await apiFetch('/contacts/icloud/credentials');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ICloudStatus;
    },

    /** POST /api/contacts/icloud/credentials {appleId, password} — store (password→Keychain). */
    async setCredentials(appleId: string, password: string): Promise<ICloudStatus> {
        const res = await apiFetch('/contacts/icloud/credentials', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ appleId, password }),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ICloudStatus;
    },

    /** DELETE /api/contacts/icloud/credentials — clear the stored credential. */
    async clearCredentials(): Promise<void> {
        const res = await apiFetch('/contacts/icloud/credentials', { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** POST /api/contacts/icloud/sync — pull the iCloud address book via CardDAV. */
    async sync(): Promise<ICloudSyncResult> {
        const res = await apiFetch('/contacts/icloud/sync', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ICloudSyncResult;
    },
};

/** Result of an iMessage chat.db pull. */
export interface IMessageSyncResult {
    fetched: number;
    inserted: number;
    watermark: number;
}

// iMessage — pull the local chat.db (needs Full Disk Access). Mirrors
// backend/internal/contacts (mac.go).
export const imessageService = {
    /** POST /api/contacts/imessage/sync — pull chat.db into the unified store. */
    async sync(): Promise<IMessageSyncResult> {
        const res = await apiFetch('/contacts/imessage/sync', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as IMessageSyncResult;
    },
};

/** One channel sub-module's consent + crawl-rule state. rules is module-specific
 * (e.g. iMessage: {timeWindowDays, includeAttachments}). */
export interface ChannelModule {
    id: string;
    consented: boolean;
    consentedAt: string;
    autoSync: boolean;
    intervalMinutes: number;
    rules: Record<string, unknown>;
}

// Per-sub-module privacy consent + deterministic crawl rules. Mirrors
// backend/internal/contacts (channels.go). Consent is required before any sync.
export const channelModuleService = {
    /** GET /api/channels/modules — consent + rules for every known sub-module. */
    async list(): Promise<ChannelModule[]> {
        const res = await apiFetch('/channels/modules');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ChannelModule[];
    },

    /** POST /api/channels/modules/{id}/consent — record explicit authorization. */
    async consent(id: string): Promise<void> {
        const res = await apiFetch(`/channels/modules/${id}/consent`, { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** DELETE /api/channels/modules/{id}/consent — revoke authorization. */
    async revoke(id: string): Promise<void> {
        const res = await apiFetch(`/channels/modules/${id}/consent`, { method: 'DELETE' });
        if (!res.ok) throw new Error(await res.text());
    },

    /** PUT /api/channels/modules/{id}/rules — set crawl rules (frequency + scope). */
    async setRules(
        id: string,
        input: { autoSync: boolean; intervalMinutes: number; rules: Record<string, unknown> }
    ): Promise<ChannelModule> {
        const res = await apiFetch(`/channels/modules/${id}/rules`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as ChannelModule;
    },
};
