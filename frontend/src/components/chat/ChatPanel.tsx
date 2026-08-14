import { h, Component } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { ChatSession } from '../types';
import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import { useBridge } from './hooks';
import { MessageList } from './MessageList';
import { Composer } from './Composer';
import { PlanChecklist } from './PlanChecklist';
import { BackgroundTasks } from './BackgroundTasks';
import { SessionTakenOverBanner } from './SessionTakenOverBanner';
import { ChatErrorBanner } from './ChatErrorBanner';
import { SessionAuthBadge } from './SessionAuthBadge';
import {
    AskUserPrompt,
    ExitPlanPrompt,
    PermissionPrompt,
    type PendingAskUser,
    type PendingExitPlan,
    type PendingPermission,
} from './MessageBubble';
import { closeAuthRequiredModal, openAuthRequiredModal } from '../../stores/modalStore';
import type { ChatItem } from './hooks';
import { useProjectedTurnItems } from './projectTurnProjection';
import { turnFocusRequest } from '../../stores/turnFocusStore';

interface ChatPanelProps {
    session: ChatSession;
    pendingInitialMessage?: string | null;
    onClearPendingInitialMessage?: () => void;
}

export class ChatPanel extends Component<ChatPanelProps> {
    render() {
        return (
            <ChatPanelInner
                session={this.props.session}
                pendingInitialMessage={this.props.pendingInitialMessage}
                onClearPendingInitialMessage={this.props.onClearPendingInitialMessage}
            />
        );
    }
}

/**
 * Inner component so we can use the useBridge hook (a functional
 * component rule) while keeping the public class-based API for
 * symmetry with the rest of the codebase.
 */
function ChatPanelInner({ session, pendingInitialMessage, onClearPendingInitialMessage }: ChatPanelProps) {
    const {
        items,
        authState,
        connection,
        typing,
        ready,
        permissionMode,
        modes,
        availableCommands,
        configOptions,
        usage,
        plan,
        backgroundTasks,
        send,
        logout,
        cancel,
        cancelQueued,
        respondPermission,
        respondAskUserQuestion,
        respondExitPlanMode,
        setPermissionMode,
        setSessionMode,
        setConfigOption,
        takenOver,
        retry,
        lastError,
        dismissError,
    } = useBridge(session);

    // Local dismiss state for the takeover banner. useSignal (not useState):
    // plain useState toggles are known to not re-render under @preact/signals
    // v2 in this repo. Reset to "not dismissed" whenever a new takeover fires.
    const bannerDismissed = useSignal(false);
    const pendingEdit = useSignal<{ turnId: string; text: string } | null>(null);
    const composerRefill = useSignal<{ text: string; nonce: number } | null>(null);
    if (!takenOver && bannerDismissed.value) bannerDismissed.value = false;

    // The composer is only usable once BOTH the WS handshake finished
    // AND the bridge has confirmed the session is initialized. The
    // latter is the new gate — without it, the first user prompt on a
    // brand-new session would race `session_ready` and bounce with
    // SESSION_NOT_FOUND.
    const composerDisabled = (connection !== 'connected' && connection !== 'reconnecting') || !ready;

    // Keep authentication modal orchestration close to the auth badge after
    // removing the redundant header wrapper.
    useEffect(() => {
        if (authState?.status === 'auth_required' && authState.methods.length > 0) {
            openAuthRequiredModal(session.id, authState.methods, authState.message);
        } else if (authState?.status === 'authenticated' || authState?.status === 'logged_out') {
            closeAuthRequiredModal();
        }
    }, [authState?.status, authState?.methods, authState?.message, session.id]);

    // New-chat home flow (192ab6a): fire the pending initial message once
    // the session is usable, then clear it so reconnects don't resend.
    useEffect(() => {
        if (!pendingInitialMessage || composerDisabled) return;
        send(pendingInitialMessage);
        onClearPendingInitialMessage?.();
        // (send/onClear identities churn per render; the two deps above are the real signals)
    }, [pendingInitialMessage, composerDisabled]);

    // Show a spinner placeholder while the WebSocket is open but the
    // bridge hasn't confirmed the session yet. For reconnecting/error
    // states the existing status bar / empty hint is more accurate.
    const showInitLoading = connection === 'connected' && !ready;

    // Native session mode drives panel-level styling: plan mode shows a
    // banner (read-only analysis), dangerous modes tint the composer. The
    // attribute mirrors the live mode so ExitPlanMode clears it via
    // mode_changed without extra wiring.
    const currentModeId = modes?.currentModeId;

    // Unresolved permissions / questionnaires / plan exits float above the
    // composer. Show them one-at-a-time in stream order so parallel tool
    // prompts (or reconnect redelivery) never stack a wall of cards.
    // Resolved ones stay as receipts inside the (default-collapsed) tool group.
    const nextPrompt = collectNextPendingPrompt(items);
    const projectedItems = useProjectedTurnItems(session, items, typing);
    const requestedEdit = pendingEdit.value;
    useEffect(() => {
        if (!requestedEdit) return;
        const cancelled = projectedItems.some(
            item => item.kind === 'user' && item.turnId === requestedEdit.turnId && item.turnStatus === 'cancelled'
        );
        if (!cancelled) return;
        composerRefill.value = { text: requestedEdit.text, nonce: Date.now() };
        pendingEdit.value = null;
    }, [projectedItems, requestedEdit]);

    const editTurn = (turnId: string, text: string, status: 'queued' | 'running') => {
        pendingEdit.value = { turnId, text };
        if (status === 'queued') {
            cancelQueued(turnId);
        } else {
            cancel();
        }
    };
    const focusRequest =
        turnFocusRequest.value?.sessionId === session.id
            ? {
                  turnId: turnFocusRequest.value.turnId,
                  aliases: turnFocusRequest.value.aliases,
                  nonce: turnFocusRequest.value.nonce,
              }
            : null;

    return (
        <div class="chat-panel" data-session-mode={currentModeId}>
            {takenOver && !bannerDismissed.value && (
                <SessionTakenOverBanner
                    onRetry={() => {
                        bannerDismissed.value = false;
                        retry();
                    }}
                    onDismiss={() => {
                        bannerDismissed.value = true;
                    }}
                />
            )}
            <SessionAuthBadge sessionId={session.id} onLogout={logout} />
            {currentModeId === 'plan' && (
                <div class="chat-plan-banner">{t('chat.sessionMode.planBanner', ui.language.value)}</div>
            )}
            {plan && plan.length > 0 && <PlanChecklist entries={plan} />}
            {backgroundTasks && backgroundTasks.length > 0 && <BackgroundTasks tasks={backgroundTasks} />}
            <MessageList
                items={projectedItems}
                typing={typing}
                emptyHint={
                    connection === 'connecting'
                        ? t('chat.connecting', ui.language.value)
                        : t('chat.empty.send', ui.language.value)
                }
                loading={showInitLoading}
                onCancelQueued={cancelQueued}
                onEditTurn={editTurn}
                focusTurn={focusRequest}
            />
            {/* Page-persistent error banner — sits above the Composer. Cleared
                by the × button or by an F5 reload; new errors overwrite old. */}
            {lastError && (
                <ChatErrorBanner message={lastError.message} code={lastError.code} onDismiss={dismissError} />
            )}
            {nextPrompt?.kind === 'permission' && (
                <PermissionPrompt permissions={[nextPrompt.data]} onRespond={respondPermission} />
            )}
            {nextPrompt?.kind === 'ask_user' && (
                <AskUserPrompt requests={[nextPrompt.data]} onRespond={respondAskUserQuestion} />
            )}
            {nextPrompt?.kind === 'exit_plan' && (
                <ExitPlanPrompt requests={[nextPrompt.data]} onRespond={respondExitPlanMode} />
            )}
            <Composer
                sessionId={session.id}
                onSend={send}
                onCancel={cancel}
                isRunning={typing}
                disabled={composerDisabled || pendingEdit.value !== null}
                permissionMode={permissionMode}
                onPermissionModeChange={setPermissionMode}
                sessionModes={modes}
                onSessionModeChange={setSessionMode}
                availableCommands={availableCommands}
                usage={usage}
                configOptions={configOptions}
                onConfigOptionChange={setConfigOption}
                draftRefill={composerRefill.value}
            />
        </div>
    );
}

