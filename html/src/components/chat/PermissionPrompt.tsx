import { h, Component } from 'preact';
import type { ChatItem } from './hooks';

interface PermissionPromptProps {
    item: Extract<ChatItem, { kind: 'permission' }>;
    onRespond: (requestId: string, allow: boolean) => void;
}

/**
 * Modal permission prompt. The cc-connect bridge protocol surfaces
 * permission requests as `buttons` messages; this component renders
 * the most recent unresolved one as a modal and emits the user's
 * decision back through the bridge.
 */
export class PermissionPrompt extends Component<PermissionPromptProps> {
    render() {
        const { item, onRespond } = this.props;
        if (item.resolved) return null;
        return (
            <div class="perm-overlay" onClick={() => onRespond(item.requestId, false)}>
                <div class="perm-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="perm-header">需要您的授权</div>
                    <div class="perm-tool">工具：{item.toolName}</div>
                    <pre class="perm-input">{item.input}</pre>
                    <div class="perm-actions">
                        <button class="perm-deny" onClick={() => onRespond(item.requestId, false)}>
                            拒绝
                        </button>
                        <button class="perm-allow" onClick={() => onRespond(item.requestId, true)}>
                            允许
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
