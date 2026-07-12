import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';
import type { FsNewItemKind } from '../../stores/modalStore';

interface FsCreateModalProps {
    name: string;
    kind: FsNewItemKind;
    parentName: string | null;
    onNameChange: (val: string) => void;
    onClose: () => void;
    onSubmit: () => void;
    language: Lang;
}

export class FsCreateModal extends Component<FsCreateModalProps> {
    render() {
        const { name, kind, parentName, onNameChange, onClose, onSubmit, language } = this.props;
        const typeKey = kind === 'folder' ? 'fileBrowser.typeFolder' : 'fileBrowser.typeFile';

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('fileBrowser.createTitle', language, { type: t(typeKey, language) })}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">
                            {t('fileBrowser.createLabel', language)}
                            {parentName && (
                                <span class="ws-modal-hint" style="margin-left: 6px; color: var(--text-muted);">
                                    ({t('fileBrowser.createIn', language, { parent: parentName })})
                                </span>
                            )}
                        </label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('fileBrowser.createPlaceholder', language, {
                                type: t(typeKey, language),
                            })}
                            value={name}
                            onInput={(e: Event) => onNameChange((e.target as HTMLInputElement).value)}
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
