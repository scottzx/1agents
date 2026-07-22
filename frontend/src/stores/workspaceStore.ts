import { signal } from '@preact/signals';

import { isFullPageTab, type WorkspaceFolder, type Workspace, type AgentType } from '../components/types';
import { workspaceService } from '../services/workspaceService';
import { setActiveDevice } from '@1agents/core/services/apiClient';
import { DEFAULT_AGENT_TYPE } from '../services/agentService';
import { t, type Lang } from '../i18n';
import * as ui from './uiStore';
import * as fs from './fsStore';
import * as sess from './sessionStore';
import * as tabsStore from './tabsStore';
import * as modal from './modalStore';

/**
 * Workspace state (workspace list/folders, active workspace, sidebar group
 * collapse, cc-connect panel urls) and its service-calling orchestration
 * (loadWorkspaces, selectWorkspace, create/update/deleteWorkspace, …).
 *
 * Note on imports: workspaceStore, sessionStore and tabsStore reference
 * each other only inside function bodies (never at module evaluation
 * time), so the import cycles between them are safe — ES-module live
 * bindings resolve the cross-store calls at call time.
 */

export const workspaces = signal<Workspace[]>([]);
export const workspacesLoading = signal(true);
// Archived projects/assistants — the overview's 已归档 board. Loaded on demand
// (not in the sidebar); kept separate from the active `workspaces` list.
export const archivedWorkspaces = signal<Workspace[]>([]);
export const folders = signal<WorkspaceFolder[]>([]);
export const activeWorkspaceId = signal(localStorage.getItem('1agents-active-workspace') || '');
/**
 * 当前激活工作空间所属的远程设备 id(#114),'' = 本机。与 activeWorkspaceId
 * 配合做切换判定:不同设备上可能存在同 id 的工作空间(如各自的 default),
 * 单看 id 会误判为"已激活"。
 */
export const activeWorkspaceDeviceId = signal('');
/**
 * One-shot injection of a workspace id into the new-chat picker's selection.
 * Set when a workspace is created from the new-chat landing so it gets
 * pre-selected without navigating away; NewChatHome consumes and clears it.
 * The real context switch stays deferred until a message is sent.
 */
export const newChatWorkspaceId = signal<string>('');
export const hasLoadedWorkspaces = signal(false);
export const onboarded = signal(localStorage.getItem('1agents-onboarded') === 'true');

// ── CC-Connect panel urls (channels / providers iframes) ──
export const ccConnectUrl = signal('');
export const ccProvidersUrl = signal('');

// ── 多设备项目视图(#114)──────────────────────────────────────────────
/**
 * 远程设备(已注册、非本机)。复用 #110/#113 的设备注册表 /api/devices,
 * 字段子集即可:分组标题(name/os/active)+ 拉取该设备项目所需的 id。
 */
export interface RemoteDevice {
    id: string;
    name: string;
    os?: string;
    active: boolean;
}
export const remoteDevices = signal<RemoteDevice[]>([]);
/** 各远程设备已展开/正在加载状态 + 拉到的项目列表(按 deviceId 索引)。 */
export const remoteExpanded = signal<Record<string, boolean>>({});
export const remoteLoading = signal<Record<string, boolean>>({});
export const remoteProjects = signal<Record<string, Workspace[]>>({});

export const toggleFolder = (folderId: string) => {
    folders.value = folders.value.map(f => (f.id === folderId ? { ...f, expanded: !f.expanded } : f));
};

/** 一键折叠:收起给定 id 范围内(助理/项目某一侧)所有展开的 project-folder。 */
export const collapseFolders = (folderIds: string[]) => {
    const idSet = new Set(folderIds);
    folders.value = folders.value.map(f => (idSet.has(f.id) && f.expanded ? { ...f, expanded: false } : f));
};

