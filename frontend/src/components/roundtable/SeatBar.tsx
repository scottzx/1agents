import { h } from 'preact';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import { liveSessionStatus } from '../../stores/sessionStore';
import { roleLabel, seatDisplayStatus, seatStatusLabel } from './roleLabels';

interface SeatBarProps {
    seats: RoundtableSeat[];
}

/**
 * In-progress seat strip: running / blocked / done / error (design §6.1).
 * Status lamp prefers live session status when the seat has a session_id.
 */
export function SeatBar({ seats }: SeatBarProps) {
    if (!seats.length) return null;

    const liveMap = liveSessionStatus.value;
    const ordered = [...seats].sort((a, b) => roleOrder(a.role) - roleOrder(b.role));

    return (
        <div class="rt-seat-bar" role="list" aria-label="席位状态">
            {ordered.map(seat => {
                const sid = seat.session_id?.trim() || '';
                const ui = seatDisplayStatus(seat, sid ? liveMap[sid] : undefined);
                return (
                    <div
                        key={seat.id}
                        class={`rt-seat-chip is-${ui}`}
                        role="listitem"
                        title={`${roleLabel(seat.role)} · ${seatStatusLabel(ui)}`}
                    >
                        <span class={`rt-seat-dot is-${ui}`} aria-hidden="true" />
                        <span class="rt-seat-name">{roleLabel(seat.role)}</span>
                        <span class="rt-seat-status">{seatStatusLabel(ui)}</span>
                    </div>
                );
            })}
        </div>
    );
}

function roleOrder(role: string): number {
    const order = ['referee', 'market', 'product', 'eng', 'ops', 'finance'];
    const i = order.indexOf(role);
    return i < 0 ? 99 : i;
}
