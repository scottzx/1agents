/**
 * 后端传输路由(方案 A)。
 *
 * 1Agents 的所有 /api 调用经此统一入口,按"后端从哪来"自动选择传输:
 *  - direct: H5 由节点本机托管,同源 /api 可达 → fetch('/api'+path)         (直连一直在)
 *  - relay:  H5 由中转/CDN 托管,同源无 /api  → 经中转打到已配对的远程节点    (旁路)
 *  - none:   中转模式但还没选节点 → 调用抛错,UI 引导去「中转旁路」配对
 *
 * apiFetch 返回标准 Response,因此各 service 的 res.ok/.json()/.text() 无需改动。
 * 注意:二进制(图片 blob / 上传)暂不经中转,仍走直连(详见 fsService)。
 */
// signals-core (not @preact/signals) keeps this transport module
// platform-agnostic for core/: it's the dependency-free reactive primitive that
// @preact/signals itself re-exports `signal` from, so preact components still
// react to `backendTarget` exactly as before.
import { signal } from '@preact/signals-core';
import type { Socket } from 'socket.io-client';
import { connect, listMachines, proxyApi, loadCredentials, type RelayMachine } from './relay/relayClient';

export type BackendTarget =
    | { mode: 'probing' }
    | { mode: 'direct' }
    | { mode: 'relay'; socket: Socket; machine: RelayMachine }
    | { mode: 'none' };

export const backendTarget = signal<BackendTarget>({ mode: 'probing' });

const LS_URL = 'oneagents.relay.url';
const LS_NODE = 'oneagents.relay.node';

export function relayUrl(): string {
    // Default to the origin the H5 is served from, including the build base
    // path (__BASE_PATH__ = '' at root, '/tunnels' under a subpath mount), so a
    // subpath-hosted SPA targets https://host/tunnels, not the bare origin.
    return localStorage.getItem(LS_URL) || window.location.origin + __BASE_PATH__;
}

async function probeDirect(): Promise<boolean> {
    try {
        const r = await fetch('/api/access/status', { signal: AbortSignal.timeout(4000) });
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
    const creds = loadCredentials();
    if (creds) {
        try {
            const socket = await connect(relayUrl(), creds);
            const machines = await listMachines(relayUrl(), creds);
            const savedId = localStorage.getItem(LS_NODE);
            const machine =
                machines.find(m => m.id === savedId && m.active) ?? machines.find(m => m.active) ?? machines[0];
            if (machine) {
                localStorage.setItem(LS_NODE, machine.id);
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
    localStorage.setItem(LS_NODE, machine.id);
    backendTarget.value = { mode: 'relay', socket, machine };
}

export function isBackendReady(): boolean {
    const m = backendTarget.value.mode;
    return m === 'direct' || m === 'relay';
}

/** 统一后端调用。direct → 同源 /api;relay → 经中转打到节点;none/probing → 抛错。 */
export async function apiFetch(path: string, init?: RequestInit): Promise<Response> {
    const t = backendTarget.value;
    if (t.mode === 'direct') {
        return fetch('/api' + path, init);
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
        const r = await proxyApi(t.socket, t.machine, '/api' + path, { method, body, headers });
        return new Response(r.body ?? '', {
            status: r.status ?? (r.success ? 200 : 502),
            headers: { 'content-type': 'application/json' },
        });
    }
    throw new Error('NO_BACKEND: 未连接后端,请先在「中转旁路」配对/选择一个节点');
}
