import { h, Component, Fragment } from 'preact';

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

interface CommitEntry {
    hash: string;
    short: string;
    message: string;
    author: string;
    time: number;
}

interface BranchEntry {
    name: string;
    current: boolean;
}

interface GitPanelProps {
    workdir: string;
    activeWorkspaceId: string;
}

interface GitPanelState {
    status: GitStatus | null;
    loading: boolean;
    commitMsg: string;
    committing: boolean;
    pushPullLoading: 'push' | 'pull' | null;
    log: CommitEntry[];
    logLoading: boolean;
    logExpanded: boolean;
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
    // log filter
    commitSearchQuery: string;
    // collapsible sections
    stagedCollapsed: boolean;
    unstagedCollapsed: boolean;
    untrackedCollapsed: boolean;
}

// ── Status label map ───────────────────────────────────────────────────────

const STATUS_LABEL: Record<string, string> = {
    M: '已修改',
    A: '已添加',
    D: '已删除',
    R: '已重命名',
    C: '已复制',
    '?': '未跟踪',
};

const STATUS_COLOR: Record<string, string> = {
    M: 'git-status-m',
    A: 'git-status-a',
    D: 'git-status-d',
    R: 'git-status-r',
    '?': 'git-status-u',
};

function relativeTime(ts: number): string {
    const diff = Math.floor(Date.now() / 1000) - ts;
    if (diff < 60) return '刚刚';
    if (diff < 3600) return `${Math.floor(diff / 60)} 分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)} 小时前`;
    if (diff < 86400 * 7) return `${Math.floor(diff / 86400)} 天前`;
    return new Date(ts * 1000).toLocaleDateString('zh-CN', {
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

const IconSearch = (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <circle cx="11" cy="11" r="8" />
        <line x1="21" y1="21" x2="16.65" y2="16.65" />
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
            log: [],
            logLoading: false,
            logExpanded: false,
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
            // log search
            commitSearchQuery: '',
            // collapsibles
            stagedCollapsed: false,
            unstagedCollapsed: false,
            untrackedCollapsed: false,
        };
    }

    componentDidMount() {
        this.refresh();
        this._refreshTimer = setInterval(() => {
            this.refresh();
            if (this.state.branchDropdownOpen) {
                this.loadBranches();
            }
        }, 15000);
    }

    componentDidUpdate(prevProps: GitPanelProps) {
        if (prevProps.activeWorkspaceId !== this.props.activeWorkspaceId) {
            this.setState({
                logExpanded: false,
                diffFile: null,
                diffContent: '',
                branchDropdownOpen: false,
                showNewBranchInput: false,
                commitSearchQuery: '',
            });
            this.refresh();
        }
    }

    componentWillUnmount() {
        if (this._refreshTimer) clearInterval(this._refreshTimer);
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

    loadLog = async () => {
        this.setState({ logLoading: true, logExpanded: true });
        try {
            const res = await fetch('/api/git/log?limit=30');
            if (!res.ok) throw new Error(await res.text());
            const log: CommitEntry[] = await res.json();
            this.setState({ log, logLoading: false });
        } catch (err) {
            console.error('[git] log error:', err);
            this.setState({ logLoading: false });
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
        const confirmDiscard = window.confirm(
            `确定要放弃对文件 "${file}" 的所有更改吗？\n此操作将直接重置文件，且无法撤销！`
        );
        if (!confirmDiscard) return;

        this.setState({ loading: true });
        try {
            const res = await fetch(`/api/git/discard?file=${encodeURIComponent(file)}`, { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            this.showToast('已放弃更改 ✓');
            if (this.state.diffFile === file) {
                this.setState({ diffFile: null, diffContent: '' });
            }
            this.refresh();
        } catch (err) {
            this.showToast(`放弃更改失败: ${err}`);
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
            this.showToast(`已切换到分支 "${branchName}" ✓`);
            this.refresh();
        } catch (err) {
            this.showToast(`切换分支失败: ${err}`);
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
            this.showToast(`已成功创建并切换到分支 "${branchName}" ✓`);
            this.setState({
                newBranchName: '',
                showNewBranchInput: false,
                branchDropdownOpen: false,
                creatingBranch: false,
            });
            this.refresh();
        } catch (err) {
            this.showToast(`创建分支失败: ${err}`);
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
            this.showToast('提交成功 ✓');
            this.refresh();
            if (this.state.logExpanded) this.loadLog();
        } catch (err) {
            this.setState({ committing: false });
            this.showToast(`提交失败: ${err}`);
        }
    };

    pushOrPull = async (action: 'push' | 'pull') => {
        this.setState({ pushPullLoading: action });
        try {
            const res = await fetch(`/api/git/${action}`, { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            this.showToast(action === 'push' ? '远程推送成功 ✓' : '拉取更新成功 ✓');
            this.refresh();
            if (this.state.logExpanded) this.loadLog();
        } catch (err) {
            this.showToast(`${action === 'push' ? '推送' : '拉取'}失败: ${err}`);
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

    // ── Render helpers ─────────────────────────────────────────────────────

    getAuthorInitials(author: string) {
        if (!author) return 'U';
        return author.trim().charAt(0).toUpperCase();
    }

    getAuthorColor(author: string) {
        if (!author) return 'var(--accent-color)';
        let hash = 0;
        for (let i = 0; i < author.length; i++) {
            hash = author.charCodeAt(i) + ((hash << 5) - hash);
        }
        const colors = [
            '#3b82f6', // blue
            '#10b981', // green
            '#8b5cf6', // purple
            '#f59e0b', // amber
            '#ec4899', // pink
            '#06b6d4', // cyan
            '#14b8a6', // teal
            '#6366f1', // indigo
        ];
        const index = Math.abs(hash) % colors.length;
        return colors[index];
    }

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
        if (!diffFile) return null;

        const parsedLines = this.parseDiffLines(diffContent);

        return (
            <div class="git-diff-panel" onClick={e => e.stopPropagation()}>
                <div class="git-diff-header">
                    <span class="git-diff-title">{diffFile}</span>
                    <button
                        class="git-diff-close-btn"
                        onClick={() => this.setState({ diffFile: null, diffContent: '' })}
                        title="关闭差异"
                    >
                        ×
                    </button>
                </div>
                {diffLoading ? (
                    <div class="git-diff-loading">
                        <div class="git-spinner" />
                        <span>正在计算差异…</span>
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
                    <div class="git-diff-empty">文件内容无差异</div>
                )}
            </div>
        );
    }

    renderFileRow(file: FileStatus, section: 'staged' | 'unstaged' | 'untracked') {
        const { diffFile, diffStaged } = this.state;
        const isStaged = section === 'staged';
        const isOpen = diffFile === file.path && diffStaged === isStaged;
        const statusCls = STATUS_COLOR[file.status] || 'git-status-u';
        const label = STATUS_LABEL[file.status] || file.status;

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
                                title="取消暂存"
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
                                        title="放弃更改 (Restore)"
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
                                    title="暂存文件"
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
                <h3 class="git-clean-title">工作区非常干净</h3>
                <p class="git-clean-desc">没有任何未提交的更改。您可以安心开发新功能或切换到其他工作分支。</p>
                <button class="git-clean-refresh-btn" onClick={this.refresh}>
                    {IconRefresh} 刷新状态
                </button>
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
            log,
            logLoading,
            logExpanded,
            toast,
            branches,
            branchDropdownOpen,
            branchesLoading,
            creatingBranch,
            newBranchName,
            showNewBranchInput,
            commitSearchQuery,
        } = this.state;

        if (!status && loading) {
            return (
                <div class="git-panel">
                    <div class="git-loading-full">
                        <div class="git-spinner" />
                        <span>读取 Git 仓库状态…</span>
                    </div>
                </div>
            );
        }

        if (!status || !status.isRepo) {
            return (
                <div class="git-panel">
                    <div class="git-no-repo">
                        <div class="git-no-repo-icon">⎇</div>
                        <span>当前目录不是 Git 仓库</span>
                        <span class="git-no-repo-hint">请在终端中运行 `git init` 初始化仓库</span>
                    </div>
                </div>
            );
        }

        const staged = status.staged || [];
        const unstaged = status.unstaged || [];
        const untracked = status.untracked || [];
        const stagedCount = staged.length;
        const hasStaged = stagedCount > 0;

        // Filter log history based on search query
        const filteredLog = log.filter(c => {
            if (!commitSearchQuery.trim()) return true;
            const q = commitSearchQuery.toLowerCase().trim();
            return (
                c.message.toLowerCase().includes(q) ||
                c.author.toLowerCase().includes(q) ||
                c.hash.toLowerCase().includes(q)
            );
        });

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
                            title="切换 / 创建分支"
                        >
                            <span class="git-branch-icon">{IconBranch}</span>
                            <span class="git-branch-name">{status.branch}</span>
                            <span class="git-branch-arrow">▼</span>
                        </div>

                        {(status.ahead > 0 || status.behind > 0) && (
                            <span class="git-ahead-behind">
                                {status.ahead > 0 && (
                                    <span class="git-ahead" title={`领先远程 ${status.ahead} 个提交`}>
                                        ↑{status.ahead}
                                    </span>
                                )}
                                {status.behind > 0 && (
                                    <span class="git-behind" title={`落后远程 ${status.behind} 个提交`}>
                                        ↓{status.behind}
                                    </span>
                                )}
                            </span>
                        )}

                        <button
                            class={`git-icon-btn ${loading ? 'spinning' : ''}`}
                            onClick={this.refresh}
                            title="刷新仓库状态"
                        >
                            {IconRefresh}
                        </button>
                    </div>

                    {/* Branch dropdown list */}
                    {branchDropdownOpen && (
                        <div class="git-branch-dropdown">
                            <div class="git-dropdown-header">
                                <span>选择分支</span>
                                <button
                                    class={`git-create-branch-toggle-btn ${showNewBranchInput ? 'active' : ''}`}
                                    onClick={e => {
                                        e.stopPropagation();
                                        this.setState({ showNewBranchInput: !showNewBranchInput });
                                    }}
                                    title="新建分支"
                                >
                                    {IconPlus}
                                </button>
                            </div>

                            {showNewBranchInput && (
                                <div class="git-new-branch-box" onClick={e => e.stopPropagation()}>
                                    <input
                                        type="text"
                                        class="git-new-branch-input"
                                        placeholder="分支名称…"
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
                                        {creatingBranch ? '…' : '创建'}
                                    </button>
                                </div>
                            )}

                            <div class="git-branch-list">
                                {branchesLoading ? (
                                    <div class="git-dropdown-loading">
                                        <div class="git-spinner" />
                                        <span>加载中…</span>
                                    </div>
                                ) : branches.length === 0 ? (
                                    <div class="git-dropdown-empty">暂无可用分支</div>
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

                {/* Commit box */}
                <div class="git-commit-box">
                    <textarea
                        class="git-commit-input"
                        placeholder={
                            hasStaged ? '输入提交说明（Ctrl+Enter 快捷提交）' : '请先暂存文件，然后输入提交说明…'
                        }
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
                            class="git-commit-btn"
                            onClick={this.commit}
                            disabled={!hasStaged || !commitMsg.trim() || committing}
                            title="Ctrl+Enter 提交暂存"
                        >
                            {committing ? (
                                '提交中…'
                            ) : (
                                <Fragment>
                                    {IconCommit}
                                    <span>提交{stagedCount > 0 ? ` (${stagedCount})` : ''}</span>
                                </Fragment>
                            )}
                        </button>
                        <button
                            class="git-push-btn"
                            onClick={() => this.pushOrPull('push')}
                            disabled={pushPullLoading !== null}
                            title="推送 (Push)"
                        >
                            {pushPullLoading === 'push' ? <div class="git-spinner" /> : IconPush}
                        </button>
                        <button
                            class="git-pull-btn"
                            onClick={() => this.pushOrPull('pull')}
                            disabled={pushPullLoading !== null}
                            title="拉取 (Pull)"
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
                        {this.renderSection('已暂存的更改', staged, 'staged', () => this.unstage(null), '全部取消')}
                        {this.renderSection('未暂存的更改', unstaged, 'unstaged', () => this.stage(null), '全部暂存 +')}
                        {this.renderSection(
                            '未跟踪的文件',
                            untracked,
                            'untracked',
                            () => this.stage(null),
                            '全部暂存 +'
                        )}
                    </div>
                )}

                {/* Commit log history timeline */}
                <div class="git-section git-log-section">
                    <div
                        class="git-section-header git-section-header-clickable"
                        onClick={logExpanded ? () => this.setState({ logExpanded: false }) : this.loadLog}
                    >
                        <span class="git-section-title">
                            {IconChevron(logExpanded)}
                            最近提交历史
                        </span>
                    </div>

                    {logExpanded && (
                        <div class="git-log-container">
                            {/* Search box for commit filter */}
                            <div class="git-log-search-bar" onClick={e => e.stopPropagation()}>
                                <span class="git-log-search-icon">{IconSearch}</span>
                                <input
                                    type="text"
                                    class="git-log-search-input"
                                    placeholder="根据信息 / 作者 / 哈希过滤…"
                                    value={commitSearchQuery}
                                    onInput={e =>
                                        this.setState({ commitSearchQuery: (e.target as HTMLInputElement).value })
                                    }
                                />
                                {commitSearchQuery && (
                                    <button
                                        class="git-log-search-clear"
                                        onClick={() => this.setState({ commitSearchQuery: '' })}
                                        title="清空搜索"
                                    >
                                        ×
                                    </button>
                                )}
                            </div>

                            {logLoading ? (
                                <div class="git-loading-row">
                                    <div class="git-spinner" />
                                    <span>正在加载提交历史…</span>
                                </div>
                            ) : filteredLog.length === 0 ? (
                                <div class="git-log-empty">未匹配到任何提交记录</div>
                            ) : (
                                <div class="git-log-timeline">
                                    <div class="git-log-timeline-line" />
                                    {filteredLog.map(c => {
                                        const avatarBg = this.getAuthorColor(c.author);
                                        const initials = this.getAuthorInitials(c.author);
                                        return (
                                            <div
                                                key={c.hash}
                                                class="git-log-timeline-item"
                                                title={`作者: ${c.author}\n完整哈希: ${c.hash}`}
                                            >
                                                <div class="git-log-avatar-container">
                                                    <div class="git-log-avatar" style={{ backgroundColor: avatarBg }}>
                                                        {initials}
                                                    </div>
                                                </div>
                                                <div class="git-log-details">
                                                    <div class="git-log-msg-row">
                                                        <span class="git-log-msg">{c.message}</span>
                                                        <span
                                                            class="git-log-hash"
                                                            onClick={e => {
                                                                e.stopPropagation();
                                                                navigator.clipboard.writeText(c.hash);
                                                                this.showToast('提交哈希已成功复制 📋');
                                                            }}
                                                            title="点击复制完整哈希值"
                                                        >
                                                            {c.short}
                                                        </span>
                                                    </div>
                                                    <div class="git-log-meta-row">
                                                        <span class="git-log-author">{c.author}</span>
                                                        <span class="git-log-bullet">•</span>
                                                        <span class="git-log-time">{relativeTime(c.time)}</span>
                                                    </div>
                                                </div>
                                            </div>
                                        );
                                    })}
                                </div>
                            )}
                        </div>
                    )}
                </div>

                {/* Modern slide-in Toast Notification */}
                {toast && (
                    <div class="git-toast-wrapper">
                        <div class="git-toast">{toast}</div>
                    </div>
                )}
            </div>
        );
    }
}
