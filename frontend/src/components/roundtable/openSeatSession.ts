/**
 * Open a roundtable seat's underlying chat session in the main ChatUI
 * (selectSession → full ChatPanel). Reuses agent index when present.
 */
import { agentService } from '@1agents/core/services/agentService';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import type { AgentType, ChatSession } from '../types';
import * as sessionStore from '../../stores/sessionStore';
import * as ui from '../../stores/uiStore';
import * as taskNav from '../../stores/taskNavStore';
import * as stage from '../../stores/stageStore';
import { roleLabel } from './roleLabels';
import { persistListView, persistRoomView } from './navState';
import { roundtableBreadcrumbs } from './breadcrumbs';

interface RoundtableSessionContext {
    roomId: string;
    roomTitle: string;
}

export async function openSeatSession(seat: RoundtableSeat, context: RoundtableSessionContext): Promise<boolean> {
    const sessionId = seat.session_id?.trim();
    if (!sessionId) {
        ui.showToast('该席位尚无会话，请稍后再试');
        return false;
    }

    let rec: ChatSession | null = null;
    try {
        rec = await agentService.get(sessionId);
    } catch {
        // fall through to synthetic shape
    }

    const name = roleLabel(seat.role);
    const session: ChatSession = rec
        ? {
              ...rec,
              active: true,
              name: rec.name || name,
              ...(seat.acp_session_id && !rec.acpSessionId ? { acpSessionId: seat.acp_session_id } : {}),
          }
        : {
              kind: 'chat',
              id: sessionId,
              workspaceId: seat.workspace_id,
              name,
              agentType: (seat.agent_type || 'claudecode') as AgentType,
              ccProject: '',
              ccSessionId: '',
              sessionKey: '',
              status: 'idle',
              active: true,
              ...(seat.acp_session_id ? { acpSessionId: seat.acp_session_id } : {}),
          };

    await sessionStore.selectSession(session);
    const openList = () => {
        taskNav.clearHeaderBackAction('roundtable-session');
        persistListView();
        stage.enterL1App('agents-roundtable');
    };
    const openRoom = () => {
        taskNav.clearHeaderBackAction('roundtable-session');
        persistRoomView(context.roomId);
        stage.enterL1App('agents-roundtable');
    };
    taskNav.headerCrumbs.value = roundtableBreadcrumbs({
        view: 'session',
        roomTitle: context.roomTitle,
        sessionTitle: session.name || name,
        onList: openList,
        onRoom: openRoom,
    });
    taskNav.registerHeaderBackAction('roundtable-session', openRoom, taskNav.HEADER_BACK_PRIORITY.surface);
    return true;
}
