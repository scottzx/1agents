import { h } from 'preact';
import { t, getLang } from '../../i18n';
import { PERMISSION_MODES, nextPermissionMode, type PermissionMode } from '../types';

// Central registry for mode display tokens.
export const MODE_LABEL_KEY: Record<PermissionMode, string> = {
    'approve-reads': 'chat.permission.mode.approveReads',
    auto: 'chat.permission.mode.auto',
    'approve-all': 'chat.permission.mode.approveAll',
    'deny-all': 'chat.permission.mode.denyAll',
};

export const MODE_TOOLTIP_KEY: Record<PermissionMode, string> = {
    'approve-reads': 'chat.permission.mode.tooltip.approveReads',
    auto: 'chat.permission.mode.tooltip.auto',
    'approve-all': 'chat.permission.mode.tooltip.approveAll',
    'deny-all': 'chat.permission.mode.tooltip.denyAll',
};

interface PermissionModePickerProps {
    value: PermissionMode;
    onChange: (mode: PermissionMode) => void;
    /**
     * 'cycle'  — single shield button that cycles through modes on click (Composer toolbar)
     * 'select' — standard <select> dropdown (modal / settings forms)
     */
    variant?: 'cycle' | 'select';
    disabled?: boolean;
}

export function PermissionModePicker({ value, onChange, variant = 'cycle', disabled }: PermissionModePickerProps) {
    const lang = getLang();

    if (variant === 'select') {
        return (
            <select
                class="agent-type-picker"
                value={value}
                disabled={disabled}
                onChange={(e: Event) => onChange((e.target as HTMLSelectElement).value as PermissionMode)}
            >
                {PERMISSION_MODES.map(m => (
                    <option key={m} value={m}>
                        {t(MODE_LABEL_KEY[m], lang)}
                    </option>
                ))}
            </select>
        );
    }

    const tooltip = `${t('chat.permission.mode.label', lang)}: ${t(MODE_LABEL_KEY[value], lang)}\n${t(MODE_TOOLTIP_KEY[value], lang)}`;

    return (
        <button
            type="button"
            class="chat-composer-mode-btn"
            data-mode={value}
            disabled={disabled}
            onClick={() => onChange(nextPermissionMode(value))}
            title={tooltip}
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
            <span class="chat-composer-mode-label">{t(MODE_LABEL_KEY[value], lang)}</span>
        </button>
    );
}
