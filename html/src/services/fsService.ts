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

    /**
     * Build a direct URL for the image preview. The browser fetches and decodes
     * the image itself — no base64 round-trip, no state, no in-memory dataURL string.
     * Used as <img src={fsService.imageUrl(entry.path)}>.
     */
    imageUrl(path: string): string {
        return `/api/fs/image/${path.split('/').map(encodeURIComponent).join('/')}`;
    },

    /**
     * Fetch a file (e.g. an image) as a Blob for download, avoiding the base64
     * overhead of readImage(). Returns a Blob along with a suggested filename.
     */
    async fetchAsBlob(path: string): Promise<{ blob: Blob; filename: string }> {
        const url = this.imageUrl(path);
        const res = await fetch(url);
        if (!res.ok) throw new Error(await res.text());
        const blob = await res.blob();
        const filename = path.split('/').pop() || 'download';
        return { blob, filename };
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

    async search(query: string, tag: string): Promise<FsEntry[]> {
        const res = await fetch(`/api/fs/search?query=${encodeURIComponent(query)}&tag=${encodeURIComponent(tag)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async setContext(path: string): Promise<void> {
        const res = await fetch('/api/context/set', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path }),
        });
        if (!res.ok) throw new Error(await res.text());
    },
};
