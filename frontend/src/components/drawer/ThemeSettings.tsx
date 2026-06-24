import { h } from 'preact';
import { t, type Lang } from '../i18n';

interface ThemeSettingsProps {
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    language: Lang;
    toggleLanguage: (lang: Lang) => void;
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
}

export function ThemeSettings({
    theme,
    toggleTheme,
    language,
    toggleLanguage,
    accessTokenExists,
    onGenerateAccessToken,
    onRevokeAccessToken,
}: ThemeSettingsProps) {
    return (
        <div class="settings-container">
            <div class="setting-group">
                <span class="setting-label">{t('theme.colorTheme', language)}</span>
                <div class="theme-options">
                    <button
                        class={`theme-btn ${theme === 'light' ? 'active' : ''}`}
                        onClick={() => toggleTheme('light')}
                    >
                        <svg
                            width="12"
                            height="12"
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
                        <span>{t('theme.light', language)}</span>
                    </button>
                    <button class={`theme-btn ${theme === 'dark' ? 'active' : ''}`} onClick={() => toggleTheme('dark')}>
                        <svg
                            width="12"
                            height="12"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                        </svg>
                        <span>{t('theme.dark', language)}</span>
                    </button>
                </div>
            </div>

            <div class="setting-group" style="margin-top: 20px;">
                <span class="setting-label">{t('theme.dictationLang', language)}</span>
                <div class="theme-options">
                    <button
                        class={`theme-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        <span>{t('theme.zhBtn', language)}</span>
                    </button>
                    <button
                        class={`theme-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        <span>{t('theme.enBtn', language)}</span>
                    </button>
                </div>
            </div>

            <div class="setting-group" style="margin-top: 20px;">
                <span class="setting-label">{t('theme.accessToken', language)}</span>
                <p
                    class="setting-desc"
                    style="font-size: 11px; color: var(--text-muted); margin: 0 0 10px 0; line-height: 1.5;"
                >
                    {accessTokenExists ? t('theme.tokenSet', language) : t('theme.tokenUnset', language)}
                </p>
                <div class="theme-options">
                    {accessTokenExists ? (
                        <button class="theme-btn" onClick={onRevokeAccessToken}>
                            {t('theme.revoke', language)}
                        </button>
                    ) : (
                        <button class="theme-btn" onClick={onGenerateAccessToken}>
                            {t('theme.generate', language)}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
