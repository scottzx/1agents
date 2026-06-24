import { h } from 'preact';
import type { AgentType } from '../types';
import { RINGED_ROLES, type SessionRole } from '../types';
import claudeLogo from '../../assets/harness-logos/claude-code-logo.svg';
import codexLogo from '../../assets/harness-logos/codex-logo.svg';
import cursorLogo from '../../assets/harness-logos/cursor-logo.svg';
import openclawLogo from '../../assets/harness-logos/openclaw-logo.svg';
import opencodeLogo from '../../assets/harness-logos/opencode-logo.svg';

interface AgentAvatarProps {
    /** Known agent type, or a free string (e.g. a terminal's detected agent). */
    agentType: AgentType | string;
    class?: string;
    title?: string;
    /**
     * Conversation role — adds a colored ring classifying the role (PMO /
     * PM / Executor / Verifier). Baseline ('general' / empty) gets no ring.
     */
    role?: string;
}

/** Maps a role string to its avatar ring modifier class (or '' for baseline). */
function roleRingClass(role?: string): string {
    return role && RINGED_ROLES.includes(role as SessionRole) ? `agent-avatar--role-${role}` : '';
}

const AGENT_LOGOS: Record<string, string> = {
    claudecode: claudeLogo,
    codex: codexLogo,
    cursor: cursorLogo,
    opencode: opencodeLogo,
    openclaw: openclawLogo,
};

export function AgentAvatar({ agentType, class: className, title, role }: AgentAvatarProps) {
    const logoSrc = AGENT_LOGOS[agentType];
    const classes = ['agent-avatar', roleRingClass(role), className].filter(Boolean).join(' ');

    if (!logoSrc) {
        // Fallback: render first two letters in uppercase
        const label = agentType.slice(0, 2).toUpperCase();
        return (
            <span class={classes} title={title} aria-hidden="true">
                <span class="agent-avatar-fallback">{label}</span>
            </span>
        );
    }

    return (
        <span class={classes} title={title} aria-hidden="true">
            <img class="agent-avatar-logo" src={logoSrc} alt={agentType} />
        </span>
    );
}
