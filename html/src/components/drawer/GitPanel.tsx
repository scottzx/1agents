import { h, Component, Fragment } from 'preact';
import { t, type Lang } from '../i18n';

// ── Types ──────────────────────────────────────────────────────────────────

interface FileStatus {
    path: string;
    status: string; // M, A, D, R, ?
}

interface GitStatus {
    branch: string;
    ahead: number;
    behind: number;
    staged: FileStatus[];
    unstaged: FileStatus[];
    untracked: FileStatus[];
    isRepo: boolean;
}

interface WorktreeEntry {
    path: string;
    head: string;
    short: string;
    branch: string;
    message: string;
    isMain: boolean;
    isCurrent: boolean;
}

interface GraphCommit {
    hash: string;
    short: string;
    parents: string[];
    refs: string[];
    author: string;
    time: number;
    message: string;
}

interface GraphRow extends GraphCommit {
    nodeLane: number;
    onMain: boolean;
    isMerge: boolean;
    incomingLanes: number[]; // lanes (from above) that terminate at this commit
    parentLanes: number[]; // lanes (below) this commit's parents continue in
    aboveLanes: number[]; // every lane carrying a line into this row from above
    belowLanes: number[]; // every lane carrying a line out of this row below
}

interface CommitFileEntry {
    status: string;
    path: string;
}

interface GitPanelProps {
    workdir: string;
    activeWorkspaceId: string;
    onLoadingChange?: (loading: boolean) => void;
    onRegisterRefresh?: (refreshFn: () => void) => void;
    language: Lang;
}

interface GitPanelState {
    status: GitStatus | null;
    loading: boolean;
    commitMsg: string;
    committing: boolean;
    pushPullLoading: 'push' | 'pull' | null;
    // diff (working-tree inline)
    diffFile: string | null;
    diffStaged: boolean;
    diffContent: string;
    diffLoading: boolean;
    // toast
    toast: string;
    // collapsible sections
    stagedCollapsed: boolean;
    unstagedCollapsed: boolean;
    untrackedCollapsed: boolean;
    // ai commit message
    aiLoading: boolean;
    // worktrees
    worktrees: WorktreeEntry[];
    worktreesLoading: boolean;
    // worktree switcher
    selectedWorktreePath: string | null;
    worktreeSwitcherOpen: boolean;
    // selected worktree status (read-only view)
    worktreeStatus: GitStatus | null;
    worktreeStatusLoading: boolean;
    // commit box collapsible
    commitBoxCollapsed: boolean;
    // graph history
    graph: GraphCommit[];
    graphLoading: boolean;
    graphExpanded: boolean;
    // commit detail (file list)
    expandedCommitHash: string | null;
    commitFiles: CommitFileEntry[];
    commitFilesLoading: boolean;
    // diff panel shared by commit-file and worktree-file diffs
    commitDiffFile: string | null;
    commitDiffContent: string;
    commitDiffLoading: boolean;
}

// ── Status label map ───────────────────────────────────────────────────────

const STATUS_KEY: Record<string, string> = {
    M: 'git.status.M',
    A: 'git.status.A',
    D: 'git.status.D',
    R: 'git.status.R',
    C: 'git.status.C',
    '?': 'git.status.?',
};

const STATUS_COLOR: Record<string, string> = {
    M: 'git-status-m',
    A: 'git-status-a',
    D: 'git-status-d',
    R: 'git-status-r',
    '?': 'git-status-u',
};

function relativeTime(ts: number, language: Lang): string {
    const diff = Math.floor(Date.now() / 1000) - ts;
    if (diff < 60) return t('git.time.justNow', language);
    if (diff < 3600) return t('git.time.minutesAgo', language, { n: Math.floor(diff / 60) });
    if (diff < 86400) return t('git.time.hoursAgo', language, { n: Math.floor(diff / 3600) });
    if (diff < 86400 * 7) return t('git.time.daysAgo', language, { n: Math.floor(diff / 86400) });
    return new Date(ts * 1000).toLocaleDateString(t('git.time.dateFmtLocale', language), {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    });
}

// ── Beautiful Premium SVG Icons ────────────────────────────────────────────

const IconRefresh = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M23 4v6h-6M1 20v-6h6" />
        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
    </svg>
);

const IconBranch = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <line x1="6" x2="6" y1="3" y2="15" />
        <circle cx="18" cy="6" r="3" />
        <circle cx="6" cy="18" r="3" />
        <path d="M18 9a9 9 0 0 1-9 9" />
    </svg>
);

const IconPlus = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <line x1="12" x2="12" y1="5" y2="19" />
        <line x1="5" x2="19" y1="12" y2="12" />
    </svg>
);

const IconMinus = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <line x1="5" x2="19" y1="12" y2="12" />
    </svg>
);

const IconChevron = (expanded: boolean) => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
        style={`transform: rotate(${expanded ? 90 : 0}deg); transition: transform 0.25s cubic-bezier(0.16, 1, 0.3, 1)`}
    >
        <polyline points="9 18 15 12 9 6" />
    </svg>
);

const IconCommit = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <circle cx="12" cy="12" r="4" />
        <line x1="1.05" x2="7" y1="12" y2="12" />
        <line x1="17.01" x2="22.96" y1="12" y2="12" />
    </svg>
);

const IconPush = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M12 2v14M12 2l-4 4M12 2l4 4M4 22h16" />
    </svg>
);

const IconPull = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M12 16V2M12 16l-4-4M12 16l4-4M4 22h16" />
    </svg>
);

const IconTrash = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <polyline points="3 6 5 6 21 6" />
        <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
    </svg>
);

// eslint-disable-next-line @typescript-eslint/no-unused-vars
const IconSparkles = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M9.813 15.904L9 21L8.188 15.904L3 15L8.188 14.096L9 9L9.813 14.096L15 15L9.813 15.904Z" />
        <path d="M19.071 4.929L18.5 8.5L17.929 4.929L14.358 4.358L17.929 3.786L18.5 0.214L19.071 3.786L22.642 4.358L19.071 4.929Z" />
    </svg>
);

// ── Main Component ─────────────────────────────────────────────────────────

export class GitPanel extends Component<GitPanelProps, GitPanelState> {
    private _refreshTimer: ReturnType<typeof setInterval> | null = null;

