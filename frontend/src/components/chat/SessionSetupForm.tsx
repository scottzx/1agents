import { h, Fragment } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';

import { t, type Lang } from '../../i18n';
import type { Workspace, AgentType } from '../types';
import { AgentTypePicker } from './AgentTypePicker';
import { sessionSetupDefaults, saveSessionSetupDefaults, type SessionMode } from '../../stores/sessionSetupDefaults';

/**
 * Pure form for the unified "new session" setup (P0-2 of 统一新建会话).
 * Holds the mode / agent|preset / name / workspace rows and validates. The
 * parent (SessionSetupModal) owns the overlay + close.
 *
 * - No role / permission fields — mode + agent|preset + name only.
 * - Toggling mode swaps the secondary row between AgentTypePicker and the
 *   terminal preset list.
 * - Enter submits; the overlay handles Escape via a global keydown listener.
 * - The optional workspace row is shown only when the modal is NOT locked
 *   to a specific workspace (assistant / project detail flows).
 * - On submit, the form mirrors the local pick back into the global
 *   `sessionSetupDefaults` so P1's skip-modal remembers the user's last choice.
 */

export type TerminalPresetBin = 'claude' | 'codex' | 'gemini' | 'shell';

/** Aligned with NewChatHome.TERMINAL_PRESETS (#108 §5.3, P0-2). */
export const TERMINAL_PRESETS: { value: TerminalPresetBin; label: string; bin?: string }[] = [
    { value: 'claude', label: 'Claude', bin: 'claude' },
    { value: 'codex', label: 'Codex', bin: 'codex' },
    { value: 'gemini', label: 'Gemini', bin: 'gemini' },
    { value: 'shell', label: 'Shell' },
];

export interface SessionSetupFormValues {
    mode: SessionMode;
    agentType: AgentType;
    terminalPreset?: TerminalPresetBin;
    name: string;
    workspaceId: string;
}

interface SessionSetupFormProps {
    workspaces: Workspace[];
    /** Required. */
    defaultWorkspaceId: string;
    /** Hidden workspace row when locked (e.g. assistant / project detail). */
    locked?: boolean;
    /**
     * Preset default agent (workspace default). Used when the global
     * defaults signal has not yet been written by this user.
     */
    defaultAgent?: AgentType;
    /** Optional prefilled terminal preset (e.g. P2 terminal empty-state CTA). */
    initialTerminalPreset?: TerminalPresetBin;
    language: Lang;
    onSubmit: (values: SessionSetupFormValues) => void;
    onCancel: () => void;
}

const DEFAULT_PRESET: TerminalPresetBin = 'claude';

const pickInitialWorkspace = (workspaces: Workspace[], preferred: string): string => {
    if (workspaces.some(w => w.id === preferred)) return preferred;
    return workspaces[0]?.id ?? '';
};

