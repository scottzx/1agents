/**
 * Icon registry — maps module `iconKey` strings to inline SVG paths.
 *
 * Modules don't ship icons. They declare a small set of `iconKey` strings; the
 * host renders them via this registry. Adding a new module icon is a one-line
 * change here. This is the intentional coupling point between the host and
 * modules.
 */

export interface IconDescriptor {
    /** viewBox is always "0 0 24 24" for this registry. */
    viewBox: '0 0 24 24';
    /** Pre-rendered children (SVG path/polyline/circle nodes as raw string). */
    paths: string;
}

/**
 * All icons a module may request. Keep this list short and stable.
 * If a module needs a new icon, add it here so the host owns the visual
 * system.
 */
export const MODULE_ICONS: Record<string, IconDescriptor> = {
    overview: {
        viewBox: '0 0 24 24',
        paths: '<rect x="3" y="3" width="7" height="9"/><rect x="14" y="3" width="7" height="5"/><rect x="14" y="12" width="7" height="9"/><rect x="3" y="16" width="7" height="5"/>',
    },
    skills: {
        viewBox: '0 0 24 24',
        paths: '<path d="M4 19.5A2.5 2.5 0 0 1 6.5 17H20"/><path d="M6.5 2H20v20H6.5A2.5 2.5 0 0 1 4 19.5v-15A2.5 2.5 0 0 1 6.5 2z"/>',
    },
    'slash-commands': {
        viewBox: '0 0 24 24',
        paths: '<path d="M4 7h11M4 12h11M4 17h11"/><path d="M19 4l3 3-3 3"/><path d="M19 14l3 3-3 3"/>',
    },
    mcp: {
        viewBox: '0 0 24 24',
        paths: '<polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/>',
    },
    marketplace: {
        viewBox: '0 0 24 24',
        paths: '<path d="M3 9l1.5-4.5A2 2 0 0 1 6.4 3h11.2a2 2 0 0 1 1.9 1.5L21 9"/><path d="M3 9v10a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V9"/><path d="M3 9h18"/><path d="M9 13h6"/>',
    },
    settings: {
        viewBox: '0 0 24 24',
        paths: '<circle cx="12" cy="12" r="3"/><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"/>',
    },
    refresh: {
        viewBox: '0 0 24 24',
        paths: '<polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/>',
    },
};

/**
 * Returns SVG `children` (inner content) for the given icon key, or null if
 * the key is unknown. Caller is expected to render a default fallback when
 * this returns null.
 */
export function getModuleIconPath(key: string | undefined): string | null {
    if (!key) return null;
    return MODULE_ICONS[key]?.paths ?? null;
}
