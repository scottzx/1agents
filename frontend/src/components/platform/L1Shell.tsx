/**
 * L1Shell (#332) — the top-level shell that hosts:
 *   - Built-in L1 sections: 助理 | 项目 | 设置
 *   - L1 page mount points from enabled apps (e.g. CRM, AI Radio)
 *   - ContentViewHost for each section's body
 *
 * The L1 shell drives the left-nav active state and injects co-pilot context
 * (MCP connectors) whenever an app's L1 page is active.
 *
 * ## Integration
 * The existing DesktopAppLayout already renders LeftSidebar + stage + right panel.
 * L1Shell is an additive layer: it reads `activeL1PageId` from appManifestStore
 * and renders app-contributed L1 pages via MountPointRenderer in ContentViewHost.
 * The existing LeftSidebar renders the built-in nav; `L1NavItems` provides the
 * app-contributed entries to append to it.
 *
 * ## Co-pilot context
 * When an L1 page becomes active, this shell reads the app manifest's connector
 * list and calls `setCopilotAppContext`. ChatPanel reads `copilotAppContext`
 * to inject MCP tools into its session context for the active app.
 */

import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { MountPointRenderer } from './MountPointRenderer';
import * as appStore from '../../stores/appManifestStore';

// ── L1 Nav Items (for LeftSidebar integration) ────────────────────────────────

export interface L1NavEntry {
    id: string;
    label: string;
    icon?: string;
    appId: string;
    /** The `view` string from the manifest, for rendering via MountPointRenderer. */
    view: string;
}

/**
 * Apps launched only from 发现中心 / 更多应用 (not permanent left-sidebar L1 nav).
 * design §6.3: Agents 圆桌 entry is 更多 → 发现中心 → 应用 → card.
 */
const DISCOVERY_ONLY_APP_IDS = new Set(['agents-roundtable']);

/**
 * Returns the list of L1 nav entries from enabled apps for the left sidebar.
 * Discovery-only apps (e.g. agents-roundtable) are excluded — they still open
 * via DiscoveryPanel → openAppById → enterL1App, but do not appear under 侧栏「应用」.
 *
 * @example
 * // In LeftSidebar.tsx, alongside the built-in nav items:
 * const appL1Entries = getL1NavEntries();
 * appL1Entries.forEach(entry => renderNavItem(entry));
 */
export function getL1NavEntries(): L1NavEntry[] {
    return appStore.l1PageMounts.value
        .filter(({ app }) => !DISCOVERY_ONLY_APP_IDS.has(app.id))
        .map(({ app, mount }) => ({
            id: mount.id,
            label: mount.label,
            icon: mount.icon,
            appId: app.id,
            view: mount.view,
        }));
}

// ── L1 App Page Renderer ──────────────────────────────────────────────────────

interface L1AppPageProps {
    /** The active L1 mount-point id (from appManifestStore.activeL1PageId). */
    mountId: string;
}

/**
 * Renders the currently active L1 app page. Drop this into the primary
 * content area (ContentViewHost or equivalent) when `activeL1PageId` is set.
 *
 * The co-pilot context is injected here so the chat panel gets the right
 * MCP tools when the user is inside an app's L1 page.
 */
export function L1AppPage({ mountId }: L1AppPageProps) {
    const mount = appStore.l1PageMounts.value.find(m => m.mount.id === mountId);

    // Inject co-pilot context for the active app when the page is mounted
    useEffect(() => {
        if (!mount) {
            appStore.clearCopilotAppContext();
            return;
        }
        appStore.setCopilotAppContext({
            appId: mount.app.id,
            namespace: mount.app.name,
            // Use all connectors declared by this app's manifest
            connectors: mount.app.mountPoints
                .filter(mp => mp.type === 'l1-page' && mp.id === mountId)
                .flatMap(() => []), // connectors come from app config in Phase 2
        });
        return () => {
            appStore.clearCopilotAppContext();
        };
    }, [mountId]);

    if (!mount) {
        return (
            <div class="l1-app-page-not-found">
                <p>应用页面未找到：{mountId}</p>
            </div>
        );
    }

    return (
        <div class="l1-app-page">
            <MountPointRenderer view={mount.mount.view} appId={mount.app.id} mountId={mount.mount.id} />
        </div>
    );
}

// ── L1 Nav Item component (for use in LeftSidebar) ───────────────────────────

interface L1NavItemProps {
    entry: L1NavEntry;
    isActive: boolean;
    onClick: () => void;
}

/**
 * A single L1 nav entry for an app-contributed page.
 * Render these in LeftSidebar below the built-in nav items.
 */
export function L1NavItem({ entry, isActive, onClick }: L1NavItemProps) {
    return (
        <button class={`l1-nav-item${isActive ? ' is-active' : ''}`} onClick={onClick} title={entry.label}>
            {entry.icon ? (
                <span class="l1-nav-item-icon" aria-hidden="true">
                    {entry.icon}
                </span>
            ) : (
                <span class="l1-nav-item-icon l1-nav-item-icon-default" aria-hidden="true">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.8"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <rect x="3" y="3" width="7" height="7" rx="1" />
                        <rect x="14" y="3" width="7" height="7" rx="1" />
                        <rect x="3" y="14" width="7" height="7" rx="1" />
                        <rect x="14" y="14" width="7" height="7" rx="1" />
                    </svg>
                </span>
            )}
            <span class="l1-nav-item-label">{entry.label}</span>
        </button>
    );
}

// ── Lens overlay ──────────────────────────────────────────────────────────────

interface LensOverlayProps {
    /** The workspace id for project-scoped lenses; undefined for home-scoped. */
    workspaceId?: string;
}

/**
 * Renders all enabled `lens` mount points as an overlay strip.
 * Lenses are cross-cutting views (e.g. cost tracker, CRM relationship view)
 * that overlay the current project or home screen non-destructively.
 *
 * Phase 1: renders registered lenses. If no lens views are registered yet,
 * nothing is shown (zero footprint).
 */
export function LensOverlay({ workspaceId }: LensOverlayProps) {
    const lenses = appStore.lensMounts.value.filter(({ mount }) =>
        workspaceId ? mount.scope === 'project' : mount.scope === 'home'
    );

    if (lenses.length === 0) return null;

    return (
        <div class="lens-overlay">
            {lenses.map(({ app, mount }) => (
                <div key={mount.id} class="lens-overlay-item">
                    <MountPointRenderer view={mount.view} appId={app.id} mountId={mount.id} workspaceId={workspaceId} />
                </div>
            ))}
        </div>
    );
}
