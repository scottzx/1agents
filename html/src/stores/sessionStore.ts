import { signal } from '@preact/signals';

import {
    isChat,
    isTerminal,
    isFullPageTab,
    type TmuxWindow,
    type Session,
    type ChatSession,
    type AgentType,
} from '../components/types';
import { terminalService } from '../services/terminalService';
import { agentService } from '../services/agentService';
import { ccCreateSession, ccDeleteSession, getCcAuth, ccProjectName } from '../services/ccconnectClient';
import { globalBridgeManager } from '../components/chat/hooks';
import { t } from '../i18n';
import * as ui from './uiStore';
import * as fs from './fsStore';
import * as wsStore from './workspaceStore';
import * as tabsStore from './tabsStore';
import * as modal from './modalStore';

/**
 * Session state (tmux terminal windows, chat session index, active session)
 * and its service-calling orchestration (loadTerminals, loadChatSessions,
 * createChatSession, selectSession, …).
 *
 * Note on imports: workspaceStore, sessionStore and tabsStore reference
 * each other only inside function bodies (never at module evaluation
 * time), so the import cycles between them are safe — ES-module live
 * bindings resolve the cross-store calls at call time.
 */

// ── Terminal / tmux state ──
export const terminalWindows = signal<TmuxWindow[]>([]);
export const terminalWindowsLoading = signal(false);
export const tmuxMouseOn = signal(true);

// ── Chat session state (1agents-side index) ──
export const chatSessions = signal<ChatSession[]>([]);
export const activeSession = signal<Session | null>(null);
export const pendingInitialMessage = signal<string | null>(null);

/** Sync tmux windows + chat sessions into workspace folders as sessions */
export const mergeSessionsIntoFolders = (windows: TmuxWindow[], chats: ChatSession[]) => {
    wsStore.folders.value = wsStore.folders.value.map(f => {
        const termSessions: Session[] = windows
            .filter(w => w.workspaceId === f.id)
            .map(w => ({
                kind: 'terminal',
                id: w.name,
                workspaceId: w.workspaceId,
                index: w.index,
                name: w.customName || t('app.session.title', ui.language.value, { index: w.index }),
                active: w.active,
                cwd: w.cwd,
                status: w.status,
                waitingFor: w.waitingFor,
                agent: w.agent,
            }));
        const chatSessionList: Session[] = chats.filter(c => c.workspaceId === f.id).map(c => ({ ...c }));
        // Chat sessions first (newer), then terminals.
        return { ...f, sessions: [...chatSessionList, ...termSessions] };
    });
    // Preserve the currently-active chat session if it still exists; otherwise
    // fall back to the most recently active terminal window.
    const prevActive = activeSession.value;
    const activeChat = prevActive && isChat(prevActive) ? chats.find(c => c.id === prevActive.id) : null;
    const activeWin = windows.find(w => w.active);
    activeSession.value = activeChat
        ? { ...activeChat, active: true }
        : activeWin
          ? {
                kind: 'terminal',
                id: activeWin.name,
                workspaceId: activeWin.workspaceId,
                index: activeWin.index,
                name: activeWin.customName || t('app.session.title', ui.language.value, { index: activeWin.index }),
                active: true,
                cwd: activeWin.cwd,
                status: activeWin.status,
                waitingFor: activeWin.waitingFor,
                agent: activeWin.agent,
            }
          : null;
};

/** Fetch all tmux windows from GET /api/terminal/list and sync to folders */
export const loadTerminals = async () => {
    terminalWindowsLoading.value = true;
    try {
        const windows = await terminalService.list();
        // Use whatever chat sessions we have cached; the chat loader
        // (loadChatSessions) will refresh them in parallel.
        mergeSessionsIntoFolders(windows, chatSessions.value);
        terminalWindows.value = windows;
        terminalWindowsLoading.value = false;
    } catch (err) {
        console.error('[terminal] list error:', err);
        terminalWindowsLoading.value = false;
    }
};

