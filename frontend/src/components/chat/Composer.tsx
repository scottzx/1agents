import { h } from 'preact';
import { useRef, useState } from 'preact/hooks';
import { t, getLang } from '../../i18n';
import type { PermissionMode } from '../types';
import type { SessionModesState, AvailableCommand } from '@1agents/core/protocol/types';
import { useSpeechRecognition } from '../../hooks/useSpeechRecognition';
import { useFileAttachments } from '../../hooks/useFileAttachments';
import { MicButton } from './input/MicButton';
import { AttachButton } from './input/AttachButton';
import { AttachmentPreview } from './input/AttachmentPreview';
import { PermissionModePicker } from './PermissionModePicker';
import { SessionModePicker } from './SessionModePicker';
import { SlashCommandPalette, slashQuery, filterCommands } from './SlashCommandPalette';

interface ComposerProps {
    onSend: (text: string) => void;
    onCancel?: () => void;
    isRunning?: boolean;
    disabled?: boolean;
    placeholder?: string;
    permissionMode: PermissionMode;
    onPermissionModeChange: (mode: PermissionMode) => void;
    /**
     * NATIVE session modes advertised by the agent. When present, the
     * native mode picker replaces the permissionMode shield (native leads;
     * the bridge gate stays fixed at approve-reads as the safety net).
     * null → mode-less agent → old shield picker.
     */
    sessionModes?: SessionModesState | null;
    onSessionModeChange?: (modeId: string) => void;
    /** Agent-advertised slash commands driving the `/` autocomplete palette. */
    availableCommands?: AvailableCommand[];
}

