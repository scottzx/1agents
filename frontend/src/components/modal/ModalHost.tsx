import { h, Fragment } from 'preact';

import { WorkspaceModal } from './WorkspaceModal';
import { AssistantModal } from './AssistantModal';
import { DirPickerModal } from './DirPickerModal';
import { AccessTokenModal } from './AccessTokenModal';
import { SessionRenameModal } from './SessionRenameModal';
import { FsRenameModal } from './FsRenameModal';
import { FsDeleteConfirmModal } from './FsDeleteConfirmModal';
import { SkillConflictModal } from './SkillConflictModal';
import { SessionCreateModal } from '../chat/SessionCreateModal';
import { DEFAULT_AGENT_TYPE } from '../../services/agentService';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';
import * as fsStore from '../../stores/fsStore';

/**
 * Renders all app-level modals from modalStore signals. Pure open/close and
 * field-setter logic lives in modalStore; submit handlers that call services
 * live in the domain stores (workspaceStore.submitWsModal,
 * sessionStore.submitRenameSession / createChatSession).
 */
export function ModalHost() {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const wsModalOpen = modal.wsModalOpen.value;
    const chatCreateOpen = modal.chatCreateOpen.value;
    const chatCreateWsId = modal.chatCreateWsId.value;
    const dirPickerOpen = modal.dirPickerOpen.value;
    const accessTokenModalToken = modal.accessTokenModalToken.value;
    const sessionRenameModalOpen = modal.sessionRenameModalOpen.value;
    const sessionRenameTarget = modal.sessionRenameTarget.value;

    return (
        <Fragment>
            {/* Workspace create/rename modal */}
            {wsModalOpen && (
                <WorkspaceModal
                    mode={modal.wsModalMode.value}
                    name={modal.wsModalName.value}
                    path={modal.wsModalPath.value}
                    terminalDir={modal.wsModalTerminalDir.value}
                    chatChannel={modal.wsModalChatChannel.value}
                    defaultAgent={modal.wsModalDefaultAgent.value}
                    onNameChange={val => (modal.wsModalName.value = val)}
                    onPathChange={val => (modal.wsModalPath.value = val)}
                    onTerminalDirChange={val => (modal.wsModalTerminalDir.value = val)}
                    onChatChannelChange={val => (modal.wsModalChatChannel.value = val)}
                    onDefaultAgentChange={val => (modal.wsModalDefaultAgent.value = val)}
                    onClose={modal.closeWsModal}
                    onBrowse={modal.openDirPickerForModal}
                    onSubmit={wsStore.submitWsModal}
                    language={language}
                />
            )}

            {/* Assistant create modal — the slim "对话" create path (name + skills) */}
            {modal.assistantModalOpen.value && (
                <AssistantModal
                    name={modal.assistantModalName.value}
                    skills={modal.assistantModalSkills.value}
                    soul={modal.assistantModalSoul.value}
                    onNameChange={val => (modal.assistantModalName.value = val)}
                    onSkillsChange={val => (modal.assistantModalSkills.value = val)}
                    onSoulChange={val => (modal.assistantModalSoul.value = val)}
                    onClose={modal.closeAssistantModal}
                    onSubmit={avatar => {
                        const name = modal.assistantModalName.value.trim();
                        if (!name) return;
                        const skills = modal.assistantModalSkills.value;
                        const soul = modal.assistantModalSoul.value;
                        // Fire-and-forget: the store shows a toast on failure
                        // and the loadWorkspaces refresh on success is what
                        // ultimately closes the picker experience.
                        modal.closeAssistantModal();
                        wsStore.createAssistant(name, skills, avatar || undefined, soul || undefined);
                    }}
                    language={language}
                />
            )}

            {/* Chat session create modal */}
            {chatCreateOpen &&
                chatCreateWsId &&
                (() => {
                    const ws = workspaces.find(w => w.id === chatCreateWsId);
                    if (!ws) return null;
                    return (
                        <SessionCreateModal
                            workspaceId={chatCreateWsId}
                            workspaceName={ws.name}
                            defaultAgent={ws.defaultAgent || DEFAULT_AGENT_TYPE}
                            onCancel={modal.closeChatCreate}
                            onSubmit={(name, agentType, permissionMode, role) => {
                                modal.closeChatCreate();
                                // 'pm' in the cross-project (default/builtin) workspace
                                // becomes 'pmo' — mirrors createPMSession / NewChatHome.
                                const effectiveRole =
                                    role === 'pm' && (ws.id === 'default' || ws.builtin) ? 'pmo' : role || undefined;
                                sess.createChatSession(
                                    chatCreateWsId,
                                    name,
                                    agentType,
                                    undefined,
                                    effectiveRole,
                                    permissionMode
                                );
                            }}
                        />
                    );
                })()}

            {/* Remote Directory Picker Modal */}
            {dirPickerOpen && (
                <DirPickerModal
                    title={modal.dirPickerTitle.value}
                    initialPath={modal.dirPickerInitialPath.value}
                    restrictPath={modal.dirPickerRestrictPath.value}
                    onClose={modal.closeDirPicker}
                    onSelect={pickedPath => {
                        const onSelect = modal.dirPickerOnSelect.value;
                        if (onSelect) {
                            onSelect(pickedPath);
                        }
                        modal.closeDirPicker();
                    }}
                    onShowToast={ui.showToast}
                    language={language}
                />
            )}

            {/* Access Token Display Modal (one-time, shown after generation) */}
            {accessTokenModalToken && (
                <AccessTokenModal
                    token={accessTokenModalToken}
                    onClose={modal.closeAccessTokenModal}
                    onShowToast={ui.showToast}
                    language={language}
                />
            )}

            {/* Session Rename Modal */}
            {sessionRenameModalOpen && sessionRenameTarget && (
                <SessionRenameModal
                    title={modal.sessionRenameName.value}
                    onTitleChange={val => (modal.sessionRenameName.value = val)}
                    onClose={modal.closeSessionRenameModal}
                    onSubmit={sess.submitRenameSession}
                    language={language}
                />
            )}

            {/* File System Rename Modal */}
            {modal.fsRenameModalOpen.value && modal.fsRenameTarget.value && (
                <FsRenameModal
                    title={modal.fsRenameName.value}
                    onTitleChange={val => (modal.fsRenameName.value = val)}
                    onClose={modal.closeFsRenameModal}
                    onSubmit={fsStore.submitFsRename}
                    language={language}
                    isDir={modal.fsRenameTarget.value.isDir}
                />
            )}

            {/* File System Delete Confirmation Modal */}
            {modal.fsDeleteModalOpen.value && modal.fsDeleteTarget.value && (
                <FsDeleteConfirmModal
                    name={modal.fsDeleteTarget.value.name}
                    isDir={modal.fsDeleteTarget.value.isDir}
                    onClose={modal.closeFsDeleteModal}
                    onSubmit={fsStore.submitFsDelete}
                    language={language}
                />
            )}

            {/* Skill push concurrent-edit conflict modal (issue #379) */}
            {modal.skillConflictOpen.value && modal.skillConflictData.value && (
                <SkillConflictModal
                    conflict={modal.skillConflictData.value}
                    onClose={modal.closeSkillConflictModal}
                    onResolved={resolution => {
                        const onResolved = modal.skillConflictOnResolved.value;
                        modal.closeSkillConflictModal();
                        onResolved?.(resolution);
                    }}
                    language={language}
                />
            )}
        </Fragment>
    );
}