/** Fetch chat session index for the active workspace from /api/agent/sessions */
export const loadChatSessions = async (workspaceId?: string) => {
    const wsId = workspaceId ?? wsStore.activeWorkspaceId.value;
    if (!wsId) return;
    try {
        const chats = await agentService.list(wsId);
        chatSessions.value = chats;
        mergeSessionsIntoFolders(terminalWindows.value, chats);
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
export const createChatSession = async (
    workspaceId: string,
    name: string,
    agentType: AgentType,
    initialMessage?: string
) => {
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);
    if (!ws) {
        ui.showToast('工作空间不存在');
        return;
    }
    try {
        ui.showToast('正在创建聊天会话…');
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
        await loadChatSessions(workspaceId);
        // Auto-select the new session and switch to the agents tab.
        activeSession.value = { ...indexed, active: true };
        pendingInitialMessage.value = initialMessage || null;
        // Switch the primary pane to the new chat. activeTabId must move off
        // 'tasks' too, otherwise the kanban stays on top and the new session
        // never shows (the project-landing → session switch bug).
        tabsStore.activeTabId.value = 'terminal';
        tabsStore.activeTab.value = 'agents';
        ui.showToast('聊天会话已创建 ✓');
    } catch (err) {
        ui.showToast(`创建聊天失败: ${(err as Error).message}`);
    }
};

export const onStartNewChat = () => {
    activeSession.value = null;
    // Move the primary pane onto the new-chat landing, leaving the project
    // landing ('tasks') so the new-chat home actually renders on top.
    tabsStore.activeTabId.value = 'terminal';
    tabsStore.activeTab.value = 'new_chat';
};

export const clearPendingInitialMessage = () => {
    pendingInitialMessage.value = null;
};

/** Kill a chat session: delete from cc-connect, then unindex from 1agents. */
export const killChatSession = async (sessionId: string) => {
    const session = chatSessions.value.find(c => c.id === sessionId);
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
        // Clean up global WebSocket bridge session
        globalBridgeManager.destroy(sessionId);
        await agentService.delete(sessionId);
        await loadChatSessions(session.workspaceId);
        const active = activeSession.value;
        if (active && isChat(active) && active.id === sessionId) {
            activeSession.value = null;
            tabsStore.activeTab.value = 'terminal';
        }
        ui.showToast('聊天会话已关闭 ✓');
    } catch (err) {
        ui.showToast(`关闭失败: ${(err as Error).message}`);
    }
};

/** Create a new terminal tab via POST /api/terminal/create */
export const createTerminal = async (workspaceId: string, cwd: string) => {
    try {
        await terminalService.create(workspaceId, cwd);
        await loadTerminals();
        ui.showToast(t('app.toast.sessionCreated', ui.language.value));
    } catch (err) {
        ui.showToast(t('app.toast.sessionCreateFailed', ui.language.value, { err: String(err) }));
    }
};

/** Switch to a tmux window via POST /api/terminal/switch */
export const switchTerminal = async (windowIndex: number) => {
    try {
        await terminalService.switch(windowIndex);
        await loadTerminals();
    } catch (err) {
        console.error('[terminal] switch error:', err);
    }
};

/** Kill a terminal tab via POST /api/terminal/kill */
export const killTerminal = async (windowIndex: number) => {
    try {
        await terminalService.kill(windowIndex);
        await loadTerminals();
        ui.showToast(t('app.toast.sessionKilled', ui.language.value));
    } catch (err) {
        ui.showToast(t('app.toast.sessionKillFailed', ui.language.value, { err: String(err) }));
    }
};

/** Fetch current tmux mouse mode state */
export const loadTmuxMouse = async () => {
    try {
        const mouseOn = await terminalService.getMouse();
        tmuxMouseOn.value = mouseOn;
    } catch (err) {
        console.error('[terminal] load mouse state error:', err);
    }
};

