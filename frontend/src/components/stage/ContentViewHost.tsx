import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import type { ITerminalOptions } from '@xterm/xterm';

import type { ContentView } from '../../stores/stageStore';
import type { App, AppState } from '../app';
import { isChat, type ChatSession } from '../types';
import { t, type Lang } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as sess from '../../stores/sessionStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabsStore from '../../stores/tabsStore';
import * as modal from '../../stores/modalStore';
import * as stage from '../../stores/stageStore';

import { Terminal } from '../terminal';
import { TerminalEmptyState } from '../shared/TerminalEmptyState';
import { ChatPanel } from '../chat/ChatPanel';
import { NewChatHome } from '../chat/NewChatHome';
import { FilePreviewContent } from '../shared/FilePreviewContent';
import { BuiltinBrowser } from '../browser/BuiltinBrowser';
import { FilesPane, ChannelsPane, activeWorkspacePath } from '../shared/WorkspacePanes';
import { GitPanel } from '../drawer/GitPanel';
import { TaskList } from '../drawer/TaskList';
import { ProjectShell } from '../platform/ProjectShell';
import { ShellNav, type ShellTab } from '../platform/ShellNav';
import { L1AppPage } from '../platform/L1Shell';
import { visibleSettingsCategories, type SettingsCategory } from '../../modules/settings-manifest';
import { RemindersPane } from '../drawer/Reminders';
import { AssistantsPage } from '../pages/AssistantsPage';
import { InboxPane } from '../drawer/Inbox';
import { ContactsPane } from '../drawer/Contacts';
import { DataSourcesPane } from '../drawer/DataSources';
import { DiscoveryPanel } from '../drawer/DiscoveryPanel';
import { CcProvidersPanel } from '../shared/CcProvidersPanel';
import { SystemSettingsHost } from '../shared/SystemSettingsHost';
import {
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
} from '../terminal/terminalConfig';

interface ContentViewHostProps {
    /** The content this pane should render. */
    view: ContentView;
    app: App;
    state: AppState;
    /** Terminal font size — 13 desktop, 12 mobile. */
    fontSize?: number;
}

/**
 * Renders a single `ContentView` into a pane. This is the one place that
 * maps a content kind to its leaf component — the consolidation of what
 * used to be split across MiddleCanvas (terminal/chat), RightPanel
 * (files/git/channels/settings) and DesktopAppLayout (preview/browser/
 * full-page modules). The pane supplies its own frame; this renders only
 * the body, so no panel chrome is duplicated here.
 *
 * It reads layout/workspace/session state from the signal stores directly
 * (the same way the old components did), so callers pass only `view`,
 * `app` and `state`.
 */
