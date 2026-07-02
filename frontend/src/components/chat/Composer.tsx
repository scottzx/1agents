import { h } from 'preact';
import { useRef } from 'preact/hooks';
import { t, getLang } from '../../i18n';
import type { PermissionMode } from '../types';
import type { SessionModesState } from '@1agents/core/protocol/types';
import { useSpeechRecognition } from '../../hooks/useSpeechRecognition';
import { useFileAttachments } from '../../hooks/useFileAttachments';
import { MicButton } from './input/MicButton';
import { AttachButton } from './input/AttachButton';
import { AttachmentPreview } from './input/AttachmentPreview';
import { PermissionModePicker } from './PermissionModePicker';
import { SessionModePicker } from './SessionModePicker';

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
}: ComposerProps) {
    const ref = useRef<HTMLTextAreaElement | null>(null);
    const lang = getLang();

    const handleKeyDown = (e: KeyboardEvent) => {
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
        attach.clear();
        // Reset height
        el.style.height = 'auto';
    };

    const handleInput = () => {
        const el = ref.current;
        if (!el) return;
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 320) + 'px';
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
