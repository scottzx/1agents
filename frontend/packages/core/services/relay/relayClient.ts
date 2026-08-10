/**
 * 1Agents Relay 客户端(方案 B / M2)。
 *
 * 让 1Agents H5 成为一个"持账户密钥"的 Happy user-scoped 客户端:
 *  - createAccount: 在中转上创建账户(主密钥只存本浏览器)
 *  - approveTerminal: 审批机器(daemon)的配对请求,把 daemon 绑到本账户
 *  - listMachines:   拉取并解密本账户名下的机器
 *  - callMachine:    经中转对某台机器发起加密 RPC(如 1agents-proxy → 该机器本地 Go 后端)
 *
 * 终端(ttyd)流暂不走这里(保留风险点),仅承载控制类 RPC。
 */
import { io, Socket } from 'socket.io-client';
import {
    encodeBase64,
    decodeBase64,
    deriveContentKeyPair,
    decryptBoxBundle,
    encryptForPublicKey,
    encrypt,
    decrypt,
    authChallenge,
    randomSecret,
    Variant,
} from './crypto';

const CLIENT_ID = 'web/1agents-relay';
const LS_KEY = 'oneagents.relay.creds';

export interface RelayCredentials {
    token: string;
    secretB64: string;
}
export interface RelayMachine {
    id: string;
    active: boolean;
    encryptionKey: Uint8Array;
    variant: Variant;
}

/**
 * Split a relay base URL into origin + base path (no trailing slash).
 * HTTP calls just concat `${serverUrl}/v1/...` so they work with or without a
 * base path, but socket.io treats a URL path as the namespace and resolves its
 * `path` option against the bare origin — so the WS path must be built from
 * origin + basePath explicitly. At root, basePath is '' and this is a no-op.
 */
function splitRelayUrl(serverUrl: string): { origin: string; basePath: string } {
    try {
        const u = new URL(serverUrl);
        return { origin: u.origin, basePath: u.pathname.replace(/\/+$/, '') };
    } catch {
        return { origin: serverUrl, basePath: '' };
    }
}

function headers(token?: string): Record<string, string> {
    const h: Record<string, string> = { 'content-type': 'application/json', 'X-Happy-Client': CLIENT_ID };
    if (token) h['Authorization'] = `Bearer ${token}`;
    return h;
}

async function jsonFetch(url: string, init: RequestInit): Promise<unknown> {
    const resp = await fetch(url, init);
    const text = await resp.text();
    if (!resp.ok) throw new Error(`${init.method ?? 'GET'} ${url} → ${resp.status}: ${text.slice(0, 200)}`);
    return text ? JSON.parse(text) : null;
}

interface RawMachine {
    id: string;
    active?: boolean;
    dataEncryptionKey?: string | null;
}

function hasLocalStorage(): boolean {
    return typeof localStorage !== 'undefined';
}

/**
 * Synchronous read of the locally cached credentials (localStorage). Returns
 * instantly for UI initialization; the backend-persisted copy (issue #109) is
 * hydrated separately via loadCredentialsRemote().
 */
export function loadCredentials(): RelayCredentials | null {
    if (!hasLocalStorage()) return null;
    try {
        const raw = localStorage.getItem(LS_KEY);
        return raw ? (JSON.parse(raw) as RelayCredentials) : null;
    } catch {
        return null;
    }
}

/**
 * Backend-persisted credentials endpoint (issue #109). The relay account master
 * key now lives on the 1agents host so it survives a localStorage wipe / device
 * change. Only meaningful when the SPA is same-origin with its backend (local
 * machine mode); on relay/CDN-hosted or 小程序 contexts the endpoint is absent
 * and these calls fail silently, falling back to the localStorage cache.
 */
const BACKEND_CREDS_PATH = '/api/relay/credentials';

/**
 * Load credentials preferring the backend (so cleared localStorage / a fresh
 * device recovers), falling back to the localStorage cache. A backend hit is
 * mirrored into localStorage for fast synchronous reads next time.
 */
