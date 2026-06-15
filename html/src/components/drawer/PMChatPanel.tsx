import { h } from 'preact';
import { ChatPanel } from '../chat/ChatPanel';
import type { ChatSession } from '../types';
import * as sess from '../../stores/sessionStore';

/**
 * AI Project Manager panel (副屏 / secondary pane).
 *
 * A thin business component that REUSES the normal chat box (ChatPanel) but is
 * kept entirely separate from the normal chat flow: PM sessions are role='pm',
 * tracked in `pmSessions`, and never appear in the sidebar / primary pane.
 *
 * The toolbar lets the user start a new PM conversation or switch between
 * existing ones — there is no fixed session id; `/new`-style flows just add
 * another role='pm' session and the drawer switches to it.
 */
export function PMChatPanel() {
    const session = sess.pmSession.value;
    const sessions = sess.pmSessions.value;

    if (!session) {
        return <div class="placeholder-view">正在召唤 AI 项目经理…</div>;
    }

    // IMPORTANT: hand the resolved values down as PLAIN PROPS. The component
    // that hosts <ChatPanel> must NOT read signals itself — under
    // @preact/signals, reading a signal in ChatPanel's direct container
    // silently breaks ChatPanelInner's useState-driven repaint, so the bridge's
    // connection/ready updates never paint and the composer stays disabled
    // forever ("连接中" stuck). The live working mount (MiddleCanvas) passes the
    // session as a plain prop for exactly this reason; PMChatView mirrors it.
    return <PMChatView session={session} sessions={sessions} />;
}

interface PMChatViewProps {
    session: ChatSession;
    sessions: ChatSession[];
}

function PMChatView({ session, sessions }: PMChatViewProps) {
    return (
        <div class="pm-panel">
            <div class="pm-panel-bar">
                {sessions.length > 1 ? (
                    <select
                        class="pm-session-select"
                        value={session.id}
                        onChange={e => sess.switchPMSession((e.target as HTMLSelectElement).value)}
                    >
                        {sessions.map(s => (
                            <option key={s.id} value={s.id}>
                                {s.name} · {s.id.slice(0, 6)}
                            </option>
                        ))}
                    </select>
                ) : (
                    <span class="pm-session-label">{session.name}</span>
                )}
                <button
                    class="pm-new-btn"
                    onClick={() => sess.newPMSession(session.workspaceId)}
                    title="开一个新的项目经理会话"
                >
                    + 新会话
                </button>
            </div>
            <div class="pm-panel-body">
                <ChatPanel session={session} />
            </div>
        </div>
    );
}
