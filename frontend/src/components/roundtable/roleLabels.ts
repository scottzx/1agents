import type { RoundtableSeat, RoundtableTurn, SeatRole, SeatStatus } from '@1agents/core/services/roundtableService';

/** Chinese display names for fixed roster roles (backend RoleLabel). */
export const ROLE_LABELS: Record<SeatRole, string> = {
    referee: '裁判',
    market: '市场',
    product: '产品',
    eng: '研发',
    ops: '运营',
    finance: '财务',
};

export function roleLabel(role: string | undefined): string {
    if (!role) return '未知';
    if (role in ROLE_LABELS) return ROLE_LABELS[role as SeatRole];
    return role;
}

/** Resolve turn author display name (function seat, 用户, 系统). */
export function resolveTurnAuthor(turn: RoundtableTurn, seats: RoundtableSeat[]): string {
    if (turn.seat_id === 'user') return '用户';
    if (turn.kind === 'system') return '系统';
    if (turn.seat_id) {
        const seat = seats.find(s => s.id === turn.seat_id);
        if (seat) return roleLabel(seat.role);
    }
    if (turn.kind === 'summary') return '裁判';
    return '未知';
}

/**
 * Unified seat lamp / label keys for UI:
 * - running  进行中 (seat speaking or session streaming)
 * - blocked  阻塞   (awaiting_permission)
 * - done     结束
 * - error    错误
 * - ready    就绪
 */
export type SeatUiStatus = 'running' | 'blocked' | 'done' | 'error' | 'ready';

/** Map orchestrator seat.status only (no live bridge). */
export function seatUiStatus(status: SeatStatus | string | undefined): SeatUiStatus {
    switch (status) {
        case 'speaking':
            return 'running';
        case 'done':
            return 'done';
        case 'failed':
            return 'error';
        default:
            return 'ready';
    }
}

/**
 * Prefer live ChatStatus (streaming / awaiting_permission / error / idle)
 * from the session bridge, then fall back to seat.status.
 */
export function seatDisplayStatus(seat: RoundtableSeat, liveChatStatus?: string | null): SeatUiStatus {
    switch (liveChatStatus) {
        case 'streaming':
            return 'running';
        case 'awaiting_permission':
            return 'blocked';
        case 'error':
            return 'error';
        default:
            break;
    }
    return seatUiStatus(seat.status);
}

export function seatStatusLabel(ui: SeatUiStatus): string {
    switch (ui) {
        case 'running':
            return '进行中';
        case 'blocked':
            return '阻塞';
        case 'done':
            return '结束';
        case 'error':
            return '错误';
        default:
            return '就绪';
    }
}
