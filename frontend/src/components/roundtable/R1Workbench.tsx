import { h } from 'preact';
import type { RefObject } from 'preact';
import type { RoundtableRoom } from '@1agents/core/services/roundtableService';
import { BriefInspector } from './BriefInspector';

interface R1WorkbenchProps {
    room: RoundtableRoom;
    loading?: boolean;
    readOnly?: boolean;
    sectionRef?: RefObject<HTMLElement>;
    onRoomUpdate?: (room: RoundtableRoom) => void | Promise<void>;
    onReload?: () => void | Promise<void>;
}

/** Step 1 · Proposal: edit while drafting, then retain the confirmed snapshot for review. */
export function R1Workbench({ room, loading, readOnly, sectionRef, onRoomUpdate, onReload }: R1WorkbenchProps) {
    return (
        <section class="rt-r1-workbench" aria-labelledby="rt-r1-title">
            <header class="rt-r1-head">
                <div>
                    <p class="rt-r1-kicker">第 1 步 · 提案</p>
                    <h2 id="rt-r1-title" class="rt-r1-title">
                        完善圆桌提案
                    </h2>
                    <p class="rt-r1-desc">
                        {readOnly
                            ? '这是后续讨论使用的已确认提案快照。'
                            : '明确议题、约束与成功标准，确认后进入独立分析。'}
                    </p>
                </div>
            </header>

            <BriefInspector
                room={room}
                loading={loading}
                readOnly={readOnly}
                sectionRef={sectionRef}
                onRoomUpdate={onRoomUpdate}
                onReload={onReload}
            />
        </section>
    );
}
