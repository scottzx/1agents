import { h, Component } from 'preact';

import { WorkspaceFolder, Workspace, FsEntry, RightDrawerTab, TmuxWindow, Session, isFullPageTab } from './types';
import { FileDetailView } from './drawer/FileDetailView';
import { AccessTokenGate } from './auth/AccessTokenGate';
import { WelcomeOnboarding } from './welcome/WelcomeOnboarding';
import { WorkspaceModal, DirPickerModal, AccessTokenModal, SessionRenameModal } from './modal';
import { workspaceService } from '../services/workspaceService';
import { terminalService } from '../services/terminalService';
import { fsService } from '../services/fsService';
import { accessService } from '../services/accessService';
import { t, type Lang } from '../i18n';
import { getModuleByTab, mergeManifests, type ModuleRegistration } from '../modules/registry';
import { postToModule, isModuleInboundMessage } from '../modules/post-message';
import type { ModuleManifest } from '../modules/module-types';
import { DesktopAppLayout } from './desktop/DesktopAppLayout';
import { MobileAppLayout } from './mobile/MobileAppLayout';
import { BuiltinBrowser } from './browser/BuiltinBrowser';

import { mergeChildren, setExpanded, mergeFreshEntries } from '../utils/fsTreeUtils';

export {
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    isMobileDevice,
} from './terminal/terminalConfig';

export interface Tab {
    id: string; // 'terminal', 'preview-[path]', 'browser-[timestamp]'
    title: string;
    type: 'terminal' | 'preview' | 'browser';
    path?: string;
    url?: string;
    closable: boolean;
}

export interface AppState {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    activeDrawerTab: RightDrawerTab;
    tabs: Tab[];
    activeTabId: string;
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
    ccProvidersUrl: string;
    // ── Directory picker modal state ──
    dirPickerOpen: boolean;
    dirPickerOnSelect: ((path: string) => void) | null;
    // ── Terminal / tmux state ──
    terminalWindows: TmuxWindow[];
    terminalWindowsLoading: boolean;
    tmuxMouseOn: boolean;
    // ── Session rename modal state ──
    sessionRenameModalOpen: boolean;
    sessionRenameTarget: Session | null;
    sessionRenameName: string;
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
    language: Lang;
    // ── Access token state ──
    accessGateVisible: boolean;
    accessAuthRequired: boolean;
    accessAuthenticated: boolean;
    accessTokenModalToken: string;
    onboarded: boolean;
    hasLoadedWorkspaces: boolean;
    // ── Module slot state ──
    /** Active sub-path inside the active module, e.g. "/skills/use". */
    activeModulePath: string;
    /** Live manifest per module id (overlays the static fallback). */
    moduleManifests: Record<string, ModuleManifest>;
}

// Drag resizer state (module-level for perf)
let _resizerActive: 'left' | 'right' | null = null;
let _resizerStartX = 0;
let _resizerStartWidth = 0;

