import type { ModuleManifest } from './module-types';

export type StudioCategory = 'record' | 'list';

export interface StudioNavItem {
    key: StudioCategory;
    path: string;
    i18nKey: string;
}

export const STUDIO_CATEGORIES: StudioNavItem[] = [
    { key: 'record', path: '/record', i18nKey: 'studio.catRecord' },
    { key: 'list', path: '/list', i18nKey: 'studio.catList' },
];

export const STUDIO_DEFAULT_CATEGORY: StudioCategory = 'record';
export const STUDIO_ENTRY_PATH = `/${STUDIO_DEFAULT_CATEGORY}`;

export const STUDIO_MODULE_ID = 'studio';

export const STUDIO_STATIC_MANIFEST: ModuleManifest = {
    moduleId: STUDIO_MODULE_ID,
    version: 1,
    entryPath: STUDIO_ENTRY_PATH,
    topLinks: STUDIO_CATEGORIES.map(c => ({
        key: `studio-${c.key}`,
        to: c.path,
        label: c.i18nKey,
    })),
    groups: [],
};

export function pathToStudioCategory(path: string | undefined | null): StudioCategory {
    if (!path) return STUDIO_DEFAULT_CATEGORY;
    const seg = path.replace(/^\//, '').split('/')[0];
    const found = STUDIO_CATEGORIES.find(c => c.key === seg);
    return found ? found.key : STUDIO_DEFAULT_CATEGORY;
}

export function studioCategoryToPath(cat: string): string {
    return `/${cat}`;
}
