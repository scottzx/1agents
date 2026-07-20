/**
 * 客户端「设备档案」(多后端切换) — Model B。
 *
 * 一个设备档案 = 从机器端扫到的凭据 bundle(LocalMachinePanel 的配置二维码):
 *   - machineId → 中转上机器实体 id,也是 RPC 寻址前缀(`${machineId}:${method}`)
 *   - machineKey→ 该机器的 dataKey(AES-256-GCM),即 RPC 载荷的 encryptionKey
 *   - serverUrl → 中转地址
 *   - token     → bundle 内 machine session(兼容字段);建连优先用用户账号 token
 *
 * 建连(B1):优先 `loadCredentials()` 的用户 token + clientType=user-scoped,
 * 中转会对无订阅/过期拒绝握手。RPC 仍用档案 machineKey;路由要求机器与用户
 * 同属一个 account(与 happy-server rpc:userId:method 房间一致)。
 *
 * 档案 + 当前激活 id 都存 localStorage(经 platform bridge,兼容小程序)。
 */
import type { Socket } from 'socket.io-client';
import { getPlatformBridge } from '../../platform/bridge';
import { decodeBase64 } from './crypto';
import { connect, loadCredentials, loadCredentialsRemote, type RelayMachine } from './relayClient';
import { getSubscription } from '../subscriptionService';

const LS_DEVICES = 'oneagents.relay.devices';
const LS_ACTIVE = 'oneagents.relay.activeDevice';

export interface DeviceProfile {
    machineId: string;
    name: string; // 用户可改的展示名,仅本地
    serverUrl: string;
    token: string;
    machineKey: string; // base64
    addedAt: number;
}

function read<T>(key: string, fallback: T): T {
    try {
        const raw = getPlatformBridge().storage.get(key);
        return raw ? (JSON.parse(raw) as T) : fallback;
    } catch {
        return fallback;
    }
}

export function loadDevices(): DeviceProfile[] {
    const list = read<DeviceProfile[]>(LS_DEVICES, []);
    return Array.isArray(list) ? list : [];
}

function saveDevices(list: DeviceProfile[]): void {
    getPlatformBridge().storage.set(LS_DEVICES, JSON.stringify(list));
}

export function getActiveDeviceId(): string | null {
    return getPlatformBridge().storage.get(LS_ACTIVE) || null;
}

export function setActiveDeviceId(id: string): void {
    getPlatformBridge().storage.set(LS_ACTIVE, id);
}

export function deviceById(id: string): DeviceProfile | null {
    return loadDevices().find(d => d.machineId === id) ?? null;
}

export function activeDevice(): DeviceProfile | null {
    const id = getActiveDeviceId();
    if (!id) return null;
    return deviceById(id);
}

/**
 * 解析扫到/粘贴的凭据 bundle。只接受 LocalMachinePanel 生成的
 * `{type:'1agents-relay', serverUrl, token, machineId, machineKey}` JSON。
 * 缺少任一必需字段返回 null(让 UI 给出明确报错,而不是存一条连不上的脏档案)。
 */
export function parseDeviceBundle(raw: string): DeviceProfile | null {
    let obj: Record<string, unknown>;
    try {
        // 容忍复制粘贴时混入的尾随分号(如 `…"machineId":"x";}`);扫码不会有。
        const cleaned = raw
            .trim()
            .replace(/;\s*}\s*$/, '}')
            .replace(/}\s*;\s*$/, '}');
        obj = JSON.parse(cleaned);
    } catch {
        return null;
    }
    if (obj.type !== '1agents-relay') return null;
    const serverUrl = typeof obj.serverUrl === 'string' ? obj.serverUrl : '';
    const token = typeof obj.token === 'string' ? obj.token : '';
    const machineId = typeof obj.machineId === 'string' ? obj.machineId : '';
    const machineKey = typeof obj.machineKey === 'string' ? obj.machineKey : '';
    const hostname = typeof obj.hostname === 'string' ? obj.hostname.trim() : '';
    if (!serverUrl || !token || !machineId || !machineKey) return null;
    // 默认名优先用机器端 mDNS 主机名,否则退回 machineId 前缀;客户端可再改 alias。
    return { machineId, name: hostname || machineId.slice(0, 8), serverUrl, token, machineKey, addedAt: Date.now() };
}

/**
 * 加入/更新一台设备(按 machineId 去重)。重复扫码时刷新凭据但保留已有的展示名。
 * 返回最终落库的档案。
 */
export function upsertDevice(p: DeviceProfile): DeviceProfile {
    const list = loadDevices();
    const idx = list.findIndex(d => d.machineId === p.machineId);
    if (idx >= 0) {
        const merged = { ...p, name: list[idx].name, addedAt: list[idx].addedAt };
        list[idx] = merged;
        saveDevices(list);
        return merged;
    }
    list.push(p);
    saveDevices(list);
    return p;
}

export function renameDevice(machineId: string, name: string): void {
    const list = loadDevices();
    const idx = list.findIndex(d => d.machineId === machineId);
    if (idx < 0) return;
    list[idx] = { ...list[idx], name: name.trim() || list[idx].name };
    saveDevices(list);
}

export function removeDevice(machineId: string): void {
    saveDevices(loadDevices().filter(d => d.machineId !== machineId));
    if (getActiveDeviceId() === machineId) getPlatformBridge().storage.remove(LS_ACTIVE);
}

/**
 * 按设备档案建立到该机器的连接。
 * 1) 客户端订阅预检 2) 用户账号 token 建连(中转验订阅) 3) RPC 用 machineKey。
 *
 * 注意:happy-server RPC 房间是 rpc:{userId}:{method},设备 daemon 必须与
 * 当前登录用户同属一个 account,否则会出现「RPC method not available」。
 */
export async function connectDevice(p: DeviceProfile): Promise<{ socket: Socket; machine: RelayMachine }> {
    const accountCreds = (await loadCredentialsRemote()) ?? loadCredentials();
    if (!accountCreds?.token) {
        throw new Error('请先登录中转账户后再连接设备');
    }

    // 客户端第一道门(中转握手还会再验 subscription_required / expired)
    try {
        const sub = await getSubscription();
        if (sub.status === 'expired') {
            throw new Error('订阅已过期，请续订后再连接设备');
        }
        if (sub.status !== 'active') {
            throw new Error('需要有效订阅才能连接设备');
        }
    } catch (e) {
        const msg = e instanceof Error ? e.message : String(e);
        // 明确的业务错误直接抛出;网络/解析错误则交给中转握手兜底
        if (/订阅|尚未创建|NoRelayAccount/.test(msg)) throw e instanceof Error ? e : new Error(msg);
    }

    const serverUrl = (p.serverUrl || '').replace(/\/+$/, '');
    if (!serverUrl) throw new Error('设备档案缺少中转地址 serverUrl');

    // B1: 用户 token + user-scoped → 中转强制验订阅
    const socket = await connect(
        serverUrl,
        { token: accountCreds.token, secretB64: accountCreds.secretB64 || '' },
        { clientType: 'user-scoped' }
    );

    const machine: RelayMachine = {
        id: p.machineId,
        active: true,
        encryptionKey: decodeBase64(p.machineKey),
        variant: 'dataKey',
    };
    return { socket, machine };
}