/** One composer-adjacent interactive prompt waiting for the user. */
type PendingPrompt =
    | { kind: 'permission'; data: PendingPermission }
    | { kind: 'ask_user'; data: PendingAskUser }
    | { kind: 'exit_plan'; data: PendingExitPlan };

/**
 * Walk chat items in stream order and return the earliest unresolved
 * permission / ask_user_question / exit_plan_mode. Parallel tool calls or
 * reconnect redelivery can leave several pending; the UI surfaces only this
 * head so the next appears after the user answers the current one.
 */
function collectNextPendingPrompt(items: ChatItem[]): PendingPrompt | null {
    const seen = new Set<string>();

    const takePermission = (p: PendingPermission): PendingPrompt | null => {
        if (!p.requestId || seen.has(p.requestId) || p.resolved) return null;
        seen.add(p.requestId);
        return { kind: 'permission', data: p };
    };
    const takeAskUser = (p: PendingAskUser): PendingPrompt | null => {
        if (!p.requestId || seen.has(p.requestId) || p.resolved) return null;
        seen.add(p.requestId);
        return { kind: 'ask_user', data: p };
    };
    const takeExitPlan = (p: PendingExitPlan): PendingPrompt | null => {
        if (!p.requestId || seen.has(p.requestId) || p.resolved) return null;
        seen.add(p.requestId);
        return { kind: 'exit_plan', data: p };
    };

    for (const item of items) {
        if (item.kind === 'permission_request') {
            const hit = takePermission({
                requestId: item.requestId,
                toolName: item.toolName,
                input: item.input,
                options: item.options,
                ...(item.resolved ? { resolved: item.resolved } : {}),
            });
            if (hit) return hit;
            continue;
        }
        if (item.kind === 'ask_user_question') {
            const hit = takeAskUser({
                requestId: item.requestId,
                toolCallId: item.toolCallId,
                mode: item.mode,
                questions: item.questions,
                ...(item.resolved ? { resolved: item.resolved } : {}),
                ...(item.answers ? { answers: item.answers } : {}),
            });
            if (hit) return hit;
            continue;
        }
        if (item.kind === 'exit_plan_mode') {
            const hit = takeExitPlan({
                requestId: item.requestId,
                toolCallId: item.toolCallId,
                planContent: item.planContent,
                ...(item.resolved ? { resolved: item.resolved } : {}),
                ...(item.comments ? { comments: item.comments } : {}),
            });
            if (hit) return hit;
            continue;
        }
        if (item.kind === 'tool_use') {
            // Within a tool group, preserve call order; permission / askUser /
            // exitPlan on the same call are mutually exclusive in practice.
            for (const call of item.calls) {
                if (call.permission) {
                    const hit = takePermission(call.permission);
                    if (hit) return hit;
                }
                if (call.askUser) {
                    const hit = takeAskUser(call.askUser);
                    if (hit) return hit;
                }
                if (call.exitPlan) {
                    const hit = takeExitPlan(call.exitPlan);
                    if (hit) return hit;
                }
            }
        }
    }
    return null;
}