export async function loadCredentialsRemote(): Promise<RelayCredentials | null> {
    if (typeof fetch !== 'undefined') {
        try {
            const resp = await fetch(BACKEND_CREDS_PATH, { headers: { 'content-type': 'application/json' } });
            if (resp.ok) {
                const text = await resp.text();
                const data = text ? (JSON.parse(text) as RelayCredentials | null) : null;
                if (data && data.token && data.secretB64) {
                    const creds: RelayCredentials = { token: data.token, secretB64: data.secretB64 };
                    if (hasLocalStorage()) localStorage.setItem(LS_KEY, JSON.stringify(creds));
                    return creds;
                }
            }
        } catch {
            /* backend unavailable (relay/CDN/小程序) → fall back to local cache */
        }
    }
    return loadCredentials();
}

/**
 * Persist credentials to the localStorage cache and (best-effort) to the
 * backend so they survive a storage wipe. Backend failure is non-fatal: the
 * localStorage write already happened, matching the prior behavior.
 */
function saveCredentials(c: RelayCredentials): void {
    if (hasLocalStorage()) localStorage.setItem(LS_KEY, JSON.stringify(c));
    // Node(无头测试)/小程序 下没有可用的同源后端时,POST 静默失败,由本地缓存兜底。
    void persistCredentialsBackend(c);
}

/** POST credentials to the backend (issue #109). Best-effort, errors swallowed. */
async function persistCredentialsBackend(c: RelayCredentials): Promise<void> {
    if (typeof fetch === 'undefined') return;
    try {
        await fetch(BACKEND_CREDS_PATH, {
            method: 'POST',
            headers: { 'content-type': 'application/json' },
            body: JSON.stringify({ token: c.token, secretB64: c.secretB64, createdAt: Date.now() }),
        });
    } catch {
        /* best-effort */
    }
}

/** Clear credentials from both the backend (issue #109) and localStorage. */
export async function clearCredentials(): Promise<void> {
    if (hasLocalStorage()) localStorage.removeItem(LS_KEY);
    if (typeof fetch === 'undefined') return;
    try {
        await fetch(BACKEND_CREDS_PATH, { method: 'DELETE' });
    } catch {
        /* best-effort */
    }
}

/** 在中转上创建一个新账户;主密钥仅存本地。返回凭据。 */
export async function createAccount(serverUrl: string): Promise<RelayCredentials> {
    const secret = randomSecret();
    const { challenge, signature, publicKey } = authChallenge(secret);
    const data = await jsonFetch(`${serverUrl}/v1/auth`, {
        method: 'POST',
        headers: headers(),
        body: JSON.stringify({
            challenge: encodeBase64(challenge),
            signature: encodeBase64(signature),
            publicKey: encodeBase64(publicKey),
        }),
    });
    const creds = { token: (data as { token: string }).token, secretB64: encodeBase64(secret) };
    saveCredentials(creds);
    return creds;
}

/**
 * 审批一个终端/机器配对请求(daemon 跑 `happy auth login` 后产生)。
 * publicKeyB64 = daemon 的临时公钥(取自 `happy://terminal?<key>` 或 connect 链接 #key=)。
 * 用 dataKey 方案:把 [0, ...contentPublicKey] box 加密给该临时公钥。
 */
export async function approveTerminal(
    serverUrl: string,
    creds: RelayCredentials,
    publicKeyB64Url: string
): Promise<void> {
    const secret = decodeBase64(creds.secretB64);
    const contentKeyPair = await deriveContentKeyPair(secret);
    // base64url → base64
    const pkB64 = publicKeyB64Url.replace(/-/g, '+').replace(/_/g, '/');
    const ephemeralPub = decodeBase64(pkB64);

    // status 检查
    const status = await jsonFetch(
        `${serverUrl}/v1/auth/request/status?publicKey=${encodeURIComponent(encodeBase64(ephemeralPub))}`,
        { method: 'GET', headers: headers() }
    );
    if ((status as { status: string }).status !== 'pending') return; // 已批准或不存在

    const bundle = new Uint8Array(33);
    bundle[0] = 0;
    bundle.set(contentKeyPair.publicKey, 1);
    const answer = encryptForPublicKey(bundle, ephemeralPub);

    await jsonFetch(`${serverUrl}/v1/auth/response`, {
        method: 'POST',
        headers: headers(creds.token),
        body: JSON.stringify({ publicKey: encodeBase64(ephemeralPub), response: encodeBase64(answer) }),
    });
}

