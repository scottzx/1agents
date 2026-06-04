import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';

type SettingsCategory = 'general' | 'appearance' | 'security' | 'feedback' | 'about';

interface SystemSettingsProps {
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    language: 'zh-CN' | 'en-US';
    toggleLanguage: (lang: 'zh-CN' | 'en-US') => void;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
}

const NAV_ITEMS: { key: SettingsCategory; labelZh: string; labelEn: string; icon: h.JSX.Element }[] = [
    {
        key: 'general',
        labelZh: '通用设置',
        labelEn: 'General',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <circle cx="12" cy="12" r="3" />
                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
            </svg>
        ),
    },
    {
        key: 'appearance',
        labelZh: '外观与终端',
        labelEn: 'Appearance & Terminal',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <circle cx="12" cy="12" r="4" />
                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
            </svg>
        ),
    },
    {
        key: 'security',
        labelZh: '安全设置',
        labelEn: 'Security',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z" />
            </svg>
        ),
    },
    {
        key: 'feedback',
        labelZh: '反馈与联系',
        labelEn: 'Feedback & Contact',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                <polyline points="22,6 12,13 2,6" />
            </svg>
        ),
    },
    {
        key: 'about',
        labelZh: '关于与维护',
        labelEn: 'About & Maintenance',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
            </svg>
        ),
    },
];

