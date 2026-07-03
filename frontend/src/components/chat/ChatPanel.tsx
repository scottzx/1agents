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
import { SessionTakenOverBanner } from './SessionTakenOverBanner';

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
        connection,
        typing,
        ready,
        permissionMode,
        modes,
        availableCommands,
        usage,
        plan,
        send,
        cancel,
        cancelQueued,
        respondPermission,
        setPermissionMode,
        setSessionMode,
        takenOver,
        retry,
    } = useBridge(session);

    // Local dismiss state for the takeover banner. useSignal (not useState):
    // plain useState toggles are known to not re-render under @preact/signals
    // v2 in this repo. Reset to "not dismissed" whenever a new takeover fires.
    const bannerDismissed = useSignal(false);
    if (!takenOver && bannerDismissed.value) bannerDismissed.value = false;

    // The composer is only usable once BOTH the WS handshake finished
    // AND the bridge has confirmed the session is initialized. The
    // latter is the new gate — without it, the first user prompt on a
    // brand-new session would race `session_ready` and bounce with
    // SESSION_NOT_FOUND.
    const composerDisabled = (connection !== 'connected' && connection !== 'reconnecting') || !ready;

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
            {currentModeId === 'plan' && (
                <div class="chat-plan-banner">{t('chat.sessionMode.planBanner', ui.language.value)}</div>
            )}
            {plan && plan.length > 0 && <PlanChecklist entries={plan} />}
            <MessageList
                items={items}
                agentType={session.agentType}
                typing={typing}
                emptyHint={
                    connection === 'connecting'
                        ? t('chat.connecting', ui.language.value)
                        : t('chat.empty.send', ui.language.value)
                }
                loading={showInitLoading}
                onRespondPermission={respondPermission}
                onCancelQueued={cancelQueued}
            />
            <Composer
                onSend={send}
                onCancel={cancel}
                isRunning={typing}
                disabled={composerDisabled}
                permissionMode={permissionMode}
                onPermissionModeChange={setPermissionMode}
                sessionModes={modes}
                onSessionModeChange={setSessionMode}
                availableCommands={availableCommands}
                usage={usage}
            />
        </div>
    );
}
