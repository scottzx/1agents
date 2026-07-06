/**
 * 后端传输路由(方案 A)。
 *
 * 1Agents 的所有 /api 调用经此统一入口,按"后端从哪来"自动选择传输:
 *  - direct: H5 由节点本机托管,同源 /api 可达 → httpFetch('/api'+path)       (直连一直在)
 *  - relay:  H5 由中转/CDN 托管,同源无 /api  → 经中转打到已配对的远程节点    (旁路)
 *  - none:   中转模式但还没选节点 → 调用抛错,UI 引导去「中转旁路」配对
 *
 * apiFetch 返回标准 Response,因此各 service 的 res.ok/.json()/.text() 无需改动。
 * 注意:二进制(图片 blob / 上传)暂不经中转,仍走直连(详见 fsService)。
 *
 * fetch / localStorage 都经 getPlatformBridge() 取用,这样同一份代码在小程序
 * (无 window.fetch / localStorage)上也能跑:web/Tauri 用全局 fetch + localStorage,
 * 小程序用 Taro.request + Taro storage。小程序没有同源 /api,改用 setDirectBackend()
 * 指定后端地址(direct 模式 + 绝对 base)。
 */
// signals-core (not @preact/signals) keeps this transport module
// platform-agnostic for core/: it's the dependency-free reactive primitive that
// @preact/signals itself re-exports `signal` from, so preact components still
// react to `backendTarget` exactly as before.
import { signal } from '@preact/signals-core';
import type { Socket } from 'socket.io-client';
import { getPlatformBridge } from '../platform/bridge';
import { connect, listMachines, proxyApi, loadCredentialsRemote, type RelayMachine } from './relay/relayClient';
import { activeDevice, deviceById, connectDevice, setActiveDeviceId } from './relay/devices';

export type BackendTarget =
    | { mode: 'probing' }
    | { mode: 'direct' }
    | { mode: 'relay'; socket: Socket; machine: RelayMachine }
    | { mode: 'none' };

export const backendTarget = signal<BackendTarget>({ mode: 'probing' });

/**
 * 当前激活的远程设备 id(多设备项目视图,#114)。
 *
 * null = 本机直连;非空 = 经宿主机的代理路由层(#111)透传到目标设备:
 * 此时 direct 模式下所有 /api 调用自动加 `/api/proxy/{deviceId}` 前缀,
 * 由宿主机反向代理到目标设备的同名 /api 路由,UI 表现与本地一致。
 *
 * 仅在 direct 模式生效:relay 模式本身已是远程节点的隧道,不再二次代理。
 */
export const activeDeviceId = signal<string | null>(null);

/** 切换当前 API 路由目标。null = 恢复本机直连。 */
export function setActiveDevice(deviceId: string | null): void {
    activeDeviceId.value = deviceId;
}

const LS_URL = 'oneagents.relay.url';
const LS_NODE = 'oneagents.relay.node';

/**
 * Absolute origin to prefix before `/api` in direct mode. Empty on the web (the
 * SPA is same-origin with the backend, so a relative URL is correct). The 小程序
 * host has no same-origin backend, so it sets this via setDirectBackend().
 */
let directBaseUrl = '';

export function relayUrl(): string {
    // Default to the origin the H5 is served from, including the build base
    // path (__BASE_PATH__ = '' at root, '/tunnels' under a subpath mount), so a
    // subpath-hosted SPA targets https://host/tunnels, not the bare origin.
    return getPlatformBridge().storage.get(LS_URL) || window.location.origin + __BASE_PATH__;
}

/**
 * Point the client at an explicit backend origin and switch to direct mode.
 * Used by hosts with no same-origin /api to probe — notably the 小程序 client,
 * which connects to a user-configured backend address.
 */
export function setDirectBackend(baseUrl: string): void {
    directBaseUrl = normalizeOrigin(baseUrl);
    backendTarget.value = { mode: 'direct' };
}

/**
 * Trim surrounding whitespace and strip trailing slashes from a backend/relay
 * origin. Shared by every settings form that lets a user type an address (web
 * relay pairing, 小程序 backend override) so they normalize identically.
 */
export function normalizeOrigin(raw: string): string {
    return raw.trim().replace(/\/+$/, '');
}

/** True when `raw` is a usable http(s) origin (the bar settings inputs validate against). */
export function isHttpOrigin(raw: string): boolean {
    return /^https?:\/\//.test(raw.trim());
}

async function probeDirect(): Promise<boolean> {
    try {
        const r = await getPlatformBridge().httpFetch(directBaseUrl + '/api/access/status', {
            signal: AbortSignal.timeout(4000),
        });
        return r.ok;
    } catch {
        return false;
    }
}

