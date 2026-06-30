/**
 * MountPointRenderer — renders a single app mount point by resolving its
 * `view` string through the app view registry.
 *
 * Graceful degradation: if the view has not been registered (Wave 3 bundle not
 * loaded yet), shows a placeholder instead of an error.
 */

import { h } from 'preact';
import { resolveAppView } from '../../modules/appViewRegistry';
import type { AppViewProps } from '../../modules/appViewRegistry';

interface MountPointRendererProps extends AppViewProps {
    /** The `view` string from the manifest mount point. */
    view: string;
}

export function MountPointRenderer({ view, appId, mountId, workspaceId }: MountPointRendererProps) {
    const Component = resolveAppView(view);

    if (!Component) {
        return (
            <div class="app-view-placeholder">
                <div class="app-view-placeholder-icon">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="1.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <rect x="3" y="3" width="18" height="18" rx="2" />
                        <path d="M9 9h6M9 12h6M9 15h4" />
                    </svg>
                </div>
                <p class="app-view-placeholder-label">{view}</p>
                <p class="app-view-placeholder-hint">此视图尚未加载。等待应用包注册后自动显示。</p>
            </div>
        );
    }

    return <Component appId={appId} mountId={mountId} workspaceId={workspaceId} />;
}
