/**
 * Discovery manifest — exposes the discovery center's categories
 * (精选推荐 / 开源项目) through the same `ModuleManifest` shape used by
 * 1skills and settings. This lets the host's `LeftSidebar` render the
 * category navigation where the workspace tree normally sits — replacing
 * the old footer submenu — so discovery matches the settings sidebar idiom.
 *
 * The discovery page is a host-rendered component (no iframe). `registry.ts`
 * registers this manifest under `ownerTab: 'discovery'` so `buildModuleNav()`
 * returns it; the desktop layout renders `DiscoveryPanel` in the primary
 * pane for the active category (see `DesktopAppLayout` / `ContentViewHost`).
 */

import type { ModuleManifest } from './module-types';

export type DiscoveryCategory = 'featured' | 'opensource';

export interface DiscoveryNavItem {
    key: DiscoveryCategory;
    /** Path used in `ModuleManifest.topLinks[*].to` and the active path. */
    path: string;
    /** i18n key — the host's `t()` falls back to the key itself. */
    i18nKey: string;
}

export const DISCOVERY_CATEGORIES: DiscoveryNavItem[] = [
    { key: 'featured', path: '/featured', i18nKey: 'discovery.catFeatured' },
    { key: 'opensource', path: '/opensource', i18nKey: 'discovery.catOpensource' },
];

export const DISCOVERY_DEFAULT_CATEGORY: DiscoveryCategory = 'featured';
export const DISCOVERY_ENTRY_PATH = `/${DISCOVERY_DEFAULT_CATEGORY}`;

export const DISCOVERY_MODULE_ID = 'discovery';

/**
 * The static manifest for the discovery page. Rendered by the host via
 * `<ModuleNav />` in the workspace's left sidebar; the content area shows
 * the host-rendered `DiscoveryPanel`, scrolled to the active category.
 */
export const DISCOVERY_STATIC_MANIFEST: ModuleManifest = {
    moduleId: DISCOVERY_MODULE_ID,
    version: 1,
    entryPath: DISCOVERY_ENTRY_PATH,
    topLinks: DISCOVERY_CATEGORIES.map(c => ({
        key: `discovery-${c.key}`,
        to: c.path,
        label: c.i18nKey,
    })),
    groups: [],
};

/**
 * Map a discovery path (e.g. "/featured") back to a category. Returns the
 * default category for unknown / empty paths.
 */
export function pathToDiscoveryCategory(path: string | undefined | null): DiscoveryCategory {
    if (!path) return DISCOVERY_DEFAULT_CATEGORY;
    const seg = path.replace(/^\//, '').split('/')[0];
    const found = DISCOVERY_CATEGORIES.find(c => c.key === seg);
    return found ? found.key : DISCOVERY_DEFAULT_CATEGORY;
}

/**
 * Inverse of `pathToDiscoveryCategory`. Always returns a leading-slash path.
 */
export function discoveryCategoryToPath(cat: string): string {
    return `/${cat}`;
}
