/**
 * Module registry — single source of truth for which modules are embedded
 * into the main app and how to embed them.
 *
 * Adding a new module:
 *   1. Append a key to `MODULES` below.
 *   2. Provide a `staticManifest` (so the sidebar renders even before the
 *      module is reachable).
 *   3. Optionally set `manifestUrl` to enable live count refresh.
 *
 * The host's `LeftSidebar` always reads through `getModuleByTab()` so the
 * registry is the only place that knows about specific modules.
 */

import type { RightDrawerTab } from '../components/types';
import type { ModuleId, ModuleManifest } from './module-types';
import { SETTINGS_MODULE_ID, SETTINGS_STATIC_MANIFEST, SETTINGS_ENTRY_PATH } from './settings-manifest';

export interface ModuleRegistration {
    moduleId: ModuleId;
    /** Which main-app drawer tab owns this module. */
    ownerTab: RightDrawerTab;
    /**
     * Base URL of the module (e.g. "/1skills/").
     *
     * @deprecated Skills is now loaded as a custom element
     * (`<skills-panel>`) — `iframeBase` is only read by
     * `buildModuleIframeSrc()` which is itself dead code kept
     * as a fallback. The embed entry is served as an ESM module
     * at `/api/embed/skills-embed.js`.
     */
    iframeBase: string;
    /**
     * Custom element tag name for the embed mode. When set the host
     * renders this tag instead of an `<iframe>` — the element is
     * defined by the submodule's embed entry (e.g. `embed.tsx`).
     *
     * If empty the module still uses the legacy iframe path.
     */
    embedElement?: string;
    /** … (unchanged) */
    staticManifest: ModuleManifest;
    /** Optional endpoint that returns a live manifest with counts. */
    manifestUrl?: string;
    /** Initial entry path the iframe/element is loaded at. */
    entryPath: string;
}

/**
 * 1skills static manifest — mirrors the structure produced by
 * `useSidebarModel()` in 1skills' `app/capability-registry/sidebar.ts`.
 * Counts are null because we don't have them on the host; the live
 * manifest endpoint fills them in.
 */
const SKILLS_STATIC_MANIFEST: ModuleManifest = {
    moduleId: 'skills',
    version: 1,
    entryPath: '/overview',
    topLinks: [{ key: 'overview', to: '/overview', label: 'module.skills.nav.overview', iconKey: 'overview' }],
    groups: [
        {
            key: 'skills',
            label: 'module.skills.group.skills',
            iconKey: 'skills',
            links: [
                { key: 'skills-use', to: '/skills/use', label: 'module.skills.link.inUse' },
                { key: 'skills-review', to: '/skills/review', label: 'module.skills.link.review', badge: 'review' },
                { key: 'skills-scan-config', to: '/scan-config', label: 'module.skills.link.scanConfig' },
            ],
        },
        {
            key: 'slash-commands',
            label: 'module.skills.group.slashCommands',
            iconKey: 'slash-commands',
            links: [
                { key: 'slash-commands-use', to: '/slash-commands/use', label: 'module.skills.link.inUse' },
                {
                    key: 'slash-commands-review',
                    to: '/slash-commands/review',
                    label: 'module.skills.link.review',
                    badge: 'review',
                },
            ],
        },
        {
            key: 'mcp',
            label: 'module.skills.group.mcp',
            iconKey: 'mcp',
            links: [
                { key: 'mcp-use', to: '/mcp/use', label: 'module.skills.link.inUse' },
                { key: 'mcp-review', to: '/mcp/review', label: 'module.skills.link.review', badge: 'review' },
            ],
        },
        {
            key: 'marketplace',
            label: 'module.skills.group.marketplace',
            iconKey: 'marketplace',
            links: [
                { key: 'marketplace-skills', to: '/marketplace', label: 'module.skills.group.skills' },
                { key: 'marketplace-mcp', to: '/marketplace/mcp', label: 'module.skills.group.mcp' },
                { key: 'marketplace-clis', to: '/marketplace/clis', label: 'module.skills.link.cli' },
            ],
        },
    ],
    headerActions: [{ key: 'refresh', label: 'module.skills.action.refresh', iconKey: 'refresh' }],
};

/**
 * The registered modules. 1skills is the only iframe-backed module today;
 * `settings` shares the same `ModuleManifest` shape so the host can render
 * its category navigation through `<ModuleNav />` in the workspace's left
 * sidebar — the content body itself is the host-rendered `SystemSettings`
 * component, not an iframe (see `DesktopAppLayout`).
 */
export const MODULES: Record<ModuleId, ModuleRegistration> = {
    skills: {
        moduleId: 'skills',
        ownerTab: 'skills',
        iframeBase: '/1skills/',
        embedElement: 'skills-panel',
        staticManifest: SKILLS_STATIC_MANIFEST,
        manifestUrl: '/1skills/api/manifest',
        entryPath: '/overview',
    },
    settings: {
        moduleId: SETTINGS_MODULE_ID,
        ownerTab: 'settings',
        // No iframe — `buildModuleIframeSrc()` is not called for settings
        // (the desktop layout renders `SystemSettings` directly). The empty
        // `iframeBase` is here only to satisfy the type contract.
        iframeBase: '',
        staticManifest: SETTINGS_STATIC_MANIFEST,
        entryPath: SETTINGS_ENTRY_PATH,
    },
};

/**
 * Returns the module registered under the given drawer tab, or null if the
 * tab doesn't host a module.
 */
export function getModuleByTab(tab: RightDrawerTab): ModuleRegistration | null {
    for (const id of Object.keys(MODULES)) {
        if (MODULES[id].ownerTab === tab) {
            return MODULES[id];
        }
    }
    return null;
}

/**
 * Builds the iframe `src` for a module. Always appends `?bare=1` so the
 * module renders without its own chrome.
 *
 * @deprecated Skills & providers are now loaded as custom elements
 * (see `embedElement` on `ModuleRegistration`). This function is kept
 * as a fallback for any module that still uses the iframe path.
 */
export function buildModuleIframeSrc(mod: ModuleRegistration, subPath?: string): string {
    const base = mod.iframeBase.endsWith('/') ? mod.iframeBase : mod.iframeBase + '/';
    const hash = subPath ? `#${subPath.startsWith('/') ? subPath : '/' + subPath}` : '';
    return `${base}?bare=1${hash}`;
}

/**
 * Deep-merges a live manifest over the static one. Field-level override:
 * the live manifest wins wherever it has its own data; missing fields fall
 * back to the static manifest.
 */
export function mergeManifests(staticManifest: ModuleManifest, live: ModuleManifest): ModuleManifest {
    if (live.moduleId !== staticManifest.moduleId) {
        return staticManifest;
    }
    return {
        ...staticManifest,
        ...live,
        topLinks: live.topLinks ?? staticManifest.topLinks,
        headerActions: live.headerActions ?? staticManifest.headerActions,
        groups: live.groups ?? staticManifest.groups,
    };
}