/** 拉取已注册设备列表,过滤掉本机(self),只留远程节点用于 Sidebar 分组。 */
export const loadRemoteDevices = async () => {
    try {
        const res = await fetch('/api/devices');
        if (!res.ok) throw new Error(`HTTP ${res.status}`);
        const list: Array<{ id: string; name?: string; os?: string; self?: boolean; active?: boolean }> =
            (await res.json()) ?? [];
        remoteDevices.value = list
            .filter(d => !d.self)
            .map(d => ({ id: d.id, name: d.name || d.id, os: d.os, active: Boolean(d.active) }));
    } catch (err) {
        console.error('[workspace] load remote devices error:', err);
        remoteDevices.value = [];
    }
};

/**
 * 折叠/展开某远程设备组。首次展开时经代理路由拉取该设备的项目列表(拉取式,方案 A)。
 * 离线设备直接给提示,不发请求。
 */
export const toggleRemoteDevice = async (device: RemoteDevice) => {
    const willExpand = !remoteExpanded.value[device.id];
    remoteExpanded.value = { ...remoteExpanded.value, [device.id]: willExpand };
    if (!willExpand) return;
    if (!device.active) {
        ui.showToast(t('sidebar.device.offlineHint', ui.language.value, { name: device.name }));
        return;
    }
    // 已加载过就不重复拉取
    if (remoteProjects.value[device.id]) return;
    remoteLoading.value = { ...remoteLoading.value, [device.id]: true };
    try {
        const list = await workspaceService.list(device.id);
        remoteProjects.value = { ...remoteProjects.value, [device.id]: list };
    } catch (err) {
        console.error('[workspace] load remote projects error:', err);
        ui.showToast(t('sidebar.device.loadFailed', ui.language.value, { name: device.name }));
    } finally {
        remoteLoading.value = { ...remoteLoading.value, [device.id]: false };
    }
};

/** Fetch all workspaces from GET /api/workspace/list */
/** Load the archived projects/assistants board (overview 已归档 cards). */
export const loadArchivedWorkspaces = async () => {
    try {
        archivedWorkspaces.value = await workspaceService.listArchived();
    } catch (err) {
        console.error('[workspace] load archived error:', err);
        archivedWorkspaces.value = [];
    }
    return archivedWorkspaces.value;
};

/** Resolve a workspace by id from the active list OR the archived board. Detail
 *  pages use this so an archived project's detail still renders (it's absent
 *  from the active `workspaces` list). */
export const findWorkspaceAnyStatus = (id: string): Workspace | undefined =>
    workspaces.value.find(w => w.id === id) ?? archivedWorkspaces.value.find(w => w.id === id);

/** Archive a project/assistant, then refresh both boards. */
export const archiveWorkspace = async (id: string) => {
    await workspaceService.archive(id);
    await Promise.all([loadWorkspaces(true), loadArchivedWorkspaces()]);
};

/** Reopen an archived project/assistant, then refresh both boards. */
export const reopenWorkspace = async (id: string) => {
    await workspaceService.reopen(id);
    await Promise.all([loadWorkspaces(true), loadArchivedWorkspaces()]);
};

export const loadWorkspaces = async (skipAutoSelect = false) => {
    workspacesLoading.value = true;
    try {
        const list = await workspaceService.list();
        // Preserve existing expand state by merging
        const existing = folders.value;
        folders.value = list.map(ws => {
            const prev = existing.find(f => f.id === ws.id);
            return {
                id: ws.id,
                name: ws.name,
                expanded: prev ? prev.expanded : ws.id === 'default',
                sessions: prev ? prev.sessions : [],
            };
        });
        workspaces.value = list;
        workspacesLoading.value = false;
        hasLoadedWorkspaces.value = true;
        if (!skipAutoSelect) {
            const activeId = activeWorkspaceId.value;
            const activeStillExists = list.some(ws => ws.id === activeId);
            if (!activeId || !activeStillExists) {
                // Active workspace was deleted or never set — switch to first available
                if (list.length > 0) {
                    selectWorkspace(list[0]);
                } else {
                    // No workspaces left — clear stale state
                    activeWorkspaceId.value = '';
                    ccConnectUrl.value = '';
                    ccProvidersUrl.value = '';
                }
            } else {
                loadCcConnectUrl();
                loadCcProvidersUrl();
            }
        }
        return list;
    } catch (err) {
        console.error('[workspace] load error:', err);
        workspacesLoading.value = false;
        hasLoadedWorkspaces.value = true;
        return [];
    }
};

