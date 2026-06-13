import { h } from 'preact';
import { Session, isChat } from '../types';
import { t, type Lang } from '../i18n';
import { AgentAvatar } from '../chat/AgentAvatar';

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
    /** Pointer is over this row — gates the terminal rename action. */
    isHovered: boolean;
    /** Task id -> title map, used by the chat task badge tooltip/label. */
    taskTitles: Record<string, string>;
    language: Lang;
    onSelect: (session: Session) => void;
    onKill: (e: MouseEvent, session: Session) => void;
    onRename: (session: Session) => void;
    onHoverChange: (id: string | null) => void;
}

// Terminal `agent` values come from backend detection ('claude', 'codex',
// 'antigravity', …); map the ones whose name differs from the AgentAvatar
// logo key. Anything else is passed through (AgentAvatar falls back to a
// two-letter badge for unknown agents).
const TERM_AGENT_LOGO_KEY: Record<string, string> = {
    claude: 'claudecode',
};

const CloseIcon = () => (
    <svg
        width="12"
        height="12"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
    >
        <line x1="18" x2="6" y1="6" y2="18" />
        <line x1="6" x2="18" y1="6" y2="18" />
    </svg>
);

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

/**
 * Unified sidebar session row. A single `.chat-item` template renders both chat
 * and terminal sessions with the same shape — agent avatar + title + trailing
 * status dot + close button. The `session.kind` discriminator only selects the
 * leading icon source, the status palette, and the terminal-only rename action.
 */
export function SessionRow({
    session,
    selected,
    killing,
    isHovered,
    taskTitles,
    language,
    onSelect,
    onKill,
    onRename,
    onHoverChange,
}: SessionRowProps) {
    const chat = isChat(session);
    const chatFallback = t('sidebar.chatSession', language) || '聊天会话';

    // Leading icon: agent avatar for chat and for agent-backed terminals;
    // a generic terminal glyph when the terminal has no detected agent.
    let leadingIcon;
    if (chat) {
        leadingIcon = <AgentAvatar agentType={session.agentType} class="chat-sidebar-avatar" title={chatFallback} />;
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
    // colour palette differs (chat statuses vs. terminal `term-*` palette).
    const statusClass = chat ? `chat-status-dot ${session.status}` : `chat-status-dot term-${session.status || 'none'}`;

    return (
        <div
            class={`chat-item chat-row-kind-${session.kind} ${selected ? 'active' : ''}${
                killing ? ' chat-item-killing' : ''
            }`}
            onClick={(e: MouseEvent) => {
                e.stopPropagation();
                onSelect(session);
            }}
            onMouseEnter={() => onHoverChange(session.id)}
            onMouseLeave={() => onHoverChange(null)}
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

            <div class="session-actions">
                {!chat && isHovered && (
                    <button
                        class="session-action-btn"
                        title={t('sidebar.renameSession', language)}
                        onClick={(e: MouseEvent) => {
                            e.stopPropagation();
                            onRename(session);
                        }}
                    >
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
                            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                        </svg>
                    </button>
                )}
                <button
                    class="session-kill-btn"
                    title={t('sidebar.closeSession', language)}
                    onClick={(e: MouseEvent) => onKill(e, session)}
                >
                    <CloseIcon />
                </button>
            </div>
        </div>
    );
}
