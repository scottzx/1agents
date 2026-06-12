import { h } from 'preact';
import { FileDetailView } from '../drawer/FileDetailView';
import { fsService } from '../../services/fsService';
import { t } from '../../i18n';
import type { App } from '../app';
import * as ui from '../../stores/uiStore';
import * as fs from '../../stores/fsStore';

interface FilePreviewContentProps {
    app: App;
    /** Id of the preview tab hosting this view; closing it goes "back". */
    activeTabId: string;
    onOpenPreview?: (path: string, name: string) => void;
}

/**
 * Shared standalone file-preview body used by both DesktopAppLayout
 * (preview tab overlay) and MobileAppLayout (preview subview). Renders
 * the FileDetailView for the selected fs entry, or a loading spinner
 * while the entry is being resolved. The surrounding chrome (tab
 * container vs. mobile subview header) stays platform-specific.
 */
export function FilePreviewContent({ app, activeTabId, onOpenPreview }: FilePreviewContentProps) {
    const language = ui.language.value;
    const selectedFsEntry = fs.selectedFsEntry.value;

    return selectedFsEntry ? (
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
            onBackToList={() => app.closeTab(activeTabId)}
            onToggleFavorite={fs.toggleFavorite}
            onCopyContent={fs.copyFileContent}
            onDownloadFile={fs.downloadFile}
            onRenameFile={fs.renameFile}
            onToggleFullscreen={() => {}}
            onShareFile={app.shareFile}
            onSaveFile={fs.saveFile}
            onToggleEditing={isEditing => (fs.isEditingDetail.value = isEditing)}
            onEditedContentChange={content => (fs.editedContent.value = content)}
            onOpenPreview={onOpenPreview}
            isStandalone={true}
            language={language}
        />
    ) : (
        <div class="fb-loading">
            <div class="fb-loading-spinner" />
            <span>{t('app.loading.preview', language)}</span>
        </div>
    );
}
