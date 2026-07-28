import type { RoundtableRoom } from '@1agents/core/services/roundtableService';

export type RoundtablePrimaryActionId =
    | 'confirm_brief'
    | 'start_r2'
    | 'start_r3'
    | 'wait'
    | 'inspect_failure'
    | 'retry_failed_seats'
    | 'retry_summary';

export interface RoundtablePrimaryAction {
    id: RoundtablePrimaryActionId;
    kind: 'button' | 'status';
    label: string;
    primary?: boolean;
}

/** User-facing action derived from the server-owned room state. */
export function primaryActionForRoom(room: RoundtableRoom): RoundtablePrimaryAction | null {
    if (room.state === 'summarizing_r2' && room.phase_status === 'summarizing') {
        return { id: 'wait', kind: 'status', label: '等待裁判提交独立分析总结' };
    }
    if (room.state === 'summarizing_r3' && room.phase_status === 'summarizing') {
        return { id: 'wait', kind: 'status', label: '等待裁判提交交叉验证终稿' };
    }
    switch (room.next_action) {
        case 'confirm_brief':
            return { id: 'confirm_brief', kind: 'button', label: '完善并确认议题', primary: true };
        case 'start_r2':
            return { id: 'start_r2', kind: 'button', label: '开始五席独立分析', primary: true };
        case 'start_r3':
            return { id: 'start_r3', kind: 'button', label: '开始交叉回应', primary: true };
        case 'wait':
            return { id: 'wait', kind: 'status', label: '正在讨论，无需操作' };
        case 'inspect_failure':
        case 'reload_room':
            return { id: 'inspect_failure', kind: 'button', label: '重新同步状态' };
        case 'retry_failed_seats':
            return { id: 'retry_failed_seats', kind: 'status', label: '请选择失败席位的恢复动作' };
        case 'retry_summary':
            return { id: 'retry_summary', kind: 'status', label: '席位结果已保留，等待重试总结' };
        default:
            return null;
    }
}
