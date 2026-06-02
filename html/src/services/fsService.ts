import { FsEntry } from '../components/types';

export const fsService = {
    async list(relPath: string): Promise<FsEntry[]> {
        const res = await fetch(`/api/fs/list?path=${encodeURIComponent(relPath || '.')}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async read(path: string): Promise<string> {
        const res = await fetch(`/api/fs/read?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.text();
    },

    async readImage(path: string): Promise<string> {
        const res = await fetch(`/api/fs/image?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.text();
    },

    async write(path: string, content: string): Promise<void> {
        const res = await fetch(`/api/fs/write?path=${encodeURIComponent(path)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
            body: content,
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async setContext(path: string): Promise<void> {
        const res = await fetch('/api/context/set', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path }),
        });
        if (!res.ok) throw new Error(await res.text());
    }
};
