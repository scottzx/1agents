import { h } from 'preact';
import { useMemo, useState } from 'preact/hooks';
import type { RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { renderMarkdown } from '../../utils/markdown';
import { resolveTurnAuthor } from './roleLabels';

interface TurnCardProps {
    turn: RoundtableTurn;
    seats: RoundtableSeat[];
}

/**
 * Timeline card for a *persisted* turn (design §6.1).
 * Body: content_text rendered via shared renderMarkdown (same as Chat bubbles).
 * Open full seat ChatUI from the right sidebar seat list — not embedded here.
 */
export function TurnCard({ turn, seats }: TurnCardProps) {
    const [processOpen, setProcessOpen] = useState(false);

    const name = resolveTurnAuthor(turn, seats);
    const kind = turn.kind || 'chat';
    const content = (turn.content_text || '').trim();
    const briefConfirmed = isBriefConfirmedTurn(turn);
    const hasContent = Boolean(content);
    const html = useMemo(() => (hasContent ? renderMarkdown(content) : ''), [content, hasContent]);

    if (briefConfirmed) {
        return (
            <article
                id={`rt-turn-${turn.id}`}
                class="rt-turn-event kind-brief-confirmed"
                data-turn-id={turn.id}
                aria-label="Brief 已确认"
            >
                <span class="rt-turn-event-dot" aria-hidden="true" />
                <span class="rt-turn-event-text">Brief 已确认</span>
                <span class="rt-turn-event-detail">进入首轮</span>
            </article>
        );
    }

    return (
        <article
            id={`rt-turn-${turn.id}`}
            class={`rt-turn-card kind-${kind}`}
            data-turn-id={turn.id}
            data-seat-id={turn.seat_id || undefined}
        >
            <header class="rt-turn-head">
                <div class="rt-turn-author">
                    <span class="rt-turn-role">{name}</span>
                    {kind !== 'chat' && kind !== 'speech' && <span class="rt-turn-kind-badge">{kindLabel(kind)}</span>}
                </div>
                {turn.round > 0 && <span class="rt-turn-round">R{turn.round}</span>}
            </header>

            <div class="rt-turn-body">
                {hasContent ? (
                    <div class="rt-turn-text markdown-body md-conv" dangerouslySetInnerHTML={{ __html: html }} />
                ) : (
                    <div class="rt-turn-speaking-placeholder" role="status">
                        <span class="rt-speaking-dots" aria-hidden="true">
                            <span />
                            <span />
                            <span />
                        </span>
                        <span>等待正文…</span>
                    </div>
                )}
            </div>

            {Boolean(turn.process_ref) && (
                <div class="rt-turn-process">
                    <button
                        type="button"
                        class="rt-process-toggle"
                        aria-expanded={processOpen}
                        onClick={() => setProcessOpen(v => !v)}
                    >
                        {processOpen ? '收起过程' : '查看过程'}
                    </button>
                    {processOpen && (
                        <div class="rt-process-panel chat-tool-output-box" role="region" aria-label="过程详情">
                            <div class="rt-process-note">详细过程可在右侧对应席位的会话中查看。</div>
                        </div>
                    )}
                </div>
            )}
        </article>
    );
}

/**
 * New records may use a dedicated kind; persisted rooms used a system turn
 * whose body starts with “Brief 已确认” and then repeats every Brief field.
 */
export function isBriefConfirmedTurn(turn: RoundtableTurn): boolean {
    const kind = (turn.kind || '').trim().toLowerCase();
    if (kind === 'brief_confirmed' || kind === 'system/brief_confirmed' || kind === 'system:brief_confirmed') {
        return true;
    }
    if (kind !== 'system') return false;

    const content = (turn.content_text || '').trim();
    return (
        /(?:已确认|confirmed)[\s\S]{0,24}brief/iu.test(content) ||
        /brief(?:[\s*_：:-]*v?\s*\d+)?[\s*_：:-]*(?:已确认|confirmed)/iu.test(content)
    );
}

function kindLabel(kind: string): string {
    switch (kind) {
        case 'summary':
            return '总结';
        case 'system':
            return '系统';
        case 'speech':
            return '发言';
        case 'chat':
            return '对话';
        default:
            return '事件';
    }
}
