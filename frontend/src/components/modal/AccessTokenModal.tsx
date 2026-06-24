import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface AccessTokenModalProps {
    token: string;
    onClose: () => void;
    onShowToast: (msg: string) => void;
    language: Lang;
}

export class AccessTokenModal extends Component<AccessTokenModalProps> {
    copyAccessToken = () => {
        const { token, onShowToast, language } = this.props;
        if (!token) return;
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard
                .writeText(token)
                .then(() => {
                    onShowToast(t('modal.token.copied', language));
                })
                .catch(() => {
                    onShowToast(t('modal.token.copyFailed', language));
                });
        } else {
            // Fallback for non-HTTPS or older browsers
            const textarea = document.createElement('textarea');
            textarea.value = token;
            textarea.style.position = 'fixed';
            textarea.style.opacity = '0';
            document.body.appendChild(textarea);
            textarea.select();
            try {
                document.execCommand('copy');
                onShowToast(t('modal.token.copied', language));
            } catch {
                onShowToast(t('modal.token.copyFailed', language));
            }
            document.body.removeChild(textarea);
        }
    };

    render() {
        const { token, onClose, language } = this.props;

        return (
            <div class="at-modal-overlay" onClick={onClose}>
                <div class="at-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="at-modal-header">{t('modal.token.title', language)}</div>
                    <div class="at-modal-warning">
                        <strong>{t('modal.token.warningLabel', language)}</strong>
                        {t('modal.token.warningBody1', language)} {t('modal.token.warningBody2', language)}
                    </div>
                    <div class="at-modal-token-box">
                        <span class="at-modal-token-text">{token}</span>
                        <button class="at-modal-copy-btn" onClick={this.copyAccessToken}>
                            {t('modal.token.copy', language)}
                        </button>
                    </div>
                    <button class="at-modal-close-btn" onClick={onClose}>
                        {t('modal.token.ack', language)}
                    </button>
                </div>
            </div>
        );
    }
}
