export const accessService = {
    async checkStatus(): Promise<{ required: boolean; authenticated: boolean }> {
        const res = await fetch('/api/access/status');
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return {
            required: !!data.required,
            authenticated: !!data.authenticated,
        };
    },

    async generateToken(): Promise<string> {
        const res = await fetch('/api/access/generate', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        return data.token;
    },

    async revokeToken(): Promise<void> {
        const res = await fetch('/api/access/revoke', { method: 'POST' });
        if (!res.ok) throw new Error(await res.text());
    },

    async pingTunnel(): Promise<void> {
        await fetch('/api/tunnel/status').catch(() => {
            /* best-effort */
        });
    },
};
