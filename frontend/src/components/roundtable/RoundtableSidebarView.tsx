import { h, type ComponentChildren } from 'preact';
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { roleLabel, seatDisplayStatus, seatStatusLabel } from './roleLabels';
import { BriefInspector } from './BriefInspector';

interface RoundtableSidebarViewProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
    turns: RoundtableTurn[];
    activeTab?: 'topic' | 'participants';
    onTabChange?: (tab: 'topic' | 'participants') => void;
    liveMap?: Record<string, string | null | undefined>;
    openingId?: string | null;
    onSeatClick?: (seat: RoundtableSeat) => void | Promise<void>;
    briefInspector?: ComponentChildren;
}

/** Brief owner, Summary status/anchor, and clickable seat list. */
export function RoundtableSidebarView({
    room,
    seats,
    turns,
    activeTab = 'topic',
    onTabChange,
    liveMap = {},
    openingId = null,
    onSeatClick,
    briefInspector,
}: RoundtableSidebarViewProps) {
    const summary = latestSummaryMeta(room, turns);
    const ordered = [...seats].sort((a, b) => roleOrder(a.role) - roleOrder(b.role));

    return (
        <aside class="rt-sidebar" aria-label="圆桌 Inspector">
            <div class="rt-inspector-tabs" role="tablist" aria-label="Inspector">
                <button
                    type="button"
                    role="tab"
                    id="rt-inspector-tab-topic"
                    aria-selected={activeTab === 'topic'}
                    aria-controls="rt-inspector-topic"
                    class={activeTab === 'topic' ? 'is-active' : ''}
                    onClick={() => onTabChange?.('topic')}
                >
                    议题
                </button>
                <button
                    type="button"
                    role="tab"
                    id="rt-inspector-tab-participants"
                    aria-selected={activeTab === 'participants'}
                    aria-controls="rt-inspector-participants"
                    class={activeTab === 'participants' ? 'is-active' : ''}
                    onClick={() => onTabChange?.('participants')}
                >
                    参与者
                </button>
            </div>

            <div
                id="rt-inspector-topic"
                class="rt-inspector-panel"
                role="tabpanel"
                aria-labelledby="rt-inspector-tab-topic"
                hidden={activeTab !== 'topic'}
            >
                {briefInspector || <BriefInspector room={room} readOnly />}

                <section class="rt-side-section">
                    <h3 class="rt-side-title">阶段总结</h3>
                    {summary ? (
                        <div class="rt-side-summary-status">
                            <span>
                                <span class="rt-side-summary-icon" aria-hidden="true">
                                    ✓
                                </span>
                                {summary.label}已生成
                            </span>
                            {summary.turnId ? (
                                <a class="rt-side-summary-anchor" href={`#rt-summary-${summary.round}-title`}>
                                    在主区查看
                                </a>
                            ) : (
                                <span class="rt-side-summary-location">正文仅在主区展示</span>
                            )}
                        </div>
                    ) : (
                        <p class="rt-side-empty">尚未形成阶段总结</p>
                    )}
                </section>
            </div>

            <div
                id="rt-inspector-participants"
                class="rt-inspector-panel"
                role="tabpanel"
                aria-labelledby="rt-inspector-tab-participants"
                hidden={activeTab !== 'participants'}
            >
                <section class="rt-side-section">
                    <h3 class="rt-side-title">六席参与者</h3>
                    <p class="rt-side-hint">查看状态；需要时打开对应席位的完整讨论。</p>
                    <ul class="rt-side-seats">
                        {ordered.map(seat => {
                            const sid = seat.session_id?.trim() || '';
                            const ui = seatDisplayStatus(seat, sid ? liveMap[sid] : undefined);
                            const canOpen = Boolean(sid && onSeatClick);
                            const isOpening = openingId === seat.id;
                            return (
                                <li key={seat.id}>
                                    <button
                                        type="button"
                                        class={`rt-side-seat is-${ui}${canOpen ? ' is-clickable' : ' is-disabled'}${
                                            isOpening ? ' is-opening' : ''
                                        }`}
                                        disabled={!canOpen || Boolean(openingId)}
                                        title={
                                            canOpen
                                                ? `打开「${roleLabel(seat.role)}」完整讨论 · ${seatStatusLabel(ui)}`
                                                : '该席讨论尚未就绪'
                                        }
                                        onClick={() => void onSeatClick?.(seat)}
                                    >
                                        <span class={`rt-seat-dot is-${ui}`} aria-hidden="true" />
                                        <span class="rt-side-seat-name">{roleLabel(seat.role)}</span>
                                        <span class="rt-side-seat-status">
                                            {isOpening ? '打开中…' : seatStatusLabel(ui)}
                                        </span>
                                    </button>
                                </li>
                            );
                        })}
                    </ul>
                </section>
            </div>
        </aside>
    );
}

function latestSummaryMeta(
    room: RoundtableRoom,
    turns: RoundtableTurn[]
): { label: string; round: 2 | 3; turnId?: string } | null {
    const round = room.summary_r3 ? 3 : room.summary_r2 ? 2 : 0;
    if (!round) return null;

    const turn = [...turns].reverse().find(item => item.kind === 'summary' && item.round === round);
    return {
        label: round === 3 ? '终稿' : '首轮总结',
        round,
        ...(turn ? { turnId: turn.id } : {}),
    };
}

function roleOrder(role: string): number {
    const order = ['referee', 'market', 'product', 'eng', 'ops', 'finance'];
    const i = order.indexOf(role);
    return i < 0 ? 99 : i;
}
