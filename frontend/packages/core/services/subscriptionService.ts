/**
 * 订阅 / 体验服务(契约先行,issue: 领取体验 + 订阅状态)。
 *
 * 直连 happy-server(中转服务器本身),而非经 apiClient 路由到远端节点 Go
 * 后端 —— 订阅是 relay 账户维度的能力,所以:
 *  - serverUrl:取自 localStorage key `oneagents.relay.url`(与 relayClient /
 *    apiClient 同源约定),未配置时回退到当前页面 origin。
 *  - token:取自 relayClient 的 relay 账户凭据(localStorage
 *    `oneagents.relay.creds`),即 createAccount() 拿到的 happy-server token。
 *  - 调用方式照搬 relayClient 的"直连 happy-server" fetch(Bearer token +
 *    X-Happy-Client 头),不走 apiFetch。
 *
 * token 不存在 = 还没有 relay 账户,抛 NoRelayAccountError 让 UI 引导去创建。
 */
import { loadCredentials } from './relay/relayClient';
import { getPlatformBridge } from '../platform/bridge';

const LS_URL = 'oneagents.relay.url';
const CLIENT_ID = 'web/1agents-relay';

/** 订阅状态(对齐 GET /v1/subscription 契约)。 */
export interface SubscriptionInfo {
    status: 'active' | 'expired' | 'none';
    plan: string | null;
    /** ISO 时间串;none 时为 null。 */
    expiresAt: string | null;
    maxDevices: number;
    source: 'trial' | 'code' | null;
}

/** token 不存在(还没在 happy-server 上建账户)时抛出。 */
export class NoRelayAccountError extends Error {
    constructor() {
        super('尚未创建中转账户');
        this.name = 'NoRelayAccountError';
    }
}

/** 体验已领取过(POST /v1/subscription/claim-trial → 409)时抛出。 */
export class TrialAlreadyClaimedError extends Error {
    constructor() {
        super('已领取过体验');
        this.name = 'TrialAlreadyClaimedError';
    }
}

/** 取中转服务器地址:localStorage 优先,回退当前页面 origin。 */
function serverUrl(): string {
    const stored = getPlatformBridge().storage.get(LS_URL);
    if (stored) return stored.replace(/\/+$/, '');
    return typeof window !== 'undefined' ? window.location.origin : '';
}

/** 取 relay 账户 token;无则抛 NoRelayAccountError。 */
function requireToken(): string {
    const creds = loadCredentials();
    if (!creds?.token) throw new NoRelayAccountError();
    return creds.token;
}

/** 是否已有 relay 账户(供 UI 决定渲染创建入口还是订阅卡)。 */
export function hasRelayAccount(): boolean {
    return !!loadCredentials()?.token;
}

function headers(token: string): Record<string, string> {
    return {
        'content-type': 'application/json',
        'X-Happy-Client': CLIENT_ID,
        Authorization: `Bearer ${token}`,
    };
}

/** 拉取当前订阅状态。 */
export async function getSubscription(): Promise<SubscriptionInfo> {
    const token = requireToken();
    const resp = await getPlatformBridge().httpFetch(`${serverUrl()}/v1/subscription`, {
        method: 'GET',
        headers: headers(token),
    });
    const text = await resp.text();
    if (!resp.ok) throw new Error(`GET /v1/subscription → ${resp.status}: ${text.slice(0, 200)}`);
    return JSON.parse(text) as SubscriptionInfo;
}

/** 领取体验(3 天)。409 → TrialAlreadyClaimedError。 */
export async function claimTrial(): Promise<{ ok: true; expiresAt: string }> {
    const token = requireToken();
    const resp = await getPlatformBridge().httpFetch(`${serverUrl()}/v1/subscription/claim-trial`, {
        method: 'POST',
        headers: headers(token),
    });
    const text = await resp.text();
    if (resp.status === 409) throw new TrialAlreadyClaimedError();
    if (!resp.ok) throw new Error(`POST /v1/subscription/claim-trial → ${resp.status}: ${text.slice(0, 200)}`);
    return JSON.parse(text) as { ok: true; expiresAt: string };
}
