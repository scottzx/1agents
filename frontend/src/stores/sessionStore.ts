import { signal } from '@preact/signals';

import {
    isChat,
    isTerminal,
    isFullPageTab,
    type TmuxWindow,
    type Session,
    type ChatSession,
    type ChatStatus,
    type AgentType,
} from '../components/types';
import type { AuthState, ConnectionState } from '@1agents/core/protocol/types';
import { terminalService } from '../services/terminalService';
import { agentService } from '../services/agentService';
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

/**
 * When the new-chat landing is opened from a specific assistant (助理 详情 的
 * 「新建对话」), it is locked to that assistant's workspace — NewChatHome hides
 * its workspace picker and the header breadcrumb shows 助理 › <name> › 新建对话.
 * null = the general cross-project new-chat (picker shown). Set by
 * AssistantDetail after onStartNewChat; cleared by every general onStartNewChat.
 */
export const lockedNewChatWorkspaceId = signal<string | null>(null);

/**
 * Live, runtime-only status overrides keyed by session id. The persisted
 * `ChatSession.status` (from the list API) is a stale snapshot; the chat
 * bridge (globalBridgeManager) pushes the *current* transient state here as
 * events stream in — `streaming` while a turn runs, `awaiting_permission`
 * while a permission bubble is pending — so the sidebar dot reflects what's
 * actually happening. A session with no entry (or `undefined`) falls back to
 * its persisted status. Only the currently-rendered session(s) have a live
 * bridge, so only those get overrides; backgrounded sessions keep their
 * snapshot until reselected.
 */
export const liveSessionStatus = signal<Record<string, ChatStatus>>({});

/** Set or clear a session's live status override (no-op when unchanged). */
export const setLiveSessionStatus = (sessionId: string, status: ChatStatus | null) => {
    const cur = liveSessionStatus.value[sessionId];
    if (status === null || status === undefined) {
        if (cur === undefined) return;
        const next = { ...liveSessionStatus.value };
        delete next[sessionId];
        liveSessionStatus.value = next;
        return;
    }
    if (cur === status) return;
    liveSessionStatus.value = { ...liveSessionStatus.value, [sessionId]: status };
};

/**
 * Live WebSocket connection state keyed by session id. Mirrors what the chat
 * bridge tracks internally (idle/connecting/connected/reconnecting/closed/
 * error) so the workspace header can show the active session's connection
 * status — the chat status bar used to own this, but it now lives in the
 * header. Only sessions with a live bridge get an entry; cleared on destroy.
 */
export const liveSessionConnection = signal<Record<string, ConnectionState>>({});

/** Set or clear a session's live connection state (no-op when unchanged). */
export const setLiveSessionConnection = (sessionId: string, conn: ConnectionState | null) => {
    const cur = liveSessionConnection.value[sessionId];
    if (conn === null) {
        if (cur === undefined) return;
        const next = { ...liveSessionConnection.value };
        delete next[sessionId];
        liveSessionConnection.value = next;
        return;
    }
    if (cur === conn) return;
    liveSessionConnection.value = { ...liveSessionConnection.value, [sessionId]: conn };
};

/**
 * Live auth state keyed by session id. Mirrored from the chat bridge so the
 * ChatHeader badge can render even when the user has backgrounded the chat
 * (no active useBridge listener). `null` = bridge hasn't spoken (UI hides the
 * badge entirely, so agents that never require auth add zero visual noise).
 */
export const liveSessionAuthState = signal<Record<string, AuthState>>({});

/** Set or clear a session's live auth state (no-op when unchanged). */
export const setLiveSessionAuthState = (sessionId: string, auth: AuthState | null) => {
    const cur = liveSessionAuthState.value[sessionId];
    if (auth === null) {
        if (cur === undefined) return;
        const next = { ...liveSessionAuthState.value };
        delete next[sessionId];
        liveSessionAuthState.value = next;
        return;
    }
    if (cur === auth) return;
    liveSessionAuthState.value = { ...liveSessionAuthState.value, [sessionId]: auth };
};

/**
 * Session id to flash-highlight for ~1.5s after a fork succeeds. Sidebar rows
 * read this signal to add the `.chat-item-highlight` class; the class itself
 * carries the animation + auto-clear, so callers only need to set the id and
 * the row's CSS handles the rest. Cleared by `clearHighlightedSession` when
 * the animation ends or the user navigates away.
 */
