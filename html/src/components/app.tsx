import { h, Component } from 'preact';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

import { WorkspaceFolder, Workspace, FsEntry, RightDrawerTab } from './types';
import { LeftSidebar } from './sidebar/LeftSidebar';
import { WorkspaceHeader } from './header/WorkspaceHeader';
import { MiddleCanvas } from './canvas/MiddleCanvas';
import { RightPanel } from './drawer/RightPanel';

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

interface AppState {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    activeDrawerTab: RightDrawerTab;
    theme: 'light' | 'dark';
    hostname: string;
    leftSidebarOpen: boolean;
    leftSidebarWidth: number;
    rightPanelWidth: number;
    bottomNavHidden: boolean;
    // ── Workspace state (from API) ──
    workspaces: Workspace[];
    workspacesLoading: boolean;
    folders: WorkspaceFolder[];
    // ── Workspace modal state ──
    wsModalOpen: boolean;
    wsModalMode: 'create' | 'rename';
    wsModalTarget: Workspace | null;
    wsModalName: string;
    wsModalPath: string;
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
            leftSidebarOpen: window.innerWidth > 768,
            leftSidebarWidth: 260,
            rightPanelWidth: 320,
            bottomNavHidden: false,
            workspaces: [],
            workspacesLoading: false,
            folders: [],
            wsModalOpen: false,
            wsModalMode: 'create',
            wsModalTarget: null,
            wsModalName: '',
            wsModalPath: '',
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
        this.loadWorkspaces();
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

    // ── Workspace API helpers ─────────────────────────────────────────────────

    /** Fetch all workspaces from GET /api/workspace/list */
    loadWorkspaces = async () => {
        this.setState({ workspacesLoading: true });
        try {
            const res = await fetch('/api/workspace/list');
            if (!res.ok) throw new Error(await res.text());
            const workspaces: Workspace[] = await res.json();
            // Preserve existing expand state by merging
            const existing = this.state.folders;
            const folders = workspaces.map(ws => {
                const prev = existing.find(f => f.id === ws.id);
                return {
                    id: ws.id,
                    name: ws.name,
                    expanded: prev ? prev.expanded : false,
                    children: prev ? prev.children : [],
                };
            });
            this.setState({ workspaces, folders, workspacesLoading: false });
        } catch (err) {
            console.error('[workspace] load error:', err);
            this.setState({ workspacesLoading: false });
        }
    };