export function Composer({
    onSend,
    onCancel,
    isRunning,
    disabled,
    placeholder,
    permissionMode,
    onPermissionModeChange,
    sessionModes,
    onSessionModeChange,
    availableCommands = [],
}: ComposerProps) {
    const ref = useRef<HTMLTextAreaElement | null>(null);
    const lang = getLang();

    // Slash-command palette state (derived from the uncontrolled textarea in
    // handleInput). `matches` empty ⇒ palette hidden.
    const [slash, setSlash] = useState<{ matches: AvailableCommand[]; index: number }>({
        matches: [],
        index: 0,
    });

    const refreshSlash = (value: string) => {
        const q = availableCommands.length > 0 ? slashQuery(value) : null;
        if (q === null) {
            if (slash.matches.length) setSlash({ matches: [], index: 0 });
            return;
        }
        const matches = filterCommands(availableCommands, q);
        setSlash({ matches, index: 0 });
    };

    const closeSlash = () => {
        if (slash.matches.length) setSlash({ matches: [], index: 0 });
    };

    const pickSlash = (command: AvailableCommand) => {
        const el = ref.current;
        if (!el) return;
        // Fill the command; leave a trailing space when it takes args so the
        // caret is ready for them, otherwise it's send-ready as-is.
        el.value = `/${command.name}${command.hasInput ? ' ' : ''}`;
        closeSlash();
        el.focus();
        handleInput();
    };

    const handleKeyDown = (e: KeyboardEvent) => {
        if (slash.matches.length > 0) {
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                setSlash(s => ({ ...s, index: (s.index + 1) % s.matches.length }));
                return;
            }
            if (e.key === 'ArrowUp') {
                e.preventDefault();
                setSlash(s => ({ ...s, index: (s.index - 1 + s.matches.length) % s.matches.length }));
                return;
            }
            if ((e.key === 'Enter' || e.key === 'Tab') && !e.shiftKey && !e.isComposing) {
                e.preventDefault();
                pickSlash(slash.matches[slash.index]);
                return;
            }
            if (e.key === 'Escape') {
                e.preventDefault();
                closeSlash();
                return;
            }
        }
        if (e.key === 'Enter' && !e.shiftKey && !e.isComposing) {
            e.preventDefault();
            submit();
        }
    };

    const submit = () => {
        const el = ref.current;
        if (!el) return;
        const text = el.value.trim();
        if (!text) return;
        onSend(text);
        el.value = '';
        closeSlash();
        attach.clear();
        // Reset height
        el.style.height = 'auto';
    };

    const handleInput = () => {
        const el = ref.current;
        if (!el) return;
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 320) + 'px';
        refreshSlash(el.value);
    };

    // System speech-to-text. The textarea is uncontrolled, so the hook reads
    // it live via getText and writes the appended transcript back into it.
    const speech = useSpeechRecognition(
        lang,
        () => ref.current?.value ?? '',
        next => {
            const el = ref.current;
            if (!el) return;
            el.value = next;
            handleInput();
        }
    );

    // File upload — uncontrolled textarea, so the hook reads/writes its value
    // via the same getter/setter the speech hook uses, then re-grows it.
    const attach = useFileAttachments(
        () => ref.current?.value ?? '',
        next => {
            const el = ref.current;
            if (!el) return;
            el.value = next;
            handleInput();
        }
    );

    return (
        <div class="chat-composer">
            {slash.matches.length > 0 && (
                <SlashCommandPalette
                    commands={slash.matches}
                    activeIndex={slash.index}
                    onPick={pickSlash}
                    onHover={i => setSlash(s => ({ ...s, index: i }))}
                />
            )}
            <div class="chat-composer-frame">
                <AttachmentPreview attachments={attach.attachments} onRemove={attach.remove} />
                <textarea
                    ref={ref}
                    class="chat-composer-input"
                    placeholder={placeholder ?? t('chat.composer.placeholder', lang)}
                    disabled={disabled}
                    onKeyDown={handleKeyDown}
                    onInput={handleInput}
                    rows={1}
                    wrap="soft"
                />
                <div class="chat-composer-toolbar">
                    {sessionModes && sessionModes.availableModes.length > 0 && onSessionModeChange ? (
                        // Native leads: the agent's own modes (plan/acceptEdits/…)
                        // replace the shield; the bridge gate silently stays at
                        // approve-reads + project allowlist as the safety net.
                        <SessionModePicker modes={sessionModes} onChange={onSessionModeChange} disabled={disabled} />
                    ) : (
                        <PermissionModePicker
                            value={permissionMode}
                            onChange={onPermissionModeChange}
                            variant="cycle"
                            disabled={disabled}
                        />
                    )}
                    <div class="chat-composer-actions">
                        <AttachButton
                            className="chat-composer-attach-inline"
                            onSelect={attach.upload}
                            uploading={attach.uploading}
                            disabled={disabled}
                            title={attach.error || t('chat.composer.attach', lang)}
                            ariaLabel={t('chat.composer.attach', lang)}
                        />
                        {/* Voice input — hidden in the desktop (Tauri) build where
                            the native webview lacks a working Web Speech API; also
                            gated on API support + secure context via speech.available. */}
                        {!IS_DESKTOP && speech.available && (
                            <MicButton
                                className="chat-composer-mic-inline"
                                recording={speech.isRecording}
                                onClick={speech.toggle}
                                disabled={disabled}
                                title={speech.error || t('terminal.action.voice', lang)}
                                ariaLabel={t('terminal.action.voice', lang)}
                            />
                        )}
                        {isRunning ? (
                            <button
                                type="button"
                                class="chat-composer-stop-inline"
                                onClick={onCancel}
                                title={t('chat.composer.stop', lang)}
                                aria-label={t('chat.composer.stop', lang)}
                            >
                                <svg viewBox="0 0 24 24" width="14" height="14" fill="currentColor" aria-hidden="true">
                                    <rect x="6" y="6" width="12" height="12" rx="2" />
                                </svg>
                            </button>
                        ) : (
                            <button
                                type="button"
                                class="chat-composer-send-inline"
                                onClick={submit}
                                disabled={disabled}
                                title={t('chat.composer.send', lang)}
                                aria-label={t('chat.composer.send', lang)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    width="14"
                                    height="14"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    aria-hidden="true"
                                >
                                    <line x1="22" y1="2" x2="11" y2="13" />
                                    <polygon points="22 2 15 22 11 13 2 9 22 2" />
                                </svg>
                            </button>
                        )}
                    </div>
                </div>
            </div>
        </div>
    );
}
