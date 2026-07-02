import { apiFetch } from './apiClient';

// CLI 生命周期服务 — probes installed CLI tools (lark-cli, etc.) and their auth
// state. Mirrors backend/internal/sources/cli handler.

export interface CLIStatus {
    tool: string;
    installed: boolean;
    path?: string;
    version?: string;
    latestVersion?: string;
    updateAvailable: boolean;
    authenticated: boolean;
    authAccount?: string;
    authIdentity?: string;
    tokenStatus?: string;
    authExpiresAt?: string;
    refreshExpiresAt?: string;
    scopes?: string[];
    loginHint?: string;
    updateHint?: string;
    installHint?: string;
    error?: string;
    checkedAt: string;
}

export const sourceCliService = {
    /** GET /api/sources/cli/{tool}/status — cached probe result. */
    async cliStatus(tool: string): Promise<CLIStatus> {
        const res = await apiFetch(`/sources/cli/${encodeURIComponent(tool)}/status`);
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CLIStatus;
    },

    /** POST /api/sources/cli/{tool}/recheck — forces a fresh probe and returns new status. */
    async cliRecheck(tool: string): Promise<CLIStatus> {
        const res = await apiFetch(`/sources/cli/${encodeURIComponent(tool)}/recheck`, {
            method: 'POST',
        });
        if (!res.ok) throw new Error(await res.text());
        return (await res.json()) as CLIStatus;
    },
};
