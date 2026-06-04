import { h, Component } from 'preact';
import { workspaceService } from '../../services/workspaceService';
import { t, type Lang } from '../i18n';

interface DirPickerModalProps {
    onClose: () => void;
    onSelect: (path: string) => void;
    onShowToast: (msg: string) => void;
    language: Lang;
}

interface DirPickerModalState {
    dirPickerPath: string;
    dirPickerParentPath: string;
    dirPickerDirs: { name: string; path: string }[];
    dirPickerLoading: boolean;
}

export class DirPickerModal extends Component<DirPickerModalProps, DirPickerModalState> {
    constructor(props: DirPickerModalProps) {
        super(props);
        this.state = {
            dirPickerPath: '',
            dirPickerParentPath: '',
            dirPickerDirs: [],
            dirPickerLoading: false,
        };
    }

    componentDidMount() {
        this.loadDirs('');
    }

    loadDirs = async (path: string) => {
        this.setState({ dirPickerLoading: true });
        try {
            const data = await workspaceService.listDirectories(path);
            this.setState({
                dirPickerPath: data.currentPath,
                dirPickerParentPath: data.parentPath || '',
                dirPickerDirs: data.directories || [],
                dirPickerLoading: false,
            });
        } catch (err) {
            this.props.onShowToast(t('modal.dirPicker.loadFailed', this.props.language, { err: String(err) }));
            this.setState({ dirPickerLoading: false });
        }
    };

    render() {
        const { onClose, onSelect, language } = this.props;
        const { dirPickerPath, dirPickerParentPath, dirPickerDirs, dirPickerLoading } = this.state;

        return (
            <div class="dp-modal-overlay" onClick={onClose}>
                <div class="dp-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    <div class="dp-modal-header">
                        <span>{t('modal.dirPicker.title', language)}</span>
                        <button class="dp-modal-close" onClick={onClose}>
                            ✕
                        </button>
                    </div>
                    <div class="dp-modal-body">
                        <div class="dp-path-row">
                            {dirPickerParentPath && (
                                <button
                                    class="dp-up-btn"
                                    onClick={() => this.loadDirs(dirPickerParentPath)}
                                    title={t('modal.dirPicker.up', language)}
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="15 18 9 12 15 6" />
                                    </svg>
                                </button>
                            )}
                            <input
                                class="dp-path-input"
                                value={dirPickerPath}
                                onInput={(e: Event) =>
                                    this.setState({
                                        dirPickerPath: (e.target as HTMLInputElement).value,
                                    })
                                }
                                onKeyDown={(e: KeyboardEvent) => {
                                    if (e.key === 'Enter') this.loadDirs(dirPickerPath);
                                }}
                                placeholder={t('modal.dirPicker.placeholder', language)}
                            />
                            <button class="dp-go-btn" onClick={() => this.loadDirs(dirPickerPath)}>
                                {t('modal.dirPicker.go', language)}
                            </button>
                        </div>

                        <div class="dp-dir-list-wrap">
                            {dirPickerLoading ? (
                                <div class="dp-loading">
                                    <div class="dp-spinner" />
                                    <span>{t('modal.dirPicker.loading', language)}</span>
                                </div>
                            ) : dirPickerDirs.length === 0 ? (
                                <div class="dp-empty">{t('modal.dirPicker.empty', language)}</div>
                            ) : (
                                <div class="dp-dir-list">
                                    {dirPickerDirs.map(dir => (
                                        <div key={dir.path} class="dp-dir-item" onClick={() => this.loadDirs(dir.path)}>
                                            <svg
                                                class="dp-folder-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                            </svg>
                                            <span class="dp-dir-name" title={dir.path}>
                                                {dir.name}
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>
                    <div class="dp-modal-footer">
                        <button class="dp-modal-cancel" onClick={onClose}>
                            {t('common.cancel', language)}
                        </button>
                        <button
                            class="dp-modal-confirm"
                            onClick={() => {
                                onSelect(dirPickerPath);
                            }}
                        >
                            {t('modal.dirPicker.selectCurrent', language)}
                        </button>
                    </div>
                </div>
            </div>
        );
    }
}
