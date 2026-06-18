import { h, Component } from 'preact';

import { FsEntry } from './types';
import { FileDetailView } from './drawer/FileDetailView';
import { AccessTokenGate } from './auth/AccessTokenGate';
import { WelcomeOnboarding } from './welcome/WelcomeOnboarding';
import { RelayPairingPanel } from './settings/RelayPairingPanel';
import { initBackend } from '../services/apiClient';
import { ModalHost } from './modal/ModalHost';
import { fsService } from '../services/fsService';
import { accessService } from '../services/accessService';
import { t } from '../i18n';
import { check as checkOta, type UpdateInfo } from '../ota/checker';
import { UpdateBanner } from '../ota/UpdateBanner';
import { DesktopAppLayout } from './desktop/DesktopAppLayout';
import { MobileAppLayout } from './mobile/MobileAppLayout';
import { DashboardApp } from './desktop/DashboardApp';

import * as ui from '../stores/uiStore';
import * as fs from '../stores/fsStore';
import * as wsStore from '../stores/workspaceStore';
import * as sess from '../stores/sessionStore';
import * as modal from '../stores/modalStore';
import * as tabsStore from '../stores/tabsStore';
import * as agentCatalog from '../stores/agentCatalogStore';
import * as stage from '../stores/stageStore';
import * as taskNav from '../stores/taskNavStore';

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

export interface AppState {
    // ── Access token state ──
    accessGateVisible: boolean;
    accessAuthRequired: boolean;
    accessAuthenticated: boolean;
    // 中转模式但未选节点 → 显示配对门禁,而不是误进工作空间引导
    backendGateVisible: boolean;
    // ── Frontend OTA update state ──
    otaUpdate: UpdateInfo | null;
}

// Drag resizer state (module-level for perf)
let _resizerActive: 'left' | 'right' | 'split' | null = null;
let _resizerStartX = 0;
let _resizerStartWidth = 0;
// Content-area width captured at drag start for the between-columns ('split')
// resizer, so we can translate pixel deltas into a split ratio.
let _resizerContainerW = 0;

export class App extends Component<{}, AppState> {
    private _tunnelHeartbeat: ReturnType<typeof setInterval> | null = null;
    private _terminalPollInterval: ReturnType<typeof setInterval> | null = null;

    constructor() {
        super();
        this.state = {
            accessGateVisible: false,
            accessAuthRequired: false,
            accessAuthenticated: true,
            backendGateVisible: false,
            otaUpdate: null,
        };
    }

    async componentDidMount() {
        // 解析后端来源:本机直连 / 经中转远程节点 / 未连接。
        const target = await initBackend();
        if (target.mode === 'none') {
            // 中转模式但还没选节点 → 显示配对门禁(在那里配对/选节点后会自动进入)。
            this.setState({ backendGateVisible: true });
            return;
        }

        // 直连模式才走本机的 access token 门禁;中转模式鉴权在中转侧,跳过。
        if (target.mode === 'direct') {
            await this.checkAccessStatus();
        }
        if (this.state.accessGateVisible) {
            document.addEventListener('keydown', this.handleKeyDown);
            document.addEventListener('mousemove', this.handleResizerMove);
            document.addEventListener('mouseup', this.handleResizerUp);
            window.addEventListener('resize', this.handleWindowResize);
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
        await Promise.all([wsStore.loadWorkspaces(true), sess.loadTerminals(), agentCatalog.loadAgentCatalog()]);

        // Synchronize terminal windows + cached chat sessions into folders
        sess.mergeSessionsIntoFolders(sess.terminalWindows.value, sess.chatSessions.value);

        // If we already have an active workspace, also refresh its chat sessions.
        if (wsStore.activeWorkspaceId.value) {
            sess.loadChatSessions(wsStore.activeWorkspaceId.value);
        }

        // Select default workspace if none is active, otherwise sync backend root
        const workspaces = wsStore.workspaces.value;
        const activeWorkspaceId = wsStore.activeWorkspaceId.value;
        if (!activeWorkspaceId && workspaces.length > 0) {
            await wsStore.selectWorkspace(workspaces[0]);
        } else if (activeWorkspaceId) {
            const ws = workspaces.find(w => w.id === activeWorkspaceId);
            if (ws) {
                await fs.switchFsContext(ws);
            } else {
                fs.loadDir('', null);
            }
        } else {
            fs.loadDir('', null);
        }

        sess.loadTmuxMouse();
        this.checkUrlPreview();
        // Task permalinks: intercept in-app clicks on `#N` autolinks and pasted
        // /{project}/tasks/{number} URLs, and resolve a deep link in the address
        // bar now that workspaces are loaded.
        taskNav.installTaskRefClicks();
        taskNav.consumeDeepLink();
        document.addEventListener('keydown', this.handleKeyDown);
        document.addEventListener('mousemove', this.handleResizerMove);
        document.addEventListener('mouseup', this.handleResizerUp);
        window.addEventListener('resize', this.handleWindowResize);
        // Module custom elements (<skills-panel>, <cc-connect-panel>)
        // bubble CustomEvent('navigate') up through the DOM when their
        // internal MemoryRouter routes change. The host mirrors the path
        // into its own URL state.
        document.addEventListener('navigate', tabsStore.handleModuleNavigate);
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
            sess.loadTerminals();
        }, 3000);