export const loadCcConnectUrl = async (workspaceId?: string) => {
    const wsId = workspaceId || activeWorkspaceId.value;
    if (!wsId) return;
    try {
        const url = await workspaceService.getCcConnectUrl(wsId, ui.theme.value, ui.language.value || 'zh-CN');
        ccConnectUrl.value = url;
    } catch (err) {
        console.error('[ccconnect] failed to load url:', err);
    }
};

export const loadCcProvidersUrl = async (workspaceId?: string) => {
    const wsId = workspaceId || activeWorkspaceId.value;
    if (!wsId) return;
    try {
        const url = await workspaceService.getCcConnectUrl(
            wsId,
            ui.theme.value,
            ui.language.value || 'zh-CN',
            '/providers'
        );
        ccProvidersUrl.value = url;
    } catch (err) {
        console.error('[ccconnect] failed to load providers url:', err);
    }
};

export const updateCcConnectUrlParams = (theme: 'light' | 'dark', lang: Lang) => {
    const urlStr = ccConnectUrl.value;
    if (!urlStr) return;
    try {
        const dummyBase = 'http://dummy.com';
        const parsed = new URL(urlStr, dummyBase);
        parsed.searchParams.set('theme', theme);

        // Map BCP-47 to CC-Connect codes
        let normalLang = 'zh';
        const langLower = (lang || '').toLowerCase();
        if (langLower.startsWith('en')) {
            normalLang = 'en';
        } else if (langLower.startsWith('zh-tw') || langLower.startsWith('zh-hk')) {
            normalLang = 'zh-TW';
        } else if (langLower.startsWith('ja')) {
            normalLang = 'ja';
        } else if (langLower.startsWith('es')) {
            normalLang = 'es';
        }
        parsed.searchParams.set('lang', normalLang);

        let newUrl = parsed.pathname + parsed.search;
        if (!urlStr.startsWith('/')) {
            newUrl = parsed.toString();
        }
        ccConnectUrl.value = newUrl;
    } catch (e) {
        console.error('[ccconnect] failed to update url params:', e);
    }
};

export const onUseTempWorkspace = async () => {
    try {
        await workspaceService.create({
            id: 'temp',
            name: 'temp',
            path: 'temp',
            status: 'active',
        });
        localStorage.setItem('1agents-onboarded', 'true');
        onboarded.value = true;
        const list = await loadWorkspaces(true);
        const tempWs = list.find(w => w.id === 'temp');
        if (tempWs) {
            await selectWorkspace(tempWs);
        }
    } catch (err) {
        console.error('[workspace] failed to create temp workspace:', err);
        ui.showToast(t('app.toast.tempCreateFailed', ui.language.value, { err: String(err) }));
    }
};

/** Create a new workspace via POST /api/workspace/create */
export const createWorkspace = async (
    name: string,
    path: string,
    terminalDir?: string,
    chatChannel?: string,
    defaultAgent?: AgentType
) => {
    let id = name
        .toLowerCase()
        .replace(/\s+/g, '-')
        .replace(/[^a-z0-9-]/g, '');
    if (!id) {
        // Fallback for non-ASCII/Chinese names: generate a clean unique ID
        id = 'ws-' + Math.random().toString(36).substring(2, 10);
    }
    let finalId = id;
    let counter = 1;
    const existingIds = new Set(workspaces.value.map(w => w.id));
    while (existingIds.has(finalId)) {
        finalId = `${id}-${counter}`;
        counter++;
    }
    const ws: Workspace = {
        id: finalId,
        name,
        path,
        status: 'active',
        terminalDir: terminalDir?.trim() || undefined,
        chatChannel: chatChannel?.trim() || undefined,
        defaultAgent: defaultAgent || DEFAULT_AGENT_TYPE,
    };
    try {
        await workspaceService.create(ws);
        localStorage.setItem('1agents-onboarded', 'true');
        onboarded.value = true;
        const list = await loadWorkspaces(true);
        const newWs = list.find(w => w.id === ws.id) || (list.length > 0 ? list[0] : undefined);
        if (newWs) {
            if (tabsStore.activeTab.value === 'new_chat') {
                // Created from the new-chat picker's "Open folder…": stay on the
                // landing and only pre-select; the real switch is deferred to send.
                newChatWorkspaceId.value = newWs.id;
            } else {
                await selectWorkspace(newWs);
            }
        }
        ui.showToast(t('app.toast.workspaceCreated', ui.language.value, { name }));
    } catch (err) {
        ui.showToast(t('app.toast.workspaceCreateFailed', ui.language.value, { err: String(err) }));
    }
};

