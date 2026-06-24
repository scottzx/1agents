// React hook wrapping the shared ChatBridgeManager for the mini-program — the
// weapp counterpart of the web app's useBridge. Subscribes to the manager's
// per-session listener set and repaints on every bump.

import { useCallback, useEffect, useState } from 'react';
import type { ChatItem, ConnectionState } from '@1agents/core/protocol/types';
import type { ChatSession, PermissionDecision } from '@1agents/core/types';
import { bridge } from '../services/chat';

export interface UseChat {
  items: ChatItem[];
  connection: ConnectionState;
  ready: boolean;
  send: (content: string) => void;
  respondPermission: (requestId: string, decision: PermissionDecision) => void;
}

export function useChat(session: ChatSession | null): UseChat {
  const [, setRev] = useState(0);
  const bump = useCallback(() => setRev(r => r + 1), []);

  useEffect(() => {
    if (!session) return;
    const state = bridge.getOrCreate(session);
    state.listeners.add(bump);
    bump();
    return () => {
      state.listeners.delete(bump);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session?.id]);

  const state = session ? bridge.getOrCreate(session) : null;
  // Flatten the realtime holding pools into the stream, mirroring useBridge.
  const items = state ? [...state.items, ...state.pendingResults, ...state.pendingPermissions] : [];
  const connection: ConnectionState = state ? state.connection : 'idle';
  const ready = state ? state.ready : false;

  const send = useCallback(
    (content: string) => {
      if (session) bridge.send(session, content);
    },
    [session?.id]
  );

  const respondPermission = useCallback(
    (requestId: string, decision: PermissionDecision) => {
      if (session) bridge.respondPermission(session, requestId, decision);
    },
    [session?.id]
  );

  return { items, connection, ready, send, respondPermission };
}
