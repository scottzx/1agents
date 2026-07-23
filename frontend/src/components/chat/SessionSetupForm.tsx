import { h, Fragment } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';

import { t, type Lang } from '../../i18n';
import type { Workspace, AgentType } from '../types';
import { AgentTypePicker } from './AgentTypePicker';
import { WorkspaceScopePicker, ONESHOT_WORKSPACE_ID } from './WorkspaceScopePicker';
import { sessionSetupDefaults, saveSessionSetupDefaults } from '../../stores/sessionSetupDefaults';
import { agentCatalog, agentCatalogLoading, pickableAgents } from '../../stores/agentCatalogStore';

/**
 * Chat-only form for the unified "new session" setup.
 * Terminal is created from the sidebar as a bare tmux pane (no mode toggle).
 */

export interface SessionSetupFormValues {
    mode: 'chat';
    agentType: AgentType;
    name: string;
    workspaceId: string;
    /** Team expert file ref; empty = primary. */
    agentRef?: string;
    skipModal?: boolean;
    /** True when workspace is oneshot / 单次对话. */
    ephemeral?: boolean;
}

export interface TeamMemberOption {
    file: string;
    name: string;
}

interface SessionSetupFormProps {
    workspaces: Workspace[];
    defaultWorkspaceId: string;
    /** Hidden workspace row when locked (assistant / project / task). */
    locked?: boolean;
    defaultAgent?: AgentType;
    initialAgentRef?: string;
    /** Team experts for the selected workspace. */
    teamMembers?: TeamMemberOption[];
    /**
     * `create` — confirm creates a session (default).
     * `config` — confirm only persists defaults (NewChatHome gear).
     */
    variant?: 'create' | 'config';
    /** Show skipModal toggle (config panel). */
    showSkipToggle?: boolean;
    /** Show "remember this choice" (create modal). Default true for create. */
    showRemember?: boolean;
    language: Lang;
    onSubmit: (values: SessionSetupFormValues) => void;
    onCancel: () => void;
    /** Fired when the user picks a different workspace (non-locked). */
    onWorkspaceChange?: (workspaceId: string) => void;
}

const pickInitialWorkspace = (workspaces: Workspace[], preferred: string): string => {
    if (preferred === ONESHOT_WORKSPACE_ID) return ONESHOT_WORKSPACE_ID;
    if (workspaces.some(w => w.id === preferred)) return preferred;
    return workspaces[0]?.id ?? ONESHOT_WORKSPACE_ID;
};