export const highlightedSessionId = signal<string | null>(null);
let highlightTimer: ReturnType<typeof setTimeout> | null = null;

/** Mark `sessionId` as the freshly-forked row; auto-clears after 1.5s. */
export const setHighlightedSession = (sessionId: string) => {
    if (highlightTimer) {
        clearTimeout(highlightTimer);
        highlightTimer = null;
    }
    highlightedSessionId.value = sessionId;
    highlightTimer = setTimeout(() => {
        highlightedSessionId.value = null;
        highlightTimer = null;
    }, 1500);
};

/**
 * Bridge-side session list cache, keyed by workspace id. Populated lazily by
 * `requestSessionsList` (sidebar "Switch Session" popover) and stale-cleared
 * 5s after the last request so repeated opens don't hammer the bridge. The
 * popover reads `sessionListByWorkspace[wsId]`; missing entry = nothing
 * fetched yet.
 */
export const sessionListByWorkspace = signal<Record<string, ChatSession[]>>({});
export const SESSION_LIST_TTL_MS = 5_000;
const sessionListFetchedAt: Record<string, number> = {};

/** Sync tmux windows + chat sessions into workspace folders as sessions */
export const mergeSessionsIntoFolders = (windows: TmuxWindow[], chats: ChatSession[]) => {
    // Always keep the currently-active chat session in the list, even when the
    // backend index doesn't return it. A session opened from a task timeline
    // (TaskDetail.openSession) may have no index record; without this it would
    // connect the bridge + load history but never appear in the sidebar, and a
    // subsequent loadChatSessions would even wipe it out of activeSession below.
    // Guard: never re-inject an archived session — it was intentionally removed.
    const prevActive = activeSession.value;
    const chatList: ChatSession[] =
        prevActive && isChat(prevActive) && !prevActive.archived && !chats.some(c => c.id === prevActive.id) ? [prevActive, ...chats] : chats;
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
        const chatSessionList: Session[] = chatList
            .filter(c => c.workspaceId === f.id)
            .filter(c => !c.archived)
            .map(c => ({ ...c }));
        // Chat sessions first (newer), then terminals.
        return { ...f, sessions: [...chatSessionList, ...termSessions] };
    });
    // Preserve the currently-active chat session if it still exists; otherwise
    // fall back to the most recently active terminal window.
    const activeChat = prevActive && isChat(prevActive) ? chatList.find(c => c.id === prevActive.id) : null;
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
        const chats = await agentService.list(wsId, true);
        // All chats (incl. role='pm' AI 项目经理) show in the normal sidebar /
        // chat column now — PM is created via New Conversation, not a 副屏.
        // Merge into the cross-workspace aggregate instead of replacing it, so
        // other workspaces' sessions aren't wiped (the session-first mobile
        // home lists every conversation across all projects).
        chatSessions.value = [...chatSessions.value.filter(c => c.workspaceId !== wsId), ...chats];
        mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
    } catch (err) {
        console.error('[agent] list error:', err);
    }
};

/**
 * Load chat sessions for EVERY workspace and aggregate them, so the home /
 * sidebar can show all conversations across the default workspace and all
 * projects at once. The backend has no "all sessions" endpoint, so fan out
 * one request per workspace and flatten.
 */
