import type { RoomState, RoundtableRoom } from '@1agents/core/services/roundtableService';

/** User-facing roundtable stages. */
export type StageId = 'r1' | 'r2' | 'r3' | 'final';

export interface StageDef {
    id: StageId;
    label: string;
}

export const STAGES: StageDef[] = [
    { id: 'r1', label: '提案' },
    { id: 'r2', label: '独立分析' },
    { id: 'r3', label: '交叉回应' },
    { id: 'final', label: '最终结论' },
];

/** Map room state machine → active stage index (0–3). */
export function stageIndexFromState(state: RoomState | string | undefined): number {
    switch (state) {
        case 'drafting_brief':
            return 0;
        case 'waiting_r2':
        case 'summarizing_r2':
            return 1;
        case 'waiting_r3':
        case 'summarizing_r3':
            return 2;
        case 'done':
            return 3;
        case 'failed':
            return -1;
        default:
            return 0;
    }
}

export function stageIdFromState(state: RoomState | string | undefined): StageId {
    const index = stageIndexFromState(state);
    return index >= 0 ? STAGES[index].id : 'r1';
}

export function stageIdFromRoom(room: RoundtableRoom | null | undefined): StageId {
    if (!room) return 'r1';
    switch (room.phase) {
        case 'r1':
        case 'r2':
        case 'r3':
            return room.phase;
        case 'done':
            return 'final';
        case 'failed':
            if (room.active_run?.round === 3 || room.summary_r2) return 'r3';
            if (room.active_run?.round === 2 || room.confirmed_brief_version || room.confirmed_brief) return 'r2';
            return 'r1';
    }
}

export function isTerminalState(state: RoomState | string | undefined): boolean {
    return state === 'done' || state === 'failed';
}

/** Poll faster while agents are summarizing / mid-flight. */
export function pollIntervalMs(state: RoomState | string | undefined): number {
    switch (state) {
        case 'summarizing_r2':
        case 'summarizing_r3':
            return 1500;
        case 'drafting_brief':
        case 'waiting_r2':
        case 'waiting_r3':
            return 4000;
        default:
            return 0; // terminal — stop polling
    }
}

export function stateLabel(state: RoomState | string | undefined): string {
    switch (state) {
        case 'drafting_brief':
            return 'R1 · 澄清命题';
        case 'waiting_r2':
            return '等待 R2';
        case 'summarizing_r2':
            return 'R2 进行中';
        case 'waiting_r3':
            return '等待 R3';
        case 'summarizing_r3':
            return 'R3 进行中';
        case 'done':
            return '已终稿';
        case 'failed':
            return '失败';
        default:
            return state ? '状态更新中' : '待开始';
    }
}

/**
 * Infer display round for a live speaking seat (turns may not exist yet).
 * R2 parallel speeches run while state is still waiting_r2; summary phases
 * use the same round number as the speeches they wrap up.
 */
export function speakingRoundFromState(state: RoomState | string | undefined): number {
    switch (state) {
        case 'drafting_brief':
            return 1;
        case 'waiting_r2':
        case 'summarizing_r2':
            return 2;
        case 'waiting_r3':
        case 'summarizing_r3':
            return 3;
        default:
            return 0;
    }
}