    constructor(props: GitPanelProps) {
        super(props);
        this.state = {
            status: null,
            loading: false,
            commitMsg: '',
            committing: false,
            pushPullLoading: null,
            diffFile: null,
            diffStaged: false,
            diffContent: '',
            diffLoading: false,
            toast: '',
            // collapsibles
            stagedCollapsed: false,
            unstagedCollapsed: false,
            untrackedCollapsed: false,
            // AI loading state
            aiLoading: false,
            // worktrees
            worktrees: [],
            worktreesLoading: false,
            // worktree switcher
            selectedWorktreePath: null,
            worktreeSwitcherOpen: false,
            // selected worktree status
            worktreeStatus: null,
            worktreeStatusLoading: false,
            // commit box
            commitBoxCollapsed: true,
            // graph history
            graph: [],
            graphLoading: false,
            graphExpanded: true,
            // commit detail
            expandedCommitHash: null,
            commitFiles: [],
            commitFilesLoading: false,
            // diff panel (commit/worktree file diffs)
            commitDiffFile: null,
            commitDiffContent: '',
            commitDiffLoading: false,
        };
    }

    componentDidMount() {
        if (this.props.onRegisterRefresh) {
            this.props.onRegisterRefresh(this.refresh);
        }
        if (this.props.onLoadingChange) {
            this.props.onLoadingChange(this.state.loading);
        }
        this.refresh();
        this._refreshTimer = setInterval(() => {
            this.refresh();
        }, 15000);
    }

    componentDidUpdate(prevProps: GitPanelProps, prevState: GitPanelState) {
        if (prevProps.activeWorkspaceId !== this.props.activeWorkspaceId) {
            this.setState({
                diffFile: null,
                diffContent: '',
                worktreeSwitcherOpen: false,
                selectedWorktreePath: null,
                worktreeStatus: null,
                expandedCommitHash: null,
                commitFiles: [],
                commitDiffFile: null,
                commitDiffContent: '',
                commitBoxCollapsed: true,
            });
            this.refresh();
        }
        if (prevState.loading !== this.state.loading) {
            this.props.onLoadingChange?.(this.state.loading);
        }
        if (prevProps.onRegisterRefresh !== this.props.onRegisterRefresh && this.props.onRegisterRefresh) {
            this.props.onRegisterRefresh(this.refresh);
        }
        // When selectedWorktreePath changes to a non-current worktree, fetch its status.
        if (prevState.selectedWorktreePath !== this.state.selectedWorktreePath) {
            const { selectedWorktreePath } = this.state;
            if (selectedWorktreePath !== null && !this.isViewingCurrent()) {
                this.loadSelectedWorktreeStatus(selectedWorktreePath);
            } else {
                this.setState({ worktreeStatus: null });
            }
        }
    }

    componentWillUnmount() {
        if (this._refreshTimer) clearInterval(this._refreshTimer);
        if (this.props.onRegisterRefresh) {
            this.props.onRegisterRefresh(() => {});
        }
        if (this.props.onLoadingChange) {
            this.props.onLoadingChange(false);
        }
    }

    // ── Helpers ────────────────────────────────────────────────────────────

    currentWorktreePath(): string | null {
        return this.state.worktrees.find(w => w.isCurrent)?.path ?? null;
    }

    isViewingCurrent(): boolean {
        const { selectedWorktreePath } = this.state;
        return selectedWorktreePath === null || selectedWorktreePath === this.currentWorktreePath();
    }

    // ── Data fetching ──────────────────────────────────────────────────────

    refresh = async () => {
        this.setState({ loading: true });
        try {
            const res = await fetch('/api/git/status');
            if (!res.ok) throw new Error(await res.text());
            const status: GitStatus = await res.json();
            this.setState({ status, loading: false });
        } catch (err) {
            console.error('[git] status error:', err);
            this.setState({ loading: false });
        }
        this.loadWorktrees();
        this.loadGraph();
        // Re-fetch worktree status on poll if viewing a non-current worktree.
        const { selectedWorktreePath } = this.state;
        if (selectedWorktreePath !== null && !this.isViewingCurrent()) {
            this.loadSelectedWorktreeStatus(selectedWorktreePath);
        }
    };

    loadWorktrees = async () => {
        this.setState({ worktreesLoading: true });
        try {
            const res = await fetch('/api/git/worktrees');
            if (!res.ok) throw new Error(await res.text());
            const worktrees: WorktreeEntry[] = await res.json();
            this.setState({ worktrees, worktreesLoading: false });
        } catch (err) {
            console.error('[git] worktrees error:', err);
            this.setState({ worktreesLoading: false });
        }
    };

    loadSelectedWorktreeStatus = async (path: string) => {
        this.setState({ worktreeStatusLoading: true });
        try {
            const res = await fetch(`/api/git/worktree-status?path=${encodeURIComponent(path)}`);
            if (!res.ok) throw new Error(await res.text());
            const worktreeStatus: GitStatus = await res.json();
            this.setState({ worktreeStatus, worktreeStatusLoading: false });
        } catch (err) {
            console.error('[git] worktree-status error:', err);
            this.setState({ worktreeStatusLoading: false });
        }
    };

    loadGraph = async () => {
        this.setState({ graphLoading: true });
        try {
            const res = await fetch('/api/git/graph?limit=100');
            if (!res.ok) throw new Error(await res.text());
            const raw: GraphCommit[] = await res.json();
            // Normalize: root commits have no parents/refs → backend may emit null
            const graph = raw.map(c => ({
                ...c,
                parents: c.parents || [],
                refs: c.refs || [],
            }));
            this.setState({ graph, graphLoading: false });
        } catch (err) {
            console.error('[git] graph error:', err);
            this.setState({ graphLoading: false });
        }
    };

