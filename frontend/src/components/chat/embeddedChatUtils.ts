// Pure helpers for EmbeddedChat — props contract + session resolution.
// Kept free of preact so node:test can exercise them without a DOM.
//
// Story-level contract (also documented on EmbeddedChat):
//   sessionId | session  → resolve a ChatSession for useBridge
//   maxHeight            → CSS max-height on the embed shell
//   readOnly=true        → hide Composer; typing/streaming still render
//   showComposer         → optional override when not readOnly
//
// Bridge side-subscribe: useBridge → globalBridgeManager.getOrCreate is
// keyed by session.id. Multiple listeners share one WS; the embed never
// calls selectSession / activeSession, so the sidebar current chat is
// untouched. sessionStore registration is optional — if the id is already
// in chatSessions we reuse its fields; otherwise a minimal stub is enough
// for ChatBridgeManager.connect (id + workspaceId + agentType).

import type { AgentType, ChatSession } from '../types';

/** Inputs accepted when resolving a session for the embed. */
export interface EmbeddedChatSessionInput {
    /** Full session when available (preferred). */
    session?: ChatSession | null;
    /** Session id path when `session` is omitted. */
    sessionId?: string;
    /** Required for stub connect when the id is not in sessionStore. */
    workspaceId?: string;
    agentType?: string;
    acpSessionId?: string;
}

/**
 * Optional list lookup (typically chatSessions.value). Pure: caller passes
 * the array so tests don't need the live store.
 */
export function resolveEmbeddedSession(
    input: EmbeddedChatSessionInput,
    storeSessions: readonly ChatSession[] = []
): ChatSession | null {
    if (input.session) {
        return input.session;
    }

    const id = input.sessionId?.trim();
    if (!id) return null;

    const fromStore = storeSessions.find(c => c.id === id);
    if (fromStore) {
        return {
            ...fromStore,
            ...(input.workspaceId ? { workspaceId: input.workspaceId } : {}),
            ...(input.agentType ? { agentType: input.agentType as AgentType } : {}),
            ...(input.acpSessionId ? { acpSessionId: input.acpSessionId } : {}),
        };
    }

    // Minimal stub — enough for ChatBridgeManager.connect WS query params.
    // Without workspaceId the backend cannot attach the session handle.
    if (!input.workspaceId?.trim()) return null;

    return {
        kind: 'chat',
        id,
        workspaceId: input.workspaceId.trim(),
        name: '',
        agentType: (input.agentType?.trim() || 'claudecode') as AgentType,
        ccProject: '',
        ccSessionId: '',
        sessionKey: '',
        status: 'idle',
        active: false,
        ...(input.acpSessionId ? { acpSessionId: input.acpSessionId } : {}),
    };
}

/**
 * Composer visibility contract:
 * - readOnly=true always hides input (typing/streaming still show)
 * - showComposer defaults to true when not readOnly
 * - showComposer=false hides input without implying read-only semantics
 */
export function shouldShowComposer(readOnly?: boolean, showComposer?: boolean): boolean {
    if (readOnly) return false;
    return showComposer ?? true;
}

/** Normalize maxHeight prop to a CSS length string. */
export function formatMaxHeight(maxHeight?: number | string): string {
    if (maxHeight === undefined || maxHeight === null || maxHeight === '') {
        return '280px';
    }
    if (typeof maxHeight === 'number' && Number.isFinite(maxHeight)) {
        return `${maxHeight}px`;
    }
    return String(maxHeight);
}
