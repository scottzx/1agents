import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';

import { FileDetailView } from '../FileDetailView';
import { fsService } from '../../../services/fsService';
import * as fs from '../../../stores/fsStore';
import * as ui from '../../../stores/uiStore';

interface FeatureDocumentPreviewDrawerProps {
    open: boolean;
    path: string | null;
    onClose: () => void;
    onOpenInFiles: (path: string) => void;
}

export function FeatureDocumentPreviewDrawer({
    open,
    path,
    onClose,
    onOpenInFiles: _onOpenInFiles,
}: FeatureDocumentPreviewDrawerProps) {
    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [open, onClose]);

    useEffect(() => {
        if (!open || !path) return;
        const fileName = path.split('/').pop() || path;
        void fs.openFileDetail({
            name: fileName,
            path,
            isDir: false,
            size: 0,
            modTime: 0,
        });
    }, [open, path]);

    if (!open || !path) return null;

    const fileName = path.split('/').pop() || path;
    const selectedFsEntry = fs.selectedFsEntry.value;

    return (
        <Fragment>
            <div class="task-preview-backdrop" onClick={onClose} />
            <aside
                class="preview-drawer document-preview"
                role="dialog"
                aria-label={`文档预览：${fileName}`}
                style={{ display: 'flex', flexDirection: 'column', height: '100%' }}
            >
                <div
                    class="task-preview-body"
                    style={{ flex: 1, display: 'flex', flexDirection: 'column', padding: 0, overflow: 'hidden' }}
                >
                    {selectedFsEntry && selectedFsEntry.path === path ? (
                        <FileDetailView
                            selectedFsEntry={selectedFsEntry}
                            favoriteFiles={fs.favoriteFiles.value}
                            detailFullscreen={false}
                            isEditingDetail={fs.isEditingDetail.value}
                            fileContent={fs.fileContent.value}
                            editedContent={fs.editedContent.value}
                            fileLoading={fs.fileLoading.value}
                            fileSaving={fs.fileSaving.value}
                            fileSaveMsg={fs.fileSaveMsg.value}
                            isImagePreview={fs.isImagePreview.value}
                            imageUrl={fsService.imageUrl(selectedFsEntry.path)}
                            onBackToList={onClose}
                            onToggleFavorite={fs.toggleFavorite}
                            onCopyContent={fs.copyFileContent}
                            onDownloadFile={fs.downloadFile}
                            onRenameFile={fs.renameFile}
                            onToggleFullscreen={() => {}}
                            onShareFile={() => {}}
                            onSaveFile={fs.saveFile}
                            onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
                            onEditedContentChange={content => (fs.editedContent.value = content)}
                            isStandalone={false}
                            hideBack={false}
                            language={ui.language.value}
                        />
                    ) : (
                        <div style={{ padding: '24px', textAlign: 'center', color: 'var(--text-secondary)' }}>
                            加载文档中…
                        </div>
                    )}
                </div>
            </aside>
        </Fragment>
    );
}