/** 拉取并解密本账户名下的机器。 */
export async function listMachines(serverUrl: string, creds: RelayCredentials): Promise<RelayMachine[]> {
    const secret = decodeBase64(creds.secretB64);
    const contentKeyPair = await deriveContentKeyPair(secret);
    const raw = await jsonFetch(`${serverUrl}/v1/machines`, { method: 'GET', headers: headers(creds.token) });
    const list: RawMachine[] = Array.isArray(raw)
        ? (raw as RawMachine[])
        : (raw as { machines?: RawMachine[] }).machines ?? [];
    const machines: RelayMachine[] = [];
    for (const m of list) {
        let encryptionKey: Uint8Array;
        let variant: Variant;
        if (m.dataEncryptionKey) {
            const wrapped = decodeBase64(m.dataEncryptionKey).slice(1); // 去版本字节
            const key = decryptBoxBundle(wrapped, contentKeyPair.secretKey);
            if (!key) continue; // 非本账户密钥可解,跳过
            encryptionKey = key;
            variant = 'dataKey';
        } else {
            encryptionKey = secret;
            variant = 'legacy';
        }
        machines.push({ id: m.id, active: !!m.active, encryptionKey, variant });
    }
    return machines;
}

/**
 * 建立中转长连接。
 * 默认 clientType=user-scoped: happy-server 会对无订阅/过期账户拒绝握手
 * (subscription_required / subscription_expired)。machine-scoped 仅 daemon 使用。
 */
export function connect(
    serverUrl: string,
    creds: RelayCredentials,
    opts?: { clientType?: 'user-scoped' | 'session-scoped' | 'machine-scoped'; machineId?: string }
): Promise<Socket> {
    const { origin, basePath } = splitRelayUrl(serverUrl);
    const clientType = opts?.clientType ?? 'user-scoped';
    const socket = io(origin, {
        auth: {
            token: creds.token,
            clientType,
            ...(opts?.machineId ? { machineId: opts.machineId } : {}),
            happyClient: CLIENT_ID,
        },
        path: `${basePath}/v1/updates`,
        // Allow the polling fallback, not websocket-only: on flaky mobile
        // networks a bare websocket can drop to nothing, whereas polling keeps
        // the session alive (and silently upgrades back to websocket). The
        // server already advertises both transports.
        transports: ['websocket', 'polling'],
        autoConnect: false,
        reconnection: true,
    });
    return new Promise((resolve, reject) => {
        const timer = setTimeout(() => reject(new Error('relay connect timeout')), 10_000);
        socket.once('connect', () => {
            clearTimeout(timer);
            resolve(socket);
        });
        socket.once('connect_error', e => {
            clearTimeout(timer);
            const msg = (e as Error)?.message ?? String(e);
            if (/subscription_required/i.test(msg)) {
                reject(new Error('需要有效订阅才能连接中转 (subscription_required)'));
                return;
            }
            if (/subscription_expired/i.test(msg)) {
                reject(new Error('订阅已过期，请续订后再连接 (subscription_expired)'));
                return;
            }
            reject(e);
        });
        socket.connect();
    });
}

/** 经中转对某台机器发起加密 RPC,返回解密后的结果对象。 */
export async function callMachine(
    socket: Socket,
    machine: RelayMachine,
    method: string,
    paramsObj: unknown
): Promise<unknown> {
    const params = encodeBase64(await encrypt(machine.encryptionKey, machine.variant, paramsObj));
    const ack = (await socket.timeout(30_000).emitWithAck('rpc-call', {
        method: `${machine.id}:${method}`,
        params,
    })) as { ok: boolean; result?: string; error?: string };
    if (!ack.ok || !ack.result) throw new Error(`RPC failed: ${ack.error ?? 'no result'}`);
    const out = await decrypt(machine.encryptionKey, machine.variant, decodeBase64(ack.result));
    if (out === null) throw new Error('RPC result decrypt failed');
    return out;
}

