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

export interface ModuleRegistration {
    moduleId: ModuleId;
    /** Which main-app drawer tab owns this module. */
    ownerTab: RightDrawerTab;
    /** Base URL of the module (e.g. "/1skills/"). */
    iframeBase: string;
    /**
     * Static manifest used as a fallback when the module is unreachable
     * (network down, FastAPI not running, etc.). The host's `<ModuleNav />`
     * renders this immediately and overlays the live manifest on top.
     */
    staticManifest: ModuleManifest;
    /** Optional endpoint that returns a live manifest with counts. */
    manifestUrl?: string;
    /** Initial entry path the iframe is loaded at. */
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
    topLinks: [{ key: 'overview', to: '/overview', label: '概览', iconKey: 'overview' }],
    groups: [
        {
            key: 'skills',
            label: '技能',
            iconKey: 'skills',
            links: [
                { key: 'skills-use', to: '/skills/use', label: '使用中' },
                { key: 'skills-review', to: '/skills/review', label: '待审阅', badge: 'review' },
                { key: 'skills-scan-config', to: '/scan-config', label: '扫描配置' },
            ],
        },
        {
            key: 'slash-commands',
            label: 'Slash 命令',
            iconKey: 'slash-commands',
            links: [
                { key: 'slash-commands-use', to: '/slash-commands/use', label: '使用中' },
                { key: 'slash-commands-review', to: '/slash-commands/review', label: '待审阅', badge: 'review' },
            ],
        },
        {
            key: 'mcp',
            label: 'MCP',
            iconKey: 'mcp',
            links: [
                { key: 'mcp-use', to: '/mcp/use', label: '使用中' },
                { key: 'mcp-review', to: '/mcp/review', label: '待审阅', badge: 'review' },
            ],
        },
        {
            key: 'marketplace',
            label: '市场',
            iconKey: 'marketplace',
            links: [
                { key: 'marketplace-skills', to: '/marketplace', label: '技能' },
                { key: 'marketplace-mcp', to: '/marketplace/mcp', label: 'MCP' },
                { key: 'marketplace-clis', to: '/marketplace/clis', label: 'CLI' },
            ],
        },
    ],
    headerActions: [{ key: 'refresh', label: '刷新', iconKey: 'refresh' }],
};

/**
 * The registered modules. Only 1skills is wired today; cc-connect and ttyd
 * will follow the same shape in subsequent PRs.
 */
export const MODULES: Record<ModuleId, ModuleRegistration> = {
    skills: {
        moduleId: 'skills',
        ownerTab: 'skills',
        iframeBase: '/1skills/',
        staticManifest: SKILLS_STATIC_MANIFEST,
        manifestUrl: '/1skills/api/manifest',
        entryPath: '/overview',
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
 * module renders without its own chrome. The active sub-path is delivered
 * to the iframe via a NAVIGATE postMessage from the host once it announces
 * READY, so we don't need to encode it in the URL.
 */
export function buildModuleIframeSrc(mod: ModuleRegistration): string {
    const base = mod.iframeBase.endsWith('/') ? mod.iframeBase : mod.iframeBase + '/';
    return `${base}?bare=1`;
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
