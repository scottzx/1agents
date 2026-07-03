import { h } from 'preact';
import { t, getLang } from '../../i18n';
import type { SessionModesState } from '@1agents/core/protocol/types';

// NATIVE session-mode picker (ACP session/set_mode) — the mode-capable
// sibling of PermissionModePicker. Strictly data-driven from the modes the
// agent advertised in session_meta: Claude Code offers up to six
// (default/acceptEdits/plan/…), Codex three (read-only/agent/…), so the id
// set is never hardcoded. Labels prefer the i18n override for known ids and
// fall back to the agent's advertised display name for anything else.

/** Ids with an i18n label override — everything else shows the advertised name. */
const KNOWN_MODE_IDS = new Set([
    'default',
    'acceptEdits',
    'plan',
    'auto',
    'dontAsk',
    'bypassPermissions',
    'read-only',
    'agent',
    'agent-full-access',
]);

/**
 * Modes that loosen safety limits (writes/commands run without asking).
 * Switching to one prompts an explicit confirm; the active state is styled
 * as a warning via data-mode-danger.
 */
const DANGEROUS_MODE_IDS = new Set(['bypassPermissions', 'dontAsk', 'agent-full-access']);

export function sessionModeLabel(id: string, advertisedName: string): string {
    const lang = getLang();
    if (!KNOWN_MODE_IDS.has(id)) {
        return advertisedName;
    }
    const label = t(`chat.sessionMode.id.${id}`, lang);
    // zh UI shows 「两字中文（英文原名）」 e.g. 规划（Plan）; en UI the advertised
    // English name alone is enough (avoids "Plan（Plan）").
    return lang === 'zh-CN' && label !== advertisedName ? `${label}（${advertisedName}）` : label;
}

export function isDangerousSessionMode(id: string): boolean {
    return DANGEROUS_MODE_IDS.has(id);
}

interface SessionModePickerProps {
    modes: SessionModesState;
    onChange: (modeId: string) => void;
    disabled?: boolean;
}

export function SessionModePicker({ modes, onChange, disabled }: SessionModePickerProps) {
    const lang = getLang();
    const current = modes.availableModes.find(m => m.id === modes.currentModeId) ?? modes.availableModes[0];
    if (!current) return null;

    const currentLabel = sessionModeLabel(current.id, current.name);
    const tooltip = `${t('chat.sessionMode.label', lang)}: ${currentLabel}${current.description ? `\n${current.description}` : ''}`;

    const handleChange = (e: Event) => {
        const select = e.target as HTMLSelectElement;
        const nextId = select.value;
        if (nextId === current.id) return;
        if (isDangerousSessionMode(nextId)) {
            const next = modes.availableModes.find(m => m.id === nextId);
            const name = next ? sessionModeLabel(next.id, next.name) : nextId;
            if (!window.confirm(t('chat.sessionMode.dangerConfirm', lang, { name }))) {
                // Controlled revert: snap the native select back to the
                // still-authoritative current mode.
                select.value = current.id;
                return;
            }
        }
        onChange(nextId);
    };

    // A styled button face with an invisible native <select> stretched over
    // it — native dropdown UX (keyboard, mobile sheets) without giving up
    // the composer toolbar look shared with PermissionModePicker.
    return (
        <label
            class="chat-composer-mode-btn chat-session-mode-picker"
            data-mode-id={current.id}
            data-mode-danger={isDangerousSessionMode(current.id) ? 'true' : undefined}
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
                {current.id === 'plan' ? (
                    // Clipboard-list: planning
                    <g>
                        <rect x="6" y="4" width="12" height="17" rx="2" />
                        <path d="M9 4V2h6v2M9 10h6M9 14h6M9 18h4" />
                    </g>
                ) : (
                    // Sliders: mode selection
                    <g>
                        <path d="M4 8h10M18 8h2M4 16h2M10 16h10" />
                        <circle cx="16" cy="8" r="2" />
                        <circle cx="8" cy="16" r="2" />
                    </g>
                )}
            </svg>
            <span class="chat-composer-mode-label">{currentLabel}</span>
            <select
                class="chat-session-mode-select"
                value={current.id}
                disabled={disabled}
                aria-label={t('chat.sessionMode.label', lang)}
                onChange={handleChange}
            >
                {modes.availableModes.map(m => (
                    <option key={m.id} value={m.id}>
                        {sessionModeLabel(m.id, m.name)}
                    </option>
                ))}
            </select>
        </label>
    );
}
