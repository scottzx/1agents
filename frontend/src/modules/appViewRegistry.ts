/**
 * App View Registry — the extensibility seam between the platform layer
 * and Wave 3 business apps.
 *
 * ## How it works
 *
 * 1. The backend returns an AppManifest with `mountPoints[].view` strings,
 *    e.g. `"MediaMaterialTab"` or `"CrmL1Page"`.
 * 2. Wave 3 app bundles call `registerAppView("MediaMaterialTab", MediaMaterialTab)`
 *    at startup (e.g. in the app's `index.tsx` or a lazy import boundary).
 * 3. `<MountPointRenderer view="MediaMaterialTab" ... />` calls `resolveAppView()`
 *    and either renders the registered component or a graceful placeholder.
 *
 * ## Wave 3 integration point
 *
 * ```ts
 * // In your Wave 3 app bundle (e.g. apps/media-studio/src/index.tsx):
 * import { registerAppView } from '../../modules/appViewRegistry';
 * import { MediaMaterialTab }  from './MediaMaterialTab';
 * import { MediaStudioL1Page } from './MediaStudioL1Page';
 *
 * registerAppView('MediaMaterialTab', MediaMaterialTab);
 * registerAppView('MediaStudioL1Page', MediaStudioL1Page);
 * ```
 *
 * The `view` string must match exactly what is declared in the app's manifest.
 */

import type { ComponentType } from 'preact';

// ── Types ─────────────────────────────────────────────────────────────────────

/**
 * Props passed by the platform shell to every mounted app view.
 * Wave 3 components should accept (at minimum) these props.
 */
export interface AppViewProps {
    /** The ID of the AppManifest that owns this view. */
    appId: string;
    /** The mount-point id within the manifest. */
    mountId: string;
    /**
     * The workspace id of the currently active project.
     * Present for `project-tab` and project-scoped `lens` mounts;
     * absent (undefined) for `l1-page` and home-scoped `lens` mounts.
     */
    workspaceId?: string;
}

export type AppViewComponent = ComponentType<AppViewProps>;

// ── Registry ──────────────────────────────────────────────────────────────────

const _registry = new Map<string, AppViewComponent>();

/**
 * Register a React/Preact component under the `view` name declared in the app's
 * manifest. Safe to call multiple times with the same name (last write wins, so
 * hot-reload replaces cleanly).
 *
 * @param view  - The `view` string from the AppManifest mount point (case-sensitive).
 * @param component - The Preact component to render when that view is active.
 *
 * @example
 * registerAppView('MediaMaterialTab', MediaMaterialTab);
 */
export function registerAppView(view: string, component: AppViewComponent): void {
    _registry.set(view, component);
}

/**
 * Look up a registered view component by name.
 * Returns `null` when the view has not been registered yet (Wave 3 bundle not
 * loaded, or manifest references a view that doesn't exist in the bundle).
 */
export function resolveAppView(view: string): AppViewComponent | null {
    return _registry.get(view) ?? null;
}

/**
 * List all currently registered view names. Useful for debugging and for
 * the app settings panel to show which views are available.
 */
export function listRegisteredViews(): string[] {
    return Array.from(_registry.keys());
}