export function ContentViewHost({ view, app, state, fontSize = 13 }: ContentViewHostProps) {
    const language = ui.language.value;
    const theme = ui.theme.value;

    switch (view.kind) {
        case 'terminal':
            return renderTerminal(app, theme, fontSize);
        case 'chat':
            return renderChat(view);
        case 'newChat':
            return renderNewChat(language);
        case 'preview':
            return (
                <FilePreviewContent
                    app={app}
                    activeTabId={view.tabId}
                    onOpenPreview={(path, name) => tabsStore.openPreviewTab(path, name)}
                />
            );
        case 'browser':
            return renderBrowser(view.tabId, language);
        case 'files':
            return <FilesPane app={app} language={language} />;
        case 'git':
            return (
                <GitPanel
                    workdir={activeWorkspacePath()}
                    activeWorkspaceId={wsStore.activeWorkspaceId.value}
                    language={language}
                />
            );
        case 'tasks': {
            // Use ProjectShell (#331) when a workspace is active — it adds the
            // 动态/计划/任务/资产 tab bar plus any enabled project-tab apps.
            // Fall back to bare TaskList when there is no active workspace.
            const activeWsId = wsStore.activeWorkspaceId.value;
            const activeWs = wsStore.workspaces.value.find(w => w.id === activeWsId);
            if (activeWsId) {
                return (
                    <div
                        style={{
                            flex: 1,
                            minHeight: 0,
                            display: 'flex',
                            flexDirection: 'column',
                            overflow: 'hidden',
                            backgroundColor: 'var(--bg-panel)',
                        }}
                    >
                        <ProjectShell workspaceId={activeWsId} workspaceName={activeWs?.name} />
                    </div>
                );
            }
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        padding: '12px 16px',
                        overflow: 'hidden',
                        boxSizing: 'border-box',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <TaskList workspaceId={activeWsId} onSelectSession={s => sess.selectSession(s)} />
                </div>
            );
        }
        case 'l1-app':
            // L1 app page (#332) — full-pane app view rendered via MountPointRenderer.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        overflow: 'hidden',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <L1AppPage mountId={view.mountId} />
                </div>
            );
        case 'reminders':
            // Personal reminders / 定时任务 — own full-page pane (#192), same
            // padded scroll frame as the tasks landing.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        padding: '12px 16px',
                        overflow: 'hidden',
                        boxSizing: 'border-box',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <RemindersPane />
                </div>
            );
        case 'assistants':
            // No padding / scroll here — AssistantsPage (grid) and its tabbed
            // detail manage their own scroll so full-height panes (任务/文件/渠道)
            // don't nest inside an outer scroller.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        overflow: 'hidden',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <AssistantsPage app={app} />
                </div>
            );
        case 'contacts':
            // 联系人聚合 — own full-page pane, same padded scroll frame as the
            // inbox/reminders/tasks landings.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        padding: '12px 16px',
                        overflow: 'hidden',
                        boxSizing: 'border-box',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <ContactsPane />
                </div>
            );
        case 'inbox':
            // Inbox 统一信息收口层 (#60) — own full-page pane, same padded
            // scroll frame as the reminders/tasks landings.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        padding: '12px 16px',
                        overflow: 'hidden',
                        boxSizing: 'border-box',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <InboxPane />
                </div>
            );
        case 'datasources':
            // 数据源管理 — full-bleed like the project detail page so the shared
            // ShellNav (breadcrumb + tab bar) spans edge-to-edge; the pane pads
            // its own content body.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        overflow: 'hidden',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <DataSourcesPane />
                </div>
            );
        case 'channels':
            return <ChannelsPane theme={theme} language={language} />;
        case 'providers':
            return wsStore.ccProvidersUrl.value ? (
                <CcProvidersPanel
                    ccProvidersUrl={wsStore.ccProvidersUrl.value}
                    panelStyle={{
                        width: '100%',
                        height: '100%',
                        display: 'flex',
                        flexDirection: 'column',
                        minHeight: 0,
                        overflow: 'hidden',
                    }}
                />
            ) : null;
        case 'skills':
            return renderSkills(theme, language);
        case 'discovery': {
            // Category nav lives in the top tab bar (ShellNav), not the sidebar.
            const discoveryTabs: ShellTab[] = [
                { id: 'apps', label: t('discovery.catApps', language) },
                { id: 'featured', label: t('discovery.catFeatured', language) },
                { id: 'opensource', label: t('discovery.catOpensource', language) },
            ];
            return (
                <div class="project-shell">
                    <ShellNav
                        tabs={discoveryTabs}
                        activeTab={tabsStore.discoveryCategory.value}
                        onSelectTab={id => tabsStore.selectDiscoveryCategory(id)}
                    />
                    <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '24px' }}>
                        <DiscoveryPanel
                            onOpenBrowserTab={tabsStore.openBrowserTab}
                            onOpenApp={appId => {
                                if (!stage.openAppById(appId)) {
                                    ui.showToast(
                                        language === 'zh-CN' ? '应用未启用或不可用' : 'App is disabled or unavailable'
                                    );
                                }
                            }}
                            language={language}
                            activeCategory={tabsStore.discoveryCategory.value}
                        />
                    </div>
                </div>
            );
        }
        case 'settings': {
            // Category nav lives in the top tab bar (ShellNav), not the sidebar.
            const settingsTabs: ShellTab[] = visibleSettingsCategories().map(c => ({
                id: c.key,
                label: t(c.i18nKey, language),
            }));
            return (
                <div class="project-shell">
                    <ShellNav
                        tabs={settingsTabs}
                        activeTab={tabsStore.activeSettingsCategory.value}
                        onSelectTab={id => tabsStore.setSettingsCategory(id as SettingsCategory)}
                    />
                    <div
                        style={{ flex: 1, minHeight: 0, overflow: 'hidden', display: 'flex', flexDirection: 'column' }}
                    >
                        <SystemSettingsHost
                            app={app}
                            state={state}
                            activeCategory={tabsStore.activeSettingsCategory.value}
                        />
                    </div>
                </div>
            );
        }

        default:
            return null;
    }
}

/**
 * The `.middle-canvas > .terminal-card` shell that wraps the workbench's
 * terminal and chat. Reproduces the exact DOM that `MiddleCanvas`
 * produced, so swapping this host into the primary slot is layout-neutral.
 */
const cardWrap = (children: h.JSX.Element) => (
    <main class="middle-canvas">
        <div class="terminal-card">{children}</div>
    </main>
);

