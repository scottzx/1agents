import { signal, effect } from '@preact/signals';

import { DEFAULT_AGENT_TYPE } from '../services/agentService';
import type { AgentType } from '../components/types';

/**
 * Global (per-device) defaults for the unified "new session" setup form
 * (P0-1 of 统一新建会话). The same form powers SessionSetupModal and the
 * NewChatHome config panel; `skipModal` lets frequent users skip the
 * modal entirely.
 *
 * Scope is **global** by product decision (PRD §1.3, §6) — not per-workspace.
 * Persisted as one localStorage entry under `1agents_session_setup_defaults`.
 * A module-level `effect` mirrors the signal back to storage on every change;
 * storage failures (private mode / quota / disabled) and malformed JSON both
 * fall back to the safe defaults without throwing.
 */

export type SessionMode = 'chat' | 'terminal';

export type TerminalPreset = 'claude' | 'codex' | 'gemini' | 'shell';

export interface SessionSetupDefaults {
    mode: SessionMode;
    agentType: AgentType;
    terminalPreset?: TerminalPreset;
    skipModal: boolean;
}

export const DEFAULT_SESSION_SETUP: SessionSetupDefaults = {
    mode: 'chat',
    agentType: DEFAULT_AGENT_TYPE,
    skipModal: false,
};

const STORAGE_KEY = '1agents_session_setup_defaults';

const VALID_MODES: readonly SessionMode[] = ['chat', 'terminal'];
const VALID_PRESETS: readonly TerminalPreset[] = ['claude', 'codex', 'gemini', 'shell'];

const isObject = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v);

const sanitize = (raw: unknown): SessionSetupDefaults => {
    const obj = isObject(raw) ? raw : {};
    const mode: SessionMode = VALID_MODES.includes(obj.mode as SessionMode)
        ? (obj.mode as SessionMode)
        : DEFAULT_SESSION_SETUP.mode;
    const base: SessionSetupDefaults = {
        mode,
        agentType:
            typeof obj.agentType === 'string' && obj.agentType.length > 0
                ? (obj.agentType as AgentType)
                : DEFAULT_SESSION_SETUP.agentType,
        skipModal: typeof obj.skipModal === 'boolean' ? obj.skipModal : DEFAULT_SESSION_SETUP.skipModal,
    };
    if (VALID_PRESETS.includes(obj.terminalPreset as TerminalPreset)) {
        base.terminalPreset = obj.terminalPreset as TerminalPreset;
    }
    return base;
};

const loadRaw = (): SessionSetupDefaults => {
    try {
        const raw = localStorage.getItem(STORAGE_KEY);
        if (!raw) return DEFAULT_SESSION_SETUP;
        return sanitize(JSON.parse(raw));
    } catch {
        return DEFAULT_SESSION_SETUP;
    }
};

export const sessionSetupDefaults = signal<SessionSetupDefaults>(loadRaw());

effect(() => {
    try {
        localStorage.setItem(STORAGE_KEY, JSON.stringify(sessionSetupDefaults.value));
    } catch {
        /* storage disabled / quota — non-fatal */
    }
});

/** Re-read from localStorage (e.g. after cross-tab updates). */
export const loadSessionSetupDefaults = (): SessionSetupDefaults => {
    const next = loadRaw();
    sessionSetupDefaults.value = next;
    return next;
};

/** Patch-and-persist. Returns the post-write value for convenience. */
export const saveSessionSetupDefaults = (patch: Partial<SessionSetupDefaults>): SessionSetupDefaults => {
    const next = sanitize({ ...sessionSetupDefaults.value, ...patch });
    sessionSetupDefaults.value = next;
    return next;
};