export const loadAllChatSessions = async () => {
    const wss = wsStore.workspaces.value;
    if (wss.length === 0) return;
    try {
        // Skip any workspace with a blank id — the backend rejects
        // workspace_id= with 400, and a blank-id row is never a real workspace.
        const lists = await Promise.all(
            wss.filter(w => w.id).map(w => agentService.list(w.id, true).catch(() => [] as ChatSession[]))
        );
        chatSessions.value = lists.flat();
        mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
    } catch (err) {
        console.error('[agent] list-all error:', err);
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
    initialMessage?: string,
    role?: string,
    permissionMode?: import('../components/types').PermissionMode,
    taskId?: string,
    agentRef?: string
) => {
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);
    if (!ws) {
        ui.showToast('工作空间不存在');
        return;
    }
    try {
        ui.showToast('正在创建聊天会话…');
        // Switch the real workspace context (terminal/fs/chat list) only now,
        // when a message is actually sent — the new-chat picker is frontend-only.
        if (wsStore.activeWorkspaceId.value !== workspaceId) {
            await wsStore.selectWorkspace(ws);
        }
        // Web 会话纯走 1acp:直接登记到 1agents 索引,不再在 cc-connect 建会话。
        const indexed = await agentService.index({
            workspace_id: workspaceId,
            name: name || `${agentType} 会话`,
            agent_type: agentType,
            role,
            permission_mode: permissionMode,
            task_id: taskId,
        });
        await loadChatSessions(workspaceId);
        // Auto-select the new session and switch to the agents tab. agentRef is a
        // transient expert pick — carried on the in-memory session so the first
        // chat-WS connect forwards it (persona is injected once, on that connect).
        activeSession.value = { ...indexed, agentRef, active: true };
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

/**
 * Open an AI Project Manager conversation for a workspace. The PM is the single
 * entry for turning ideas into work: it decides, through the dialogue, whether
 * to record a discussion card (still fuzzy) or create a requirement (clear,
 * with a deliverable). Picks the workspace's default agent and maps the role to
 * 'pmo' in the built-in default workspace (mirrors NewChatHome), so the backend
 * attaches the project-locked tasks MCP either way.
 */
export const createPMSession = async (workspaceId: string, name: string, initialMessage?: string, taskId?: string) => {
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);
    const agentType = (ws?.defaultAgent || 'claudecode') as AgentType;
    const role = workspaceId === 'default' || ws?.builtin ? 'pmo' : 'pm';
    await createChatSession(workspaceId, name, agentType, initialMessage, role, undefined, taskId);
};

export const onStartNewChat = () => {
    // A full-page drawer tab (providers/skills/discovery/settings) overrides
    // the primary pane, so without closing it the new-chat landing stays
    // hidden behind the footer panel — the "New Conversation does nothing
    // from the sidebar-footer" bug.
    if (isFullPageTab(tabsStore.activeDrawerTab.value)) {
        tabsStore.activeDrawerTab.value = 'none';
    }
    activeSession.value = null;
    // General entry → not scoped to any assistant, so the picker shows.
    lockedNewChatWorkspaceId.value = null;
    // Move the primary pane onto the new-chat landing, leaving the project
    // landing ('tasks') so the new-chat home actually renders on top.
    tabsStore.activeTabId.value = 'terminal';
    tabsStore.activeTab.value = 'new_chat';
};

export const clearPendingInitialMessage = () => {
    pendingInitialMessage.value = null;
};

/**
 * Archive a chat session: tear down the live 1acp bridge, then soft-delete the
 * 1agents index record. The conversation metadata is preserved (it drops out
 * of the sidebar but stays in the 会话 archive view, and can be reopened — the
 * bridge re-establishes from acpSessionId).
 */
export const selectNextAvailableSession = (deletedSessionId: string, workspaceId: string) => {
    const active = activeSession.value;
    if (active && isChat(active) && active.id === deletedSessionId) {
        activeSession.value = null;
        lockedNewChatWorkspaceId.value = workspaceId || null;
        tabsStore.activeTab.value = 'new_chat';
    }
};

export const killChatSession = async (sessionId: string) => {
    const session = chatSessions.value.find(c => c.id === sessionId);
    if (!session) return;
    try {
        // Clean up global WebSocket bridge session
        globalBridgeManager.destroy(sessionId);
        // Optimistic UI: mark archived locally so sidebar + project detail
        // update immediately, before the API round-trip completes.
        session.archived = true;
        session.archivedAt = new Date().toISOString();
        chatSessions.value = [...chatSessions.value];
        selectNextAvailableSession(sessionId, session.workspaceId);
        mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
        ui.showToast('会话已归档 ✓');
        // Persist to backend (fire-and-forget; the optimistic state is canonical).
        await agentService.setArchived(sessionId, true);
    } catch (err) {
        // Rollback optimistic update on failure.
        session.archived = false;
        session.archivedAt = undefined;
        chatSessions.value = [...chatSessions.value];
        mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
        ui.showToast(`归档失败: ${(err as Error).message}`);
    }
};

// ── Issue #96 block A — fork / delete / list_sessions via the bridge WS ──
// These three actions are sent through the chat WebSocket (not REST) because
// the underlying 1acp runtime owns session lifecycle and can only mutate the
// live conversation from inside its own loop. The frontend triggers them via
// `globalBridgeManager.forkSession/deleteSession/listSessions`, then waits for
// the corresponding event (`session_forked` / `session_deleted` /
// `sessions_list`) which the handlers below wire into the sessionStore.

// Normalize the wire payload the bridge returns for a fork / list call into
// the canonical ChatSession shape the sidebar expects. Defensive: backend may
// still emit the older snake_case shape during the rollout window, so accept
// both. Unknown / missing fields fall through to safe defaults — a partial
// row is still useful (the user can click it; ChatPanel will resync on its
// first history_response).
function normalizeBridgeSession(raw: Record<string, unknown>): ChatSession | null {
    if (!raw || typeof raw !== 'object') return null;
    const id = String((raw as { id?: unknown }).id ?? '');
    if (!id) return null;
    const workspaceId = String(
        (raw as { workspaceId?: unknown; workspace_id?: unknown }).workspaceId ??
            (raw as { workspace_id?: unknown }).workspace_id ??
            ''
    );
    return {
        kind: 'chat',
        id,
        workspaceId,
        taskId:
            ((raw as { taskId?: unknown; task_id?: unknown }).taskId as string | undefined) ??
            ((raw as { task_id?: unknown }).task_id as string | undefined),
        name: String((raw as { name?: unknown }).name ?? ''),
        agentType: ((raw as { agentType?: unknown; agent_type?: unknown }).agentType ??
            (raw as { agent_type?: unknown }).agent_type ??
            'claudecode') as AgentType,
        ccProject: String(
            (raw as { ccProject?: unknown; cc_project?: unknown }).ccProject ??
                (raw as { cc_project?: unknown }).cc_project ??
                ''
        ),
        ccSessionId: String(
            (raw as { ccSessionId?: unknown; cc_session_id?: unknown }).ccSessionId ??
                (raw as { cc_session_id?: unknown }).cc_session_id ??
                ''
        ),
        acpSessionId: ((raw as { acpSessionId?: unknown; acp_session_id?: unknown }).acpSessionId ??
            (raw as { acp_session_id?: unknown }).acp_session_id) as string | undefined,
        sessionKey: String(
            (raw as { sessionKey?: unknown; session_key?: unknown }).sessionKey ??
                (raw as { session_key?: unknown }).session_key ??
                ''
        ),
        status: ((raw as { status?: unknown }).status ?? 'idle') as ChatSession['status'],
        createdAt: ((raw as { createdAt?: unknown; created_at?: unknown }).createdAt ??
            (raw as { created_at?: unknown }).created_at) as string | undefined,
        lastEventAt: ((raw as { lastEventAt?: unknown; last_event_at?: unknown }).lastEventAt ??
            (raw as { last_event_at?: unknown }).last_event_at) as string | undefined,
        archivedAt: ((raw as { archivedAt?: unknown; archived_at?: unknown }).archivedAt ??
            (raw as { archived_at?: unknown }).archived_at) as string | undefined,
        archived: Boolean(
            (raw as { archived?: unknown }).archived ??
            ((raw as { archivedAt?: unknown; archived_at?: unknown }).archivedAt ??
                (raw as { archived_at?: unknown }).archived_at)
        ),
        active: Boolean((raw as { active?: unknown }).active),
        role: (raw as { role?: unknown }).role as string | undefined,
        permissionMode: ((raw as { permissionMode?: unknown; permission_mode?: unknown }).permissionMode ??
            (raw as { permission_mode?: unknown }).permission_mode) as ChatSession['permissionMode'],
    };
}

/** Send a `fork_session` action over the chat WS. The bridge answers with
 *  `session_forked` → `handleSessionForked` → sidebar row + 1.5s highlight. */
export const requestForkSession = (sessionId: string) => {
    const session = chatSessions.value.find(c => c.id === sessionId);
    if (!session) {
        ui.showToast('会话不存在');
        return;
    }
    globalBridgeManager.forkSession(session);
    ui.showToast('正在 Fork 会话…');
};

export const requestDeleteSession = (sessionId: string) => {
    const session = chatSessions.value.find(c => c.id === sessionId);
    if (!session) {
        ui.showToast('会话不存在');
        return;
    }
    // 1. Send delete session action to the bridge
    globalBridgeManager.deleteSession(session);

    // 2. Perform optimistic UI updates immediately
    chatSessions.value = chatSessions.value.filter(c => c.id !== sessionId);
    globalBridgeManager.destroy(sessionId);
    selectNextAvailableSession(sessionId, session.workspaceId);
    mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
    ui.showToast('会话已删除 ✓');
    sessionListFetchedAt[session.workspaceId] = 0;
};

/** Ask the bridge for the full session list of `workspaceId`. Cached for 5s
 *  per workspace so re-opening the Switch Session popover doesn't hammer the
 *  bridge. No-op when no chat session exists for the workspace (the popover
 *  has no parent to ride on). */
export const requestSessionsList = (workspaceId: string) => {
    const anySession = chatSessions.value.find(c => c.workspaceId === workspaceId);
    if (!anySession) {
        // Popover was opened from a terminal-only workspace — nothing to ride.
        return;
    }
    const cachedAt = sessionListFetchedAt[workspaceId];
    if (cachedAt && Date.now() - cachedAt < SESSION_LIST_TTL_MS) return;
    sessionListFetchedAt[workspaceId] = Date.now();
    globalBridgeManager.listSessions(anySession, workspaceId);
};

/** Bridge answered `session_forked` — prepend the new row, kick the
 *  highlight, and scroll it into view. The parent id is informational only
 *  (we don't navigate away from the parent). */
export const handleSessionForked = (_parentId: string, sessionRaw: unknown) => {
    const session = normalizeBridgeSession(sessionRaw as Record<string, unknown>);
    if (!session) return;
    const next = [session, ...chatSessions.value.filter(c => c.id !== session.id)];
    chatSessions.value = next;
    mergeSessionsIntoFolders(terminalWindows.value, next);
    setHighlightedSession(session.id);
    ui.showToast(`已 Fork 出新会话：${session.name || session.id} ✓`);
    // Drop the cached "Switch Session" list — it now has a new entry.
    sessionListFetchedAt[session.workspaceId] = 0;
};

/** Bridge answered `session_deleted` — drop the row, the bridge state, and
 *  any active-session pointers pointing at it. */
export const handleSessionDeleted = (sessionId: string) => {
    const session = chatSessions.value.find(c => c.id === sessionId);
    if (!session) {
        // Already gone (race with another action). Still clear the bridge
        // state to avoid a stale ghost row.
        globalBridgeManager.destroy(sessionId);
        return;
    }
    chatSessions.value = chatSessions.value.filter(c => c.id !== sessionId);
    globalBridgeManager.destroy(sessionId);
    selectNextAvailableSession(sessionId, session.workspaceId);
    mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
    ui.showToast('会话已删除 ✓');
    // Drop the cached list — row no longer exists.
    sessionListFetchedAt[session.workspaceId] = 0;
};

/** Bridge answered `sessions_list` — refresh the per-workspace cache used by
 *  the Switch Session popover. */
export const handleSessionsList = (workspaceId: string | undefined, sessionsRaw: unknown) => {
    if (!workspaceId) return;
    if (!Array.isArray(sessionsRaw)) return;
    const normalized = sessionsRaw
        .map(r => normalizeBridgeSession(r as Record<string, unknown>))
        .filter((s): s is ChatSession => !!s);
    sessionListByWorkspace.value = { ...sessionListByWorkspace.value, [workspaceId]: normalized };
};

/**
 * Create a new terminal tab via POST /api/terminal/create. When
 * initialCommand is given (e.g. `claude "..."`), the backend types it into
 * the new window's shell. After creation we switch the pane to the freshly
 * created (now active) terminal so its xterm view shows immediately.
 */
export const createTerminal = async (workspaceId: string, cwd: string, initialCommand?: string) => {
    try {
        await terminalService.create(workspaceId, cwd, initialCommand);
        await loadTerminals();
        // The backend selects the new window; loadTerminals marks it active.
        // Surface it in the main pane via the normal selection path.
        const folder = wsStore.folders.value.find(f => f.id === workspaceId);
        const newTerm = folder?.sessions.find(s => isTerminal(s) && s.active);
        if (newTerm) await selectSession(newTerm);
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
    // No "last window" guard: the backend keeps a hidden anchor window alive to
    // hold the tmux session, so any real terminal may be closed. When none
    // remain the pane shows an empty state (see ContentViewHost/MiddleCanvas).
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
    // A session opened with a transient initialMessage (issue-model follow-up /
    // new-session reply) auto-sends that prompt once ChatPanel is ready. Plain
    // switches carry none, which also clears any stale pending message.
    pendingInitialMessage.value = (isChat(session) && session.initialMessage) || null;
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
