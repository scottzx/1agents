import { h, Component, Fragment } from 'preact';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

import { WorkspaceFolder, Workspace, FsEntry, RightDrawerTab, TmuxWindow, Session } from './types';
import { LeftSidebar } from './sidebar/LeftSidebar';
import { WorkspaceHeader } from './header/WorkspaceHeader';
import { MiddleCanvas } from './canvas/MiddleCanvas';
import { RightPanel } from './drawer/RightPanel';
import { FileDetailView } from './drawer/FileDetailView';
import { AccessTokenGate } from './auth/AccessTokenGate';
import { WelcomeOnboarding } from './welcome/WelcomeOnboarding';
import { WorkspaceModal, DirPickerModal, AccessTokenModal } from './modal';
import { workspaceService } from '../services/workspaceService';
import { terminalService } from '../services/terminalService';
import { fsService } from '../services/fsService';
import { accessService } from '../services/accessService';
import { crawlDirRecursive } from '../utils/fileCrawler';

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
    selectionBackground: '#0969da',
    selectionForeground: '#ffffff',
    selectionInactiveBackground: '#e2e8f0',
    black: '#1f2328',
    red: '#cf222e',
    green: '#1a7f37',
    yellow: '#9a6700',
    blue: '#0969da',
    magenta: '#8250df',
    cyan: '#1b7c83',
    white: '#57606a',
    brightBlack: '#6e7781',
    brightRed: '#d1242f',
    brightGreen: '#2da44e',
    brightYellow: '#b48600',
    brightBlue: '#2188ff',
    brightMagenta: '#a371f7',
    brightCyan: '#31929a',
    brightWhite: '#1f2328',
} as ITheme;

const darkTermTheme = {
    foreground: '#d2d2d2',
    background: '#0d1117',
    cursor: '#adadad',
    selectionBackground: '#2f81f7',
    selectionForeground: '#ffffff',
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
    fontFamily: 'JetBrains Mono, Consolas, Liberation Mono, Menlo, monospace',
    allowProposedApi: true,
    minimumContrastRatio: 4.5,
} as ITerminalOptions;

const isMobileDevice = () =>
    /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent) ||
    window.innerWidth <= 768;

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
    activeWorkspaceId: string;
    // ── Workspace modal state ──
    wsModalOpen: boolean;
    wsModalMode: 'create' | 'rename';
    wsModalTarget: Workspace | null;
    wsModalName: string;
    wsModalPath: string;
    wsModalTerminalDir: string;
    wsModalChatChannel: string;
    ccConnectUrl: string;
    // ── Directory picker modal state ──
    dirPickerOpen: boolean;
    dirPickerOnSelect: ((path: string) => void) | null;
    // ── Terminal / tmux state ──
    terminalWindows: TmuxWindow[];
    terminalWindowsLoading: boolean;
    tmuxMouseOn: boolean;
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
    isMobile: boolean;
    keyboardVisible: boolean;
    viewportHeight: number;
    activeSession: Session | null;
    language: 'zh-CN' | 'en-US';
    // ── Access token state ──
    accessGateVisible: boolean;
    accessAuthRequired: boolean;
    accessAuthenticated: boolean;
    accessTokenModalToken: string;
}

// Drag resizer state (module-level for perf)
let _resizerActive: 'left' | 'right' | null = null;
let _resizerStartX = 0;
let _resizerStartWidth = 0;