function renderTerminal(app: App, theme: 'light' | 'dark', fontSize: number) {
    // No real terminals → show the empty state instead of mounting xterm, which
    // would otherwise expose the hidden anchor window's bare shell.
    if (sess.terminalWindows.value.length === 0) {
        return cardWrap(<TerminalEmptyState language={ui.language.value} />);
    }
    const termOptions = {
        ...baseTermOptions,
        theme: theme === 'light' ? lightTermTheme : darkTermTheme,
        fontSize,
    } as ITerminalOptions;
    return cardWrap(
        <Terminal
            id="terminal-container"
            wsUrl={wsUrl}
            tokenUrl={tokenUrl}
            clientOptions={clientOptions}
            termOptions={termOptions}
            flowControl={flowControl}
            isMobile={ui.isMobile.value}
            onMobileDetect={isMobile => (ui.isMobile.value = isMobile)}
            onKeyboardStateChange={app.handleKeyboardStateChange}
            tmuxMouseOn={sess.tmuxMouseOn.value}
            onTmuxMouseToggle={sess.toggleTmuxMouse}
            language={ui.language.value}
        />
    );
}

function renderChat(view: { kind: 'chat'; sessionId?: string }) {
    const session = resolveChatSession(view.sessionId);
    if (!session) {
        return cardWrap(
            <div class="placeholder-view" style="margin: 0; border: none; border-radius: 0; height: 100%;">
                <svg
                    class="placeholder-icon"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                </svg>
                <h3 class="placeholder-title">选择一个聊天会话</h3>
                <p class="placeholder-desc">点击左侧工作空间旁的 +，选择"新建聊天"以开始一个会话。</p>
            </div>
        );
    }
    return cardWrap(
        <ChatPanel
            session={session}
            pendingInitialMessage={sess.pendingInitialMessage.value}
            onClearPendingInitialMessage={sess.clearPendingInitialMessage}
        />
    );
}

function renderNewChat(language: Lang) {
    return (
        <NewChatHome
            workspaces={wsStore.workspaces.value}
            activeWorkspaceId={wsStore.activeWorkspaceId.value}
            onOpenFolder={modal.openCreateWorkspacePicker}
            lockedWorkspaceId={sess.lockedNewChatWorkspaceId.value || undefined}
            language={language}
        />
    );
}

function resolveChatSession(sessionId?: string): ChatSession | null {
    if (sessionId) {
        for (const folder of wsStore.folders.value) {
            const found = folder.sessions.find(s => s.id === sessionId);
            if (found && isChat(found)) return found;
        }
        return null;
    }
    const active = sess.activeSession.value;
    return active && isChat(active) ? active : null;
}

function renderBrowser(tabId: string, language: Lang) {
    // Prefer explicit tabId; fall back to active workspace's browser session.
    let tab = tabsStore.tabs.value.find(t => t.id === tabId);
    if (!tab || tab.type !== 'browser') {
        tab = tabsStore.tabs.value.find(t => t.type === 'browser');
    }
    if (!tab) {
        // Ensure a home tab for the active project so the column is not empty.
        tabsStore.openBrowserTab('');
        tab = tabsStore.tabs.value.find(t => t.type === 'browser');
    }
    if (!tab) return null;
    return (
        <div
            class="builtin-browser-container"
            style={{ flex: 1, height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}
        >
            <BuiltinBrowser tab={tab} active={true} onUrlChange={tabsStore.updateBrowserUrl} language={language} />
        </div>
    );
}

function HarnessKitIframe(_props: { theme: 'light' | 'dark'; language: Lang }) {
    const [webUrl, setWebUrl] = useState<string | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let active = true;
        fetch('/api/harnesskit/status')
            .then(res => res.json())
            .then(data => {
                if (active && data.webUrl) {
                    setWebUrl(data.webUrl);
                }
            })
            .catch(() => {})
            .finally(() => {
                if (active) setLoading(false);
            });
        return () => {
            active = false;
        };
    }, []);

    if (loading) {
        return (
            <div style="display: flex; align-items: center; justify-content: center; width: 100%; height: 100%; font-family: sans-serif; opacity: 0.6;">
                Loading HarnessKit...
            </div>
        );
    }

    if (!webUrl) {
        return (
            <div style="display: flex; align-items: center; justify-content: center; width: 100%; height: 100%; font-family: sans-serif; color: #ef4444;">
                HarnessKit service unavailable
            </div>
        );
    }

    return (
        <iframe
            id="harnesskit-iframe"
            src={webUrl}
            allow="clipboard-read; clipboard-write"
            style="width: 100%; height: 100%; border: none; display: block; min-height: 0;"
        />
    );
}

function renderSkills(theme: 'light' | 'dark', language: Lang) {
    return <HarnessKitIframe theme={theme} language={language} />;
}
