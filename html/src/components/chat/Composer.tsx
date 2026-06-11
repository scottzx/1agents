import { h } from 'preact';
import { useRef } from 'preact/hooks';
import { t, getLang } from '../../i18n';
import { nextPermissionMode } from '../types';
import type { PermissionMode } from '../types';

interface ComposerProps {
    onSend: (text: string) => void;
    onCancel?: () => void;
    isRunning?: boolean;
    disabled?: boolean;
    placeholder?: string;
    permissionMode: PermissionMode;
    onPermissionModeChange: (mode: PermissionMode) => void;
}

// Visual + label tokens for the three permission modes. Each mode picks up
// `[data-mode]` in SCSS to vary the accent colour — green for the safe
// default, amber for the broad allow, red for the blanket deny.
const MODE_LABEL_KEY: Record<PermissionMode, string> = {
    'approve-reads': 'chat.permission.mode.approveReads',
    'approve-all': 'chat.permission.mode.approveAll',
    'deny-all': 'chat.permission.mode.denyAll',
};

const MODE_TOOLTIP_KEY: Record<PermissionMode, string> = {
    'approve-reads': 'chat.permission.mode.tooltip.approveReads',
    'approve-all': 'chat.permission.mode.tooltip.approveAll',
    'deny-all': 'chat.permission.mode.tooltip.denyAll',
};

export function Composer({
    onSend,
    onCancel,
    isRunning,
    disabled,
    placeholder,
    permissionMode,
    onPermissionModeChange,
}: ComposerProps) {
    const ref = useRef<HTMLTextAreaElement | null>(null);
    const lang = getLang();

    const handleKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
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
        // Reset height
        el.style.height = 'auto';
    };

    const handleInput = () => {
        const el = ref.current;
        if (!el) return;
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 320) + 'px';
    };

    const cycleMode = () => {
        onPermissionModeChange(nextPermissionMode(permissionMode));
    };

    // Single tooltip combines the mode label + behaviour summary so users
    // know both what's currently active and what clicking would change.
    const modeTooltip = `${t('chat.permission.mode.label', lang)}: ${t(MODE_LABEL_KEY[permissionMode], lang)}\n${t(MODE_TOOLTIP_KEY[permissionMode], lang)}`;

    return (
        <div class="chat-composer">
            <div class="chat-composer-frame">
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
                    <button
                        type="button"
                        class="chat-composer-mode-btn"
                        data-mode={permissionMode}
                        onClick={cycleMode}
                        title={modeTooltip}
                        aria-label={t('chat.permission.mode.label', lang)}
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
                            <path d="M12 2 4 6v6c0 5 3.5 9 8 10 4.5-1 8-5 8-10V6l-8-4z" />
                        </svg>
                        <span class="chat-composer-mode-label">{t(MODE_LABEL_KEY[permissionMode], lang)}</span>
                    </button>
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
    );
}
