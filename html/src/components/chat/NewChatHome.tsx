import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { Workspace, AgentType, AGENT_TYPES, AGENT_TYPE_LABELS } from '../types';
import { t, type Lang } from '../i18n';
import * as wsStore from '../../stores/workspaceStore';
import { pickableAgents } from '../../stores/agentCatalogStore';

interface NewChatHomeProps {
    workspaces: Workspace[];
    activeWorkspaceId: string;
    onSubmitChat: (workspaceId: string, agentType: AgentType, prompt: string) => void;
    onOpenFolder: () => void;
    language: Lang;
}

export function NewChatHome({ workspaces, activeWorkspaceId, onSubmitChat, onOpenFolder, language }: NewChatHomeProps) {
    const [prompt, setPrompt] = useState('');
    const [selectedAgent, setSelectedAgent] = useState<AgentType>('claudecode');
    // Frontend-only pre-selection. Switching the actual workspace context is
    // deferred until the user sends a message (see handleSubmit → onSubmitChat).
    const [selectedWorkspaceId, setSelectedWorkspaceId] = useState(activeWorkspaceId);
    const wsDropdownOpen = useSignal(false);
    const wsDropdownRef = useRef<HTMLDivElement | null>(null);

    const activeWorkspace = workspaces.find(w => w.id === selectedWorkspaceId) || workspaces[0];

    // Offer only installed agents (falls back to the full static list before
    // the catalog loads). Keep the current selection present even if it isn't
    // installed, so a workspace's defaultAgent still renders.
    const pickable = pickableAgents.value;
    const agentOptions: { type: AgentType; label: string }[] = pickable.length
        ? pickable.map(a => ({ type: a.type, label: a.label }))
        : AGENT_TYPES.map(ty => ({ type: ty, label: AGENT_TYPE_LABELS[ty] ?? ty }));
    if (selectedAgent && !agentOptions.some(o => o.type === selectedAgent)) {
        agentOptions.unshift({ type: selectedAgent, label: AGENT_TYPE_LABELS[selectedAgent] ?? selectedAgent });
    }

    // Align local state agent selector with workspace's default agent if it changes
    useEffect(() => {
        if (activeWorkspace?.defaultAgent && AGENT_TYPES.includes(activeWorkspace.defaultAgent)) {
            setSelectedAgent(activeWorkspace.defaultAgent);
        }
    }, [selectedWorkspaceId, activeWorkspace]);

    // Adopt a workspace freshly created via "Open folder…" as the picker
    // selection, without leaving the new-chat landing. One-shot: consume + clear.
    useEffect(() => {
        const injected = wsStore.newChatWorkspaceId.value;
        if (injected) {
            setSelectedWorkspaceId(injected);
            wsStore.newChatWorkspaceId.value = '';
        }
    }, [wsStore.newChatWorkspaceId.value]);

    // Handle outside click for the workspace dropdown
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

    const handleSubmit = (e?: Event) => {
        if (e) e.preventDefault();
        const trimmed = prompt.trim();
        if (!trimmed || !activeWorkspace) return;
        onSubmitChat(activeWorkspace.id, selectedAgent, trimmed);
        setPrompt('');
    };

    const handleKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSubmit();
        }
    };

    return (
        <div class="new-chat-home">
            {/* Top Workspace Picker Dropdown */}
            {activeWorkspace && (
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
                            <div class="dropdown-header">切换项目工作空间</div>
                            {workspaces.map(ws => (
                                <button
                                    key={ws.id}
                                    class={`dropdown-item ${ws.id === selectedWorkspaceId ? 'active' : ''}`}
                                    onClick={() => {
                                        setSelectedWorkspaceId(ws.id);
                                        wsDropdownOpen.value = false;
                                    }}
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        style="width: 14px; height: 14px; opacity: 0.7;"
                                    >
                                        <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                    </svg>
                                    <span class="item-name">{ws.name}</span>
                                    {ws.id === selectedWorkspaceId && <span class="checkmark">✓</span>}
                                </button>
                            ))}
                            <button
                                class="dropdown-item open-folder"
                                onClick={() => {
                                    onOpenFolder();
                                    wsDropdownOpen.value = false;
                                }}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    style="width: 14px; height: 14px; opacity: 0.7;"
                                >
                                    <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                    <line x1="12" y1="10" x2="12" y2="16" />
                                    <line x1="9" y1="13" x2="15" y2="13" />
                                </svg>
                                <span class="item-name">{t('sidebar.newWorkspace', language)}</span>
                            </button>
                        </div>
                    )}
                </div>
            )}

            {/* Central Chat Input Box */}
            <div class="new-chat-input-wrapper">
                <textarea
                    class="new-chat-textarea"
                    placeholder="Ask anything, @ to mention, / for actions"
                    value={prompt}
                    onInput={(e: Event) => setPrompt((e.target as HTMLTextAreaElement).value)}
                    onKeyDown={handleKeyDown}
                    rows={1}
                />
                <div class="new-chat-actions-row">
                    <div class="actions-left">
                        {/* + Button placeholder */}
                        <button class="action-btn-circle plus-btn" title="Add attachment">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <line x1="12" y1="5" x2="12" y2="19" />
                                <line x1="5" y1="12" x2="19" y2="12" />
                            </svg>
                        </button>

                        {/* Model / Agent Selector Dropdown */}
                        <div class="select-dropdown-wrapper">
                            <select
                                class="new-chat-select model-select"
                                value={selectedAgent}
                                onChange={(e: Event) =>
                                    setSelectedAgent((e.target as HTMLSelectElement).value as AgentType)
                                }
                            >
                                {agentOptions.map(o => (
                                    <option key={o.type} value={o.type}>
                                        {o.label}
                                    </option>
                                ))}
                            </select>
                            <svg
                                class="select-chevron"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                            >
                                <polyline points="6 9 12 15 18 9" />
                            </svg>
                        </div>

                        {/* Environment Picker Dropdown (Placeholder) */}
                        <div class="select-dropdown-wrapper">
                            <select class="new-chat-select env-select" disabled>
                                <option value="local">💻 Local</option>
                            </select>
                            <svg
                                class="select-chevron"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                            >
                                <polyline points="6 9 12 15 18 9" />
                            </svg>
                        </div>
                    </div>

                    <div class="actions-right">
                        {/* Mic Button placeholder */}
                        <button class="action-btn-circle mic-btn" title="Voice Input (Placeholder)">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                                <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                                <line x1="12" y1="19" x2="12" y2="23" />
                                <line x1="8" y1="23" x2="16" y2="23" />
                            </svg>
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
}
