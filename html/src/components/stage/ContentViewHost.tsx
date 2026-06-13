import { h } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';

import type { ContentView } from '../../stores/stageStore';
import type { App, AppState } from '../app';
import { isChat, type ChatSession } from '../types';
import { AGENT_TYPE_LABELS } from '../types';
import type { Lang } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';
import * as sess from '../../stores/sessionStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabsStore from '../../stores/tabsStore';
import { getModuleByTab } from '../../modules/registry';
import { fsService } from '../../services/fsService';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';

import { Terminal } from '../terminal';
import { ChatPanel } from '../chat/ChatPanel';
import { NewChatHome } from '../chat/NewChatHome';
import { FilePreviewContent } from '../shared/FilePreviewContent';
import { BuiltinBrowser } from '../browser/BuiltinBrowser';
import { FlatFileBrowser } from '../drawer/FlatFileBrowser';
import { FileDetailView } from '../drawer/FileDetailView';
import { GitPanel } from '../drawer/GitPanel';
import { TaskList } from '../drawer/TaskList';
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
                    onOpenPreview={IS_DESKTOP ? (path, name) => tabsStore.openPreviewTab(path, name) : undefined}
                />
            );
        case 'browser':
            return renderBrowser(view.tabId, language);
        case 'files':
            return renderFiles(app, language);
        case 'git':
            return (
                <GitPanel
                    workdir={activeWorkspacePath()}
                    activeWorkspaceId={wsStore.activeWorkspaceId.value}
                    language={language}
                />
            );
        case 'tasks':
            // Padded scroll frame — replaces the old .kanban-background-layer
            // which provided the project-landing's padding/scroll.
            return (
                <div
                    style={{
                        flex: 1,
                        minHeight: 0,
                        display: 'flex',
                        flexDirection: 'column',
                        padding: '12px 16px',
                        overflow: 'auto',
                        backgroundColor: 'var(--bg-panel)',
                    }}
                >
                    <TaskList
                        workspaceId={wsStore.activeWorkspaceId.value}
                        onSelectSession={s => sess.selectSession(s)}
                    />
                </div>
            );
        case 'channels':
            return renderChannels(theme, language);
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
        case 'discovery':
            return (
                <div style={{ flex: 1, overflowY: 'auto', padding: '24px' }}>
                    <DiscoveryPanel
                        onOpenBrowserTab={IS_DESKTOP ? tabsStore.openBrowserTab : undefined}
                        language={language}
                        scrollToCategory={tabsStore.discoveryCategory.value}
                    />
                </div>
            );
        case 'settings':
            return (
                <div style={{ flex: 1, overflow: 'hidden', display: 'flex', flexDirection: 'column', height: '100%' }}>
                    <SystemSettingsHost
                        app={app}
                        state={state}
                        activeCategory={tabsStore.activeSettingsCategory.value}
                    />
                </div>
            );
        default:
            return null;
    }
}

const activeWorkspacePath = (): string => {
    const ws = wsStore.workspaces.value.find(w => w.id === wsStore.activeWorkspaceId.value);
    return ws?.path || '.';
};

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
            onSelectWorkspace={ws => wsStore.selectWorkspace(ws)}
            onSubmitChat={(wsId, agentType, prompt) => {
                const name = `${AGENT_TYPE_LABELS[agentType] ?? agentType} 会话`;
                sess.createChatSession(wsId, name, agentType, prompt);
            }}
            onSubmitTerminal={(wsId, cwd, initialCommand) => {
                sess.createTerminal(wsId, cwd, initialCommand);
            }}
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
    const tab = tabsStore.tabs.value.find(t => t.id === tabId);
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

function renderFiles(app: App, language: Lang) {
    const selectedFsEntry = fs.selectedFsEntry.value;
    if (fs.viewMode.value === 'list') {
        return (
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
        );
    }
    if (!selectedFsEntry) return null;
    return (
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
            onToggleFullscreen={() => openSelectedAsPreview()}
            onShareFile={app.shareFile}
            onSaveFile={fs.saveFile}
            onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
            onEditedContentChange={content => (fs.editedContent.value = content)}
            onOpenPreview={IS_DESKTOP ? (path, name) => tabsStore.openPreviewTab(path, name) : undefined}
            language={language}
        />
    );
}

/** Desktop "fullscreen": promote the selected file to its own preview tab. */
function openSelectedAsPreview() {
    const entry = fs.selectedFsEntry.value;
    if (!entry) return;
    const base = activeWorkspacePath();
    const absolutePath = entry.path.startsWith('/') ? entry.path : `${base}/${entry.path}`;
    if (IS_DESKTOP) {
        tabsStore.openPreviewTab(absolutePath, entry.name);
    } else {
        const shareUrl = `${window.location.origin}${window.location.pathname}?preview=${encodeURIComponent(
            absolutePath
        )}`;
        window.open(shareUrl, '_blank');
    }
}

function renderChannels(theme: 'light' | 'dark', language: Lang) {
    const ccConnectUrl = wsStore.ccConnectUrl.value;
    if (!ccConnectUrl) return null;
    return (
        <div style="flex: 1; overflow: hidden; display: flex; flex-direction: column; height: 100%;">
            <cc-connect-panel
                id="cc-channels-panel"
                route={extractCcRedirect(ccConnectUrl)}
                theme={theme}
                lang={language}
                auth-token={extractCcToken(ccConnectUrl)}
                style="width: 100%; height: 100%; display: flex; flex-direction: column; min-height: 0; overflow: hidden;"
            />
        </div>
    );
}

function renderSkills(theme: 'light' | 'dark', language: Lang) {
    const skillsMod = getModuleByTab('skills');
    const activeModulePath = tabsStore.activeModulePath.value;
    const initialRoute = activeModulePath || (skillsMod ? skillsMod.entryPath : '/overview');
    return (
        <skills-panel
            id="skills-panel"
            route={initialRoute}
            theme={theme}
            lang={language}
            style="width: 100%; height: 100%; display: flex; flex-direction: column; min-height: 0; overflow: hidden;"
        />
    );
}