export class App extends Component<{}, AppState> {
    private _tunnelHeartbeat: ReturnType<typeof setInterval> | null = null;
    private _crawlCounter = 0;

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
            activeWorkspaceId: localStorage.getItem('1agents-active-workspace') || '',
            activeSession: null,
            wsModalOpen: false,
            wsModalMode: 'create',
            wsModalTarget: null,
            wsModalName: '',
            wsModalPath: '',
            wsModalTerminalDir: '',
            wsModalChatChannel: '',
            ccConnectUrl: '',
            dirPickerOpen: false,
            dirPickerOnSelect: null,
            terminalWindows: [],
            terminalWindowsLoading: false,
            tmuxMouseOn: true,
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
            isMobile: window.innerWidth <= 768,
            keyboardVisible: false,
            viewportHeight: window.visualViewport ? window.visualViewport.height : window.innerHeight,
            language: (localStorage.getItem('1agents-language') || 'zh-CN') as 'zh-CN' | 'en-US',
            accessGateVisible: false,
            accessAuthRequired: false,
            accessAuthenticated: true,
            accessTokenModalToken: '',
        };
    }

    async componentDidMount() {
        const savedTheme = localStorage.getItem('1agents-theme') as 'light' | 'dark' | null;
        const theme = savedTheme || 'light';
        this.setState({ theme });
        document.documentElement.setAttribute('data-theme', theme);
        this.setState({ hostname: window.location.hostname || 'localhost' });

        // Check access token gate before loading any data
        await this.checkAccessStatus();
        if (this.state.accessGateVisible) {
            document.addEventListener('keydown', this.handleKeyDown);
            document.addEventListener('mousemove', this.handleResizerMove);
            document.addEventListener('mouseup', this.handleResizerUp);
            if (window.visualViewport) {
                window.visualViewport.addEventListener('resize', this.viewportResizeHandler);
            }
            this._tunnelHeartbeat = setInterval(
                () => {
                    accessService.pingTunnel();
                },
                5 * 60 * 1000
            );
            return;
        }

        // Wait for both workspaces and terminal sessions to load in parallel
        await Promise.all([this.loadWorkspaces(true), this.loadTerminals()]);

        // Synchronize terminal windows into folders
        this.mergeSessionsIntoFolders(this.state.terminalWindows);

        // Select default workspace if none is active, otherwise sync backend root
        const { workspaces, activeWorkspaceId } = this.state;
        if (!activeWorkspaceId && workspaces.length > 0) {
            await this.selectWorkspace(workspaces[0]);
        } else if (activeWorkspaceId) {
            const ws = workspaces.find(w => w.id === activeWorkspaceId);
            if (ws) {
                await this.switchWorkspaceContext(ws);
            } else {
                this.loadDir('', null);
                this.loadFlatFiles();
            }
        } else {
            this.loadDir('', null);
            this.loadFlatFiles();
        }

        this.loadTmuxMouse();
        this.checkUrlPreview();
        document.addEventListener('keydown', this.handleKeyDown);
        document.addEventListener('mousemove', this.handleResizerMove);
        document.addEventListener('mouseup', this.handleResizerUp);
        if (window.visualViewport) {
            window.visualViewport.addEventListener('resize', this.viewportResizeHandler);
        }

        // Tunnel idle heartbeat — polls /api/tunnel/status every 5 min to prevent auto-stop
        this._tunnelHeartbeat = setInterval(
            () => {
                accessService.pingTunnel();
            },
            5 * 60 * 1000
        );
    }

    componentWillUnmount() {
        document.removeEventListener('keydown', this.handleKeyDown);
        document.removeEventListener('mousemove', this.handleResizerMove);
        document.removeEventListener('mouseup', this.handleResizerUp);
        if (window.visualViewport) {
            window.visualViewport.removeEventListener('resize', this.viewportResizeHandler);
        }
        if (this._tunnelHeartbeat) {
            clearInterval(this._tunnelHeartbeat);
            this._tunnelHeartbeat = null;
        }
    }

    viewportResizeHandler = () => {
        if (this.state.isMobile) {
            this.setState({
                viewportHeight: window.visualViewport ? window.visualViewport.height : window.innerHeight,
            });
            this.triggerTerminalFit();
        }
    };

    handleKeyboardStateChange = (visible: boolean) => {
        this.setState({ keyboardVisible: visible });
        this.triggerTerminalFit();
    };

    handleKeyDown = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 's') {
            e.preventDefault();
            this.saveFile();
        }
    };

    // ── Workspace API helpers ─────────────────────────────────────────────────

    /** Fetch all workspaces from GET /api/workspace/list */
    loadWorkspaces = async (skipAutoSelect = false) => {
        this.setState({ workspacesLoading: true });
        try {
            const workspaces = await workspaceService.list();
            // Preserve existing expand state by merging
            const existing = this.state.folders;
            const folders = workspaces.map(ws => {
                const prev = existing.find(f => f.id === ws.id);
                return {
                    id: ws.id,
                    name: ws.name,
                    expanded: prev ? prev.expanded : false,
                    sessions: prev ? prev.sessions : [],
                };
            });
            this.setState({ workspaces, folders, workspacesLoading: false }, () => {
                if (skipAutoSelect) return;
                if (!this.state.activeWorkspaceId && workspaces.length > 0) {
                    this.selectWorkspace(workspaces[0]);
                } else if (this.state.activeWorkspaceId) {
                    this.loadCcConnectUrl();
                }
            });
        } catch (err) {
            console.error('[workspace] load error:', err);
            this.setState({ workspacesLoading: false });
        }
    };

    loadCcConnectUrl = async (workspaceId?: string) => {
        const wsId = workspaceId || this.state.activeWorkspaceId;
        if (!wsId) return;
        try {
            const url = await workspaceService.getCcConnectUrl(
                wsId,
                this.state.theme,
                this.state.language || 'zh-CN'
            );
            this.setState({ ccConnectUrl: url });
        } catch (err) {
            console.error('[ccconnect] failed to load url:', err);
        }
    };

    /** Create a new workspace via POST /api/workspace/create */
    createWorkspace = async (name: string, path: string, terminalDir?: string, chatChannel?: string) => {
        let id = name
            .toLowerCase()
            .replace(/\s+/g, '-')
            .replace(/[^a-z0-9-]/g, '');
        if (!id) {
            // Fallback for non-ASCII/Chinese names: generate a clean unique ID
            id = 'ws-' + Math.random().toString(36).substring(2, 10);
        }
        const ws: Workspace = {
            id,
            name,
            path,
            status: 'active',
            terminalDir: terminalDir?.trim() || undefined,
            chatChannel: chatChannel?.trim() || undefined,
        };
        try {
            await workspaceService.create(ws);
            await this.loadWorkspaces();
            this.showToast(`工作空间 "${name}" 已创建 ✓`);
        } catch (err) {
            this.showToast(`创建失败: ${err}`);
        }
    };

    /** Update an existing workspace via POST /api/workspace/update */
    updateWorkspace = async (ws: Workspace) => {
        try {
            await workspaceService.update(ws);
            await this.loadWorkspaces();
            this.showToast('工作空间已更新 ✓');
        } catch (err) {
            this.showToast(`更新失败: ${err}`);
        }
    };

    /** Delete a workspace via DELETE /api/workspace/delete?id=xxx */
    deleteWorkspace = async (id: string) => {
        try {
            await workspaceService.delete(id);
            await this.loadWorkspaces();
            this.showToast('工作空间已删除 ✓');
        } catch (err) {
            this.showToast(`删除失败: ${err}`);
        }
    };

    /** Open custom directory picker and create workspace from selected directory */
    openCreateWorkspacePicker = () => {
        this.openDirPicker(pickedPath => {
            const sep = pickedPath.includes('\\') ? '\\' : '/';
            const dirName = pickedPath.split(sep).filter(Boolean).pop() || pickedPath;

            // Open standard workspace create modal with prefilled data!
            this.setState({
                wsModalOpen: true,
                wsModalMode: 'create',
                wsModalTarget: null,
                wsModalName: dirName,
                wsModalPath: pickedPath,
                wsModalTerminalDir: '',
                wsModalChatChannel: '',
            });
        });
    };

    openDirPicker = (onSelect: (path: string) => void) => {
        this.setState({
            dirPickerOpen: true,
            dirPickerOnSelect: onSelect,
        });
    };

    openDirPickerForModal = () => {
        this.openDirPicker(path => {
            this.setState({ wsModalPath: path });
        });
    };

    /** Open the modal for renaming/editing an existing workspace */
    openRenameWorkspaceModal = (ws: Workspace) => {
        this.setState({
            wsModalOpen: true,
            wsModalMode: 'rename',
            wsModalTarget: ws,
            wsModalName: ws.name,
            wsModalPath: ws.path,
            wsModalTerminalDir: ws.terminalDir || '',
            wsModalChatChannel: ws.chatChannel || '',
        });
    };

    closeWsModal = () => {
        this.setState({
            wsModalOpen: false,
            wsModalTarget: null,
            wsModalName: '',
            wsModalPath: '',
            wsModalTerminalDir: '',
            wsModalChatChannel: '',
        });
    };

    submitWsModal = async () => {
        const { wsModalMode, wsModalTarget, wsModalName, wsModalPath, wsModalTerminalDir, wsModalChatChannel } =
            this.state;
        if (!wsModalName.trim()) return;
        this.closeWsModal();
        if (wsModalMode === 'create') {
            await this.createWorkspace(
                wsModalName.trim(),
                wsModalPath.trim(),
                wsModalTerminalDir.trim(),
                wsModalChatChannel.trim()
            );
        } else if (wsModalMode === 'rename' && wsModalTarget) {
            await this.updateWorkspace({
                ...wsModalTarget,
                name: wsModalName.trim(),
                path: wsModalPath.trim(),
                terminalDir: wsModalTerminalDir.trim() || undefined,
                chatChannel: wsModalChatChannel.trim() || undefined,
            });
        }
    };

    // ── Terminal (tmux) API helpers ────────────────────────────────────────────

    /** Fetch all tmux windows from GET /api/terminal/list and sync to folders */
    loadTerminals = async () => {
        this.setState({ terminalWindowsLoading: true });
        try {
            const windows = await terminalService.list();
            this.mergeSessionsIntoFolders(windows);
            this.setState({ terminalWindows: windows, terminalWindowsLoading: false });
        } catch (err) {
            console.error('[terminal] list error:', err);
            this.setState({ terminalWindowsLoading: false });
        }
    };

    /** Sync tmux windows into workspace folders as sessions */
    mergeSessionsIntoFolders(windows: TmuxWindow[]) {
        this.setState(prev => ({
            folders: prev.folders.map(f => ({
                ...f,
                sessions: windows
                    .filter(w => w.workspaceId === f.id)
                    .map(w => ({
                        id: w.name,
                        workspaceId: w.workspaceId,
                        index: w.index,
                        name: `会话 #${w.index}`,
                        active: w.active,
                        cwd: w.cwd,
                    })),
            })),
        }));
        const activeWin = windows.find(w => w.active);
        const activeSession: Session | null = activeWin
            ? {
                  id: activeWin.name,
                  workspaceId: activeWin.workspaceId,
                  index: activeWin.index,
                  name: `会话 #${activeWin.index}`,
                  active: true,
                  cwd: activeWin.cwd,
              }
            : null;
        this.setState({ activeSession });
    }

    /** Create a new terminal tab via POST /api/terminal/create */
    createTerminal = async (workspaceId: string, cwd: string) => {
        try {
            await terminalService.create(workspaceId, cwd);
            await this.loadTerminals();
            this.showToast('新会话已创建 ✓');
        } catch (err) {
            this.showToast(`创建会话失败: ${err}`);
        }
    };

    /** Switch to a tmux window via POST /api/terminal/switch */
    switchTerminal = async (windowIndex: number) => {
        try {
            await terminalService.switch(windowIndex);
            await this.loadTerminals();
        } catch (err) {
            console.error('[terminal] switch error:', err);
        }
    };

    /** Kill a terminal tab via POST /api/terminal/kill */
    killTerminal = async (windowIndex: number) => {
        try {
            await terminalService.kill(windowIndex);
            await this.loadTerminals();
            this.showToast('会话已关闭 ✓');
        } catch (err) {
            this.showToast(`关闭会话失败: ${err}`);
        }
    };

    /** Fetch current tmux mouse mode state */
    loadTmuxMouse = async () => {
        try {
            const mouseOn = await terminalService.getMouse();
            this.setState({ tmuxMouseOn: mouseOn });
        } catch (err) {
            console.error('[terminal] load mouse state error:', err);
        }
    };

    /** Toggle tmux mouse mode state */
    toggleTmuxMouse = async () => {
        const nextState = !this.state.tmuxMouseOn;
        try {
            const actualState = await terminalService.setMouse(nextState);
            this.setState({ tmuxMouseOn: actualState });
            if (actualState) {
                this.showToast('已开启滚轮滑动模式 (可通过方向键选择历史命令) ✓');
            } else {
                this.showToast('已开启鼠标选择复制模式 (可直接拖拽选中复制) ✓');
            }
        } catch (err) {
            this.showToast(`切换鼠标模式失败: ${err}`);
        }
    };

    /**
     * Core workspace context switch — tells the backend to change its fs+git roots,
     * then resets all file-browser state and triggers a reload.
     * Called by both selectWorkspace() and selectSession().
     */
    switchWorkspaceContext = async (ws: Workspace) => {
        // Tell backend to update fs + git roots atomically
        try {
            await fsService.setContext(ws.path);
        } catch (err) {
            console.error('[context] set error:', err);
        }

        // Reset file-browser state, then reload from new root
        this.setState({
            fsEntries: [],
            selectedFsEntry: null,
            fileContent: '',
            editedContent: '',
        });
        this.loadDir('', null);
        this.loadFlatFiles();
    };

    selectSession = async (session: Session) => {
        const oldWorkspaceId = this.state.activeWorkspaceId;
        const { workspaces } = this.state;

        // 1. Optimistic UI update: Immediately mark the session as active and expand/set workspace ID
        this.setState(prev => {
            const updatedFolders = prev.folders.map(f => ({
                ...f,
                sessions: f.sessions.map(s => ({
                    ...s,
                    active: s.index === session.index,
                })),
            }));
            localStorage.setItem('1agents-active-workspace', session.workspaceId);
            return {
                activeSession: { ...session, active: true },
                folders:
                    session.workspaceId !== oldWorkspaceId
                        ? updatedFolders.map(f => (f.id === session.workspaceId ? { ...f, expanded: true } : f))
                        : updatedFolders,
                activeWorkspaceId: session.workspaceId,
            };
        });

        // Helper to perform the actual terminal window and workspace context switching
        const performSwitch = async () => {
            // Always switch the tmux window first
            await this.switchTerminal(session.index);

            if (session.workspaceId !== oldWorkspaceId) {
                this.loadCcConnectUrl(session.workspaceId);
                // Switch backend context and reload file browser / git panel
                const ws = workspaces.find(w => w.id === session.workspaceId);
                if (ws) {
                    await this.switchWorkspaceContext(ws);
                    this.showToast(`已切换到 "${ws.name}" ✓`);
                }
            }
        };

        if (this.state.isMobile) {
            // Close sidebar immediately on mobile for instant visual response
            this.setState({ leftSidebarOpen: false });
            // Delay the heavy backend connection operations by 200ms to let the slide-out CSS transition finish smoothly without main-thread jank
            setTimeout(performSwitch, 200);
        } else {
            // Desktop: switch immediately
            await performSwitch();
        }
    };

    /** Switch active workspace and cd into it in a matching tmux window */
    selectWorkspace = async (ws: Workspace) => {
        const { activeWorkspaceId, terminalWindows } = this.state;
        if (ws.id === activeWorkspaceId) return;

        this.setState({ activeWorkspaceId: ws.id }, () => {
            this.loadCcConnectUrl(ws.id);
            localStorage.setItem('1agents-active-workspace', ws.id);
        });

        // Find an existing window for this workspace, or create one
        const win =
            terminalWindows.find(w => w.workspaceId === ws.id && w.active) ||
            terminalWindows.find(w => w.workspaceId === ws.id);
        if (win) {
            await this.switchTerminal(win.index);
        } else {
            await this.createTerminal(ws.id, ws.terminalDir || ws.path);
        }

        // Switch backend context (fs + git roots) and reload file browser
        await this.switchWorkspaceContext(ws);
        this.showToast(`已切换到 "${ws.name}" ✓`);
    };

    // ── File system API helpers ──────────────────────────────────────────────

    /** Fetch directory entries from /api/fs/list and merge into the tree */
    loadDir = async (relPath: string, parent: FsEntry | null) => {
        if (!parent) {
            this.setState({ fsLoading: true });
        }
        try {
            const entries = await fsService.list(relPath);

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
                const dataUrl = await fsService.readImage(entry.path);
                this.setState({ imageDataUrl: dataUrl, isImagePreview: true, fileLoading: false });
            } catch (err) {
                console.error('[fs] image load error:', err);
                this.setState({ fileContent: `Error loading image: ${err}`, fileLoading: false });
            }
            return;
        }

        try {
            const text = await fsService.read(entry.path);
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
            await fsService.write(selectedFsEntry.path, editedContent);
            this.setState({ fileContent: editedContent, fileSaving: false, fileSaveMsg: '已保存 ✓' });
            setTimeout(() => this.setState({ fileSaveMsg: '' }), 2000);
        } catch (err) {
            console.error('[fs] write error:', err);
            this.setState({ fileSaving: false, fileSaveMsg: `保存失败: ${err}` });
        }
    };

    updateCcConnectUrlParams = (theme: 'light' | 'dark', lang: 'zh-CN' | 'en-US') => {
        const urlStr = this.state.ccConnectUrl;
        if (!urlStr) return;
        try {
            const dummyBase = 'http://dummy.com';
            const parsed = new URL(urlStr, dummyBase);
            parsed.searchParams.set('theme', theme);

            // Map BCP-47 to CC-Connect codes
            let normalLang = 'zh';
            const langLower = (lang || '').toLowerCase();
            if (langLower.startsWith('en')) {
                normalLang = 'en';
            } else if (langLower.startsWith('zh-tw') || langLower.startsWith('zh-hk')) {
                normalLang = 'zh-TW';
            } else if (langLower.startsWith('ja')) {
                normalLang = 'ja';
            } else if (langLower.startsWith('es')) {
                normalLang = 'es';
            }
            parsed.searchParams.set('lang', normalLang);

            let newUrl = parsed.pathname + parsed.search;
            if (!urlStr.startsWith('/')) {
                newUrl = parsed.toString();
            }
            this.setState({ ccConnectUrl: newUrl });
        } catch (e) {
            console.error('[ccconnect] failed to update url params:', e);
        }
    };

    toggleTheme = (themeMode?: 'light' | 'dark') => {
        const targetTheme = themeMode || (this.state.theme === 'light' ? 'dark' : 'light');
        this.setState({ theme: targetTheme }, () => {
            // Also notify the CC-Connect iframe of the theme change
            const iframe = document.getElementById('cc-connect-iframe') as HTMLIFrameElement | null;
            if (iframe && iframe.contentWindow) {
                iframe.contentWindow.postMessage({ type: 'THEME_CHANGE', theme: targetTheme }, '*');
            }
        });
        document.documentElement.setAttribute('data-theme', targetTheme);
        localStorage.setItem('1agents-theme', targetTheme);
        this.triggerTerminalFit();
    };

    toggleLanguage = (lang: 'zh-CN' | 'en-US') => {
        this.setState({ language: lang }, () => {
            // Also notify the CC-Connect iframe of the language change
            const iframe = document.getElementById('cc-connect-iframe') as HTMLIFrameElement | null;
            if (iframe && iframe.contentWindow) {
                iframe.contentWindow.postMessage({ type: 'LANG_CHANGE', lang: lang }, '*');
            }
        });
        localStorage.setItem('1agents-language', lang);
        this.showToast(`默认识别语言已切换为: ${lang === 'zh-CN' ? '中文' : 'English'} ✓`);
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
            // Collapse the drawer
            this.setState({ activeDrawerTab: 'none' });
        } else {
            // Expand drawer with smart width: wider for channels, git, and files panels
            const smartWidth =
                tab === 'channels' || tab === 'git' || tab === 'files'
                    ? Math.max(this.state.rightPanelWidth, 450)
                    : 320;
            this.setState({ activeDrawerTab: tab, rightPanelWidth: smartWidth });
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

    loadFlatFiles = async () => {
        this._crawlCounter++;
        const currentCrawl = this._crawlCounter;
        this.setState({ flatFilesLoading: true });
        try {
            const files = await crawlDirRecursive('', currentCrawl, () => this._crawlCounter);
            if (currentCrawl === this._crawlCounter) {
                this.setState({ flatFiles: files, flatFilesLoading: false });
            }
        } catch (err) {
            if (currentCrawl === this._crawlCounter) {
                console.error('[flat] crawl error:', err);
                this.setState({ flatFilesLoading: false });
            }
        }
    };

    // ── File detail action handlers ────────────────────────────────────────

    showToast = (msg: string) => {
        this.setState({ toastMsg: msg });
        setTimeout(() => this.setState({ toastMsg: '' }), 2200);
    };

    checkAccessStatus = async () => {
        try {
            const data = await accessService.checkStatus();
            this.setState({
                accessAuthRequired: data.required,
                accessAuthenticated: data.authenticated,
                accessGateVisible: data.required && !data.authenticated,
            });
        } catch {
            this.setState({
                accessAuthRequired: false,
                accessAuthenticated: true,
                accessGateVisible: false,
            });
        }
    };

    onAccessAuthenticated = async () => {
        await this.checkAccessStatus();
        if (!this.state.accessGateVisible) {
            this.loadDir('', null);
            this.loadFlatFiles();
            await Promise.all([this.loadWorkspaces(true), this.loadTerminals()]);
            this.mergeSessionsIntoFolders(this.state.terminalWindows);
            const { workspaces, activeWorkspaceId } = this.state;
            if (!activeWorkspaceId && workspaces.length > 0) {
                await this.selectWorkspace(workspaces[0]);
            } else if (activeWorkspaceId) {
                await this.loadCcConnectUrl();
            }
            this.loadTmuxMouse();
            this.checkUrlPreview();
        }
    };

    generateAccessToken = async () => {
        try {
            const token = await accessService.generateToken();
            this.setState({ accessTokenModalToken: token, accessAuthRequired: true });
        } catch (err) {
            this.showToast(`生成令牌失败: ${err}`);
        }
    };

    revokeAccessToken = async () => {
        try {
            await accessService.revokeToken();
            this.showToast('访问令牌已撤销');
            await this.checkAccessStatus();
        } catch (err) {
            this.showToast(`撤销失败: ${err}`);
        }
    };



    closeAccessTokenModal = () => {
        this.setState({ accessTokenModalToken: '' });
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
                const dataUrl = await fsService.readImage(entry.path);
                this.setState({ imageDataUrl: dataUrl, isImagePreview: true, fileLoading: false });
            } catch (err) {
                console.error('[fs] image load error:', err);
                this.setState({ fileContent: `Error loading image: ${err}`, fileLoading: false });
            }
            return;
        }

        try {
            const text = await fsService.read(entry.path);
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
            await fsService.write(newPath, fileContent);
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
            await fsService.write(newPath, fileContent);
            this.showToast('重命名成功 ✓');
            this.setState({ selectedFsEntry: { ...selectedFsEntry, name: newName, path: newPath }, viewMode: 'list' });
            this.loadFlatFiles();
        } catch (err) {
            this.showToast(`重命名失败: ${err}`);
        }
    };

    shareFile = async () => {
        const { selectedFsEntry, workspaces, activeWorkspaceId } = this.state;
        if (!selectedFsEntry) return;

        const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const absolutePath = selectedFsEntry.path.startsWith('/')
            ? selectedFsEntry.path
            : `${activeWorkspacePath}/${selectedFsEntry.path}`;

        const shareUrl = `${window.location.origin}${window.location.pathname}?preview=${encodeURIComponent(
            absolutePath
        )}`;
        try {
            await navigator.clipboard.writeText(shareUrl);
            this.showToast('分享链接已复制到剪贴板 ✓');
        } catch (_) {
            this.showToast('复制分享链接失败，请手动复制');
        }
    };

    checkUrlPreview = async () => {
        const params = new URLSearchParams(window.location.search);
        const previewPath = params.get('preview') || params.get('path') || params.get('file');
        if (!previewPath) return;

        const name = previewPath.split('/').pop() || previewPath;
        const entry: FsEntry = {
            name,
            path: previewPath,
            isDir: false,
            size: 0,
            modTime: 0,
        };

        this.setState({
            activeDrawerTab: 'files',
            viewMode: 'detail',
            detailFullscreen: true,
        });
        await this.openFileDetail(entry);
    };

    render() {
        const {
            activeTab,
            activeDrawerTab,
            theme,
            leftSidebarOpen,
            leftSidebarWidth,
            tmuxMouseOn,
            rightPanelWidth,
            folders,
            workspaces,
            workspacesLoading,
            activeWorkspaceId,
            wsModalOpen,
            wsModalMode,
            wsModalName,
            wsModalPath,
            wsModalTerminalDir,
            wsModalChatChannel,
            ccConnectUrl,
            dirPickerOpen,
            dirPickerOnSelect,
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
            activeSession,
            language,
            accessGateVisible,
            accessTokenModalToken,
            accessAuthRequired,
        } = this.state;

        // If access gate is visible, render only the gate
        if (accessGateVisible) {
            return <AccessTokenGate onAuthenticated={this.onAccessAuthenticated} />;
        }

        // Check if there is a preview query parameter in the URL
        const params = new URLSearchParams(window.location.search);
        const hasPreview = params.has('preview') || params.has('path') || params.has('file');
        if (hasPreview) {
            if (!selectedFsEntry) {
                return (
                    <div
                        class="fb-detail-view fullscreen"
                        style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; background-color: var(--bg-panel);"
                    >
                        <div class="fb-loading" style="display: flex; flex-direction: column; align-items: center;">
                            <div class="fb-loading-spinner" />
                            <span style="color: var(--text-main); margin-top: 12px;">载入分享文件预览中…</span>
                        </div>
                    </div>
                );
            }

            return (
                <div
                    class="fb-detail-view fullscreen"
                    style="height: 100vh; padding: 20px 24px; box-sizing: border-box; background-color: var(--bg-panel);"
                >
                    <FileDetailView
                        selectedFsEntry={selectedFsEntry}
                        favoriteFiles={favoriteFiles}
                        detailFullscreen={true}
                        isEditingDetail={isEditingDetail}
                        fileContent={fileContent}
                        editedContent={editedContent}
                        fileLoading={fileLoading}
                        fileSaving={fileSaving}
                        fileSaveMsg={fileSaveMsg}
                        isImagePreview={isImagePreview}
                        imageDataUrl={imageDataUrl}
                        onBackToList={() => {
                            // Go back to the main workspace by clearing URL params
                            window.location.href = window.location.origin + window.location.pathname;
                        }}
                        onToggleFavorite={this.toggleFavorite}
                        onCopyContent={this.copyFileContent}
                        onDownloadFile={this.downloadFile}
                        onRenameFile={this.renameFile}
                        onToggleFullscreen={() => {
                            window.location.href = window.location.origin + window.location.pathname;
                        }}
                        onShareFile={this.shareFile}
                        onSaveFile={this.saveFile}
                        onToggleEditing={isEditing => this.setState({ isEditingDetail: isEditing })}
                        onEditedContentChange={content => this.setState({ editedContent: content })}
                        isStandalone={true}
                    />
                    {toastMsg && (
                        <div class="fb-toast">
                            <span>{toastMsg}</span>
                        </div>
                    )}
                </div>
            );
        }

        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
            fontSize: isMobileDevice() ? 12 : 13,
        } as ITerminalOptions;

        // Derive the filesystem path of the currently active workspace
        const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
        const activeWorkspacePath = activeWorkspace?.path || '.';

        return (
            <div class="app-container">
                {workspaces.length === 0 && !workspacesLoading ? (
                    <WelcomeOnboarding language={language} onCreateWorkspace={this.openCreateWorkspacePicker} />
                ) : (
                    <Fragment>
                        {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar */}
                        <LeftSidebar
                            folders={folders}
                            workspaces={workspaces}
                            workspacesLoading={workspacesLoading}
                            leftSidebarOpen={leftSidebarOpen}
                            leftSidebarWidth={leftSidebarWidth}
                            activeWorkspaceId={activeWorkspaceId}
                            toggleLeftSidebar={this.toggleLeftSidebar}
                            toggleFolder={this.toggleFolder}
                            toggleDrawerTab={this.toggleDrawerTab}
                            onCreateWorkspace={this.openCreateWorkspacePicker}
                            onRenameWorkspace={ws => this.openRenameWorkspaceModal(ws)}
                            onDeleteWorkspace={this.deleteWorkspace}
                            onSelectWorkspace={ws => this.selectWorkspace(ws)}
                            onSelectSession={s => this.selectSession(s)}
                            onTerminalCreate={(wsId, cwd) => this.createTerminal(wsId, cwd)}
                            onTerminalKill={idx => this.killTerminal(idx)}
                        />

                        {/* Workspace create/rename modal */}
                        {wsModalOpen && (
                            <WorkspaceModal
                                mode={wsModalMode}
                                name={wsModalName}
                                path={wsModalPath}
                                terminalDir={wsModalTerminalDir}
                                chatChannel={wsModalChatChannel}
                                onNameChange={val => this.setState({ wsModalName: val })}
                                onPathChange={val => this.setState({ wsModalPath: val })}
                                onTerminalDirChange={val => this.setState({ wsModalTerminalDir: val })}
                                onChatChannelChange={val => this.setState({ wsModalChatChannel: val })}
                                onClose={this.closeWsModal}
                                onBrowse={this.openDirPickerForModal}
                                onSubmit={this.submitWsModal}
                            />
                        )}

                        {/* Remote Directory Picker Modal */}
                        {dirPickerOpen && (
                            <DirPickerModal
                                onClose={() => this.setState({ dirPickerOpen: false })}
                                onSelect={pickedPath => {
                                    if (this.state.dirPickerOnSelect) {
                                        this.state.dirPickerOnSelect(pickedPath);
                                    }
                                    this.setState({ dirPickerOpen: false });
                                }}
                                onShowToast={this.showToast}
                            />
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
                        <div
                            class="workspace-main-content"
                            style={
                                this.state.isMobile
                                    ? {
                                          // Constrain height to visual viewport when keyboard is open
                                          height: this.state.keyboardVisible
                                              ? `${this.state.viewportHeight}px`
                                              : undefined,
                                      }
                                    : undefined
                            }
                        >
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
                                keyboardVisible={this.state.keyboardVisible}
                                workspaceName={activeWorkspace?.name || ''}
                                sessionName={activeSession?.name || ''}
                                tmuxMouseOn={tmuxMouseOn}
                                onTmuxMouseToggle={this.toggleTmuxMouse}
                            />

                            {/* [WORKSPACE BODY CONTAINER]: terminal & drawers */}
                            <div class={`workspace-body-container ${activeDrawerTab !== 'none' ? 'drawer-open' : ''}`}>
                                {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                                <MiddleCanvas
                                    activeTab={activeTab as 'terminal' | 'agents' | 'console' | 'folders'}
                                    wsUrl={wsUrl}
                                    tokenUrl={tokenUrl}
                                    clientOptions={clientOptions}
                                    termOptions={termOptions}
                                    flowControl={flowControl}
                                    onMobileDetect={isMobile => this.setState({ isMobile })}
                                    onKeyboardStateChange={this.handleKeyboardStateChange}
                                    tmuxMouseOn={tmuxMouseOn}
                                    onTmuxMouseToggle={this.toggleTmuxMouse}
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
                                    activeWorkspaceId={activeWorkspaceId}
                                    activeWorkspacePath={activeWorkspacePath}
                                    rightPanelWidth={rightPanelWidth}
                                    closeDrawer={() => this.setState({ activeDrawerTab: 'none' })}
                                    ccConnectUrl={ccConnectUrl}
                                    theme={theme}
                                    toggleTheme={this.toggleTheme}
                                    language={language}
                                    toggleLanguage={this.toggleLanguage}
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
                                    onRefreshFlatFiles={async () => {
                                        this.loadDir('', null);
                                        this.loadFlatFiles();
                                        try {
                                            await this.checkAccessStatus();
                                            await Promise.all([this.loadWorkspaces(true), this.loadTerminals()]);

                                            const { workspaces, activeWorkspaceId } = this.state;
                                            if (!activeWorkspaceId && workspaces.length > 0) {
                                                await this.selectWorkspace(workspaces[0]);
                                            } else if (activeWorkspaceId) {
                                                await this.loadCcConnectUrl();
                                            }
                                        } catch (e) {
                                            console.error('Failed to reconnect/refresh:', e);
                                        }
                                    }}
                                    onOpenFileDetail={this.openFileDetail}
                                    onBackToList={() => this.setState({ viewMode: 'list', detailFullscreen: false })}
                                    onToggleFavorite={this.toggleFavorite}
                                    onCopyContent={this.copyFileContent}
                                    onDownloadFile={this.downloadFile}
                                    onRenameFile={this.renameFile}
                                    onToggleFullscreen={() => {
                                        const { selectedFsEntry, workspaces, activeWorkspaceId } = this.state;
                                        if (selectedFsEntry) {
                                            const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId);
                                            const activeWorkspacePath = activeWorkspace?.path || '.';
                                            const absolutePath = selectedFsEntry.path.startsWith('/')
                                                ? selectedFsEntry.path
                                                : `${activeWorkspacePath}/${selectedFsEntry.path}`;
                                            const shareUrl = `${window.location.origin}${
                                                window.location.pathname
                                            }?preview=${encodeURIComponent(absolutePath)}`;
                                            window.open(shareUrl, '_blank');
                                        }
                                    }}
                                    onShareFile={this.shareFile}
                                    onSaveFile={this.saveFile}
                                    onToggleEditing={isEditing => this.setState({ isEditingDetail: isEditing })}
                                    onEditedContentChange={content => this.setState({ editedContent: content })}
                                    fsEntries={this.state.fsEntries}
                                    fsLoading={this.state.fsLoading}
                                    onToggleFsDir={this.toggleFsDir}
                                    accessTokenExists={accessAuthRequired}
                                    onGenerateAccessToken={this.generateAccessToken}
                                    onRevokeAccessToken={this.revokeAccessToken}
                                />
                            </div>
                        </div>
                    </Fragment>
                )}

                {/* Access Token Display Modal (one-time, shown after generation) */}
                {accessTokenModalToken && (
                    <AccessTokenModal
                        token={accessTokenModalToken}
                        onClose={this.closeAccessTokenModal}
                        onShowToast={this.showToast}
                    />
                )}

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
