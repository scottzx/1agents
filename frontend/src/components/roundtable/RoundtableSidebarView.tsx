import { h } from 'preact';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import { roleLabel, seatDisplayStatus, seatStatusLabel } from './roleLabels';

interface RoundtableSidebarViewProps {
    seats: RoundtableSeat[];
    liveMap?: Record<string, string | null | undefined>;
    openingId?: string | null;
    onSeatClick?: (seat: RoundtableSeat) => void | Promise<void>;
}

/** 固定角色面板：始终显示六席参与者列表（头像），无 Tab 切换。 */
export function RoundtableSidebarView({
    seats,
    liveMap = {},
    openingId = null,
    onSeatClick,
}: RoundtableSidebarViewProps) {
    const ordered = [...seats].sort((a, b) => roleOrder(a.role) - roleOrder(b.role));

    return (
        <aside class="rt-sidebar" aria-label="圆桌 Inspector">
            <section class="rt-side-section">
                <h3 class="rt-side-title">六席参与者</h3>
                <p class="rt-side-hint">查看状态；需要时打开对应席位的完整讨论。</p>
                <ul class="rt-side-seats">
                    {ordered.map(seat => {
                        const label = roleLabel(seat.role);
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
                                            ? `打开「${label}」完整讨论 · ${seatStatusLabel(ui)}`
                                            : '该席讨论尚未就绪'
                                    }
                                    onClick={() => void onSeatClick?.(seat)}
                                >
                                    <span class="rt-side-seat-avatar" aria-hidden="true">
                                        <span>{label.slice(0, 1)}</span>
                                        <span class={`rt-seat-dot is-${ui}`} />
                                    </span>
                                    <span class="rt-side-seat-name">{label}</span>
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

function roleOrder(role: string): number {
    const order = ['referee', 'market', 'product', 'eng', 'ops', 'finance'];
    const i = order.indexOf(role);
    return i < 0 ? 99 : i;
}
