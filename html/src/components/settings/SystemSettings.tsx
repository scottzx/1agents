import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';
import { t, type Lang } from '../../i18n';
import type { SettingsCategory } from '../../modules/settings-manifest';

export type { SettingsCategory };

interface SystemSettingsProps {
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    language: Lang;
    toggleLanguage: (lang: Lang) => void;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
    /**
     * Active sub-category. The component is purely content — it doesn't
     * carry an internal sidebar. The host (workspace's left sidebar in
     * desktop state, the "more" menu in mobile state) renders the category
     * navigation in its own chrome and passes the active one down.
     */
    activeCategory: SettingsCategory;
}

/**
 * System settings — content view for the active sub-category.
 *
 * The category navigation lives outside this component (in the host's
 * own sidebar/header, mirroring the skill-management design). Switching
 * categories re-renders this component with a different `activeCategory`
 * prop; no internal state is needed for that.
 */
export function SystemSettings(props: SystemSettingsProps) {
    const {
        theme,
        toggleTheme,
        language,
        toggleLanguage,
        tmuxMouseOn,
        onTmuxMouseToggle,
        accessTokenExists,
        onGenerateAccessToken,
        onRevokeAccessToken,
        activeCategory,
    } = props;

    const [confirmReset, setConfirmReset] = useState(false);

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
            <div class="sys-settings-section-title">{t('settings.general.title', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.general.desc', language)}</div>

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
                        <div class="sys-settings-card-title">{t('settings.general.dictationLang', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('settings.general.dictationLangDesc', language)}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        {t('settings.general.dictationLangZh', language)}
                    </button>
                    <button
                        class={`sys-settings-option-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        {t('settings.general.dictationLangEn', language)}
                    </button>
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
                        <div class="sys-settings-card-title">{t('settings.general.uiLang', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.general.uiLangDesc', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        {t('settings.general.uiLangZh', language)}
                    </button>
                    <button
                        class={`sys-settings-option-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        {t('settings.general.uiLangEn', language)}
                    </button>
                </div>
            </div>
        </div>
    );

    const renderAppearance = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.appearance.title', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.appearance.desc', language)}</div>

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
                        <div class="sys-settings-card-title">{t('settings.appearance.colorTheme', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('settings.appearance.colorThemeDesc', language)}
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
                            {t('settings.appearance.lightMode', language)}
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
                            {t('settings.appearance.darkMode', language)}
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
                            <div class="sys-settings-card-title">{t('settings.appearance.mouse', language)}</div>
                            <div class="sys-settings-card-subtitle">
                                {tmuxMouseOn
                                    ? t('settings.appearance.mouseScroll', language)
                                    : t('settings.appearance.mouseSelect', language)}
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
                            {t('settings.appearance.scrollLabel', language)}
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
                            {t('settings.appearance.selectLabel', language)}
                        </button>
                    </div>
                </div>
            )}
        </div>
    );

    const renderSecurity = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.security.title', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.security.desc', language)}</div>

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
                        <div class="sys-settings-card-title">{t('settings.security.token', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {accessTokenExists
                                ? t('settings.security.tokenSet', language)
                                : t('settings.security.tokenUnset', language)}
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
                                {t('settings.security.active', language)}
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
                                {t('settings.security.inactive', language)}
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
                            {t('settings.security.revoke', language)}
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
                            {t('settings.security.generate', language)}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );

    const renderFeedback = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.feedback.title', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.feedback.desc', language)}</div>

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
                        <div class="sys-settings-card-title">{t('settings.feedback.company', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.feedback.companyName', language)}</div>
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
                        <div class="sys-settings-card-title">{t('settings.feedback.email', language)}</div>
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
                        <div class="sys-settings-card-title">{t('settings.feedback.form', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.feedback.formDesc', language)}</div>
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
                        {t('settings.feedback.open', language)}
                    </a>
                </div>
            </div>
        </div>
    );

    const renderAbout = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.about.title', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.about.desc', language)}</div>

            <div class="sys-settings-card sys-settings-about-card">
                <div class="sys-settings-about-brand">
                    <img class="sys-settings-about-logo" src="/logo.png" alt="1agents logo" />
                    <div class="sys-settings-about-info">
                        <div class="sys-settings-about-name">1agents</div>
                        <div class="sys-settings-about-tagline">{t('settings.about.tagline', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-about-meta">
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.version', language)}</span>
                        <span class="meta-value">1.0.0</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.platform', language)}</span>
                        <span class="meta-value">Web / Desktop</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.project', language)}</span>
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
                        <div class="sys-settings-card-title">{t('settings.about.reset', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {confirmReset
                                ? t('settings.about.resetWarning', language)
                                : t('settings.about.resetDesc', language)}
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
                                {t('settings.about.confirmReset', language)}
                            </button>
                            <button class="sys-settings-btn ghost" onClick={() => setConfirmReset(false)}>
                                {t('common.cancel', language)}
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
                            {t('settings.about.resetBtn', language)}
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
        <div class="sys-settings-page sys-settings-page--bare">
            <div class="sys-settings-content">{renderContent()}</div>
        </div>
    );
}
