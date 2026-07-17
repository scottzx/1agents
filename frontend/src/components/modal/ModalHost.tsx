import { h, Fragment } from 'preact';

import { WorkspaceModal } from './WorkspaceModal';
import { AssistantModal } from './AssistantModal';
import { DirPickerModal } from './DirPickerModal';
import { AccessTokenModal } from './AccessTokenModal';
import { SessionRenameModal } from './SessionRenameModal';
import { FsRenameModal } from './FsRenameModal';
import { FsCreateModal } from './FsCreateModal';
import { FsDeleteConfirmModal } from './FsDeleteConfirmModal';
import { PushPreviewModal } from './PushPreviewModal';
import { SessionSetupModal } from '../chat/SessionSetupModal';
import { ReauthModal } from '../chat/ReauthModal';
import { DEFAULT_AGENT_TYPE } from '../../services/agentService';
import { globalBridgeManager } from '../chat/hooks';
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
    const sessionSetupOpen = modal.sessionSetupOpen.value;
    const sessionSetupOpts = modal.sessionSetupOpts.value;
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

            {/* Unified Session Setup (replaces SessionCreateModal; no role/permission) */}
            {sessionSetupOpen &&
                (() => {
                    const locked = !!sessionSetupOpts.locked && !!sessionSetupOpts.workspaceId;
                    const defaultWsId =
                        sessionSetupOpts.workspaceId || wsStore.activeWorkspaceId.value || workspaces[0]?.id || '';
                    const ws = workspaces.find(w => w.id === defaultWsId);
                    if (!defaultWsId && workspaces.length === 0) return null;
                    return (
                        <SessionSetupModal
                            workspaces={workspaces}
                            defaultWorkspaceId={defaultWsId}
                            defaultAgent={sessionSetupOpts.defaultAgent || ws?.defaultAgent || DEFAULT_AGENT_TYPE}
                            locked={locked}
                            workspaceName={locked ? ws?.name || '' : ''}
                            initialAgentRef={sessionSetupOpts.agentRef}
                            language={language}
                            onCancel={modal.closeSessionSetup}
                            onSubmit={values => {
                                const opts = modal.sessionSetupOpts.value;
                                modal.closeSessionSetup();
                                void sess.createFromSessionSetup({
                                    workspaceId: values.workspaceId || defaultWsId,
                                    agentType: values.agentType,
                                    name: values.name,
                                    initialMessage: opts.initialMessage,
                                    taskId: opts.taskId,
                                    agentRef: values.agentRef || opts.agentRef,
                                });
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

            {/* File System New Folder / New File Modal */}
            {modal.fsNewItemModalOpen.value && (
                <FsCreateModal
                    name={modal.fsNewItemName.value}
                    kind={modal.fsNewItemKind.value}
                    parentName={modal.fsNewItemParent.value?.name ?? null}
                    onNameChange={val => (modal.fsNewItemName.value = val)}
                    onClose={modal.closeFsNewItemModal}
                    onSubmit={fsStore.submitFsNewItem}
                    language={language}
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

            {/* Push-preview modal (issue #379 follow-up) */}
            {modal.pushPreviewOpen.value && modal.pushPreviewData.value && (
                <PushPreviewModal
                    preview={modal.pushPreviewData.value}
                    workspaceId={modal.pushPreviewWorkspaceId.value}
                    skillRef={modal.pushPreviewSkillRef.value}
                    onClose={modal.closePushPreviewModal}
                    onDone={result => {
                        const onDone = modal.pushPreviewOnDone.value;
                        modal.closePushPreviewModal();
                        onDone?.(result);
                    }}
                    language={language}
                />
            )}

            {/* Re-auth modal (task #106) — triggered automatically when the
                bridge pushes auth_required and manually from the session auth
                badge. Submit forwards to the bridge's
                authenticate action; auth_completed auto-closes via the
                status->useEffect loop in ChatPanel. */}
            {modal.authRequiredModalOpen.value && <ReauthHost onClose={modal.closeAuthRequiredModal} />}
        </Fragment>
    );
}

/**
 * Thin wrapper that subscribes to the bridge for the session currently being
 * re-authed (resolved from the modalStore's session id), so the modal can
 * pull live `auth.lastError` and dispatch `authenticate` to the right
 * session without prop-drilling the ChatSession down from the panel.
 */
function ReauthHost({ onClose }: { onClose: () => void }) {
    const sessionId = modal.authRequiredSessionId.value;
    const methods = modal.authRequiredMethods.value;
    const message = modal.authRequiredMessage.value;
    // Per-session mirror — the modal reads the live `lastError` from here so
    // retries keep the input alive and the failure message sits next to the
    // form rather than competing with the composer banner.
    const auth = sess.liveSessionAuthState.value[sessionId] ?? null;
    const submitting = auth?.status === 'auth_required';
    const errorMessage = auth?.lastError?.message;

    if (!sessionId) {
        // Nothing to authenticate against — render a no-op close button.
        return null;
    }

    return (
        <ReauthModal
            methods={methods}
            message={message}
            submitting={!!submitting && !errorMessage}
            errorMessage={errorMessage}
            onClose={onClose}
            onSubmit={(methodId, credentials) => {
                // Resolve the ChatSession object from the session id so the
                // manager can forward through the live WebSocket. Falls back
                // to a minimal synthetic if the session list hasn't loaded
                // yet (rare — usually modal only opens for a known session).
                const session = sess.chatSessions.value.find(s => s.id === sessionId);
                if (!session) return;
                globalBridgeManager.authenticate(session, methodId, credentials);
            }}
        />
    );
}