/** Toggle tmux mouse mode state */
export const toggleTmuxMouse = async () => {
    const nextState = !tmuxMouseOn.value;
    try {
        const actualState = await terminalService.setMouse(nextState);
        tmuxMouseOn.value = actualState;
        if (actualState) {
            ui.showToast(t('app.toast.mouseScrollOn', ui.language.value));
        } else {
            ui.showToast(t('app.toast.mouseSelectOn', ui.language.value));
        }
    } catch (err) {
        ui.showToast(t('app.toast.mouseToggleFailed', ui.language.value, { err: String(err) }));
    }
};

export const selectSession = async (session: Session) => {
    if (isFullPageTab(tabsStore.activeDrawerTab.value)) {
        tabsStore.activeDrawerTab.value = 'none';
    }
    const oldWorkspaceId = wsStore.activeWorkspaceId.value;
    const workspaces = wsStore.workspaces.value;

    // 1. Optimistic UI update: mark the session active and switch tab.
    const updatedFolders = wsStore.folders.value.map(f => ({
        ...f,
        sessions: f.sessions.map(s => {
            if (isChat(s) && isChat(session)) return { ...s, active: s.id === session.id };
            if (isTerminal(s) && isTerminal(session)) return { ...s, active: s.index === session.index };
            return { ...s, active: false };
        }),
    }));
    localStorage.setItem('1agents-active-workspace', session.workspaceId);
    activeSession.value = { ...session, active: true };
    wsStore.folders.value =
        session.workspaceId !== oldWorkspaceId
            ? updatedFolders.map(f => (f.id === session.workspaceId ? { ...f, expanded: true } : f))
            : updatedFolders;
    wsStore.activeWorkspaceId.value = session.workspaceId;
    tabsStore.activeTabId.value = 'terminal';
    // Chat sessions live in the agents tab; terminals in the terminal tab.
    tabsStore.activeTab.value = isChat(session) ? 'agents' : 'terminal';

    // Chat sessions don't need tmux / fs / git context switching; just
    // ensure the workspace is loaded and we're done.
    if (isChat(session)) {
        if (session.workspaceId !== oldWorkspaceId) {
            const ws = workspaces.find(w => w.id === session.workspaceId);
            if (ws) await fs.switchFsContext(ws);
        }
        loadChatSessions(session.workspaceId);
        if (ui.isMobile.value) ui.leftSidebarOpen.value = false;
        return;
    }

    // Helper to perform the actual terminal window and workspace context switching
    const performSwitch = async () => {
        // Always switch the tmux window first
        await switchTerminal((session as Extract<Session, { kind: 'terminal' }>).index);

        if (session.workspaceId !== oldWorkspaceId) {
            wsStore.loadCcConnectUrl(session.workspaceId);
            wsStore.loadCcProvidersUrl(session.workspaceId);
            // Switch backend context and reload file browser / git panel
            const ws = workspaces.find(w => w.id === session.workspaceId);
            if (ws) {
                await fs.switchFsContext(ws);
                ui.showToast(t('app.toast.workspaceSwitched', ui.language.value, { name: ws.name }));
            }
        }
    };

    if (ui.isMobile.value) {
        // Close sidebar immediately on mobile for instant visual response
        ui.leftSidebarOpen.value = false;
        // Delay the heavy backend connection operations by 200ms to let the slide-out CSS transition finish smoothly without main-thread jank
        setTimeout(performSwitch, 200);
    } else {
        // Desktop: switch immediately
        await performSwitch();
    }
};

/** Submit handler for the session rename modal. */
export const submitRenameSession = async () => {
    const sessionRenameTarget = modal.sessionRenameTarget.value;
    if (!sessionRenameTarget) return;
    const trimmed = modal.sessionRenameName.value.trim();
    try {
        await terminalService.rename(sessionRenameTarget.id, trimmed);
        modal.closeSessionRenameModal();
        await loadTerminals();
        ui.showToast(t('app.toast.sessionRenamed', ui.language.value));
    } catch (err) {
        ui.showToast(t('app.toast.sessionRenameFailed', ui.language.value, { err: String(err) }));
    }
};
