// The mini-program's single ChatBridgeManager instance. The transport-owning
// manager is shared core code; here we only inject the host specifics — the
// direct-mode WebSocket origin (from config). Connection state mirrors into a
// host store aren't needed yet, so onStatus/onConnection are omitted.

import { ChatBridgeManager } from '@1agents/core/services/chat/chatBridge';
import { BACKEND_WS_ORIGIN } from '../config';

export const bridge = new ChatBridgeManager({
  directWsOrigin: () => BACKEND_WS_ORIGIN,
});
