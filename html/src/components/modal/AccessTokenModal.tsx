import { h, Component } from 'preact';

interface AccessTokenModalProps {
    token: string;
    onClose: () => void;
    onShowToast: (msg: string) => void;
}

export class AccessTokenModal extends Component<AccessTokenModalProps> {
    copyAccessToken = () => {
        const { token, onShowToast } = this.props;
        if (!token) return;
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard
                .writeText(token)
                .then(() => {
                    onShowToast('令牌已复制到剪贴板 ✓');
                })
                .catch(() => {
                    onShowToast('复制失败，请手动复制');
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
                onShowToast('令牌已复制到剪贴板 ✓');
            } catch {
                onShowToast('复制失败，请手动复制');
            }
            document.body.removeChild(textarea);
        }
    };

    render() {
        const { token, onClose } = this.props;

        return (
            <div class="at-modal-overlay" onClick={onClose}>
                <div class="at-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="at-modal-header">访问令牌已生成</div>
                    <div class="at-modal-warning">
                        <strong>请立即保存此令牌！</strong>此令牌仅在本次展示，关闭后将无法再次查看。
                        请将其妥善保管，非本地网络访问时需要提供此令牌。
                    </div>
                    <div class="at-modal-token-box">
                        <span class="at-modal-token-text">{token}</span>
                        <button class="at-modal-copy-btn" onClick={this.copyAccessToken}>
                            复制
                        </button>
                    </div>
                    <button class="at-modal-close-btn" onClick={onClose}>
                        我已保存令牌
                    </button>
                </div>
            </div>
        );
    }
}
