import { h } from 'preact';
import { t, getLang } from '../../i18n';
import type { SessionConfigOption } from '@1agents/core/protocol/types';

// Generic native config-option picker (ACP session/set_config_option) — one
// per advertised select option (model, reasoning effort, …), rendered
// data-driven from session_meta. The mode select is excluded upstream (it has
// the dedicated SessionModePicker), so this stays fully generic: label + value
// come straight from the agent's advertised names, i18n only supplies the
// icon-less category fallbacks.
//
// Same styled-button-with-invisible-<select> pattern as SessionModePicker so
// it shares the composer toolbar look.

/** Icon hint by category so model vs effort are glanceable. */
function categoryIcon(category: string | undefined) {
    if (category === 'model') {
        // Chip: model selection
        return (
            <g>
                <rect x="4" y="4" width="16" height="16" rx="2" />
                <path d="M9 9h6v6H9zM4 9h2M4 13h2M18 9h2M18 13h2M9 4v2M13 4v2M9 18v2M13 18v2" />
            </g>
        );
    }
    if (category === 'effort' || category === 'reasoning' || category === 'reasoningEffort') {
        // Gauge: reasoning effort
        return (
            <g>
                <path d="M4 15a8 8 0 0 1 16 0" />
                <path d="M12 15l4-4" stroke-linecap="round" />
            </g>
        );
    }
    // Sliders fallback
    return (
        <g>
            <path d="M4 8h10M18 8h2M4 16h2M10 16h10" />
            <circle cx="16" cy="8" r="2" />
            <circle cx="8" cy="16" r="2" />
        </g>
    );
}

interface ConfigOptionPickerProps {
    option: SessionConfigOption;
    onChange: (key: string, value: string) => void;
    disabled?: boolean;
}

export function ConfigOptionPicker({ option, onChange, disabled }: ConfigOptionPickerProps) {
    const lang = getLang();
    const current = option.options.find(o => o.value === option.currentValue) ?? option.options[0];
    if (!current) return null;

    const label = t('chat.configOption.label', lang, { name: option.name });
    const tooltip = `${label}: ${current.name}${current.description ? `\n${current.description}` : ''}`;

    const handleChange = (e: Event) => {
        const select = e.target as HTMLSelectElement;
        if (select.value !== current.value) {
            onChange(option.id, select.value);
        }
    };

    return (
        <label
            class="chat-composer-mode-btn chat-config-option-picker"
            data-config-category={option.category}
            title={tooltip}
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
                {categoryIcon(option.category)}
            </svg>
            <span class="chat-composer-mode-label">{current.name}</span>
            <select
                class="chat-config-option-select"
                value={current.value}
                disabled={disabled}
                aria-label={label}
                onChange={handleChange}
            >
                {option.options.map(o => (
                    <option key={o.value} value={o.value}>
                        {o.name}
                    </option>
                ))}
            </select>
        </label>
    );
}
