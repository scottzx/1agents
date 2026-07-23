import { h } from 'preact';
import { useState } from 'preact/hooks';
import type { RoundtableBrief, RoundtableRoom, RoundtableSeat } from '@1agents/core/services/roundtableService';
import { liveSessionStatus } from '../../stores/sessionStore';
import { roleLabel, seatDisplayStatus, seatStatusLabel } from './roleLabels';
import { openSeatSession } from './openSeatSession';

interface RoundtableSidebarProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
}

/**
 * Minimal side panel: Brief, latest Summary, clickable seat list (design §6.2).
 * Click a seat → open that session in main ChatUI; status lamp follows live
 * session status (进行中 / 阻塞 / 结束 / …).
 */
export function RoundtableSidebar({ room, seats }: RoundtableSidebarProps) {
    const brief = room.brief;
    const latestSummary = room.summary_r3 || room.summary_r2 || '';
    const summaryLabel = room.summary_r3 ? '终稿 Summary₃' : room.summary_r2 ? 'Summary₂' : '最新 Summary';
    const ordered = [...seats].sort((a, b) => roleOrder(a.role) - roleOrder(b.role));
    const [openingId, setOpeningId] = useState<string | null>(null);

    // Subscribe to live bridge status so lamps repaint while seats stream.
    const liveMap = liveSessionStatus.value;

    const onSeatClick = async (seat: RoundtableSeat) => {
        if (!seat.session_id?.trim() || openingId) return;
        setOpeningId(seat.id);
        try {
            await openSeatSession(seat);
        } finally {
            setOpeningId(null);
        }
    };

    return (
        <aside class="rt-sidebar" aria-label="圆桌侧栏">
            <section class="rt-side-section">
                <h3 class="rt-side-title">Brief</h3>
                {brief ? <BriefBlock brief={brief} /> : <p class="rt-side-empty">尚未确认 Brief</p>}
            </section>

            <section class="rt-side-section">
                <h3 class="rt-side-title">{summaryLabel}</h3>
                {latestSummary ? (
                    <div class="rt-side-summary">{latestSummary}</div>
                ) : (
                    <p class="rt-side-empty">尚无总结</p>
                )}
            </section>

            <section class="rt-side-section">
                <h3 class="rt-side-title">席位</h3>
                <p class="rt-side-hint">点击席位打开完整会话</p>
                <ul class="rt-side-seats">
                    {ordered.map(s => {
                        const sid = s.session_id?.trim() || '';
                        const live = sid ? liveMap[sid] : undefined;
                        const ui = seatDisplayStatus(s, live);
                        const canOpen = Boolean(sid);
                        const isOpening = openingId === s.id;
                        return (
                            <li key={s.id}>
                                <button
                                    type="button"
                                    class={`rt-side-seat is-${ui}${canOpen ? ' is-clickable' : ' is-disabled'}${
                                        isOpening ? ' is-opening' : ''
                                    }`}
                                    disabled={!canOpen || Boolean(openingId)}
                                    title={
                                        canOpen
                                            ? `打开「${roleLabel(s.role)}」会话 · ${seatStatusLabel(ui)}`
                                            : '会话尚未就绪'
                                    }
                                    onClick={() => void onSeatClick(s)}
                                >
                                    <span class={`rt-seat-dot is-${ui}`} aria-hidden="true" />
                                    <span class="rt-side-seat-name">{roleLabel(s.role)}</span>
                                    <span class="rt-side-seat-status">
                                        {isOpening ? '打开中…' : seatStatusLabel(ui)}
                                    </span>
                                </button>
                            </li>
                        );
                    })}
                </ul>
            </section>
        </aside>
    );
}

function BriefBlock({ brief }: { brief: RoundtableBrief }) {
    return (
        <dl class="rt-brief">
            <div class="rt-brief-row">
                <dt>标题</dt>
                <dd>{brief.title || '—'}</dd>
            </div>
            <div class="rt-brief-row">
                <dt>议题</dt>
                <dd>{brief.question || '—'}</dd>
            </div>
            {brief.constraints ? (
                <div class="rt-brief-row">
                    <dt>约束</dt>
                    <dd>{brief.constraints}</dd>
                </div>
            ) : null}
            {brief.success_criteria ? (
                <div class="rt-brief-row">
                    <dt>成功标准</dt>
                    <dd>{brief.success_criteria}</dd>
                </div>
            ) : null}
            {brief.product_kind ? (
                <div class="rt-brief-row">
                    <dt>品类</dt>
                    <dd>{brief.product_kind}</dd>
                </div>
            ) : null}
        </dl>
    );
}

function roleOrder(role: string): number {
    const order = ['referee', 'market', 'product', 'eng', 'ops', 'finance'];
    const i = order.indexOf(role);
    return i < 0 ? 99 : i;
}
