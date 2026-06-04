import { h, Component } from 'preact';

interface SettingsModalProps {
    show: boolean;
    onClose: () => void;
    
    // Theme options
    theme: 'light' | 'dark';
    toggleTheme: (mode?: 'light' | 'dark') => void;
    
    // Dictation & language
    language: 'zh-CN' | 'en-US';
    toggleLanguage: (lang: 'zh-CN' | 'en-US') => void;
    
    // Tmux Mouse toggle
    tmuxMouseOn: boolean;
    toggleTmuxMouse: () => Promise<void>;
    
    // Access token
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
    
    // Workspace state
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    workspaceName: string;
    
    // Cache management callback
    onClearWorkspaceCache?: () => void;
    onShowToast: (msg: string) => void;
}

interface SettingsModalState {
    activeCategory: 'general' | 'appearance' | 'security' | 'system';
    pinging: boolean;
    pingStatus: 'success' | 'failed' | null;
}

export class SettingsModal extends Component<SettingsModalProps, SettingsModalState> {
    constructor(props: SettingsModalProps) {
        super(props);
        this.state = {
            activeCategory: 'general',
            pinging: false,
            pingStatus: null,
        };
    }

    componentDidMount() {
        this.testBackendConnection();
    }

    testBackendConnection = async () => {
        this.setState({ pinging: true, pingStatus: null });
        try {
            const res = await fetch('/api/access/status');
            if (res.ok) {
                this.setState({ pingStatus: 'success', pinging: false });
            } else {
                this.setState({ pingStatus: 'failed', pinging: false });
            }
        } catch {
            this.setState({ pingStatus: 'failed', pinging: false });
        }
    };

    handleCacheReset = async () => {
        if (!confirm('确定要清除系统缓存并重置所有设置吗？\n这将清除文件浏览器缓存、偏好配置（主题、语言等）并强制重新加载页面。')) {
            return;
        }

        // 1. Clear parent cache if callback exists
        if (this.props.onClearWorkspaceCache) {
            this.props.onClearWorkspaceCache();
        }

        // 2. Clear localStorage variables
        localStorage.removeItem('1agents-theme');
        localStorage.removeItem('1agents-language');
        localStorage.removeItem('1agents-active-workspace');
        localStorage.removeItem('fav-files');
        localStorage.removeItem('1agents-onboarded');

        // 3. Clear Service Worker caches
        if ('caches' in window) {
            try {
                const keys = await caches.keys();
                await Promise.all(keys.map(key => caches.delete(key)));
            } catch (e) {
                console.error('[Settings] Cache deletion failed:', e);
            }
        }

        this.props.onShowToast('所有缓存已清除，页面即将重新加载...');
        
        setTimeout(() => {
            window.location.reload();
        }, 1500);
    };