/**
 * Create an assistant — the "对话" concept, extended to N. Sends name + kind +
 * optional avatar + selected skill refs so the backend mints a badge folder
 * under ~/.1agents/projects/<YYYYMMDD-NNNN>/ and weak-copies skills into
 * <ws>/.claude/skills (#360). No client-side directory pick.
 *
 * Returns true on success. A 409 (duplicate name) surfaces the backend message
 * as a toast; the modal stays open by not throwing further.
 */
export const createAssistant = async (
    name: string,
    skills: string[],
    avatar?: string,
    soul?: string
): Promise<boolean> => {
    try {
        await workspaceService.create({ name, status: 'active', kind: 'workforce', avatar }, skills, soul);
        localStorage.setItem('1agents-onboarded', 'true');
        onboarded.value = true;
        const list = await loadWorkspaces(true);
        const newWs = [...list].reverse().find(w => w.name === name);
        if (newWs) await selectWorkspace(newWs);
        ui.showToast(t('app.toast.workspaceCreated', ui.language.value, { name }));
        return true;
    } catch (err) {
        // The backend 409 body is JSON: {error:"name_taken", message:"..."}.
        // Surface the message verbatim so the user knows to rename.
        const raw = String(err);
        let msg = raw;
        try {
            const parsed = JSON.parse(raw.replace(/^Error: /, '')) as { message?: string };
            if (parsed?.message) msg = parsed.message;
        } catch {
            /* fall through */
        }
        ui.showToast(msg);
        return false;
    }
};

/** Update an existing workspace via POST /api/workspace/update */
export const updateWorkspace = async (ws: Workspace) => {
    try {
        await workspaceService.update(ws);
        await loadWorkspaces();
        ui.showToast(t('app.toast.workspaceUpdated', ui.language.value));
    } catch (err) {
        ui.showToast(t('app.toast.workspaceUpdateFailed', ui.language.value, { err: String(err) }));
    }
};

/** Delete a workspace via DELETE /api/workspace/delete?id=xxx */
export const deleteWorkspace = async (id: string) => {
    const target = workspaces.value.find(w => w.id === id);
    if (id === 'default' || target?.builtin) {
        ui.showToast(t('app.toast.workspaceDeleteBuiltin', ui.language.value));
        return;
    }
    if (workspaces.value.length <= 1) {
        ui.showToast(t('app.toast.workspaceDeleteLast', ui.language.value));
        return;
    }
    try {
        // If we're deleting the currently active workspace, clear it first so
        // loadWorkspaces knows to auto-select a new one instead of re-fetching
        // the CC-Connect URL for a workspace that no longer exists.
        if (activeWorkspaceId.value === id) {
            activeWorkspaceId.value = '';
            ccConnectUrl.value = '';
        }
        await workspaceService.delete(id);
        await sess.loadTerminals();
        await loadWorkspaces();
        ui.showToast(t('app.toast.workspaceDeleted', ui.language.value));
    } catch (err) {
        ui.showToast(t('app.toast.workspaceDeleteFailed', ui.language.value, { err: String(err) }));
    }
};

