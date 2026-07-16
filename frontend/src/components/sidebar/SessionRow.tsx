import { h } from 'preact';
import { Session, isChat } from '../types';
import { t, type Lang } from '../i18n';
import { AgentAvatar, normalizeAgentStatus } from '../chat/AgentAvatar';
import { liveSessionStatus, requestForkSession, requestDeleteSession } from '../../stores/sessionStore';
import { FsRowActionsMenu, type FsRowAction } from '../drawer/FsRowActionsMenu';

interface SessionRowProps {
    /** Session to render. `kind` ('chat' | 'terminal') drives the type-specific bits. */
    session: Session;
    /**
     * Whether this row is the user's current selection. Single source of
     * truth for the highlight — derived from `activeSession` identity, NOT
     * from the per-session `active` flag (the tmux backend keeps exactly one
     * window flagged `active`, which would otherwise fight chat selection).
     */
    selected: boolean;
    /** Currently collapsing (kill animation) — adds `chat-item-killing`. */
    killing: boolean;
    /** Task id -> title map, used by the chat task badge tooltip/label. */
    taskTitles: Record<string, string>;
    language: Lang;
    onSelect: (session: Session) => void;
    onKill: (e: MouseEvent, session: Session) => void;
    onRename: (session: Session) => void;
}

// Terminal `agent` values come from backend detection ('claude', 'codex',
// 'antigravity', …); map the ones whose name differs from the AgentAvatar
// logo key. Anything else is passed through (AgentAvatar falls back to a
// two-letter badge for unknown agents).
const TERM_AGENT_LOGO_KEY: Record<string, string> = {
    claude: 'claudecode',
};

const TerminalGlyph = () => (
    <svg
        width="12"
        height="12"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
    >
        <polyline points="4 17 10 11 4 5" />
        <line x1="12" y1="19" x2="20" y2="19" />
    </svg>
);

// 12px SVG icons used by the chat/terminal "..." menu (重命名 / 归档).
const SESSION_ACTION_ICONS = {
    rename: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
        </svg>
    ),
    archive: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="21 8 21 21 3 21 3 8" />
            <rect x="1" y="3" width="22" height="5" />
            <line x1="10" y1="12" x2="14" y2="12" />
        </svg>
    ),
    fork: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="18" cy="18" r="3" />
            <circle cx="6" cy="6" r="3" />
            <circle cx="6" cy="18" r="3" />
            <path d="M18 15V9a4 4 0 0 0-4-4H9" />
            <line x1="6" y1="9" x2="6" y2="15" />
        </svg>
    ),
    delete: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="3 6 5 6 21 6" />
            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
            <line x1="10" y1="11" x2="10" y2="17" />
            <line x1="14" y1="11" x2="14" y2="17" />
        </svg>
    ),
    switch: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="10" />
            <polyline points="12 16 16 12 12 8" />
            <line x1="8" y1="12" x2="16" y2="12" />
        </svg>
    ),
};

/** Build the "..." dropdown items for a session row: rename + fork + delete.
 *  Archive/close is a dedicated button to the left of "..." (high-frequency). */
function buildSessionActions(session: Session, props: SessionRowProps): FsRowAction[] {
    const actions: FsRowAction[] = [
        {
            id: 'rename',
            labelKey: 'sidebar.renameSession',
            icon: SESSION_ACTION_ICONS.rename,
            onSelect: () => props.onRename(session),
        },
    ];

    if (isChat(session) && session.forkSupported === true) {
        actions.push({
            id: 'fork',
            labelKey: 'sidebar.forkSession',
            icon: SESSION_ACTION_ICONS.fork,
            onSelect: () => requestForkSession(session.id),
        });
    }

    if (isChat(session)) {
        actions.push({
            id: 'delete',
            labelKey: 'sidebar.deleteSession',
            icon: SESSION_ACTION_ICONS.delete,
            danger: true,
            onSelect: () => {
                const isZh = props.language === 'zh-CN';
                const msg = isZh
                    ? '确定要永久删除该会话吗？此操作不可撤销。'
                    : 'Are you sure you want to permanently delete this session? This action cannot be undone.';
                if (window.confirm(msg)) {
                    requestDeleteSession(session.id);
                }
            },
        });
    }

    /*
    actions.push({
        id: 'switch',
        labelKey: 'sidebar.switchSession',
        icon: SESSION_ACTION_ICONS.switch,
        onSelect: () => props.onSelect(session),
    });
    */

    return actions;
}

