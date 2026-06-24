/**
 * 客户端「设备档案」(多后端切换)。
 *
 * 一个设备档案 = 从机器端扫到的凭据 bundle(LocalMachinePanel 的配置二维码),
 * 足以直连那台机器的本地后端:
 *   - token     → 中转 socket 鉴权(connect 只用 token)
 *   - machineId → 中转上机器实体 id,也是 RPC 寻址前缀(`${machineId}:${method}`)
 *   - machineKey→ 该机器的 dataKey(AES-256-GCM),即 RPC 载荷的 encryptionKey
 *   - serverUrl → 中转地址
 * 注意:这条路径不需要账户主密钥(secret),所以客户端可同时存多台机器、随时切换。
 *
 * 档案 + 当前激活 id 都存 localStorage(经 platform bridge,兼容小程序)。
 * 重命名只改本地 name,不动凭据。
 */
import type { Socket } from 'socket.io-client';
import { getPlatformBridge } from '../../platform/bridge';
import { decodeBase64 } from './crypto';
import { connect, type RelayMachine } from './relayClient';

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
        obj = JSON.parse(raw.trim());
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

/** 按设备档案建立到该机器的连接,返回 socket + 可直接用于 RPC 的 RelayMachine。 */
export async function connectDevice(p: DeviceProfile): Promise<{ socket: Socket; machine: RelayMachine }> {
    // connect 只用 token;secret 在这条路径用不到,传空串即可。
    const socket = await connect(p.serverUrl, { token: p.token, secretB64: '' });
    const machine: RelayMachine = {
        id: p.machineId,
        active: true,
        encryptionKey: decodeBase64(p.machineKey),
        variant: 'dataKey',
    };
    return { socket, machine };
}
