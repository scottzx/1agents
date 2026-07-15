import { apiFetch } from './apiClient';
import { FsEntry } from '../types';
import { getPlatformBridge } from '../platform/bridge';

export const fsService = {
    async list(relPath: string, refresh = false): Promise<FsEntry[]> {
        const params = new URLSearchParams({ path: relPath || '.' });
        if (refresh) params.set('refresh', 'true');
        const res = await apiFetch(`/fs/list?${params.toString()}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async read(path: string): Promise<string> {
        const res = await apiFetch(`/fs/read?path=${encodeURIComponent(path)}`);
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
        const res = await apiFetch(`/fs/image?path=${encodeURIComponent(path)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.text();
    },

    async write(path: string, content: string): Promise<void> {
        const res = await apiFetch(`/fs/write?path=${encodeURIComponent(path)}`, {
            method: 'POST',
            headers: { 'Content-Type': 'text/plain; charset=utf-8' },
            body: content,
        });
        if (!res.ok) throw new Error(await res.text());
    },

    /**
     * Upload an arbitrary file. The backend saves it to /tmp under a randomized
     * name (preserving the original base name + extension) and returns the
     * absolute path, which the chat input drops in as text for the local agent.
     */
    async upload(file: File): Promise<{ path: string; name: string }> {
        // Route through the platform bridge so the desktop/mobile shells can
        // diverge later. The web bridge runs the same multipart POST as before.
        return getPlatformBridge().uploadFile(file);
    },

    async search(query: string, tag: string): Promise<FsEntry[]> {
        const res = await apiFetch(`/fs/search?query=${encodeURIComponent(query)}&tag=${encodeURIComponent(tag)}`);
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    },

    async setContext(path: string): Promise<void> {
        const res = await apiFetch('/context/set', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ path }),
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async rename(oldPath: string, newPath: string): Promise<void> {
        const res = await fetch(
            `/api/fs/rename?oldPath=${encodeURIComponent(oldPath)}&newPath=${encodeURIComponent(newPath)}`,
            {
                method: 'POST',
            }
        );
        if (!res.ok) throw new Error(await res.text());
    },

    async mkdir(path: string): Promise<void> {
        const res = await fetch(`/api/fs/mkdir?path=${encodeURIComponent(path)}`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async copy(src: string, dst: string): Promise<void> {
        const res = await fetch(`/api/fs/copy?src=${encodeURIComponent(src)}&dst=${encodeURIComponent(dst)}`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await res.text());
    },

    async delete(path: string, recursive = false): Promise<void> {
        const res = await fetch(`/api/fs/delete?path=${encodeURIComponent(path)}&recursive=${recursive}`, {
            method: 'DELETE',
        });
        if (!res.ok) throw new Error(await res.text());
    },
};
