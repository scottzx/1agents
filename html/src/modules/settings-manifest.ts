/**
 * Settings manifest — exposes the system settings page's sub-categories
 * (通用/外观/安全/反馈/关于) through the same `ModuleManifest` shape used
 * by 1skills. This lets the host's `LeftSidebar` (desktop) and the mobile
 * "more" menu render the category navigation in their own chrome — instead
 * of having the `SystemSettings` component carry its own internal sidebar.
 *
 * The settings page is a host-rendered React component, not an iframe.
 * `registry.ts` registers this manifest under `ownerTab: 'settings'` so
 * `buildModuleNav()` returns it; the desktop layout detects the host
 * component and renders it directly (see `DesktopAppLayout`).
 */

import type { ModuleManifest } from './module-types';

export type SettingsCategory = 'general' | 'appearance' | 'agents' | 'security' | 'feedback' | 'about' | 'credits';

export interface SettingsNavItem {
    key: SettingsCategory;
    /** Path used in `ModuleManifest.topLinks[*].to` and the active path. */
    path: string;
    /** i18n key — the host's `t()` falls back to the key itself. */
    i18nKey: string;
}

export const SETTINGS_CATEGORIES: SettingsNavItem[] = [
    { key: 'general', path: '/general', i18nKey: 'settings.nav.general' },
    { key: 'appearance', path: '/appearance', i18nKey: 'settings.nav.appearance' },
    { key: 'agents', path: '/agents', i18nKey: 'settings.nav.agents' },
    { key: 'security', path: '/security', i18nKey: 'settings.nav.security' },
    { key: 'feedback', path: '/feedback', i18nKey: 'settings.nav.feedback' },
    { key: 'about', path: '/about', i18nKey: 'settings.nav.about' },
    { key: 'credits', path: '/credits', i18nKey: 'settings.nav.credits' },
];

export const SETTINGS_DEFAULT_CATEGORY: SettingsCategory = 'general';
export const SETTINGS_ENTRY_PATH = `/${SETTINGS_DEFAULT_CATEGORY}`;

export const SETTINGS_MODULE_ID = 'settings';

/**
 * The static manifest for the settings page. Rendered by the host via
 * `<ModuleNav />` in the workspace's left sidebar; the content area
 * (right of the sidebar) shows the host-rendered `SystemSettings`
 * component for the active category.
 */
export const SETTINGS_STATIC_MANIFEST: ModuleManifest = {
    moduleId: SETTINGS_MODULE_ID,
    version: 1,
    entryPath: SETTINGS_ENTRY_PATH,
    topLinks: SETTINGS_CATEGORIES.map(c => ({
        key: `settings-${c.key}`,
        to: c.path,
        label: c.i18nKey,
    })),
    groups: [],
};

/**
 * Map a settings path (e.g. "/general") back to a category. Returns the
 * default category for unknown / empty paths.
 */
export function pathToSettingsCategory(path: string | undefined | null): SettingsCategory {
    if (!path) return SETTINGS_DEFAULT_CATEGORY;
    const seg = path.replace(/^\//, '').split('/')[0];
    const found = SETTINGS_CATEGORIES.find(c => c.key === seg);
    return found ? found.key : SETTINGS_DEFAULT_CATEGORY;
}

/**
 * Inverse of `pathToSettingsCategory`. Always returns a leading-slash path.
 */
export function settingsCategoryToPath(cat: SettingsCategory): string {
    return `/${cat}`;
}