/**
 * Unified sidebar session row. A single `.chat-item` template renders both chat
 * and terminal sessions with the same shape — agent avatar (with run-status
 * indicator) + title + "..." actions menu. The `session.kind` discriminator
 * Archive/close is a dedicated button (high-frequency); rename / fork / delete
 * stay in the "..." dropdown.
 */
export function SessionRow({
    session,
    selected,
    killing,
    taskTitles,
    language,
    onSelect,
    onKill,
    onRename,
}: SessionRowProps) {
    const chat = isChat(session);
    const chatFallback = t('sidebar.chatSession', language) || '聊天会话';

    // Live bridge status (streaming / awaiting_permission) overrides the stale
    // persisted snapshot; reading the signal's .value here subscribes the row
    // so it repaints the moment the bridge publishes a change.
    const liveStatus = chat ? liveSessionStatus.value[session.id] : undefined;
    const effectiveStatus = liveStatus ?? session.status;
    // Brand-new chat sessions (idle + no acpSessionId) show a hollow indicator.
    const avatarStatus =
        chat && effectiveStatus === 'idle' && !session.acpSessionId
            ? 'none'
            : chat
              ? String(effectiveStatus ?? 'none')
              : String(session.status || 'none');
    const statusKey = normalizeAgentStatus(avatarStatus) ?? 'none';

    // Leading icon: agent avatar (composite status indicator) for chat and
    // agent-backed terminals; generic terminal glyph when no agent detected.
    let leadingIcon;
    if (chat) {
        leadingIcon = (
            <AgentAvatar
                agentType={session.agentType}
                status={avatarStatus}
                class="chat-sidebar-avatar"
                title={chatFallback}
            />
        );
    } else if (session.agent) {
        leadingIcon = (
            <AgentAvatar
                agentType={TERM_AGENT_LOGO_KEY[session.agent] || session.agent}
                status={avatarStatus}
                class="chat-sidebar-avatar"
                title={session.agent}
            />
        );
    } else {
        leadingIcon = (
            <span class="chat-sidebar-avatar chat-terminal-icon" aria-hidden="true">
                <TerminalGlyph />
                <span class={`agent-avatar-status agent-avatar-status--${statusKey}`} />
            </span>
        );
    }

    return (
        <div
            class={`chat-item chat-row-kind-${session.kind} ${selected ? 'active' : ''}${
                killing ? ' chat-item-killing' : ''
            }`}
            onClick={(e: MouseEvent) => {
                e.stopPropagation();
                onSelect(session);
            }}
        >
            <div class="chat-item-left">
                {leadingIcon}
                <span class="chat-title" title={session.name}>
                    {session.name || (chat ? chatFallback : '')}
                </span>
                {chat && session.taskId && (
                    <span class="chat-task-badge" title={`任务: ${taskTitles[session.taskId] || session.taskId}`}>
                        {'\u{1F4CB}'}
                        {taskTitles[session.taskId] && (
                            <span class="chat-task-badge-title">{taskTitles[session.taskId]}</span>
                        )}
                    </span>
                )}
            </div>

            <div class="chat-actions" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <button
                    type="button"
                    class="fb-row-action-btn chat-actions-archive"
                    title={
                        isChat(session) ? t('sidebar.archiveSession', language) : t('sidebar.closeSession', language)
                    }
                    aria-label={
                        isChat(session) ? t('sidebar.archiveSession', language) : t('sidebar.closeSession', language)
                    }
                    onClick={(e: MouseEvent) => onKill(e, session)}
                >
                    {SESSION_ACTION_ICONS.archive}
                </button>
                <FsRowActionsMenu
                    entry={session as unknown as { path?: string }}
                    items={buildSessionActions(session, {
                        session,
                        selected,
                        killing,
                        taskTitles,
                        language,
                        onSelect,
                        onKill,
                        onRename,
                    })}
                    language={language}
                    triggerClassName="chat-actions-trigger"
                />
            </div>
        </div>
    );
}