export class App extends Component<{}, AppState> {
    private _tunnelHeartbeat: ReturnType<typeof setInterval> | null = null;
    private _terminalPollInterval: ReturnType<typeof setInterval> | null = null;
    private _crawlCounter = 0;
    private _searchTimeout: number | null = null;
    private _workspaceTreeCache: Record<string, FsEntry[]> = {};

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
            workspacesLoading: true,
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
            ccProvidersUrl: '',
            dirPickerOpen: false,
            dirPickerOnSelect: null,
            terminalWindows: [],
            terminalWindowsLoading: false,
            tmuxMouseOn: true,
            sessionRenameModalOpen: false,
            sessionRenameTarget: null,
            sessionRenameName: '',
            fsEntries: [],
            fsLoading: false,
            selectedFsEntry: null,
            fileContent: '',
            editedContent: '',
            fileLoading: false,
            fileSaving: false,
            fileSaveMsg: '',
            isImagePreview: false,
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
            language: (localStorage.getItem('1agents-language') || 'zh-CN') as Lang,
            accessGateVisible: false,
            accessAuthRequired: false,
            accessAuthenticated: true,
            accessTokenModalToken: '',
            onboarded: localStorage.getItem('1agents-onboarded') === 'true',
            hasLoadedWorkspaces: false,
            tabs: [{ id: 'terminal', title: t('app.tab.workbench', 'zh-CN'), type: 'terminal', closable: false }],
            activeTabId: 'terminal',
            activeModulePath: '',
            moduleManifests: {},
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
            }
        } else {
            this.loadDir('', null);
        }

        this.loadTmuxMouse();
        this.checkUrlPreview();
        document.addEventListener('keydown', this.handleKeyDown);
        document.addEventListener('mousemove', this.handleResizerMove);
        document.addEventListener('mouseup', this.handleResizerUp);
        // Listen for postMessage events from module iframes (NAV_CHANGE, READY).
        window.addEventListener('message', this.handleModuleMessage);
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

        // Periodically poll terminal sessions (status indicator updates) every 3 seconds
        this._terminalPollInterval = setInterval(() => {
            this.loadTerminals();
        }, 3000);
    }

    componentWillUnmount() {
        document.removeEventListener('keydown', this.handleKeyDown);
        document.removeEventListener('mousemove', this.handleResizerMove);
        document.removeEventListener('mouseup', this.handleResizerUp);
        window.removeEventListener('message', this.handleModuleMessage);
        if (window.visualViewport) {
            window.visualViewport.removeEventListener('resize', this.viewportResizeHandler);
        }
        if (this._tunnelHeartbeat) {
            clearInterval(this._tunnelHeartbeat);
            this._tunnelHeartbeat = null;
        }
        if (this._terminalPollInterval) {
            clearInterval(this._terminalPollInterval);
            this._terminalPollInterval = null;
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
            this.setState({ workspaces, folders, workspacesLoading: false, hasLoadedWorkspaces: true }, () => {
                if (skipAutoSelect) return;
                const { activeWorkspaceId } = this.state;
                const activeStillExists = workspaces.some(ws => ws.id === activeWorkspaceId);
                if (!activeWorkspaceId || !activeStillExists) {
                    // Active workspace was deleted or never set — switch to first available
                    if (workspaces.length > 0) {
                        this.selectWorkspace(workspaces[0]);
                    } else {
                        // No workspaces left — clear stale state
                        this.setState({ activeWorkspaceId: '', ccConnectUrl: '', ccProvidersUrl: '' });
                    }
                } else {
                    this.loadCcConnectUrl();
                    this.loadCcProvidersUrl();
                }
            });
            return workspaces;
        } catch (err) {
            console.error('[workspace] load error:', err);
            this.setState({ workspacesLoading: false, hasLoadedWorkspaces: true });
            return [];
        }
    };

    loadCcConnectUrl = async (workspaceId?: string) => {
        const wsId = workspaceId || this.state.activeWorkspaceId;
        if (!wsId) return;
        try {
            const url = await workspaceService.getCcConnectUrl(wsId, this.state.theme, this.state.language || 'zh-CN');
            this.setState({ ccConnectUrl: url });
        } catch (err) {
            console.error('[ccconnect] failed to load url:', err);
        }
    };

    loadCcProvidersUrl = async (workspaceId?: string) => {
        const wsId = workspaceId || this.state.activeWorkspaceId;
        if (!wsId) return;
        try {
            const url = await workspaceService.getCcConnectUrl(
                wsId,
                this.state.theme,
                this.state.language || 'zh-CN',
                '/providers'
            );
            this.setState({ ccProvidersUrl: url });
        } catch (err) {
            console.error('[ccconnect] failed to load providers url:', err);
        }
    };

    onUseTempWorkspace = async () => {
        try {
            await workspaceService.create({
                id: 'temp',
                name: 'temp',
                path: 'temp',
                status: 'active',
            });
            localStorage.setItem('1agents-onboarded', 'true');
            this.setState({ onboarded: true });
            const workspaces = await this.loadWorkspaces(true);
            const tempWs = workspaces.find(w => w.id === 'temp');
            if (tempWs) {
                await this.selectWorkspace(tempWs);
            }
        } catch (err) {
            console.error('[workspace] failed to create temp workspace:', err);
            this.showToast(t('app.toast.tempCreateFailed', this.state.language, { err: String(err) }));
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
            localStorage.setItem('1agents-onboarded', 'true');
            this.setState({ onboarded: true });
            const workspaces = await this.loadWorkspaces(true);
            const newWs = workspaces.find(w => w.id === ws.id);
            if (newWs) {
                await this.selectWorkspace(newWs);
            } else {
                if (workspaces.length > 0) {
                    await this.selectWorkspace(workspaces[0]);
                }
            }
            this.showToast(t('app.toast.workspaceCreated', this.state.language, { name }));
        } catch (err) {
            this.showToast(t('app.toast.workspaceCreateFailed', this.state.language, { err: String(err) }));
        }
    };

    /** Update an existing workspace via POST /api/workspace/update */
    updateWorkspace = async (ws: Workspace) => {
        try {
            await workspaceService.update(ws);
            await this.loadWorkspaces();
            this.showToast(t('app.toast.workspaceUpdated', this.state.language));
        } catch (err) {
            this.showToast(t('app.toast.workspaceUpdateFailed', this.state.language, { err: String(err) }));
        }
    };

    /** Delete a workspace via DELETE /api/workspace/delete?id=xxx */
    deleteWorkspace = async (id: string) => {
        if (this.state.workspaces.length <= 1) {
            this.showToast(t('app.toast.workspaceDeleteLast', this.state.language));
            return;
        }
        try {
            // If we're deleting the currently active workspace, clear it first so
            // loadWorkspaces knows to auto-select a new one instead of re-fetching
            // the CC-Connect URL for a workspace that no longer exists.
            if (this.state.activeWorkspaceId === id) {
                this.setState({ activeWorkspaceId: '', ccConnectUrl: '' });
            }
            await workspaceService.delete(id);
            await this.loadTerminals();
            await this.loadWorkspaces();
            this.showToast(t('app.toast.workspaceDeleted', this.state.language));
        } catch (err) {
            this.showToast(t('app.toast.workspaceDeleteFailed', this.state.language, { err: String(err) }));
        }
    };

    /** Reorder workspaces on drag and drop */
    reorderFolders = async (draggedId: string, targetId: string, position: 'before' | 'after') => {
        const { workspaces, folders } = this.state;

        const draggedIdx = workspaces.findIndex(w => w.id === draggedId);
        const targetIdx = workspaces.findIndex(w => w.id === targetId);

        if (draggedIdx === -1 || targetIdx === -1 || draggedIdx === targetIdx) return;

        const newWorkspaces = [...workspaces];
        const [draggedItem] = newWorkspaces.splice(draggedIdx, 1);

        let newTargetIdx = newWorkspaces.findIndex(w => w.id === targetId);
        if (position === 'after') {
            newTargetIdx += 1;
        }

        newWorkspaces.splice(newTargetIdx, 0, draggedItem);

        const newFolders = newWorkspaces.map(ws => {
            const f = folders.find(folder => folder.id === ws.id);
            return f || { id: ws.id, name: ws.name, expanded: false, sessions: [] };
        });

        // Optimistic UI update
        this.setState({ workspaces: newWorkspaces, folders: newFolders });

        try {
            await workspaceService.reorder(newWorkspaces.map(w => w.id));
        } catch (err) {
            console.error('[workspace] reorder error:', err);
            // Rollback on error
            this.setState({ workspaces, folders });
            this.showToast(t('app.toast.workspaceReorderFailed', this.state.language, { err: String(err) }));
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

    openRenameSessionModal = (s: Session) => {
        this.setState({
            sessionRenameModalOpen: true,
            sessionRenameTarget: s,
            sessionRenameName: s.name,
        });
    };

    closeSessionRenameModal = () => {
        this.setState({
            sessionRenameModalOpen: false,
            sessionRenameTarget: null,
            sessionRenameName: '',
        });
    };

    submitRenameSession = async () => {
        const { sessionRenameTarget, sessionRenameName } = this.state;
        if (!sessionRenameTarget) return;
        const trimmed = sessionRenameName.trim();
        try {
            await terminalService.rename(sessionRenameTarget.id, trimmed);
            this.closeSessionRenameModal();
            await this.loadTerminals();
            this.showToast(t('app.toast.sessionRenamed', this.state.language));
        } catch (err) {
            this.showToast(t('app.toast.sessionRenameFailed', this.state.language, { err: String(err) }));
        }
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
                        name: w.customName || t('app.session.title', this.state.language, { index: w.index }),
                        active: w.active,
                        cwd: w.cwd,
                        status: w.status,
                        waitingFor: w.waitingFor,
                        agent: w.agent,
                    })),
            })),
        }));
        const activeWin = windows.find(w => w.active);
        const activeSession: Session | null = activeWin
            ? {
                  id: activeWin.name,
                  workspaceId: activeWin.workspaceId,
                  index: activeWin.index,
                  name: activeWin.customName || t('app.session.title', this.state.language, { index: activeWin.index }),
                  active: true,
                  cwd: activeWin.cwd,
                  status: activeWin.status,
                  waitingFor: activeWin.waitingFor,
                  agent: activeWin.agent,
              }
            : null;
        this.setState({ activeSession });
    }

    /** Create a new terminal tab via POST /api/terminal/create */
    createTerminal = async (workspaceId: string, cwd: string) => {
        try {
            await terminalService.create(workspaceId, cwd);
            await this.loadTerminals();
            this.showToast(t('app.toast.sessionCreated', this.state.language));
        } catch (err) {
            this.showToast(t('app.toast.sessionCreateFailed', this.state.language, { err: String(err) }));
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
            this.showToast(t('app.toast.sessionKilled', this.state.language));
        } catch (err) {
            this.showToast(t('app.toast.sessionKillFailed', this.state.language, { err: String(err) }));
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
                this.showToast(t('app.toast.mouseScrollOn', this.state.language));
            } else {
                this.showToast(t('app.toast.mouseSelectOn', this.state.language));
            }
        } catch (err) {
            this.showToast(t('app.toast.mouseToggleFailed', this.state.language, { err: String(err) }));
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

        const cached = this._workspaceTreeCache[ws.id] || [];

        // Reset file-browser state, using cache if available to prevent UI flashing
        this.setState({
            fsEntries: cached,
            selectedFsEntry: null,
            fileContent: '',
            editedContent: '',
            fsLoading: cached.length === 0,
        });
        this.loadDir('', null);
    };

    selectSession = async (session: Session) => {
        if (isFullPageTab(this.state.activeDrawerTab)) {
            this.setState({ activeDrawerTab: 'none' });
        }
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
                this.loadCcProvidersUrl(session.workspaceId);
                // Switch backend context and reload file browser / git panel
                const ws = workspaces.find(w => w.id === session.workspaceId);
                if (ws) {
                    await this.switchWorkspaceContext(ws);
                    this.showToast(t('app.toast.workspaceSwitched', this.state.language, { name: ws.name }));
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
        if (isFullPageTab(this.state.activeDrawerTab)) {
            this.setState({ activeDrawerTab: 'none' });
        }
        const { activeWorkspaceId, terminalWindows } = this.state;
        if (ws.id === activeWorkspaceId) return;

        this.setState({ activeWorkspaceId: ws.id }, () => {
            this.loadCcConnectUrl(ws.id);
            this.loadCcProvidersUrl(ws.id);
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
        this.showToast(t('app.toast.workspaceSwitched', this.state.language, { name: ws.name }));
    };

    // ── File system API helpers ──────────────────────────────────────────────

    /** Fetch directory entries from /api/fs/list and merge into the tree */
    loadDir = async (relPath: string, parent: FsEntry | null) => {
        if (!parent) {
            const hasCache = this.state.fsEntries && this.state.fsEntries.length > 0;
            if (!hasCache) {
                this.setState({ fsLoading: true });
            }
        }
        try {
            const entries = await fsService.list(relPath);

            if (!parent) {
                this.setState(prev => {
                    let nextEntries = entries;
                    if (prev.fsEntries && prev.fsEntries.length > 0) {
                        nextEntries = mergeFreshEntries(prev.fsEntries, entries);
                    }
                    if (this.state.activeWorkspaceId) {
                        this._workspaceTreeCache[this.state.activeWorkspaceId] = nextEntries;
                    }
                    return {
                        fsEntries: nextEntries,
                        fsLoading: false,
                    };
                });
            } else {
                // Merge children into the existing tree
                this.setState(prev => {
                    const nextEntries = mergeChildren(prev.fsEntries, parent.path, entries);
                    if (this.state.activeWorkspaceId) {
                        this._workspaceTreeCache[this.state.activeWorkspaceId] = nextEntries;
                    }
                    return {
                        fsEntries: nextEntries,
                    };
                });
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
        this.setState(
            prev => {
                const nextEntries = setExpanded(prev.fsEntries, entry.path, willExpand);
                if (this.state.activeWorkspaceId) {
                    this._workspaceTreeCache[this.state.activeWorkspaceId] = nextEntries;
                }
                return {
                    fsEntries: nextEntries,
                };
            },
            () => {
                // Lazy-load children only on first expand
                if (willExpand && (!entry.children || entry.children.length === 0)) {
                    this.loadDir(entry.path, entry);
                }
            }
        );
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
        });

        // Check if this is an image file
        if (this.isImageFile(entry.name)) {
            // Image is now rendered directly via <img src={fsService.imageUrl(path)}> —
            // no need to fetch into state. Just mark the preview mode.
            this.setState({ isImagePreview: true, fileLoading: false });
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
            this.setState({
                fileContent: editedContent,
                fileSaving: false,
                fileSaveMsg: t('app.toast.fileSaved', this.state.language),
            });
            setTimeout(() => this.setState({ fileSaveMsg: '' }), 2000);
        } catch (err) {
            console.error('[fs] write error:', err);
            this.setState({
                fileSaving: false,
                fileSaveMsg: t('app.toast.fileSaveFailed', this.state.language, { err: String(err) }),
            });
        }
    };

    updateCcConnectUrlParams = (theme: 'light' | 'dark', lang: Lang) => {
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
            const providersIframe = document.getElementById('cc-providers-iframe') as HTMLIFrameElement | null;
            if (providersIframe && providersIframe.contentWindow) {
                providersIframe.contentWindow.postMessage({ type: 'THEME_CHANGE', theme: targetTheme }, '*');
            }
            const skillsIframe = document.getElementById('skills-iframe') as HTMLIFrameElement | null;
            if (skillsIframe && skillsIframe.contentWindow) {
                skillsIframe.contentWindow.postMessage({ type: 'THEME_CHANGE', theme: targetTheme }, '*');
            }
        });
        document.documentElement.setAttribute('data-theme', targetTheme);
        localStorage.setItem('1agents-theme', targetTheme);
        this.triggerTerminalFit();
    };

    toggleLanguage = (lang: Lang) => {
        this.setState({ language: lang }, () => {
            // Also notify the CC-Connect iframe of the language change
            const iframe = document.getElementById('cc-connect-iframe') as HTMLIFrameElement | null;
            if (iframe && iframe.contentWindow) {
                iframe.contentWindow.postMessage({ type: 'LANG_CHANGE', lang: lang }, '*');
            }
            const providersIframe = document.getElementById('cc-providers-iframe') as HTMLIFrameElement | null;
            if (providersIframe && providersIframe.contentWindow) {
                providersIframe.contentWindow.postMessage({ type: 'LANG_CHANGE', lang: lang }, '*');
            }
            const skillsIframe = document.getElementById('skills-iframe') as HTMLIFrameElement | null;
            if (skillsIframe && skillsIframe.contentWindow) {
                skillsIframe.contentWindow.postMessage({ type: 'LANG_CHANGE', lang: lang }, '*');
            }
        });
        localStorage.setItem('1agents-language', lang);
        const langName = t(lang === 'zh-CN' ? 'app.langName.zh' : 'app.langName.en', lang);
        this.showToast(t('app.toast.langChanged', lang, { lang: langName }));
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

    selectTab = async (tabId: string) => {
        const tab = this.state.tabs.find(t => t.id === tabId);
        if (!tab) return;

        this.setState({ activeTabId: tabId });

        if (tab.type === 'preview' && tab.path) {
            const entry: FsEntry = {
                name: tab.title.replace(t('app.preview.prefix', this.state.language), ''),
                path: tab.path,
                isDir: false,
                size: 0,
                modTime: 0,
            };
            await this.openFileDetail(entry);
        } else if (tab.type === 'terminal') {
            this.triggerTerminalFit();
        }
    };

    openPreviewTab = async (path: string, fileName: string) => {
        const tabId = `preview-${path}`;
        const { tabs } = this.state;
        const exists = tabs.some(t => t.id === tabId);

        if (!exists) {
            const newTab: Tab = {
                id: tabId,
                title: `${t('app.preview.prefix', this.state.language)}${fileName}`,
                type: 'preview',
                path: path,
                closable: true,
            };
            this.setState({ tabs: [...tabs, newTab] }, () => {
                this.selectTab(tabId);
            });
        } else {
            this.selectTab(tabId);
        }
    };

    openBrowserTab = (url = '') => {
        const tabId = `browser-${Date.now()}`;
        const newTab: Tab = {
            id: tabId,
            title: t('app.browser.title', this.state.language),
            type: 'browser',
            url: url,
            closable: true,
        };
        this.setState({ tabs: [...this.state.tabs, newTab] }, () => {
            this.selectTab(tabId);
        });
    };

    closeTab = (tabId: string) => {
        const { tabs, activeTabId } = this.state;
        if (tabs.length <= 1) return;

        const index = tabs.findIndex(t => t.id === tabId);
        if (index === -1) return;

        const nextTabs = tabs.filter(t => t.id !== tabId);
        let nextActiveId = activeTabId;

        if (activeTabId === tabId) {
            const nextActiveTab = nextTabs[index - 1] || nextTabs[index] || nextTabs[0];
            nextActiveId = nextActiveTab ? nextActiveTab.id : 'terminal';
        }

        this.setState({ tabs: nextTabs }, () => {
            this.selectTab(nextActiveId);
        });
    };

    updateBrowserUrl = (tabId: string, url: string) => {
        this.setState(prev => ({
            tabs: prev.tabs.map(t => {
                if (t.id === tabId) {
                    return { ...t, url };
                }
                return t;
            }),
        }));
    };

    renderBuiltinBrowser = (tab: Tab) => {
        return (
            <BuiltinBrowser
                tab={tab}
                active={this.state.activeTabId === tab.id}
                onUrlChange={this.updateBrowserUrl}
                language={this.state.language}
            />
        );
    };

    // Coze click shortcut toggle dynamic drawer logic
    toggleDrawerTab = (tab: RightDrawerTab) => {
        if (this.state.activeDrawerTab === tab) {
            // Collapse the drawer
            this.setState({ activeDrawerTab: 'none', activeModulePath: '' });
        } else {
            // Expand drawer with smart width: wider for channels, git, and files panels
            const smartWidth =
                tab === 'channels' || tab === 'providers' || tab === 'git' || tab === 'files'
                    ? Math.max(this.state.rightPanelWidth, 450)
                    : 320;

            // Module-backed tabs get their entry path; non-module tabs clear it.
            const mod = getModuleByTab(tab);
            const newModulePath = mod ? mod.entryPath : '';
            this.setState(
                { activeDrawerTab: tab, rightPanelWidth: smartWidth, activeModulePath: newModulePath },
                () => {
                    if (tab === 'channels') {
                        this.loadCcConnectUrl();
                    } else if (tab === 'providers') {
                        this.loadCcProvidersUrl();
                    } else if (mod) {
                        this.loadModuleManifest(mod);
                    }
                }
            );
        }
        this.triggerTerminalFit();
    };

    /**
     * Module iframe reported a route change (NAV_CHANGE). Mirror it into host
     * state and the main app URL. We use `replaceState` rather than `pushState`
     * to avoid polluting the back/forward history when the user clicks around
     * inside the iframe.
     */
    handleModuleMessage = (e: MessageEvent) => {
        if (!isModuleInboundMessage(e.data)) return;

        if (e.data.type === 'NAV_CHANGE') {
            const path = e.data.path || '';
            if (path === this.state.activeModulePath) return;
            this.setState({ activeModulePath: path });
            this.syncModuleUrl(path);
        } else if (e.data.type === 'READY') {
            // Module announces it's mounted — re-fetch live manifest counts now
            // and immediately push the current theme/lang/nav.
            const iframe = this.findActiveModuleIframe();
            if (!iframe) return;
            postToModule(iframe, { type: 'THEME_CHANGE', theme: this.state.theme });
            postToModule(iframe, { type: 'LANG_CHANGE', lang: this.state.language });
            if (this.state.activeModulePath) {
                postToModule(iframe, { type: 'NAVIGATE', to: this.state.activeModulePath });
            }
            const mod = getModuleByTab(this.state.activeDrawerTab);
            if (mod) this.loadModuleManifest(mod);
        }
    };

    /** Locates the iframe element for the active module-backed drawer tab. */
    findActiveModuleIframe = (): HTMLIFrameElement | null => {
        const id = `${this.state.activeDrawerTab}-iframe`;
        return document.getElementById(id) as HTMLIFrameElement | null;
    };

    /**
     * Pushes a NAVIGATE message to the active module iframe and updates host
     * state. Called by `<ModuleNav />` when the user clicks a manifest link.
     */
    navigateInModule = (to: string) => {
        if (!to) return;
        if (to === this.state.activeModulePath) return;
        const iframe = this.findActiveModuleIframe();
        this.setState({ activeModulePath: to });
        this.syncModuleUrl(to);
        if (iframe) {
            postToModule(iframe, { type: 'NAVIGATE', to });
        }
    };

    /**
     * Mirrors the active module path into the main app URL as
     * `/m/<moduleId>/<subPath>`. Uses `replaceState` so the iframe's
     * internal back/forward doesn't get clobbered.
     */
    syncModuleUrl = (subPath: string) => {
        const mod = getModuleByTab(this.state.activeDrawerTab);
        if (!mod) return;
        const url = new URL(window.location.href);
        const cleanPath = subPath.startsWith('/') ? subPath : '/' + subPath;
        url.search = '';
        url.hash = `/m/${mod.moduleId}${cleanPath}`;
        try {
            window.history.replaceState({}, '', url.toString());
        } catch {
            /* ignore */
        }
    };

    /**
     * Fetches the live manifest for a module and merges it over the static
     * one. Failures are silent — the static manifest keeps the sidebar
     * functional even when the module is offline.
     */
    loadModuleManifest = async (mod: ModuleRegistration) => {
        if (!mod.manifestUrl) return;
        try {
            const res = await fetch(mod.manifestUrl, { credentials: 'same-origin' });
            if (!res.ok) return;
            const live = (await res.json()) as ModuleManifest;
            this.setState(prev => ({
                moduleManifests: {
                    ...prev.moduleManifests,
                    [mod.moduleId]: mergeManifests(mod.staticManifest, live),
                },
            }));
        } catch {
            /* static manifest is the fallback — nothing to do */
        }
    };

    /**
     * Returns the module nav data to pass to `LeftSidebar`, or undefined if
     * the active drawer tab isn't module-backed. The live manifest is used
     * when available; the static manifest is the fallback.
     */
    buildModuleNav(): { manifest: ModuleManifest; activePath: string; onNavigate: (to: string) => void } | undefined {
        const mod = getModuleByTab(this.state.activeDrawerTab);
        if (!mod) return undefined;
        const live = this.state.moduleManifests[mod.moduleId];
        const manifest = live ?? mod.staticManifest;
        return {
            manifest,
            activePath: this.state.activeModulePath || mod.entryPath,
            onNavigate: this.navigateInModule,
        };
    }

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

    // ── Flat file crawler & search ──────────────────────────────────────────

    loadFlatFiles = async () => {
        const { searchQuery, selectedFilterTag } = this.state;
        const isSearching = searchQuery !== '' || selectedFilterTag !== 'all';
        if (!isSearching) {
            this.setState({ flatFiles: [], flatFilesLoading: false });
            return;
        }

        this._crawlCounter++;
        const currentCrawl = this._crawlCounter;
        this.setState({ flatFilesLoading: true });
        try {
            const files = await fsService.search(searchQuery, selectedFilterTag);
            if (currentCrawl === this._crawlCounter) {
                this.setState({ flatFiles: files, flatFilesLoading: false });
            }
        } catch (err) {
            if (currentCrawl === this._crawlCounter) {
                console.error('[search] error:', err);
                this.setState({ flatFilesLoading: false });
            }
        }
    };

    handleSearchChange = (query: string) => {
        this.setState({ searchQuery: query });
        if (this._searchTimeout) {
            clearTimeout(this._searchTimeout);
            this._searchTimeout = null;
        }
        if (query === '' && this.state.selectedFilterTag === 'all') {
            this.setState({ flatFiles: [], flatFilesLoading: false });
            return;
        }
        this._searchTimeout = setTimeout(() => {
            this.loadFlatFiles();
        }, 300) as unknown as number;
    };

    handleFilterTagChange = (tag: 'all' | 'doc' | 'img' | 'code') => {
        this.setState({ selectedFilterTag: tag }, () => {
            if (this._searchTimeout) {
                clearTimeout(this._searchTimeout);
                this._searchTimeout = null;
            }
            this.loadFlatFiles();
        });
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
            await Promise.all([this.loadWorkspaces(true), this.loadTerminals()]);
            this.mergeSessionsIntoFolders(this.state.terminalWindows);
            const { workspaces, activeWorkspaceId } = this.state;
            if (!activeWorkspaceId && workspaces.length > 0) {
                await this.selectWorkspace(workspaces[0]);
            } else if (activeWorkspaceId) {
                await Promise.all([this.loadCcConnectUrl(), this.loadCcProvidersUrl()]);
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
            this.showToast(t('app.toast.tokenGenerateFailed', this.state.language, { err: String(err) }));
        }
    };

    revokeAccessToken = async () => {
        try {
            await accessService.revokeToken();
            this.showToast(t('app.toast.tokenRevoked', this.state.language));
            await this.checkAccessStatus();
        } catch (err) {
            this.showToast(t('app.toast.tokenRevokeFailed', this.state.language, { err: String(err) }));
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
        });

        // Check if this is an image file
        if (this.isImageFile(entry.name)) {
            this.setState({ isImagePreview: true, fileLoading: false });
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
            this.showToast(t('app.toast.copySuccess', this.state.language));
        } catch (_) {
            this.showToast(t('app.toast.copyFailed', this.state.language));
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
            this.showToast(t('app.toast.fileDuplicated', this.state.language));
            this.loadDir('', null);
        } catch (err) {
            this.showToast(t('app.toast.fileDuplicateFailed', this.state.language, { err: String(err) }));
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
        const newName = window.prompt(t('app.prompt.rename', this.state.language), selectedFsEntry.name);
        if (!newName || newName === selectedFsEntry.name) return;
        const dir = selectedFsEntry.path.includes('/')
            ? selectedFsEntry.path.slice(0, selectedFsEntry.path.lastIndexOf('/') + 1)
            : '';
        const newPath = `${dir}${newName}`;
        try {
            // Write content to new path
            await fsService.write(newPath, fileContent);
            this.showToast(t('app.toast.renameSuccess', this.state.language));
            this.setState({ selectedFsEntry: { ...selectedFsEntry, name: newName, path: newPath }, viewMode: 'list' });
            this.loadDir('', null);
        } catch (err) {
            this.showToast(t('app.toast.renameFailed', this.state.language, { err: String(err) }));
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
            this.showToast(t('app.toast.shareCopied', this.state.language));
        } catch (_) {
            this.showToast(t('app.toast.shareCopyFailed', this.state.language));
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

    getCcConnectIframeUrl = (url?: string) => {
        if (!url) return '';
        if (url.startsWith('/')) {
            return url;
        }
        try {
            const parsed = new URL(url);
            if (parsed.hostname === 'localhost' || parsed.hostname === '127.0.0.1') {
                parsed.hostname = window.location.hostname;
            }
            return parsed.toString();
        } catch (e) {
            return url;
        }
    };

    render() {
        const {
            workspaces,
            workspacesLoading,
            wsModalOpen,
            wsModalMode,
            wsModalName,
            wsModalPath,
            wsModalTerminalDir,
            wsModalChatChannel,
            dirPickerOpen,
            favoriteFiles,
            isEditingDetail,
            selectedFsEntry,
            fileContent,
            editedContent,
            fileLoading,
            fileSaving,
            fileSaveMsg,
            isImagePreview,
            toastMsg,
            language,
            accessGateVisible,
            accessTokenModalToken,
            sessionRenameModalOpen,
            sessionRenameTarget,
            sessionRenameName,
        } = this.state;
        // If access gate is visible, render only the gate
        if (accessGateVisible) {
            return <AccessTokenGate onAuthenticated={this.onAccessAuthenticated} language={language} />;
        }

        // If workspaces are empty and loading on initial load, show a loading spinner
        if (workspaces.length === 0 && workspacesLoading) {
            return (
                <div
                    class="app-container"
                    style="display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; background-color: var(--bg-panel);"
                >
                    <div class="fb-loading" style="display: flex; flex-direction: column; align-items: center;">
                        <div class="fb-loading-spinner" />
                        <span style="color: var(--text-main); margin-top: 12px; font-family: var(--font-sans);">
                            {t('app.loading.workspaces', language)}
                        </span>
                    </div>
                </div>
            );
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
                            <span style="color: var(--text-main); margin-top: 12px;">
                                {t('app.loading.sharePreview', language)}
                            </span>
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
                        imageUrl={fsService.imageUrl(selectedFsEntry.path)}
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
                        language={language}
                    />
                    {toastMsg && (
                        <div class="fb-toast">
                            <span>{toastMsg}</span>
                        </div>
                    )}
                </div>
            );
        }

        return (
            <div class="app-container" style="display: flex; flex-direction: column;">
                {this.state.hasLoadedWorkspaces && workspaces.length === 0 ? (
                    <WelcomeOnboarding
                        language={language}
                        onCreateWorkspace={this.openCreateWorkspacePicker}
                        onUseTempWorkspace={this.onUseTempWorkspace}
                    />
                ) : this.state.isMobile ? (
                    <MobileAppLayout app={this} state={this.state} />
                ) : (
                    <DesktopAppLayout app={this} state={this.state} />
                )}

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
                        language={language}
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
                        language={language}
                    />
                )}

                {/* Access Token Display Modal (one-time, shown after generation) */}
                {accessTokenModalToken && (
                    <AccessTokenModal
                        token={accessTokenModalToken}
                        onClose={this.closeAccessTokenModal}
                        onShowToast={this.showToast}
                        language={language}
                    />
                )}

                {/* Session Rename Modal */}
                {sessionRenameModalOpen && sessionRenameTarget && (
                    <SessionRenameModal
                        title={sessionRenameName}
                        onTitleChange={val => this.setState({ sessionRenameName: val })}
                        onClose={this.closeSessionRenameModal}
                        onSubmit={this.submitRenameSession}
                        language={language}
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