    renderCategoryMenu() {
        const categories = [
            { id: 'general', label: '通用设置', desc: '语言、工作空间、主路径', icon: 'M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm1 14h-2v-6h2zm0-8h-2V7h2z' },
            { id: 'appearance', label: '外观与终端', desc: '色彩主题、终端交互行为', icon: 'M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z' },
            { id: 'security', label: '安全设置', desc: '外部访问令牌与验证', icon: 'M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z' },
            { id: 'system', label: '系统维护与信息', desc: '系统诊断、重置与缓存清理', icon: 'M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6' },
        ];

        return categories.map(cat => (
            <button
                key={cat.id}
                class={`settings-sidebar-item ${this.state.activeCategory === cat.id ? 'active' : ''}`}
                onClick={() => this.setState({ activeCategory: cat.id as any })}
            >
                <div class="settings-sidebar-icon">
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                        {cat.id === 'general' && <circle cx="12" cy="12" r="10" />}
                        {cat.id === 'general' && <line x1="12" y1="16" x2="12" y2="12" />}
                        {cat.id === 'general' && <line x1="12" y1="8" x2="12.01" y2="8" />}
                        
                        {cat.id === 'appearance' && <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />}
                        {cat.id === 'appearance' && <line x1="8" y1="21" x2="16" y2="21" />}
                        {cat.id === 'appearance' && <line x1="12" y1="17" x2="12" y2="21" />}
                        
                        {cat.id === 'security' && <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />}
                        {cat.id === 'security' && <path d="M7 11V7a5 5 0 0 1 10 0v4" />}
                        
                        {cat.id === 'system' && <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z" />}
                    </svg>
                </div>
                <div class="settings-sidebar-text">
                    <div class="settings-sidebar-label">{cat.label}</div>
                    <div class="settings-sidebar-desc">{cat.desc}</div>
                </div>
            </button>
        ));
    }

    renderContent() {
        const {
            theme,
            toggleTheme,
            language,
            toggleLanguage,
            tmuxMouseOn,
            toggleTmuxMouse,
            accessTokenExists,
            onGenerateAccessToken,
            onRevokeAccessToken,
            activeWorkspaceId,
            activeWorkspacePath,
            workspaceName,
        } = this.props;

        switch (this.state.activeCategory) {
            case 'general':
                return (
                    <div class="settings-section">
                        <div class="settings-section-title">通用设置 (General Settings)</div>
                        
                        <div class="settings-item">
                            <label class="settings-label">系统语言 & 识别语言 (Language & Dictation)</label>
                            <span class="settings-item-desc">选择界面的默认语言，以及语音输入识别时使用的主要方言。</span>
                            <div class="settings-btn-group">
                                <button
                                    class={`settings-toggle-btn ${language === 'zh-CN' ? 'active' : ''}`}
                                    onClick={() => toggleLanguage('zh-CN')}
                                >
                                    中文 (Chinese - zh-CN)
                                </button>
                                <button
                                    class={`settings-toggle-btn ${language === 'en-US' ? 'active' : ''}`}
                                    onClick={() => toggleLanguage('en-US')}
                                >
                                    English (US - en-US)
                                </button>
                            </div>
                        </div>

                        <div class="settings-item">
                            <label class="settings-label">当前活跃工作空间 (Active Workspace)</label>
                            <span class="settings-item-desc">显示当前交互及文件管理器所处的工作空间路径。</span>
                            <div class="settings-info-box">
                                <div class="info-row">
                                    <span class="info-key">空间名称 (Name):</span>
                                    <span class="info-val font-semibold">{workspaceName || '未指定'}</span>
                                </div>
                                <div class="info-row">
                                    <span class="info-key">物理路径 (Path):</span>
                                    <span class="info-val font-mono">{activeWorkspacePath || '未指定'}</span>
                                </div>
                                <div class="info-row">
                                    <span class="info-key">空间ID (ID):</span>
                                    <span class="info-val font-mono">{activeWorkspaceId || '未指定'}</span>
                                </div>
                            </div>
                        </div>
                    </div>
                );

            case 'appearance':
                return (
                    <div class="settings-section">
                        <div class="settings-section-title">外观与终端 (Appearance & Terminal)</div>
                        
                        <div class="settings-item">
                            <label class="settings-label font-medium">色彩主题 (Color Theme)</label>
                            <span class="settings-item-desc">切换整个开发台与终端模拟器的色彩主题模式。</span>
                            <div class="theme-options">
                                <button
                                    class={`theme-btn ${theme === 'light' ? 'active' : ''}`}
                                    onClick={() => toggleTheme('light')}
                                >
                                    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                        <circle cx="12" cy="12" r="5" />
                                        <line x1="12" y1="1" x2="12" y2="3" />
                                        <line x1="12" y1="21" x2="12" y2="23" />
                                        <line x1="4.22" y1="4.22" x2="5.64" y2="5.64" />
                                        <line x1="18.36" y1="18.36" x2="19.78" y2="19.78" />
                                        <line x1="1" y1="12" x2="3" y2="12" />
                                        <line x1="21" y1="12" x2="23" y2="12" />
                                        <line x1="4.22" y1="19.78" x2="5.64" y2="18.36" />
                                        <line x1="18.36" y1="5.64" x2="19.78" y2="4.22" />
                                    </svg>
                                    <span>浅色模式 (Light)</span>
                                </button>
                                <button
                                    class={`theme-btn ${theme === 'dark' ? 'active' : ''}`}
                                    onClick={() => toggleTheme('dark')}
                                >
                                    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                                        <path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z" />
                                    </svg>
                                    <span>深色模式 (Dark)</span>
                                </button>
                            </div>
                        </div>

                        <div class="settings-item">
                            <label class="settings-label font-medium">Tmux 鼠标交互行为 (Tmux Mouse Behavior)</label>
                            <span class="settings-item-desc">切换终端鼠标控制方式。开启后可滚动日志历史；关闭后可使用终端自带复制。</span>
                            
                            <div class="settings-switch-wrapper" onClick={() => toggleTmuxMouse()}>
                                <div class={`settings-switch ${tmuxMouseOn ? 'checked' : ''}`}>
                                    <div class="settings-switch-handle" />
                                </div>
                                <span class="settings-switch-label">
                                    {tmuxMouseOn ? '滚轮滑动模式 (使用鼠标滚轮滚动日志)' : '鼠标拖拽模式 (可直接拖拽选中复制文本)'}
                                </span>
                            </div>
                        </div>
                    </div>
                );

            case 'security':
                return (
                    <div class="settings-section">
                        <div class="settings-section-title">安全设置 (Security Settings)</div>
                        
                        <div class="settings-item">
                            <label class="settings-label">远程访问令牌 (Remote Access Token)</label>
                            <span class="settings-item-desc">
                                当非本地网络（例如外网穿透、远程 IP 访问）尝试访问工作区时，需要输入令牌以防未授权侵入。
                            </span>
                            
                            <div class="settings-status-indicator">
                                <span class={`status-dot ${accessTokenExists ? 'active' : 'inactive'}`} />
                                <span class="status-text">
                                    {accessTokenExists ? '已设置访问令牌防护 (受保护)' : '未设置访问令牌 (仅限制本地网段免密)'}
                                </span>
                            </div>
                            
                            <div class="settings-btn-group" style="margin-top: 12px;">
                                {accessTokenExists ? (
                                    <button class="settings-danger-btn" onClick={onRevokeAccessToken}>
                                        撤销现有令牌 (Revoke)
                                    </button>
                                ) : (
                                    <button class="settings-primary-btn" onClick={onGenerateAccessToken}>
                                        生成新访问令牌 (Generate)
                                    </button>
                                )}
                            </div>
                        </div>
                    </div>
                );

            case 'system':
                const { pinging, pingStatus } = this.state;
                return (
                    <div class="settings-section">
                        <div class="settings-section-title">系统维护与信息 (System Maintenance & Info)</div>
                        
                        <div class="settings-item">
                            <label class="settings-label">系统诊断 (System Diagnostics)</label>
                            <div class="settings-diagnostic-box">
                                <div class="diag-row">
                                    <span>后端 API 通信连接:</span>
                                    {pinging ? (
                                        <span class="diag-status loading">测试中...</span>
                                    ) : pingStatus === 'success' ? (
                                        <span class="diag-status success">✓ 连接成功</span>
                                    ) : (
                                        <span class="diag-status failed">✗ 连接失败</span>
                                    )}
                                </div>
                                <div class="diag-row">
                                    <span>客户端内核 (User Agent):</span>
                                    <span class="diag-value font-mono">{navigator.userAgent.slice(0, 50)}...</span>
                                </div>
                                <div class="diag-row">
                                    <span>系统屏幕分辨率:</span>
                                    <span class="diag-value font-mono">{window.innerWidth} × {window.innerHeight}</span>
                                </div>
                            </div>
                            <button
                                class="settings-secondary-btn"
                                style="margin-top: 10px;"
                                onClick={this.testBackendConnection}
                                disabled={pinging}
                            >
                                重新检测连接
                            </button>
                        </div>

                        <div class="settings-item">
                            <label class="settings-label text-danger">重置偏好及系统缓存 (Reset Preferences & Cache)</label>
                            <span class="settings-item-desc text-danger-muted">
                                当界面显示错误、目录加载闪烁或缓存需要同步时，可以使用该功能清除浏览器 LocalStorage 和 Service Worker 离线缓存，重置后页面将强制刷新。
                            </span>
                            <button class="settings-danger-btn" onClick={this.handleCacheReset}>
                                清除缓存并重置页面 (Clear Cache & Reset)
                            </button>
                        </div>
                    </div>
                );

            default:
                return null;
        }
    }

    render() {
        const { show, onClose } = this.props;

        if (!show) return null;

        return (
            <div class="settings-overlay" onClick={onClose}>
                <div class="settings-card" onClick={(e: MouseEvent) => e.stopPropagation()}>
                    {/* Left category navigation */}
                    <div class="settings-sidebar">
                        <div class="settings-sidebar-header">
                            <h2>设置</h2>
                            <p>Settings & Configuration</p>
                        </div>
                        <div class="settings-sidebar-menu">
                            {this.renderCategoryMenu()}
                        </div>
                        <div class="settings-sidebar-footer">
                            <button class="settings-close-btn" onClick={onClose}>
                                关闭面板
                            </button>
                        </div>
                    </div>

                    {/* Right content view */}
                    <div class="settings-main-container">
                        <div class="settings-main-header">
                            <button class="settings-mobile-back" onClick={onClose} title="返回">
                                <svg viewBox="0 0 24 24" width="18" height="18" fill="none" stroke="currentColor" stroke-width="2">
                                    <polyline points="15 18 9 12 15 6" />
                                </svg>
                            </button>
                            <span class="settings-category-title">
                                {this.state.activeCategory === 'general' && '通用设置'}
                                {this.state.activeCategory === 'appearance' && '外观与终端'}
                                {this.state.activeCategory === 'security' && '安全设置'}
                                {this.state.activeCategory === 'system' && '系统维护与信息'}
                            </span>
                            <button class="settings-close-x" onClick={onClose} title="关闭 (Esc)">
                                ✕
                            </button>
                        </div>
                        <div class="settings-main-body">
                            {this.renderContent()}
                        </div>
                    </div>
                </div>
            </div>
        );
    }
}
