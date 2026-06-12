import { signal } from '@preact/signals';

import type { TmuxWindow, Session, ChatSession } from '../components/types';

/**
 * Session state (tmux terminal windows, chat session index, active session).
 * Previously lived on App's god-state; now any consumer reads the signals
 * directly. Service-calling orchestration (loadTerminals, loadChatSessions,
 * createChatSession, …) stays in App.
 */

// ── Terminal / tmux state ──
export const terminalWindows = signal<TmuxWindow[]>([]);
export const terminalWindowsLoading = signal(false);
export const tmuxMouseOn = signal(true);

// ── Chat session state (1agents-side index) ──
export const chatSessions = signal<ChatSession[]>([]);
export const activeSession = signal<Session | null>(null);
export const pendingInitialMessage = signal<string | null>(null);
