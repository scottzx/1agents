import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { Workspace, AgentType, AGENT_TYPES, AGENT_TYPE_LABELS } from '../types';
import { t, type Lang } from '../i18n';
import * as wsStore from '../../stores/workspaceStore';
import { pickableAgents } from '../../stores/agentCatalogStore';
import { useSpeechRecognition } from '../../hooks/useSpeechRecognition';
import { MicButton } from './input/MicButton';

interface NewChatHomeProps {
    workspaces: Workspace[];
    activeWorkspaceId: string;
    onSubmitChat: (workspaceId: string, agentType: AgentType, prompt: string) => void;
    /**
     * Terminal mode: open a terminal in the workspace dir and optionally run
     * an initial command (e.g. `claude "..."`). cwd resolves to the
     * workspace's terminalDir || path; initialCommand is '' for a bare shell.
     */
    onSubmitTerminal?: (workspaceId: string, cwd: string, initialCommand: string) => void;
    onOpenFolder: () => void;
    language: Lang;
}

type TerminalPreset = 'claude' | 'codex' | 'gemini' | 'shell';

/** Preset → CLI binary. A missing `bin` means "plain shell, run nothing". */
const TERMINAL_PRESETS: { value: TerminalPreset; label: string; bin?: string }[] = [
    { value: 'claude', label: 'Claude', bin: 'claude' },
    { value: 'codex', label: 'Codex', bin: 'codex' },
    { value: 'gemini', label: 'Gemini', bin: 'gemini' },
    { value: 'shell', label: '纯 Shell' },
];

/** Wrap a prompt as a single double-quoted bash argument, escaping the chars
 *  that stay special inside double quotes. */
function quoteArg(s: string): string {
    return '"' + s.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/`/g, '\\`').replace(/\$/g, '\\$') + '"';
}

export function NewChatHome({
    workspaces,
    activeWorkspaceId,
    onSubmitChat,
    onSubmitTerminal,
    onOpenFolder,
    language,
}: NewChatHomeProps) {
    const [prompt, setPrompt] = useState('');
    const [selectedAgent, setSelectedAgent] = useState<AgentType>('claudecode');
    const [selectedPreset, setSelectedPreset] = useState<TerminalPreset>('claude');
    // useSignal (not useState) for the mode toggle — plain useState toggles
    // can fail to re-render under @preact/signals.
    const mode = useSignal<'chat' | 'terminal'>('chat');
    // Frontend-only pre-selection. Switching the actual workspace context is
    // deferred until the user sends a message (see handleSubmit → onSubmitChat).
    const [selectedWorkspaceId, setSelectedWorkspaceId] = useState(activeWorkspaceId);
    const wsDropdownOpen = useSignal(false);
    const wsDropdownRef = useRef<HTMLDivElement | null>(null);

    const activeWorkspace = workspaces.find(w => w.id === selectedWorkspaceId) || workspaces[0];

    // System speech-to-text for the prompt box. Reuses the terminal's
    // voice-input logic via a shared hook; appends to whatever is typed.
    const speech = useSpeechRecognition(language, () => prompt, setPrompt);

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
        if (!activeWorkspace) return;
        const trimmed = prompt.trim();

        if (mode.value === 'terminal') {
            const cwd = activeWorkspace.terminalDir || activeWorkspace.path;
            const preset = TERMINAL_PRESETS.find(p => p.value === selectedPreset) ?? TERMINAL_PRESETS[0];
            // No bin → bare shell; bin without prompt → launch the CLI alone.
            const initialCommand = preset.bin ? (trimmed ? `${preset.bin} ${quoteArg(trimmed)}` : preset.bin) : '';
            onSubmitTerminal?.(activeWorkspace.id, cwd, initialCommand);
            setPrompt('');
            return;
        }

        if (!trimmed) return;
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
                    placeholder={
                        speech.isRecording
                            ? t('terminal.speech.listening', language)
                            : mode.value === 'terminal'
                              ? '描述要让命令行执行什么（纯 Shell 可留空）'
                              : 'Ask anything, @ to mention, / for actions'
                    }
                    value={prompt}
                    onInput={(e: Event) => setPrompt((e.target as HTMLTextAreaElement).value)}
                    onKeyDown={handleKeyDown}
                    rows={1}
                />
                <div class="new-chat-actions-row">
                    <div class="actions-left">
                        {/* Mode toggle: chat vs terminal — icon segmented control */}
                        <div class="new-chat-mode-switch" role="group" aria-label="模式切换">
                            <button
                                type="button"
                                class={`mode-switch-btn ${mode.value === 'chat' ? 'active' : ''}`}
                                title="对话"
                                aria-pressed={mode.value === 'chat'}
                                onClick={() => (mode.value = 'chat')}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
                                </svg>
                            </button>
                            <button
                                type="button"
                                class={`mode-switch-btn ${mode.value === 'terminal' ? 'active' : ''}`}
                                title="终端"
                                aria-pressed={mode.value === 'terminal'}
                                onClick={() => (mode.value = 'terminal')}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="4 17 10 11 4 5" />
                                    <line x1="12" y1="19" x2="20" y2="19" />
                                </svg>
                            </button>
                        </div>

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

                        {/* Model / Agent selector (chat) — or preset selector (terminal) */}
                        <div class="select-dropdown-wrapper">
                            {mode.value === 'terminal' ? (
                                <select
                                    class="new-chat-select model-select"
                                    value={selectedPreset}
                                    onChange={(e: Event) =>
                                        setSelectedPreset((e.target as HTMLSelectElement).value as TerminalPreset)
                                    }
                                >
                                    {TERMINAL_PRESETS.map(p => (
                                        <option key={p.value} value={p.value}>
                                            {p.label}
                                        </option>
                                    ))}
                                </select>
                            ) : (
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
                            )}
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
                        {/* Mic Button — system speech-to-text. Hidden in the
                            desktop (Tauri) build, where the native webview lacks
                            a working Web Speech API and users have their own IME /
                            dictation. `speech.available` further requires the API +
                            a secure context. */}
                        {!IS_DESKTOP && speech.available && (
                            <MicButton
                                className="action-btn-circle mic-btn"
                                recording={speech.isRecording}
                                onClick={speech.toggle}
                                title={speech.error || t('terminal.action.voice', language)}
                                ariaLabel={t('terminal.action.voice', language)}
                            />
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}
