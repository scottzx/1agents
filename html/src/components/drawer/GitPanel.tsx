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

interface BranchEntry {
    name: string;
    current: boolean;
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

interface LaneCommit extends GraphCommit {
    row: number;
    lane: number;
    onMain: boolean;
    isMerge: boolean;
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
    // diff
    diffFile: string | null;
    diffStaged: boolean;
    diffContent: string;
    diffLoading: boolean;
    // toast
    toast: string;
    // branch management
    branches: BranchEntry[];
    branchDropdownOpen: boolean;
    branchesLoading: boolean;
    creatingBranch: boolean;
    newBranchName: string;
    showNewBranchInput: boolean;
    // collapsible sections
    stagedCollapsed: boolean;
    unstagedCollapsed: boolean;
    untrackedCollapsed: boolean;
    // ai commit message
    aiLoading: boolean;
    // worktrees
    worktrees: WorktreeEntry[];
    worktreesLoading: boolean;
    worktreesExpanded: boolean;
    // graph history
    graph: GraphCommit[];
    graphLoading: boolean;
    graphExpanded: boolean;
    // commit detail (file list)
    expandedCommitHash: string | null;
    commitFiles: CommitFileEntry[];
    commitFilesLoading: boolean;
    // worktree detail (inline uncommitted changes)
    expandedWorktreePath: string | null;
    worktreeStatus: GitStatus | null;
    worktreeStatusLoading: boolean;
    // diff overlay (shared by commit-file and worktree-file diffs)
    commitDiffFile: string | null;
    commitDiffSubtitle: string;
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

const IconCheck = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2.5"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <polyline points="20 6 9 17 4 12" />
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
            // branch dropdown list
            branches: [],
            branchDropdownOpen: false,
            branchesLoading: false,
            creatingBranch: false,
            newBranchName: '',
            showNewBranchInput: false,
            // collapsibles
            stagedCollapsed: false,
            unstagedCollapsed: false,
            untrackedCollapsed: false,
            // AI loading state
            aiLoading: false,
            // worktrees
            worktrees: [],
            worktreesLoading: false,
            worktreesExpanded: true,
            // graph history
            graph: [],
            graphLoading: false,
            graphExpanded: true,
            // commit detail
            expandedCommitHash: null,
            commitFiles: [],
            commitFilesLoading: false,
            // worktree detail
            expandedWorktreePath: null,
            worktreeStatus: null,
            worktreeStatusLoading: false,
            // diff overlay
            commitDiffFile: null,
            commitDiffSubtitle: '',
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
            if (this.state.branchDropdownOpen) {
                this.loadBranches();
            }
        }, 15000);
    }

    componentDidUpdate(prevProps: GitPanelProps, prevState: GitPanelState) {
        if (prevProps.activeWorkspaceId !== this.props.activeWorkspaceId) {
            this.setState({
                diffFile: null,
                diffContent: '',
                branchDropdownOpen: false,
                showNewBranchInput: false,
                expandedCommitHash: null,
                commitFiles: [],
                expandedWorktreePath: null,
                worktreeStatus: null,
                commitDiffFile: null,
                commitDiffContent: '',
            });
            this.refresh();
        }
        if (prevState.loading !== this.state.loading) {
            this.props.onLoadingChange?.(this.state.loading);
        }
        if (prevProps.onRegisterRefresh !== this.props.onRegisterRefresh && this.props.onRegisterRefresh) {
            this.props.onRegisterRefresh(this.refresh);
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
    };

    loadBranches = async () => {
        this.setState({ branchesLoading: true });
        try {
            const res = await fetch('/api/git/branches');
            if (!res.ok) throw new Error(await res.text());
            const branches: BranchEntry[] = await res.json();
            this.setState({ branches, branchesLoading: false });
        } catch (err) {
            console.error('[git] branches error:', err);
            this.setState({ branchesLoading: false });
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
            this.setState({ expandedCommitHash: null, commitFiles: [] });
            return;
        }
        this.setState({ expandedCommitHash: hash, commitFiles: [], commitFilesLoading: true });
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
        this.setState({
            commitDiffFile: file,
            commitDiffSubtitle: hash.slice(0, 7),
            commitDiffLoading: true,
            commitDiffContent: '',
        });
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

    // Toggle inline expansion of a worktree → show its uncommitted changes.
    toggleWorktree = async (path: string) => {
        if (this.state.expandedWorktreePath === path) {
            this.setState({ expandedWorktreePath: null, worktreeStatus: null });
            return;
        }
        this.setState({ expandedWorktreePath: path, worktreeStatus: null, worktreeStatusLoading: true });
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

    openWorktreeDiff = async (path: string, file: string, subtitle: string) => {
        this.setState({
            commitDiffFile: file,
            commitDiffSubtitle: subtitle,
            commitDiffLoading: true,
            commitDiffContent: '',
        });
        try {
            const res = await fetch(
                `/api/git/worktree-diff?path=${encodeURIComponent(path)}&file=${encodeURIComponent(file)}`
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

    checkoutBranch = async (branchName: string) => {
        this.setState({ loading: true, branchDropdownOpen: false });
        try {
            const res = await fetch('/api/git/checkout', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ branch: branchName, create: false }),
            });
            if (!res.ok) throw new Error(await res.text());
            this.showToast(t('git.toast.branchSwitched', this.props.language, { branch: branchName }));
            this.refresh();
        } catch (err) {
            this.showToast(t('git.toast.branchSwitchFailed', this.props.language, { err: String(err) }));
            this.setState({ loading: false });
        }
    };

    createBranch = async () => {
        const { newBranchName } = this.state;
        if (!newBranchName.trim()) return;
        this.setState({ creatingBranch: true });
        try {
            const res = await fetch('/api/git/checkout', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ branch: newBranchName.trim(), create: true }),
            });
            if (!res.ok) throw new Error(await res.text());
            const branchName = newBranchName.trim();
            this.showToast(t('git.toast.branchCreated', this.props.language, { branch: branchName }));
            this.setState({
                newBranchName: '',
                showNewBranchInput: false,
                branchDropdownOpen: false,
                creatingBranch: false,
            });
            this.refresh();
        } catch (err) {
            this.showToast(t('git.toast.branchCreateFailed', this.props.language, { err: String(err) }));
            this.setState({ creatingBranch: false });
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
    buildGraphLayout(commits: GraphCommit[]): LaneCommit[] {
        const byHash = new Map(commits.map(c => [c.hash, c]));

        // Identify the trunk tip: prefer main/master, else its remote, else first row.
        const mainTip =
            commits.find(c => c.refs.some(r => r === 'main' || r === 'master')) ||
            commits.find(c => c.refs.some(r => r.endsWith('/main') || r.endsWith('/master'))) ||
            commits[0];

        // Walk first-parent chain from the trunk tip → these hashes own lane 0.
        const onMain = new Set<string>();
        let cur: GraphCommit | undefined = mainTip;
        while (cur && !onMain.has(cur.hash)) {
            onMain.add(cur.hash);
            cur = cur.parents[0] ? byHash.get(cur.parents[0]) : undefined;
        }

        const laneOwners: (string | null)[] = [null]; // index 0 reserved for main
        const result: LaneCommit[] = [];

        const freeLane = (): number => {
            for (let i = 1; i < laneOwners.length; i++) {
                if (laneOwners[i] === null) return i;
            }
            laneOwners.push(null);
            return laneOwners.length - 1;
        };

        commits.forEach((commit, row) => {
            const isOnMain = onMain.has(commit.hash);
            let lane: number;
            if (isOnMain) {
                lane = 0;
            } else {
                lane = laneOwners.findIndex((hh, i) => i >= 1 && hh === commit.hash);
                if (lane < 1) lane = freeLane();
            }
            laneOwners[lane] = null; // release before re-seating parents

            commit.parents.forEach((p, i) => {
                if (onMain.has(p)) {
                    laneOwners[0] = p; // parent belongs to the trunk
                    return;
                }
                if (laneOwners.includes(p)) return; // already seated in some lane
                if (i === 0 && !isOnMain) {
                    laneOwners[lane] = p; // first parent continues this branch's lane
                } else {
                    laneOwners[freeLane()] = p; // merged-in / additional parent opens a lane
                }
            });

            result.push({
                ...commit,
                row,
                lane,
                onMain: isOnMain,
                isMerge: commit.parents.length > 1,
            });
        });

        return result;
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

    renderDiff() {
        const { diffFile, diffContent, diffLoading } = this.state;
        const { language } = this.props;
        if (!diffFile) return null;

        const parsedLines = this.parseDiffLines(diffContent);

        return (
            <div class="git-diff-panel" onClick={e => e.stopPropagation()}>
                <div class="git-diff-header">
                    <span class="git-diff-title">{diffFile}</span>
                    <button
                        class="git-diff-close-btn"
                        onClick={() => this.setState({ diffFile: null, diffContent: '' })}
                        title={t('git.diff.close', language)}
                    >
                        ×
                    </button>
                </div>
                {diffLoading ? (
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

    renderWorktrees() {
        const { worktrees, worktreesExpanded, worktreesLoading } = this.state;
        const { language } = this.props;
        if (!worktrees.length && !worktreesLoading) return null;

        return (
            <div class="git-section git-worktrees-section">
                <div
                    class="git-section-header git-section-header-clickable"
                    onClick={() => this.setState({ worktreesExpanded: !worktreesExpanded })}
                >
                    <span class="git-section-title">
                        {IconChevron(worktreesExpanded)}
                        {t('git.worktrees.title', language)}
                        <span class="git-section-count">{worktrees.length}</span>
                    </span>
                    {worktreesLoading && <div class="git-spinner git-spinner-sm" />}
                </div>
                {worktreesExpanded && (
                    <div class="git-worktrees-list">
                        {worktrees.map(wt => {
                            const isOpen = this.state.expandedWorktreePath === wt.path;
                            return (
                                <Fragment key={wt.path}>
                                    <div
                                        class={`git-worktree-item ${wt.isCurrent ? 'is-current' : ''} ${isOpen ? 'open' : ''}`}
                                        onClick={() => this.toggleWorktree(wt.path)}
                                    >
                                        <div class="git-worktree-chevron">{IconChevron(isOpen)}</div>
                                        <div class={`git-worktree-dot ${wt.isCurrent ? 'current' : 'other'}`} />
                                        <div class="git-worktree-info">
                                            <div class="git-worktree-branch-row">
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
                                    </div>
                                    {isOpen && this.renderWorktreeChanges(wt)}
                                </Fragment>
                            );
                        })}
                    </div>
                )}
            </div>
        );
    }

    // Inline list of a worktree's uncommitted changes; click a file → diff overlay.
    renderWorktreeChanges(wt: WorktreeEntry) {
        const { worktreeStatus, worktreeStatusLoading } = this.state;
        const { language } = this.props;
        const subtitle = wt.branch || wt.short;

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
            <div class="git-commit-detail">
                {files.map(f => (
                    <div
                        key={f.path}
                        class="git-commit-file-row"
                        onClick={e => {
                            e.stopPropagation();
                            this.openWorktreeDiff(wt.path, f.path, subtitle);
                        }}
                    >
                        {this.renderStatusBadge(f.status)}
                        <span class="git-commit-file-path">{f.path}</span>
                    </div>
                ))}
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

    renderGraphSection() {
        const { graph, graphExpanded, graphLoading, expandedCommitHash, commitFiles, commitFilesLoading } = this.state;
        const { language } = this.props;

        const LANE_W = 16;
        const ROW_H = 26;
        // Lane 0 is the trunk (accent); side branches cycle through the palette.
        const TRUNK_COLOR = 'var(--accent-fg)';
        const LANE_COLORS = ['#2196F3', '#FF9800', '#9C27B0', '#00BCD4', '#F44336', '#8BC34A'];
        const laneColor = (lane: number) => (lane === 0 ? TRUNK_COLOR : LANE_COLORS[(lane - 1) % LANE_COLORS.length]);

        const laneCommits = graphExpanded && graph.length > 0 ? this.buildGraphLayout(graph) : [];
        const hashToCommit = new Map(laneCommits.map(c => [c.hash, c]));
        const maxLane = laneCommits.reduce((m, c) => Math.max(m, c.lane), 0);
        const svgW = (maxLane + 1) * LANE_W + 4;
        const svgH = laneCommits.length * ROW_H;

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

                {graphExpanded && laneCommits.length > 0 && (
                    <div class="git-graph-scroll">
                        <div class="git-graph-content">
                            <svg
                                class="git-graph-svg"
                                width={svgW}
                                height={svgH}
                                style={{ flexShrink: 0 }}
                            >
                                {/* Edges first, so nodes draw on top */}
                                {laneCommits.flatMap(commit => {
                                    const cx = commit.lane * LANE_W + LANE_W / 2;
                                    const cy = commit.row * ROW_H + ROW_H / 2;
                                    return commit.parents.map(parentHash => {
                                        const parent = hashToCommit.get(parentHash);
                                        if (!parent) return null;
                                        const px = parent.lane * LANE_W + LANE_W / 2;
                                        const py = parent.row * ROW_H + ROW_H / 2;
                                        // Edge color/width follow the busier (non-trunk) lane it travels.
                                        const edgeLane = Math.max(commit.lane, parent.lane);
                                        const trunkEdge = commit.lane === 0 && parent.lane === 0;
                                        const d =
                                            commit.lane === parent.lane
                                                ? `M${cx},${cy} L${px},${py}`
                                                : `M${cx},${cy} C${cx},${(cy + py) / 2} ${px},${(cy + py) / 2} ${px},${py}`;
                                        return (
                                            <path
                                                key={`line-${commit.hash}-${parentHash}`}
                                                d={d}
                                                stroke={laneColor(edgeLane)}
                                                stroke-width={trunkEdge ? 2.5 : 1.6}
                                                fill="none"
                                            />
                                        );
                                    });
                                })}
                                {/* Nodes: trunk emphasized, merges drawn as a ring */}
                                {laneCommits.map(commit => {
                                    const cx = commit.lane * LANE_W + LANE_W / 2;
                                    const cy = commit.row * ROW_H + ROW_H / 2;
                                    const color = laneColor(commit.lane);
                                    if (commit.isMerge) {
                                        return (
                                            <circle
                                                key={`dot-${commit.hash}`}
                                                cx={cx}
                                                cy={cy}
                                                r="4.5"
                                                fill="var(--bg-card)"
                                                stroke={color}
                                                stroke-width="2.5"
                                            />
                                        );
                                    }
                                    return (
                                        <circle
                                            key={`dot-${commit.hash}`}
                                            cx={cx}
                                            cy={cy}
                                            r={commit.onMain ? 4.5 : 3.5}
                                            fill={color}
                                            stroke="var(--bg-card)"
                                            stroke-width="1.5"
                                        />
                                    );
                                })}
                            </svg>

                            <div class="git-graph-rows">
                                {laneCommits.map(commit => (
                                    <div key={commit.hash}>
                                        <div
                                            class={`git-graph-row ${expandedCommitHash === commit.hash ? 'expanded' : ''}`}
                                            onClick={() => this.toggleCommit(commit.hash)}
                                            title={`${commit.author} · ${commit.hash}`}
                                        >
                                            <div class="git-graph-line1">
                                                {commit.refs.map(ref => (
                                                    <span key={ref}
                                                        class={`git-ref-badge ${ref === 'HEAD' || ref.startsWith('HEAD') ? 'head' : ''}`}>
                                                        {ref}
                                                    </span>
                                                ))}
                                                <span class="git-graph-short">{commit.short}</span>
                                                <span class="git-graph-msg">{commit.message}</span>
                                                <span class="git-graph-time">
                                                    {relativeTime(commit.time, language)}
                                                </span>
                                            </div>
                                        </div>

                                        {expandedCommitHash === commit.hash && (
                                            <div class="git-commit-detail">
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
                                                        <div
                                                            key={f.path}
                                                            class="git-commit-file-row"
                                                            onClick={e => {
                                                                e.stopPropagation();
                                                                this.openCommitDiff(commit.hash, f.path);
                                                            }}
                                                        >
                                                            {this.renderStatusBadge(f.status)}
                                                            <span class="git-commit-file-path">{f.path}</span>
                                                        </div>
                                                    ))
                                                )}
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>
                        </div>
                    </div>
                )}

                {graphExpanded && !graphLoading && graph.length === 0 && (
                    <div class="git-log-empty">{t('git.graph.empty', language)}</div>
                )}
            </div>
        );
    }

    renderCommitDiffOverlay() {
        const { commitDiffFile, commitDiffSubtitle, commitDiffContent, commitDiffLoading } = this.state;
        const { language } = this.props;
        if (!commitDiffFile) return null;

        const parsedLines = this.parseDiffLines(commitDiffContent);

        return (
            <div class="git-commit-diff-overlay" onClick={e => e.stopPropagation()}>
                <div class="git-commit-diff-header">
                    <span class="git-diff-title">{commitDiffFile}</span>
                    {commitDiffSubtitle && (
                        <span class="git-graph-short">@ {commitDiffSubtitle}</span>
                    )}
                    <button
                        class="git-diff-close-btn"
                        style={{ marginLeft: 'auto' }}
                        onClick={() => this.setState({ commitDiffFile: null, commitDiffContent: '' })}
                        title={t('git.diff.close', language)}
                    >
                        ×
                    </button>
                </div>
                <div class="git-commit-diff-body">
                    {commitDiffLoading ? (
                        <div class="git-diff-loading">
                            <div class="git-spinner" />
                            <span>{t('git.diff.loading', language)}</span>
                        </div>
                    ) : parsedLines.length > 0 ? (
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
                    ) : (
                        <div class="git-diff-empty">{t('git.diff.empty', language)}</div>
                    )}
                </div>
            </div>
        );
    }

    render() {
        const {
            status,
            loading,
            commitMsg,
            committing,
            pushPullLoading,
            toast,
            branches,
            branchDropdownOpen,
            branchesLoading,
            creatingBranch,
            newBranchName,
            showNewBranchInput,
        } = this.state;
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

        const staged = status.staged || [];
        const unstaged = status.unstaged || [];
        const untracked = status.untracked || [];
        const stagedCount = staged.length;
        const hasStaged = stagedCount > 0;

        return (
            <div class="git-panel">
                {/* Backdrop overlay for closing branch selector */}
                {branchDropdownOpen && (
                    <div
                        class="git-dropdown-overlay"
                        onClick={() => this.setState({ branchDropdownOpen: false, showNewBranchInput: false })}
                    />
                )}

                {/* Branch selector & Actions */}
                <div class="git-branch-bar-container">
                    <div class="git-branch-bar">
                        <div
                            class={`git-branch-selector ${branchDropdownOpen ? 'active' : ''}`}
                            onClick={() => {
                                const nextOpen = !branchDropdownOpen;
                                this.setState({ branchDropdownOpen: nextOpen });
                                if (nextOpen) this.loadBranches();
                            }}
                            title={t('git.branch.toggleTitle', language)}
                        >
                            <span class="git-branch-icon">{IconBranch}</span>
                            <span class="git-branch-name">{status.branch}</span>
                            <span class="git-branch-arrow">▼</span>
                        </div>

                        {(status.ahead > 0 || status.behind > 0) && (
                            <span class="git-ahead-behind">
                                {status.ahead > 0 && (
                                    <span
                                        class="git-ahead"
                                        title={t('git.branch.ahead', language, { n: status.ahead })}
                                    >
                                        ↑{status.ahead}
                                    </span>
                                )}
                                {status.behind > 0 && (
                                    <span
                                        class="git-behind"
                                        title={t('git.branch.behind', language, { n: status.behind })}
                                    >
                                        ↓{status.behind}
                                    </span>
                                )}
                            </span>
                        )}
                    </div>

                    {/* Branch dropdown list */}
                    {branchDropdownOpen && (
                        <div class="git-branch-dropdown">
                            <div class="git-dropdown-header">
                                <span>{t('git.branch.selectTitle', language)}</span>
                                <button
                                    class={`git-create-branch-toggle-btn ${showNewBranchInput ? 'active' : ''}`}
                                    onClick={e => {
                                        e.stopPropagation();
                                        this.setState({ showNewBranchInput: !showNewBranchInput });
                                    }}
                                    title={t('git.branch.new', language)}
                                >
                                    {IconPlus}
                                </button>
                            </div>

                            {showNewBranchInput && (
                                <div class="git-new-branch-box" onClick={e => e.stopPropagation()}>
                                    <input
                                        type="text"
                                        class="git-new-branch-input"
                                        placeholder={t('git.branch.namePlaceholder', language)}
                                        value={newBranchName}
                                        onInput={e =>
                                            this.setState({ newBranchName: (e.target as HTMLInputElement).value })
                                        }
                                        onKeyDown={(e: KeyboardEvent) => {
                                            if (e.key === 'Enter') this.createBranch();
                                        }}
                                        autoFocus
                                    />
                                    <button
                                        class="git-new-branch-submit"
                                        onClick={this.createBranch}
                                        disabled={creatingBranch || !newBranchName.trim()}
                                    >
                                        {creatingBranch
                                            ? t('git.branch.creating', language)
                                            : t('git.branch.create', language)}
                                    </button>
                                </div>
                            )}

                            <div class="git-branch-list">
                                {branchesLoading ? (
                                    <div class="git-dropdown-loading">
                                        <div class="git-spinner" />
                                        <span>{t('git.branch.loading', language)}</span>
                                    </div>
                                ) : branches.length === 0 ? (
                                    <div class="git-dropdown-empty">{t('git.branch.empty', language)}</div>
                                ) : (
                                    branches.map(b => (
                                        <div
                                            key={b.name}
                                            class={`git-branch-item ${b.current ? 'current' : ''}`}
                                            onClick={() => !b.current && this.checkoutBranch(b.name)}
                                        >
                                            <span class="git-branch-item-icon">{IconBranch}</span>
                                            <span class="git-branch-item-name">{b.name}</span>
                                            {b.current && <span class="git-branch-item-check">{IconCheck}</span>}
                                        </div>
                                    ))
                                )}
                            </div>
                        </div>
                    )}
                </div>

                {/* Worktrees section */}
                {this.renderWorktrees()}

                {/* Commit box */}
                <div class="git-commit-box">
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

                {/* Changes sections */}
                {staged.length === 0 && unstaged.length === 0 && untracked.length === 0 ? (
                    this.renderCleanState()
                ) : (
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
                )}

                {/* Commit graph history (replaces the old linear log) */}
                {this.renderGraphSection()}

                {/* Modern slide-in Toast Notification */}
                {toast && (
                    <div class="git-toast-wrapper">
                        <div class="git-toast">{toast}</div>
                    </div>
                )}

                {/* Commit diff overlay (absolute, covers full panel) */}
                {this.renderCommitDiffOverlay()}
            </div>
        );
    }
}