    toggleCommit = async (hash: string) => {
        if (this.state.expandedCommitHash === hash) {
            this.setState({ expandedCommitHash: null, commitFiles: [], commitDiffFile: null, commitDiffContent: '' });
            return;
        }
        this.setState({
            expandedCommitHash: hash,
            commitFiles: [],
            commitFilesLoading: true,
            commitDiffFile: null,
            commitDiffContent: '',
        });
        try {
            const res = await fetch(`/api/git/commit-files?hash=${encodeURIComponent(hash)}`);
            if (!res.ok) throw new Error(await res.text());
            const commitFiles: CommitFileEntry[] = await res.json();
            this.setState({ commitFiles, commitFilesLoading: false });
        } catch (err) {
            console.error('[git] commit-files error:', err);
            this.setState({ commitFilesLoading: false });
        }
    };

    openCommitDiff = async (hash: string, file: string) => {
        // Toggle off if clicking the same file again.
        if (this.state.commitDiffFile === file) {
            this.setState({ commitDiffFile: null, commitDiffContent: '' });
            return;
        }
        this.setState({ commitDiffFile: file, commitDiffLoading: true, commitDiffContent: '' });
        try {
            const res = await fetch(
                `/api/git/commit-diff?hash=${encodeURIComponent(hash)}&file=${encodeURIComponent(file)}`
            );
            if (!res.ok) throw new Error(await res.text());
            const text = await res.text();
            this.setState({ commitDiffContent: text, commitDiffLoading: false });
        } catch (err) {
            this.setState({ commitDiffContent: `Error: ${err}`, commitDiffLoading: false });
        }
    };

    openWorktreeDiff = async (wtPath: string, file: string) => {
        // Toggle off if clicking the same file again.
        if (this.state.commitDiffFile === file) {
            this.setState({ commitDiffFile: null, commitDiffContent: '' });
            return;
        }
        this.setState({ commitDiffFile: file, commitDiffLoading: true, commitDiffContent: '' });
        try {
            const res = await fetch(
                `/api/git/worktree-diff?path=${encodeURIComponent(wtPath)}&file=${encodeURIComponent(file)}`
            );
            if (!res.ok) throw new Error(await res.text());
            const text = await res.text();
            this.setState({ commitDiffContent: text, commitDiffLoading: false });
        } catch (err) {
            this.setState({ commitDiffContent: `Error: ${err}`, commitDiffLoading: false });
        }
    };

    loadDiff = async (file: string, staged: boolean) => {
        if (this.state.diffFile === file && this.state.diffStaged === staged) {
            this.setState({ diffFile: null, diffContent: '' });
            return;
        }
        this.setState({ diffFile: file, diffStaged: staged, diffLoading: true, diffContent: '' });
        try {
            const res = await fetch(`/api/git/diff?file=${encodeURIComponent(file)}&staged=${staged}`);
            if (!res.ok) throw new Error(await res.text());
            const text = await res.text();
            this.setState({ diffContent: text, diffLoading: false });
        } catch (err) {
            this.setState({ diffContent: `Error loading diff: ${err}`, diffLoading: false });
        }
    };

    // ── Actions ────────────────────────────────────────────────────────────

    stage = async (file: string | null) => {
        const url = file ? `/api/git/stage?file=${encodeURIComponent(file)}` : '/api/git/stage?all=true';
        await fetch(url, { method: 'POST' });
        if (file && this.state.diffFile === file && !this.state.diffStaged) {
            this.setState({ diffFile: null, diffContent: '' });
        }
        this.refresh();
    };

    unstage = async (file: string | null) => {
        const url = file ? `/api/git/unstage?file=${encodeURIComponent(file)}` : '/api/git/unstage?all=true';
        await fetch(url, { method: 'POST' });
        if (file && this.state.diffFile === file && this.state.diffStaged) {
            this.setState({ diffFile: null, diffContent: '' });
        }
        this.refresh();
    };