/** Reorder workspaces on drag and drop */
export const reorderFolders = async (draggedId: string, targetId: string, position: 'before' | 'after') => {
    const prevWorkspaces = workspaces.value;
    const prevFolders = folders.value;

    const draggedIdx = prevWorkspaces.findIndex(w => w.id === draggedId);
    const targetIdx = prevWorkspaces.findIndex(w => w.id === targetId);

    if (draggedIdx === -1 || targetIdx === -1 || draggedIdx === targetIdx) return;

    const newWorkspaces = [...prevWorkspaces];
    const [draggedItem] = newWorkspaces.splice(draggedIdx, 1);

    let newTargetIdx = newWorkspaces.findIndex(w => w.id === targetId);
    if (position === 'after') {
        newTargetIdx += 1;
    }

    newWorkspaces.splice(newTargetIdx, 0, draggedItem);

    const newFolders = newWorkspaces.map(ws => {
        const f = prevFolders.find(folder => folder.id === ws.id);
        return f || { id: ws.id, name: ws.name, expanded: false, sessions: [] };
    });

    // Optimistic UI update
    workspaces.value = newWorkspaces;
    folders.value = newFolders;

    try {
        await workspaceService.reorder(newWorkspaces.map(w => w.id));
    } catch (err) {
        console.error('[workspace] reorder error:', err);
        // Rollback on error
        workspaces.value = prevWorkspaces;
        folders.value = prevFolders;
        ui.showToast(t('app.toast.workspaceReorderFailed', ui.language.value, { err: String(err) }));
    }
};

/** Switch active workspace and cd into it in a matching tmux window */
export const selectWorkspace = async (ws: Workspace) => {
    if (isFullPageTab(tabsStore.activeDrawerTab.value)) {
        tabsStore.activeDrawerTab.value = 'none';
    }
    // Always land on the 任务 view — even when this workspace is already
    // active (e.g. clicking 任务 while a terminal/chat in the same workspace
    // is showing). No terminal is auto-created or switched here.
    tabsStore.activeTabId.value = 'tasks';

    // 多设备(#114):切换 API 路由目标。远程项目 → 经 #111 代理路由;本机 → 直连。
    const targetDeviceId = ws.deviceId ?? '';
    setActiveDevice(targetDeviceId || null);

    const activeId = activeWorkspaceId.value;
    if (ws.id === activeId && activeWorkspaceDeviceId.value === targetDeviceId) return;
    activeWorkspaceDeviceId.value = targetDeviceId;

    const prevWsId = activeId;
    activeWorkspaceId.value = ws.id;
    loadCcConnectUrl(ws.id);
    loadCcProvidersUrl(ws.id);
    sess.loadChatSessions(ws.id);
    localStorage.setItem('1agents-active-workspace', ws.id);

    // Per-project built-in browser: restore this workspace's URL; unload others.
    tabsStore.onWorkspaceBrowserSwitch(prevWsId, ws.id);

    // Switch backend context (fs + git roots) and reload file browser
    await fs.switchFsContext(ws);
    ui.showToast(t('app.toast.workspaceSwitched', ui.language.value, { name: ws.name }));
};

/** Submit handler for the workspace create/rename modal. */
export const submitWsModal = async () => {
    const wsModalMode = modal.wsModalMode.value;
    const wsModalTarget = modal.wsModalTarget.value;
    const wsModalName = modal.wsModalName.value;
    const wsModalPath = modal.wsModalPath.value;
    const wsModalTerminalDir = modal.wsModalTerminalDir.value;
    const wsModalChatChannel = modal.wsModalChatChannel.value;
    const wsModalDefaultAgent = modal.wsModalDefaultAgent.value;
    if (!wsModalName.trim()) return;
    modal.closeWsModal();
    if (wsModalMode === 'create') {
        await createWorkspace(
            wsModalName.trim(),
            wsModalPath.trim(),
            wsModalTerminalDir.trim(),
            wsModalChatChannel.trim(),
            wsModalDefaultAgent
        );
    } else if (wsModalMode === 'rename' && wsModalTarget) {
        await updateWorkspace({
            ...wsModalTarget,
            name: wsModalName.trim(),
            path: wsModalPath.trim(),
            terminalDir: wsModalTerminalDir.trim() || undefined,
            chatChannel: wsModalChatChannel.trim() || undefined,
            defaultAgent: wsModalDefaultAgent,
        });
    }
};
