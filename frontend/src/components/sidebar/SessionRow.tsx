import { h } from 'preact';
import { Session, isChat } from '../types';
import { t, type Lang } from '../i18n';
import { AgentAvatar } from '../chat/AgentAvatar';
import { liveSessionStatus } from '../../stores/sessionStore';
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

const TerminalIcon = () => (
    <span class="chat-sidebar-avatar chat-terminal-icon" aria-hidden="true">
        <svg
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
        </svg>
    </span>
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
};

/** Build the "..." dropdown items for a session row: rename + archive. */
function buildSessionActions(session: Session, props: SessionRowProps): FsRowAction[] {
    return [
        {
            id: 'rename',
            labelKey: 'sidebar.renameSession',
            icon: SESSION_ACTION_ICONS.rename,
            onSelect: () => props.onRename(session),
        },
        {
            id: 'archive',
            labelKey: isChat(session) ? 'sidebar.archiveSession' : 'sidebar.closeSession',
            icon: SESSION_ACTION_ICONS.archive,
            danger: true,
            onSelect: () => {
                // Synthesize a MouseEvent so the existing onKill handler (which
                // expects the stopPropagation + animation-timer pattern) works
                // unchanged. The event is never dispatched — we just need a
                // value to pass.
                const ev = { stopPropagation: () => {} } as MouseEvent;
                props.onKill(ev, session);
            },
        },
    ];
}

/**
 * Unified sidebar session row. A single `.chat-item` template renders both chat
 * and terminal sessions with the same shape — agent avatar + title + trailing
 * status dot + "..." actions menu. The `session.kind` discriminator only
 * selects the leading icon source and the status palette; the rename / archive
 * actions are shared and accessed via the dropdown triggered by the trailing
 * `.chat-actions-trigger` button.
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

    // Leading icon: agent avatar for chat and for agent-backed terminals;
    // a generic terminal glyph when the terminal has no detected agent.
    let leadingIcon;
    if (chat) {
        leadingIcon = (
            <AgentAvatar
                agentType={session.agentType}
                role={session.role}
                class="chat-sidebar-avatar"
                title={chatFallback}
            />
        );
    } else if (session.agent) {
        leadingIcon = (
            <AgentAvatar
                agentType={TERM_AGENT_LOGO_KEY[session.agent] || session.agent}
                class="chat-sidebar-avatar"
                title={session.agent}
            />
        );
    } else {
        leadingIcon = <TerminalIcon />;
    }

    // Trailing status dot — same element/position for both kinds, only the
    // colour palette differs (chat `chat-*` palette vs. terminal `term-*` palette).
    // For chat sessions: a brand-new session (idle + no lastEventAt) shows as
    // hollow ring (chat-none); once it has activity, idle → blue "completed" dot.
    const CHAT_STATUS_CLASS: Record<string, string> = {
        idle: 'chat-idle',
        streaming: 'chat-busy',
        awaiting_permission: 'chat-waiting',
        error: 'chat-error',
    };
    // Live bridge status (streaming / awaiting_permission) overrides the stale
    // persisted snapshot; reading the signal's .value here subscribes the row
    // so it repaints the moment the bridge publishes a change.
    const liveStatus = chat ? liveSessionStatus.value[session.id] : undefined;
    const effectiveStatus = liveStatus ?? session.status;
    const rawStatus = String(effectiveStatus ?? '');
    const chatStatus =
        chat && effectiveStatus === 'idle' && !session.acpSessionId
            ? 'chat-none'
            : CHAT_STATUS_CLASS[rawStatus] ?? `chat-${rawStatus}`;
    const statusClass = chat ? `chat-status-dot ${chatStatus}` : `chat-status-dot term-${session.status || 'none'}`;

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

            <span class={statusClass} />

            <div class="chat-actions" onClick={(e: MouseEvent) => e.stopPropagation()}>
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
