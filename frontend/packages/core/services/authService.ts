/**
 * 用户登录(手机号 + 验证码)—— H5 / 小程序共用。
 *
 * 身份模型:公钥账户为根,手机号是其上的标签。verifyPhone 本地生成一对
 * TweetNaCl 公钥(与 relayClient.createAccount 同一套),签名挑战发给
 * happy-server 的 /v1/auth/phone/verify,过 mock 验证码后换回 token;凭据
 * {token, secretB64} 落到平台存储(web=localStorage,小程序=Taro storage),
 * key 与 relayClient 一致,后续 E2E/配对复用这把 secret。
 *
 * 全程走平台桥(getPlatformBridge),所以在没有全局 fetch / localStorage 的
 * 小程序宿主里也能跑;serverUrl 由调用方传入(小程序传 getBackendBase(),
 * H5 传其 relay 地址),保持本服务与平台解耦。
 */
import { getPlatformBridge } from '../platform/bridge';
import { encodeBase64, randomSecret, authChallenge } from './relay/crypto';

const LS_CREDS = 'oneagents.relay.creds';
const CLIENT_ID = 'app/1agents-auth';

export interface AuthCredentials {
    token: string;
    secretB64: string;
}

/** 验证码错误(后端 400 invalid_code)时抛出,供登录 UI 区分提示。 */
export class InvalidCodeError extends Error {
    constructor() {
        super('invalid_code');
        this.name = 'InvalidCodeError';
    }
}

function authHeaders(): Record<string, string> {
    return { 'content-type': 'application/json', 'X-Happy-Client': CLIENT_ID };
}

async function postJson(serverUrl: string, path: string, body: unknown): Promise<unknown> {
    const resp = await getPlatformBridge().httpFetch(`${serverUrl}${path}`, {
        method: 'POST',
        headers: authHeaders(),
        body: JSON.stringify(body),
    });
    const text = await resp.text();
    const data: unknown = text ? JSON.parse(text) : null;
    if (!resp.ok) {
        throw new Error((data as { error?: string } | null)?.error || `HTTP ${resp.status}`);
    }
    return data;
}

/** 发送验证码(脚手架:后端 dev 不发真短信,固定码 123456)。 */
export async function sendCode(serverUrl: string, phone: string): Promise<void> {
    await postJson(serverUrl, '/v1/auth/phone/send-code', { phone });
}

/**
 * 验证码登录:生成公钥账户密钥对 → 签名挑战 + 验证码 → 换 token → 落地凭据。
 * 成功返回凭据;验证码错误抛 InvalidCodeError。
 */
export async function verifyPhone(serverUrl: string, phone: string, code: string): Promise<AuthCredentials> {
    const secret = randomSecret();
    const { challenge, signature, publicKey } = authChallenge(secret);
    let data: unknown;
    try {
        data = await postJson(serverUrl, '/v1/auth/phone/verify', {
            phone,
            code,
            publicKey: encodeBase64(publicKey),
            challenge: encodeBase64(challenge),
            signature: encodeBase64(signature),
        });
    } catch (e) {
        if ((e as Error).message === 'invalid_code') throw new InvalidCodeError();
        throw e;
    }
    const creds: AuthCredentials = { token: (data as { token: string }).token, secretB64: encodeBase64(secret) };
    getPlatformBridge().storage.set(LS_CREDS, JSON.stringify(creds));
    return creds;
}

/** 读取本地登录凭据(无则 null)。 */
export function loadCredentials(): AuthCredentials | null {
    try {
        const raw = getPlatformBridge().storage.get(LS_CREDS);
        return raw ? (JSON.parse(raw) as AuthCredentials) : null;
    } catch {
        return null;
    }
}

/** 是否已登录(有 token)。 */
export function isLoggedIn(): boolean {
    const c = loadCredentials();
    return !!(c && c.token);
}

/** 退出登录:清除本地凭据。 */
export function logout(): void {
    getPlatformBridge().storage.remove(LS_CREDS);
}
