import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface FsRenameModalProps {
    title: string;
    onTitleChange: (val: string) => void;
    onClose: () => void;
    onSubmit: () => void;
    language: Lang;
    isDir: boolean;
}

export class FsRenameModal extends Component<FsRenameModalProps> {
    render() {
        const { title, onTitleChange, onClose, onSubmit, language, isDir } = this.props;
        const typeStr = t(isDir ? 'fileBrowser.typeFolder' : 'fileBrowser.typeFile', language);

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('fileBrowser.renameTitle', language, { type: typeStr })}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">{t('fileBrowser.renameLabel', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('fileBrowser.renamePlaceholder', language)}
                            value={title}
                            onInput={(e: Event) => onTitleChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                                else if (e.key === 'Escape') onClose();
                            }}
                            autoFocus
                        />
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onClose}>
                            {t('common.cancel', language)}
                        </button>
                        <button class="ws-modal-confirm" onClick={onSubmit}>
                            {t('common.confirm', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
