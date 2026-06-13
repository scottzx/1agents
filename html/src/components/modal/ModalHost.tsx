import { h, Fragment } from 'preact';

import { WorkspaceModal } from './WorkspaceModal';
import { DirPickerModal } from './DirPickerModal';
import { AccessTokenModal } from './AccessTokenModal';
import { SessionRenameModal } from './SessionRenameModal';
import { SessionCreateModal } from '../chat/SessionCreateModal';
import { DEFAULT_AGENT_TYPE } from '../../services/agentService';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as modal from '../../stores/modalStore';

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
                            onSubmit={(name, agentType) => {
                                modal.closeChatCreate();
                                sess.createChatSession(chatCreateWsId, name, agentType);
                            }}
                        />
                    );
                })()}

            {/* Remote Directory Picker Modal */}
            {dirPickerOpen && (
                <DirPickerModal
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
        </Fragment>
    );
}
