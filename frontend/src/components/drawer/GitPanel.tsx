import { h, Component, Fragment } from 'preact';
import { t, type Lang } from '../i18n';
import {
    gitService,
    type FileStatus,
    type GitStatus,
    type WorktreeEntry,
    type GraphCommit,
    type CommitFileEntry,
    type BranchEntry,
    type SubmoduleEntry,
} from '../../services/gitService';
import { buildGraphLayout } from './git/buildGraphLayout';
import { DiffPanel } from './git/DiffPanel';
import type { GraphRow } from './git/types';
import * as fsStore from '../../stores/fsStore';
import * as tabsStore from '../../stores/tabsStore';

// ── Constants ──────────────────────────────────────────────────────────────

const STATUS_POLL_MS = 15_000;
const GRAPH_POLL_MS = 60_000;
const DEFAULT_GRAPH_LIMIT = 30;
const GRAPH_LIMIT_STEP = 30;

const STATUS_KEY: Record<string, string> = {
    M: 'git.status.M',
    A: 'git.status.A',
    D: 'git.status.D',
    R: 'git.status.R',
    C: 'git.status.C',
    U: 'git.status.U',
    '?': 'git.status.?',
};

const STATUS_COLOR: Record<string, string> = {
    M: 'git-status-m',
    A: 'git-status-a',
    D: 'git-status-d',
    R: 'git-status-r',
    U: 'git-status-conflict',
    '?': 'git-status-u',
};

const CONFLICT_CODES = new Set(['U', 'UU', 'AA', 'DD', 'AU', 'UA', 'DU', 'UD']);

function isConflictStatus(status: string): boolean {
    return CONFLICT_CODES.has(status) || status === 'U';
}

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

function joinWorkdir(workdir: string, relPath: string): string {
    if (!relPath) return workdir;
    if (relPath.startsWith('/')) return relPath;
    const base = (workdir || '').replace(/\/+$/, '');
    return base ? `${base}/${relPath}` : relPath;
}

function pathBaseName(p: string): string {
    return p.split('/').filter(Boolean).pop() || p;
}

function samePath(a: string, b: string): boolean {
    const norm = (p: string) =>
        (p || '')
            .replace(/\/+$/, '')
            .replace(/^\/private/, '')
            .toLowerCase();
    return !!a && !!b && norm(a) === norm(b);
}

/** main + worktrees are peer cards; submodules nest under main. */
type SelectedCtx = { kind: 'main' } | { kind: 'worktree'; path: string } | { kind: 'submodule'; path: string };

// ── Icons ──────────────────────────────────────────────────────────────────

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

// Factory — never reuse one VNode across multiple cards (Preact would reparent the DOM node).
const IconBranchEl = () => (
    <svg
        width="14"
        height="14"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
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

const IconOpenEl = () => (
    <svg
        width="12"
        height="12"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
    >
        <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
        <polyline points="15 3 21 3 21 9" />
        <line x1="10" y1="14" x2="21" y2="3" />
    </svg>
);

// ── Props / State ──────────────────────────────────────────────────────────

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
    fetching: boolean;
    diffFile: string | null;
    diffStaged: boolean;
    diffContent: string;
    diffLoading: boolean;
    toast: string;
    /** Unified "Changes" section expand/collapse. */
    changesExpanded: boolean;
    aiLoading: boolean;
    worktrees: WorktreeEntry[];
    worktreesLoading: boolean;
    selected: SelectedCtx;
    selectedStatus: GitStatus | null;
    selectedStatusLoading: boolean;
    graph: GraphCommit[];
    graphLoading: boolean;
    graphExpanded: boolean;
    graphLimit: number;
    expandedCommitHash: string | null;
    commitFiles: CommitFileEntry[];
    commitFilesLoading: boolean;
    commitDiffFile: string | null;
    commitDiffContent: string;
    commitDiffLoading: boolean;
    submodules: SubmoduleEntry[];
    submodulesLoading: boolean;
    /** Collapse nested submodule cards under the main repo card. */
    submodulesCollapsed: boolean;
    // Branch picker (#149) — main repo only
    branchDropdownOpen: boolean;
    branches: BranchEntry[];
    branchesLoading: boolean;
    branchSwitching: boolean;
}

// ── Component ──────────────────────────────────────────────────────────────

export class GitPanel extends Component<GitPanelProps, GitPanelState> {
    private _statusTimer: ReturnType<typeof setInterval> | null = null;
    private _graphTimer: ReturnType<typeof setInterval> | null = null;
    private _toastTimer: ReturnType<typeof setTimeout> | null = null;
    private _abort: AbortController | null = null;
    private _mounted = false;
    private _gen = 0;
    private _graphCacheKey = '';
    private _graphLayout: { rows: GraphRow[]; maxLanes: number } = { rows: [], maxLanes: 1 };

    constructor(props: GitPanelProps) {
        super(props);
        this.state = {
            status: null,
            loading: false,
            commitMsg: '',
            committing: false,
            pushPullLoading: null,
            fetching: false,
            diffFile: null,
            diffStaged: false,
            diffContent: '',
            diffLoading: false,
            toast: '',
            changesExpanded: true,
            aiLoading: false,
            worktrees: [],
            worktreesLoading: false,
            selected: { kind: 'main' },
            selectedStatus: null,
            selectedStatusLoading: false,
            graph: [],
            graphLoading: false,
            graphExpanded: true,
            graphLimit: DEFAULT_GRAPH_LIMIT,
            expandedCommitHash: null,
            commitFiles: [],
            commitFilesLoading: false,
            commitDiffFile: null,
            commitDiffContent: '',
            commitDiffLoading: false,
            submodules: [],
            submodulesLoading: false,
            submodulesCollapsed: false,
            branchDropdownOpen: false,
            branches: [],
            branchesLoading: false,
            branchSwitching: false,
        };
    }

    componentDidMount() {
        this._mounted = true;
        this.props.onRegisterRefresh?.(this.refreshManual);
        this.props.onLoadingChange?.(this.state.loading);
        this.refresh({ silent: false, includeGraph: true });
        this.startTimers();
        document.addEventListener('visibilitychange', this.onVisibility);
    }