        // Frontend OTA: non-blocking manifest check (throttled inside checker).
        this.checkForFrontendUpdate();
    }

    componentWillUnmount() {
        document.removeEventListener('keydown', this.handleKeyDown);
        document.removeEventListener('mousemove', this.handleResizerMove);
        document.removeEventListener('mouseup', this.handleResizerUp);
        window.removeEventListener('resize', this.handleWindowResize);
        document.removeEventListener('navigate', tabsStore.handleModuleNavigate);
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
        if (ui.isMobile.value) {
            ui.viewportHeight.value = window.visualViewport ? window.visualViewport.height : window.innerHeight;
            ui.triggerTerminalFit();
        }
    };

    handleWindowResize = () => {
        const isMobile = window.innerWidth <= 768;
        if (isMobile !== ui.isMobile.value) {
            ui.isMobile.value = isMobile;
        }
    };

    handleKeyboardStateChange = (visible: boolean) => {
        ui.keyboardVisible.value = visible;
        ui.triggerTerminalFit();
    };

    handleKeyDown = (e: KeyboardEvent) => {
        if ((e.ctrlKey || e.metaKey) && e.key === 's') {
            e.preventDefault();
            fs.saveFile();
        }
    };

    /**
     * Fire-and-forget frontend OTA check. The checker is self-throttling
     * (6h) and soft-fails on missing manifest endpoint, so it's safe to
     * call from componentDidMount without try/catch here.
     */
    checkForFrontendUpdate = async () => {
        const info = await checkOta();
        if (info.hasUpdate) {
            this.setState({ otaUpdate: info });
        }
    };

    // ── Resizer drag handlers ──
    handleResizerDown = (side: 'left' | 'right' | 'split', e: MouseEvent) => {
        e.preventDefault();
        _resizerActive = side;
        _resizerStartX = e.clientX;
        if (side === 'split') {
            const el = document.querySelector('.workspace-body-container') as HTMLElement | null;
            _resizerContainerW = el?.clientWidth || window.innerWidth;
            _resizerStartWidth = stage.splitRatio.value * _resizerContainerW;
        } else {
            _resizerStartWidth = side === 'left' ? ui.leftSidebarWidth.value : ui.rightPanelWidth.value;
        }
        document.body.style.cursor = 'col-resize';
        document.body.style.userSelect = 'none';
    };

    handleResizerMove = (e: MouseEvent) => {
        if (!_resizerActive) return;
        const dx = e.clientX - _resizerStartX;
        if (_resizerActive === 'left') {
            const w = Math.max(160, Math.min(480, _resizerStartWidth + dx));
            ui.leftSidebarWidth.value = w;
        } else if (_resizerActive === 'split') {
            // Dragging right widens the chat (left) column.
            const ratio = _resizerContainerW > 0 ? (_resizerStartWidth + dx) / _resizerContainerW : 0.6;
            stage.setSplitRatio(ratio);
        } else {
            const w = Math.max(200, Math.min(600, _resizerStartWidth - dx));
            ui.rightPanelWidth.value = w;
        }
        ui.triggerTerminalFit();
    };

    handleResizerUp = () => {
        if (!_resizerActive) return;
        _resizerActive = null;
        document.body.style.cursor = '';
        document.body.style.userSelect = '';
        ui.triggerTerminalFit();
    };

    // ── Access token handlers ──────────────────────────────────────────────

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
            fs.loadDir('', null);
            await Promise.all([wsStore.loadWorkspaces(true), sess.loadTerminals()]);
            sess.mergeSessionsIntoFolders(sess.terminalWindows.value, sess.chatSessions.value);
            const workspaces = wsStore.workspaces.value;
            const activeWorkspaceId = wsStore.activeWorkspaceId.value;
            if (!activeWorkspaceId && workspaces.length > 0) {
                await wsStore.selectWorkspace(workspaces[0]);
            } else if (activeWorkspaceId) {
                await Promise.all([wsStore.loadCcConnectUrl(), wsStore.loadCcProvidersUrl()]);
            }
            sess.loadTmuxMouse();
            this.checkUrlPreview();
        }
    };

    generateAccessToken = async () => {
        try {
            const token = await accessService.generateToken();
            modal.accessTokenModalToken.value = token;
            this.setState({ accessAuthRequired: true });
        } catch (err) {
            ui.showToast(t('app.toast.tokenGenerateFailed', ui.language.value, { err: String(err) }));
        }
    };

    revokeAccessToken = async () => {
        try {
            await accessService.revokeToken();
            ui.showToast(t('app.toast.tokenRevoked', ui.language.value));
            await this.checkAccessStatus();
        } catch (err) {
            ui.showToast(t('app.toast.tokenRevokeFailed', ui.language.value, { err: String(err) }));
        }
    };

    shareFile = async () => {
        const selectedFsEntry = fs.selectedFsEntry.value;
        if (!selectedFsEntry) return;

        const activeWorkspace = wsStore.workspaces.value.find(w => w.id === wsStore.activeWorkspaceId.value);
        const activeWorkspacePath = activeWorkspace?.path || '.';
        const absolutePath = selectedFsEntry.path.startsWith('/')
            ? selectedFsEntry.path
            : `${activeWorkspacePath}/${selectedFsEntry.path}`;

        const shareUrl = `${window.location.origin}${window.location.pathname}?preview=${encodeURIComponent(
            absolutePath
        )}`;
        try {
            await navigator.clipboard.writeText(shareUrl);
            ui.showToast(t('app.toast.shareCopied', ui.language.value));
        } catch (_) {
            ui.showToast(t('app.toast.shareCopyFailed', ui.language.value));
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

        tabsStore.activeDrawerTab.value = 'files';
        fs.viewMode.value = 'detail';
        fs.detailFullscreen.value = true;
        await fs.openFileDetail(entry);
    };

    render() {
        const dashboardModeParams = new URLSearchParams(window.location.search);
        if (dashboardModeParams.get('mode') === 'dashboard') {
            return <DashboardApp />;
        }

        const { accessGateVisible, backendGateVisible, otaUpdate } = this.state;
        const toastMsg = ui.toastMsg.value;
        const language = ui.language.value;
        const workspaces = wsStore.workspaces.value;
        const workspacesLoading = wsStore.workspacesLoading.value;
        const hasLoadedWorkspaces = wsStore.hasLoadedWorkspaces.value;
        const favoriteFiles = fs.favoriteFiles.value;
        const isEditingDetail = fs.isEditingDetail.value;
        const selectedFsEntry = fs.selectedFsEntry.value;
        const fileContent = fs.fileContent.value;
        const editedContent = fs.editedContent.value;
        const fileLoading = fs.fileLoading.value;
        const fileSaving = fs.fileSaving.value;
        const fileSaveMsg = fs.fileSaveMsg.value;
        const isImagePreview = fs.isImagePreview.value;
        // If access gate is visible, render only the gate
        if (accessGateVisible) {
            return <AccessTokenGate onAuthenticated={this.onAccessAuthenticated} language={language} />;
        }

        // 中转模式但未连接到节点 → 显示配对门禁(配对/选节点后会自动进入主界面)。
        if (backendGateVisible) {
            return (
                <div class="app-container" style="min-height:100vh;background:var(--bg-page);overflow:auto">
                    <div class="sys-settings-page sys-settings-page--bare">
                        <div class="sys-settings-content">
                            <div class="sys-settings-section" style="margin-bottom:8px">
                                <div class="sys-settings-section-title">未检测到本地后端</div>
                                <div class="sys-settings-section-desc">
                                    当前页面由中转服务器提供,需要先配对/选择一台远程节点才能使用。配对一次后,以后进来会自动连接。
                                </div>
                            </div>
                            <RelayPairingPanel onNodeSelected={() => window.location.reload()} />
                        </div>
                    </div>
                </div>
            );
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
                        onToggleFavorite={fs.toggleFavorite}
                        onCopyContent={fs.copyFileContent}
                        onDownloadFile={fs.downloadFile}
                        onRenameFile={fs.renameFile}
                        onToggleFullscreen={() => {
                            window.location.href = window.location.origin + window.location.pathname;
                        }}
                        onShareFile={this.shareFile}
                        onSaveFile={fs.saveFile}
                        onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
                        onEditedContentChange={content => (fs.editedContent.value = content)}
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
                {otaUpdate && <UpdateBanner info={otaUpdate} language={language} />}
                {hasLoadedWorkspaces && workspaces.length === 0 ? (
                    <WelcomeOnboarding
                        language={language}
                        onCreateWorkspace={modal.openCreateWorkspacePicker}
                        onUseTempWorkspace={wsStore.onUseTempWorkspace}
                    />
                ) : ui.isMobile.value ? (
                    <MobileAppLayout app={this} state={this.state} />
                ) : (
                    <DesktopAppLayout app={this} state={this.state} />
                )}

                {/* App-level modals (workspace, chat-create, dir picker, token, rename) */}
                <ModalHost />

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
