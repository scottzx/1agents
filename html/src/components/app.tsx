import { h, Component } from 'preact';

import { Terminal } from './terminal';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const path = window.location.pathname.replace(/[/]+$/, '');
const wsUrl = [protocol, '//', window.location.host, path, '/ws', window.location.search].join('');
const tokenUrl = [window.location.protocol, '//', window.location.host, path, '/token'].join('');

const clientOptions = {
    rendererType: 'webgl',
    disableLeaveAlert: false,
    disableResizeOverlay: false,
    enableZmodem: false,
    enableTrzsz: false,
    enableSixel: false,
    closeOnDisconnect: false,
    isWindows: false,
    unicodeVersion: '11',
} as ClientOptions;

const flowControl = {
    limit: 100000,
    highWater: 10,
    lowWater: 4,
} as FlowControl;

const lightTermTheme = {
    foreground: '#1f2328',
    background: '#fafafa',
    cursor: '#1f2328',
    black: '#1f2328',
    red: '#cf222e',
    green: '#1a7f37',
    yellow: '#9a6700',
    blue: '#0969da',
    magenta: '#8250df',
    cyan: '#1b7c83',
    white: '#ffffff',
    brightBlack: '#6e7781',
    brightRed: '#d1242f',
    brightGreen: '#2da44e',
    brightYellow: '#b48600',
    brightBlue: '#2188ff',
    brightMagenta: '#a371f7',
    brightCyan: '#31929a',
    brightWhite: '#ffffff',
} as ITheme;

const darkTermTheme = {
    foreground: '#d2d2d2',
    background: '#0d1117',
    cursor: '#adadad',
    black: '#000000',
    red: '#d81e00',
    green: '#5ea702',
    yellow: '#cfae00',
    blue: '#427ab3',
    magenta: '#89658e',
    cyan: '#00a7aa',
    white: '#dbded8',
    brightBlack: '#686a66',
    brightRed: '#f54235',
    brightGreen: '#99e343',
    brightYellow: '#fdeb61',
    brightBlue: '#84b0d8',
    brightMagenta: '#bc94b7',
    brightCyan: '#37e6e8',
    brightWhite: '#f1f1f0',
} as ITheme;

const baseTermOptions = {
    fontSize: 13,
    fontFamily: 'JetBrains Mono, Consolas, Liberation Mono, Menlo, monospace',
    allowProposedApi: true,
} as ITerminalOptions;

interface WorkspaceFolder {
    id: string;
    name: string;
    expanded: boolean;
    children: Array<{
        id: string;
        title: string;
        time: string;
        active?: boolean;
    }>;
}

/** A single file or directory entry returned by /api/fs/list */
interface FsEntry {
    name: string;
    path: string; // relative to workdir root
    isDir: boolean;
    size: number;
    modTime: number;
    // client-only: children loaded on expand
    children?: FsEntry[];
    expanded?: boolean;
}

type RightDrawerTab = 'files' | 'tasks' | 'settings' | 'none';

interface AppState {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    activeDrawerTab: RightDrawerTab;
    theme: 'light' | 'dark';
    hostname: string;
    leftSidebarOpen: boolean;
    leftSidebarWidth: number;
    rightPanelWidth: number;
    bottomNavHidden: boolean;
    folders: WorkspaceFolder[];
    // ── File system state ──
    fsEntries: FsEntry[];
    fsLoading: boolean;
    selectedFsEntry: FsEntry | null;
    fileContent: string;
    editedContent: string;
    fileLoading: boolean;
    fileSaving: boolean;
    fileSaveMsg: string;
    // ── Image preview ──
    isImagePreview: boolean;
    imageDataUrl: string;
    // ── Flat file browser ──
    flatFiles: FsEntry[];
    flatFilesLoading: boolean;
    searchQuery: string;
    selectedFilterTag: 'all' | 'doc' | 'img' | 'code';
    viewMode: 'list' | 'detail';
    favoriteFiles: string[];
    detailFullscreen: boolean;
    isEditingDetail: boolean;
    toastMsg: string;
}
// Drag resizer state (module-level for perf)
let _resizerActive: 'left' | 'right' | null = null;
let _resizerStartX = 0;
let _resizerStartWidth = 0;

export class App extends Component<{}, AppState> {
    constructor() {
        super();
        let favs: string[] = [];
        try {
            favs = JSON.parse(localStorage.getItem('fav-files') || '[]');
        } catch {
            /* ignore */
        }
        this.state = {
            activeTab: 'terminal',
            activeDrawerTab: 'none',
            theme: 'light',
            hostname: 'Ashley Walker',
            leftSidebarOpen: true,
            leftSidebarWidth: 260,
            rightPanelWidth: 320,
            bottomNavHidden: false,
            folders: [
                {
                    id: 'remote-agents',
                    name: 'remote-agents',
                    expanded: true,
                    children: [
                        { id: 'f-custom', title: 'Frontend Customization G...', time: '2m', active: false },
                        { id: 'a-terminal', title: 'Analyzing Web Terminal...', time: '11m', active: true },
                    ],
                },
                {
                    id: 'bee-write-back',
                    name: 'bee-write-back',
                    expanded: false,
                    children: [{ id: 'a-bee', title: 'Analyzing Bee Write Back', time: '3h' }],
                },
                {
                    id: 'cc-connect',
                    name: 'cc-connect',
                    expanded: false,
                    children: [{ id: 'a-cc', title: '帮我分析一下这个项目。理...', time: '4h' }],
                },
                {
                    id: 'html-slides',
                    name: 'html-slides',
                    expanded: false,
                    children: [
                        { id: 'a-slide-1', title: 'Designing Agent Collabor...', time: '18h' },
                        { id: 'a-slide-2', title: 'Designing Agent Collabor...', time: '18h' },
                    ],
                },
                {
                    id: 'html-anything',
                    name: 'html-anything',
                    expanded: false,
                    children: [{ id: 'a-anything', title: 'Querying LLM Usage', time: '1d' }],
                },
            ],
            fsEntries: [],
            fsLoading: false,
            selectedFsEntry: null,
            fileContent: '',
            editedContent: '',
            fileLoading: false,
            fileSaving: false,
            fileSaveMsg: '',
            isImagePreview: false,
            imageDataUrl: '',
            flatFiles: [],
            flatFilesLoading: false,
            searchQuery: '',
            selectedFilterTag: 'all',
            viewMode: 'list',
            favoriteFiles: favs,
            detailFullscreen: false,
            isEditingDetail: false,
            toastMsg: '',
        };
    }