    componentDidUpdate(prevProps: GitPanelProps, prevState: GitPanelState) {
        if (prevProps.activeWorkspaceId !== this.props.activeWorkspaceId) {
            this.abortInFlight();
            this.setState({
                diffFile: null,
                diffContent: '',
                selected: { kind: 'main' },
                selectedStatus: null,
                expandedCommitHash: null,
                commitFiles: [],
                commitDiffFile: null,
                commitDiffContent: '',
                changesExpanded: true,
                submodules: [],
                branchDropdownOpen: false,
                branches: [],
                branchSwitching: false,
                graphLimit: DEFAULT_GRAPH_LIMIT,
            });
            this.refresh({ silent: false, includeGraph: true });
        }
        if (prevState.loading !== this.state.loading) {
            this.props.onLoadingChange?.(this.state.loading);
        }
        if (prevProps.onRegisterRefresh !== this.props.onRegisterRefresh && this.props.onRegisterRefresh) {
            this.props.onRegisterRefresh(this.refreshManual);
        }
        if (JSON.stringify(prevState.selected) !== JSON.stringify(this.state.selected)) {
            const sel = this.state.selected;
            if (sel.kind === 'main') {
                this.setState({ selectedStatus: null });
            } else {
                this.loadSelectedPathStatus(sel.path);
            }
            if (this.graphRepoPath(prevState.selected) !== this.graphRepoPath(sel)) {
                this.setState(
                    {
                        graph: [],
                        graphLimit: DEFAULT_GRAPH_LIMIT,
                        expandedCommitHash: null,
                        commitFiles: [],
                        commitDiffFile: null,
                        commitDiffContent: '',
                    },
                    () => {
                        if (this.state.graphExpanded) this.loadGraph(false);
                    }
                );
            }
        }
        if (prevState.graphExpanded !== this.state.graphExpanded) {
            this.restartGraphTimer();
            if (this.state.graphExpanded) this.loadGraph(true);
        }
    }

    componentWillUnmount() {
        this._mounted = false;
        this.stopTimers();
        this.abortInFlight();
        document.removeEventListener('visibilitychange', this.onVisibility);
        if (this._toastTimer) clearTimeout(this._toastTimer);
        this.props.onRegisterRefresh?.(() => {});
        this.props.onLoadingChange?.(false);
    }

    // ── Timers ─────────────────────────────────────────────────────────────

    onVisibility = () => {
        if (document.visibilityState === 'hidden') this.stopTimers();
        else if (this._mounted) {
            this.refresh({ silent: true, includeGraph: this.state.graphExpanded });
            this.startTimers();
        }
    };

    startTimers() {
        this.stopTimers();
        if (document.visibilityState === 'hidden') return;
        this._statusTimer = setInterval(() => {
            this.refresh({ silent: true, includeGraph: false });
        }, STATUS_POLL_MS);
        this.restartGraphTimer();
    }

    restartGraphTimer() {
        if (this._graphTimer) {
            clearInterval(this._graphTimer);
            this._graphTimer = null;
        }
        if (!this.state.graphExpanded || document.visibilityState === 'hidden') return;
        this._graphTimer = setInterval(() => this.loadGraph(true), GRAPH_POLL_MS);
    }

    stopTimers() {
        if (this._statusTimer) {
            clearInterval(this._statusTimer);
            this._statusTimer = null;
        }
        if (this._graphTimer) {
            clearInterval(this._graphTimer);
            this._graphTimer = null;
        }
    }

    abortInFlight() {
        if (this._abort) {
            this._abort.abort();
            this._abort = null;
        }
    }

    signal(): AbortSignal {
        this.abortInFlight();
        this._abort = new AbortController();
        return this._abort.signal;
    }

    // ── Helpers ────────────────────────────────────────────────────────────

    isViewingMain(): boolean {
        return this.state.selected.kind === 'main';
    }

    isInteractive(): boolean {
        return this.activeStatus()?.isRepo === true;
    }

    activeStatus(): GitStatus | null {
        return this.isViewingMain() ? this.state.status : this.state.selectedStatus;
    }

    activeRepoPath(): string | null {
        return this.state.selected.kind === 'main' ? null : this.state.selected.path;
    }

    activeRepoRoot(): string {
        const path = this.activeRepoPath();
        return path ? joinWorkdir(this.props.workdir, path) : this.props.workdir;
    }

    graphRepoPath(selected = this.state.selected): string | null {
        return selected.kind === 'submodule' ? selected.path : null;
    }

    graphRepoRoot(): string {
        const path = this.graphRepoPath();
        return path ? joinWorkdir(this.props.workdir, path) : this.props.workdir;
    }

    setActiveStatus(status: GitStatus) {
        if (this.isViewingMain()) this.setState({ status });
        else this.setState({ selectedStatus: status });
    }

    repoDisplayName(): string {
        return pathBaseName(this.props.workdir || '') || 'repo';
    }

    selectMain = () => {
        this.setState({
            selected: { kind: 'main' },
            selectedStatus: null,
            commitMsg: '',
            diffFile: null,
            diffContent: '',
            commitDiffFile: null,
            commitDiffContent: '',
        });
    };

    selectWorktree = (path: string, isCurrent: boolean) => {
        if (isCurrent) {
            this.selectMain();
            return;
        }
        this.setState({
            selected: { kind: 'worktree', path },
            selectedStatus: null,
            commitMsg: '',
            diffFile: null,
            diffContent: '',
            commitDiffFile: null,
            commitDiffContent: '',
        });
    };

    selectSubmodule = (path: string) => {
        this.setState({
            selected: { kind: 'submodule', path },
            selectedStatus: null,
            commitMsg: '',
            diffFile: null,
            diffContent: '',
            commitDiffFile: null,
            commitDiffContent: '',
        });
    };

    showToast = (msg: string) => {
        if (this._toastTimer) clearTimeout(this._toastTimer);
        this.setState({ toast: msg });
        this._toastTimer = setTimeout(() => {
            if (this._mounted) this.setState({ toast: '' });
        }, 3000);
    };

    getLayout(): { rows: GraphRow[]; maxLanes: number } {
        const { graph } = this.state;
        const key = graph.map(c => c.hash).join(',');
        if (key !== this._graphCacheKey) {
            this._graphCacheKey = key;
            this._graphLayout = graph.length > 0 ? buildGraphLayout(graph) : { rows: [], maxLanes: 1 };
        }
        return this._graphLayout;
    }

    // ── Data ───────────────────────────────────────────────────────────────

