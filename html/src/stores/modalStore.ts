import { signal } from '@preact/signals';

import type { Workspace, Session, AgentType } from '../components/types';
import { DEFAULT_AGENT_TYPE } from '../services/agentService';

/**
 * Modal state (workspace create/rename modal, chat-create modal, directory
 * picker, session rename, access-token display). Previously lived on App's
 * god-state; now any consumer reads the signals directly. Submit handlers
 * that call services live with their domain (workspaceStore.submitWsModal,
 * sessionStore.submitRenameSession, …).
 */

// ── Workspace create/rename modal ──
export const wsModalOpen = signal(false);
export const wsModalMode = signal<'create' | 'rename'>('create');
export const wsModalTarget = signal<Workspace | null>(null);
export const wsModalName = signal('');
export const wsModalPath = signal('');
export const wsModalTerminalDir = signal('');
export const wsModalChatChannel = signal('');
export const wsModalDefaultAgent = signal<AgentType>(DEFAULT_AGENT_TYPE);

// ── Chat session creation modal ──
export const chatCreateOpen = signal(false);
export const chatCreateWsId = signal('');

// ── Directory picker modal ──
type DirPickerOnSelect = (path: string) => void;
export const dirPickerOpen = signal(false);
export const dirPickerOnSelect = signal<DirPickerOnSelect | null>(null);

// ── Session rename modal ──
export const sessionRenameModalOpen = signal(false);
export const sessionRenameTarget = signal<Session | null>(null);
export const sessionRenameName = signal('');

// ── Access token display modal (one-time, shown after generation) ──
export const accessTokenModalToken = signal('');

export const openDirPicker = (onSelect: (path: string) => void) => {
    dirPickerOpen.value = true;
    dirPickerOnSelect.value = onSelect;
};

export const closeDirPicker = () => {
    dirPickerOpen.value = false;
};

/** Open custom directory picker, then the workspace create modal prefilled from the pick. */
export const openCreateWorkspacePicker = () => {
    openDirPicker(pickedPath => {
        const sep = pickedPath.includes('\\') ? '\\' : '/';
        const dirName = pickedPath.split(sep).filter(Boolean).pop() || pickedPath;

        // Open standard workspace create modal with prefilled data!
        wsModalOpen.value = true;
        wsModalMode.value = 'create';
        wsModalTarget.value = null;
        wsModalName.value = dirName;
        wsModalPath.value = pickedPath;
        wsModalTerminalDir.value = '';
        wsModalChatChannel.value = '';
        wsModalDefaultAgent.value = DEFAULT_AGENT_TYPE;
    });
};

export const openDirPickerForModal = () => {
    openDirPicker(path => {
        wsModalPath.value = path;
    });
};

/** Open the modal for renaming/editing an existing workspace */
export const openRenameWorkspaceModal = (ws: Workspace) => {
    wsModalOpen.value = true;
    wsModalMode.value = 'rename';
    wsModalTarget.value = ws;
    wsModalName.value = ws.name;
    wsModalPath.value = ws.path;
    wsModalTerminalDir.value = ws.terminalDir || '';
    wsModalChatChannel.value = ws.chatChannel || '';
    wsModalDefaultAgent.value = ws.defaultAgent || DEFAULT_AGENT_TYPE;
};

export const closeWsModal = () => {
    wsModalOpen.value = false;
    wsModalTarget.value = null;
    wsModalName.value = '';
    wsModalPath.value = '';
    wsModalTerminalDir.value = '';
    wsModalChatChannel.value = '';
    wsModalDefaultAgent.value = DEFAULT_AGENT_TYPE;
};

/** Open the chat-create modal for a given workspace. */
export const openChatCreate = (workspaceId: string) => {
    chatCreateOpen.value = true;
    chatCreateWsId.value = workspaceId;
};

export const closeChatCreate = () => {
    chatCreateOpen.value = false;
    chatCreateWsId.value = '';
};

export const openRenameSessionModal = (s: Session) => {
    sessionRenameModalOpen.value = true;
    sessionRenameTarget.value = s;
    sessionRenameName.value = s.name;
};

export const closeSessionRenameModal = () => {
    sessionRenameModalOpen.value = false;
    sessionRenameTarget.value = null;
    sessionRenameName.value = '';
};

export const closeAccessTokenModal = () => {
    accessTokenModalToken.value = '';
};
