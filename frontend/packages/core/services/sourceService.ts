import { apiFetch } from './apiClient';

// 数据源 (data sources) — the read-only bronze layer (backend/internal/sources).
// Overview cards read the per-(source,kind) rollup; the 多维表格 detail view reads
// the raw records for one (source, kind). Mirrors sources.SourceSummary /
// sources.SourceRecordRow (json-tagged → camelCase).

export interface SourceSummary {
    source: string;
    /** bronze account_id this rollup belongs to (源为中心 per-account totals). */
    accountId: string;
    kind: string;
    /** Non-deleted record count. */
    count: number;
    /** Distinct collections (address books / calendars / chats). */
    collections: number;
    /** Epoch ms of the most recent pull; 0 when empty. */
    lastFetchedAt: number;
}

// One native property of a record (vCard FN/TEL/…). No fixed schema — the grid
// builds columns from whatever keys appear, so the same viewer serves contacts
// now and todos/calendars later.
export interface SourceField {
    key: string;
    value: string;
}

// One bronze record for the grid: the generic envelope + its native fields in
// source order (repeats preserved, e.g. two TELs).
export interface SourceRecordRow {
    uid: string;
    collection: string;
    etag: string;
    contentType: string;
    deleted: boolean;
    fetchedAt: number;
    fields: SourceField[];
    preview: string;
}

// 集合(表)配置 — per-source collection crawl settings.
export interface CollectionView {
    source: string;
    kind: string;
    enabled: boolean;
    initialLookbackDays: number;
    incrementalMinutes: number;
    pageSize: number;
    updatedAt: string;
    domain: string;
    label: string;
    implemented: boolean;
    perChat: boolean;
    configured: boolean;
}

export interface CollectionUpdateInput {
    kind: string;
    enabled: boolean;
    initialLookbackDays: number;
    incrementalMinutes: number;
    pageSize: number;
}

// 同步历史 — each completed or in-progress work-order sync run.
export interface SyncRun {
    taskId: string;
    kind: string;
    collection?: string;
    status: string;
    result?: string;
    createdAt: string;
    completedAt?: string;
}

// 数据源厂家能力 (vendor capability) — drives the 添加数据源 flow: which regions a
// vendor offers (国际/大陆), whether multiple accounts are allowed (飞书 is
// single-account), and how the account authenticates. Mirrors sources.VendorSpec.
export interface VendorSpec {
    vendor: string;
    label: string;
    multiAccount: boolean;
    regions: string[]; // 'intl' | 'cn'
    authKind: string; // 'credentials' | 'cli' | 'oauth'
}

// 数据源账号 (source account) — 厂家 + 账号 = 一个源. Mirrors meta.SourceAccount.
export interface SourceAccount {
    id: string;
    vendor: string;
    region: string; // 'intl' | 'cn'
    label: string;
    status: string;
    createdAt: string;
    updatedAt: string;
}

export interface CreateAccountInput {
    vendor: string;
    region: string;
    label?: string;
    appleId?: string; // iCloud only
    password?: string; // iCloud only (stored in Keychain server-side)
}

// Microsoft Graph OAuth connect state for one account. `configured` is false
// until the per-region app registration is present.
export interface MSOAuthStatus {
    configured: boolean;
    connected: boolean;
    expiresAt: number; // epoch seconds; 0 when not connected
    scope: string;
    region: string; // 'intl' | 'cn'
}

// Per-region Microsoft app registration (clientId is an app identifier, not a
// secret). Backs the in-UI configure form.
export interface MSOAuthConfig {
    region: string;
    clientId: string;
    tenant: string;
    redirectUri: string;
    configured: boolean;
}

