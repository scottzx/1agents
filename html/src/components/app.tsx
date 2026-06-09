import { h, Component, Fragment } from 'preact';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

import {
    WorkspaceFolder,
    Workspace,
    FsEntry,
    RightDrawerTab,
    TmuxWindow,
    Session,
    ChatSession,
    AgentType,
    isChat,
    isTerminal,
    isFullPageTab,
} from './types';
import { LeftSidebar } from './sidebar/LeftSidebar';
import { WorkspaceHeader } from './header/WorkspaceHeader';
import { MiddleCanvas } from './canvas/MiddleCanvas';
import { RightPanel } from './drawer/RightPanel';
import { DiscoveryPanel } from './drawer/DiscoveryPanel';
import { SystemSettings } from './settings/SystemSettings';
import { FileDetailView } from './drawer/FileDetailView';
import { AccessTokenGate } from './auth/AccessTokenGate';
import { WelcomeOnboarding } from './welcome/WelcomeOnboarding';
import { WorkspaceModal, DirPickerModal, AccessTokenModal } from './modal';
import { SessionCreateModal } from './chat/SessionCreateModal';
import { workspaceService } from '../services/workspaceService';
import { terminalService } from '../services/terminalService';
import { fsService } from '../services/fsService';
import { accessService } from '../services/accessService';
import { agentService, DEFAULT_AGENT_TYPE } from '../services/agentService';
import { ccCreateSession, ccDeleteSession, getCcAuth, ccProjectName } from '../services/ccconnectClient';

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

interface Tab {
    id: string; // 'terminal', 'preview-[path]', 'browser-[timestamp]'
    title: string;
    type: 'terminal' | 'preview' | 'browser';
    path?: string;
    url?: string;
    closable: boolean;
}

interface AppState {
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
    wsModalDefaultAgent: AgentType;
    ccConnectUrl: string;
    ccProvidersUrl: string;
    // ── Chat session creation modal state ──
    chatCreateOpen: boolean;
    chatCreateWsId: string;
    // ── Directory picker modal state ──
    dirPickerOpen: boolean;
    dirPickerOnSelect: ((path: string) => void) | null;
    // ── Terminal / tmux state ──
    terminalWindows: TmuxWindow[];
    terminalWindowsLoading: boolean;
    tmuxMouseOn: boolean;
    // ── Chat session state (1agents-side index) ──
    chatSessions: ChatSession[];
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
    onboarded: boolean;
    hasLoadedWorkspaces: boolean;
}

// Drag resizer state (module-level for perf)
let _resizerActive: 'left' | 'right' | null = null;
let _resizerStartX = 0;
let _resizerStartWidth = 0;

interface BuiltinBrowserProps {
    tab: Tab;
    active: boolean;
    onUrlChange: (tabId: string, url: string) => void;
}

interface BuiltinBrowserState {
    iframeSrc: string;
}

class BuiltinBrowser extends Component<BuiltinBrowserProps, BuiltinBrowserState> {
    private inputRef: HTMLInputElement | null = null;
    private iframeRef: HTMLIFrameElement | null = null;
    private lastLoadedUrl: string = '';

    state: BuiltinBrowserState = {
        iframeSrc: this.getIframeUrl(this.props.tab.url || ''),
    };

    componentDidMount() {
        window.addEventListener('message', this.handleIframeMessage);
    }

    componentWillUnmount() {
        window.removeEventListener('message', this.handleIframeMessage);
    }

    componentWillReceiveProps(nextProps: BuiltinBrowserProps) {
        if (nextProps.tab.url !== this.props.tab.url) {
            if (nextProps.tab.url !== this.lastLoadedUrl) {
                this.setState({
                    iframeSrc: this.getIframeUrl(nextProps.tab.url || ''),
                });
            }
        }
    }

    handleIframeMessage = (e: MessageEvent) => {
        if (this.iframeRef && e.source === this.iframeRef.contentWindow) {
            const data = e.data;
            if (data && data.type === 'iframe_navigate' && typeof data.url === 'string') {
                const newUrl = data.url;
                if (newUrl && newUrl !== this.props.tab.url) {
                    this.lastLoadedUrl = newUrl;
                    this.props.onUrlChange(this.props.tab.id, newUrl);
                }
            }
        }
    };

    getOriginalUrl = (urlStr: string): string => {
        try {
            const url = new URL(urlStr);
            if (url.pathname === '/api/proxy') {
                const target = url.searchParams.get('url');
                if (target) return target;
            }
            return urlStr;
        } catch (e) {
            return urlStr;
        }
    };

    handleIframeLoad = () => {
        if (!this.iframeRef || !this.iframeRef.contentWindow) return;
        try {
            const iframeUrl = this.iframeRef.contentWindow.location.href;
            if (iframeUrl && iframeUrl !== 'about:blank') {
                const targetUrl = this.getOriginalUrl(iframeUrl);
                if (targetUrl && targetUrl !== this.props.tab.url) {
                    this.lastLoadedUrl = targetUrl;
                    this.props.onUrlChange(this.props.tab.id, targetUrl);
                }
            }
        } catch (e) {
            // Expected cross-origin error when loading non-proxied localhost/intranet sites
        }
    };

