/**
 * Open a roundtable seat's underlying chat session in the main ChatUI
 * (selectSession → full ChatPanel). Reuses agent index when present.
 */
import { agentService } from '@1agents/core/services/agentService';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import type { AgentType, ChatSession } from '../types';
import * as sessionStore from '../../stores/sessionStore';
import * as ui from '../../stores/uiStore';
import { roleLabel } from './roleLabels';

export async function openSeatSession(seat: RoundtableSeat): Promise<boolean> {
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
    return true;
}
