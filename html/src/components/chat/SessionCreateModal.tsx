import { h, Component } from 'preact';
import { AGENT_TYPE_LABELS, type AgentType } from '../types';
import { AgentTypePicker } from './AgentTypePicker';

interface SessionCreateModalProps {
    workspaceId: string;
    workspaceName: string;
    defaultAgent: AgentType;
    onCancel: () => void;
    onSubmit: (name: string, agentType: AgentType) => void;
    onPickAgent?: (onChange: (t: AgentType) => void) => void;
}

/**
 * Modal for "new chat session" — sibling to the implicit "new terminal"
 * flow that fires when the sidebar `+` is pressed. Reached via the
 * new "新建聊天" entry in the sidebar's `+` dropdown.
 */
export class SessionCreateModal extends Component<SessionCreateModalProps> {
    state = {
        name: '',
        agentType: this.props.defaultAgent,
    };

    componentDidUpdate(prevProps: SessionCreateModalProps) {
        if (prevProps.defaultAgent !== this.props.defaultAgent && this.state.agentType === prevProps.defaultAgent) {
            this.setState({ agentType: this.props.defaultAgent });
        }
    }

    render() {
        const { workspaceName, onCancel, onSubmit } = this.props;
        return (
            <div class="ws-modal-overlay" onClick={onCancel}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>新建聊天会话 · {workspaceName}</span>
                        <button class="ws-modal-close" onClick={onCancel}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">会话名称（可选）</label>
                        <input
                            class="ws-modal-input"
                            placeholder="留空将自动生成"
                            value={this.state.name}
                            onInput={(e: Event) => this.setState({ name: (e.target as HTMLInputElement).value })}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit(this.state.name, this.state.agentType);
                            }}
                            autoFocus
                        />
                        <label class="ws-modal-label">智能体类型</label>
                        <AgentTypePicker value={this.state.agentType} onChange={t => this.setState({ agentType: t })} />
                        <p class="ws-modal-hint">
                            会话创建后将固定使用 {AGENT_TYPE_LABELS[this.state.agentType] ?? this.state.agentType}，
                            不可在会话过程中更换。
                        </p>
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onCancel}>
                            取消
                        </button>
                        <button
                            class="ws-modal-confirm"
                            onClick={() => onSubmit(this.state.name, this.state.agentType)}
                        >
                            创建
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
