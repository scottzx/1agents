import { h, Component, Fragment } from 'preact';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

import { WorkspaceFolder, Workspace, FsEntry, RightDrawerTab, TmuxWindow, Session, isFullPageTab } from './types';
import { LeftSidebar } from './sidebar/LeftSidebar';
import { WorkspaceHeader } from './header/WorkspaceHeader';
import { MiddleCanvas } from './canvas/MiddleCanvas';
import { RightPanel } from './drawer/RightPanel';
import { DiscoveryPanel } from './drawer/DiscoveryPanel';
import { SystemSettings } from './settings/SystemSettings';
import { FileDetailView } from './drawer/FileDetailView';
import { AccessTokenGate } from './auth/AccessTokenGate';
import { WelcomeOnboarding } from './welcome/WelcomeOnboarding';
import { WorkspaceModal, DirPickerModal, AccessTokenModal, SessionRenameModal } from './modal';
import { workspaceService } from '../services/workspaceService';
import { terminalService } from '../services/terminalService';
import { fsService } from '../services/fsService';
import { accessService } from '../services/accessService';
import { t, type Lang } from '../i18n';
import { getModuleByTab, buildModuleIframeSrc, mergeManifests, type ModuleRegistration } from '../modules/registry';
import { postToModule, isModuleInboundMessage } from '../modules/post-message';
import type { ModuleManifest } from '../modules/module-types';

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

interface BuiltinBrowserProps {
    tab: Tab;
    active: boolean;
    onUrlChange: (tabId: string, url: string) => void;
    language: Lang;
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
            // Reject cross-origin messages so a misbehaving page can't poison the URL bar
            if (e.origin !== window.location.origin) return;
            const data = e.data;
            if (data && data.type === 'iframe_navigate' && typeof data.url === 'string') {
                // Strip /api/proxy?url= wrapper — mirrors handleIframeLoad's extraction
                const newUrl = this.getOriginalUrl(data.url);
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
        // Don't double-wrap an already-proxied URL — breaks the feedback loop
        // if tab.url is transiently a /api/proxy?url=... string
        if (urlStr.startsWith(`${window.location.origin}/api/proxy?url=`)) {
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
        const { language } = this.props;
        const isHome = !tab.url || tab.url === 'about:blank';

        return (
            <div
                class="builtin-browser"
                style={{ display: active ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}
            >
                <div class="browser-nav-bar">
                    <button
                        class="browser-refresh-btn"
                        onClick={this.handleRefresh}
                        title={t('app.browser.refresh', this.props.language)}
                        disabled={isHome}
                    >
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
                        placeholder={t('app.browser.placeholder', this.props.language)}
                        value={tab.url === 'about:blank' ? '' : tab.url}
                        ref={el => {
                            this.inputRef = el;
                        }}
                        onKeyDown={this.handleKeyPress}
                    />
                    <button
                        class="browser-open-external-btn"
                        onClick={this.handleOpenExternal}
                        title={t('app.browser.openExternal', this.props.language)}
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
                                <h3 class="welcome-title">{t('app.browser.title', language)}</h3>
                                <p class="welcome-desc">{t('app.browser.welcomeDesc', language)}</p>
                                <div class="welcome-tips">
                                    <div class="tip-item">
                                        <strong>{t('app.browser.tipProxyLabel', language)}</strong>
                                        <span>{t('app.browser.tipProxyDesc', language)}</span>
                                    </div>
                                    <div class="tip-item">
                                        <strong>{t('app.browser.tipExternalLabel', language)}</strong>
                                        <span>{t('app.browser.tipExternalDesc', language)}</span>
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
            ccProvidersUrl,
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
            toastMsg,
            activeSession,
            language,
            accessGateVisible,
            accessTokenModalToken,
            accessAuthRequired,
            tabs,
            activeTabId,
            sessionRenameModalOpen,
            sessionRenameTarget,
            sessionRenameName,
        } = this.state;

        const activeTabObj = tabs.find(t => t.id === activeTabId);

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
                                                        title={t('common.closeTab', language)}
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
                                    title={t('common.openBrowserTab', language)}
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
                                        onRenameSession={s => this.openRenameSessionModal(s)}
                                        onReorderFolders={this.reorderFolders}
                                        language={language}
                                        moduleNav={this.buildModuleNav()}
                                    />

                                    {/* Resizer: between LEFT sidebar and MIDDLE canvas */}
                                    {leftSidebarOpen && (
                                        <div
                                            class="resizer resizer-left"
                                            onMouseDown={(e: MouseEvent) => this.handleResizerDown('left', e)}
                                            title={t('app.resizer.leftTitle', language)}
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
                                              // Constrain height to visual viewport when keyboard is open.
                                              // Subtract the desktop tab bar height (38px) since it sits
                                              // above this container in the flex column and would otherwise
                                              // push content past the visual viewport.
                                              height: this.state.keyboardVisible
                                                  ? `${this.state.viewportHeight - (IS_DESKTOP ? 38 : 0)}px`
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
                                            language={language}
                                            moduleNav={this.buildModuleNav()}
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
                                                    {activeDrawerTab === 'skills' &&
                                                        (() => {
                                                            const skillsMod = getModuleByTab('skills');
                                                            const skillsSrc = skillsMod
                                                                ? buildModuleIframeSrc(skillsMod)
                                                                : '/1skills/';
                                                            return (
                                                                <iframe
                                                                    id="skills-iframe"
                                                                    src={skillsSrc}
                                                                    onLoad={e => {
                                                                        const iframe = e.target as HTMLIFrameElement;
                                                                        postToModule(iframe, {
                                                                            type: 'THEME_CHANGE',
                                                                            theme,
                                                                        });
                                                                        postToModule(iframe, {
                                                                            type: 'LANG_CHANGE',
                                                                            lang: language,
                                                                        });
                                                                    }}
                                                                    style={{
                                                                        width: '100%',
                                                                        height: '100%',
                                                                        border: 'none',
                                                                        background: 'transparent',
                                                                    }}
                                                                />
                                                            );
                                                        })()}
                                                    {activeDrawerTab === 'discovery' && (
                                                        <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
                                                            <DiscoveryPanel
                                                                onOpenBrowserTab={
                                                                    IS_DESKTOP ? this.openBrowserTab : undefined
                                                                }
                                                                language={language}
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
                                                        language={language}
                                                    />

                                                    {/* Resizer: between MIDDLE canvas and RIGHT panel */}
                                                    {activeDrawerTab !== 'none' && (
                                                        <div
                                                            class="resizer resizer-right"
                                                            onMouseDown={(e: MouseEvent) =>
                                                                this.handleResizerDown('right', e)
                                                            }
                                                            title={t('app.resizer.rightTitle', language)}
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
                                                        imageUrl={fsService.imageUrl(selectedFsEntry?.path ?? '')}
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
                                                        imageUrl={fsService.imageUrl(selectedFsEntry.path)}
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
                                                        language={language}
                                                    />
                                                ) : (
                                                    <div class="fb-loading">
                                                        <div class="fb-loading-spinner" />
                                                        <span>{t('app.loading.preview', language)}</span>
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
 *
 * On collapse, the previously-loaded `children` array is dropped so it can be
 * garbage-collected. The next time the directory is expanded, `loadDir` will
 * re-fetch its children. This prevents the tree from holding onto every
 * expanded directory's contents for the lifetime of the App instance.
 */
function setExpanded(entries: FsEntry[], targetPath: string, expanded: boolean): FsEntry[] {
    return entries.map(e => {
        if (e.path === targetPath) {
            return { ...e, expanded, children: expanded ? e.children : undefined };
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