export const sourceService = {
    /** GET /api/sources/vendors — vendor capability table for the add-source flow. */
    async vendors(): Promise<VendorSpec[]> {
        const res = await apiFetch('/sources/vendors');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as VendorSpec[];
    },

    /** GET /api/sources/accounts — every registered (厂家+账号) source. */
    async accounts(): Promise<SourceAccount[]> {
        const res = await apiFetch('/sources/accounts');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceAccount[];
    },

    /** POST /api/sources/accounts — register a new account (a new source). */
    async createAccount(input: CreateAccountInput): Promise<SourceAccount> {
        const res = await apiFetch('/sources/accounts', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceAccount;
    },

    /** DELETE /api/sources/accounts/{id} — remove an account. */
    async deleteAccount(id: string): Promise<void> {
        const res = await apiFetch(`/sources/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' });
        if (!res.ok && res.status !== 204) throw new Error(await res.text());
    },

    /** GET /api/sources/summary — per-(source,kind) rollup for the overview cards. */
    async summary(): Promise<SourceSummary[]> {
        const res = await apiFetch('/sources/summary');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceSummary[];
    },

    /** GET /api/sources/records?source=&kind=&account=&limit= — raw records as grid
     * rows. account (optional) scopes to one bronze account_id (源为中心). */
    async records(source: string, kind: string, limit = 1000, account = ''): Promise<SourceRecordRow[]> {
        const qs = new URLSearchParams({ source, kind, limit: String(limit) });
        if (account) qs.set('account', account);
        const res = await apiFetch(`/sources/records?${qs.toString()}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SourceRecordRow[];
    },

    /** GET /api/sources/{source}/collections — crawl config for every collection kind. */
    async collections(source: string): Promise<CollectionView[]> {
        const res = await apiFetch(`/sources/${encodeURIComponent(source)}/collections`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CollectionView[];
    },

    /** PUT /api/sources/{source}/collections — save one collection's crawl config. */
    async setCollection(source: string, cfg: CollectionUpdateInput): Promise<CollectionView> {
        const res = await apiFetch(`/sources/${encodeURIComponent(source)}/collections`, {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(cfg),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CollectionView;
    },

    /** POST /api/sources/{source}/sync — dispatch an immediate work-order sync for a kind. */
    async syncNow(source: string, kind: string): Promise<{ taskId: string }> {
        const res = await apiFetch(`/sources/${encodeURIComponent(source)}/sync`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ kind }),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as { taskId: string };
    },

    /** GET /api/sources/{source}/history — sync run history, newest first. */
    async syncHistory(source: string): Promise<SyncRun[]> {
        const res = await apiFetch(`/sources/${encodeURIComponent(source)}/history`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as SyncRun[];
    },

    /** GET /api/sources/oauth/microsoft/config — current app registration for a
     * region (to prefill the in-UI settings form). Never returns a secret. */
    async msOAuthGetConfig(region: string): Promise<MSOAuthConfig> {
        const res = await apiFetch(`/sources/oauth/microsoft/config?region=${encodeURIComponent(region)}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as MSOAuthConfig;
    },

    /** POST /api/sources/oauth/microsoft/config — save clientId/tenant for a region
     * (hot-reloaded, no restart), so the connect button lights up. */
    async msOAuthSetConfig(input: {
        region: string;
        clientId: string;
        tenant: string;
        redirectUri?: string;
    }): Promise<{ configured: boolean }> {
        const res = await apiFetch('/sources/oauth/microsoft/config', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(input),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as { configured: boolean };
    },

    /** POST /api/sources/oauth/microsoft/start — begin the Microsoft Graph connect
     * (OAuth authorization-code + PKCE). Returns the authorization URL to open;
     * the region (大陆/国际) is taken from the account. */
    async msOAuthStart(accountId: string): Promise<{ authUrl: string }> {
        const res = await apiFetch('/sources/oauth/microsoft/start', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ accountId }),
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as { authUrl: string };
    },

    /** GET /api/sources/oauth/microsoft/status — connect state for one account. */
    async msOAuthStatus(accountId: string): Promise<MSOAuthStatus> {
        const res = await apiFetch(`/sources/oauth/microsoft/status?accountId=${encodeURIComponent(accountId)}`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as MSOAuthStatus;
    },

    /** POST /api/sources/oauth/microsoft/disconnect — drop an account's token. */
    async msOAuthDisconnect(accountId: string): Promise<void> {
        const res = await apiFetch('/sources/oauth/microsoft/disconnect', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ accountId }),
        });
        if (!res.ok && res.status !== 204) throw new Error(await res.text());
    },

    /** GET /api/sources/feishu/chats — the CACHED group list (bronze feishu_chat
     * rows joined with tracked state). Never shells out to lark-cli; refresh by
     * dispatching a feishu_chat sync and re-fetching. */
    async feishuChats(): Promise<CachedChatsResponse> {
        const res = await apiFetch('/sources/feishu/chats');
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CachedChatsResponse;
    },
};

/** One Feishu group from the bronze cache, with its message-scope tracked flag. */
export interface CachedChat {
    chatId: string;
    name: string;
    avatar: string;
    description: string;
    external: boolean;
    tenantKey: string;
    tracked: boolean;
}

export interface CachedChatsResponse {
    chats: CachedChat[];
    /** Epoch ms of the newest bronze row; 0 = 群列表 never synced. */
    cachedAt: number;
}
