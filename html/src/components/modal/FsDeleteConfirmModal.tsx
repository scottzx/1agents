import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface FsDeleteConfirmModalProps {
    onClose: () => void;
    onSubmit: () => void;
    language: Lang;
    name: string;
    isDir: boolean;
}

export class FsDeleteConfirmModal extends Component<FsDeleteConfirmModalProps> {
    render() {
        const { onClose, onSubmit, language, name, isDir } = this.props;
        const typeStr = t(isDir ? 'fileBrowser.typeFolder' : 'fileBrowser.typeFile', language);

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('fileBrowser.deleteConfirmTitle', language)}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div
                        class="ws-modal-body"
                        style="font-size: 13.5px; line-height: 1.6; color: var(--text-main); padding: 16px 20px;"
                    >
                        <p>{t('fileBrowser.deleteConfirmMessage', language, { type: typeStr })}</p>
                        <p style="font-weight: 600; margin: 8px 0; word-break: break-all; color: var(--accent-color);">
                            {name}
                        </p>
                        {isDir && (
                            <p style="color: var(--danger-fg); font-size: 12px; margin-top: 12px; background-color: rgba(var(--danger-rgb), 0.08); padding: 8px; border-radius: 6px; border: 1px solid rgba(var(--danger-rgb), 0.12);">
                                {t('fileBrowser.deleteConfirmWarn', language)}
                            </p>
                        )}
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onClose}>
                            {t('common.cancel', language)}
                        </button>
                        <button class="ws-modal-confirm ws-modal-confirm-danger" onClick={onSubmit}>
                            {t('fileBrowser.delete', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