export function SystemSettings({
    theme,
    toggleTheme,
    language,
    toggleLanguage,
    tmuxMouseOn,
    onTmuxMouseToggle,
    accessTokenExists,
    onGenerateAccessToken,
    onRevokeAccessToken,
}: SystemSettingsProps) {
    const [activeCategory, setActiveCategory] = useState<SettingsCategory>('general');
    const [confirmReset, setConfirmReset] = useState(false);

    const t = (zh: string, en: string) => (language === 'zh-CN' ? zh : en);

    const handleResetCache = () => {
        if (!confirmReset) {
            setConfirmReset(true);
            return;
        }
        try {
            localStorage.clear();
        } catch (_) {
            /* ignore */
        }
        window.location.reload();
    };

    const renderGeneral = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('通用设置', 'General Settings')}</div>
            <div class="sys-settings-section-desc">
                {t('配置语言和语音识别选项。', 'Configure language and dictation options.')}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                            <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                            <line x1="12" y1="19" x2="12" y2="23" />
                            <line x1="8" y1="23" x2="16" y2="23" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('语音识别语言', 'Voice Dictation Language')}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('选择语音输入使用的语言', 'Choose the language for voice input')}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        {t('中文 (Chinese)', 'Chinese (中文)')}
                    </button>
                    <button
                        class={`sys-settings-option-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        {t('英语 (English)', 'English')}
                    </button>
                </div>
            </div>
        </div>
    );

    const renderAppearance = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('外观与终端', 'Appearance & Terminal')}</div>
            <div class="sys-settings-section-desc">
                {t('自定义主题、颜色和终端行为。', 'Customize theme, color, and terminal behavior.')}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="4" />
                            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('色彩主题', 'Color Theme')}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('切换浅色或深色外观模式', 'Switch between light and dark appearance modes')}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-theme-grid">
                    <button
                        class={`sys-settings-theme-card ${theme === 'light' ? 'active' : ''}`}
                        onClick={() => toggleTheme('light')}
                    >
                        <div class="sys-settings-theme-preview light-preview">
                            <div class="preview-bar" />
                            <div class="preview-content">
                                <div class="preview-line" style="width:70%" />
                                <div class="preview-line" style="width:50%" />
                            </div>
                        </div>
                        <div class="sys-settings-theme-label">
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="4" />
                                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                            </svg>
                            {t('浅色模式', 'Light Mode')}
                        </div>
                        {theme === 'light' && <div class="sys-settings-theme-check">✓</div>}
                    </button>
                    <button
                        class={`sys-settings-theme-card ${theme === 'dark' ? 'active' : ''}`}
                        onClick={() => toggleTheme('dark')}
                    >
                        <div class="sys-settings-theme-preview dark-preview">
                            <div class="preview-bar" />
                            <div class="preview-content">
                                <div class="preview-line" style="width:70%" />
                                <div class="preview-line" style="width:50%" />
                            </div>
                        </div>
                        <div class="sys-settings-theme-label">
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                            </svg>
                            {t('深色模式', 'Dark Mode')}
                        </div>
                        {theme === 'dark' && <div class="sys-settings-theme-check">✓</div>}
                    </button>
                </div>
            </div>

            {onTmuxMouseToggle && (
                <div class="sys-settings-card">
                    <div class="sys-settings-card-header">
                        <div class="sys-settings-card-icon">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect x="5" y="2" width="14" height="20" rx="7" />
                                <path d="M12 2v6" />
                                <path d="M5 10h14" />
                            </svg>
                        </div>
                        <div>
                            <div class="sys-settings-card-title">{t('终端鼠标行为', 'Terminal Mouse Behavior')}</div>
                            <div class="sys-settings-card-subtitle">
                                {tmuxMouseOn
                                    ? t(
                                          '当前：滚轮滑动模式（鼠标滚轮滚动内容）',
                                          'Current: Scroll mode (mouse wheel scrolls content)'
                                      )
                                    : t(
                                          '当前：选择复制模式（鼠标选择可复制文本）',
                                          'Current: Selection mode (mouse selects & copies text)'
                                      )}
                            </div>
                        </div>
                    </div>
                    <div class="sys-settings-toggle-group">
                        <button
                            class={`sys-settings-option-btn ${tmuxMouseOn ? 'active' : ''}`}
                            onClick={() => !tmuxMouseOn && onTmuxMouseToggle()}
                        >
                            <svg
                                width="13"
                                height="13"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect x="5" y="2" width="14" height="20" rx="7" />
                                <path d="M12 2v6" />
                                <path d="M5 10h14" />
                            </svg>
                            {t('滚轮滑动', 'Scroll Mode')}
                        </button>
                        <button
                            class={`sys-settings-option-btn ${!tmuxMouseOn ? 'active' : ''}`}
                            onClick={() => tmuxMouseOn && onTmuxMouseToggle()}
                        >
                            <svg
                                width="13"
                                height="13"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="4 7 4 4 20 4 20 7" />
                                <line x1="9" y1="20" x2="15" y2="20" />
                                <line x1="12" y1="4" x2="12" y2="20" />
                            </svg>
                            {t('选择复制', 'Selection Mode')}
                        </button>
                    </div>
                </div>
            )}
        </div>
    );

    const renderSecurity = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('安全设置', 'Security Settings')}</div>
            <div class="sys-settings-section-desc">
                {t('管理访问令牌，保护您的远程访问安全。', 'Manage access tokens to secure remote access.')}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('访问令牌', 'Access Token')}</div>
                        <div class="sys-settings-card-subtitle">
                            {accessTokenExists
                                ? t(
                                      '已设置访问令牌。非本地网络访问需要提供此令牌验证。',
                                      'Access token is set. Non-local access requires this token for verification.'
                                  )
                                : t(
                                      '未设置访问令牌。生成后，非本地访问将需要令牌验证。',
                                      'No access token set. After generation, non-local access will require token verification.'
                                  )}
                        </div>
                    </div>
                </div>

                <div class="sys-settings-token-status">
                    <div class={`sys-settings-token-badge ${accessTokenExists ? 'active' : 'inactive'}`}>
                        {accessTokenExists ? (
                            <Fragment>
                                <svg
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                                {t('已启用令牌保护', 'Token Protection Active')}
                            </Fragment>
                        ) : (
                            <Fragment>
                                <svg
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="10" />
                                    <line x1="12" y1="8" x2="12" y2="12" />
                                    <line x1="12" y1="16" x2="12.01" y2="16" />
                                </svg>
                                {t('未设置令牌（本地访问不受影响）', 'No token set (local access unaffected)')}
                            </Fragment>
                        )}
                    </div>
                </div>

                <div class="sys-settings-action-row">
                    {accessTokenExists ? (
                        <button class="sys-settings-btn danger" onClick={onRevokeAccessToken}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="3 6 5 6 21 6" />
                                <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
                                <path d="M10 11v6M14 11v6" />
                                <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
                            </svg>
                            {t('撤销令牌 (Revoke)', 'Revoke Token')}
                        </button>
                    ) : (
                        <button class="sys-settings-btn primary" onClick={onGenerateAccessToken}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M5 12h14M12 5v14" />
                            </svg>
                            {t('生成访问令牌 (Generate)', 'Generate Access Token')}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );

    const renderFeedback = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('反馈与联系', 'Feedback & Contact')}</div>
            <div class="sys-settings-section-desc">
                {t(
                    '如有问题、建议或合作意向，欢迎通过以下方式联系我们。',
                    'For questions, suggestions, or collaboration inquiries, please reach out via the channels below.'
                )}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                            <polyline points="9 22 9 12 15 12 15 22" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('公司', 'Company')}</div>
                        <div class="sys-settings-card-subtitle">杭州一芥智能有限公司</div>
                    </div>
                </div>
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                            <polyline points="22,6 12,13 2,6" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('联系邮箱', 'Email')}</div>
                        <div class="sys-settings-card-subtitle">
                            <a
                                href="mailto:xiaofengzeng93@outlook.com"
                                class="meta-link"
                                style="word-break: break-all;"
                            >
                                xiaofengzeng93@outlook.com
                            </a>
                        </div>
                    </div>
                </div>
            </div>
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('用户反馈表', 'Feedback Form')}</div>
                        <div class="sys-settings-card-subtitle">
                            {t(
                                '提交功能建议或问题反馈，帮助我们改进产品。',
                                'Submit feature requests or bug reports to help us improve.'
                            )}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-action-row">
                    <a
                        class="sys-settings-btn primary"
                        href="https://my.feishu.cn/share/base/form/shrcn0OGqn5ZBCiPEpmJuJ3Djtc"
                        target="_blank"
                        rel="noopener noreferrer"
                        style="text-decoration: none; display: inline-flex; align-items: center; gap: 6px;"
                    >
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                            <polyline points="15 3 21 3 21 9" />
                            <line x1="10" y1="14" x2="21" y2="3" />
                        </svg>
                        {t('打开反馈表', 'Open Feedback Form')}
                    </a>
                </div>
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="10" />
                            <line x1="2" y1="12" x2="22" y2="12" />
                            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('界面语言', 'Interface Language')}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('选择应用的显示语言', 'Choose the display language of the app')}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        🇨🇳 中文 (Chinese)
                    </button>
                    <button
                        class={`sys-settings-option-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        🇺🇸 English
                    </button>
                </div>
            </div>
        </div>
    );

    const renderAbout = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('关于与维护', 'About & Maintenance')}</div>
            <div class="sys-settings-section-desc">
                {t('查看应用信息，管理本地缓存。', 'View app information and manage local cache.')}
            </div>

            <div class="sys-settings-card sys-settings-about-card">
                <div class="sys-settings-about-brand">
                    <img class="sys-settings-about-logo" src="/logo.png" alt="1agents logo" />
                    <div class="sys-settings-about-info">
                        <div class="sys-settings-about-name">1agents</div>
                        <div class="sys-settings-about-tagline">
                            {t('智能 AI 工作空间管理器', 'Intelligent AI Workspace Manager')}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-about-meta">
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('版本', 'Version')}</span>
                        <span class="meta-value">1.0.0</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('平台', 'Platform')}</span>
                        <span class="meta-value">Web / Desktop</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('项目', 'Project')}</span>
                        <a
                            class="meta-value meta-link"
                            href="https://github.com"
                            target="_blank"
                            rel="noopener noreferrer"
                        >
                            github.com/1agents
                        </a>
                    </div>
                </div>
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon danger-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <polyline points="1 4 1 10 7 10" />
                            <path d="M3.51 15a9 9 0 1 0 .49-3.51" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('重置应用数据', 'Reset Application Data')}</div>
                        <div class="sys-settings-card-subtitle">
                            {confirmReset
                                ? t(
                                      '⚠️ 此操作将清除所有本地设置、工作空间记录和缓存，不可恢复。',
                                      '⚠️ This will clear all local settings, workspace records, and cache. This cannot be undone.'
                                  )
                                : t(
                                      '清除所有本地缓存数据并重置应用到初始状态。',
                                      'Clear all local cached data and reset the app to its initial state.'
                                  )}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-action-row">
                    {confirmReset ? (
                        <Fragment>
                            <button class="sys-settings-btn danger" onClick={handleResetCache}>
                                <svg
                                    width="14"
                                    height="14"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                                {t('确认重置', 'Confirm Reset')}
                            </button>
                            <button class="sys-settings-btn ghost" onClick={() => setConfirmReset(false)}>
                                {t('取消', 'Cancel')}
                            </button>
                        </Fragment>
                    ) : (
                        <button class="sys-settings-btn warning" onClick={handleResetCache}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="1 4 1 10 7 10" />
                                <path d="M3.51 15a9 9 0 1 0 .49-3.51" />
                            </svg>
                            {t('重置应用数据', 'Reset App Data')}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );

    const renderContent = () => {
        switch (activeCategory) {
            case 'general':
                return renderGeneral();
            case 'appearance':
                return renderAppearance();
            case 'security':
                return renderSecurity();
            case 'feedback':
                return renderFeedback();
            case 'about':
                return renderAbout();
            default:
                return null;
        }
    };

    return (
        <div class="sys-settings-page">
            {/* Left nav column */}
            <nav class="sys-settings-nav">
                <div class="sys-settings-nav-title">{t('系统设置', 'System Settings')}</div>
                {NAV_ITEMS.map(item => (
                    <button
                        key={item.key}
                        class={`sys-settings-nav-item ${activeCategory === item.key ? 'active' : ''}`}
                        onClick={() => setActiveCategory(item.key)}
                    >
                        <span class="sys-settings-nav-icon">{item.icon}</span>
                        <span class="sys-settings-nav-label">{language === 'zh-CN' ? item.labelZh : item.labelEn}</span>
                    </button>
                ))}
            </nav>

            {/* Right content area */}
            <div class="sys-settings-content">{renderContent()}</div>
        </div>
    );
}
