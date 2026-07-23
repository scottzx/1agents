import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';

import { t, type Lang } from '../../i18n';
import type { Workspace, AgentType } from '../types';
import { SessionSetupForm, type SessionSetupFormValues, type TeamMemberOption } from './SessionSetupForm';
import { ONESHOT_WORKSPACE_ID } from './WorkspaceScopePicker';
import { soulService } from '@1agents/core/services/soulService';

/**
 * Modal wrapper around SessionSetupForm (chat-only new session).
 * Terminal is created from the sidebar as a bare tmux pane.
 */

interface SessionSetupModalProps {
    workspaces: Workspace[];
    defaultWorkspaceId: string;
    defaultAgent?: AgentType;
    locked?: boolean;
    workspaceName?: string;
    initialAgentRef?: string;
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
    initialAgentRef,
    language,
    onCancel,
    onSubmit,
}: SessionSetupModalProps) {
    const [teamMembers, setTeamMembers] = useState<TeamMemberOption[]>([]);
    const [formWorkspaceId, setFormWorkspaceId] = useState(defaultWorkspaceId);

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

    useEffect(() => {
        const wsId = locked ? defaultWorkspaceId : formWorkspaceId || defaultWorkspaceId;
        let cancelled = false;
        if (!wsId || wsId === ONESHOT_WORKSPACE_ID) {
            setTeamMembers([]);
            return;
        }
        soulService
            .getWorkspaceTeam(wsId)
            .then(team => {
                if (!cancelled) {
                    setTeamMembers(team.members.map(m => ({ file: m.file, name: m.name })));
                }
            })
            .catch(() => {
                if (!cancelled) setTeamMembers([]);
            });
        return () => {
            cancelled = true;
        };
    }, [locked, defaultWorkspaceId, formWorkspaceId]);

    const title = t('modal.sessionSetup.title', language);
    const subtitle = workspaceName ? ` · ${workspaceName}` : '';

    return (
        <div class="ws-modal-overlay session-setup-overlay" onClick={onCancel}>
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
                        initialAgentRef={initialAgentRef}
                        teamMembers={teamMembers}
                        language={language}
                        onCancel={onCancel}
                        onWorkspaceChange={id => setFormWorkspaceId(id)}
                        onSubmit={onSubmit}
                    />
                </div>
            </div>
        </div>
    );
}
