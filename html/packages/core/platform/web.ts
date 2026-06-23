// Web (browser) platform bridge — the default host.

import type { PlatformBridge, UploadResult } from './bridge';

export class WebPlatformBridge implements PlatformBridge {
    /**
     * Upload via the same multipart POST the workbench has always used.
     * Kept byte-for-byte identical to the original fsService.upload so the
     * web path is unchanged by the bridge indirection.
     */
    async uploadFile(file: File): Promise<UploadResult> {
        const fd = new FormData();
        fd.append('file', file);
        const res = await fetch('/api/fs/upload', { method: 'POST', body: fd });
        if (!res.ok) throw new Error(await res.text());
        return res.json();
    }

    async openExternal(url: string): Promise<void> {
        window.open(url, '_blank', 'noopener');
    }
}
