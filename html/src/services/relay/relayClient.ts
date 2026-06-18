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
export function loadCredentials(): RelayCredentials | null {
    if (!hasLocalStorage()) return null;
    try {
        const raw = localStorage.getItem(LS_KEY);
        return raw ? (JSON.parse(raw) as RelayCredentials) : null;
    } catch {
        return null;
    }
}
function saveCredentials(c: RelayCredentials): void {
    if (!hasLocalStorage()) return; // Node(无头测试)下由调用方自行持久化
    localStorage.setItem(LS_KEY, JSON.stringify(c));
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

/** 建立 user-scoped 长连接。 */
export function connect(serverUrl: string, creds: RelayCredentials): Promise<Socket> {
    const socket = io(serverUrl, {
        auth: { token: creds.token },
        path: '/v1/updates',
        transports: ['websocket'],
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
