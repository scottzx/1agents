import { signal } from '@preact/signals';

import type { WorkspaceFolder, Workspace } from '../components/types';

/**
 * Workspace state (workspace list/folders, active workspace, sidebar group
 * collapse). Previously lived on App's god-state and was threaded through
 * the layouts; now any consumer reads the signals directly. Service-calling
 * orchestration (loadWorkspaces, selectWorkspace, …) stays in App.
 */

export const workspaces = signal<Workspace[]>([]);
export const workspacesLoading = signal(true);
export const folders = signal<WorkspaceFolder[]>([]);
export const activeWorkspaceId = signal(localStorage.getItem('1agents-active-workspace') || '');
/**
 * Per-workspace collapse state for the sidebar's 聊天 / 终端 sub-page
 * groups. Owned here (not inside LeftSidebar's local state) so it
 * survives any remount of LeftSidebar and is preserved across
 * workspace switches.
 */
export const sidebarCollapsedGroups = signal<Record<string, { chat?: boolean; term?: boolean }>>({});
export const hasLoadedWorkspaces = signal(false);

export const toggleFolder = (folderId: string) => {
    folders.value = folders.value.map(f => (f.id === folderId ? { ...f, expanded: !f.expanded } : f));
};

/** Toggle a per-workspace 聊天/终端 sub-page group's collapse state. */
export const toggleSidebarGroup = (folderId: string, key: 'chat' | 'term') => {
    sidebarCollapsedGroups.value = {
        ...sidebarCollapsedGroups.value,
        [folderId]: {
            ...sidebarCollapsedGroups.value[folderId],
            [key]: !sidebarCollapsedGroups.value[folderId]?.[key],
        },
    };
};
