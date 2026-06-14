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
    showNewFolderInput: boolean;
    newFolderName: string;
    recentPaths: string[];
}

export class DirPickerModal extends Component<DirPickerModalProps, DirPickerModalState> {
    constructor(props: DirPickerModalProps) {
        super(props);
        this.state = {
            dirPickerPath: '',
            dirPickerParentPath: '',
            dirPickerDirs: [],
            dirPickerLoading: false,
            showNewFolderInput: false,
            newFolderName: '',
            recentPaths: [],
        };
    }

    componentDidMount() {
        this.loadDirs('');
        this.loadRecentPaths();
    }

    loadRecentPaths = () => {
        try {
            const raw = localStorage.getItem('1agents_recent_dirs');
            if (raw) {
                const paths = JSON.parse(raw);
                if (Array.isArray(paths)) {
                    this.setState({ recentPaths: paths });
                    return;
                }
            }
        } catch (e) {
            console.error('Failed to parse recent paths', e);
        }
        this.setState({ recentPaths: [] });
    };

    handleCreateFolder = async () => {
        const { newFolderName, dirPickerPath } = this.state;
        const { onShowToast, language } = this.props;
        const name = newFolderName.trim();
        if (!name) return;

        try {
            const newPath = await workspaceService.createDirectory(dirPickerPath, name);
            onShowToast(t('modal.dirPicker.createSuccess', language));
            this.setState({ showNewFolderInput: false, newFolderName: '' });
            this.loadDirs(newPath);
        } catch (err) {
            onShowToast(t('modal.dirPicker.createFailed', language, { err: String(err) }));
        }
    };

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
        const {
            dirPickerPath,
            dirPickerParentPath,
            dirPickerDirs,
            dirPickerLoading,
            showNewFolderInput,
            newFolderName,
            recentPaths,
        } = this.state;

        const presets = [
            { labelKey: 'modal.dirPicker.presetHome', path: '~' },
            { labelKey: 'modal.dirPicker.presetDesktop', path: '~/Desktop' },
            { labelKey: 'modal.dirPicker.presetDocuments', path: '~/Documents' },
        ];

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
                            <button
                                class="dp-new-folder-btn"
                                onClick={() =>
                                    this.setState({ showNewFolderInput: !showNewFolderInput, newFolderName: '' })
                                }
                                title={t('modal.dirPicker.newFolder', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <line x1="12" y1="5" x2="12" y2="19"></line>
                                    <line x1="5" y1="12" x2="19" y2="12"></line>
                                </svg>
                            </button>
                        </div>

                        {showNewFolderInput && (
                            <div class="dp-new-folder-row">
                                <input
                                    class="dp-new-folder-input"
                                    value={newFolderName}
                                    onInput={(e: Event) =>
                                        this.setState({ newFolderName: (e.target as HTMLInputElement).value })
                                    }
                                    placeholder={t('modal.dirPicker.newFolderPlaceholder', language)}
                                    onKeyDown={(e: KeyboardEvent) => {
                                        if (e.key === 'Enter') this.handleCreateFolder();
                                    }}
                                    autoFocus
                                />
                                <button class="dp-new-folder-confirm" onClick={this.handleCreateFolder}>
                                    {t('common.confirm', language)}
                                </button>
                                <button
                                    class="dp-new-folder-cancel"
                                    onClick={() => this.setState({ showNewFolderInput: false, newFolderName: '' })}
                                >
                                    {t('common.cancel', language)}
                                </button>
                            </div>
                        )}

                        <div class="dp-shortcuts-bar">
                            <div class="dp-presets">
                                {presets.map(p => (
                                    <button
                                        key={p.path}
                                        class="dp-shortcut-btn"
                                        onClick={() => this.loadDirs(p.path)}
                                        title={p.path}
                                    >
                                        {t(p.labelKey, language)}
                                    </button>
                                ))}
                            </div>
                            {recentPaths.length > 0 && (
                                <div class="dp-recents">
                                    <span class="dp-recents-label">{t('modal.dirPicker.recent', language)}:</span>
                                    {recentPaths.map(p => {
                                        const parts = p.split(new RegExp('[\\\\/]'));
                                        const name = parts[parts.length - 1] || p;
                                        return (
                                            <button
                                                key={p}
                                                class="dp-shortcut-btn dp-recent-btn"
                                                onClick={() => this.loadDirs(p)}
                                                title={p}
                                            >
                                                {name}
                                            </button>
                                        );
                                    })}
                                </div>
                            )}
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