export function SessionSetupForm({
    workspaces,
    defaultWorkspaceId,
    locked = false,
    defaultAgent,
    initialAgentRef,
    teamMembers = [],
    variant = 'create',
    showSkipToggle = false,
    showRemember,
    language,
    onSubmit,
    onCancel,
    onWorkspaceChange,
}: SessionSetupFormProps) {
    const stored = sessionSetupDefaults.value;
    const rememberDefault = showRemember ?? variant === 'create';

    const initialWorkspaceId = useMemo(
        () => pickInitialWorkspace(workspaces, defaultWorkspaceId),
        [workspaces, defaultWorkspaceId]
    );

    const [agentType, setAgentType] = useState<AgentType>(defaultAgent ?? stored.agentType ?? 'claudecode');
    const [name, setName] = useState('');
    const [workspaceId, setWorkspaceId] = useState<string>(initialWorkspaceId);
    const [agentRef, setAgentRef] = useState(initialAgentRef ?? '');
    const [skipModal, setSkipModal] = useState(stored.skipModal);
    const [remember, setRemember] = useState(true);

    useEffect(() => {
        setWorkspaceId(pickInitialWorkspace(workspaces, defaultWorkspaceId));
    }, [defaultWorkspaceId, workspaces]);

    useEffect(() => {
        if (defaultAgent) setAgentType(defaultAgent);
    }, [defaultAgent]);

    // Catalog still loading with no data → block confirm.
    const catalogBlocking = agentCatalogLoading.value && agentCatalog.value.length === 0;
    const noPickableAgent =
        !agentCatalogLoading.value && pickableAgents.value.length === 0 && agentCatalog.value.length > 0;

    const effectiveWorkspaceId = locked ? defaultWorkspaceId : workspaceId;
    const isOneshot = effectiveWorkspaceId === ONESHOT_WORKSPACE_ID;

    const submit = () => {
        if (catalogBlocking || noPickableAgent) return;
        const values: SessionSetupFormValues = {
            mode: 'chat',
            agentType,
            name: name.trim(),
            workspaceId: effectiveWorkspaceId,
            agentRef: isOneshot ? undefined : agentRef || undefined,
            skipModal: showSkipToggle ? skipModal : undefined,
            ephemeral: isOneshot,
        };

        if (variant === 'config' || (rememberDefault && remember)) {
            saveSessionSetupDefaults({
                mode: 'chat',
                agentType,
                ...(showSkipToggle ? { skipModal } : {}),
            });
        }

        onSubmit(values);
    };

    const onKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
            e.preventDefault();
            submit();
        }
    };

    const confirmDisabled = catalogBlocking || noPickableAgent || (!locked && !workspaceId);

    const confirmLabel =
        variant === 'config'
            ? t('modal.sessionSetup.saveDefaults', language)
            : t('modal.sessionSetup.confirm', language);

    return (
        <div class="session-setup-form" onKeyDown={onKeyDown}>
            <label class="ws-modal-label">{t('sessionSetup.agent.label', language)}</label>
            <AgentTypePicker value={agentType} onChange={setAgentType} disabled={catalogBlocking} />

            {teamMembers.length > 0 && !isOneshot && (
                <Fragment>
                    <label class="ws-modal-label">{t('newchat.expert.aria', language)}</label>
                    <select
                        class="agent-type-picker"
                        value={agentRef}
                        onChange={(e: Event) => setAgentRef((e.target as HTMLSelectElement).value)}
                        title={t('newchat.expert.hint', language)}
                    >
                        <option value="">{t('newchat.expert.default', language)}</option>
                        {teamMembers.map(m => (
                            <option key={m.file} value={m.file}>
                                {m.name}
                            </option>
                        ))}
                    </select>
                </Fragment>
            )}

            {!locked && (
                <Fragment>
                    <label class="ws-modal-label">{t('modal.sessionSetup.workspace', language)}</label>
                    <WorkspaceScopePicker
                        workspaces={workspaces}
                        value={workspaceId}
                        language={language}
                        disabled={catalogBlocking}
                        onChange={id => {
                            setWorkspaceId(id);
                            onWorkspaceChange?.(id);
                        }}
                    />
                    {isOneshot && (
                        <p class="session-setup-hint" role="note">
                            {t('modal.sessionSetup.oneshotHint', language)}
                        </p>
                    )}
                </Fragment>
            )}

            {variant === 'create' && (
                <Fragment>
                    <label class="ws-modal-label">{t('modal.sessionSetup.nameOptional', language)}</label>
                    <input
                        class="ws-modal-input"
                        placeholder={t('modal.sessionSetup.namePlaceholder', language)}
                        value={name}
                        onInput={(e: Event) => setName((e.target as HTMLInputElement).value)}
                        autoFocus
                    />
                </Fragment>
            )}

            {showSkipToggle && (
                <label class="session-setup-check-row">
                    <input
                        type="checkbox"
                        checked={skipModal}
                        onChange={(e: Event) => setSkipModal((e.target as HTMLInputElement).checked)}
                    />
                    <span>{t('modal.sessionSetup.skipModal', language)}</span>
                </label>
            )}

            {rememberDefault && variant === 'create' && (
                <label class="session-setup-check-row">
                    <input
                        type="checkbox"
                        checked={remember}
                        onChange={(e: Event) => setRemember((e.target as HTMLInputElement).checked)}
                    />
                    <span>{t('modal.sessionSetup.remember', language)}</span>
                </label>
            )}

            {(catalogBlocking || noPickableAgent) && (
                <p class="session-setup-hint" role="status">
                    {catalogBlocking
                        ? t('modal.sessionSetup.catalogLoading', language)
                        : t('modal.sessionSetup.catalogEmpty', language)}
                </p>
            )}

            <div class="ws-modal-footer">
                <button type="button" class="ws-modal-cancel" onClick={onCancel}>
                    {t('common.cancel', language)}
                </button>
                <button type="button" class="ws-modal-confirm" onClick={submit} disabled={confirmDisabled}>
                    {confirmLabel}
                </button>
            </div>
        </div>
    );
}