export function SessionSetupForm({
    workspaces,
    defaultWorkspaceId,
    locked = false,
    defaultAgent,
    initialTerminalPreset,
    language,
    onSubmit,
    onCancel,
}: SessionSetupFormProps) {
    const stored = sessionSetupDefaults.value;

    const initialWorkspaceId = useMemo(
        () => pickInitialWorkspace(workspaces, defaultWorkspaceId),
        [workspaces, defaultWorkspaceId]
    );

    const [mode, setMode] = useState<SessionMode>(stored.mode);
    const [agentType, setAgentType] = useState<AgentType>(stored.agentType ?? defaultAgent ?? 'claudecode');
    const [terminalPreset, setTerminalPreset] = useState<TerminalPresetBin>(
        initialTerminalPreset ?? stored.terminalPreset ?? DEFAULT_PRESET
    );
    const [name, setName] = useState('');
    const [workspaceId, setWorkspaceId] = useState<string>(initialWorkspaceId);

    useEffect(() => {
        setWorkspaceId(pickInitialWorkspace(workspaces, defaultWorkspaceId));
    }, [defaultWorkspaceId, workspaces]);

    useEffect(() => {
        if (defaultAgent) setAgentType(defaultAgent);
    }, [defaultAgent]);

    const submit = () => {
        const values: SessionSetupFormValues = {
            mode,
            agentType,
            terminalPreset: mode === 'terminal' ? terminalPreset : undefined,
            name: name.trim(),
            workspaceId: locked ? defaultWorkspaceId : workspaceId,
        };
        saveSessionSetupDefaults({
            mode,
            agentType,
            terminalPreset: mode === 'terminal' ? terminalPreset : stored.terminalPreset,
        });
        onSubmit(values);
    };

    const onKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
            e.preventDefault();
            submit();
        }
    };

    return (
        <div class="session-setup-form" onKeyDown={onKeyDown}>
            <label class="ws-modal-label">{t('newchat.modeSwitch', language)}</label>
            <div class="session-setup-mode-toggle" role="group" aria-label={t('newchat.modeSwitch', language)}>
                <button
                    type="button"
                    class={`session-setup-mode-btn ${mode === 'chat' ? 'active' : ''}`}
                    aria-pressed={mode === 'chat'}
                    title={t('newchat.modeChatTitle', language)}
                    onClick={() => setMode('chat')}
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
                        <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
                    </svg>
                    <span class="session-setup-mode-label">{t('newchat.modeChatTitle', language)}</span>
                </button>
                <button
                    type="button"
                    class={`session-setup-mode-btn ${mode === 'terminal' ? 'active' : ''}`}
                    aria-pressed={mode === 'terminal'}
                    title={t('newchat.modeTerminalTitle', language)}
                    onClick={() => setMode('terminal')}
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
                        <polyline points="4 17 10 11 4 5" />
                        <line x1="12" y1="19" x2="20" y2="19" />
                    </svg>
                    <span class="session-setup-mode-label">{t('newchat.modeTerminalTitle', language)}</span>
                </button>
            </div>

            <label class="ws-modal-label">
                {mode === 'terminal'
                    ? t('newchat.modeTerminalTitle', language)
                    : t('sessionSetup.agent.label', language)}
            </label>
            {mode === 'terminal' ? (
                <select
                    class="agent-type-picker"
                    value={terminalPreset}
                    onChange={(e: Event) =>
                        setTerminalPreset((e.target as HTMLSelectElement).value as TerminalPresetBin)
                    }
                >
                    {TERMINAL_PRESETS.map(p => (
                        <option key={p.value} value={p.value}>
                            {p.value === 'shell' ? t('newchat.terminalShell', language) : p.label}
                        </option>
                    ))}
                </select>
            ) : (
                <AgentTypePicker value={agentType} onChange={setAgentType} />
            )}

            {!locked && (
                <>
                    <label class="ws-modal-label">{t('modal.sessionSetup.workspace', language)}</label>
                    <select
                        class="agent-type-picker"
                        value={workspaceId}
                        onChange={(e: Event) => setWorkspaceId((e.target as HTMLSelectElement).value)}
                    >
                        {workspaces.map(w => (
                            <option key={w.id} value={w.id}>
                                {w.name}
                            </option>
                        ))}
                    </select>
                </>
            )}

            <label class="ws-modal-label">{t('modal.session.name', language)}</label>
            <input
                class="ws-modal-input"
                placeholder={t('modal.session.namePlaceholder', language)}
                value={name}
                onInput={(e: Event) => setName((e.target as HTMLInputElement).value)}
                autoFocus
            />
            <div class="ws-modal-footer">
                <button type="button" class="ws-modal-cancel" onClick={onCancel}>
                    {t('common.cancel', language)}
                </button>
                <button type="button" class="ws-modal-confirm" onClick={submit} disabled={!locked && !workspaceId}>
                    {t('modal.workspace.create', language)}
                </button>
            </div>
        </div>
    );
}