    /** Header / clean-state refresh: fetch remote first, then reload local git views. */
    refreshManual = async () => {
        const path = this.activeRepoPath();
        this.setState({ fetching: true, loading: true });
        try {
            await gitService.fetchRemote(path);
        } catch (err) {
            // Still reload local status even when fetch fails (offline / no upstream).
            this.showToast(t('git.toast.fetchFailedPrefix', this.props.language, { err: String(err) }));
        } finally {
            if (this._mounted) this.setState({ fetching: false });
        }
        await this.refresh({ silent: false, includeGraph: true });
    };

    refresh = async (opts: { silent: boolean; includeGraph: boolean }) => {
        const { silent, includeGraph } = opts;
        const gen = ++this._gen;
        if (!silent) this.setState({ loading: true });
        const signal = this.signal();
        try {
            const status = await gitService.status({ signal });
            if (!this._mounted || gen !== this._gen) return;
            this.setState({ status, loading: false });
        } catch (err) {
            if ((err as Error)?.name === 'AbortError') return;
            console.error('[git] status error:', err);
            if (this._mounted && gen === this._gen) this.setState({ loading: false });
        }
        if (gen !== this._gen) return;
        this.loadWorktrees(gen);
        this.loadSubmodules(gen);
        if (includeGraph && this.state.graphExpanded) this.loadGraph(true, gen);
        const sel = this.state.selected;
        if (sel.kind !== 'main') this.loadSelectedPathStatus(sel.path, gen);
    };

    loadWorktrees = async (gen = this._gen) => {
        this.setState({ worktreesLoading: true });
        try {
            const worktrees = await gitService.worktrees();
            if (this._mounted && gen === this._gen) this.setState({ worktrees, worktreesLoading: false });
        } catch (err) {
            if ((err as Error)?.name === 'AbortError') return;
            console.error('[git] worktrees error:', err);
            if (this._mounted && gen === this._gen) this.setState({ worktreesLoading: false });
        }
    };

    loadSubmodules = async (gen = this._gen) => {
        this.setState({ submodulesLoading: true });
        try {
            const submodules = await gitService.submodules();
            if (this._mounted && gen === this._gen) this.setState({ submodules, submodulesLoading: false });
        } catch (err) {
            console.error('[git] submodules error:', err);
            if (this._mounted && gen === this._gen) this.setState({ submodulesLoading: false });
        }
    };

    loadSelectedPathStatus = async (path: string, gen = this._gen) => {
        this.setState({ selectedStatusLoading: true });
        try {
            const selectedStatus = await gitService.worktreeStatus(path);
            if (this._mounted && gen === this._gen) this.setState({ selectedStatus, selectedStatusLoading: false });
        } catch (err) {
            console.error('[git] path-status error:', err);
            if (this._mounted && gen === this._gen) this.setState({ selectedStatusLoading: false });
        }
    };

    loadGraph = async (silent = true, gen = this._gen) => {
        const path = this.graphRepoPath();
        if (!silent) this.setState({ graphLoading: true });
        try {
            const graph = await gitService.graph(this.state.graphLimit, path);
            if (this._mounted && gen === this._gen && this.graphRepoPath() === path) {
                this.setState({ graph, graphLoading: false });
            }
        } catch (err) {
            if ((err as Error)?.name === 'AbortError') return;
            console.error('[git] graph error:', err);
            if (this._mounted && gen === this._gen) this.setState({ graphLoading: false });
        }
    };

