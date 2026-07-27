import { h } from 'preact';
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import type { AgentType } from '../types';
import { EmbeddedChat } from '../chat/EmbeddedChat';
import { r1BriefEvents } from './r1Timeline';

interface R1WorkbenchProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
    turns: RoundtableTurn[];
    sending?: boolean;
    onSend: (text: string) => void | Promise<void>;
    onFocusBrief: () => void;
    onOpenReferee: (seat: RoundtableSeat) => void | Promise<void>;
}

/** R1 stays inside the roundtable: full referee history/stream plus Composer. */
export function R1Workbench({
    room,
    seats,
    turns,
    sending = false,
    onSend,
    onFocusBrief,
    onOpenReferee,
}: R1WorkbenchProps) {
    const referee = seats.find(seat => seat.role === 'referee') || null;
    const events = r1BriefEvents(turns);

    return (
        <section class="rt-r1-workbench" aria-labelledby="rt-r1-title">
            <header class="rt-r1-head">
                <div>
                    <p class="rt-r1-kicker">R1 · 命题澄清</p>
                    <h2 id="rt-r1-title" class="rt-r1-title">
                        与裁判完善 Brief
                    </h2>
                    <p class="rt-r1-desc">直接在这里继续多轮对话；右侧 Inspector 始终显示服务端当前版本。</p>
                </div>
                <button
                    type="button"
                    class="rt-btn rt-btn-ghost"
                    disabled={!referee?.session_id}
                    onClick={() => referee && void onOpenReferee(referee)}
                >
                    打开完整裁判会话
                </button>
            </header>

            <EmbeddedChat
                sessionId={referee?.session_id}
                workspaceId={referee?.workspace_id}
                agentType={(referee?.agent_type || 'grok-build') as AgentType}
                acpSessionId={referee?.acp_session_id}
                maxHeight="100%"
                readOnly={false}
                showComposer
                onSend={onSend}
                isRunning={sending}
                events={events}
                onEventActivate={onFocusBrief}
                className="rt-r1-chat"
                emptyHint={
                    referee
                        ? '告诉裁判你要讨论的问题；可连续追问，直到 Brief 足够明确。'
                        : `正在为「${room.title || '圆桌议题'}」准备裁判会话…`
                }
            />
        </section>
    );
}
