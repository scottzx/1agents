import { h } from 'preact';
import type { AgentType } from '../types';
import claudeLogo from '../../assets/harness-logos/claude-code-logo.svg';
import codexLogo from '../../assets/harness-logos/codex-logo.svg';
import cursorLogo from '../../assets/harness-logos/cursor-logo.svg';
import openclawLogo from '../../assets/harness-logos/openclaw-logo.svg';
import opencodeLogo from '../../assets/harness-logos/opencode-logo.svg';

interface AgentAvatarProps {
    agentType: AgentType;
    class?: string;
    title?: string;
}

const AGENT_LOGOS: Record<string, string> = {
    claudecode: claudeLogo,
    codex: codexLogo,
    cursor: cursorLogo,
    opencode: opencodeLogo,
    openclaw: openclawLogo,
};

export function AgentAvatar({ agentType, class: className, title }: AgentAvatarProps) {
    const logoSrc = AGENT_LOGOS[agentType];
    const classes = ['agent-avatar', className].filter(Boolean).join(' ');

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