    discard = async (file: string) => {
        const confirmDiscard = window.confirm(t('git.discardConfirm', this.props.language, { file }));
        if (!confirmDiscard) return;

        this.setState({ loading: true });
        try {
            const res = await fetch(`/api/git/discard?file=${encodeURIComponent(file)}`, { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            this.showToast(t('git.toast.discarded', this.props.language));
            if (this.state.diffFile === file) {
                this.setState({ diffFile: null, diffContent: '' });
            }
            this.refresh();
        } catch (err) {
            this.showToast(t('git.toast.discardFailed', this.props.language, { err: String(err) }));
            this.setState({ loading: false });
        }
    };

    commit = async () => {
        const { commitMsg } = this.state;
        if (!commitMsg.trim()) return;
        this.setState({ committing: true });
        try {
            const res = await fetch('/api/git/commit', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ message: commitMsg.trim() }),
            });
            if (!res.ok) throw new Error(await res.text());
            this.setState({ commitMsg: '', committing: false });
            this.showToast(t('git.toast.committed', this.props.language));
            this.refresh();
        } catch (err) {
            this.setState({ committing: false });
            this.showToast(t('git.toast.commitFailed', this.props.language, { err: String(err) }));
        }
    };

    generateAICommit = async () => {
        this.setState({ aiLoading: true });
        this.showToast(t('git.toast.aiAnalyzing', this.props.language));
        try {
            const res = await fetch('/api/git/ai-commit', { method: 'POST' });
            const data = await res.json();
            if (!res.ok) {
                throw new Error(data.error || t('git.toast.aiFailedFallback', this.props.language));
            }
            this.setState({ commitMsg: data.message, aiLoading: false });
            this.showToast(t('git.toast.aiSuccess', this.props.language));
        } catch (err) {
            this.setState({ aiLoading: false });
            const errMsg = (err as Error)?.message || String(err);
            this.showToast(t('git.toast.aiFailed', this.props.language, { msg: errMsg }));
        }
    };

    pushOrPull = async (action: 'push' | 'pull') => {
        this.setState({ pushPullLoading: action });
        try {
            const res = await fetch(`/api/git/${action}`, { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            this.showToast(
                t(action === 'push' ? 'git.toast.pushSuccess' : 'git.toast.pullSuccess', this.props.language)
            );
            this.refresh();
        } catch (err) {
            this.showToast(
                t(
                    action === 'push' ? 'git.toast.pushFailedPrefix' : 'git.toast.pullFailedPrefix',
                    this.props.language,
                    { err: String(err) }
                )
            );
        } finally {
            this.setState({ pushPullLoading: null });
        }
    };

    showToast = (msg: string) => {
        this.setState({ toast: msg });
        setTimeout(() => this.setState({ toast: '' }), 3000);
    };

    toggleSection = (section: 'staged' | 'unstaged' | 'untracked') => {
        if (section === 'staged') {
            this.setState({ stagedCollapsed: !this.state.stagedCollapsed });
        } else if (section === 'unstaged') {
            this.setState({ unstagedCollapsed: !this.state.unstagedCollapsed });
        } else if (section === 'untracked') {
            this.setState({ untrackedCollapsed: !this.state.untrackedCollapsed });
        }
    };

    // ── Graph layout ───────────────────────────────────────────────────────
    //
    // Trunk-anchored lane assignment: the main branch's first-parent chain is
    // pinned to lane 0 (the leftmost vertical trunk). Side branches occupy
    // lanes ≥1, so a branch tip whose first parent is on main reads as a fork
    // off the trunk, and a merge commit on main pulls a side lane back into 0.
    // Per-row lane layout. Each row carries its own rail-drawing instructions
    // (incoming/parent connectors + straight pass-through lanes) so the SVG rail
    // can be rendered one row at a time — staying perfectly aligned with the text
    // rows even when a commit is expanded. Lane 0 is the main trunk.
    buildGraphLayout(commits: GraphCommit[]): { rows: GraphRow[]; maxLanes: number } {
        const byHash = new Map(commits.map(c => [c.hash, c]));

        const mainTip =
            commits.find(c => c.refs.some(r => r === 'main' || r === 'master')) ||
            commits.find(c => c.refs.some(r => r.endsWith('/main') || r.endsWith('/master'))) ||
            commits[0];

        const onMain = new Set<string>();
        let cur: GraphCommit | undefined = mainTip;
        while (cur && !onMain.has(cur.hash)) {
            onMain.add(cur.hash);
            cur = cur.parents[0] ? byHash.get(cur.parents[0]) : undefined;
        }

        const lanes: (string | null)[] = []; // lanes[i] = hash expected next in lane i
        // Returns the first free lane index >= `from`, extending the array as
        // needed. Must never return < from (else a side branch could seize the
        // reserved main trunk lane 0).
        const firstFree = (from: number): number => {
            let i = from;
            while (i < lanes.length && lanes[i] !== null) i++;
            while (lanes.length <= i) lanes.push(null);
            return i;
        };

        const rows: GraphRow[] = [];

        commits.forEach(commit => {
            const isOnMain = onMain.has(commit.hash);

            const incomingLanes: number[] = [];
            lanes.forEach((hh, i) => {
                if (hh === commit.hash) incomingLanes.push(i);
            });

            let nodeLane: number;
            if (isOnMain) nodeLane = 0;
            else if (incomingLanes.length) nodeLane = Math.min(...incomingLanes);
            else nodeLane = firstFree(1);
            while (lanes.length <= nodeLane) lanes.push(null);

            const aboveSnap = lanes.slice();

            incomingLanes.forEach(i => {
                lanes[i] = null;
            });

            const parentLanes: number[] = [];
            commit.parents.forEach((p, idx) => {
                if (onMain.has(p)) {
                    lanes[0] = p;
                    if (!parentLanes.includes(0)) parentLanes.push(0);
                    return;
                }
                const existing = lanes.findIndex(h => h === p);
                if (existing >= 0) {
                    if (!parentLanes.includes(existing)) parentLanes.push(existing);
                    return;
                }
                let lane: number;
                if (idx === 0 && !isOnMain) lane = nodeLane;
                else lane = firstFree(1); // extra/merge parents never take the trunk lane 0
                lanes[lane] = p;
                if (!parentLanes.includes(lane)) parentLanes.push(lane);
            });

            const belowSnap = lanes.slice();

            // Every lane carrying a line into/out of this row. The renderer draws a
            // segment for each active half independently, so lines never break and
            // the trunk runs straight through fork/merge rows (diagonals layer on top).
            const aboveLanes: number[] = [];
            aboveSnap.forEach((hh, i) => {
                if (hh !== null) aboveLanes.push(i);
            });
            const belowLanes: number[] = [];
            belowSnap.forEach((hh, i) => {
                if (hh !== null) belowLanes.push(i);
            });

            rows.push({
                ...commit,
                nodeLane,
                onMain: isOnMain,
                isMerge: commit.parents.length > 1,
                incomingLanes,
                parentLanes,
                aboveLanes,
                belowLanes,
            });
        });

        let hi = 0;
        rows.forEach(r => {
            [r.nodeLane, ...r.aboveLanes, ...r.belowLanes].forEach(L => {
                if (L > hi) hi = L;
            });
        });

        return { rows, maxLanes: hi + 1 };
    }

    // ── Render helpers ─────────────────────────────────────────────────────

    parseDiffLines(content: string) {
        if (!content) return [];
        const lines = content.split('\n');
        let oldLine = 0;
        let newLine = 0;

        const result: {
            oldLineNum: number | '';
            newLineNum: number | '';
            type: 'ctx' | 'add' | 'del' | 'hunk' | 'header';
            text: string;
        }[] = [];

        for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            if (i === lines.length - 1 && line === '') continue; // Skip final split newline

            if (line.startsWith('@@ ')) {
                const match = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
                if (match) {
                    oldLine = parseInt(match[1], 10);
                    newLine = parseInt(match[2], 10);
                }
                result.push({ oldLineNum: '', newLineNum: '', type: 'hunk', text: line });
            } else if (line.startsWith('+++ ') || line.startsWith('--- ')) {
                result.push({ oldLineNum: '', newLineNum: '', type: 'header', text: line });
            } else if (line.startsWith('+')) {
                result.push({ oldLineNum: '', newLineNum: newLine++, type: 'add', text: line });
            } else if (line.startsWith('-')) {
                result.push({ oldLineNum: oldLine++, newLineNum: '', type: 'del', text: line });
            } else if (line.startsWith(' ')) {
                result.push({ oldLineNum: oldLine++, newLineNum: newLine++, type: 'ctx', text: line });
            } else {
                result.push({ oldLineNum: '', newLineNum: '', type: 'header', text: line });
            }
        }
        return result;
    }

    // Shared diff panel markup used by working-tree diffs, commit diffs, and worktree diffs.
    renderDiffPanel(file: string, content: string, loading: boolean, onClose: () => void) {
        const { language } = this.props;
        const parsedLines = this.parseDiffLines(content);

        return (
            <div class="git-diff-panel" onClick={e => e.stopPropagation()}>
                <div class="git-diff-header">
                    <span class="git-diff-title">{file}</span>
                    <button class="git-diff-close-btn" onClick={onClose} title={t('git.diff.close', language)}>
                        ×
                    </button>
                </div>
                {loading ? (
                    <div class="git-diff-loading">
                        <div class="git-spinner" />
                        <span>{t('git.diff.loading', language)}</span>
                    </div>
                ) : parsedLines.length > 0 ? (
                    <div class="git-diff-wrapper">
                        <div class="git-diff-table">
                            {parsedLines.map((line, idx) => {
                                const lineCls = `diff-line-${line.type}`;
                                return (
                                    <div key={idx} class={`git-diff-row ${lineCls}`}>
                                        <div class="diff-num diff-num-old">{line.oldLineNum}</div>
                                        <div class="diff-num diff-num-new">{line.newLineNum}</div>
                                        <div class="diff-char">
                                            {line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' '}
                                        </div>
                                        <div class="diff-text">
                                            {line.type === 'add' || line.type === 'del'
                                                ? line.text.substring(1)
                                                : line.text}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                ) : (
                    <div class="git-diff-empty">{t('git.diff.empty', language)}</div>
                )}
            </div>
        );
    }

    renderDiff() {
        const { diffFile, diffContent, diffLoading } = this.state;
        if (!diffFile) return null;
        return this.renderDiffPanel(diffFile, diffContent, diffLoading, () =>
            this.setState({ diffFile: null, diffContent: '' })
        );
    }

    renderFileRow(file: FileStatus, section: 'staged' | 'unstaged' | 'untracked') {
        const { diffFile, diffStaged } = this.state;
        const { language } = this.props;
        const isStaged = section === 'staged';
        const isOpen = diffFile === file.path && diffStaged === isStaged;
        const statusCls = STATUS_COLOR[file.status] || 'git-status-u';
        const label = STATUS_KEY[file.status] ? t(STATUS_KEY[file.status], language) : file.status;

        return (
            <Fragment key={`${section}-${file.path}`}>
                <div class={`git-file-row ${isOpen ? 'open' : ''}`}>
                    <span class={`git-file-status ${statusCls}`} title={label}>
                        {file.status}
                    </span>
                    <span
                        class="git-file-path"
                        onClick={() => section !== 'untracked' && this.loadDiff(file.path, isStaged)}
                        title={file.path}
                    >
                        {file.path}
                    </span>
                    <div class="git-file-actions">
                        {section === 'staged' ? (
                            <button
                                class="git-action-btn git-action-unstage"
                                onClick={e => {
                                    e.stopPropagation();
                                    this.unstage(file.path);
                                }}
                                title={t('git.action.unstage', language)}
                            >
                                {IconMinus}
                            </button>
                        ) : section === 'unstaged' || section === 'untracked' ? (
                            <Fragment>
                                {section === 'unstaged' && (
                                    <button
                                        class="git-action-btn git-action-discard"
                                        onClick={e => {
                                            e.stopPropagation();
                                            this.discard(file.path);
                                        }}
                                        title={t('git.action.discard', language)}
                                    >
                                        {IconTrash}
                                    </button>
                                )}
                                <button
                                    class="git-action-btn git-action-stage"
                                    onClick={e => {
                                        e.stopPropagation();
                                        this.stage(file.path);
                                    }}
                                    title={t('git.action.stage', language)}
                                >
                                    {IconPlus}
                                </button>
                            </Fragment>
                        ) : null}
                    </div>
                </div>
                {isOpen && section !== 'untracked' && this.renderDiff()}
            </Fragment>
        );
    }

    renderSection(
        title: string,
        files: FileStatus[],
        section: 'staged' | 'unstaged' | 'untracked',
        allAction?: () => void,
        allLabel?: string
    ) {
        if (files.length === 0) return null;

        const isCollapsed =
            section === 'staged'
                ? this.state.stagedCollapsed
                : section === 'unstaged'
                  ? this.state.unstagedCollapsed
                  : this.state.untrackedCollapsed;

        return (
            <div class="git-section">
                <div
                    class="git-section-header git-section-header-clickable"
                    onClick={() => this.toggleSection(section)}
                >
                    <span class="git-section-title">
                        {IconChevron(!isCollapsed)}
                        {title}
                        <span class="git-section-count">{files.length}</span>
                    </span>
                    {allAction && (
                        <button
                            class="git-section-action"
                            onClick={e => {
                                e.stopPropagation();
                                allAction();
                            }}
                            title={allLabel}
                        >
                            {allLabel}
                        </button>
                    )}
                </div>
                {!isCollapsed && <div class="git-file-list">{files.map(f => this.renderFileRow(f, section))}</div>}
            </div>
        );
    }

    renderCleanState() {
        const { language } = this.props;
        return (
            <div class="git-clean-state-card">
                <div class="git-clean-illustration">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14" />
                        <polyline points="22 4 12 14.01 9 11.01" />
                    </svg>
                </div>
                <h3 class="git-clean-title">{t('git.clean.title', language)}</h3>
                <p class="git-clean-desc">{t('git.clean.desc', language)}</p>
                <button class="git-clean-refresh-btn" onClick={this.refresh}>
                    {IconRefresh} {t('git.clean.refresh', language)}
                </button>
            </div>
        );
    }

    // Styled status badge (filled background) shared by commit & worktree file lists.
    renderStatusBadge(status: string) {
        const cls = STATUS_COLOR[status] || 'git-status-u';
        const label = STATUS_KEY[status] ? t(STATUS_KEY[status], this.props.language) : status;
        return (
            <span class={`git-file-status-badge ${cls}`} title={label}>
                {status}
            </span>
        );
    }

    // §1 Worktree switcher (replaces branch selector at the top)
    renderWorktreeSwitcher() {
        const { worktrees, worktreeSwitcherOpen, selectedWorktreePath, worktreesLoading, status } = this.state;
        const { language } = this.props;

        // Determine which worktree is active for display.
        const activeWt = selectedWorktreePath
            ? worktrees.find(w => w.path === selectedWorktreePath)
            : worktrees.find(w => w.isCurrent);
        const displayBranch = activeWt?.branch || status?.branch || '…';

        // Active status for ahead/behind display (§2).
        const activeStatus = this.isViewingCurrent() ? status : this.state.worktreeStatus;

        return (
            <div class="git-branch-bar-container git-worktree-switcher-container">
                {worktreeSwitcherOpen && (
                    <div class="git-dropdown-overlay" onClick={() => this.setState({ worktreeSwitcherOpen: false })} />
                )}
                <div class="git-branch-bar">
                    <div
                        class={`git-branch-selector ${worktreeSwitcherOpen ? 'active' : ''}`}
                        onClick={() => this.setState({ worktreeSwitcherOpen: !worktreeSwitcherOpen })}
                        title={t('git.worktrees.switchTitle', language)}
                    >
                        <span class="git-branch-icon">{IconBranch}</span>
                        <span class="git-branch-name">{displayBranch}</span>
                        <span class="git-branch-arrow">▼</span>
                    </div>

                    {activeStatus && (activeStatus.ahead > 0 || activeStatus.behind > 0) && (
                        <span class="git-ahead-behind">
                            {activeStatus.ahead > 0 && (
                                <span
                                    class="git-ahead"
                                    title={t('git.branch.ahead', language, { n: activeStatus.ahead })}
                                >
                                    ↑{activeStatus.ahead}
                                </span>
                            )}
                            {activeStatus.behind > 0 && (
                                <span
                                    class="git-behind"
                                    title={t('git.branch.behind', language, { n: activeStatus.behind })}
                                >
                                    ↓{activeStatus.behind}
                                </span>
                            )}
                        </span>
                    )}

                    {worktreesLoading && <div class="git-spinner git-spinner-sm" style="margin-left:auto" />}
                </div>

                {/* Worktree dropdown */}
                {worktreeSwitcherOpen && worktrees.length > 0 && (
                    <div class="git-branch-dropdown git-worktree-switcher-dropdown">
                        <div class="git-dropdown-header">
                            <span>{t('git.worktrees.switchTitle', language)}</span>
                        </div>
                        <div class="git-branch-list git-worktree-switcher-list">
                            {worktrees.map(wt => {
                                const isSel =
                                    selectedWorktreePath === wt.path || (selectedWorktreePath === null && wt.isCurrent);
                                return (
                                    <div
                                        key={wt.path}
                                        class={`git-worktree-switcher-item ${isSel ? 'selected' : ''}`}
                                        onClick={() => {
                                            const next = wt.isCurrent ? null : wt.path;
                                            this.setState({
                                                selectedWorktreePath: next,
                                                worktreeSwitcherOpen: false,
                                                commitDiffFile: null,
                                                commitDiffContent: '',
                                            });
                                        }}
                                    >
                                        <div class="git-worktree-switcher-branch-row">
                                            <span class="git-worktree-branch">
                                                {wt.branch || t('git.worktrees.detached', language)}
                                            </span>
                                            {wt.isMain && (
                                                <span class="git-worktree-tag">
                                                    {t('git.worktrees.main', language)}
                                                </span>
                                            )}
                                            {wt.isCurrent && (
                                                <span class="git-worktree-tag git-worktree-tag-current">
                                                    {t('git.worktrees.current', language)}
                                                </span>
                                            )}
                                        </div>
                                        {wt.message && (
                                            <div class="git-worktree-msg">
                                                <span class="git-worktree-short">{wt.short}</span>
                                                <span class="git-worktree-commit-msg">{wt.message}</span>
                                            </div>
                                        )}
                                        <div class="git-worktree-path" title={wt.path}>
                                            {wt.path}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    </div>
                )}
            </div>
        );
    }

    // §3 Changes section — current worktree (full interactive) or another (read-only).
    renderChangesSection() {
        const { language } = this.props;
        const { status, worktreeStatus, worktreeStatusLoading, commitDiffFile, commitDiffContent, commitDiffLoading } =
            this.state;

        if (this.isViewingCurrent()) {
            // Full interactive current-worktree view.
            const staged = status?.staged || [];
            const unstaged = status?.unstaged || [];
            const untracked = status?.untracked || [];
            if (staged.length === 0 && unstaged.length === 0 && untracked.length === 0) {
                return this.renderCleanState();
            }
            return (
                <div class="git-sections-container">
                    {this.renderSection(
                        t('git.section.staged', language),
                        staged,
                        'staged',
                        () => this.unstage(null),
                        t('git.section.unstageAll', language)
                    )}
                    {this.renderSection(
                        t('git.section.unstaged', language),
                        unstaged,
                        'unstaged',
                        () => this.stage(null),
                        t('git.section.stageAll', language)
                    )}
                    {this.renderSection(
                        t('git.section.untracked', language),
                        untracked,
                        'untracked',
                        () => this.stage(null),
                        t('git.section.stageAll', language)
                    )}
                </div>
            );
        }

        // Read-only view for a peeked worktree.
        const selPath = this.state.selectedWorktreePath!;
        if (worktreeStatusLoading) {
            return (
                <div class="git-commit-detail">
                    <div class="git-loading-row">
                        <div class="git-spinner" />
                    </div>
                </div>
            );
        }
        const files = worktreeStatus
            ? [...worktreeStatus.staged, ...worktreeStatus.unstaged, ...worktreeStatus.untracked]
            : [];
        if (files.length === 0) {
            return (
                <div class="git-commit-detail">
                    <div class="git-commit-detail-empty">{t('git.worktrees.clean', language)}</div>
                </div>
            );
        }
        return (
            <div class="git-sections-container">
                <div class="git-section">
                    <div class="git-section-header">
                        <span class="git-section-title">
                            {t('git.section.unstaged', language)}
                            <span class="git-section-count">{files.length}</span>
                        </span>
                    </div>
                    <div class="git-file-list">
                        {files.map(f => (
                            <Fragment key={f.path}>
                                <div
                                    class={`git-commit-file-row ${commitDiffFile === f.path ? 'open' : ''}`}
                                    onClick={() => this.openWorktreeDiff(selPath, f.path)}
                                >
                                    {this.renderStatusBadge(f.status)}
                                    <span class="git-commit-file-path">{f.path}</span>
                                </div>
                                {commitDiffFile === f.path &&
                                    this.renderDiffPanel(f.path, commitDiffContent, commitDiffLoading, () =>
                                        this.setState({ commitDiffFile: null, commitDiffContent: '' })
                                    )}
                            </Fragment>
                        ))}
                    </div>
                </div>
            </div>
        );
    }

    // §4 Commit box — collapsible, current worktree only.
    renderCommitBox() {
        const { commitMsg, committing, pushPullLoading, commitBoxCollapsed, status } = this.state;
        const { language } = this.props;
        if (!this.isViewingCurrent()) return null;

        const staged = status?.staged || [];
        const stagedCount = staged.length;
        const hasStaged = stagedCount > 0;

        return (
            <div class="git-section git-commit-box">
                <div
                    class="git-section-header git-section-header-clickable"
                    onClick={() => this.setState({ commitBoxCollapsed: !commitBoxCollapsed })}
                >
                    <span class="git-section-title">
                        {IconChevron(!commitBoxCollapsed)}
                        {t('git.commit.sectionTitle', language)}
                    </span>
                </div>
                {!commitBoxCollapsed && (
                    <div class="git-commit-box-body">
                        <textarea
                            class="git-commit-input"
                            placeholder={t(
                                hasStaged ? 'git.commit.placeholderReady' : 'git.commit.placeholderEmpty',
                                language
                            )}
                            disabled={!hasStaged}
                            value={commitMsg}
                            onInput={e => this.setState({ commitMsg: (e.target as HTMLTextAreaElement).value })}
                            onKeyDown={(e: KeyboardEvent) => {
                                if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') this.commit();
                            }}
                            rows={2}
                        />
                        <div class="git-commit-actions">
                            <button
                                class="git-ai-commit-btn"
                                onClick={this.generateAICommit}
                                disabled={!hasStaged || this.state.aiLoading}
                                title={t('git.commit.aiTitle', language)}
                            >
                                {this.state.aiLoading ? <div class="git-spinner" /> : IconSparkles}
                            </button>
                            <button
                                class="git-commit-btn"
                                onClick={this.commit}
                                disabled={!hasStaged || !commitMsg.trim() || committing}
                                title={t('git.commit.submitTitle', language)}
                            >
                                {committing ? (
                                    t('git.commit.committing', language)
                                ) : (
                                    <Fragment>
                                        {IconCommit}
                                        <span>
                                            {t('git.commit.commitLabel', language, {
                                                n: stagedCount > 0 ? ` (${stagedCount})` : '',
                                            })}
                                        </span>
                                    </Fragment>
                                )}
                            </button>
                            <button
                                class="git-push-btn"
                                onClick={() => this.pushOrPull('push')}
                                disabled={pushPullLoading !== null}
                                title={t('git.action.push', language)}
                            >
                                {pushPullLoading === 'push' ? <div class="git-spinner" /> : IconPush}
                            </button>
                            <button
                                class="git-pull-btn"
                                onClick={() => this.pushOrPull('pull')}
                                disabled={pushPullLoading !== null}
                                title={t('git.action.pull', language)}
                            >
                                {pushPullLoading === 'pull' ? <div class="git-spinner" /> : IconPull}
                            </button>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    // §5 Graph section with redesigned commit row.
    renderGraphSection() {
        const {
            graph,
            graphExpanded,
            graphLoading,
            expandedCommitHash,
            commitFiles,
            commitFilesLoading,
            commitDiffFile,
            commitDiffContent,
            commitDiffLoading,
        } = this.state;
        const { language } = this.props;

        const LANE_W = 16;
        const ROW_H = 28;
        // Lane 0 = main trunk (blue); side branches cycle a warm/cool palette.
        const LANE_COLORS = ['#f59e0b', '#8b5cf6', '#10b981', '#ec4899', '#06b6d4', '#ef4444'];
        const laneColor = (lane: number) => (lane === 0 ? '#3b82f6' : LANE_COLORS[(lane - 1) % LANE_COLORS.length]);
        const cx = (lane: number) => lane * LANE_W + LANE_W / 2;

        const layout = graphExpanded && graph.length > 0 ? this.buildGraphLayout(graph) : { rows: [], maxLanes: 1 };
        const rows = layout.rows;
        const railW = layout.maxLanes * LANE_W;

        return (
            <div class="git-section git-graph-section">
                <div
                    class="git-section-header git-section-header-clickable"
                    onClick={() => this.setState({ graphExpanded: !graphExpanded })}
                >
                    <span class="git-section-title">
                        {IconChevron(graphExpanded)}
                        {t('git.graph.title', language)}
                        {graph.length > 0 && <span class="git-section-count">{graph.length}</span>}
                    </span>
                    {graphLoading && <div class="git-spinner git-spinner-sm" />}
                </div>

                {graphExpanded && rows.length > 0 && (
                    <div class="git-graph-scroll">
                        {rows.map(rw => {
                            const xn = cx(rw.nodeLane);
                            const yc = ROW_H / 2;
                            const nodeColor = laneColor(rw.nodeLane);
                            const segs: h.JSX.Element[] = [];
                            const incoming = new Set(rw.incomingLanes);
                            const parents = new Set(rw.parentLanes);
                            const above = new Set(rw.aboveLanes);
                            // Connectors that touch the trunk keep the side branch's color.
                            const diagColor = (L: number) => laneColor(L === 0 ? rw.nodeLane : L);
                            const sw = (L: number) => (L === 0 ? 2.5 : 1.6);
                            const push = (key: string, d: string, color: string, width: number) =>
                                segs.push(<path key={key} d={d} stroke={color} stroke-width={width} fill="none" />);
                            const maxL = railW / LANE_W;

                            for (let L = 0; L < maxL; L++) {
                                const xl = cx(L);
                                // ── TOP half (line entering this row from above) ──
                                if (above.has(L) || (L === rw.nodeLane && incoming.has(L))) {
                                    if (L !== rw.nodeLane && incoming.has(L)) {
                                        // a child branch in lane L converging into this node
                                        push(
                                            `i${L}`,
                                            `M${xl},0 C${xl},${ROW_H / 4} ${xn},${ROW_H / 4} ${xn},${yc}`,
                                            diagColor(L),
                                            sw(L)
                                        );
                                    } else if (above.has(L)) {
                                        // node's own lane or a pass-through trunk: straight
                                        push(`ts${L}`, `M${xl},0 L${xl},${yc}`, laneColor(L), sw(L));
                                    }
                                }
                                // ── BOTTOM half (line leaving this row below) ──
                                const botActive = rw.belowLanes.includes(L);
                                if (botActive || (L === rw.nodeLane && parents.has(L))) {
                                    if (L !== rw.nodeLane && parents.has(L)) {
                                        // this node's parent continues in lane L (fork / merge target)
                                        push(
                                            `p${L}`,
                                            `M${xn},${yc} C${xn},${(ROW_H * 3) / 4} ${xl},${(ROW_H * 3) / 4} ${xl},${ROW_H}`,
                                            diagColor(L),
                                            sw(L)
                                        );
                                        // if the lane was already flowing (trunk), keep its straight line too
                                        if (above.has(L))
                                            push(`bs${L}`, `M${xl},${yc} L${xl},${ROW_H}`, laneColor(L), sw(L));
                                    } else if (botActive) {
                                        push(`bs${L}`, `M${xl},${yc} L${xl},${ROW_H}`, laneColor(L), sw(L));
                                    }
                                }
                            }

                            const hasMain = rw.refs.some(r => r === 'main' || r === 'master');
                            const refBadge =
                                rw.refs.length > 0 ? (
                                    <span class="git-graph-ref-count" title={rw.refs.join(', ')}>
                                        {hasMain && <span class="git-ref-badge head">main</span>}
                                        <span class="git-graph-ref-count-badge">
                                            <span class="git-branch-icon-sm">{IconBranch}</span>
                                            {rw.refs.length}
                                        </span>
                                    </span>
                                ) : null;

                            return (
                                <Fragment key={rw.hash}>
                                    <div
                                        class={`git-graph-row ${expandedCommitHash === rw.hash ? 'expanded' : ''}`}
                                        onClick={() => this.toggleCommit(rw.hash)}
                                        title={`${rw.author} · ${rw.hash}`}
                                    >
                                        <svg class="git-graph-rail" width={railW} height={ROW_H}>
                                            {segs}
                                            {rw.isMerge ? (
                                                <circle
                                                    cx={xn}
                                                    cy={yc}
                                                    r="4.5"
                                                    fill="var(--bg-card)"
                                                    stroke={nodeColor}
                                                    stroke-width="2.5"
                                                />
                                            ) : (
                                                <circle
                                                    cx={xn}
                                                    cy={yc}
                                                    r={rw.onMain ? 4.5 : 3.5}
                                                    fill={nodeColor}
                                                    stroke="var(--bg-card)"
                                                    stroke-width="1.5"
                                                />
                                            )}
                                        </svg>
                                        <div class="git-graph-line1">
                                            <span class="git-graph-short">{rw.short}</span>
                                            <span class="git-graph-msg">{rw.message}</span>
                                            {refBadge}
                                            <span class="git-graph-time">{relativeTime(rw.time, language)}</span>
                                        </div>
                                    </div>

                                    {expandedCommitHash === rw.hash && (
                                        <div class="git-commit-detail" style={{ marginLeft: railW }}>
                                            {commitFilesLoading ? (
                                                <div class="git-loading-row">
                                                    <div class="git-spinner" />
                                                </div>
                                            ) : commitFiles.length === 0 ? (
                                                <div class="git-commit-detail-empty">
                                                    {t('git.graph.noFiles', language)}
                                                </div>
                                            ) : (
                                                commitFiles.map(f => (
                                                    <Fragment key={f.path}>
                                                        <div
                                                            class={`git-commit-file-row ${commitDiffFile === f.path ? 'open' : ''}`}
                                                            onClick={e => {
                                                                e.stopPropagation();
                                                                this.openCommitDiff(rw.hash, f.path);
                                                            }}
                                                        >
                                                            {this.renderStatusBadge(f.status)}
                                                            <span class="git-commit-file-path">{f.path}</span>
                                                        </div>
                                                        {commitDiffFile === f.path &&
                                                            this.renderDiffPanel(
                                                                f.path,
                                                                commitDiffContent,
                                                                commitDiffLoading,
                                                                () =>
                                                                    this.setState({
                                                                        commitDiffFile: null,
                                                                        commitDiffContent: '',
                                                                    })
                                                            )}
                                                    </Fragment>
                                                ))
                                            )}
                                        </div>
                                    )}
                                </Fragment>
                            );
                        })}
                    </div>
                )}

                {graphExpanded && !graphLoading && graph.length === 0 && (
                    <div class="git-log-empty">{t('git.graph.empty', language)}</div>
                )}
            </div>
        );
    }

    render() {
        const { status, loading, toast } = this.state;
        const { language } = this.props;

        if (!status && loading) {
            return (
                <div class="git-panel">
                    <div class="git-loading-full">
                        <div class="git-spinner" />
                        <span>{t('git.loading.status', language)}</span>
                    </div>
                </div>
            );
        }

        if (!status || !status.isRepo) {
            return (
                <div class="git-panel">
                    <div class="git-no-repo">
                        <div class="git-no-repo-icon">⎇</div>
                        <span>{t('git.noRepo.title', language)}</span>
                        <span class="git-no-repo-hint">{t('git.noRepo.hint', language)}</span>
                    </div>
                </div>
            );
        }

        return (
            <div class="git-panel">
                {/* §1 Worktree switcher (replaces branch selector) */}
                {this.renderWorktreeSwitcher()}

                {/* §3 Changes section — current (full) or peeked worktree (read-only) */}
                {this.renderChangesSection()}

                {/* §4 Commit box — collapsible, current worktree only */}
                {this.renderCommitBox()}

                {/* §5 Commit graph history */}
                {this.renderGraphSection()}

                {/* Toast notification */}
                {toast && (
                    <div class="git-toast-wrapper">
                        <div class="git-toast">{toast}</div>
                    </div>
                )}
            </div>
        );
    }
}