    /** Create a new workspace via POST /api/workspace/create */
    createWorkspace = async (name: string, path: string) => {
        const id = name
            .toLowerCase()
            .replace(/\s+/g, '-')
            .replace(/[^a-z0-9-]/g, '');
        const ws: Workspace = { id, name, path };
        try {
            const res = await fetch('/api/workspace/create', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(ws),
            });
            if (!res.ok) throw new Error(await res.text());
            await this.loadWorkspaces();
            this.showToast(`工作空间 "${name}" 已创建 ✓`);
        } catch (err) {
            this.showToast(`创建失败: ${err}`);
        }
    };

    /** Update an existing workspace via POST /api/workspace/update */
    updateWorkspace = async (ws: Workspace) => {
        try {
            const res = await fetch('/api/workspace/update', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(ws),
            });
            if (!res.ok) throw new Error(await res.text());
            await this.loadWorkspaces();
            this.showToast('工作空间已更新 ✓');
        } catch (err) {
            this.showToast(`更新失败: ${err}`);
        }
    };

    /** Delete a workspace via DELETE /api/workspace/delete?id=xxx */
    deleteWorkspace = async (id: string) => {
        try {
            const res = await fetch(`/api/workspace/delete?id=${encodeURIComponent(id)}`, {
                method: 'DELETE',
            });
            if (!res.ok) throw new Error(await res.text());
            await this.loadWorkspaces();
            this.showToast('工作空间已删除 ✓');
        } catch (err) {
            this.showToast(`删除失败: ${err}`);
        }
    };

    /** Open native folder picker and create workspace from selected directory */
    openCreateWorkspacePicker = async () => {
        try {
            const res = await fetch('/api/workspace/pick-directory', { method: 'POST' });
            if (!res.ok) throw new Error(await res.text());
            const data = await res.json();
            const pickedPath = (data.path || '').trim();
            if (!pickedPath) return; // user cancelled

            const sep = pickedPath.includes('\\') ? '\\' : '/';
            const dirName = pickedPath.split(sep).filter(Boolean).pop() || pickedPath;
            await this.createWorkspace(dirName, pickedPath);
        } catch (err) {
            this.showToast(`选取目录失败: ${err}`);
        }
    };

    /** Open the modal for renaming/editing an existing workspace */
    openRenameWorkspaceModal = (ws: Workspace) => {
        this.setState({
            wsModalOpen: true,
            wsModalMode: 'rename',
            wsModalTarget: ws,
            wsModalName: ws.name,
            wsModalPath: ws.path,
        });
    };

    closeWsModal = () => {
        this.setState({ wsModalOpen: false, wsModalTarget: null, wsModalName: '', wsModalPath: '' });
    };

    submitWsModal = async () => {
        const { wsModalMode, wsModalTarget, wsModalName, wsModalPath } = this.state;
        if (!wsModalName.trim()) return;
        this.closeWsModal();
        if (wsModalMode === 'create') {
            await this.createWorkspace(wsModalName.trim(), wsModalPath.trim());
        } else if (wsModalMode === 'rename' && wsModalTarget) {
            await this.updateWorkspace({ ...wsModalTarget, name: wsModalName.trim(), path: wsModalPath.trim() });
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
    private readonly IGNORE_DIRS = new Set(['node_modules', 'dist', 'build', '__pycache__', 'vendor']);

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

    render() {
        const {
            activeTab,
            activeDrawerTab,
            theme,
            leftSidebarOpen,
            leftSidebarWidth,
            rightPanelWidth,
            folders,
            workspaces,
            workspacesLoading,
            wsModalOpen,
            wsModalMode,
            wsModalName,
            wsModalPath,
            flatFiles,
            flatFilesLoading,
            searchQuery,
            selectedFilterTag,
            viewMode,
            favoriteFiles,
            detailFullscreen,
            isEditingDetail,
            selectedFsEntry,
            fileContent,
            editedContent,
            fileLoading,
            fileSaving,
            fileSaveMsg,
            isImagePreview,
            imageDataUrl,
            toastMsg,
        } = this.state;

        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
        } as ITerminalOptions;

        return (
            <div class="app-container">
                {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar */}
                <LeftSidebar
                    folders={folders}
                    workspaces={workspaces}
                    workspacesLoading={workspacesLoading}
                    leftSidebarOpen={leftSidebarOpen}
                    leftSidebarWidth={leftSidebarWidth}
                    toggleLeftSidebar={this.toggleLeftSidebar}
                    toggleFolder={this.toggleFolder}
                    toggleDrawerTab={this.toggleDrawerTab}
                    onCreateWorkspace={this.openCreateWorkspacePicker}
                    onRenameWorkspace={ws => this.openRenameWorkspaceModal(ws)}
                    onDeleteWorkspace={this.deleteWorkspace}
                />

                {/* Workspace create/rename modal */}
                {wsModalOpen && (
                    <div class="ws-modal-overlay" onClick={this.closeWsModal}>
                        <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                            <div class="ws-modal-header">
                                <span>{wsModalMode === 'create' ? '新建工作空间' : '编辑工作空间'}</span>
                                <button class="ws-modal-close" onClick={this.closeWsModal}>
                                    ✕
                                </button>
                            </div>
                            <div class="ws-modal-body">
                                <label class="ws-modal-label">名称</label>
                                <input
                                    class="ws-modal-input"
                                    placeholder="工作空间名称"
                                    value={wsModalName}
                                    onInput={(e: Event) =>
                                        this.setState({ wsModalName: (e.target as HTMLInputElement).value })
                                    }
                                    onKeyDown={(e: KeyboardEvent) => {
                                        if (e.key === 'Enter') this.submitWsModal();
                                    }}
                                    autoFocus
                                />
                                <label class="ws-modal-label">路径</label>
                                <input
                                    class="ws-modal-input"
                                    placeholder="/path/to/project  (可选)"
                                    value={wsModalPath}
                                    onInput={(e: Event) =>
                                        this.setState({ wsModalPath: (e.target as HTMLInputElement).value })
                                    }
                                    onKeyDown={(e: KeyboardEvent) => {
                                        if (e.key === 'Enter') this.submitWsModal();
                                    }}
                                />
                            </div>
                            <div class="ws-modal-footer">
                                <button class="ws-modal-cancel" onClick={this.closeWsModal}>
                                    取消
                                </button>
                                <button class="ws-modal-confirm" onClick={this.submitWsModal}>
                                    {wsModalMode === 'create' ? '创建' : '保存'}
                                </button>
                            </div>
                        </div>
                    </div>
                )}

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
                    <WorkspaceHeader
                        leftSidebarOpen={leftSidebarOpen}
                        toggleLeftSidebar={this.toggleLeftSidebar}
                        activeDrawerTab={activeDrawerTab}
                        toggleDrawerTab={this.toggleDrawerTab}
                        activeTab={activeTab}
                        setActiveTab={this.setActiveTab}
                        theme={theme}
                        toggleTheme={this.toggleTheme}
                    />

                    {/* [WORKSPACE BODY CONTAINER]: terminal & drawers */}
                    <div class="workspace-body-container">
                        {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                        <MiddleCanvas
                            activeTab={activeTab}
                            wsUrl={wsUrl}
                            tokenUrl={tokenUrl}
                            clientOptions={clientOptions}
                            termOptions={termOptions}
                            flowControl={flowControl}
                        />

                        {/* Resizer: between MIDDLE canvas and RIGHT panel */}
                        {activeDrawerTab !== 'none' && (
                            <div
                                class="resizer resizer-right"
                                onMouseDown={(e: MouseEvent) => this.handleResizerDown('right', e)}
                                title="拖动调整右侧栏宽度"
                            />
                        )}

                        {/* [COLUMN 3]: RIGHT side dynamic sliding drawer panel */}
                        <RightPanel
                            activeDrawerTab={activeDrawerTab}
                            rightPanelWidth={rightPanelWidth}
                            closeDrawer={() => this.setState({ activeDrawerTab: 'none' })}
                            theme={theme}
                            toggleTheme={this.toggleTheme}
                            flatFiles={flatFiles}
                            flatFilesLoading={flatFilesLoading}
                            searchQuery={searchQuery}
                            selectedFilterTag={selectedFilterTag}
                            viewMode={viewMode}
                            favoriteFiles={favoriteFiles}
                            detailFullscreen={detailFullscreen}
                            isEditingDetail={isEditingDetail}
                            selectedFsEntry={selectedFsEntry}
                            fileContent={fileContent}
                            editedContent={editedContent}
                            fileLoading={fileLoading}
                            fileSaving={fileSaving}
                            fileSaveMsg={fileSaveMsg}
                            isImagePreview={isImagePreview}
                            imageDataUrl={imageDataUrl}
                            onSearchQueryChange={query => this.setState({ searchQuery: query })}
                            onFilterTagChange={tag => this.setState({ selectedFilterTag: tag })}
                            onRefreshFlatFiles={this.loadFlatFiles}
                            onOpenFileDetail={this.openFileDetail}
                            onBackToList={() => this.setState({ viewMode: 'list' })}
                            onToggleFavorite={this.toggleFavorite}
                            onCopyContent={this.copyFileContent}
                            onDuplicateFile={this.duplicateFile}
                            onDownloadFile={this.downloadFile}
                            onRenameFile={this.renameFile}
                            onToggleFullscreen={() => this.setState(s => ({ detailFullscreen: !s.detailFullscreen }))}
                            onSaveFile={this.saveFile}
                            onToggleEditing={isEditing => this.setState({ isEditingDetail: isEditing })}
                            onEditedContentChange={content => this.setState({ editedContent: content })}
                            fsEntries={this.state.fsEntries}
                            fsLoading={this.state.fsLoading}
                            onToggleFsDir={this.toggleFsDir}
                        />
                    </div>
                </div>

                {/* Toast Notification */}
                {toastMsg && (
                    <div class="fb-toast">
                        <span>{toastMsg}</span>
                    </div>
                )}
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
