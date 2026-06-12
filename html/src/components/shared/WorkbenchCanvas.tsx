import { h } from 'preact';
import type { ITerminalOptions } from '@xterm/xterm';
import { isChat } from '../types';
import { MiddleCanvas } from '../canvas/MiddleCanvas';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as sess from '../../stores/sessionStore';
import {
    lightTermTheme,
    darkTermTheme,
    baseTermOptions,
    wsUrl,
    tokenUrl,
    clientOptions,
    flowControl,
} from '../terminal/terminalConfig';

interface WorkbenchCanvasProps {
    app: App;
    state: AppState;
    /** Terminal font size: 13 on desktop, 12 on mobile. */
    fontSize: number;
    pendingInitialMessage?: string | null;
    onClearPendingInitialMessage?: () => void;
}

/**
 * Shared MiddleCanvas wiring used by both DesktopAppLayout and
 * MobileAppLayout. The terminal websocket/token/flow-control config and
 * the active chat-session plumbing are identical on both platforms; only
 * the terminal font size (and the desktop-only pending-initial-message
 * hand-off) differ.
 */
export function WorkbenchCanvas({
    app,
    state,
    fontSize,
    pendingInitialMessage,
    onClearPendingInitialMessage,
}: WorkbenchCanvasProps) {
    const theme = ui.theme.value;
    const language = ui.language.value;
    const isMobile = ui.isMobile.value;
    const activeSession = sess.activeSession.value;
    const tmuxMouseOn = sess.tmuxMouseOn.value;

    const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
    const termOptions = {
        ...baseTermOptions,
        theme: currentTheme,
        fontSize,
    } as ITerminalOptions;

    return (
        <MiddleCanvas
            activeTab={state.activeTab as 'terminal' | 'agents' | 'console' | 'folders'}
            wsUrl={wsUrl}
            tokenUrl={tokenUrl}
            clientOptions={clientOptions}
            termOptions={termOptions}
            flowControl={flowControl}
            isMobile={isMobile}
            onMobileDetect={isMobile => (ui.isMobile.value = isMobile)}
            onKeyboardStateChange={app.handleKeyboardStateChange}
            tmuxMouseOn={tmuxMouseOn}
            onTmuxMouseToggle={app.toggleTmuxMouse}
            language={language}
            activeChatSession={activeSession && isChat(activeSession) ? activeSession : null}
            pendingInitialMessage={pendingInitialMessage}
            onClearPendingInitialMessage={onClearPendingInitialMessage}
        />
    );
}
