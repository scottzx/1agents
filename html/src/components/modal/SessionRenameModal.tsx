import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface SessionRenameModalProps {
    title: string;
    onTitleChange: (val: string) => void;
    onClose: () => void;
    onSubmit: () => void;
    language: Lang;
}

export class SessionRenameModal extends Component<SessionRenameModalProps> {
    render() {
        const { title, onTitleChange, onClose, onSubmit, language } = this.props;

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{t('modal.session.renameTitle', language)}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">{t('modal.session.name', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.session.namePlaceholder', language)}
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
                            {t('modal.session.save', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
