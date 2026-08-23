/**
 * Side-effecting chrome-mode switch: snapshot/restore workbench surfaces
 * around the pure `switchChromeMode` reducer.
 */
import * as sess from './sessionStore';
import * as stage from './stageStore';
import * as tabsStore from './tabsStore';
import * as ui from './uiStore';
import { activeChatRoomId, openChatRoom } from './agentChatStore';
import {
    chromeMode,
    lastChatRoomId,
    lastWorkbenchSurface,
    setChromeMode,
    type ChromeMode,
    type WorkbenchSurface,
} from './chromeModeStore';

export function snapshotWorkbench(): WorkbenchSurface {
    return {
        sidebarMode: ui.sidebarMode.value,
        stageView: stage.stageView.value,
        activeDrawerTab: tabsStore.activeDrawerTab.value,
        activeTab: tabsStore.activeTab.value,
        activeSessionId: sess.activeSession.value?.id ?? null,
    };
}

export function restoreWorkbench(surface: WorkbenchSurface | null): void {
    if (!surface) return;
    ui.sidebarMode.value = surface.sidebarMode;
    try {
        localStorage.setItem('1agents-sidebar-mode', surface.sidebarMode);
    } catch {
        /* ignore */
    }
    stage.stageView.value = surface.stageView;
    try {
        localStorage.setItem('1agents-stage-view', surface.stageView);
    } catch {
        /* ignore */
    }
    tabsStore.activeDrawerTab.value = surface.activeDrawerTab as typeof tabsStore.activeDrawerTab.value;
    tabsStore.activeTab.value = surface.activeTab as typeof tabsStore.activeTab.value;
    if (surface.activeSessionId) {
        const chats = sess.chatSessions.value;
        const match = chats.find(s => s.id === surface.activeSessionId);
        if (match) sess.selectSession(match);
    }
}

export function switchToChromeMode(next: ChromeMode): void {
    if (next === chromeMode.value) return;
    if (next === 'chat') {
        setChromeMode('chat', { workbench: snapshotWorkbench() });
        const roomId = lastChatRoomId.value || activeChatRoomId.value;
        if (roomId) openChatRoom(roomId);
        return;
    }
    setChromeMode('workbench', { chatRoomId: activeChatRoomId.value });
    restoreWorkbench(lastWorkbenchSurface.value);
}

/** Unified chrome picker: a product shell or the 聊天 surface. */
export function pickWorkbenchSurface(item: { id: string; kind: 'shell' | 'chat' }): void {
    if (item.kind === 'chat') {
        switchToChromeMode('chat');
        return;
    }
    if (chromeMode.value === 'chat') {
        switchToChromeMode('workbench');
    }
    if (item.id && item.id !== 'workbench') {
        stage.switchShell(item.id);
    }
}
