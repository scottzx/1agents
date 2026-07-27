import { h, type ComponentChildren, type RefObject } from 'preact';
import type { RoundtableRoom } from '@1agents/core/services/roundtableService';
import { stateLabel } from './stage';
import { progressText } from './workbench';

interface RoundtableHeaderProps {
    room: RoundtableRoom;
    busy?: boolean;
    loading?: boolean;
    action?: ComponentChildren;
    statusRef?: RefObject<HTMLDivElement>;
}

/** One compact control surface: roundtable, phase, real progress, primary action. */
export function RoundtableHeader({ room, busy, loading, action, statusRef }: RoundtableHeaderProps) {
    const statusIcon =
        room.phase_status === 'failed' || room.phase_status === 'partial_failed'
            ? '!'
            : room.phase_status === 'completed' || room.phase === 'done'
              ? '✓'
              : room.phase_status === 'running'
                ? '↻'
                : '○';
    return (
        <header class="rt-room-header" aria-label="圆桌状态">
            <div class="rt-room-title-block">
                <span class="rt-room-eyebrow">圆桌</span>
                <h1 class="rt-room-title">{room.title || '未命名圆桌'}</h1>
            </div>
            <div class="rt-room-phase" ref={statusRef} tabIndex={-1}>
                <span class="rt-room-phase-label">
                    <span class="rt-room-phase-icon" aria-hidden="true">
                        {statusIcon}
                    </span>
                    {stateLabel(room.state)}
                </span>
                <span class="rt-room-progress" role="status" aria-live="polite" aria-atomic="true">
                    {progressText(room)}
                    {loading ? ' · 同步中' : ''}
                </span>
            </div>
            <div class="rt-room-primary-action" aria-busy={busy}>
                {action}
            </div>
        </header>
    );
}
