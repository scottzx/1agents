import { signal } from '@preact/signals';

import type { Workspace, Session, AgentType, FsEntry } from '../components/types';
import { DEFAULT_AGENT_TYPE } from '../services/agentService';
import type { SkillPushPreview } from '@1agents/core/services/skillService';

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

// ── Assistant create modal ─────────────────────────────────────────────────
// Assistants ARE workspaces (the "对话" concept, extended to N of them). Their
// create flow is intentionally slimmer than WorkspaceModal: no directory pick
// (the backend mints ~/.1agents/projects/<badge>/), just a name + skill picker
// that weak-copies selected skills into <ws>/.claude/skills on create (#360).
export const assistantModalOpen = signal(false);
export const assistantModalName = signal('');
export const assistantModalSkills = signal<string[]>([]);
// Selected persona preset ref (see presets/souls); '' = 空人设 (no SOUL.md).
export const assistantModalSoul = signal('');

export const openCreateAssistantModal = () => {
    assistantModalOpen.value = true;
    assistantModalName.value = '';
    assistantModalSkills.value = [];
    assistantModalSoul.value = '';
};

export const closeAssistantModal = () => {
    assistantModalOpen.value = false;
    assistantModalName.value = '';
    assistantModalSkills.value = [];
    assistantModalSoul.value = '';
};

// ── Chat session creation modal ──
export const chatCreateOpen = signal(false);
export const chatCreateWsId = signal('');

// ── Directory picker modal ──
type DirPickerOnSelect = (path: string) => void;
export const dirPickerOpen = signal(false);
export const dirPickerOnSelect = signal<DirPickerOnSelect | null>(null);
export const dirPickerTitle = signal('');
export const dirPickerInitialPath = signal('');
export const dirPickerRestrictPath = signal(false);

// ── Session rename modal ──
export const sessionRenameModalOpen = signal(false);
export const sessionRenameTarget = signal<Session | null>(null);
export const sessionRenameName = signal('');

// ── Access token display modal (one-time, shown after generation) ──
export const accessTokenModalToken = signal('');

export const openDirPicker = (
    onSelect: (path: string) => void,
    title?: string,
    initialPath?: string,
    restrictPath?: boolean
) => {
    dirPickerOpen.value = true;
    dirPickerOnSelect.value = onSelect;
    dirPickerTitle.value = title || '';
    dirPickerInitialPath.value = initialPath || '';
    dirPickerRestrictPath.value = !!restrictPath;
};

export const closeDirPicker = () => {
    dirPickerOpen.value = false;
    dirPickerTitle.value = '';
    dirPickerInitialPath.value = '';
    dirPickerRestrictPath.value = false;
};

const recordRecentPath = (path: string) => {
    if (!path) return;
    try {
        const raw = localStorage.getItem('1agents_recent_dirs');
        let paths: string[] = [];
        if (raw) {
            paths = JSON.parse(raw);
        }
        if (!Array.isArray(paths)) {
            paths = [];
        }
        paths = paths.filter(p => p !== path);
        paths.unshift(path);
        paths = paths.slice(0, 3);
        localStorage.setItem('1agents_recent_dirs', JSON.stringify(paths));
    } catch (e) {
        console.error('Failed to save recent path', e);
    }
};

/** Open custom directory picker, then the workspace create modal prefilled from the pick. */
export const openCreateWorkspacePicker = () => {
    openDirPicker(pickedPath => {
        const sep = pickedPath.includes('\\') ? '\\' : '/';
        const dirName = pickedPath.split(sep).filter(Boolean).pop() || pickedPath;

        recordRecentPath(pickedPath);

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
        recordRecentPath(path);

        if (!wsModalName.value.trim() && path) {
            const sep = path.includes('\\') ? '\\' : '/';
            const dirName = path.split(sep).filter(Boolean).pop() || path;
            wsModalName.value = dirName;
        }
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

// ── File browser rename/delete modals ──
export const fsRenameModalOpen = signal(false);
export const fsRenameTarget = signal<FsEntry | null>(null);
export const fsRenameName = signal('');

// ── File browser new-item modal (new folder / new file inside a directory) ──
// Parent dir is captured here so the modal doesn't need access to activeWs etc.
// — entry can be null meaning "create at the workspace root".
export type FsNewItemKind = 'folder' | 'file';
export const fsNewItemModalOpen = signal(false);
export const fsNewItemParent = signal<FsEntry | null>(null);
export const fsNewItemKind = signal<FsNewItemKind>('folder');
export const fsNewItemName = signal('');

export const fsDeleteModalOpen = signal(false);
export const fsDeleteTarget = signal<FsEntry | null>(null);

export const openFsRenameModal = (entry: FsEntry) => {
    fsRenameModalOpen.value = true;
    fsRenameTarget.value = entry;
    fsRenameName.value = entry.name;
};

export const closeFsRenameModal = () => {
    fsRenameModalOpen.value = false;
    fsRenameTarget.value = null;
    fsRenameName.value = '';
};

export const openFsNewItemModal = (kind: FsNewItemKind, parent: FsEntry | null) => {
    fsNewItemModalOpen.value = true;
    fsNewItemKind.value = kind;
    fsNewItemParent.value = parent;
    fsNewItemName.value = '';
};

export const closeFsNewItemModal = () => {
    fsNewItemModalOpen.value = false;
    fsNewItemParent.value = null;
    fsNewItemName.value = '';
};

export const openFsDeleteModal = (entry: FsEntry) => {
    fsDeleteModalOpen.value = true;
    fsDeleteTarget.value = entry;
};

export const closeFsDeleteModal = () => {
    fsDeleteModalOpen.value = false;
    fsDeleteTarget.value = null;
};

// ── Push-preview modal (issue #379 follow-up) ──────────────────────────────
// Shown on every "推送到母体" click, replacing the silent-overwrite push:
// previewPush's read-only diff response drives the dialog, and the user picks
// update/fork/create from there instead of the push having already happened.
export const pushPreviewOpen = signal(false);
export const pushPreviewData = signal<SkillPushPreview | null>(null);
export const pushPreviewWorkspaceId = signal('');
export const pushPreviewSkillRef = signal('');
// Callback into SkillsTab so the modal doesn't need to know about workspace
// refresh/flash wiring; set alongside the preview data when opening. Receives
// a short status the caller turns into a flash message.
type PushPreviewOnDone = (result: 'created' | 'main' | 'fork') => void;
export const pushPreviewOnDone = signal<PushPreviewOnDone | null>(null);

export const openPushPreviewModal = (
    preview: SkillPushPreview,
    workspaceId: string,
    skillRef: string,
    onDone: (result: 'created' | 'main' | 'fork') => void
) => {
    pushPreviewOpen.value = true;
    pushPreviewData.value = preview;
    pushPreviewWorkspaceId.value = workspaceId;
    pushPreviewSkillRef.value = skillRef;
    pushPreviewOnDone.value = onDone;
};

export const closePushPreviewModal = () => {
    pushPreviewOpen.value = false;
    pushPreviewData.value = null;
    pushPreviewWorkspaceId.value = '';
    pushPreviewSkillRef.value = '';
    pushPreviewOnDone.value = null;
};
