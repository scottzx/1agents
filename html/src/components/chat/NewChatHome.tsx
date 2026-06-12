import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { Workspace, AgentType, AGENT_TYPES, AGENT_TYPE_LABELS } from '../types';
import { t, type Lang } from '../i18n';

interface NewChatHomeProps {
    workspaces: Workspace[];
    activeWorkspaceId: string;
    onSelectWorkspace: (ws: Workspace) => void;
    onSubmitChat: (workspaceId: string, agentType: AgentType, prompt: string) => void;
    language: Lang;
}

export function NewChatHome({
    workspaces,
    activeWorkspaceId,
    onSelectWorkspace,
    onSubmitChat,
    language,
}: NewChatHomeProps) {
    const [prompt, setPrompt] = useState('');
    const [selectedAgent, setSelectedAgent] = useState<AgentType>('claudecode');
    const [wsDropdownOpen, setWsDropdownOpen] = useState(false);
    const wsDropdownRef = useRef<HTMLDivElement | null>(null);

    const activeWorkspace = workspaces.find(w => w.id === activeWorkspaceId) || workspaces[0];

    // Align local state agent selector with workspace's default agent if it changes
    useEffect(() => {
        if (activeWorkspace?.defaultAgent && AGENT_TYPES.includes(activeWorkspace.defaultAgent)) {
            setSelectedAgent(activeWorkspace.defaultAgent);
        }
    }, [activeWorkspaceId, activeWorkspace]);

    // Handle outside click for the workspace dropdown
    useEffect(() => {
        if (!wsDropdownOpen) return;
        const handleDown = (e: MouseEvent) => {
            if (wsDropdownRef.current && !wsDropdownRef.current.contains(e.target as Node)) {
                setWsDropdownOpen(false);
            }
        };
        document.addEventListener('mousedown', handleDown);
        return () => document.removeEventListener('mousedown', handleDown);
    }, [wsDropdownOpen]);

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
                        onClick={() => setWsDropdownOpen(!wsDropdownOpen)}
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
                            class={`chevron ${wsDropdownOpen ? 'open' : ''}`}
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
                    {wsDropdownOpen && (
                        <div class="new-chat-ws-dropdown">
                            <div class="dropdown-header">切换项目工作空间</div>
                            {workspaces.map(ws => (
                                <button
                                    key={ws.id}
                                    class={`dropdown-item ${ws.id === activeWorkspaceId ? 'active' : ''}`}
                                    onClick={() => {
                                        onSelectWorkspace(ws);
                                        setWsDropdownOpen(false);
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
                                    {ws.id === activeWorkspaceId && <span class="checkmark">✓</span>}
                                </button>
                            ))}
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
                                {AGENT_TYPES.map(t => (
                                    <option key={t} value={t}>
                                        {AGENT_TYPE_LABELS[t] ?? t}
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