    private invokeTauri = async (command: string, args: Record<string, unknown> = {}): Promise<unknown> => {
        const tauri = (
            window as unknown as {
                __TAURI__?: { core: { invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown> } };
            }
        ).__TAURI__;
        if (tauri) {
            try {
                return await tauri.core.invoke(command, args);
            } catch (e) {
                console.error(`Failed to invoke Tauri command ${command}:`, e);
            }
        }
        return null;
    };

    isLocalUrl(urlStr: string): boolean {
        try {
            const url = new URL(urlStr);
            const hostname = url.hostname.toLowerCase();
            return (
                hostname === 'localhost' ||
                hostname === '127.0.0.1' ||
                hostname === '::1' ||
                hostname.startsWith('192.168.') ||
                hostname.startsWith('10.') ||
                hostname.startsWith('172.')
            );
        } catch (e) {
            const lower = urlStr.toLowerCase();
            return lower.includes('localhost') || lower.includes('127.0.0.1') || lower.includes('::1');
        }
    }

    getIframeUrl(urlStr: string): string {
        if (!urlStr || urlStr === 'about:blank') {
            return 'about:blank';
        }
        if (this.isLocalUrl(urlStr)) {
            return urlStr;
        }
        return `${window.location.origin}/api/proxy?url=${encodeURIComponent(urlStr)}`;
    }

    handleKeyPress = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && this.inputRef) {
            let url = this.inputRef.value.trim();
            if (url) {
                if (!/^https?:\/\//i.test(url) && !url.startsWith('about:')) {
                    url = 'http://' + url;
                }
                this.lastLoadedUrl = '';
                this.props.onUrlChange(this.props.tab.id, url);
            }
        }
    };

    handleRefresh = () => {
        if (this.iframeRef && this.iframeRef.contentWindow) {
            try {
                this.iframeRef.contentWindow.location.reload();
            } catch (e) {
                this.iframeRef.src = this.state.iframeSrc;
            }
        }
    };

    handleOpenExternal = () => {
        const { tab } = this.props;
        if (!tab.url || tab.url === 'about:blank') return;

        const isDesktopEnv =
            IS_DESKTOP || (typeof window !== 'undefined' && !!(window as unknown as { __TAURI__?: object }).__TAURI__);
        if (isDesktopEnv) {
            this.invokeTauri('open_in_external_browser', { url: tab.url });
        } else {
            window.open(tab.url, '_blank');
        }
    };

    render() {
        const { tab, active } = this.props;
        const isHome = !tab.url || tab.url === 'about:blank';

        return (
            <div
                class="builtin-browser"
                style={{ display: active ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}
            >
                <div class="browser-nav-bar">
                    <button class="browser-refresh-btn" onClick={this.handleRefresh} title="刷新页面" disabled={isHome}>
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.72 2.78L21 8" />
                            <polyline points="21 3 21 8 16 8" />
                        </svg>
                    </button>
                    <input
                        type="text"
                        class="browser-url-input"
                        placeholder="输入网址并回车 (e.g. www.bing.com 或 localhost:3000)"
                        value={tab.url === 'about:blank' ? '' : tab.url}
                        ref={el => {
                            this.inputRef = el;
                        }}
                        onKeyDown={this.handleKeyPress}
                    />
                    <button
                        class="browser-open-external-btn"
                        onClick={this.handleOpenExternal}
                        title="在本地浏览器中打开"
                        disabled={isHome}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                            <polyline points="15 3 21 3 21 9" />
                            <line x1="10" y1="14" x2="21" y2="3" />
                        </svg>
                    </button>
                </div>
                <div class="browser-iframe-wrapper" style="flex: 1; position: relative; width: 100%; height: 100%;">
                    {isHome && (
                        <div
                            class="browser-welcome-page"
                            style="position: absolute; top: 0; left: 0; width: 100%; height: 100%; z-index: 1;"
                        >
                            <div class="welcome-card">
                                <svg
                                    class="welcome-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="1.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="10" />
                                    <line x1="2" y1="12" x2="22" y2="12" />
                                    <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                                </svg>
                                <h3 class="welcome-title">内置浏览器</h3>
                                <p class="welcome-desc">在上方地址栏输入网址并按回车键进行浏览。</p>
                                <div class="welcome-tips">
                                    <div class="tip-item">
                                        <strong>💡 提示：</strong>
                                        <span>
                                            该浏览器基于 iframe 渲染，对于公网网页，自动使用 Go
                                            后端进行代理以解决跨域与安全策略拦截；本地服务直连加载。
                                        </span>
                                    </div>
                                    <div class="tip-item">
                                        <strong>🌐 外部打开：</strong>
                                        <span>
                                            若页面遇到复杂的 JS
                                            渲染问题或白屏，可点击输入框右侧的按钮，直接使用系统默认浏览器打开该网页。
                                        </span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    )}
                    {!isHome && (
                        <iframe
                            ref={el => {
                                this.iframeRef = el;
                            }}
                            src={this.state.iframeSrc}
                            class="browser-iframe"
                            style="width: 100%; height: 100%; border: none; background: #fff;"
                            onLoad={this.handleIframeLoad}
                        />
                    )}
                </div>
            </div>
        );
    }
}

