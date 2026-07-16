import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';
import type { Workspace } from '../types';
import { SessionSetupForm, type SessionSetupFormValues } from '../chat/SessionSetupForm';

interface SessionSetupModalProps {
    workspaces: Workspace[];
    /** Locked workspace id (picker hidden when set). Empty = unlocked picker. */
    workspaceId?: string;
    /** Workspace default agent, used to seed the form. */
    defaultAgent?: import('../types').AgentType;
    onClose: () => void;
    onSubmit: (values: SessionSetupFormValues) => void;
    language: Lang;
}

/**
 * Thin overlay wrapper around SessionSetupForm for the unified "new session"
 * entry (P0-2 / P0-3 of 统一新建会话). Replaces the legacy SessionCreateModal;
 * no role / permission fields — mode + agent|preset + name only.
 *
 * `workspaceId` controls both the picker visibility (locked when set) and the
 * default cwd. Submit closes the modal — the parent decides whether to build
 * a chat or terminal session from the returned values.
 */
export class SessionSetupModal extends Component<SessionSetupModalProps> {
    render() {
        const { workspaces, workspaceId, defaultAgent, onClose, onSubmit, language } = this.props;
        const locked = !!workspaceId;
        const effectiveWorkspaceId = workspaceId || workspaces[0]?.id || '';

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal session-setup-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('modal.session.setupTitle', language)}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <SessionSetupForm
                            workspaces={workspaces}
                            defaultWorkspaceId={effectiveWorkspaceId}
                            locked={locked}
                            defaultAgent={defaultAgent}
                            language={language}
                            onSubmit={values => onSubmit(values)}
                            onCancel={onClose}
                        />
                    </div>
                </div>
            </div>
        );
    }
}
