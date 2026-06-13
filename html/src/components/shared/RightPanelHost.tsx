import { h } from 'preact';
import { RightPanel } from '../drawer/RightPanel';
import type { Session } from '../types';
import type { App, AppState } from '../app';
import * as fs from '../../stores/fsStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabsStore from '../../stores/tabsStore';

interface RightPanelHostProps {
    app: App;
    state: AppState;
    /** Desktop: the globally active workspace; mobile: the locally selected one. */
    activeWorkspaceId: string;
    activeWorkspacePath: string;
    /** Desktop: the resizable drawer width; mobile: full window width. */
    rightPanelWidth: number;
    /** Platform-specific fullscreen/preview behavior for the selected file. */
    onToggleFullscreen: () => void;
    onOpenPreview?: (path: string, name: string) => void;
    onSelectSession?: (session: Session) => void;
    /** Extra platform-specific refresh work run after the shared file reload. */
    onExtraRefresh?: () => Promise<void>;
}

/**
 * Shared RightPanel wiring used by both DesktopAppLayout and
 * MobileAppLayout. The drawer close/share/access-token plumbing and the
 * file-list refresh are identical on both platforms; the workspace id,
 * panel width and fullscreen behavior diverge and are passed in.
 */
export function RightPanelHost({
    app,
    state,
    activeWorkspaceId,
    activeWorkspacePath,
    rightPanelWidth,
    onToggleFullscreen,
    onOpenPreview,
    onSelectSession,
    onExtraRefresh,
}: RightPanelHostProps) {
    return (
        <RightPanel
            activeDrawerTab={tabsStore.activeDrawerTab.value}
            activeWorkspaceId={activeWorkspaceId}
            activeWorkspacePath={activeWorkspacePath}
            rightPanelWidth={rightPanelWidth}
            closeDrawer={() => (tabsStore.activeDrawerTab.value = 'none')}
            ccConnectUrl={wsStore.ccConnectUrl.value}
            onRefreshFlatFiles={async () => {
                fs.loadDir('', null);
                const isSearching = fs.searchQuery.value !== '' || fs.selectedFilterTag.value !== 'all';
                if (isSearching) {
                    fs.loadFlatFiles();
                }
                if (onExtraRefresh) {
                    await onExtraRefresh();
                }
            }}
            onToggleFullscreen={onToggleFullscreen}
            onShareFile={app.shareFile}
            onOpenPreview={onOpenPreview}
            accessTokenExists={state.accessAuthRequired}
            onGenerateAccessToken={app.generateAccessToken}
            onRevokeAccessToken={app.revokeAccessToken}
            onSelectSession={onSelectSession}
        />
    );
}