export class App extends Component<{}, AppState> {
    private _tunnelHeartbeat: ReturnType<typeof setInterval> | null = null;
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
            wsModalDefaultAgent: DEFAULT_AGENT_TYPE,
            ccConnectUrl: '',
            ccProvidersUrl: '',
            chatCreateOpen: false,
            chatCreateWsId: '',
            dirPickerOpen: false,
            dirPickerOnSelect: null,
            terminalWindows: [],
            terminalWindowsLoading: false,
            tmuxMouseOn: true,
            chatSessions: [],
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
            onboarded: localStorage.getItem('1agents-onboarded') === 'true',
            hasLoadedWorkspaces: false,
            tabs: [{ id: 'terminal', title: '工作台', type: 'terminal', closable: false }],
            activeTabId: 'terminal',
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

        // Synchronize terminal windows + cached chat sessions into folders
        this.mergeSessionsIntoFolders(this.state.terminalWindows, this.state.chatSessions);

        // If we already have an active workspace, also refresh its chat sessions.
        if (this.state.activeWorkspaceId) {
            this.loadChatSessions(this.state.activeWorkspaceId);
        }

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
            this.showToast(`创建 temp 空间失败: ${err}`);
        }
    };

    /** Create a new workspace via POST /api/workspace/create */
    createWorkspace = async (
        name: string,
        path: string,
        terminalDir?: string,
        chatChannel?: string,
        defaultAgent?: AgentType
    ) => {
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
            defaultAgent: defaultAgent || DEFAULT_AGENT_TYPE,
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
        if (this.state.workspaces.length <= 1) {
            this.showToast('无法删除，系统需保留至少一个工作空间');
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
                wsModalDefaultAgent: DEFAULT_AGENT_TYPE,
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
            wsModalDefaultAgent: ws.defaultAgent || DEFAULT_AGENT_TYPE,
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
            wsModalDefaultAgent: DEFAULT_AGENT_TYPE,
        });
    };

    submitWsModal = async () => {
        const {
            wsModalMode,
            wsModalTarget,
            wsModalName,
            wsModalPath,
            wsModalTerminalDir,
            wsModalChatChannel,
            wsModalDefaultAgent,
        } = this.state;
        if (!wsModalName.trim()) return;
        this.closeWsModal();
        if (wsModalMode === 'create') {
            await this.createWorkspace(
                wsModalName.trim(),
                wsModalPath.trim(),
                wsModalTerminalDir.trim(),
                wsModalChatChannel.trim(),
                wsModalDefaultAgent
            );
        } else if (wsModalMode === 'rename' && wsModalTarget) {
            await this.updateWorkspace({
                ...wsModalTarget,
                name: wsModalName.trim(),
                path: wsModalPath.trim(),
                terminalDir: wsModalTerminalDir.trim() || undefined,
                chatChannel: wsModalChatChannel.trim() || undefined,
                defaultAgent: wsModalDefaultAgent,
            });
        }
    };

    // ── Terminal (tmux) API helpers ────────────────────────────────────────────

    /** Fetch all tmux windows from GET /api/terminal/list and sync to folders */
    loadTerminals = async () => {
        this.setState({ terminalWindowsLoading: true });
        try {
            const windows = await terminalService.list();
            // Use whatever chat sessions we have cached; the chat loader
            // (loadChatSessions) will refresh them in parallel.
            this.mergeSessionsIntoFolders(windows, this.state.chatSessions);
            this.setState({ terminalWindows: windows, terminalWindowsLoading: false });
        } catch (err) {
            console.error('[terminal] list error:', err);
            this.setState({ terminalWindowsLoading: false });
        }
    };

    /** Fetch chat session index for the active workspace from /api/agent/sessions */
    loadChatSessions = async (workspaceId?: string) => {
        const wsId = workspaceId ?? this.state.activeWorkspaceId;
        if (!wsId) return;
        try {
            const chats = await agentService.list(wsId);
            this.setState({ chatSessions: chats });
            this.mergeSessionsIntoFolders(this.state.terminalWindows, chats);
        } catch (err) {
            console.error('[agent] list error:', err);
        }
    };

    /**
     * Create a new chat session.
     *
     * Flow:
     *   1. Pick cc-connect project name from workspace + agent type
     *   2. Generate a 1agents-side id + session_key
     *   3. POST cc-connect to create the actual session
     *   4. POST 1agents to index the mapping
     *   5. Refresh local state + select the new session
     */
    createChatSession = async (workspaceId: string, name: string, agentType: AgentType) => {
        const ws = this.state.workspaces.find(w => w.id === workspaceId);
        if (!ws) {
            this.showToast('工作空间不存在');
            return;
        }
        try {
            this.showToast('正在创建聊天会话…');
            const project = ccProjectName(ws.name || ws.id, agentType);
            const { token } = await getCcAuth(workspaceId);
            const sessionKey = `oneagents:${ws.id}:${agentType}:${Date.now()}:${Math.random().toString(36).slice(2, 8)}`;
            const cc = await ccCreateSession(project, { session_key: sessionKey, name: name || undefined }, token);
            const indexed = await agentService.index({
                workspace_id: workspaceId,
                name: name || `${agentType} 会话`,
                agent_type: agentType,
                cc_project: project,
                cc_session_id: cc.id,
                session_key: sessionKey,
            });
            await this.loadChatSessions(workspaceId);
            // Auto-select the new session and switch to the agents tab.
            this.setState({
                activeSession: { ...indexed, active: true },
                activeTab: 'agents',
            });
            this.showToast('聊天会话已创建 ✓');
        } catch (err) {
            this.showToast(`创建聊天失败: ${(err as Error).message}`);
        }
    };

    /** Open the chat-create modal for a given workspace. */
    openChatCreate = (workspaceId: string) => {
        this.setState({ chatCreateOpen: true, chatCreateWsId: workspaceId });
    };
    closeChatCreate = () => this.setState({ chatCreateOpen: false, chatCreateWsId: '' });

    /** Kill a chat session: delete from cc-connect, then unindex from 1agents. */
    killChatSession = async (sessionId: string) => {
        const session = this.state.chatSessions.find(c => c.id === sessionId);
        if (!session) return;
        try {
            try {
                const { token } = await getCcAuth(session.workspaceId);
                await ccDeleteSession(session.ccProject, session.ccSessionId, session.sessionKey, token);
            } catch (err) {
                // Log but don't block — the user may want to clean up a
                // dangling index even when cc-connect side is already gone.
                console.warn('[agent] cc-connect delete failed:', err);
            }
            await agentService.delete(sessionId);
            await this.loadChatSessions(session.workspaceId);
            if (
                this.state.activeSession &&
                isChat(this.state.activeSession) &&
                this.state.activeSession.id === sessionId
            ) {
                this.setState({ activeSession: null, activeTab: 'terminal' });
            }
            this.showToast('聊天会话已关闭 ✓');
        } catch (err) {
            this.showToast(`关闭失败: ${(err as Error).message}`);
        }
    };

    /** Sync tmux windows + chat sessions into workspace folders as sessions */
    mergeSessionsIntoFolders(windows: TmuxWindow[], chats: ChatSession[]) {
        this.setState(prev => ({
            folders: prev.folders.map(f => {
                const termSessions: Session[] = windows
                    .filter(w => w.workspaceId === f.id)
                    .map(w => ({
                        kind: 'terminal',
                        id: w.name,
                        workspaceId: w.workspaceId,
                        index: w.index,
                        name: `会话 #${w.index}`,
                        active: w.active,
                        cwd: w.cwd,
                    }));
                const chatSessions: Session[] = chats.filter(c => c.workspaceId === f.id).map(c => ({ ...c }));
                // Chat sessions first (newer), then terminals.
                return { ...f, sessions: [...chatSessions, ...termSessions] };
            }),
        }));
        // Preserve the currently-active chat session if it still exists; otherwise
        // fall back to the most recently active terminal window.
        const prevActive = this.state.activeSession;
        const activeChat = prevActive && isChat(prevActive) ? chats.find(c => c.id === prevActive.id) : null;
        const activeWin = windows.find(w => w.active);
        const activeSession: Session | null = activeChat
            ? { ...activeChat, active: true }
            : activeWin
              ? {
                    kind: 'terminal',
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

        // 1. Optimistic UI update: mark the session active and switch tab.
        this.setState(prev => {
            const updatedFolders = prev.folders.map(f => ({
                ...f,
                sessions: f.sessions.map(s => {
                    if (isChat(s) && isChat(session)) return { ...s, active: s.id === session.id };
                    if (isTerminal(s) && isTerminal(session)) return { ...s, active: s.index === session.index };
                    return { ...s, active: false };
                }),
            }));
            localStorage.setItem('1agents-active-workspace', session.workspaceId);
            return {
                activeSession: { ...session, active: true },
                // Chat sessions live in the agents tab; terminals in the terminal tab.
                activeTab: isChat(session) ? 'agents' : 'terminal',
                folders:
                    session.workspaceId !== oldWorkspaceId
                        ? updatedFolders.map(f => (f.id === session.workspaceId ? { ...f, expanded: true } : f))
                        : updatedFolders,
                activeWorkspaceId: session.workspaceId,
            };
        });

        // Chat sessions don't need tmux / fs / git context switching; just
        // ensure the workspace is loaded and we're done.
        if (isChat(session)) {
            if (session.workspaceId !== oldWorkspaceId) {
                const ws = workspaces.find(w => w.id === session.workspaceId);
                if (ws) await this.switchWorkspaceContext(ws);
            }
            this.loadChatSessions(session.workspaceId);
            if (this.state.isMobile) this.setState({ leftSidebarOpen: false });
            return;
        }

        // Helper to perform the actual terminal window and workspace context switching
        const performSwitch = async () => {
            // Always switch the tmux window first
            await this.switchTerminal((session as Extract<Session, { kind: 'terminal' }>).index);

            if (session.workspaceId !== oldWorkspaceId) {
                this.loadCcConnectUrl(session.workspaceId);
                this.loadCcProvidersUrl(session.workspaceId);
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
        if (isFullPageTab(this.state.activeDrawerTab)) {
            this.setState({ activeDrawerTab: 'none' });
        }
        const { activeWorkspaceId, terminalWindows } = this.state;
        if (ws.id === activeWorkspaceId) return;

        this.setState({ activeWorkspaceId: ws.id }, () => {
            this.loadCcConnectUrl(ws.id);
            this.loadCcProvidersUrl(ws.id);
            this.loadChatSessions(ws.id);
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

    toggleLanguage = (lang: 'zh-CN' | 'en-US') => {
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

    selectTab = async (tabId: string) => {
        const tab = this.state.tabs.find(t => t.id === tabId);
        if (!tab) return;

        this.setState({ activeTabId: tabId });

        if (tab.type === 'preview' && tab.path) {
            const entry: FsEntry = {
                name: tab.title.replace('预览: ', ''),
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
                title: `预览: ${fileName}`,
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
            title: '内置浏览器',
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
            <BuiltinBrowser tab={tab} active={this.state.activeTabId === tab.id} onUrlChange={this.updateBrowserUrl} />
        );
    };

    // Coze click shortcut toggle dynamic drawer logic
    toggleDrawerTab = (tab: RightDrawerTab) => {
        if (this.state.activeDrawerTab === tab) {
            // Collapse the drawer
            this.setState({ activeDrawerTab: 'none' });
        } else {
            // Expand drawer with smart width: wider for channels, git, and files panels
            const smartWidth =
                tab === 'channels' || tab === 'providers' || tab === 'git' || tab === 'files'
                    ? Math.max(this.state.rightPanelWidth, 450)
                    : 320;
            this.setState({ activeDrawerTab: tab, rightPanelWidth: smartWidth }, () => {
                if (tab === 'channels') {
                    this.loadCcConnectUrl();
                } else if (tab === 'providers') {
                    this.loadCcProvidersUrl();
                }
            });
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
            this.mergeSessionsIntoFolders(this.state.terminalWindows, this.state.chatSessions);
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
            this.loadDir('', null);
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
            this.loadDir('', null);
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
            wsModalDefaultAgent,
            ccConnectUrl,
            ccProvidersUrl,
            chatCreateOpen,
            chatCreateWsId,
            dirPickerOpen,
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
            tabs,
            activeTabId,
            chatSessions,
        } = this.state;

        const activeTabObj = tabs.find(t => t.id === activeTabId);

        // If access gate is visible, render only the gate
        if (accessGateVisible) {
            return <AccessTokenGate onAuthenticated={this.onAccessAuthenticated} />;
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
                            {language === 'zh-CN' ? '正在载入工作空间…' : 'Loading workspaces…'}
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
            <div class="app-container" style="display: flex; flex-direction: column;">
                {this.state.hasLoadedWorkspaces && workspaces.length === 0 ? (
                    <WelcomeOnboarding
                        language={language}
                        onCreateWorkspace={this.openCreateWorkspacePicker}
                        onUseTempWorkspace={this.onUseTempWorkspace}
                    />
                ) : (
                    <Fragment>
                        {IS_DESKTOP && (
                            <div class="workspace-tabs-bar">
                                <div class="workspace-tabs-list">
                                    {tabs.map(tab => {
                                        const isActive = tab.id === activeTabId;
                                        return (
                                            <div
                                                key={tab.id}
                                                class={`workspace-tab-item ${isActive ? 'active' : ''}`}
                                                onClick={() => this.selectTab(tab.id)}
                                            >
                                                <span class="tab-title">{tab.title}</span>
                                                {tab.closable && (
                                                    <span
                                                        class="workspace-tab-close"
                                                        onClick={(e: MouseEvent) => {
                                                            e.stopPropagation();
                                                            this.closeTab(tab.id);
                                                        }}
                                                        title="关闭标签页"
                                                    >
                                                        <svg
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2.5"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <line x1="18" y1="6" x2="6" y2="18" />
                                                            <line x1="6" y1="6" x2="18" y2="18" />
                                                        </svg>
                                                    </span>
                                                )}
                                            </div>
                                        );
                                    })}
                                </div>
                                <button
                                    class="workspace-tab-add-btn"
                                    onClick={() => this.openBrowserTab('')}
                                    title="打开新浏览器标签页"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <line x1="12" y1="5" x2="12" y2="19" />
                                        <line x1="5" y1="12" x2="19" y2="12" />
                                    </svg>
                                </button>
                            </div>
                        )}

                        <div
                            class="app-main-layout"
                            style="display: flex; flex: 1; flex-direction: row; overflow: hidden; width: 100%;"
                        >
                            {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar */}
                            {activeTabId === 'terminal' && (
                                <Fragment>
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
                                        activeDrawerTab={activeDrawerTab}
                                        onCreateWorkspace={this.openCreateWorkspacePicker}
                                        onRenameWorkspace={ws => this.openRenameWorkspaceModal(ws)}
                                        onDeleteWorkspace={this.deleteWorkspace}
                                        onSelectWorkspace={ws => this.selectWorkspace(ws)}
                                        onSelectSession={s => this.selectSession(s)}
                                        onTerminalCreate={(wsId, cwd) => this.createTerminal(wsId, cwd)}
                                        onTerminalKill={idx => this.killTerminal(idx)}
                                        onChatCreate={wsId => this.openChatCreate(wsId)}
                                        onChatKill={id => this.killChatSession(id)}
                                    />

                                    {/* Resizer: between LEFT sidebar and MIDDLE canvas */}
                                    {leftSidebarOpen && (
                                        <div
                                            class="resizer resizer-left"
                                            onMouseDown={(e: MouseEvent) => this.handleResizerDown('left', e)}
                                            title="拖动调整左侧栏宽度"
                                        />
                                    )}
                                </Fragment>
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
                                {activeTabId === 'terminal' ? (
                                    <Fragment>
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
                                            hasChatSession={chatSessions.some(c => c.workspaceId === activeWorkspaceId)}
                                        />

                                        {/* [WORKSPACE BODY CONTAINER]: terminal & drawers */}
                                        <div
                                            class={`workspace-body-container ${activeDrawerTab !== 'none' && !isFullPageTab(activeDrawerTab) ? 'drawer-open' : ''}`}
                                        >
                                            {isFullPageTab(activeDrawerTab) ? (
                                                <div
                                                    style={{
                                                        flex: 1,
                                                        display: 'flex',
                                                        flexDirection: 'column',
                                                        height: '100%',
                                                        width: '100%',
                                                        overflow: 'hidden',
                                                    }}
                                                >
                                                    {activeDrawerTab === 'providers' && ccProvidersUrl && (
                                                        <iframe
                                                            id="cc-providers-iframe"
                                                            src={this.getCcConnectIframeUrl(ccProvidersUrl)}
                                                            onLoad={e => {
                                                                const iframe = e.target as HTMLIFrameElement;
                                                                if (iframe && iframe.contentWindow) {
                                                                    iframe.contentWindow.postMessage(
                                                                        { type: 'THEME_CHANGE', theme },
                                                                        '*'
                                                                    );
                                                                    iframe.contentWindow.postMessage(
                                                                        { type: 'LANG_CHANGE', lang: language },
                                                                        '*'
                                                                    );
                                                                }
                                                            }}
                                                            style={{
                                                                width: '100%',
                                                                height: '100%',
                                                                border: 'none',
                                                                background: 'transparent',
                                                            }}
                                                        />
                                                    )}
                                                    {activeDrawerTab === 'skills' && (
                                                        <iframe
                                                            id="skills-iframe"
                                                            src="/1skills/"
                                                            onLoad={e => {
                                                                const iframe = e.target as HTMLIFrameElement;
                                                                if (iframe && iframe.contentWindow) {
                                                                    iframe.contentWindow.postMessage(
                                                                        { type: 'THEME_CHANGE', theme },
                                                                        '*'
                                                                    );
                                                                    iframe.contentWindow.postMessage(
                                                                        { type: 'LANG_CHANGE', lang: language },
                                                                        '*'
                                                                    );
                                                                }
                                                            }}
                                                            style={{
                                                                width: '100%',
                                                                height: '100%',
                                                                border: 'none',
                                                                background: 'transparent',
                                                            }}
                                                        />
                                                    )}
                                                    {activeDrawerTab === 'discovery' && (
                                                        <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
                                                            <DiscoveryPanel
                                                                onOpenBrowserTab={
                                                                    IS_DESKTOP ? this.openBrowserTab : undefined
                                                                }
                                                            />
                                                        </div>
                                                    )}
                                                    {activeDrawerTab === 'settings' && (
                                                        <div
                                                            style={{
                                                                flex: 1,
                                                                overflow: 'hidden',
                                                                display: 'flex',
                                                                flexDirection: 'column',
                                                                height: '100%',
                                                            }}
                                                        >
                                                            <SystemSettings
                                                                theme={theme}
                                                                toggleTheme={this.toggleTheme}
                                                                language={language}
                                                                toggleLanguage={this.toggleLanguage}
                                                                tmuxMouseOn={tmuxMouseOn}
                                                                onTmuxMouseToggle={this.toggleTmuxMouse}
                                                                accessTokenExists={accessAuthRequired}
                                                                onGenerateAccessToken={this.generateAccessToken}
                                                                onRevokeAccessToken={this.revokeAccessToken}
                                                            />
                                                        </div>
                                                    )}
                                                </div>
                                            ) : (
                                                <Fragment>
                                                    {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                                                    <MiddleCanvas
                                                        activeTab={
                                                            activeTab as 'terminal' | 'agents' | 'console' | 'folders'
                                                        }
                                                        wsUrl={wsUrl}
                                                        tokenUrl={tokenUrl}
                                                        clientOptions={clientOptions}
                                                        termOptions={termOptions}
                                                        flowControl={flowControl}
                                                        onMobileDetect={isMobile => this.setState({ isMobile })}
                                                        onKeyboardStateChange={this.handleKeyboardStateChange}
                                                        tmuxMouseOn={tmuxMouseOn}
                                                        onTmuxMouseToggle={this.toggleTmuxMouse}
                                                        activeChatSession={
                                                            activeSession && isChat(activeSession)
                                                                ? activeSession
                                                                : null
                                                        }
                                                    />

                                                    {/* Resizer: between MIDDLE canvas and RIGHT panel */}
                                                    {activeDrawerTab !== 'none' && (
                                                        <div
                                                            class="resizer resizer-right"
                                                            onMouseDown={(e: MouseEvent) =>
                                                                this.handleResizerDown('right', e)
                                                            }
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
                                                        onSearchQueryChange={this.handleSearchChange}
                                                        onFilterTagChange={this.handleFilterTagChange}
                                                        onRefreshFlatFiles={async () => {
                                                            this.loadDir('', null);
                                                            const isSearching =
                                                                searchQuery !== '' || selectedFilterTag !== 'all';
                                                            if (isSearching) {
                                                                this.loadFlatFiles();
                                                            }
                                                            try {
                                                                await this.checkAccessStatus();
                                                                await Promise.all([
                                                                    this.loadWorkspaces(true),
                                                                    this.loadTerminals(),
                                                                ]);

                                                                const { workspaces, activeWorkspaceId } = this.state;
                                                                if (!activeWorkspaceId && workspaces.length > 0) {
                                                                    await this.selectWorkspace(workspaces[0]);
                                                                } else if (activeWorkspaceId) {
                                                                    await Promise.all([
                                                                        this.loadCcConnectUrl(),
                                                                        this.loadCcProvidersUrl(),
                                                                    ]);
                                                                }
                                                            } catch (e) {
                                                                console.error('Failed to reconnect/refresh:', e);
                                                            }
                                                        }}
                                                        onOpenFileDetail={this.openFileDetail}
                                                        onBackToList={() =>
                                                            this.setState({ viewMode: 'list', detailFullscreen: false })
                                                        }
                                                        onToggleFavorite={this.toggleFavorite}
                                                        onCopyContent={this.copyFileContent}
                                                        onDownloadFile={this.downloadFile}
                                                        onRenameFile={this.renameFile}
                                                        onToggleFullscreen={() => {
                                                            const { selectedFsEntry, workspaces, activeWorkspaceId } =
                                                                this.state;
                                                            if (selectedFsEntry) {
                                                                const activeWorkspace = workspaces.find(
                                                                    w => w.id === activeWorkspaceId
                                                                );
                                                                const activeWorkspacePath =
                                                                    activeWorkspace?.path || '.';
                                                                const absolutePath = selectedFsEntry.path.startsWith(
                                                                    '/'
                                                                )
                                                                    ? selectedFsEntry.path
                                                                    : `${activeWorkspacePath}/${selectedFsEntry.path}`;
                                                                if (IS_DESKTOP) {
                                                                    this.openPreviewTab(
                                                                        absolutePath,
                                                                        selectedFsEntry.name
                                                                    );
                                                                } else {
                                                                    const shareUrl = `${window.location.origin}${
                                                                        window.location.pathname
                                                                    }?preview=${encodeURIComponent(absolutePath)}`;
                                                                    window.open(shareUrl, '_blank');
                                                                }
                                                            }
                                                        }}
                                                        onShareFile={this.shareFile}
                                                        onSaveFile={this.saveFile}
                                                        onToggleEditing={isEditing =>
                                                            this.setState({ isEditingDetail: isEditing })
                                                        }
                                                        onEditedContentChange={content =>
                                                            this.setState({ editedContent: content })
                                                        }
                                                        onOpenPreview={
                                                            IS_DESKTOP
                                                                ? (path, name) => this.openPreviewTab(path, name)
                                                                : undefined
                                                        }
                                                        fsEntries={this.state.fsEntries}
                                                        fsLoading={this.state.fsLoading}
                                                        onToggleFsDir={this.toggleFsDir}
                                                        accessTokenExists={accessAuthRequired}
                                                        onGenerateAccessToken={this.generateAccessToken}
                                                        onRevokeAccessToken={this.revokeAccessToken}
                                                    />
                                                </Fragment>
                                            )}
                                        </div>
                                    </Fragment>
                                ) : (
                                    <div class="workspace-body-container dynamic-tab-view">
                                        {activeTabObj?.type === 'preview' && (
                                            <div
                                                class="fb-detail-view-tab-container"
                                                style="flex: 1; height: 100%; display: flex; flex-direction: column; overflow: hidden; background-color: var(--bg-panel); padding: 12px 16px;"
                                            >
                                                {selectedFsEntry ? (
                                                    <FileDetailView
                                                        selectedFsEntry={selectedFsEntry}
                                                        favoriteFiles={favoriteFiles}
                                                        detailFullscreen={false}
                                                        isEditingDetail={isEditingDetail}
                                                        fileContent={fileContent}
                                                        editedContent={editedContent}
                                                        fileLoading={fileLoading}
                                                        fileSaving={fileSaving}
                                                        fileSaveMsg={fileSaveMsg}
                                                        isImagePreview={isImagePreview}
                                                        imageDataUrl={imageDataUrl}
                                                        onBackToList={() => this.closeTab(activeTabId)}
                                                        onToggleFavorite={this.toggleFavorite}
                                                        onCopyContent={this.copyFileContent}
                                                        onDownloadFile={this.downloadFile}
                                                        onRenameFile={this.renameFile}
                                                        onToggleFullscreen={() => {}}
                                                        onShareFile={this.shareFile}
                                                        onSaveFile={this.saveFile}
                                                        onToggleEditing={isEditing =>
                                                            this.setState({ isEditingDetail: isEditing })
                                                        }
                                                        onEditedContentChange={content =>
                                                            this.setState({ editedContent: content })
                                                        }
                                                        onOpenPreview={
                                                            IS_DESKTOP
                                                                ? (path, name) => this.openPreviewTab(path, name)
                                                                : undefined
                                                        }
                                                        isStandalone={true}
                                                    />
                                                ) : (
                                                    <div class="fb-loading">
                                                        <div class="fb-loading-spinner" />
                                                        <span>正在载入预览…</span>
                                                    </div>
                                                )}
                                            </div>
                                        )}
                                        <div
                                            class="builtin-browser-container"
                                            style={{
                                                flex: 1,
                                                height: '100%',
                                                display: activeTabObj?.type === 'browser' ? 'flex' : 'none',
                                                flexDirection: 'column',
                                                overflow: 'hidden',
                                            }}
                                        >
                                            {this.state.tabs
                                                .filter(t => t.type === 'browser')
                                                .map(t => this.renderBuiltinBrowser(t))}
                                        </div>
                                    </div>
                                )}
                            </div>
                        </div>
                    </Fragment>
                )}

                {/* Workspace create/rename modal */}
                {wsModalOpen && (
                    <WorkspaceModal
                        mode={wsModalMode}
                        name={wsModalName}
                        path={wsModalPath}
                        terminalDir={wsModalTerminalDir}
                        chatChannel={wsModalChatChannel}
                        defaultAgent={wsModalDefaultAgent}
                        onNameChange={val => this.setState({ wsModalName: val })}
                        onPathChange={val => this.setState({ wsModalPath: val })}
                        onTerminalDirChange={val => this.setState({ wsModalTerminalDir: val })}
                        onChatChannelChange={val => this.setState({ wsModalChatChannel: val })}
                        onDefaultAgentChange={val => this.setState({ wsModalDefaultAgent: val })}
                        onClose={this.closeWsModal}
                        onBrowse={this.openDirPickerForModal}
                        onSubmit={this.submitWsModal}
                    />
                )}

                {/* Chat session create modal */}
                {chatCreateOpen &&
                    chatCreateWsId &&
                    (() => {
                        const ws = workspaces.find(w => w.id === chatCreateWsId);
                        if (!ws) return null;
                        return (
                            <SessionCreateModal
                                workspaceId={chatCreateWsId}
                                workspaceName={ws.name}
                                defaultAgent={ws.defaultAgent || DEFAULT_AGENT_TYPE}
                                onCancel={this.closeChatCreate}
                                onSubmit={(name, agentType) => {
                                    this.closeChatCreate();
                                    this.createChatSession(chatCreateWsId, name, agentType);
                                }}
                            />
                        );
                    })()}

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

/**
 * Merges a fresh list of directory entries into the existing tree structure,
 * preserving already loaded children and expansion states of matching paths.
 */
function mergeFreshEntries(existing: FsEntry[], fresh: FsEntry[]): FsEntry[] {
    const existingMap = new Map<string, FsEntry>();
    existing.forEach(e => {
        existingMap.set(e.path, e);
    });

    return fresh.map(f => {
        const ext = existingMap.get(f.path);
        if (ext) {
            return {
                ...f,
                expanded: ext.expanded,
                children: ext.children,
            };
        }
        return f;
    });
}
