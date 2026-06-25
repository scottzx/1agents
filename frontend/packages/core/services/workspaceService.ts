import { apiFetch } from './apiClient';
import { Workspace } from '../types';

export const workspaceService = {
    /**
     * 列出工作空间。传 deviceId 时拉取该远程设备的项目(经 #111 代理路由层),
     * 不改动全局路由目标;无值则走当前激活后端。apiFetch 会统一补 `/api` 前缀,
     * 因此远程路径写成 `/proxy/{id}/api/workspace/list` → `/api/proxy/{id}/api/workspace/list`,
     * 宿主机剥掉 `/api/proxy/{id}` 后把 `/api/workspace/list` 转发到目标设备。
     * 远程项目对象会被打上 deviceId 标记,供 Sidebar 分组与点击切路由用。
     */
    async list(deviceId?: string): Promise<Workspace[]> {
        const path = deviceId ? `/proxy/${encodeURIComponent(deviceId)}/api/workspace/list` : '/workspace/list';
        const res = await apiFetch(path);
        if (!res.ok) throw new Error(await res.text());
        const list: Workspace[] = await res.json();
        return deviceId ? list.map(ws => ({ ...ws, deviceId })) : list;
    },

    async create(ws: Workspace): Promise<void> {
        const res = await apiFetch('/workspace/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(ws),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async update(ws: Workspace): Promise<void> {
        const res = await apiFetch('/workspace/update', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(ws),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async delete(id: string): Promise<void> {
        const res = await apiFetch(`/workspace/delete?id=${encodeURIComponent(id)}`, {
            method: 'DELETE',
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async reorder(ids: string[]): Promise<void> {
        const res = await apiFetch('/workspace/reorder', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ids }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async listDirectories(path: string): Promise<{
        currentPath: string;
        parentPath: string | null;
        directories: { name: string; path: string }[];
    }> {
        const res = await apiFetch(`/workspace/list-directories?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async createDirectory(parentPath: string, name: string): Promise<string> {
        const res = await apiFetch('/workspace/create-directory', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ parentPath, name }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return data.path;
    },

    async getCcConnectUrl(workspaceId: string, theme: string, lang: string, path?: string): Promise<string> {
        const res = await apiFetch('/cc-connect/url', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ workspace: workspaceId, theme, lang, path }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return data.url;
    },
};
