import { h } from 'preact';

interface ThemeSettingsProps {
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
}

export function ThemeSettings({ theme, toggleTheme }: ThemeSettingsProps) {
    return (
        <div class="settings-container">
            <div class="setting-group">
                <span class="setting-label">色彩主题样式 (Color Theme)</span>
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
                        <span>浅色模式</span>
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
                        <span>深色模式</span>
                    </button>
                </div>
            </div>
        </div>
    );
}
