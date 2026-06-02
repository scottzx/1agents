import { Workspace } from '../components/types';

export const workspaceService = {
    async list(): Promise<Workspace[]> {
        const res = await fetch('/api/workspace/list');
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async create(ws: Workspace): Promise<void> {
        const res = await fetch('/api/workspace/create', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(ws),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async update(ws: Workspace): Promise<void> {
        const res = await fetch('/api/workspace/update', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(ws),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async delete(id: string): Promise<void> {
        const res = await fetch(`/api/workspace/delete?id=${encodeURIComponent(id)}`, {
            method: 'DELETE',
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async listDirectories(path: string): Promise<{
        currentPath: string;
        parentPath: string | null;
        directories: { name: string; path: string }[];
    }> {
        const res = await fetch(`/api/workspace/list-directories?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async getCcConnectUrl(workspaceId: string, theme: string, lang: string): Promise<string> {
        const res = await fetch('/api/cc-connect/url', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ workspace: workspaceId, theme, lang }),
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return data.url;
    },
};
