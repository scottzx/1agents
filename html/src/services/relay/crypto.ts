/**
 * Relay 客户端加密核心(浏览器版,移植自 Happy)。
 *
 * 复刻 packages/happy-{agent,app} 的线缆加密,字节布局必须完全一致,否则与中转/机器互通失败:
 *  - dataKey RPC 载荷:AES-256-GCM,bundle = version(1=0) + nonce(12) + ciphertext + authTag(16)
 *  - legacy RPC 载荷:tweetnacl.secretbox,bundle = nonce(24) + box
 *  - 机器密钥解包:tweetnacl.box,bundle = ephemeralPub(32) + nonce(24) + ciphertext
 *  - 内容密钥派生:HMAC-SHA512 树 → SHA-512(seed)[0:32] → box keypair from secretKey
 *
 * AES/HMAC/SHA 用浏览器 WebCrypto(需安全上下文:HTTPS 或 localhost / WebView 自定义协议)。
 */
import nacl from 'tweetnacl';

const td = new TextDecoder();
const te = new TextEncoder();

// --- base64 ---
export function encodeBase64(buf: Uint8Array): string {
    let s = '';
    for (let i = 0; i < buf.length; i++) s += String.fromCharCode(buf[i]);
    return btoa(s);
}
export function decodeBase64(b64: string): Uint8Array {
    const s = atob(b64);
    const out = new Uint8Array(s.length);
    for (let i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
    return out;
}

// --- WebCrypto helpers ---
async function sha512(data: Uint8Array): Promise<Uint8Array> {
    return new Uint8Array(await crypto.subtle.digest('SHA-512', data));
}
async function hmacSha512(key: Uint8Array, data: Uint8Array): Promise<Uint8Array> {
    const k = await crypto.subtle.importKey('raw', key, { name: 'HMAC', hash: 'SHA-512' }, false, ['sign']);
    return new Uint8Array(await crypto.subtle.sign('HMAC', k, data));
}

// --- key derivation tree (HMAC-SHA512), 同 deriveKey('Happy EnCoder', ['content']) ---
async function deriveKey(master: Uint8Array, usage: string, path: string[]): Promise<Uint8Array> {
    let I = await hmacSha512(te.encode(usage + ' Master Seed'), master);
    let key = I.slice(0, 32);
    let chainCode = I.slice(32);
    for (const index of path) {
        const data = new Uint8Array([0x00, ...te.encode(index)]);
        I = await hmacSha512(chainCode, data);
        key = I.slice(0, 32);
        chainCode = I.slice(32);
    }
    return key;
}

export async function deriveContentKeyPair(secret: Uint8Array): Promise<nacl.BoxKeyPair> {
    const seed = await deriveKey(secret, 'Happy EnCoder', ['content']);
    const hashedSeed = await sha512(seed); // libsodium crypto_box_seed_keypair = SHA-512(seed)[0:32]
    return nacl.box.keyPair.fromSecretKey(hashedSeed.slice(0, 32));
}

// --- box(公钥)解包 / 封包 ---
export function decryptBoxBundle(bundle: Uint8Array, recipientSecretKey: Uint8Array): Uint8Array | null {
    if (bundle.length < 32 + 24) return null;
    const ephemeralPublicKey = bundle.slice(0, 32);
    const nonce = bundle.slice(32, 56);
    const ciphertext = bundle.slice(56);
    const out = nacl.box.open(ciphertext, nonce, ephemeralPublicKey, recipientSecretKey);
    return out ? new Uint8Array(out) : null;
}
export function encryptForPublicKey(data: Uint8Array, recipientPublicKey: Uint8Array): Uint8Array {
    const eph = nacl.box.keyPair();
    const nonce = nacl.randomBytes(nacl.box.nonceLength);
    const ct = nacl.box(data, nonce, recipientPublicKey, eph.secretKey);
    const out = new Uint8Array(32 + 24 + ct.length);
    out.set(eph.publicKey, 0);
    out.set(nonce, 32);
    out.set(ct, 56);
    return out;
}

// --- RPC 载荷加密:dataKey(AES-256-GCM)/ legacy(secretbox) ---
export type Variant = 'dataKey' | 'legacy';

async function encryptDataKey(data: unknown, dataKey: Uint8Array): Promise<Uint8Array> {
    const nonce = nacl.randomBytes(12);
    const key = await crypto.subtle.importKey('raw', dataKey, { name: 'AES-GCM' }, false, ['encrypt']);
    const ctTag = new Uint8Array(
        await crypto.subtle.encrypt({ name: 'AES-GCM', iv: nonce }, key, te.encode(JSON.stringify(data)))
    ); // WebCrypto: 输出即 ciphertext||authTag(16),与 Happy 布局一致
    const bundle = new Uint8Array(1 + 12 + ctTag.length);
    bundle[0] = 0;
    bundle.set(nonce, 1);
    bundle.set(ctTag, 13);
    return bundle;
}
async function decryptDataKey(bundle: Uint8Array, dataKey: Uint8Array): Promise<unknown | null> {
    if (bundle.length < 1 + 12 + 16 || bundle[0] !== 0) return null;
    const nonce = bundle.slice(1, 13);
    const ctTag = bundle.slice(13);
    try {
        const key = await crypto.subtle.importKey('raw', dataKey, { name: 'AES-GCM' }, false, ['decrypt']);
        const pt = new Uint8Array(await crypto.subtle.decrypt({ name: 'AES-GCM', iv: nonce }, key, ctTag));
        return JSON.parse(td.decode(pt));
    } catch {
        return null;
    }
}
function encryptLegacy(data: unknown, secret: Uint8Array): Uint8Array {
    const nonce = nacl.randomBytes(nacl.secretbox.nonceLength);
    const ct = nacl.secretbox(te.encode(JSON.stringify(data)), nonce, secret);
    const out = new Uint8Array(nonce.length + ct.length);
    out.set(nonce);
    out.set(ct, nonce.length);
    return out;
}
function decryptLegacy(data: Uint8Array, secret: Uint8Array): unknown | null {
    try {
        const nonce = data.slice(0, nacl.secretbox.nonceLength);
        const ct = data.slice(nacl.secretbox.nonceLength);
        const pt = nacl.secretbox.open(ct, nonce, secret);
        return pt ? JSON.parse(td.decode(pt)) : null;
    } catch {
        return null;
    }
}

export async function encrypt(key: Uint8Array, variant: Variant, data: unknown): Promise<Uint8Array> {
    return variant === 'legacy' ? encryptLegacy(data, key) : encryptDataKey(data, key);
}
export async function decrypt(key: Uint8Array, variant: Variant, data: Uint8Array): Promise<unknown | null> {
    return variant === 'legacy' ? decryptLegacy(data, key) : decryptDataKey(data, key);
}

// --- 创建账户用的挑战签名(Ed25519) ---
export function authChallenge(secret: Uint8Array): {
    challenge: Uint8Array;
    publicKey: Uint8Array;
    signature: Uint8Array;
} {
    const signing = nacl.sign.keyPair.fromSeed(secret);
    const challenge = nacl.randomBytes(32);
    const signature = nacl.sign.detached(challenge, signing.secretKey);
    return { challenge, publicKey: signing.publicKey, signature };
}

export function randomSecret(): Uint8Array {
    return nacl.randomBytes(32);
}