/** 解析后端目标:本机直连优先;否则用已存凭据自动连中转 + 选已配对节点;再否则 none。 */
export async function initBackend(): Promise<BackendTarget> {
    if (await probeDirect()) {
        backendTarget.value = { mode: 'direct' };
        return backendTarget.value;
    }
    // 多设备模式:优先用「当前激活的设备档案」直连那台机器(只需 token+machineId+machineKey,
    // 无需账户主密钥)。连不上则继续往下走旧的账户/节点路径,最终落到 none → 门禁页。
    const dev = activeDevice();
    if (dev) {
        try {
            const { socket, machine } = await connectDevice(dev);
            backendTarget.value = { mode: 'relay', socket, machine };
            return backendTarget.value;
        } catch {
            /* 落到下面的账户路径 / none */
        }
    }
    const creds = await loadCredentialsRemote();
    if (creds) {
        try {
            const socket = await connect(relayUrl(), creds);
            const machines = await listMachines(relayUrl(), creds);
            const savedId = getPlatformBridge().storage.get(LS_NODE);
            const machine =
                machines.find(m => m.id === savedId && m.active) ?? machines.find(m => m.active) ?? machines[0];
            if (machine) {
                getPlatformBridge().storage.set(LS_NODE, machine.id);
                backendTarget.value = { mode: 'relay', socket, machine };
                return backendTarget.value;
            }
            socket.close();
        } catch {
            /* 落到 none */
        }
    }
    backendTarget.value = { mode: 'none' };
    return backendTarget.value;
}

/** 由「中转旁路」面板调用:把某个节点设为当前后端。 */
export function setRelayNode(socket: Socket, machine: RelayMachine): void {
    getPlatformBridge().storage.set(LS_NODE, machine.id);
    backendTarget.value = { mode: 'relay', socket, machine };
}

/**
 * 切换/添加设备后由设备面板调用:连上该设备档案并设为当前激活后端。
 * 切换前会关掉旧 relay socket,避免连接泄漏。
 */
export async function switchToDevice(machineId: string): Promise<void> {
    const dev = deviceById(machineId);
    if (!dev) throw new Error('设备不存在');
    const prev = backendTarget.value;
    const { socket, machine } = await connectDevice(dev);
    setActiveDeviceId(machineId);
    backendTarget.value = { mode: 'relay', socket, machine };
    if (prev.mode === 'relay' && prev.socket !== socket) prev.socket.close();
}

export function isBackendReady(): boolean {
    const m = backendTarget.value.mode;
    return m === 'direct' || m === 'relay';
}

// 连续中转失败计数:节点 daemon 掉线/被顶替时,socket 仍连着中转(账号没变),
// 但打到该 machine 的 RPC 会失败/超时。累计到阈值就判定"节点失联"。
let relayFailStreak = 0;
const RELAY_FAIL_THRESHOLD = 2;

/**
 * 中转节点失联(daemon 掉线 / 被新账号顶替 / 后端重启)时的恢复:关掉指向死节点
 * 的 socket、把后端目标翻回 none。backendTarget 是信号,顶层 app 订阅后会重新显示
 * 配对门禁,让用户重选节点/重新配对,而不是卡死在中转模式打不通的界面。
 */
export function reportBackendUnreachable(): void {
    const t = backendTarget.value;
    if (t.mode === 'relay') {
        try {
            t.socket.close();
        } catch {
            /* ignore */
        }
    }
    relayFailStreak = 0;
    backendTarget.value = { mode: 'none' };
}

/** 统一后端调用。direct → 同源 /api;relay → 经中转打到节点;none/probing → 抛错。 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
    const t = backendTarget.value;
    if (t.mode === 'direct') {
        // 多设备:激活了远程设备时,经宿主机代理路由层(#111)透传到目标设备。
        // /api/proxy/{deviceId} 前缀由宿主机剥离后转发同名 /api 路由。
        // 调用方已显式写了 /proxy/... 路径(如按设备拉项目列表)时不再叠加,避免二次代理。
        const dev = activeDeviceId.value;
        const prefix = dev && !path.startsWith('/proxy/') ? `/api/proxy/${encodeURIComponent(dev)}` : '';
        return getPlatformBridge().httpFetch(directBaseUrl + prefix + '/api' + path, init);
    }
    if (t.mode === 'relay') {
        const method = (init?.method || 'GET').toUpperCase();
        const rawBody = init?.body;
        const body =
            rawBody === null || rawBody === undefined
                ? undefined
                : typeof rawBody === 'string'
                  ? rawBody
                  : String(rawBody);
        const headers =
            init?.headers && !(init.headers instanceof Headers) ? (init.headers as Record<string, string>) : undefined;
        let r: Awaited<ReturnType<typeof proxyApi>>;
        try {
            r = await proxyApi(t.socket, t.machine, '/api' + path, { method, body, headers });
        } catch (e) {
            // RPC 失败/超时 = 打不到该节点的 daemon。累计到阈值判定节点失联并回退到
            // 门禁(见 reportBackendUnreachable),避免卡死;单次抖动不误伤。
            if (++relayFailStreak >= RELAY_FAIL_THRESHOLD) reportBackendUnreachable();
            throw e;
        }
        relayFailStreak = 0;
        return new Response(r.body ?? '', {
            status: r.status ?? (r.success ? 200 : 502),
            headers: { 'content-type': 'application/json' },
        });
    }
    throw new Error('NO_BACKEND: 未连接后端,请先在「中转旁路」配对/选择一个节点');
}