    loadMoreGraph = async () => {
        const next = this.state.graphLimit + GRAPH_LIMIT_STEP;
        const gen = this._gen;
        const path = this.graphRepoPath();
        this.setState({ graphLimit: next, graphLoading: true });
        try {
            const graph = await gitService.graph(next, path);
            if (this._mounted && gen === this._gen && this.graphRepoPath() === path) {
                this.setState({ graph, graphLoading: false });
            }
        } catch (err) {
            console.error('[git] graph more error:', err);
            if (this._mounted && gen === this._gen) this.setState({ graphLoading: false });
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
        const path = this.graphRepoPath();
        try {
            const commitFiles = await gitService.commitFiles(hash, path);
            if (this._mounted && this.graphRepoPath() === path) {
                this.setState({ commitFiles, commitFilesLoading: false });
            }
        } catch (err) {
            console.error('[git] commit-files error:', err);
            if (this._mounted) this.setState({ commitFilesLoading: false });
        }
    };

    openCommitDiff = async (hash: string, file: string) => {
        if (this.state.commitDiffFile === file) {
            this.setState({ commitDiffFile: null, commitDiffContent: '' });
            return;
        }
        const path = this.graphRepoPath();
        this.setState({ commitDiffFile: file, commitDiffLoading: true, commitDiffContent: '' });
        try {
            const text = await gitService.commitDiff(hash, file, path);
            if (this._mounted && this.graphRepoPath() === path) {
                this.setState({ commitDiffContent: text, commitDiffLoading: false });
            }
        } catch (err) {
            if (this._mounted) this.setState({ commitDiffContent: `Error: ${err}`, commitDiffLoading: false });
        }
    };

    loadDiff = async (file: string, staged: boolean) => {
        if (this.state.diffFile === file && this.state.diffStaged === staged) {
            this.setState({ diffFile: null, diffContent: '' });
            return;
        }
        const path = this.activeRepoPath();
        this.setState({ diffFile: file, diffStaged: staged, diffLoading: true, diffContent: '' });
        try {
            const text = await gitService.diff(file, staged, path);
            if (this._mounted) this.setState({ diffContent: text, diffLoading: false });
        } catch (err) {
            if (this._mounted) this.setState({ diffContent: `Error loading diff: ${err}`, diffLoading: false });
        }
    };

    // ── Actions ────────────────────────────────────────────────────────────

    applyOptimisticStage(file: string | null, direction: 'stage' | 'unstage') {
        const status = this.activeStatus();
        if (!status) return null;
        const snap: GitStatus = {
            ...status,
            staged: [...status.staged],
            unstaged: [...status.unstaged],
            untracked: [...status.untracked],
        };
        if (file === null) {
            if (direction === 'stage') {
                const moved = [...status.unstaged, ...status.untracked];
                this.setActiveStatus({ ...status, staged: [...status.staged, ...moved], unstaged: [], untracked: [] });
            } else {
                this.setActiveStatus({ ...status, staged: [], unstaged: [...status.unstaged, ...status.staged] });
            }
            return snap;
        }
        if (direction === 'stage') {
            const fromUnstaged = status.unstaged.find(f => f.path === file);
            const fromUntracked = status.untracked.find(f => f.path === file);
            const entry = fromUnstaged || fromUntracked || { path: file, status: 'M' };
            this.setActiveStatus({
                ...status,
                staged: status.staged.some(f => f.path === file)
                    ? status.staged
                    : [...status.staged, { path: entry.path, status: entry.status === '?' ? 'A' : entry.status }],
                unstaged: status.unstaged.filter(f => f.path !== file),
                untracked: status.untracked.filter(f => f.path !== file),
            });
            this.setState({
                diffFile: this.state.diffFile === file && !this.state.diffStaged ? null : this.state.diffFile,
                diffContent: this.state.diffFile === file && !this.state.diffStaged ? '' : this.state.diffContent,
            });
        } else {
            const fromStaged = status.staged.find(f => f.path === file);
            const entry = fromStaged || { path: file, status: 'M' };
            this.setActiveStatus({
                ...status,
                staged: status.staged.filter(f => f.path !== file),
                unstaged: status.unstaged.some(f => f.path === file) ? status.unstaged : [...status.unstaged, entry],
            });
            this.setState({
                diffFile: this.state.diffFile === file && this.state.diffStaged ? null : this.state.diffFile,
                diffContent: this.state.diffFile === file && this.state.diffStaged ? '' : this.state.diffContent,
            });
        }
        return snap;
    }

    stage = async (file: string | null) => {
        const path = this.activeRepoPath();
        const snap = this.applyOptimisticStage(file, 'stage');
        try {
            await gitService.stage(file, path);
            this.refresh({ silent: true, includeGraph: false });
        } catch (err) {
            if (snap && this._mounted) this.setActiveStatus(snap);
            this.showToast(t('git.toast.stageFailed', this.props.language, { err: String(err) }));
        }
    };

    unstage = async (file: string | null) => {
        const path = this.activeRepoPath();
        const snap = this.applyOptimisticStage(file, 'unstage');
        try {
            await gitService.unstage(file, path);
            this.refresh({ silent: true, includeGraph: false });
        } catch (err) {
            if (snap && this._mounted) this.setActiveStatus(snap);
            this.showToast(t('git.toast.unstageFailed', this.props.language, { err: String(err) }));
        }
    };

    discard = async (file: string) => {
        if (!window.confirm(t('git.discardConfirm', this.props.language, { file }))) return;
        const path = this.activeRepoPath();
        this.setState({ loading: true });
        try {
            await gitService.discard(file, path);
            this.showToast(t('git.toast.discarded', this.props.language));
            if (this.state.diffFile === file) this.setState({ diffFile: null, diffContent: '' });
            this.refresh({ silent: false, includeGraph: false });
        } catch (err) {
            this.showToast(t('git.toast.discardFailed', this.props.language, { err: String(err) }));
            this.setState({ loading: false });
        }
    };

    commit = async () => {
        const { commitMsg } = this.state;
        if (!commitMsg.trim()) return;
        const path = this.activeRepoPath();
        this.setState({ committing: true });
        try {
            await gitService.commit(commitMsg.trim(), path);
            this.setState({ commitMsg: '', committing: false });
            this.showToast(t('git.toast.committed', this.props.language));
            this.refresh({ silent: false, includeGraph: true });
        } catch (err) {
            this.setState({ committing: false });
            this.showToast(t('git.toast.commitFailed', this.props.language, { err: String(err) }));
        }
    };

    generateAICommit = async () => {
        const path = this.activeRepoPath();
        this.setState({ aiLoading: true });
        this.showToast(t('git.toast.aiAnalyzing', this.props.language));
        try {
            const message = await gitService.aiCommit(path);
            this.setState({ commitMsg: message, aiLoading: false });
            this.showToast(t('git.toast.aiSuccess', this.props.language));
        } catch (err) {
            this.setState({ aiLoading: false });
            this.showToast(
                t('git.toast.aiFailed', this.props.language, { msg: (err as Error)?.message || String(err) })
            );
        }
    };

    pushOrPull = async (action: 'push' | 'pull') => {
        const path = this.activeRepoPath();
        this.setState({ pushPullLoading: action });
        try {
            if (action === 'push') await gitService.push(path);
            else await gitService.pull(path);
            this.showToast(
                t(action === 'push' ? 'git.toast.pushSuccess' : 'git.toast.pullSuccess', this.props.language)
            );
            this.refresh({ silent: false, includeGraph: true });
        } catch (err) {
            this.showToast(
                t(
                    action === 'push' ? 'git.toast.pushFailedPrefix' : 'git.toast.pullFailedPrefix',
                    this.props.language,
                    {
                        err: String(err),
                    }
                )
            );
        } finally {
            this.setState({ pushPullLoading: null });
        }
    };

    /**
     * Click .git-card-origin: fetch, then pull if behind / push if ahead.
     * path empty = main worktree root; otherwise worktree or submodule path.
     */
    syncWithRemote = async (path?: string | null) => {
        if (this.state.fetching || this.state.pushPullLoading) return;
        const scoped = path && path.length > 0 ? path : null;
        this.setState({ fetching: true });
        this.showToast(t('git.toast.syncing', this.props.language));
        try {
            await gitService.fetchRemote(scoped);
            let st = scoped ? await gitService.worktreeStatus(scoped) : await gitService.status();
            if (!this._mounted) return;
            if (scoped) {
                // Update selected peek status when syncing that card.
                const sel = this.state.selected;
                if (
                    (sel.kind === 'worktree' || sel.kind === 'submodule') &&
                    (sel.path === scoped || samePath(sel.path, scoped))
                ) {
                    this.setState({ selectedStatus: st });
                }
            } else {
                this.setState({ status: st });
            }

            if (st.behind > 0) {
                this.setState({ pushPullLoading: 'pull' });
                await gitService.pull(scoped);
                st = scoped ? await gitService.worktreeStatus(scoped) : await gitService.status();
                if (this._mounted) {
                    if (scoped) this.setState({ selectedStatus: st });
                    else this.setState({ status: st });
                }
            }
            if (st.ahead > 0) {
                this.setState({ pushPullLoading: 'push' });
                await gitService.push(scoped);
            }
            this.showToast(t('git.toast.syncSuccess', this.props.language));
            await this.refresh({ silent: false, includeGraph: true });
            if (scoped) await this.loadSelectedPathStatus(scoped);
        } catch (err) {
            this.showToast(t('git.toast.syncFailed', this.props.language, { err: String(err) }));
            await this.refresh({ silent: true, includeGraph: false });
            if (scoped) await this.loadSelectedPathStatus(scoped);
        } finally {
            if (this._mounted) this.setState({ fetching: false, pushPullLoading: null });
        }
    };

    loadBranches = async () => {
        this.setState({ branchesLoading: true });
        try {
            const branches = await gitService.branches();
            if (this._mounted) this.setState({ branches, branchesLoading: false });
        } catch (err) {
            console.error('[git] branches error:', err);
            if (this._mounted) this.setState({ branchesLoading: false });
        }
    };

    openBranchPicker = (e: Event) => {
        e.preventDefault();
        e.stopPropagation();
        const open = !this.state.branchDropdownOpen;
        this.setState({ branchDropdownOpen: open });
        if (open) this.loadBranches();
    };

    closeBranchPicker = () => {
        this.setState({ branchDropdownOpen: false });
    };

    checkoutBranch = async (branch: string) => {
        if (this.state.branchSwitching) return;
        this.setState({ branchSwitching: true });
        try {
            await gitService.checkout(branch, false);
            this.showToast(t('git.toast.branchSwitched', this.props.language, { branch }));
            this.setState({ branchDropdownOpen: false, branchSwitching: false });
            await this.refresh({ silent: false, includeGraph: true });
        } catch (err) {
            this.showToast(t('git.toast.branchSwitchFailed', this.props.language, { err: String(err) }));
            if (this._mounted) this.setState({ branchSwitching: false });
        }
    };

    openFile = (relPath: string, status?: string, isDir = false, repoRoot = this.props.workdir) => {
        if (status === 'D') {
            this.showToast(t('git.toast.fileDeleted', this.props.language, { file: relPath }));
            return;
        }
        const abs = joinWorkdir(repoRoot, relPath);
        const name = relPath.split('/').pop() || relPath;
        tabsStore.openContentTab('files');
        void fsStore.openFileDetail({ name, path: abs, isDir, size: 0, modTime: 0 });
    };

    renderOriginBadge(
        ahead: number,
        behind: number,
        language: Lang,
        opts?: { syncPath?: string | null; busy?: boolean }
    ) {
        // Always show both counters so the right column never collapses.
        // syncPath: null/undefined = main root; string = worktree/submodule path.
        const syncable = opts?.syncPath !== undefined; // caller passes null for main, string for scoped
        const busy = !!opts?.busy;
        const path = opts?.syncPath ?? null;
        return (
            <button
                type="button"
                class={`git-card-origin ${syncable ? 'clickable' : ''} ${busy ? 'busy' : ''}`}
                title={
                    syncable
                        ? t('git.card.originSyncTitle', language, { ahead, behind })
                        : t('git.card.originTitle', language, { ahead, behind })
                }
                disabled={!syncable || busy}
                onClick={
                    syncable
                        ? e => {
                              e.preventDefault();
                              e.stopPropagation();
                              void this.syncWithRemote(path);
                          }
                        : e => e.stopPropagation()
                }
            >
                <span
                    class={`git-card-ahead ${ahead > 0 ? 'active' : ''}`}
                    title={t('git.branch.ahead', language, { n: ahead })}
                >
                    ↑{ahead}
                </span>
                <span
                    class={`git-card-behind ${behind > 0 ? 'active' : ''}`}
                    title={t('git.branch.behind', language, { n: behind })}
                >
                    ↓{behind}
                </span>
            </button>
        );
    }

    /** Single-line repo card: [name] [kind] …… [branch] [↑↓] [extra] */
    renderRepoCardRow(opts: {
        key: string;
        selected: boolean;
        conflict?: boolean;
        child?: boolean;
        name: string;
        nameTitle?: string;
        kind: string;
        branch: string;
        ahead: number;
        behind: number;
        onClick: () => void;
        trailing?: h.JSX.Element | null;
        /** Main card: branch control opens floating picker (#149). */
        branchPicker?: boolean;
        /**
         * Origin badge sync path.
         * - pass `null` for main root
         * - pass absolute/relative repo path for worktree/submodule
         * - omit for non-syncable (should not happen after product change)
         */
        originSyncPath?: string | null;
    }) {
        const { language } = this.props;
        const { fetching, pushPullLoading, branchDropdownOpen } = this.state;
        const originBusy = fetching || pushPullLoading !== null;
        const hasOriginSync = opts.originSyncPath !== undefined;
        return (
            <div
                key={opts.key}
                class={`git-repo-card ${opts.child ? 'git-repo-card-child' : ''} ${opts.selected ? 'selected' : ''} ${
                    opts.conflict ? 'conflict' : ''
                }`}
                onClick={opts.onClick}
            >
                <span class="git-repo-card-name" title={opts.nameTitle || opts.name}>
                    {opts.name}
                </span>
                {opts.kind ? <span class="git-repo-card-kind">{opts.kind}</span> : null}
                <span class="git-repo-card-grow" />
                {opts.branchPicker ? (
                    <button
                        type="button"
                        class={`git-repo-card-branch clickable ${branchDropdownOpen ? 'open' : ''}`}
                        title={t('git.branch.toggleTitle', language)}
                        onClick={this.openBranchPicker}
                        onMouseDown={e => {
                            // Prevent parent card from receiving the click first.
                            e.stopPropagation();
                        }}
                    >
                        {IconBranchEl()}
                        <span class="git-repo-card-branch-text">{opts.branch}</span>
                        <span class={`git-repo-card-branch-caret ${branchDropdownOpen ? 'open' : ''}`}>▼</span>
                    </button>
                ) : (
                    <span class="git-repo-card-branch" title={opts.branch}>
                        {IconBranchEl()}
                        <span class="git-repo-card-branch-text">{opts.branch}</span>
                    </span>
                )}
                {this.renderOriginBadge(opts.ahead, opts.behind, language, {
                    syncPath: hasOriginSync ? opts.originSyncPath! : undefined,
                    busy: originBusy && hasOriginSync,
                })}
                {opts.trailing}
            </div>
        );
    }

    // ── Repo cards (main + worktrees peers; submodules under main) ─────────

    renderRepoCards() {
        const { worktrees, status, selected, submodules, submodulesCollapsed } = this.state;
        const { language, workdir } = this.props;
        const repoName = this.repoDisplayName();
        const mainSelected = selected.kind === 'main';
        const ahead = status?.ahead ?? 0;
        const behind = status?.behind ?? 0;
        const mainBranch = status?.branch || '…';

        // Dedup: main card already represents isMain / isCurrent / workdir path.
        // Path compare also covers macOS /private prefix mismatches on isCurrent.
        const mainWt = worktrees.find(w => w.isMain) || worktrees.find(w => w.isCurrent);
        const mainPath = mainWt?.path || workdir;
        const peerWorktrees = worktrees.filter(w => {
            if (w.isMain || w.isCurrent) return false;
            if (samePath(w.path, mainPath) || samePath(w.path, workdir)) return false;
            return true;
        });

        // Submodule expand/collapse lives on the main card chevron.
        const submodulesToggle =
            submodules.length > 0 ? (
                <button
                    type="button"
                    class="git-card-branch-btn"
                    onClick={e => {
                        e.stopPropagation();
                        this.setState({ submodulesCollapsed: !submodulesCollapsed });
                    }}
                    title={
                        submodulesCollapsed
                            ? t('git.submodules.expand', language)
                            : t('git.submodules.collapse', language)
                    }
                    aria-expanded={!submodulesCollapsed}
                >
                    {IconChevron(!submodulesCollapsed)}
                </button>
            ) : null;

        return (
            <div class="git-repo-cards">
                {/* Main card — branch text opens picker; origin badge syncs remote */}
                {this.renderRepoCardRow({
                    key: 'main',
                    selected: mainSelected,
                    name: repoName,
                    nameTitle: workdir,
                    kind: t('git.card.main', language),
                    branch: mainBranch,
                    ahead,
                    behind,
                    onClick: this.selectMain,
                    trailing: submodulesToggle,
                    branchPicker: true,
                    originSyncPath: null, // main root
                })}

                {/* Submodules under main (collapsed via main-card chevron) */}
                {submodules.length > 0 && !submodulesCollapsed && (
                    <div class="git-repo-card-children">
                        {submodules.map(sm => {
                            const sel =
                                selected.kind === 'submodule' &&
                                (selected.path === sm.path ||
                                    selected.path.endsWith('/' + sm.path) ||
                                    samePath(selected.path, sm.path));
                            const branch = sm.branch || sm.desc || (sm.flag === '-' ? '—' : sm.short) || '…';
                            return this.renderRepoCardRow({
                                key: sm.path,
                                selected: !!sel,
                                conflict: sm.flag === 'U',
                                child: true,
                                name: sm.path,
                                nameTitle: sm.path,
                                kind: '', // nesting already marks submodule; no kind chip
                                branch,
                                ahead: sm.ahead ?? 0,
                                behind: sm.behind ?? 0,
                                onClick: () => this.selectSubmodule(sm.path),
                                originSyncPath: sm.path,
                            });
                        })}
                    </div>
                )}

                {/* Peer worktrees only (main already shown above) */}
                {peerWorktrees.map(wt =>
                    this.renderRepoCardRow({
                        key: wt.path,
                        selected: selected.kind === 'worktree' && selected.path === wt.path,
                        name: pathBaseName(wt.path),
                        nameTitle: wt.path,
                        kind: t('git.card.worktree', language),
                        branch: wt.branch || t('git.worktrees.detached', language),
                        ahead: wt.ahead ?? 0,
                        behind: wt.behind ?? 0,
                        onClick: () => this.selectWorktree(wt.path, wt.isCurrent),
                        originSyncPath: wt.path,
                    })
                )}
            </div>
        );
    }

    /** Floating branch picker popover (viewport-fixed, not clipped by panel scroll). */
    renderBranchPopover() {
        const { branchDropdownOpen, branches, branchesLoading, branchSwitching } = this.state;
        const { language } = this.props;
        if (!branchDropdownOpen) return null;
        return (
            <div class="git-branch-popover-root" onClick={e => e.stopPropagation()}>
                <div
                    class="git-branch-popover-backdrop"
                    onClick={e => {
                        e.stopPropagation();
                        this.closeBranchPicker();
                    }}
                />
                <div class="git-branch-popover" role="dialog" aria-label={t('git.branch.selectTitle', language)}>
                    <div class="git-branch-popover-header">
                        <span>{t('git.branch.selectTitle', language)}</span>
                        {branchSwitching && <div class="git-spinner git-spinner-sm" />}
                        <button
                            type="button"
                            class="git-diff-close-btn"
                            onClick={this.closeBranchPicker}
                            title={t('git.diff.close', language)}
                        >
                            ×
                        </button>
                    </div>
                    <div class="git-branch-popover-list">
                        {branchesLoading ? (
                            <div class="git-dropdown-loading">{t('git.branch.loading', language)}</div>
                        ) : branches.length === 0 ? (
                            <div class="git-dropdown-empty">{t('git.branch.empty', language)}</div>
                        ) : (
                            branches.map(b => (
                                <button
                                    type="button"
                                    key={b.name}
                                    class={`git-branch-popover-item ${b.current ? 'current' : ''}`}
                                    disabled={branchSwitching}
                                    onClick={() => {
                                        if (b.current || branchSwitching) {
                                            this.closeBranchPicker();
                                            return;
                                        }
                                        void this.checkoutBranch(b.name);
                                    }}
                                >
                                    <span class="git-branch-item-icon">{IconBranchEl()}</span>
                                    <span class="git-branch-item-name">{b.name}</span>
                                    {b.current && <span class="git-branch-item-check">✓</span>}
                                </button>
                            ))
                        )}
                    </div>
                </div>
            </div>
        );
    }

    // ── Changes / commit ───────────────────────────────────────────────────

    renderStatusBadge(status: string) {
        const conflict = isConflictStatus(status);
        const cls = STATUS_COLOR[status] || (conflict ? 'git-status-conflict' : 'git-status-u');
        const label = STATUS_KEY[status]
            ? t(STATUS_KEY[status], this.props.language)
            : conflict
              ? t('git.status.U', this.props.language)
              : status;
        return (
            <span class={`git-file-status-badge ${cls}`} title={label}>
                {status}
            </span>
        );
    }

    renderDiff() {
        const { diffFile, diffContent, diffLoading } = this.state;
        if (!diffFile) return null;
        return (
            <DiffPanel
                file={diffFile}
                content={diffContent}
                loading={diffLoading}
                language={this.props.language}
                onClose={() => this.setState({ diffFile: null, diffContent: '' })}
            />
        );
    }

    renderFileRow(file: FileStatus, section: 'staged' | 'unstaged') {
        const { diffFile, diffStaged } = this.state;
        const { language } = this.props;
        const isStaged = section === 'staged';
        const isUntracked = file.status === '?';
        const canDiff = !isUntracked;
        const isOpen = canDiff && diffFile === file.path && diffStaged === isStaged;
        const conflict = isConflictStatus(file.status);
        const statusCls = STATUS_COLOR[file.status] || (conflict ? 'git-status-conflict' : 'git-status-u');
        const label = STATUS_KEY[file.status]
            ? t(STATUS_KEY[file.status], language)
            : conflict
              ? t('git.status.U', language)
              : file.status;
        const interactive = this.isInteractive();

        return (
            <Fragment key={`${section}-${file.path}`}>
                <div class={`git-file-row ${isOpen ? 'open' : ''} ${conflict ? 'git-file-conflict' : ''}`}>
                    <span class={`git-file-status ${statusCls}`} title={label}>
                        {file.status}
                    </span>
                    <span
                        class="git-file-path"
                        onClick={() => {
                            if (!interactive || !canDiff) return;
                            this.loadDiff(file.path, isStaged);
                        }}
                        title={file.path}
                    >
                        {file.path}
                    </span>
                    {interactive && (
                        <div class="git-file-actions">
                            {file.status !== 'D' && !isUntracked && (
                                <button
                                    class="git-action-btn git-action-open"
                                    onClick={e => {
                                        e.stopPropagation();
                                        this.openFile(file.path, file.status, false, this.activeRepoRoot());
                                    }}
                                    title={t('git.action.openFile', language)}
                                >
                                    {IconOpenEl()}
                                </button>
                            )}
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
                            ) : (
                                <Fragment>
                                    {!isUntracked && !conflict && (
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
                                    {!conflict && (
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
                                    )}
                                </Fragment>
                            )}
                        </div>
                    )}
                </div>
                {isOpen && interactive && this.renderDiff()}
            </Fragment>
        );
    }

    /** Staged or unstaged file group inside the unified Changes section. */
    renderFileGroup(
        title: string,
        files: FileStatus[],
        section: 'staged' | 'unstaged',
        allAction?: () => void,
        allLabel?: string
    ) {
        if (files.length === 0) return null;
        const interactive = this.isInteractive();

        return (
            <div class={`git-changes-group git-changes-group-${section}`}>
                <div class="git-changes-group-header">
                    <span class="git-changes-group-title">
                        {title}
                        <span class="git-section-count">{files.length}</span>
                    </span>
                    {interactive && allAction && (
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
                <div class="git-file-list">{files.map(f => this.renderFileRow(f, section))}</div>
            </div>
        );
    }

    /** Working tree has no staged / unstaged / untracked files. */
    isWorkingTreeClean(): boolean {
        const activeStatus = this.activeStatus();
        if (!activeStatus?.isRepo) return false;
        return (
            (activeStatus.staged || []).length === 0 &&
            (activeStatus.unstaged || []).length === 0 &&
            (activeStatus.untracked || []).length === 0
        );
    }

    /** Clean-tree illustration + refresh (nested inside Changes when empty). */
    renderCleanStateCard() {
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
                <button class="git-clean-refresh-btn" onClick={this.refreshManual}>
                    {IconRefresh} {t('git.clean.refresh', language)}
                </button>
            </div>
        );
    }

    /** Commit message + AI / commit / push / pull — lives under Changes. */
    renderCommitZone() {
        if (!this.isInteractive()) return null;
        const { commitMsg, committing, pushPullLoading } = this.state;
        const { language } = this.props;
        const stagedCount = (this.activeStatus()?.staged || []).length;
        const hasStaged = stagedCount > 0;

        return (
            <div class="git-changes-zone git-commit-zone">
                <div class="git-changes-zone-label">{t('git.commit.sectionTitle', language)}</div>
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
            </div>
        );
    }

    /**
     * Unified collapsible "变更" section: commit form + staged + unstaged
     * (untracked files are folded into unstaged).
     */
    renderChangesSection() {
        const { language } = this.props;
        const { selectedStatusLoading, changesExpanded } = this.state;
        if (!this.isViewingMain() && selectedStatusLoading) {
            return (
                <div class="git-commit-detail">
                    <div class="git-loading-row">
                        <div class="git-spinner" />
                    </div>
                </div>
            );
        }
        const activeStatus = this.activeStatus();
        if (!activeStatus?.isRepo) {
            return null;
        }

        const staged = activeStatus.staged || [];
        // Merge untracked into unstaged for a single "未暂存" list.
        const unstaged = [...(activeStatus.unstaged || []), ...(activeStatus.untracked || [])];
        const changeCount = staged.length + unstaged.length;
        const isClean = changeCount === 0;
        const interactive = this.isInteractive();

        // Read-only peek with a clean tree: still show a compact clean card.
        if (isClean && !interactive) {
            return this.renderCleanStateCard();
        }

        return (
            <div class={`git-section git-changes-section${isClean ? ' is-clean' : ''}`}>
                <div
                    class="git-section-header git-section-header-clickable"
                    onClick={() => this.setState({ changesExpanded: !changesExpanded })}
                >
                    <span class="git-section-title">
                        {IconChevron(changesExpanded)}
                        {t('git.section.changes', language)}
                        {changeCount > 0 && <span class="git-section-count">{changeCount}</span>}
                    </span>
                </div>
                {changesExpanded && (
                    <div class="git-section-body git-changes-body">
                        {isClean && this.renderCleanStateCard()}
                        {this.renderCommitZone()}
                        {this.renderFileGroup(
                            t('git.section.staged', language),
                            staged,
                            'staged',
                            () => this.unstage(null),
                            t('git.section.unstageAll', language)
                        )}
                        {this.renderFileGroup(
                            t('git.section.unstaged', language),
                            unstaged,
                            'unstaged',
                            () => this.stage(null),
                            t('git.section.stageAll', language)
                        )}
                    </div>
                )}
            </div>
        );
    }

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
            worktrees,
            graphLimit,
        } = this.state;
        const { language } = this.props;

        const LANE_W = 16;
        const ROW_H = 28;
        const LANE_COLORS = ['#f59e0b', '#8b5cf6', '#10b981', '#ec4899', '#06b6d4', '#ef4444'];
        const laneColor = (lane: number) =>
            lane === 0 ? 'var(--accent-color, #3b82f6)' : LANE_COLORS[(lane - 1) % LANE_COLORS.length];
        const cx = (lane: number) => lane * LANE_W + LANE_W / 2;

        const layout = graphExpanded && graph.length > 0 ? this.getLayout() : { rows: [] as GraphRow[], maxLanes: 1 };
        const rows = layout.rows;
        const railW = layout.maxLanes * LANE_W;

        const worktreeByBranch = new Map<string, WorktreeEntry>();
        if (!this.graphRepoPath()) {
            for (const wt of worktrees) {
                if (wt.branch) worktreeByBranch.set(wt.branch, wt);
            }
        }
        const branchInitials = (name: string): string => {
            const tail = name.includes('/') ? name.slice(name.lastIndexOf('/') + 1) : name;
            const initials = tail
                .split(/[-_\s.]+/)
                .filter(Boolean)
                .map(s => s[0])
                .join('')
                .toUpperCase();
            return (initials || tail).slice(0, 4);
        };

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
                            const diagColor = (L: number) => laneColor(L === 0 ? rw.nodeLane : L);
                            const sw = (L: number) => (L === 0 ? 2.5 : 1.6);
                            const push = (key: string, d: string, color: string, width: number) =>
                                segs.push(<path key={key} d={d} stroke={color} stroke-width={width} fill="none" />);
                            const maxL = railW / LANE_W;

                            for (let L = 0; L < maxL; L++) {
                                const xl = cx(L);
                                if (above.has(L) || (L === rw.nodeLane && incoming.has(L))) {
                                    if (L !== rw.nodeLane && incoming.has(L)) {
                                        push(
                                            `i${L}`,
                                            `M${xl},0 C${xl},${ROW_H / 4} ${xn},${ROW_H / 4} ${xn},${yc}`,
                                            diagColor(L),
                                            sw(L)
                                        );
                                    } else if (above.has(L)) {
                                        push(`ts${L}`, `M${xl},0 L${xl},${yc}`, laneColor(L), sw(L));
                                    }
                                }
                                const botActive = rw.belowLanes.includes(L);
                                if (botActive || (L === rw.nodeLane && parents.has(L))) {
                                    if (L !== rw.nodeLane && parents.has(L)) {
                                        push(
                                            `p${L}`,
                                            `M${xn},${yc} C${xn},${(ROW_H * 3) / 4} ${xl},${(ROW_H * 3) / 4} ${xl},${ROW_H}`,
                                            diagColor(L),
                                            sw(L)
                                        );
                                        if (above.has(L))
                                            push(`bs${L}`, `M${xl},${yc} L${xl},${ROW_H}`, laneColor(L), sw(L));
                                    } else if (botActive) {
                                        push(`bs${L}`, `M${xl},${yc} L${xl},${ROW_H}`, laneColor(L), sw(L));
                                    }
                                }
                            }

                            const refBadgeList = rw.refs
                                .map(r => {
                                    if (r === 'main' || r === 'master') {
                                        return (
                                            <span key={r} class="git-ref-badge head" title={r}>
                                                {r}
                                            </span>
                                        );
                                    }
                                    const wt = worktreeByBranch.get(r);
                                    if (!wt) return null;
                                    return (
                                        <span
                                            key={r}
                                            class={`git-ref-badge worktree ${wt.isCurrent ? 'current' : ''}`}
                                            title={wt.isCurrent ? `${r} · ${t('git.worktrees.current', language)}` : r}
                                        >
                                            <span class="git-branch-icon-sm">{IconBranchEl()}</span>
                                            {branchInitials(r)}
                                        </span>
                                    );
                                })
                                .filter(Boolean);

                            return (
                                <Fragment key={rw.hash}>
                                    <div
                                        class={`git-graph-row ${expandedCommitHash === rw.hash ? 'expanded' : ''}`}
                                        onClick={() => this.toggleCommit(rw.hash)}
                                        title={rw.message}
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
                                            {refBadgeList.length > 0 && (
                                                <span class="git-graph-ref-count">{refBadgeList}</span>
                                            )}
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
                                                            {f.status !== 'D' && (
                                                                <button
                                                                    class="git-action-btn git-action-open"
                                                                    onClick={e => {
                                                                        e.stopPropagation();
                                                                        this.openFile(
                                                                            f.path,
                                                                            f.status,
                                                                            false,
                                                                            this.graphRepoRoot()
                                                                        );
                                                                    }}
                                                                    title={t('git.action.openFile', language)}
                                                                >
                                                                    {IconOpenEl()}
                                                                </button>
                                                            )}
                                                        </div>
                                                        {commitDiffFile === f.path && (
                                                            <DiffPanel
                                                                file={f.path}
                                                                content={commitDiffContent}
                                                                loading={commitDiffLoading}
                                                                language={language}
                                                                onClose={() =>
                                                                    this.setState({
                                                                        commitDiffFile: null,
                                                                        commitDiffContent: '',
                                                                    })
                                                                }
                                                            />
                                                        )}
                                                    </Fragment>
                                                ))
                                            )}
                                        </div>
                                    )}
                                </Fragment>
                            );
                        })}
                        {graph.length >= graphLimit && (
                            <button
                                class="git-section-action"
                                style={{ margin: '8px auto', display: 'block' }}
                                onClick={e => {
                                    e.stopPropagation();
                                    this.loadMoreGraph();
                                }}
                                disabled={graphLoading}
                            >
                                {t('git.graph.loadMore', language)}
                            </button>
                        )}
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
                {this.renderRepoCards()}
                {this.renderChangesSection()}
                {this.renderGraphSection()}
                {this.renderBranchPopover()}
                {toast && (
                    <div class="git-toast-wrapper">
                        <div class="git-toast">{toast}</div>
                    </div>
                )}
            </div>
        );
    }
}
