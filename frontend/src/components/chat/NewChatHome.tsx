import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import { Workspace, type AgentType } from '../types';
import { t, type Lang } from '../../i18n';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import { SessionSetupForm, type TeamMemberOption } from './SessionSetupForm';
import { WorkspaceScopePicker, ONESHOT_WORKSPACE_ID } from './WorkspaceScopePicker';
import { soulService } from '@1agents/core/services/soulService';

/**
 * Simplified New Conversation landing (P1 of 统一新建会话).
 * No create-on-submit parameter bar: config gear + CTA that open
 * SessionSetup (or the embedded config form for defaults / skipModal).
 */

interface NewChatHomeProps {
    workspaces: Workspace[];
    activeWorkspaceId: string;
    /**
     * @deprecated P1 removed create-on-submit. Kept optional so old callers
     * compile during transition; ignored.
     */
    onSubmitChat?: (
        workspaceId: string,
        agentType: AgentType,
        prompt: string,
        role: string,
        permissionMode: string,
        agentRef: string
    ) => void;
    onSubmitTerminal?: (workspaceId: string, cwd: string, initialCommand: string) => void;
    onOpenFolder: () => void;
    lockedWorkspaceId?: string;
    language: Lang;
}

export function NewChatHome({
    workspaces,
    activeWorkspaceId,
    onOpenFolder,
    lockedWorkspaceId,
    language,
}: NewChatHomeProps) {
    const [showConfig, setShowConfig] = useState(false);
    const [teamMembers, setTeamMembers] = useState<TeamMemberOption[]>([]);
    // Unified picker (no assistant/project mode switch): remember last choice.
    const [selectedWsId, setSelectedWsId] = useState(() => {
        if (activeWorkspaceId && workspaces.some(w => w.id === activeWorkspaceId && !w.deviceId)) {
            return activeWorkspaceId;
        }
        return (
            workspaces.find(w => (w.kind ?? 'project') === 'workforce' && !w.deviceId)?.id ||
            workspaces.find(w => !w.deviceId)?.id ||
            ONESHOT_WORKSPACE_ID
        );
    });
    const selectedWorkspaceId = lockedWorkspaceId ?? selectedWsId;
    const setSelectedWorkspaceId = (id: string) => setSelectedWsId(id);

    const isOneshot = selectedWorkspaceId === ONESHOT_WORKSPACE_ID;
    const activeWorkspace = isOneshot ? undefined : workspaces.find(w => w.id === selectedWorkspaceId) || workspaces[0];

    useEffect(() => {
        const injected = wsStore.newChatWorkspaceId.value;
        if (injected) {
            setSelectedWorkspaceId(injected);
            wsStore.newChatWorkspaceId.value = '';
        }
    }, [wsStore.newChatWorkspaceId.value]);

    useEffect(() => {
        let cancelled = false;
        if (!selectedWorkspaceId || selectedWorkspaceId === ONESHOT_WORKSPACE_ID) {
            setTeamMembers([]);
            return;
        }
        soulService
            .getWorkspaceTeam(selectedWorkspaceId)
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
    }, [selectedWorkspaceId]);

    const openCreate = () => {
        void sess.openSessionSetup({
            workspaceId: selectedWorkspaceId || activeWorkspaceId || ONESHOT_WORKSPACE_ID,
            locked: !!lockedWorkspaceId,
            defaultAgent: activeWorkspace?.defaultAgent,
        });
    };

    const onConfigSave = () => {
        setShowConfig(false);
    };

    return (
        <div class="new-chat-home new-chat-home--simplified">
            {!lockedWorkspaceId && (
                <div class="new-chat-ws-picker-container new-chat-ws-picker-container--scope">
                    <WorkspaceScopePicker
                        workspaces={workspaces}
                        value={selectedWorkspaceId || ONESHOT_WORKSPACE_ID}
                        language={language}
                        onChange={setSelectedWorkspaceId}
                    />
                    <button
                        type="button"
                        class="new-chat-open-folder-btn"
                        onClick={onOpenFolder}
                        title={t('sidebar.newWorkspace', language)}
                    >
                        {t('sidebar.newWorkspace', language)}
                    </button>
                </div>
            )}

            <div class="new-chat-simplified-body">
                <div class="new-chat-simplified-hero">
                    <h2 class="new-chat-simplified-title">{t('newchat.simplified.title', language)}</h2>
                    <p class="new-chat-simplified-desc">{t('newchat.simplified.desc', language)}</p>
                </div>

                {showConfig ? (
                    <div class="new-chat-config-panel session-setup-modal">
                        <div class="new-chat-config-header">
                            <span>{t('newchat.simplified.configTitle', language)}</span>
                            <button
                                type="button"
                                class="ws-modal-close"
                                onClick={() => setShowConfig(false)}
                                aria-label={t('common.close', language)}
                            >
                                ✕
                            </button>
                        </div>
                        <SessionSetupForm
                            workspaces={workspaces}
                            defaultWorkspaceId={selectedWorkspaceId || activeWorkspaceId}
                            locked
                            defaultAgent={activeWorkspace?.defaultAgent}
                            teamMembers={teamMembers}
                            variant="config"
                            showSkipToggle
                            showRemember={false}
                            language={language}
                            onCancel={() => setShowConfig(false)}
                            onSubmit={onConfigSave}
                        />
                    </div>
                ) : (
                    <div class="new-chat-simplified-actions">
                        <button type="button" class="new-chat-cta-primary" onClick={openCreate}>
                            {t('newchat.simplified.cta', language)}
                        </button>
                        <button
                            type="button"
                            class="new-chat-cta-gear"
                            onClick={() => setShowConfig(true)}
                            title={t('newchat.simplified.configTitle', language)}
                            aria-label={t('newchat.simplified.configTitle', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <circle cx="12" cy="12" r="3" />
                                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                            </svg>
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
}