    componentDidMount() {
        const savedTheme = localStorage.getItem('remote-agents-theme') as 'light' | 'dark' | null;
        const theme = savedTheme || 'light';
        this.setState({ theme });
        document.documentElement.setAttribute('data-theme', theme);
        this.setState({ hostname: window.location.hostname || 'localhost' });
        this.loadDir('', null);
        this.loadFlatFiles();
        document.addEventListener('keydown', this.handleKeyDown);
        document.addEventListener('mousemove', this.handleResizerMove);
        document.addEventListener('mouseup', this.handleResizerUp);
    }

    componentWillUnmount() {
        document.removeEventListener('keydown', this.handleKeyDown);
        document.removeEventListener('mousemove', this.handleResizerMove);
        document.removeEventListener('mouseup', this.handleResizerUp);
    }

    handleKeyDown = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 's') {
            e.preventDefault();
            this.saveFile();
        }
    };

    // ── File system API helpers ──────────────────────────────────────────────

    /** Fetch directory entries from /api/fs/list and merge into the tree */
    loadDir = async (relPath: string, parent: FsEntry | null) => {
        if (!parent) {
            this.setState({ fsLoading: true });
        }
        try {
            const res = await fetch(`/api/fs/list?path=${encodeURIComponent(relPath || '.')}`);
            if (!res.ok) throw new Error(await res.text());
            const entries: FsEntry[] = await res.json();

            if (!parent) {
                this.setState({ fsEntries: entries, fsLoading: false });
            } else {
                // Merge children into the existing tree
                this.setState(prev => ({
                    fsEntries: mergeChildren(prev.fsEntries, parent.path, entries),
                }));
            }
        } catch (err) {
            console.error('[fs] list error:', err);
            if (!parent) this.setState({ fsLoading: false });
        }
    };

    /** Toggle expand/collapse of a directory entry */
    toggleFsDir = (entry: FsEntry) => {
        if (!entry.isDir) return;
        const willExpand = !entry.expanded;
        this.setState(prev => ({
            fsEntries: setExpanded(prev.fsEntries, entry.path, willExpand),
        }));
        // Lazy-load children only on first expand
        if (willExpand && (!entry.children || entry.children.length === 0)) {
            this.loadDir(entry.path, entry);
        }
    };

    /** Open a file and load its content from /api/fs/read */
    selectFsFile = async (entry: FsEntry) => {
        if (entry.isDir) {
            this.toggleFsDir(entry);
            return;
        }
        this.setState({
            selectedFsEntry: entry,
            fileLoading: true,
            fileContent: '',
            editedContent: '',
            fileSaveMsg: '',
            isImagePreview: false,
            imageDataUrl: '',
        });

        // Check if this is an image file
        if (this.isImageFile(entry.name)) {
            try {
                const res = await fetch(`/api/fs/image?path=${encodeURIComponent(entry.path)}`);
                if (!res.ok) throw new Error(await res.text());
                const dataUrl = await res.text();
                this.setState({ imageDataUrl: dataUrl, isImagePreview: true, fileLoading: false });
            } catch (err) {
                console.error('[fs] image load error:', err);
                this.setState({ fileContent: `Error loading image: ${err}`, fileLoading: false });
            }
            return;
        }

        try {
            const res = await fetch(`/api/fs/read?path=${encodeURIComponent(entry.path)}`);
            if (!res.ok) throw new Error(await res.text());
            const text = await res.text();
            this.setState({ fileContent: text, editedContent: text, fileLoading: false });
        } catch (err) {
            console.error('[fs] read error:', err);
            this.setState({ fileContent: `Error loading file: ${err}`, editedContent: '', fileLoading: false });
        }
    };

    /** Check if a filename has an image extension */
    isImageFile(name: string): boolean {
        const ext = name.toLowerCase().split('.').pop() || '';
        return ['gif', 'png', 'jpg', 'jpeg', 'webp', 'bmp', 'svg'].includes(ext);
    }

    /** Write editedContent back to the server via /api/fs/write */
    saveFile = async () => {
        const { selectedFsEntry, editedContent, fileSaving } = this.state;
        if (!selectedFsEntry || selectedFsEntry.isDir || fileSaving) return;
        this.setState({ fileSaving: true, fileSaveMsg: '' });
        try {
            const res = await fetch(`/api/fs/write?path=${encodeURIComponent(selectedFsEntry.path)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'text/plain; charset=utf-8' },
                body: editedContent,
            });
            if (!res.ok) throw new Error(await res.text());
            this.setState({ fileContent: editedContent, fileSaving: false, fileSaveMsg: '已保存 ✓' });
            setTimeout(() => this.setState({ fileSaveMsg: '' }), 2000);
        } catch (err) {
            console.error('[fs] write error:', err);
            this.setState({ fileSaving: false, fileSaveMsg: `保存失败: ${err}` });
        }
    };

    toggleTheme = (themeMode?: 'light' | 'dark') => {
        const targetTheme = themeMode || (this.state.theme === 'light' ? 'dark' : 'light');
        this.setState({ theme: targetTheme });
        document.documentElement.setAttribute('data-theme', targetTheme);
        localStorage.setItem('remote-agents-theme', targetTheme);
        this.triggerTerminalFit();
    };

    triggerTerminalFit = () => {
        setTimeout(() => {
            const term = (window as unknown as { term?: { fit?: () => void } }).term;
            if (term && term.fit) {
                term.fit();
            }
        }, 150);
    };

    setActiveTab = (tab: 'terminal' | 'agents' | 'console' | 'folders') => {
        this.setState({ activeTab: tab });
        this.triggerTerminalFit();
    };

    // Coze click shortcut toggle dynamic drawer logic
    toggleDrawerTab = (tab: RightDrawerTab) => {
        if (this.state.activeDrawerTab === tab) {
            this.setState({ activeDrawerTab: 'none' });
        } else {
            this.setState({ activeDrawerTab: tab });
        }
        this.triggerTerminalFit();
    };

    toggleLeftSidebar = () => {
        const opening = !this.state.leftSidebarOpen;
        const leftSidebarWidth = opening
            ? this.state.leftSidebarWidth > 40
                ? this.state.leftSidebarWidth
                : 260
            : this.state.leftSidebarWidth;
        this.setState({ leftSidebarOpen: opening, leftSidebarWidth });
        this.triggerTerminalFit();
    };

    // ── Resizer drag handlers ──
    handleResizerDown = (side: 'left' | 'right', e: MouseEvent) => {
        e.preventDefault();
        _resizerActive = side;
        _resizerStartX = e.clientX;
        _resizerStartWidth = side === 'left' ? this.state.leftSidebarWidth : this.state.rightPanelWidth;
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';
    };

    handleResizerMove = (e: MouseEvent) => {
        if (!_resizerActive) return;
        const dx = e.clientX - _resizerStartX;
        if (_resizerActive === 'left') {
            const w = Math.max(160, Math.min(480, _resizerStartWidth + dx));
            this.setState({ leftSidebarWidth: w });
        } else {
            const w = Math.max(200, Math.min(600, _resizerStartWidth - dx));
            this.setState({ rightPanelWidth: w });
        }
        this.triggerTerminalFit();
    };

    handleResizerUp = () => {
        if (!_resizerActive) return;
        _resizerActive = null;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        this.triggerTerminalFit();
    };

    toggleFolder = (folderId: string) => {
        this.setState({
            folders: this.state.folders.map(f => (f.id === folderId ? { ...f, expanded: !f.expanded } : f)),
        });
    };

    // ── Flat file crawler ──────────────────────────────────────────────────

    /** Dirs to skip during recursive crawl */
    private readonly IGNORE_DIRS = new Set([
        'node_modules',
        'dist',
        '.git',
        '.yarn',
        'build',
        '__pycache__',
        '.next',
        'vendor',
    ]);

    /** Recursively fetch all files under relPath, ignoring heavy dirs */
    crawlDirRecursive = async (relPath: string): Promise<FsEntry[]> => {
        const res = await fetch(`/api/fs/list?path=${encodeURIComponent(relPath || '.')}`);
        if (!res.ok) return [];
        const entries: FsEntry[] = await res.json();
        const results: FsEntry[] = [];
        await Promise.all(
            entries.map(async e => {
                if (e.isDir) {
                    if (this.IGNORE_DIRS.has(e.name)) return;
                    const sub = await this.crawlDirRecursive(e.path);
                    results.push(...sub);
                } else {
                    results.push(e);
                }
            })
        );
        return results;
    };

    loadFlatFiles = async () => {
        this.setState({ flatFilesLoading: true });
        try {
            const files = await this.crawlDirRecursive('');
            this.setState({ flatFiles: files, flatFilesLoading: false });
        } catch (err) {
            console.error('[flat] crawl error:', err);
            this.setState({ flatFilesLoading: false });
        }
    };

    // ── File detail action handlers ────────────────────────────────────────

    showToast = (msg: string) => {
        this.setState({ toastMsg: msg });
        setTimeout(() => this.setState({ toastMsg: '' }), 2200);
    };

    openFileDetail = async (entry: FsEntry) => {
        this.setState({
            selectedFsEntry: entry,
            viewMode: 'detail',
            fileLoading: true,
            fileContent: '',
            editedContent: '',
            isEditingDetail: false,
            isImagePreview: false,
            imageDataUrl: '',
        });

        // Check if this is an image file
        if (this.isImageFile(entry.name)) {
            try {
                const res = await fetch(`/api/fs/image?path=${encodeURIComponent(entry.path)}`);
                if (!res.ok) throw new Error(await res.text());
                const dataUrl = await res.text();
                this.setState({ imageDataUrl: dataUrl, isImagePreview: true, fileLoading: false });
            } catch (err) {
                console.error('[fs] image load error:', err);
                this.setState({ fileContent: `Error loading image: ${err}`, fileLoading: false });
            }
            return;
        }

        try {
            const res = await fetch(`/api/fs/read?path=${encodeURIComponent(entry.path)}`);
            if (!res.ok) throw new Error(await res.text());
            const text = await res.text();
            this.setState({ fileContent: text, editedContent: text, fileLoading: false });
        } catch (err) {
            this.setState({ fileContent: `Error: ${err}`, editedContent: '', fileLoading: false });
        }
    };

    toggleFavorite = (path: string) => {
        const favs = this.state.favoriteFiles.includes(path)
            ? this.state.favoriteFiles.filter(p => p !== path)
            : [...this.state.favoriteFiles, path];
        this.setState({ favoriteFiles: favs });
        try {
            localStorage.setItem('fav-files', JSON.stringify(favs));
        } catch {
            /* ignore */
        }
    };

    copyFileContent = async () => {
        try {
            await navigator.clipboard.writeText(this.state.fileContent);
            this.showToast('复制成功 ✓');
        } catch (_) {
            this.showToast('复制失败');
        }
    };

    duplicateFile = async () => {
        const { selectedFsEntry, fileContent } = this.state;
        if (!selectedFsEntry) return;
        const dot = selectedFsEntry.name.lastIndexOf('.');
        const base = dot > 0 ? selectedFsEntry.name.slice(0, dot) : selectedFsEntry.name;
        const ext = dot > 0 ? selectedFsEntry.name.slice(dot) : '';
        const dir = selectedFsEntry.path.includes('/')
            ? selectedFsEntry.path.slice(0, selectedFsEntry.path.lastIndexOf('/') + 1)
            : '';
        const newPath = `${dir}${base}_copy${ext}`;
        try {
            const res = await fetch(`/api/fs/write?path=${encodeURIComponent(newPath)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'text/plain; charset=utf-8' },
                body: fileContent,
            });
            if (!res.ok) throw new Error(await res.text());
            this.showToast('已复制文件 ✓');
            this.loadFlatFiles();
        } catch (err) {
            this.showToast(`复制失败: ${err}`);
        }
    };

    downloadFile = () => {
        const { selectedFsEntry, fileContent } = this.state;
        if (!selectedFsEntry) return;
        const blob = new Blob([fileContent], { type: 'text/plain;charset=utf-8' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = selectedFsEntry.name;
        a.click();
        URL.revokeObjectURL(url);
    };

    renameFile = async () => {
        const { selectedFsEntry, fileContent } = this.state;
        if (!selectedFsEntry) return;
        const newName = window.prompt('请输入新文件名:', selectedFsEntry.name);
        if (!newName || newName === selectedFsEntry.name) return;
        const dir = selectedFsEntry.path.includes('/')
            ? selectedFsEntry.path.slice(0, selectedFsEntry.path.lastIndexOf('/') + 1)
            : '';
        const newPath = `${dir}${newName}`;
        try {
            // Write content to new path
            const writeRes = await fetch(`/api/fs/write?path=${encodeURIComponent(newPath)}`, {
                method: 'POST',
                headers: { 'Content-Type': 'text/plain; charset=utf-8' },
                body: fileContent,
            });
            if (!writeRes.ok) throw new Error(await writeRes.text());
            this.showToast('重命名成功 ✓');
            this.setState({ selectedFsEntry: { ...selectedFsEntry, name: newName, path: newPath }, viewMode: 'list' });
            this.loadFlatFiles();
        } catch (err) {
            this.showToast(`重命名失败: ${err}`);
        }
    };

    // ── Tag helpers ────────────────────────────────────────────────────────

    getFileTag(name: string): 'doc' | 'img' | 'code' | 'other' {
        const ext = name.includes('.') ? name.split('.').pop()!.toLowerCase() : '';
        const docs = ['md', 'txt', 'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'csv'];
        const imgs = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp'];
        const code = [
            'js',
            'jsx',
            'ts',
            'tsx',
            'html',
            'css',
            'scss',
            'json',
            'go',
            'py',
            'rs',
            'cpp',
            'c',
            'h',
            'sh',
            'yaml',
            'yml',
            'toml',
            'xml',
        ];
        if (docs.includes(ext)) return 'doc';
        if (imgs.includes(ext)) return 'img';
        if (code.includes(ext)) return 'code';
        return 'other';
    }

    getFilteredFiles(): FsEntry[] {
        const { flatFiles, searchQuery, selectedFilterTag } = this.state;
        return flatFiles.filter(f => {
            const matchSearch =
                !searchQuery ||
                f.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
                f.path.toLowerCase().includes(searchQuery.toLowerCase());
            const tag = this.getFileTag(f.name);
            const matchTag = selectedFilterTag === 'all' || tag === selectedFilterTag;
            return matchSearch && matchTag;
        });
    }

    /** Recursively render the file tree from FsEntry nodes */
    renderFsTree(entries: FsEntry[], depth: number): h.JSX.Element[] {
        return entries.map(entry => {
            const isSelected = this.state.selectedFsEntry?.path === entry.path;
            return (
                <div key={entry.path}>
                    <div
                        class={`file-node${isSelected ? ' active' : ''}`}
                        style={`padding-left: ${12 + depth * 16}px`}
                        onClick={() => this.selectFsFile(entry)}
                    >
                        {entry.isDir ? (
                            <svg
                                class={`folder-icon${entry.expanded ? ' expanded' : ''}`}
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                            </svg>
                        ) : (
                            <svg
                                class="file-icon"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" />
                                <path d="M14 2v4a2 2 0 0 0 2 2h4" />
                            </svg>
                        )}
                        <span class="file-name">{entry.name}</span>
                        {!entry.isDir && entry.size > 0 && <span class="file-size">{formatBytes(entry.size)}</span>}
                    </div>
                    {entry.isDir && entry.expanded && entry.children && entry.children.length > 0 && (
                        <div>{this.renderFsTree(entry.children, depth + 1)}</div>
                    )}
                </div>
            );
        });
    }

    renderDrawerTitle(tab: RightDrawerTab) {
        switch (tab) {
            case 'files':
                return '文件浏览器 (Files)';
            case 'tasks':
                return '任务调试看板 (Tasks)';
            case 'settings':
                return '系统终端设置 (Settings)';
            default:
                return '';
        }
    }

    render() {
        const { activeTab, activeDrawerTab, theme, leftSidebarOpen, folders } = this.state;
        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
        } as ITerminalOptions;

        return (
            <div class="app-container">
                {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar (直通顶部 100vh) */}
                <aside
                    class={`left-sidebar ${leftSidebarOpen ? '' : 'collapsed'}`}
                    style={leftSidebarOpen ? `width: ${this.state.leftSidebarWidth}px` : ''}
                >
                    <div class="sidebar-header">
                        <div class="coze-brand">
                            <div class="brand-left">
                                <img
                                    class="brand-logo-img"
                                    src="data:image/jpeg;base64,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"
                                />
                                <span>1agents</span>
                            </div>
                            <div class="sidebar-close-btn" onClick={this.toggleLeftSidebar} title="折叠侧边栏">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="15 18 9 12 15 6" />
                                </svg>
                            </div>
                        </div>

                        <button class="new-conv-btn">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M5 12h14M12 5v14" />
                            </svg>
                            <span>新建会话</span>
                        </button>

                        <div class="history-title-container">
                            <span>历史会话</span>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="10" />
                                <polyline points="12 6 12 12 16 14" />
                            </svg>
                        </div>
                    </div>

                    <div class="sidebar-scroll">
                        <div class="workspace-section">
                            <div class="section-header">
                                <span>工作空间 Workspaces</span>
                                <div class="header-actions">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M3 16h10M3 12h18M3 8h18" />
                                    </svg>
                                </div>
                            </div>

                            {folders.map(folder => (
                                <div key={folder.id} class="project-node">
                                    <div
                                        class={`project-folder ${folder.expanded ? 'expanded' : ''}`}
                                        onClick={() => this.toggleFolder(folder.id)}
                                    >
                                        <svg
                                            class="chevron"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="9 18 15 12 9 6" />
                                        </svg>
                                        <svg
                                            class="folder-icon"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                        </svg>
                                        <span>{folder.name}</span>
                                    </div>

                                    {folder.expanded && (
                                        <div class="project-children">
                                            {folder.children.map(child => (
                                                <div key={child.id} class={`chat-item ${child.active ? 'active' : ''}`}>
                                                    <span class="chat-title" title={child.title}>
                                                        {child.title}
                                                    </span>
                                                    <span class="chat-time">{child.time}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    </div>

                    <div class="sidebar-footer">
                        <div class="footer-item" onClick={() => this.toggleDrawerTab('settings')}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="3" />
                                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                            </svg>
                            <span>Settings</span>
                        </div>
                        <div class="footer-item">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                            </svg>
                            <span>Feedback</span>
                        </div>
                    </div>
                </aside>

                {/* Resizer: between LEFT sidebar and MIDDLE canvas */}
                {leftSidebarOpen && (
                    <div
                        class="resizer resizer-left"
                        onMouseDown={(e: MouseEvent) => this.handleResizerDown('left', e)}
                        title="拖动调整左侧栏宽度"
                    />
                )}

                {/* [WORKSPACE MAIN CONTENT]: Occupies rest of screen */}
                <div class="workspace-main-content">
                    {/* [COZE PAGE HEADER]: Replaces top global header */}
                    <header class="workspace-header">
                        <div class="header-left">
                            {!leftSidebarOpen && (
                                <button
                                    class="sidebar-toggle-btn"
                                    onClick={this.toggleLeftSidebar}
                                    style="margin-right: 4px;"
                                    title="展开左侧栏"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="9 18 15 12 9 6" />
                                    </svg>
                                </button>
                            )}
                            <div class="header-title-group">
                                <span class="title">1agents</span>
                                <div class="status-indicator">
                                    <div class="pulse-dot" />
                                    <span>运行中</span>
                                </div>
                            </div>
                        </div>

                        {/* Coze right shortcut buttons in red box */}
                        <div class="header-right">
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('files')}
                                title="文件浏览器"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                </svg>
                            </button>
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'tasks' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('tasks')}
                                title="任务追踪与调试"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" />
                                    <path d="m9 12 2 2 4-4" />
                                </svg>
                            </button>
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('settings')}
                                title="系统设置"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="3" />
                                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                                </svg>
                            </button>
                            <div class="divider" />
                            <button
                                class="shortcut-btn"
                                onClick={() => this.toggleTheme()}
                                title={theme === 'light' ? '深色主题' : '浅色主题'}
                            >
                                {theme === 'light' ? (
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                                    </svg>
                                ) : (
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <circle cx="12" cy="12" r="4" />
                                        <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                                    </svg>
                                )}
                            </button>
                        </div>
                    </header>

                    {/* [WORKSPACE BODY CONTAINER]: terminal & drawers */}
                    <div class="workspace-body-container">
                        {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                        <main class="middle-canvas">
                            <div class="terminal-toolbar">
                                <div class="toolbar-left">
                                    <h2 class="page-title">系统主控制终端</h2>
                                </div>

                                <div class="toolbar-right">
                                    <div class="shell-selector" title="选择 Shell 终端">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="4 17 10 11 4 5" />
                                            <line x1="12" x2="20" y1="19" y2="19" />
                                        </svg>
                                        <span>bash</span>
                                        <svg
                                            width="10"
                                            height="10"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="6 9 12 15 18 9" />
                                        </svg>
                                    </div>
                                    <button class="tool-btn" title="添加新标签页">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <path d="M5 12h14M12 5v14" />
                                        </svg>
                                    </button>
                                    <button class="tool-btn" title="分屏显示">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <rect width="18" height="18" x="3" y="3" rx="2" />
                                            <line x1="12" x2="12" y1="3" y2="21" />
                                        </svg>
                                    </button>
                                    <button class="tool-btn btn-danger" title="终止并清理当前终端">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <path d="M3 6h18M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" />
                                            <line x1="10" x2="10" y1="11" y2="17" />
                                            <line x1="14" x2="14" y1="11" y2="17" />
                                        </svg>
                                    </button>
                                </div>
                            </div>

                            {/* Card wrapper containing the actual Web terminal canvas */}
                            <div class="terminal-card">
                                {activeTab === 'terminal' ? (
                                    <Terminal
                                        id="terminal-container"
                                        wsUrl={wsUrl}
                                        tokenUrl={tokenUrl}
                                        clientOptions={clientOptions}
                                        termOptions={termOptions}
                                        flowControl={flowControl}
                                    />
                                ) : (
                                    <div
                                        class="placeholder-view"
                                        style="margin: 0; border: none; border-radius: 0; height: 100%;"
                                    >
                                        <svg
                                            class="placeholder-icon"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="1.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <rect width="20" height="16" x="2" y="4" rx="2" />
                                            <path d="m7 8 3 2-3 2" />
                                            <path d="M12 12h4" />
                                        </svg>
                                        <h3 class="placeholder-title">终端就绪</h3>
                                        <p class="placeholder-desc">在全局导航栏中点击【终端】以开始交互会话。</p>
                                    </div>
                                )}
                            </div>
                        </main>

                        {/* Resizer: between MIDDLE canvas and RIGHT panel */}
                        {activeDrawerTab !== 'none' && (
                            <div
                                class="resizer resizer-right"
                                onMouseDown={(e: MouseEvent) => this.handleResizerDown('right', e)}
                                title="拖动调整右侧栏宽度"
                            />
                        )}

                        {/* [COLUMN 3]: RIGHT side dynamic sliding drawer panel */}
                        <aside
                            class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`}
                            style={activeDrawerTab !== 'none' ? `width: ${this.state.rightPanelWidth}px` : ''}
                        >
                            <div class="panel-tabs-header">
                                <span class="panel-tab-title">{this.renderDrawerTitle(activeDrawerTab)}</span>
                                <div
                                    class="panel-close-btn"
                                    onClick={() => this.setState({ activeDrawerTab: 'none' })}
                                    title="收起面板"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <line x1="18" x2="6" y1="6" y2="18" />
                                        <line x1="6" x2="18" y1="6" y2="18" />
                                    </svg>
                                </div>
                            </div>

                            <div class="panel-body-scroll">
                                {activeDrawerTab === 'files' &&
                                    this.state.viewMode === 'list' &&
                                    (() => {
                                        const filtered = this.getFilteredFiles();
                                        return (
                                            <div class="flat-file-browser">
                                                {/* Search Input */}
                                                <div class="fb-search-wrap">
                                                    <svg
                                                        class="fb-search-icon"
                                                        viewBox="0 0 24 24"
                                                        fill="none"
                                                        stroke="currentColor"
                                                        stroke-width="2"
                                                        stroke-linecap="round"
                                                        stroke-linejoin="round"
                                                    >
                                                        <circle cx="11" cy="11" r="8" />
                                                        <path d="m21 21-4.35-4.35" />
                                                    </svg>
                                                    <input
                                                        id="fb-search-input"
                                                        class="fb-search-input"
                                                        type="text"
                                                        placeholder="搜索文件名或路径..."
                                                        value={this.state.searchQuery}
                                                        onInput={e =>
                                                            this.setState({
                                                                searchQuery: (e.target as HTMLInputElement).value,
                                                            })
                                                        }
                                                    />
                                                </div>
                                                {/* Filter Tags */}
                                                <div class="fb-filter-tags">
                                                    {(['all', 'doc', 'img', 'code'] as const).map(tag => (
                                                        <button
                                                            key={tag}
                                                            class={`fb-tag ${
                                                                this.state.selectedFilterTag === tag ? 'active' : ''
                                                            }`}
                                                            onClick={() => this.setState({ selectedFilterTag: tag })}
                                                        >
                                                            {tag === 'all'
                                                                ? '全部'
                                                                : tag === 'doc'
                                                                ? '文档'
                                                                : tag === 'img'
                                                                ? '图片'
                                                                : '代码'}
                                                        </button>
                                                    ))}
                                                    <button
                                                        class="fb-tag fb-tag-refresh"
                                                        onClick={this.loadFlatFiles}
                                                        title="刷新文件列表"
                                                    >
                                                        <svg
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2.5"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                            style="width:12px;height:12px"
                                                        >
                                                            <path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8" />
                                                            <path d="M3 3v5h5" />
                                                        </svg>
                                                    </button>
                                                </div>
                                                {/* File List */}
                                                {this.state.flatFilesLoading ? (
                                                    <div class="fb-loading">
                                                        <div class="fb-loading-spinner" />
                                                        <span>扫描文件中…</span>
                                                    </div>
                                                ) : filtered.length === 0 ? (
                                                    <div class="fb-empty">没有匹配的文件</div>
                                                ) : (
                                                    <div class="fb-file-list">
                                                        {filtered.map(f => {
                                                            const tag = this.getFileTag(f.name);
                                                            const ext = f.name.includes('.')
                                                                ? f.name.split('.').pop()!
                                                                : '?';
                                                            const isFav = this.state.favoriteFiles.includes(f.path);
                                                            return (
                                                                <div
                                                                    key={f.path}
                                                                    class="fb-file-row"
                                                                    onClick={() => this.openFileDetail(f)}
                                                                >
                                                                    <div class={`fb-ext-badge fb-ext-${tag}`}>
                                                                        {ext.slice(0, 4)}
                                                                    </div>
                                                                    <div class="fb-file-info">
                                                                        <span class="fb-file-name">{f.name}</span>
                                                                        <span class="fb-file-meta">
                                                                            {formatBytes(f.size)} · {f.path}
                                                                        </span>
                                                                    </div>
                                                                    {isFav && (
                                                                        <svg
                                                                            class="fb-star-indicator"
                                                                            viewBox="0 0 24 24"
                                                                            fill="currentColor"
                                                                        >
                                                                            <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                                                                        </svg>
                                                                    )}
                                                                </div>
                                                            );
                                                        })}
                                                        <div class="fb-list-footer">共 {filtered.length} 个文件</div>
                                                    </div>
                                                )}
                                            </div>
                                        );
                                    })()}

                                {activeDrawerTab === 'files' &&
                                    this.state.viewMode === 'detail' &&
                                    this.state.selectedFsEntry &&
                                    (() => {
                                        const entry = this.state.selectedFsEntry!;
                                        const isFav = this.state.favoriteFiles.includes(entry.path);
                                        const tag = this.getFileTag(entry.name);
                                        const isImg = tag === 'img';
                                        const isMd = entry.name.endsWith('.md');
                                        return (
                                            <div
                                                class={`fb-detail-view ${
                                                    this.state.detailFullscreen ? 'fullscreen' : ''
                                                }`}
                                            >
                                                {/* Detail Header */}
                                                <div class="fb-detail-header">
                                                    <button
                                                        class="fb-detail-back"
                                                        onClick={() => this.setState({ viewMode: 'list' })}
                                                        title="返回列表"
                                                    >
                                                        <svg
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2.5"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <polyline points="15 18 9 12 15 6" />
                                                        </svg>
                                                    </button>
                                                    <div class="fb-detail-title-wrap">
                                                        <span class="fb-detail-filename">{entry.name}</span>
                                                        <span class="fb-detail-path">{entry.path}</span>
                                                    </div>
                                                    <div class="fb-detail-actions">
                                                        <button
                                                            class={`fb-icon-btn ${isFav ? 'active-fav' : ''}`}
                                                            onClick={() => this.toggleFavorite(entry.path)}
                                                            title={isFav ? '取消收藏' : '收藏'}
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill={isFav ? 'currentColor' : 'none'}
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                            >
                                                                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                                                            </svg>
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={this.copyFileContent}
                                                            title="复制内容"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <rect width="14" height="14" x="8" y="8" rx="2" />
                                                                <path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2" />
                                                            </svg>
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={this.duplicateFile}
                                                            title="复制文件"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2" />
                                                                <rect width="8" height="4" x="8" y="2" rx="1" />
                                                            </svg>
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={this.downloadFile}
                                                            title="下载"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                                                <polyline points="7 10 12 15 17 10" />
                                                                <line x1="12" x2="12" y1="15" y2="3" />
                                                            </svg>
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={this.renameFile}
                                                            title="重命名"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z" />
                                                            </svg>
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={() =>
                                                                this.setState(s => ({
                                                                    detailFullscreen: !s.detailFullscreen,
                                                                }))
                                                            }
                                                            title="全屏"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M8 3H5a2 2 0 0 0-2 2v3m18 0V5a2 2 0 0 0-2-2h-3m0 18h3a2 2 0 0 0 2-2v-3M3 16v3a2 2 0 0 0 2 2h3" />
                                                            </svg>
                                                        </button>
                                                    </div>
                                                </div>
                                                {/* Save bar */}
                                                {this.state.isEditingDetail && (
                                                    <div class="fb-detail-savebar">
                                                        {this.state.fileSaveMsg && (
                                                            <span class="fb-save-msg">{this.state.fileSaveMsg}</span>
                                                        )}
                                                        <button
                                                            class="fb-save-btn"
                                                            onClick={this.saveFile}
                                                            disabled={this.state.fileSaving}
                                                        >
                                                            {this.state.fileSaving ? '保存中…' : '保存 (Ctrl+S)'}
                                                        </button>
                                                        <button
                                                            class="fb-icon-btn"
                                                            onClick={() => this.setState({ isEditingDetail: false })}
                                                            title="预览"
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M2 12s3-7 10-7 10 7 10 7-3 7-10 7-10-7-10-7Z" />
                                                                <circle cx="12" cy="12" r="3" />
                                                            </svg>
                                                        </button>
                                                    </div>
                                                )}
                                                {/* Content */}
                                                <div class="fb-detail-content">
                                                    {this.state.fileLoading ? (
                                                        <div class="fb-loading">
                                                            <div class="fb-loading-spinner" />
                                                            <span>读取文件中…</span>
                                                        </div>
                                                    ) : this.state.isImagePreview ? (
                                                        <div class="image-preview-container">
                                                            <img
                                                                src={this.state.imageDataUrl}
                                                                alt={this.state.selectedFsEntry?.name}
                                                                class="image-preview"
                                                            />
                                                        </div>
                                                    ) : isImg ? (
                                                        <div class="fb-img-preview">
                                                            <span class="fb-img-placeholder">🖼 {entry.name}</span>
                                                        </div>
                                                    ) : this.state.isEditingDetail ? (
                                                        <textarea
                                                            class="fb-editor"
                                                            spellcheck={false}
                                                            value={this.state.editedContent}
                                                            onInput={e =>
                                                                this.setState({
                                                                    editedContent: (e.target as HTMLTextAreaElement)
                                                                        .value,
                                                                })
                                                            }
                                                        />
                                                    ) : isMd ? (
                                                        <div class="fb-md-preview">
                                                            <pre
                                                                class="fb-md-raw"
                                                                onClick={() => this.setState({ isEditingDetail: true })}
                                                            >
                                                                {this.state.fileContent}
                                                            </pre>
                                                        </div>
                                                    ) : (
                                                        <pre
                                                            class="fb-code-preview"
                                                            onClick={() => this.setState({ isEditingDetail: true })}
                                                        >
                                                            {this.state.fileContent}
                                                        </pre>
                                                    )}
                                                </div>
                                            </div>
                                        );
                                    })()}

                                {activeDrawerTab === 'tasks' && (
                                    <div class="task-list-container">
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>移除了顶部全局导航栏以呈现 Coze 极简风格</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>整合会话头部标题栏，引入运行中动态绿色脉冲灯</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>引入 Coze 右上角快捷功能按钮栏 (文件树、任务控制、系统设置)</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>实现右侧滑出式抽屉面板 (Quick Drawer System) 及其缓动过渡</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>完全兼容并无损保留移动端快捷同步键盘及输入面板</span>
                                        </div>
                                    </div>
                                )}

                                {activeDrawerTab === 'settings' && (
                                    <div class="settings-container">
                                        <div class="setting-group">
                                            <span class="setting-label">色彩主题样式 (Color Theme)</span>
                                            <div class="theme-options">
                                                <button
                                                    class={`theme-btn ${theme === 'light' ? 'active' : ''}`}
                                                    onClick={() => this.toggleTheme('light')}
                                                >
                                                    <svg
                                                        width="12"
                                                        height="12"
                                                        viewBox="0 0 24 24"
                                                        fill="none"
                                                        stroke="currentColor"
                                                        stroke-width="2"
                                                        stroke-linecap="round"
                                                        stroke-linejoin="round"
                                                    >
                                                        <circle cx="12" cy="12" r="4" />
                                                        <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                                                    </svg>
                                                    <span>浅色模式</span>
                                                </button>
                                                <button
                                                    class={`theme-btn ${theme === 'dark' ? 'active' : ''}`}
                                                    onClick={() => this.toggleTheme('dark')}
                                                >
                                                    <svg
                                                        width="12"
                                                        height="12"
                                                        viewBox="0 0 24 24"
                                                        fill="none"
                                                        stroke="currentColor"
                                                        stroke-width="2"
                                                        stroke-linecap="round"
                                                        stroke-linejoin="round"
                                                    >
                                                        <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                                                    </svg>
                                                    <span>深色模式</span>
                                                </button>
                                            </div>
                                        </div>
                                    </div>
                                )}
                            </div>
                        </aside>
                    </div>
                </div>
            </div>
        );
    }
}

// ── Module-level helpers for immutable FsEntry tree manipulation ──────────────

/**
 * Walk the tree and set `children` on the node whose path matches `targetPath`.
 * Returns a new array (immutable update).
 */
function mergeChildren(entries: FsEntry[], targetPath: string, children: FsEntry[]): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, children };
        }
        if (e.children) {
            return { ...e, children: mergeChildren(e.children, targetPath, children) };
        }
        return e;
    });
}

/**
 * Walk the tree and toggle `expanded` on the node whose path matches `targetPath`.
 * Returns a new array (immutable update).
 */
function setExpanded(entries: FsEntry[], targetPath: string, expanded: boolean): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, expanded };
        }
        if (e.children) {
            return { ...e, children: setExpanded(e.children, targetPath, expanded) };
        }
        return e;
    });
}

/** Format a byte count as a human-readable string (e.g. 12.3 KB) */
function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
