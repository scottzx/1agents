/**
 * EmbeddedChat — height-capped, optionally read-only chat surface for
 * embedding into cards (roundtable speech cards, process panels, …).
 *
 * Reuses the same bridge + MessageList stack as ChatPanel so typing waves
 * and streaming bubbles match the main ChatUI. It is intentionally NOT a
 * full session stage: no auth modal orchestration, no plan banner, no
 * session-store selection side effects.
 *
 * ## Props contract
 *
 * | prop          | role |
 * |---------------|------|
 * | session       | full ChatSession (preferred when you already have one) |
 * | sessionId     | id-only path; pair with workspaceId (+ agentType) when unknown to store |
 * | workspaceId   | required for stub sessions not already in chatSessions |
 * | agentType     | WS connect param; defaults to claudecode on stubs |
 * | acpSessionId  | resume hint when known (seat.acp_session_id) |
 * | maxHeight     | shell max-height (number → px); message list scrolls inside |
 * | readOnly      | hide Composer; typing/streaming still visible |
 * | showComposer  | override input visibility when not readOnly (default true) |
 * | onSend        | optional send override (e.g. roundtable R1 API); default = bridge send |
 * | isRunning     | optional Composer running flag; default = bridge typing |
 *
 * ## Bridge side-subscribe (does not hijack main tab / sidebar)
 *
 * - Uses `useBridge` → `globalBridgeManager.getOrCreate(session)`.
 * - Bridge state is keyed by `session.id`. Multiple components can listen
 *   on the same session (`listeners` Set); they share one WebSocket.
 * - This component never calls `selectSession` / writes `activeSession`,
 *   so the left-sidebar "current session" is unchanged.
 * - Unmount only removes the local listener; it does not `destroy()` the
 *   bridge (same as leaving ChatPanel via tab switch).
 * - sessionStore: optional. If `sessionId` is already in `chatSessions`,
 *   fields are reused. Seat sessions not listed in the store need a
 *   minimal mount: `sessionId` + `workspaceId` + `agentType` (and
 *   `acpSessionId` when known). No store registration is required.
 *
 * ## Story (readOnly)
 *
 * ```tsx
 * // Live seat stream inside a turn card — watch only
 * <EmbeddedChat
 *   sessionId={seat.session_id}
 *   workspaceId={seat.workspace_id}
 *   agentType={seat.agent_type}
 *   acpSessionId={seat.acp_session_id}
 *   maxHeight={240}
 *   readOnly
 * />
 *
 * // Referee can type (R1) while still height-capped
 * <EmbeddedChat session={refSession} maxHeight={320} readOnly={false} />
 * ```
 */

import { h } from 'preact';
import { useMemo } from 'preact/hooks';
import type { ChatSession } from '../types';
import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import { chatSessions } from '../../stores/sessionStore';
import { useBridge } from './hooks';
import { MessageList } from './MessageList';
import { Composer } from './Composer';
import {
    formatMaxHeight,
    resolveEmbeddedSession,
    shouldShowComposer,
    type EmbeddedChatSessionInput,
} from './embeddedChatUtils';

export type { EmbeddedChatSessionInput };

export interface EmbeddedChatProps extends EmbeddedChatSessionInput {
    /**
     * Container max-height. Number is treated as px; string is used as-is
     * (e.g. '40vh'). Default 280px.
     */
    maxHeight?: number | string;
    /**
     * When true, Composer is hidden. Typing wave and streaming bubbles
     * still render via MessageList.
     */
    readOnly?: boolean;
    /**
     * Explicit composer toggle when not readOnly. Defaults to true.
     * Ignored when readOnly is true.
     */
    showComposer?: boolean;
    /**
     * When set, Composer sends through this callback instead of the bridge.
     * Use for orchestrated flows (e.g. POST /roundtable/.../chat) that must
     * still show the same session stream via useBridge.
     */
    onSend?: (text: string) => void;
    /**
     * Force Composer "running" (cancel affordance / disable send). Defaults
     * to bridge `typing` when omitted.
     */
    isRunning?: boolean;
    /** Extra class on the shell (e.g. parent card variants). */
    className?: string;
    emptyHint?: string;
}

export function EmbeddedChat({
    session: sessionProp,
    sessionId,
    workspaceId,
    agentType,
    acpSessionId,
    maxHeight,
    readOnly = false,
    showComposer,
    onSend: onSendProp,
    isRunning: isRunningProp,
    className,
    emptyHint,
}: EmbeddedChatProps) {
    // Resolve once per identity change. chatSessions is a signal — reading
    // .value here keeps store-backed sessions in sync when the list updates.
    const storeList = chatSessions.value;
    const session: ChatSession | null = useMemo(
        () =>
            resolveEmbeddedSession(
                { session: sessionProp, sessionId, workspaceId, agentType, acpSessionId },
                storeList
            ),
        // sessionProp identity + id fields; storeList ref changes when sessions reload
        [sessionProp, sessionId, workspaceId, agentType, acpSessionId, storeList]
    );

    const {
        items,
        connection,
        typing,
        ready,
        permissionMode,
        modes,
        availableCommands,
        configOptions,
        usage,
        send,
        cancel,
        cancelQueued,
        setPermissionMode,
        setSessionMode,
        setConfigOption,
    } = useBridge(session);

    const composerVisible = shouldShowComposer(readOnly, showComposer);
    // Orchestrated onSend (roundtable R1) does not need bridge ready — only WS stream does.
    const bridgeGate = (connection !== 'connected' && connection !== 'reconnecting') || !ready;
    const composerDisabled = onSendProp ? Boolean(isRunningProp) : bridgeGate;
    const showInitLoading = connection === 'connected' && !ready;
    const heightCss = formatMaxHeight(maxHeight);
    const composerSend = onSendProp ?? send;
    const composerRunning = isRunningProp ?? typing;

    if (!session) {
        return (
            <div
                class={`embedded-chat is-empty${className ? ` ${className}` : ''}`}
                style={{ maxHeight: heightCss }}
                data-readonly={readOnly ? 'true' : 'false'}
            >
                <div class="chat-empty">
                    <p>{emptyHint ?? t('chat.empty.send', ui.language.value)}</p>
                </div>
            </div>
        );
    }

    return (
        <div
            class={`embedded-chat${className ? ` ${className}` : ''}`}
            style={{ maxHeight: heightCss }}
            data-session-id={session.id}
            data-readonly={readOnly ? 'true' : 'false'}
            data-composer={composerVisible ? 'true' : 'false'}
        >
            <MessageList
                items={items}
                typing={typing}
                emptyHint={
                    emptyHint ??
                    (connection === 'connecting'
                        ? t('chat.connecting', ui.language.value)
                        : t('chat.empty.send', ui.language.value))
                }
                loading={showInitLoading}
                onCancelQueued={composerVisible ? cancelQueued : undefined}
            />
            {composerVisible && (
                <Composer
                    sessionId={session.id}
                    onSend={composerSend}
                    onCancel={cancel}
                    isRunning={composerRunning}
                    disabled={composerDisabled}
                    permissionMode={permissionMode}
                    onPermissionModeChange={setPermissionMode}
                    sessionModes={modes}
                    onSessionModeChange={setSessionMode}
                    availableCommands={availableCommands}
                    usage={usage}
                    configOptions={configOptions}
                    onConfigOptionChange={setConfigOption}
                />
            )}
        </div>
    );
}
