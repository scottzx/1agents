import { h } from 'preact';
import type { RefObject } from 'preact';
import type { RoundtableRoom } from '@1agents/core/services/roundtableService';
import { BriefInspector } from './BriefInspector';

interface R1WorkbenchProps {
    room: RoundtableRoom;
    loading?: boolean;
    sectionRef?: RefObject<HTMLElement>;
    onRoomUpdate?: (room: RoundtableRoom) => void | Promise<void>;
    onReload?: () => void | Promise<void>;
}

/** R1 · 提案阶段：简化的 Brief 编辑，无裁判聊天窗口嵌入。 */
export function R1Workbench({ room, loading, sectionRef, onRoomUpdate, onReload }: R1WorkbenchProps) {
    return (
        <section class="rt-r1-workbench" aria-labelledby="rt-r1-title">
            <header class="rt-r1-head">
                <div>
                    <p class="rt-r1-kicker">R1 · 命题澄清</p>
                    <h2 id="rt-r1-title" class="rt-r1-title">
                        与裁判完善 Brief
                    </h2>
                    <p class="rt-r1-desc">直接在这里完善 Brief；右侧始终显示当前角色信息。</p>
                </div>
            </header>

            <BriefInspector
                room={room}
                loading={loading}
                sectionRef={sectionRef}
                onRoomUpdate={onRoomUpdate}
                onReload={onReload}
            />
        </section>
    );
}
