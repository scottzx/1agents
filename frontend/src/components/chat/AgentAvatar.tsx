import { h } from 'preact';
import type { AgentType } from '../types';
import claudeLogo from '../../assets/harness-logos/claude-code-logo.svg';
import codexLogo from '../../assets/harness-logos/codex-logo.svg';
import cursorLogo from '../../assets/harness-logos/cursor-logo.svg';
import openclawLogo from '../../assets/harness-logos/openclaw-logo.svg';
import opencodeLogo from '../../assets/harness-logos/opencode-logo.svg';

/** Visual run-status keys for the composite agent avatar indicator. */
export type AgentRunStatus = 'none' | 'idle' | 'busy' | 'waiting' | 'error' | 'shell' | 'transparent';

interface AgentAvatarProps {
    /** Known agent type, or a free string (e.g. a terminal's detected agent). */
    agentType: AgentType | string;
    class?: string;
    title?: string;
    /**
     * Agent run status — renders a corner indicator light on the logo.
     * Accepts wire/live values (`streaming`, `awaiting_permission`, …) which
     * are normalized. Omit to hide the indicator (e.g. create-session preview).
     */
    status?: string | null;
}

const AGENT_LOGOS: Record<string, string> = {
    claudecode: claudeLogo,
    codex: codexLogo,
    cursor: cursorLogo,
    opencode: opencodeLogo,
    openclaw: openclawLogo,
};

/**
 * Normalize wire/live status strings into avatar indicator keys.
 * Chat: idle | streaming | awaiting_permission | error
 * Terminal: none | idle | busy | waiting | shell
 */
export function normalizeAgentStatus(status?: string | null): AgentRunStatus | undefined {
    if (status === null || status === undefined || status === '') return undefined;
    switch (status) {
        case 'streaming':
            return 'busy';
        case 'awaiting_permission':
            return 'waiting';
        case 'none':
        case 'idle':
        case 'busy':
        case 'waiting':
        case 'error':
        case 'shell':
        case 'transparent':
            return status;
        default:
            return 'none';
    }
}

export function AgentAvatar({ agentType, class: className, title, status }: AgentAvatarProps) {
    const logoSrc = AGENT_LOGOS[agentType];
    const runStatus = normalizeAgentStatus(status);
    const classes = ['agent-avatar', className].filter(Boolean).join(' ');
    const statusEl =
        runStatus !== undefined ? (
            <span class={`agent-avatar-status agent-avatar-status--${runStatus}`} aria-hidden="true" />
        ) : null;

    if (!logoSrc) {
        // Fallback: render first two letters in uppercase
        const label = agentType.slice(0, 2).toUpperCase();
        return (
            <span class={classes} title={title} aria-hidden="true">
                <span class="agent-avatar-fallback">{label}</span>
                {statusEl}
            </span>
        );
    }

    return (
        <span class={classes} title={title} aria-hidden="true">
            <img class="agent-avatar-logo" src={logoSrc} alt={agentType} />
            {statusEl}
        </span>
    );
}
