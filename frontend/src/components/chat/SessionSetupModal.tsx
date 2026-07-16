import { h } from 'preact';
import { useEffect } from 'preact/hooks';

import { t, type Lang } from '../../i18n';
import type { Workspace, AgentType } from '../types';
import { SessionSetupForm, type SessionSetupFormValues, type TerminalPresetBin } from './SessionSetupForm';

/**
 * Modal wrapper around SessionSetupForm (P0-2 of 统一新建会话). The form is
 * reusable for the NewChatHome config panel; this wrapper owns the overlay,
 * header, and dismiss (overlay-click, ✕ button, Escape).
 *
 * Wire-shape notes:
 *  - `workspaceName` is only used to compose the header; pass '' when the
 *    user picked the workspace from the form's own row (no separate lookup).
 *  - `onSubmit` receives the form values; the ModalHost#submit branch in
 *    #113 is responsible for forwarding chat → createChatSession / term →
 *    createTerminal.
 */

interface SessionSetupModalProps {
    workspaces: Workspace[];
    defaultWorkspaceId: string;
    defaultAgent: AgentType;
    /** When true, the workspace row is hidden (locked context). */
    locked?: boolean;
    /** Optional header suffix (e.g. workspace name) — empty string omits it. */
    workspaceName?: string;
    initialTerminalPreset?: TerminalPresetBin;
    language: Lang;
    onCancel: () => void;
    onSubmit: (values: SessionSetupFormValues) => void;
}

export function SessionSetupModal({
    workspaces,
    defaultWorkspaceId,
    defaultAgent,
    locked = false,
    workspaceName = '',
    initialTerminalPreset,
    language,
    onCancel,
    onSubmit,
}: SessionSetupModalProps) {
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                e.stopPropagation();
                onCancel();
            }
        };
        document.addEventListener('keydown', onKey, true);
        return () => document.removeEventListener('keydown', onKey, true);
    }, [onCancel]);

    const title = t('modal.sessionSetup.title', language);
    const subtitle = workspaceName ? ` · ${workspaceName}` : '';

    return (
        <div class="ws-modal-overlay" onClick={onCancel}>
            <div class="ws-modal session-setup-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>
                        {title}
                        {subtitle}
                    </span>
                    <button class="ws-modal-close" onClick={onCancel} aria-label={t('common.close', language)}>
                        ✕
                    </button>
                </div>
                <div class="ws-modal-body">
                    <SessionSetupForm
                        workspaces={workspaces}
                        defaultWorkspaceId={defaultWorkspaceId}
                        locked={locked}
                        defaultAgent={defaultAgent}
                        initialTerminalPreset={initialTerminalPreset}
                        language={language}
                        onCancel={onCancel}
                        onSubmit={onSubmit}
                    />
                </div>
            </div>
        </div>
    );
}
