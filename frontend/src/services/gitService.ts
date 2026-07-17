// Git API client — all /api/git/* calls go through here (AbortSignal-friendly).

export interface FileStatus {
    path: string;
    status: string; // M, A, D, R, U, ?, …
}

export interface GitStatus {
    branch: string;
    ahead: number;
    behind: number;
    staged: FileStatus[];
    unstaged: FileStatus[];
    untracked: FileStatus[];
    isRepo: boolean;
}

export interface WorktreeEntry {
    path: string;
    head: string;
    short: string;
    branch: string;
    message: string;
    isMain: boolean;
    isCurrent: boolean;
    ahead?: number;
    behind?: number;
}

export interface GraphCommit {
    hash: string;
    short: string;
    parents: string[];
    refs: string[];
    author: string;
    time: number;
    message: string;
}

export interface CommitFileEntry {
    status: string;
    path: string;
}

export interface BranchEntry {
    name: string;
    current: boolean;
}

export interface SubmoduleEntry {
    path: string;
    hash: string;
    short: string;
    desc: string;
    /** "" clean, "+" SHA differs, "-" not initialized, "U" conflict */
    flag: string;
    branch?: string;
    ahead?: number;
    behind?: number;
}

export type FetchOpts = { signal?: AbortSignal };

async function readError(res: Response): Promise<string> {
    try {
        const t = await res.text();
        return t || res.statusText || String(res.status);
    } catch {
        return res.statusText || String(res.status);
    }
}

async function ensureOk(res: Response): Promise<Response> {
    if (!res.ok) throw new Error(await readError(res));
    return res;
}

export const gitService = {
    async status(opts?: FetchOpts): Promise<GitStatus> {
        const res = await ensureOk(await fetch('/api/git/status', { signal: opts?.signal }));
        return res.json();
    },

    async worktrees(opts?: FetchOpts): Promise<WorktreeEntry[]> {
        const res = await ensureOk(await fetch('/api/git/worktrees', { signal: opts?.signal }));
        return res.json();
    },

    async worktreeStatus(path: string, opts?: FetchOpts): Promise<GitStatus> {
        const res = await ensureOk(
            await fetch(`/api/git/worktree-status?path=${encodeURIComponent(path)}`, {
                signal: opts?.signal,
            })
        );
        return res.json();
    },

    async graph(limit = 30, opts?: FetchOpts): Promise<GraphCommit[]> {
        const res = await ensureOk(await fetch(`/api/git/graph?limit=${limit}`, { signal: opts?.signal }));
        const raw: GraphCommit[] = await res.json();
        return raw.map(c => ({
            ...c,
            parents: c.parents || [],
            refs: c.refs || [],
        }));
    },

    async commitFiles(hash: string, opts?: FetchOpts): Promise<CommitFileEntry[]> {
        const res = await ensureOk(
            await fetch(`/api/git/commit-files?hash=${encodeURIComponent(hash)}`, {
                signal: opts?.signal,
            })
        );
        return res.json();
    },

    async commitDiff(hash: string, file: string, opts?: FetchOpts): Promise<string> {
        const res = await ensureOk(
            await fetch(`/api/git/commit-diff?hash=${encodeURIComponent(hash)}&file=${encodeURIComponent(file)}`, {
                signal: opts?.signal,
            })
        );
        return res.text();
    },

    async worktreeDiff(wtPath: string, file: string, opts?: FetchOpts): Promise<string> {
        const res = await ensureOk(
            await fetch(`/api/git/worktree-diff?path=${encodeURIComponent(wtPath)}&file=${encodeURIComponent(file)}`, {
                signal: opts?.signal,
            })
        );
        return res.text();
    },

    async diff(file: string, staged: boolean, opts?: FetchOpts): Promise<string> {
        const res = await ensureOk(
            await fetch(`/api/git/diff?file=${encodeURIComponent(file)}&staged=${staged}`, {
                signal: opts?.signal,
            })
        );
        return res.text();
    },

    async stage(file: string | null, opts?: FetchOpts): Promise<void> {
        const url = file ? `/api/git/stage?file=${encodeURIComponent(file)}` : '/api/git/stage?all=true';
        await ensureOk(await fetch(url, { method: 'POST', signal: opts?.signal }));
    },

    async unstage(file: string | null, opts?: FetchOpts): Promise<void> {
        const url = file ? `/api/git/unstage?file=${encodeURIComponent(file)}` : '/api/git/unstage?all=true';
        await ensureOk(await fetch(url, { method: 'POST', signal: opts?.signal }));
    },

    async discard(file: string, opts?: FetchOpts): Promise<void> {
        await ensureOk(
            await fetch(`/api/git/discard?file=${encodeURIComponent(file)}`, {
                method: 'POST',
                signal: opts?.signal,
            })
        );
    },

    async commit(message: string, opts?: FetchOpts): Promise<void> {
        await ensureOk(
            await fetch('/api/git/commit', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message }),
                signal: opts?.signal,
            })
        );
    },

    async aiCommit(opts?: FetchOpts): Promise<string> {
        const res = await fetch('/api/git/ai-commit', { method: 'POST', signal: opts?.signal });
        const data = await res.json().catch(() => ({}));
        if (!res.ok) throw new Error((data as { error?: string }).error || (await readError(res)));
        return (data as { message: string }).message;
    },

    async push(path?: string | null, opts?: FetchOpts): Promise<void> {
        const q = path ? `?path=${encodeURIComponent(path)}` : '';
        await ensureOk(await fetch(`/api/git/push${q}`, { method: 'POST', signal: opts?.signal }));
    },

    async pull(path?: string | null, opts?: FetchOpts): Promise<void> {
        const q = path ? `?path=${encodeURIComponent(path)}` : '';
        await ensureOk(await fetch(`/api/git/pull${q}`, { method: 'POST', signal: opts?.signal }));
    },

    async fetchRemote(path?: string | null, opts?: FetchOpts): Promise<void> {
        const q = path ? `?path=${encodeURIComponent(path)}` : '';
        await ensureOk(await fetch(`/api/git/fetch${q}`, { method: 'POST', signal: opts?.signal }));
    },

    async branches(opts?: FetchOpts): Promise<BranchEntry[]> {
        const res = await ensureOk(await fetch('/api/git/branches', { signal: opts?.signal }));
        return res.json();
    },

    async checkout(branch: string, create = false, opts?: FetchOpts): Promise<void> {
        await ensureOk(
            await fetch('/api/git/checkout', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ branch, create }),
                signal: opts?.signal,
            })
        );
    },

    async submodules(opts?: FetchOpts): Promise<SubmoduleEntry[]> {
        const res = await fetch('/api/git/submodules', { signal: opts?.signal });
        if (res.status === 404) {
            // Old backend binary without the route — surface empty rather than throw.
            console.warn('[git] /api/git/submodules not found; restart backend after rebuild');
            return [];
        }
        await ensureOk(res);
        return res.json();
    },
};
