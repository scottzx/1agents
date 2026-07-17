import { h } from 'preact';
import { t, type Lang } from '../../i18n';
import * as sess from '../../stores/sessionStore';
import * as wsStore from '../../stores/workspaceStore';

interface Props {
    language: Lang;
}

/**
 * Shown in the terminal pane when no real terminal windows are open. The
 * backend keeps a hidden anchor window (tmux index 0, a root shell created at
 * startup) alive to hold the tmux session, so the user can close every real
 * terminal (index ≥ 1) without ever destroying the session. With no real
 * terminal left, ttyd would otherwise expose that anchor's bare shell — this
 * empty state replaces the live terminal view until the user opens a new one.
 */
export function TerminalEmptyState({ language }: Props) {
    const createTerminal = () => {
        const wsId = wsStore.activeWorkspaceId.value;
        if (!wsId) return;
        const ws = wsStore.workspaces.value.find(w => w.id === wsId);
        // Bare tmux pane — same path as sidebar「新建终端」, no preset picker.
        void sess.createTerminal(wsId, ws?.terminalDir || ws?.path || '.');
    };

    return (
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
                <rect width="20" height="16" x="2" y="4" rx="2" />
                <path d="m7 8 3 2-3 2" />
                <path d="M12 12h4" />
            </svg>
            <h3 class="placeholder-title">{t('canvas.noTerminal', language)}</h3>
            <p class="placeholder-desc">{t('canvas.noTerminalDesc', language)}</p>
            <button
                style="margin-top: 16px; padding: 8px 16px; border: none; border-radius: 8px; cursor: pointer; font-size: 13px; background: var(--accent-emphasis); color: var(--on-accent);"
                onClick={createTerminal}
            >
                {t('canvas.createTerminal', language)}
            </button>
        </div>
    );
}