/** 便捷:经中转打到机器本地 Go 后端的 /api。 */
export async function proxyApi(
    socket: Socket,
    machine: RelayMachine,
    path: string,
    init?: { method?: string; body?: string; headers?: Record<string, string> }
): Promise<{ success: boolean; status?: number; body?: string; error?: string }> {
    return (await callMachine(socket, machine, '1agents-proxy', {
        method: init?.method ?? 'GET',
        path,
        body: init?.body,
        headers: init?.headers,
    })) as { success: boolean; status?: number; body?: string; error?: string };
}

/**
 * Agent 聊天流过中转(issue #17)。这三者只是 callMachine 的薄封装,配合
 * relayChatSocket.ts 把聊天 WS 改走中转:节点边车把 Go 聊天流镜像成 Happy
 * session 消息,H5 订阅 socket.on('update') 解密渲染(终端 ttyd 不走这里)。
 */
export interface RelayChatParams {
    workspaceId: string;
    taskId?: string;
    sessionId: string; // 1Agents chat session id
    agentType: string;
    profileId?: string;
    replyId?: string;
    agentRef?: string; // team expert persona pick (forwarded to the node bridge)
}

/** 在节点上开一条聊天桥,返回用于扇出过滤的 Happy session id。 */
export async function openChat(
    socket: Socket,
    machine: RelayMachine,
    params: RelayChatParams
): Promise<{ happySessionId: string }> {
    const r = (await callMachine(socket, machine, '1agents-chat-open', params)) as {
        success: boolean;
        happySessionId?: string;
        error?: string;
    };
    if (!r.success || !r.happySessionId) throw new Error(r.error ?? 'open chat failed');
    return { happySessionId: r.happySessionId };
}

/** 把一条原样 action JSON 经中转写进节点本地 Go 聊天 WS。 */
export async function sendChat(socket: Socket, machine: RelayMachine, sessionId: string, raw: string): Promise<void> {
    await callMachine(socket, machine, '1agents-chat-send', { sessionId, raw });
}

/** 关闭节点上的聊天桥(best-effort)。 */
export async function closeChat(socket: Socket, machine: RelayMachine, sessionId: string): Promise<void> {
    try {
        await callMachine(socket, machine, '1agents-chat-close', { sessionId });
    } catch {
        /* best-effort */
    }
}

/**
 * 终端流过中转(issue #17 终端那一路)。同样只是 callMachine 的薄封装,配合
 * relayTerminalSocket.ts 把终端 WS 改走中转:节点边车把本机 ttyd 的二进制帧
 * 镜像成 Happy session 消息,H5 订阅 socket.on('update') 解密后逐帧喂给 xterm。
 */
export interface RelayTerminalParams {
    termId: string;
    cols: number;
    rows: number;
}

/** 在节点上开一条终端桥,返回用于扇出过滤的 Happy session id。 */
export async function openTerminal(
    socket: Socket,
    machine: RelayMachine,
    params: RelayTerminalParams
): Promise<{ happySessionId: string }> {
    const r = (await callMachine(socket, machine, 'terminal-open', params)) as {
        success: boolean;
        happySessionId?: string;
        error?: string;
    };
    if (!r.success || !r.happySessionId) throw new Error(r.error ?? 'open terminal failed');
    return { happySessionId: r.happySessionId };
}

/** 把一帧原始 ttyd 字节(base64)经中转写进节点本地 ttyd WS。 */
export async function inputTerminal(socket: Socket, machine: RelayMachine, termId: string, raw: string): Promise<void> {
    await callMachine(socket, machine, 'terminal-input', { termId, raw });
}

/** 关闭节点上的终端桥(best-effort)。 */
export async function closeTerminal(socket: Socket, machine: RelayMachine, termId: string): Promise<void> {
    try {
        await callMachine(socket, machine, 'terminal-close', { termId });
    } catch {
        /* best-effort */
    }
}
