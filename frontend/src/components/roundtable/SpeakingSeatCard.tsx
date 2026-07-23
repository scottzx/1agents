import { h } from 'preact';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import { EmbeddedChat } from '../chat/EmbeddedChat';
import { roleLabel } from './roleLabels';

/** Default embed height for speaking seats (tokenized in SCSS). */
export const RT_EMBED_MAX_HEIGHT = 320;

export interface SpeakingSeatCardProps {
    seat: RoundtableSeat;
    /** Display round number (R1/R2/R3). 0 hides the badge. */
    round?: number;
    /**
     * Allow Composer in the embed. Panelist seats stay readOnly;
     * referee never uses this card (bottom dock only).
     */
    showComposer?: boolean;
}

/**
 * Live *panelist* seat card while status === speaking: height-capped
 * EmbeddedChat bound to this seat's session only. RoundtableRoom filters
 * out referee — referee stream is the fixed bottom EmbeddedChat (#260).
 */
export function SpeakingSeatCard({ seat, round = 0, showComposer = false }: SpeakingSeatCardProps) {
    const name = roleLabel(seat.role);
    const sessionId = seat.session_id?.trim() || '';
    const hasSession = Boolean(sessionId);

    return (
        <article
            class="rt-turn-card is-speaking"
            data-seat-id={seat.id}
            data-session-id={sessionId || undefined}
            data-role={seat.role}
        >
            <header class="rt-turn-head">
                <div class="rt-turn-author">
                    <span class="rt-turn-role">{name}</span>
                    <span class="rt-turn-live-badge" aria-live="polite">
                        发言中
                    </span>
                </div>
                {round > 0 && <span class="rt-turn-round">R{round}</span>}
            </header>

            <div class="rt-turn-body rt-turn-embed">
                {hasSession ? (
                    <EmbeddedChat
                        sessionId={sessionId}
                        workspaceId={seat.workspace_id}
                        agentType={seat.agent_type}
                        acpSessionId={seat.acp_session_id}
                        maxHeight={RT_EMBED_MAX_HEIGHT}
                        readOnly={!showComposer}
                        className="rt-seat-embed"
                        emptyHint="正在接收…"
                    />
                ) : (
                    <div class="rt-turn-speaking-placeholder" role="status">
                        <span class="rt-speaking-dots" aria-hidden="true">
                            <span />
                            <span />
                            <span />
                        </span>
                        <span>正在输出…</span>
                    </div>
                )}
            </div>
        </article>
    );
}
