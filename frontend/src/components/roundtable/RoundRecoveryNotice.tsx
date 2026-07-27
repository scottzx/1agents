import { h } from 'preact';
import type { RoundtableRoom, SeatRole } from '@1agents/core/services/roundtableService';
import { roleLabel } from './roleLabels';

export type RecoveryKind = 'none' | 'room' | 'seat' | 'summary';

export function recoveryKind(room: RoundtableRoom): RecoveryKind {
    return room.active_run?.error_scope || 'none';
}

interface RoundRecoveryNoticeProps {
    room: RoundtableRoom;
    busy?: boolean;
    onRetrySeat: (role: SeatRole) => void | Promise<void>;
    onSkip: () => void | Promise<void>;
    onRetrySummary: () => void | Promise<void>;
    onReload: () => void | Promise<void>;
}

/** Recovery controls are derived entirely from the persisted room projection. */
export function RoundRecoveryNotice({
    room,
    busy,
    onRetrySeat,
    onSkip,
    onRetrySummary,
    onReload,
}: RoundRecoveryNoticeProps) {
    const kind = recoveryKind(room);
    const failedRoles = room.progress.failed_roles || [];
    if (kind === 'none') return null;

    if (kind === 'seat') {
        return (
            <section class="rt-recovery is-seat" role="alert" aria-labelledby="rt-recovery-title">
                <div>
                    <strong id="rt-recovery-title">部分席位未完成</strong>
                    <p>已完成席位及其结果已保留。可只重试失败席位，或将其记为缺席后继续总结。</p>
                </div>
                <div class="rt-recovery-actions">
                    {failedRoles.map(role => (
                        <button
                            key={role}
                            type="button"
                            class="rt-btn"
                            disabled={busy}
                            onClick={() => void onRetrySeat(role)}
                        >
                            仅重试{roleLabel(role)}席
                        </button>
                    ))}
                    <button type="button" class="rt-btn rt-btn-ghost" disabled={busy} onClick={() => void onSkip()}>
                        跳过缺席席位并继续总结
                    </button>
                </div>
            </section>
        );
    }

    if (kind === 'summary') {
        return (
            <section class="rt-recovery is-summary" role="alert" aria-labelledby="rt-recovery-title">
                <div>
                    <strong id="rt-recovery-title">席位结果已完成，总结生成失败</strong>
                    <p>重试只会再次执行裁判总结，不会重新运行任何 panelist。</p>
                </div>
                <button
                    type="button"
                    class="rt-btn rt-btn-primary"
                    disabled={busy}
                    onClick={() => void onRetrySummary()}
                >
                    仅重试总结
                </button>
            </section>
        );
    }

    return (
        <section class="rt-recovery is-room" role="alert" aria-labelledby="rt-recovery-title">
            <div>
                <strong id="rt-recovery-title">房间状态同步失败</strong>
                <p>席位恢复动作暂不可用，请先从服务端重新同步当前 RoundRun。</p>
            </div>
            <button type="button" class="rt-btn" disabled={busy} onClick={() => void onReload()}>
                重新同步房间
            </button>
        </section>
    );
}
