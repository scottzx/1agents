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

interface GitPanelProps {
    workdir: string;
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
}

// ── Status label map ───────────────────────────────────────────────────────

const STATUS_LABEL: Record<string, string> = {
    M: 'M',
    A: 'A',
    D: 'D',
    R: 'R',
    C: 'C',
    '?': '?',
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
    if (diff < 3600) return `${Math.floor(diff / 60)}分钟前`;
    if (diff < 86400) return `${Math.floor(diff / 3600)}小时前`;
    if (diff < 86400 * 7) return `${Math.floor(diff / 86400)}天前`;
    return new Date(ts * 1000).toLocaleDateString('zh-CN');
}

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
        <polyline points="23 4 23 10 17 10" />
        <polyline points="1 20 1 14 7 14" />
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
        style={`transform: rotate(${expanded ? 90 : 0}deg); transition: transform 0.2s`}
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
        };
    }

    componentDidMount() {
        this.refresh();
        this._refreshTimer = setInterval(() => this.refresh(), 15000);
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

    loadLog = async () => {
        this.setState({ logLoading: true, logExpanded: true });
        try {
            const res = await fetch('/api/git/log?limit=20');
            if (!res.ok) throw new Error(await res.text());
            const log: CommitEntry[] = await res.json();
            this.setState({ log, logLoading: false });
        } catch (err) {
            console.error('[git] log error:', err);
            this.setState({ logLoading: false });
        }
    };

    loadDiff = async (file: string, staged: boolean) => {
        // Toggle off if clicking the same file
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
            this.setState({ diffContent: `Error: ${err}`, diffLoading: false });
        }
    };

    // ── Actions ────────────────────────────────────────────────────────────

    stage = async (file: string | null) => {
        const url = file ? `/api/git/stage?file=${encodeURIComponent(file)}` : '/api/git/stage?all=true';
        await fetch(url, { method: 'POST' });
        // Clear diff if we just staged the viewed file
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
            this.showToast(action === 'push' ? '推送成功 ✓' : '拉取成功 ✓');
            this.refresh();
        } catch (err) {
            this.showToast(`${action === 'push' ? '推送' : '拉取'}失败: ${err}`);
        } finally {
            this.setState({ pushPullLoading: null });
        }
    };

    showToast = (msg: string) => {
        this.setState({ toast: msg });
        setTimeout(() => this.setState({ toast: '' }), 2500);
    };

    // ── Render helpers ─────────────────────────────────────────────────────

    renderDiff() {
        const { diffFile, diffContent, diffLoading } = this.state;
        if (!diffFile) return null;
        return (
            <div class="git-diff-panel">
                {diffLoading ? (
                    <div class="git-diff-loading">
                        <div class="git-spinner" />
                        <span>加载 diff…</span>
                    </div>
                ) : diffContent ? (
                    <pre class="git-diff-content">{this.renderDiffLines(diffContent)}</pre>
                ) : (
                    <div class="git-diff-empty">无差异内容</div>
                )}
            </div>
        );
    }

    renderDiffLines(content: string) {
        return content.split('\n').map((line, i) => {
            let cls = 'diff-ctx';
            if (line.startsWith('+++') || line.startsWith('---')) cls = 'diff-header';
            else if (line.startsWith('@@')) cls = 'diff-hunk';
            else if (line.startsWith('+')) cls = 'diff-add';
            else if (line.startsWith('-')) cls = 'diff-del';
            return (
                <span key={i} class={`diff-line ${cls}`}>
                    {line + '\n'}
                </span>
            );
        });
    }

    renderFileRow(file: FileStatus, section: 'staged' | 'unstaged' | 'untracked') {
        const { diffFile, diffStaged } = this.state;
        const isStaged = section === 'staged';
        const isOpen = diffFile === file.path && diffStaged === isStaged;
        const statusCls = STATUS_COLOR[file.status] || 'git-status-u';

        return (
            <Fragment key={`${section}-${file.path}`}>
                <div class={`git-file-row ${isOpen ? 'open' : ''}`}>
                    <span class={`git-file-status ${statusCls}`} title={file.status}>
                        {STATUS_LABEL[file.status] || file.status}
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
                                onClick={() => this.unstage(file.path)}
                                title="取消暂存"
                            >
                                {IconMinus}
                            </button>
                        ) : section === 'unstaged' || section === 'untracked' ? (
                            <button
                                class="git-action-btn git-action-stage"
                                onClick={() => this.stage(file.path)}
                                title="暂存"
                            >
                                {IconPlus}
                            </button>
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
        return (
            <div class="git-section">
                <div class="git-section-header">
                    <span class="git-section-title">
                        {title}
                        <span class="git-section-count">{files.length}</span>
                    </span>
                    {allAction && (
                        <button class="git-section-action" onClick={allAction} title={allLabel}>
                            {allLabel}
                        </button>
                    )}
                </div>
                <div class="git-file-list">{files.map(f => this.renderFileRow(f, section))}</div>
            </div>
        );
    }

    render() {
        const { status, loading, commitMsg, committing, pushPullLoading, log, logLoading, logExpanded, toast } =
            this.state;

        if (!status && loading) {
            return (
                <div class="git-panel">
                    <div class="git-loading-full">
                        <div class="git-spinner" />
                        <span>读取仓库状态…</span>
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
                        <span class="git-no-repo-hint">在终端运行 git init 初始化</span>
                    </div>
                </div>
            );
        }

        const stagedCount = status.staged.length;
        const hasStaged = stagedCount > 0;

        return (
            <div class="git-panel">
                {/* Branch bar */}
                <div class="git-branch-bar">
                    <span class="git-branch-icon">{IconBranch}</span>
                    <span class="git-branch-name">{status.branch}</span>
                    {(status.ahead > 0 || status.behind > 0) && (
                        <span class="git-ahead-behind">
                            {status.ahead > 0 && <span class="git-ahead">↑{status.ahead}</span>}
                            {status.behind > 0 && <span class="git-behind">↓{status.behind}</span>}
                        </span>
                    )}
                    <button class={`git-icon-btn ${loading ? 'spinning' : ''}`} onClick={this.refresh} title="刷新">
                        {IconRefresh}
                    </button>
                </div>

                {/* Commit box */}
                <div class="git-commit-box">
                    <textarea
                        class="git-commit-input"
                        placeholder={hasStaged ? '提交信息（必填）' : '暂存文件后填写提交信息…'}
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
                            title="Ctrl+Enter 提交"
                        >
                            {committing ? '提交中…' : `${IconCommit} 提交${stagedCount > 0 ? ` (${stagedCount})` : ''}`}
                        </button>
                        <button
                            class="git-push-btn"
                            onClick={() => this.pushOrPull('push')}
                            disabled={pushPullLoading !== null}
                            title="推送"
                        >
                            {pushPullLoading === 'push' ? '…' : '↑'}
                        </button>
                        <button
                            class="git-pull-btn"
                            onClick={() => this.pushOrPull('pull')}
                            disabled={pushPullLoading !== null}
                            title="拉取"
                        >
                            {pushPullLoading === 'pull' ? '…' : '↓'}
                        </button>
                    </div>
                </div>

                {/* Changes sections */}
                {status.staged.length === 0 && status.unstaged.length === 0 && status.untracked.length === 0 && (
                    <div class="git-clean-state">
                        <span>✓ 工作区干净</span>
                    </div>
                )}

                {this.renderSection('暂存的更改', status.staged, 'staged', () => this.unstage(null), '全部取消')}
                {this.renderSection('更改', status.unstaged, 'unstaged', () => this.stage(null), '全部暂存 +')}
                {this.renderSection('未跟踪', status.untracked, 'untracked', () => this.stage(null), '全部暂存 +')}

                {/* Commit log */}
                <div class="git-section git-log-section">
                    <div
                        class="git-section-header git-section-header-clickable"
                        onClick={logExpanded ? () => this.setState({ logExpanded: false }) : this.loadLog}
                    >
                        <span class="git-section-title">
                            {IconChevron(logExpanded)}
                            最近提交
                        </span>
                    </div>
                    {logExpanded && (
                        <div class="git-log-list">
                            {logLoading ? (
                                <div class="git-loading-row">
                                    <div class="git-spinner" />
                                </div>
                            ) : log.length === 0 ? (
                                <div class="git-log-empty">暂无提交历史</div>
                            ) : (
                                log.map(c => (
                                    <div key={c.hash} class="git-log-row" title={c.hash}>
                                        <span class="git-log-hash">{c.short}</span>
                                        <span class="git-log-msg">{c.message}</span>
                                        <span class="git-log-time">{relativeTime(c.time)}</span>
                                    </div>
                                ))
                            )}
                        </div>
                    )}
                </div>

                {/* Toast */}
                {toast && <div class="git-toast">{toast}</div>}
            </div>
        );
    }
}
