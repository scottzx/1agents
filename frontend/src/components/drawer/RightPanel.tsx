import { h } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';
import { useState, useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { RightDrawerTab, Session } from '../types';
import { FlatFileBrowser } from './FlatFileBrowser';
import { FileDetailView } from './FileDetailView';
import { ThemeSettings } from './ThemeSettings';
import { GitPanel } from './GitPanel';
import { TaskList } from './TaskList';
import { BuiltinBrowser } from '../browser/BuiltinBrowser';
import * as tabsStore from '../../stores/tabsStore';
import * as sess from '../../stores/sessionStore';
import { ProjectShell } from '../platform/ProjectShell';
import { t } from '../../i18n';
import { fsService } from '../../services/fsService';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as taskNav from '../../stores/taskNavStore';
import * as appStore from '../../stores/appManifestStore';
import { isOneshotWorkspaceId } from '../../utils/oneshot';
import { terminalService } from '@1agents/core/services/terminalService';
import { Terminal } from '../terminal';
import { BackgroundTaskPanel } from './BackgroundTaskPanel';
import {
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
} from '../terminal/terminalConfig';

interface RightPanelProps {
    activeDrawerTab: RightDrawerTab;
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    rightPanelWidth: number;
    closeDrawer: () => void;
    ccConnectUrl?: string;
    onSelectSession?: (session: Session) => void;
    /**
     * Overrides the aside's inline sizing. Desktop two-column passes a
     * flex/split style; mobile/legacy leaves it unset to keep the fixed
     * `rightPanelWidth` px behavior.
     */
    paneStyle?: string;

    // Context-dependent file actions (need app/workspace knowledge)
    onRefreshFlatFiles: () => void;
    onToggleFullscreen: () => void;
    onShareFile: () => void;
    onOpenPreview?: (path: string, name: string) => void;

    // Access token props
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
}

export function RightPanel({
    activeDrawerTab,
    activeWorkspaceId,
    activeWorkspacePath,
    rightPanelWidth,
    closeDrawer,
    ccConnectUrl,
    onSelectSession,
    onRefreshFlatFiles,
    onToggleFullscreen,
    onShareFile,
    onOpenPreview,
    accessTokenExists,
    onGenerateAccessToken,
    onRevokeAccessToken,
    paneStyle,
}: RightPanelProps) {
    const [gitLoading, setGitLoading] = useState(false);
    const [gitRefreshFn, setGitRefreshFn] = useState<(() => void) | null>(null);
    const taskSelectedId = useSignal<string | null>(null);
    // Back stack for in-detail task→task navigation (e.g. clicking a #N
    // reference). The header back arrow pops this so it returns to the task you
    // came from (GitHub-style), falling back to the list when empty.
    const taskNavStack = useSignal<string[]>([]);
    const language = ui.language.value;
    const theme = ui.theme.value;
    const sideTabs = tabsStore.sidePanelTabs.value;
    const activeSideTab = tabsStore.activeSidePanelTab.value;
    const sidePanelDrawerActive =
        activeDrawerTab === 'tasks' ||
        activeDrawerTab === 'files' ||
        activeDrawerTab === 'browser' ||
        activeDrawerTab === 'git' ||
        activeDrawerTab === 'terminal' ||
        activeDrawerTab === 'background';
    const sidePanelMode = !ui.isMobile.value && tabsStore.sidePanelOpen.value && sidePanelDrawerActive;
    const sidePanelEmpty = sidePanelMode && sideTabs.length === 0;
    const panelType = (
        sidePanelMode && activeSideTab ? activeSideTab.type : sidePanelEmpty ? 'none' : activeDrawerTab
    ) as RightDrawerTab;
    const selectTask = (id: string | null) => {
        const cur = taskSelectedId.value;
        if (id === null) {
            taskNavStack.value = [];
        } else if (cur && cur !== id) {
            taskNavStack.value = [...taskNavStack.value, cur];
        }
        taskSelectedId.value = id;
    };
    // Task detail uses the same global header back icon on desktop and mobile.
    // Its higher-priority layer temporarily wins over project/roundtable back;
    // unregistering it restores the parent action automatically.
    const hasTaskSelection = panelType === 'tasks' && (activeSideTab?.selectedTaskId ?? taskSelectedId.value) !== null;
    useEffect(() => {
        if (!hasTaskSelection) return;
        return taskNav.registerHeaderBackAction(
            'right-panel-task-detail',
            () => {
                const stack = taskNavStack.value;
                if (stack.length > 0) {
                    const nextId = stack[stack.length - 1];
                    taskSelectedId.value = nextId;
                    if (activeSideTab) tabsStore.updateSidePanelTab(activeSideTab.id, { selectedTaskId: nextId });
                    taskNavStack.value = stack.slice(0, -1);
                } else {
                    taskSelectedId.value = null;
                    if (activeSideTab) tabsStore.updateSidePanelTab(activeSideTab.id, { selectedTaskId: null });
                }
            },
            taskNav.HEADER_BACK_PRIORITY.detail
        );
    }, [hasTaskSelection, activeWorkspaceId]);

    const viewMode = fs.viewMode.value;
    const selectedFsEntry = fs.selectedFsEntry.value;
    useEffect(() => {
        if (!sidePanelMode || panelType !== 'files' || !activeSideTab?.path) return;
        const name = activeSideTab.path.split('/').filter(Boolean).pop() || activeSideTab.path;
        void fs.openFileDetail(
            { name, path: activeSideTab.path, isDir: false, size: 0, modTime: 0 },
            activeSideTab.line,
            activeSideTab.lineEnd
        );
    }, [sidePanelMode, panelType, activeSideTab?.id, activeSideTab?.path, activeSideTab?.line, activeSideTab?.lineEnd]);
    // cc-connect addresses projects by their name (== workspace display name).
    const activeWorkspaceName = wsStore.workspaces.value.find(w => w.id === activeWorkspaceId)?.name ?? '';

    let isSpinning = false;
    if (panelType === 'files') {
        isSpinning = fs.fsLoading.value || fs.flatFilesLoading.value;
    } else if (panelType === 'git') {
        isSpinning = gitLoading;
    }
    // Desktop two-column passes an explicit flex/split style; otherwise fall
    // back to the legacy fixed px width (mobile full-width overlay).
    const asideStyle =
        activeDrawerTab === 'none' ? '' : paneStyle !== undefined ? paneStyle : `width: ${rightPanelWidth}px`;

    return (
        <aside class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`} style={asideStyle}>
            <div class="panel-tabs-header">
                {sidePanelMode ? (
                    <div class="side-panel-tab-strip">
                        {sideTabs.map(tab => (
                            <button
                                key={tab.id}
                                class={`side-panel-tab ${activeSideTab?.id === tab.id ? 'active' : ''} ${tab.reclaimed ? 'reclaimed' : ''} ${tabsStore.isPinnedSidePanelTab(tab) ? 'pinned' : ''}`}
                                onClick={() => tabsStore.selectSidePanelTab(tab.id)}
                                title={tab.title}
                            >
                                <span class="side-panel-tab-title">{tab.title}</span>
                                {!tabsStore.isPinnedSidePanelTab(tab) && (
                                    <span
                                        class="side-panel-tab-close"
                                        onClick={(e: MouseEvent) => {
                                            e.stopPropagation();
                                            tabsStore.closeSidePanelTab(tab.id);
                                        }}
                                        title={t('common.closeTab', language)}
                                    >
                                        ×
                                    </span>
                                )}
                            </button>
                        ))}
                        {sideTabs.length === 0 && (
                            <span class="panel-tab-title">{t('sidePanel.empty.title', language)}</span>
                        )}
                    </div>
                ) : (
                    <span class="panel-tab-title">{legacyDrawerTitle(panelType, language)}</span>
                )}
                <div class="panel-header-actions">
                    {sidePanelMode && <SidePanelAddMenu language={language} />}
                    {/* Board-level create action (新建讨论 / 新建里程碑), published by
                        TaskList for the current view. Hidden while a task is open. */}
                    {panelType === 'tasks' &&
                        (activeSideTab?.selectedTaskId ?? taskSelectedId.value) === null &&
                        taskNav.taskAddAction.value && (
                            <button
                                class="panel-add-btn"
                                title={taskNav.taskAddAction.value.title}
                                onClick={() => taskNav.taskAddAction.value?.run()}
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
                        )}
                    {(panelType === 'files' || panelType === 'git') && (
                        <div
                            class={`panel-refresh-btn ${isSpinning ? 'spinning' : ''}`}
                            onClick={panelType === 'files' ? onRefreshFlatFiles : () => gitRefreshFn?.()}
                            title={
                                panelType === 'files'
                                    ? t('drawer.refresh.files', language)
                                    : t('drawer.refresh.git', language)
                            }
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.72 2.78L21 8" />
                                <polyline points="21 3 21 8 16 8" />
                            </svg>
                        </div>
                    )}
                    <div
                        class="panel-close-btn"
                        onClick={() => {
                            taskSelectedId.value = null;
                            taskNavStack.value = [];
                            closeDrawer();
                        }}
                        title={t('drawer.collapse', language)}
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
            </div>

            {sidePanelEmpty && <SidePanelEmpty language={language} />}

            {/* cc-connect channels panel (custom element, kept alive to avoid remount latency) */}
            <div
                class="panel-body-iframe"
                style={`flex: 1; overflow: hidden; display: ${
                    panelType === 'channels' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%;`}
            >
                {/* Per-channel agent binding now lives inside the cc-connect
                    panel below (project detail → each channel's agent picker),
                    so the standalone 1agents section was removed. */}
                {ccConnectUrl && (
                    <cc-connect-panel
                        id="cc-channels-panel"
                        route={extractCcRedirect(ccConnectUrl)}
                        theme={theme}
                        lang={language}
                        auth-token={extractCcToken(ccConnectUrl)}
                        style="width: 100%; height: 100%; display: flex; flex-direction: column; min-height: 0; overflow: hidden;"
                    />
                )}
            </div>

            {/* Tasks panel (side-by-side with terminal/chat) */}
            <div
                class="panel-body-tasks"
                style={`flex: 1; overflow: hidden; display: ${
                    panelType === 'tasks' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%; min-height: 0;`}
            >
                {panelType === 'tasks' &&
                    // When a workspace is active and enabled apps contribute project
                    // tabs (#331), host the task list inside ProjectShell so the
                    // 动态/计划/任务/资产 scaffold + app tabs (素材/阶段追踪 …) show.
                    // No apps enabled → bare TaskList, identical to before.
                    // Oneshot (单次对话) has no projects row — never open ProjectShell.
                    (activeWorkspaceId &&
                    activeWorkspaceId !== 'oneshot' &&
                    !isOneshotWorkspaceId(activeWorkspaceId) &&
                    // kind=tmp: light board only, no app ProjectShell scaffold
                    wsStore.workspaces.value.find(w => w.id === activeWorkspaceId)?.kind !== 'tmp' &&
                    appStore.projectTabMounts.value.length > 0 ? (
                        <ProjectShell workspaceId={activeWorkspaceId} workspaceName={activeWorkspaceName} />
                    ) : (
                        <TaskList
                            workspaceId={activeWorkspaceId}
                            selectedTaskId={activeSideTab?.selectedTaskId ?? taskSelectedId.value}
                            onTaskSelect={id => {
                                if (activeSideTab)
                                    tabsStore.updateSidePanelTab(activeSideTab.id, { selectedTaskId: id });
                                selectTask(id);
                            }}
                            onSelectSession={onSelectSession}
                        />
                    ))}
            </div>

            {/* Built-in lightweight browser (peer of 文件) */}
            <div
                class="panel-body-browser"
                style={`flex: 1; overflow: hidden; display: ${
                    panelType === 'browser' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%; min-height: 0;`}
            >
                {panelType === 'browser' &&
                    (sidePanelMode && activeSideTab ? (
                        <BuiltinBrowser
                            tab={{
                                id: activeSideTab.id,
                                title: activeSideTab.title,
                                type: 'browser',
                                url: activeSideTab.url || '',
                                closable: true,
                            }}
                            active={true}
                            onUrlChange={tabsStore.updateSidePanelBrowserUrl}
                            language={language}
                        />
                    ) : (
                        <LegacyBrowser language={language} />
                    ))}
            </div>

            <div
                class="panel-body-terminal"
                style={`flex: 1; overflow: hidden; display: ${
                    panelType === 'terminal' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%; min-height: 0;`}
            >
                {panelType === 'terminal' && sidePanelMode && activeSideTab && (
                    <SidePanelTerminal
                        tab={activeSideTab}
                        activeWorkspaceId={activeWorkspaceId}
                        activeWorkspacePath={activeWorkspacePath}
                        theme={theme}
                        language={language}
                    />
                )}
            </div>

            {/* Live background-task status (peer of 终端/任务, driven by the
                active chat session's bridge state) */}
            <div
                class="panel-body-background"
                style={`flex: 1; overflow: hidden; display: ${
                    panelType === 'background' ? 'flex' : 'none'
                }; flex-direction: column; height: 100%; min-height: 0;`}
            >
                {panelType === 'background' && <BackgroundTaskPanel language={language} />}
            </div>

            {/* Other drawer tab contents (files, git, settings) */}
            <div
                class="panel-body-scroll"
                style={`display: ${
                    panelType !== 'channels' &&
                    panelType !== 'tasks' &&
                    panelType !== 'browser' &&
                    panelType !== 'terminal' &&
                    panelType !== 'background' &&
                    panelType !== 'none'
                        ? 'flex'
                        : 'none'
                };`}
            >
                {panelType === 'files' &&
                    (activeWorkspaceId === 'oneshot' ? (
                        <div class="task-oneshot-empty">
                            <div class="task-oneshot-empty-inner">
                                <p class="task-oneshot-empty-title">{t('tasks.oneshot.emptyTitle', language)}</p>
                                <p class="task-oneshot-empty-desc">{t('tasks.oneshot.emptyDesc', language)}</p>
                            </div>
                        </div>
                    ) : viewMode === 'list' ? (
                        <FlatFileBrowser
                            flatFiles={fs.flatFiles.value}
                            flatFilesLoading={fs.flatFilesLoading.value}
                            searchQuery={fs.searchQuery.value}
                            selectedFilterTag={fs.selectedFilterTag.value}
                            favoriteFiles={fs.favoriteFiles.value}
                            onSearchQueryChange={fs.handleSearchChange}
                            onFilterTagChange={fs.handleFilterTagChange}
                            onOpenFileDetail={fs.openFileDetail}
                            fsEntries={fs.fsEntries.value}
                            fsLoading={fs.fsLoading.value}
                            onToggleFsDir={fs.toggleFsDir}
                            language={language}
                        />
                    ) : (
                        selectedFsEntry && (
                            <FileDetailView
                                selectedFsEntry={selectedFsEntry}
                                favoriteFiles={fs.favoriteFiles.value}
                                detailFullscreen={fs.detailFullscreen.value}
                                isEditingDetail={fs.isEditingDetail.value}
                                fileContent={fs.fileContent.value}
                                editedContent={fs.editedContent.value}
                                fileLoading={fs.fileLoading.value}
                                fileSaving={fs.fileSaving.value}
                                fileSaveMsg={fs.fileSaveMsg.value}
                                isImagePreview={fs.isImagePreview.value}
                                imageUrl={fsService.imageUrl(selectedFsEntry.path)}
                                onBackToList={() => {
                                    fs.viewMode.value = 'list';
                                    fs.detailFullscreen.value = false;
                                }}
                                onToggleFavorite={fs.toggleFavorite}
                                onCopyContent={fs.copyFileContent}
                                onDownloadFile={fs.downloadFile}
                                onRenameFile={fs.renameFile}
                                onToggleFullscreen={onToggleFullscreen}
                                onShareFile={onShareFile}
                                onSaveFile={fs.saveFile}
                                onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
                                onEditedContentChange={content => (fs.editedContent.value = content)}
                                onOpenPreview={onOpenPreview}
                                targetLine={fs.detailTargetLine.value ?? undefined}
                                targetLineEnd={fs.detailTargetLineEnd.value ?? undefined}
                                language={language}
                            />
                        )
                    ))}

                {panelType === 'git' &&
                    (activeWorkspaceId === 'oneshot' ? (
                        <div class="task-oneshot-empty">
                            <div class="task-oneshot-empty-inner">
                                <p class="task-oneshot-empty-title">{t('tasks.oneshot.emptyTitle', language)}</p>
                                <p class="task-oneshot-empty-desc">{t('tasks.oneshot.emptyDesc', language)}</p>
                            </div>
                        </div>
                    ) : (
                        <GitPanel
                            workdir={activeWorkspacePath}
                            activeWorkspaceId={activeWorkspaceId}
                            onLoadingChange={setGitLoading}
                            onRegisterRefresh={fn => setGitRefreshFn(() => fn)}
                            language={language}
                        />
                    ))}

                {panelType === 'settings' && (
                    <ThemeSettings
                        theme={theme}
                        toggleTheme={ui.toggleTheme}
                        language={language}
                        toggleLanguage={ui.toggleLanguage}
                        accessTokenExists={accessTokenExists}
                        onGenerateAccessToken={onGenerateAccessToken}
                        onRevokeAccessToken={onRevokeAccessToken}
                    />
                )}
            </div>
        </aside>
    );
}

function legacyDrawerTitle(tab: RightDrawerTab, language: typeof ui.language.value) {
    switch (tab) {
        case 'files':
            return t('drawer.title.files', language);
        case 'browser':
            return t('app.browser.title', language);
        case 'git':
            return t('drawer.title.git', language);
        case 'channels':
            return t('drawer.title.channels', language);
        case 'providers':
            return t('drawer.title.providers', language);
        case 'settings':
            return t('drawer.title.settings', language);
        case 'skills':
            return t('drawer.title.skills', language);
        case 'discovery':
            return t('drawer.title.discovery', language);
        case 'tasks':
            return t('header.col.tasks', language);
        case 'terminal':
            return t('sidePanel.tab.terminal', language);
        case 'background':
            return t('sidePanel.tab.background', language);
        default:
            return '';
    }
}

function LegacyBrowser({ language }: { language: typeof ui.language.value }) {
    let tab = tabsStore.tabs.value.find(tb => tb.type === 'browser');
    if (!tab) {
        tabsStore.openBrowserTab('');
        tab = tabsStore.tabs.value.find(tb => tb.type === 'browser');
    }
    if (!tab) {
        return (
            <div class="browser-welcome-page" style="flex:1;display:flex;align-items:center;justify-content:center;">
                <button class="shortcut-btn" onClick={() => tabsStore.openBrowserTab('')}>
                    {t('app.browser.title', language)}
                </button>
            </div>
        );
    }
    return <BuiltinBrowser tab={tab} active={true} onUrlChange={tabsStore.updateBrowserUrl} language={language} />;
}

function SidePanelAddMenu({ language }: { language: typeof ui.language.value }) {
    const [open, setOpen] = useState(false);
    const options: Array<{ type: tabsStore.SidePanelTabType; label: string }> = [
        { type: 'tasks', label: t('sidePanel.tab.tasks', language) },
        { type: 'files', label: t('sidePanel.tab.files', language) },
        { type: 'browser', label: t('sidePanel.tab.browser', language) },
        { type: 'git', label: t('sidePanel.tab.git', language) },
        { type: 'terminal', label: t('sidePanel.tab.terminal', language) },
    ];
    return (
        <div class="side-panel-add-menu">
            <button class="panel-add-btn" title={t('sidePanel.addTab', language)} onClick={() => setOpen(!open)}>
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
            {open && (
                <div class="side-panel-add-popover">
                    {options.map(opt => (
                        <button
                            key={opt.type}
                            onClick={() => {
                                tabsStore.addSidePanelTab(opt.type);
                                setOpen(false);
                            }}
                        >
                            {opt.label}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

function SidePanelEmpty({ language }: { language: typeof ui.language.value }) {
    const options: Array<{ type: tabsStore.SidePanelTabType; label: string; desc: string }> = [
        { type: 'tasks', label: t('sidePanel.tab.tasks', language), desc: t('sidePanel.empty.tasks', language) },
        { type: 'files', label: t('sidePanel.tab.files', language), desc: t('sidePanel.empty.files', language) },
        { type: 'browser', label: t('sidePanel.tab.browser', language), desc: t('sidePanel.empty.browser', language) },
        { type: 'git', label: t('sidePanel.tab.git', language), desc: t('sidePanel.empty.git', language) },
        {
            type: 'terminal',
            label: t('sidePanel.tab.terminal', language),
            desc: t('sidePanel.empty.terminal', language),
        },
    ];
    return (
        <div class="side-panel-empty">
            <div class="side-panel-empty-inner">
                <h3>{t('sidePanel.empty.title', language)}</h3>
                <p>{t('sidePanel.empty.desc', language)}</p>
                <div class="side-panel-empty-grid">
                    {options.map(opt => (
                        <button key={opt.type} onClick={() => tabsStore.addSidePanelTab(opt.type)}>
                            <strong>{opt.label}</strong>
                            <span>{opt.desc}</span>
                        </button>
                    ))}
                </div>
            </div>
        </div>
    );
}

function SidePanelTerminal({
    tab,
    activeWorkspaceId,
    activeWorkspacePath,
    theme,
    language,
}: {
    tab: tabsStore.SidePanelTab;
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    theme: 'light' | 'dark';
    language: typeof ui.language.value;
}) {
    const createAndBind = async () => {
        if (!activeWorkspaceId) return;
        const created = await terminalService.create(activeWorkspaceId, activeWorkspacePath || '.');
        await sess.loadTerminals();
        const active =
            sess.terminalWindows.value.find(w => w.index === created.index) ||
            sess.terminalWindows.value.find(w => w.active) ||
            sess.terminalWindows.value[0];
        if (active && typeof active.index === 'number') {
            tabsStore.bindTerminalToSidePanelTab(tab.id, active.index);
        }
    };

    useEffect(() => {
        if (tab.reclaimed || typeof tab.terminalWindowIndex === 'number') return;
        void createAndBind();
    }, [tab.id, tab.reclaimed, tab.terminalWindowIndex, activeWorkspaceId, activeWorkspacePath]);

    useEffect(() => {
        if (typeof tab.terminalWindowIndex !== 'number') return;
        void sess.switchTerminal(tab.terminalWindowIndex);
        tabsStore.touchSidePanelTab(tab.id);
    }, [tab.id, tab.terminalWindowIndex]);

    if (tab.reclaimed) {
        return (
            <div class="side-panel-terminal-empty">
                <h3>{t('sidePanel.terminal.reclaimedTitle', language)}</h3>
                <p>{t('sidePanel.terminal.reclaimedDesc', language)}</p>
                <button onClick={() => void createAndBind()}>{t('sidePanel.terminal.recreate', language)}</button>
            </div>
        );
    }

    if (typeof tab.terminalWindowIndex !== 'number') {
        return <div class="side-panel-terminal-empty">{t('common.loading', language)}</div>;
    }

    const termOptions = {
        ...baseTermOptions,
        theme: theme === 'light' ? lightTermTheme : darkTermTheme,
        fontSize: 13,
    } as ITerminalOptions;

    return (
        <div class="side-panel-terminal-host" onFocus={() => tabsStore.touchSidePanelTab(tab.id)}>
            <Terminal
                key={tab.id}
                id={`side-terminal-${tab.id}`}
                wsUrl={wsUrl}
                tokenUrl={tokenUrl}
                clientOptions={clientOptions}
                termOptions={termOptions}
                flowControl={flowControl}
                isMobile={ui.isMobile.value}
                tmuxMouseOn={sess.tmuxMouseOn.value}
                onTmuxMouseToggle={sess.toggleTmuxMouse}
                language={language}
            />
        </div>
    );
}
