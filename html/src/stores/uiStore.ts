import { signal } from '@preact/signals';

import { t, type Lang } from '../i18n';

/**
 * UI / chrome state shared across the whole app (theme, language, layout
 * dimensions, toast). Lives outside the component tree so any component —
 * class or function — can read a signal's `.value` during render and
 * subscribe to fine-grained updates, instead of threading props from App.
 */

const initialLang = (localStorage.getItem('1agents-language') || 'zh-CN') as Lang;
const initialTheme = (localStorage.getItem('1agents-theme') as 'light' | 'dark' | null) || 'light';

export const theme = signal<'light' | 'dark'>(initialTheme);
export const language = signal<Lang>(initialLang);
export const hostname = signal(window.location.hostname || 'localhost');
export const leftSidebarOpen = signal(window.innerWidth > 768);
export const leftSidebarWidth = signal(260);
export const rightPanelWidth = signal(320);
export const bottomNavHidden = signal(false);
export const toastMsg = signal('');
export const isMobile = signal(window.innerWidth <= 768);
export const keyboardVisible = signal(false);
export const viewportHeight = signal(window.visualViewport ? window.visualViewport.height : window.innerHeight);

// Reflect the persisted theme on the root element immediately at startup
// (previously done in App.componentDidMount).
document.documentElement.setAttribute('data-theme', initialTheme);

let toastTimer: ReturnType<typeof setTimeout> | null = null;

export const showToast = (msg: string) => {
    toastMsg.value = msg;
    if (toastTimer) clearTimeout(toastTimer);
    toastTimer = setTimeout(() => {
        toastMsg.value = '';
    }, 2200);
};

/** Ask the active xterm instance to refit after a layout change. */
export const triggerTerminalFit = () => {
    setTimeout(() => {
        const term = (window as unknown as { term?: { fit?: () => void } }).term;
        if (term && term.fit) {
            term.fit();
        }
    }, 150);
};

/** Broadcast a message to the module iframes (cc-connect / providers / skills). */
const postToModuleIframes = (msg: Record<string, unknown>) => {
    for (const id of ['cc-connect-iframe', 'cc-providers-iframe', 'skills-iframe']) {
        const iframe = document.getElementById(id) as HTMLIFrameElement | null;
        if (iframe && iframe.contentWindow) {
            iframe.contentWindow.postMessage(msg, '*');
        }
    }
};

export const toggleTheme = (themeMode?: 'light' | 'dark') => {
    const targetTheme = themeMode || (theme.value === 'light' ? 'dark' : 'light');
    theme.value = targetTheme;
    postToModuleIframes({ type: 'THEME_CHANGE', theme: targetTheme });
    document.documentElement.setAttribute('data-theme', targetTheme);
    localStorage.setItem('1agents-theme', targetTheme);
    triggerTerminalFit();
};

export const toggleLanguage = (lang: Lang) => {
    language.value = lang;
    postToModuleIframes({ type: 'LANG_CHANGE', lang });
    localStorage.setItem('1agents-language', lang);
    const langName = t(lang === 'zh-CN' ? 'app.langName.zh' : 'app.langName.en', lang);
    showToast(t('app.toast.langChanged', lang, { lang: langName }));
};

export const toggleLeftSidebar = () => {
    const opening = !leftSidebarOpen.value;
    if (opening && leftSidebarWidth.value <= 40) {
        leftSidebarWidth.value = 260;
    }
    leftSidebarOpen.value = opening;
    triggerTerminalFit();
};
