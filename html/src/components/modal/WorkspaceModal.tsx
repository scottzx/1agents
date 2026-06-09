import { h, Component } from 'preact';
import { AgentTypePicker } from '../chat/AgentTypePicker';
import { type AgentType } from '../types';
import { DEFAULT_AGENT_TYPE } from '../../services/agentService';

interface WorkspaceModalProps {
    mode: 'create' | 'rename';
    name: string;
    path: string;
    terminalDir: string;
    chatChannel: string;
    defaultAgent: AgentType;
    onNameChange: (val: string) => void;
    onPathChange: (val: string) => void;
    onTerminalDirChange: (val: string) => void;
    onChatChannelChange: (val: string) => void;
    onDefaultAgentChange: (val: AgentType) => void;
    onClose: () => void;
    onBrowse: () => void;
    onSubmit: () => void;
}

export class WorkspaceModal extends Component<WorkspaceModalProps> {
    render() {
        const {
            mode,
            name,
            path,
            terminalDir,
            chatChannel,
            defaultAgent,
            onNameChange,
            onPathChange,
            onTerminalDirChange,
            onChatChannelChange,
            onDefaultAgentChange,
            onClose,
            onBrowse,
            onSubmit,
        } = this.props;

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>{mode === 'create' ? '新建工作空间' : '编辑工作空间'}</span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">名称</label>
                        <input
                            class="ws-modal-input"
                            placeholder="工作空间名称"
                            value={name}
                            onInput={(e: Event) => onNameChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                            autoFocus
                        />
                        <label class="ws-modal-label">路径</label>
                        <div style="display: flex; gap: 8px; width: 100%;">
                            <input
                                class="ws-modal-input"
                                placeholder="/path/to/project  (可选)"
                                value={path}
                                onInput={(e: Event) => onPathChange((e.target as HTMLInputElement).value)}
                                onKeyDown={(e: KeyboardEvent) => {
                                    if (e.key === 'Enter') onSubmit();
                                }}
                                style="flex: 1;"
                            />
                            <button
                                class="ws-modal-cancel"
                                onClick={onBrowse}
                                style="height: 38px; flex-shrink: 0; padding: 0 12px; margin: 0; font-size: 12px; display: flex; align-items: center; justify-content: center;"
                            >
                                浏览...
                            </button>
                        </div>
                        <label class="ws-modal-label">终端文件夹 (可选)</label>
                        <input
                            class="ws-modal-input"
                            placeholder="终端窗口默认打开的目录 (重写路径)"
                            value={terminalDir}
                            onInput={(e: Event) => onTerminalDirChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                        />
                        <label class="ws-modal-label">默认智能体</label>
                        <AgentTypePicker value={defaultAgent || DEFAULT_AGENT_TYPE} onChange={onDefaultAgentChange} />
                        <p class="ws-modal-hint">新建聊天会话时默认使用此智能体；可以在创建会话时改为其他类型。</p>
                        <label class="ws-modal-label">AI 聊天频道 (可选)</label>
                        <input
                            class="ws-modal-input"
                            placeholder="CC-Connect 聊天频道或会话 key"
                            value={chatChannel}
                            onInput={(e: Event) => onChatChannelChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                        />
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onClose}>
                            取消
                        </button>
                        <button class="ws-modal-confirm" onClick={onSubmit}>
                            {mode === 'create' ? '创建' : '保存'}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
