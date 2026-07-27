import type { RoomState } from '@1agents/core/services/roundtableService';

/** Stage strip: R1 命题 · R2 首轮 · R3 次轮 · 终稿 */
export type StageId = 'r1' | 'r2' | 'r3' | 'final';

export interface StageDef {
    id: StageId;
    label: string;
}

export const STAGES: StageDef[] = [
    { id: 'r1', label: 'R1 命题' },
    { id: 'r2', label: 'R2 首轮' },
    { id: 'r3', label: 'R3 次轮' },
    { id: 'final', label: '终稿' },
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
