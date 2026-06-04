import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface WorkspaceModalProps {
    mode: 'create' | 'rename';
    name: string;
    path: string;
    terminalDir: string;
    chatChannel: string;
    onNameChange: (val: string) => void;
    onPathChange: (val: string) => void;
    onTerminalDirChange: (val: string) => void;
    onChatChannelChange: (val: string) => void;
    onClose: () => void;
    onBrowse: () => void;
    onSubmit: () => void;
    language: Lang;
}

export class WorkspaceModal extends Component<WorkspaceModalProps> {
    render() {
        const {
            mode,
            name,
            path,
            terminalDir,
            chatChannel,
            onNameChange,
            onPathChange,
            onTerminalDirChange,
            onChatChannelChange,
            onClose,
            onBrowse,
            onSubmit,
            language,
        } = this.props;

        return (
            <div class="ws-modal-overlay" onClick={onClose}>
                <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="ws-modal-header">
                        <span>
                            {mode === 'create'
                                ? t('modal.workspace.createTitle', language)
                                : t('modal.workspace.editTitle', language)}
                        </span>
                        <button class="ws-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="ws-modal-body">
                        <label class="ws-modal-label">{t('modal.workspace.name', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.workspace.namePlaceholder', language)}
                            value={name}
                            onInput={(e: Event) => onNameChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                            autoFocus
                        />
                        <label class="ws-modal-label">{t('modal.workspace.path', language)}</label>
                        <div style="display: flex; gap: 8px; width: 100%;">
                            <input
                                class="ws-modal-input"
                                placeholder={t('modal.workspace.pathPlaceholder', language)}
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
                                {t('modal.workspace.browse', language)}
                            </button>
                        </div>
                        <label class="ws-modal-label">{t('modal.workspace.terminalDir', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.workspace.terminalDirPlaceholder', language)}
                            value={terminalDir}
                            onInput={(e: Event) => onTerminalDirChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                        />
                        <label class="ws-modal-label">{t('modal.workspace.chatChannel', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.workspace.chatChannelPlaceholder', language)}
                            value={chatChannel}
                            onInput={(e: Event) => onChatChannelChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter') onSubmit();
                            }}
                        />
                    </div>
                    <div class="ws-modal-footer">
                        <button class="ws-modal-cancel" onClick={onClose}>
                            {t('common.cancel', language)}
                        </button>
                        <button class="ws-modal-confirm" onClick={onSubmit}>
                            {mode === 'create'
                                ? t('modal.workspace.create', language)
                                : t('modal.workspace.save', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
