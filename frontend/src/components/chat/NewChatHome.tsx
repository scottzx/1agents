import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { Workspace, type AgentType } from '../types';
import { t, type Lang } from '../../i18n';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import { SessionSetupForm, type TeamMemberOption } from './SessionSetupForm';
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
    const [wsSearch, setWsSearch] = useState('');
    const [teamMembers, setTeamMembers] = useState<TeamMemberOption[]>([]);
    // Unified picker (no assistant/project mode switch): remember last choice.
    const [selectedWsId, setSelectedWsId] = useState(() => {
        if (activeWorkspaceId && workspaces.some(w => w.id === activeWorkspaceId && !w.deviceId)) {
            return activeWorkspaceId;
        }
        return (
            workspaces.find(w => (w.kind ?? 'project') === 'assistant' && !w.deviceId)?.id ||
            workspaces.find(w => !w.deviceId)?.id ||
            activeWorkspaceId
        );
    });
    const selectedWorkspaceId = lockedWorkspaceId ?? selectedWsId;
    const setSelectedWorkspaceId = (id: string) => setSelectedWsId(id);
    const wsDropdownOpen = useSignal(false);
    const wsDropdownRef = useRef<HTMLDivElement | null>(null);

    const activeWorkspace = workspaces.find(w => w.id === selectedWorkspaceId) || workspaces[0];
    const isAssistantWs = (w: Workspace) => (w.kind ?? 'project') === 'assistant';

    const pickerWorkspaces = workspaces
        .filter(ws => !ws.deviceId)
        .filter(ws => ws.name.toLowerCase().includes(wsSearch.toLowerCase()))
        .sort((a, b) => {
            const ka = isAssistantWs(a) ? 0 : 1;
            const kb = isAssistantWs(b) ? 0 : 1;
            if (ka !== kb) return ka - kb;
            if (a.id === 'default') return -1;
            if (b.id === 'default') return 1;
            return 0;
        });

    useEffect(() => {
        const injected = wsStore.newChatWorkspaceId.value;
        if (injected) {
            setSelectedWorkspaceId(injected);
            wsStore.newChatWorkspaceId.value = '';
        }
    }, [wsStore.newChatWorkspaceId.value]);

    useEffect(() => {
        if (!wsDropdownOpen.value) return;
        const handleDown = (e: MouseEvent) => {
            if (wsDropdownRef.current && !wsDropdownRef.current.contains(e.target as Node)) {
                wsDropdownOpen.value = false;
            }
        };
        document.addEventListener('mousedown', handleDown);
        return () => document.removeEventListener('mousedown', handleDown);
    }, [wsDropdownOpen.value]);

    useEffect(() => {
        let cancelled = false;
        if (!selectedWorkspaceId) {
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
            workspaceId: selectedWorkspaceId || activeWorkspaceId,
            locked: !!lockedWorkspaceId,
            defaultAgent: activeWorkspace?.defaultAgent,
        });
    };

    const onConfigSave = () => {
        setShowConfig(false);
    };

    return (
        <div class="new-chat-home new-chat-home--simplified">
            {activeWorkspace && !lockedWorkspaceId && (
                <div class="new-chat-ws-picker-container" ref={wsDropdownRef}>
                    <button
                        class="new-chat-ws-picker-trigger"
                        onClick={() => (wsDropdownOpen.value = !wsDropdownOpen.value)}
                        title={t('sidebar.workspaces', language)}
                    >
                        <svg
                            class="folder-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                        </svg>
                        <span class="ws-name">{activeWorkspace.name}</span>
                        <span
                            class={`dropdown-kind-tag ${isAssistantWs(activeWorkspace) ? 'is-assistant' : 'is-project'}`}
                        >
                            {isAssistantWs(activeWorkspace)
                                ? t('newchat.kind.assistant', language)
                                : t('newchat.kind.project', language)}
                        </span>
                        <svg
                            class={`chevron ${wsDropdownOpen.value ? 'open' : ''}`}
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <polyline points="6 9 12 15 18 9" />
                        </svg>
                    </button>
                    {wsDropdownOpen.value && (
                        <div class="new-chat-ws-dropdown">
                            <div class="dropdown-search-wrap">
                                <input
                                    class="dropdown-search-input"
                                    type="text"
                                    placeholder={t('newchat.searchWs', language)}
                                    value={wsSearch}
                                    onInput={(e: Event) => setWsSearch((e.target as HTMLInputElement).value)}
                                    autoFocus
                                />
                            </div>
                            <div class="dropdown-list">
                                {pickerWorkspaces.map(ws => (
                                    <button
                                        key={ws.id}
                                        class={`dropdown-item ${ws.id === selectedWorkspaceId ? 'active' : ''}`}
                                        onClick={() => {
                                            setSelectedWorkspaceId(ws.id);
                                            wsDropdownOpen.value = false;
                                            setWsSearch('');
                                        }}
                                    >
                                        <span class="item-name">{ws.name}</span>
                                        <span
                                            class={`dropdown-kind-tag ${
                                                isAssistantWs(ws) ? 'is-assistant' : 'is-project'
                                            }`}
                                        >
                                            {isAssistantWs(ws)
                                                ? t('newchat.kind.assistant', language)
                                                : t('newchat.kind.project', language)}
                                        </span>
                                    </button>
                                ))}
                                {pickerWorkspaces.length === 0 && (
                                    <div class="dropdown-empty">{t('newchat.noWsMatch', language)}</div>
                                )}
                            </div>
                            <button
                                class="dropdown-item open-folder"
                                onClick={() => {
                                    onOpenFolder();
                                    wsDropdownOpen.value = false;
                                    setWsSearch('');
                                }}
                            >
                                <span class="item-name">{t('sidebar.newWorkspace', language)}</span>
                            </button>
                        </div>
                    )}
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
