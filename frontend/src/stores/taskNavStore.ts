// Task permalink navigation.
//
// Drives in-app navigation to a task from three sources that all converge
// here: the deep-link bootstrap (page loaded at /{project}/tasks/{number}),
// clicks on autolinked `#N` references in rendered Markdown, and the
// "copy link" affordance's counterpart. Switching project + opening the task
// view + selecting the task is centralized so every entry point behaves the
// same.

import { signal } from '@preact/signals';

import * as wsStore from './workspaceStore';
import * as tabsStore from './tabsStore';
import * as ui from './uiStore';
import * as fsStore from './fsStore';
import { fsService } from '../services/fsService';
import { projectItemService } from '@1agents/core/services/taskService';
import { parseTaskPermalink } from '../utils/markdown';

/**
 * A pending request to open a task, consumed by the mounted <TaskList> for the
 * matching workspace. A signal (not a direct call) bridges the async gap: the
 * project switch + view open may mount TaskList a tick later, and its effect
 * picks the request up whenever it lands.
 */
export const pendingTaskNav = signal<{ workspaceId: string; taskId: string } | null>(null);

/**
 * Header breadcrumb bridge. A full-page module can publish its own breadcrumb
 * trail (including the root level) to the global WorkspaceHeader, overriding the
 * default `FULLPAGE_TITLE_KEYS` title — so its internal drill nav (e.g. 数据源 ›
 * 联系人) shows in the one global header instead of a second stacked bar. Set on
 * mount / state-change, clear to null on unmount (mirrors copilotAppContext).
 * `HeaderCrumb` is structurally the same as ShellNav's `Crumb`.
 */
export interface HeaderCrumb {
    label: string;
    onClick?: () => void;
}
export const headerCrumbs = signal<HeaderCrumb[] | null>(null);
export {
    HEADER_BACK_PRIORITY,
    clearHeaderBackAction,
    clearHeaderBackActions,
    headerBackAction,
    registerHeaderBackAction,
} from './headerBackStore';

/**
 * Add-action bridge for the panel header. When TaskList runs inside the panel
 * (controlled mode), it publishes the current view's create action here —
 * 新建讨论 / 新建里程碑 — so the panel-tabs-header can render one "+" on the
 * right instead of a second button row inside the board. null = no add action
 * for the current view (or TaskList is standalone and renders its own button).
 */
export const taskAddAction = signal<{ title: string; run: () => void } | null>(null);

/** Clear a consumed request (called by TaskList once it applies the selection). */
export const consumePendingTaskNav = (): void => {
    pendingTaskNav.value = null;
};

/**
 * Focus a task by its workspace + hex id: switch to the project, open the task
 * board, and queue the selection. Safe to call whether or not that project is
 * already active.
 */
export const openTaskById = (workspaceId: string, taskId: string): void => {
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);
    if (ws) {
        void wsStore.selectWorkspace(ws);
    }
    // Ensure the task board is the visible content (desktop = right column,
    // mobile = legacy 任务 subview).
    if (ui.isMobile.value) {
        tabsStore.selectTab('tasks');
    } else {
        tabsStore.openContentTab('tasks');
    }
    pendingTaskNav.value = { workspaceId, taskId };
};

/**
 * Resolve a GitHub-style reference (project name/id + #number) to a task and
 * focus it. Shows a toast when the reference can't be resolved rather than
 * navigating to a dead URL.
 */
export const openTaskByRef = async (project: string, number: number): Promise<void> => {
    try {
        const ref = await projectItemService.resolve(project, number);
        if (!ref) {
            ui.showToast(`未找到任务：${project}#${number}`);
            return;
        }
        openTaskById(ref.workspaceId, ref.task.id);
    } catch {
        ui.showToast(`无法打开任务：${project}#${number}`);
    }
};

/** Build the shareable permalink for a task (absolute, GitHub-style). */
export const taskPermalink = (projectName: string, number: number): string =>
    `${window.location.origin}/${encodeURIComponent(projectName)}/tasks/${number}`;

/**
 * Open a file by its path (relative to the current workspace root, or
 * absolute) in the right-side files pane's detail view — the same surface the
 * file browser uses when you click a file. Silently does nothing when the file
 * can't be found, so a mistyped ref is treated as a typo.
 */
const openFileByPath = async (path: string, line?: number, lineEnd?: number): Promise<void> => {
    // Preflight: bail silently on 404 so a mistyped ref is treated as a typo
    // rather than opening the pane onto an error.
    try {
        await fsService.read(path);
    } catch {
        return;
    }

    const name = path.split('/').pop() || path;
    // Open the files pane (mobile = files subview, desktop = right column),
    // then load the file into its detail view via the shared store action.
    if (ui.isMobile.value) {
        tabsStore.selectTab('files');
    } else {
        tabsStore.openContentTab('files');
    }
    void fsStore.openFileDetail({ name, path, isDir: false, size: 0, modTime: 0 }, line, lineEnd);
};

/**
 * Global delegated click handler for autolinked task references and pasted
 * permalink URLs. Intercepts left-clicks (no modifier) so navigation stays
 * in-app; modifier-clicks fall through to the browser (open in new tab works
 * because the anchor carries a real href).
 */
const onDocumentClick = (e: MouseEvent): void => {
    if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) {
        return;
    }
    const anchor = (e.target as HTMLElement | null)?.closest('a');
    if (!anchor) return;

    // Autolinked file-path reference (`path/to/file.ext` or `file.ts:42-85`).
    if (anchor.hasAttribute('data-file-ref')) {
        const path = anchor.getAttribute('data-path') || '';
        if (!path) return;
        const line = parseInt(anchor.getAttribute('data-line') || '', 10) || undefined;
        const lineEnd = parseInt(anchor.getAttribute('data-line-end') || '', 10) || undefined;
        e.preventDefault();
        void openFileByPath(path, line, lineEnd);
        return;
    }

    // Autolinked `#N` reference carries explicit data attributes.
    if (anchor.hasAttribute('data-task-ref')) {
        const number = parseInt(anchor.getAttribute('data-number') || '', 10);
        if (!Number.isFinite(number)) return;
        const project = anchor.getAttribute('data-project') || activeProjectName();
        if (!project) return;
        e.preventDefault();
        void openTaskByRef(project, number);
        return;
    }

    // A bare permalink URL (e.g. an agent pasted the full link). Only intercept
    // same-origin ones that match the permalink shape.
    const href = anchor.getAttribute('href') || '';
    if (href.startsWith('/') || anchor.origin === window.location.origin) {
        const ref = parseTaskPermalink(anchor.pathname);
        if (ref) {
            e.preventDefault();
            void openTaskByRef(ref.project, ref.number);
            return;
        }
    }

    // External / local http(s) links → built-in browser side pane
    // (e.g. http://localhost:3000, https://…). mailto/tel fall through.
    if (/^https?:\/\//i.test(href) || href.startsWith('//')) {
        try {
            const u = new URL(href, window.location.href);
            e.preventDefault();
            tabsStore.openBrowserTab(u.href);
        } catch {
            /* ignore bad URLs */
        }
    }
};

/** Active workspace display name, used as the implicit project for bare `#N`. */
export const activeProjectName = (): string => {
    const id = wsStore.activeWorkspaceId.value;
    return wsStore.workspaces.value.find(w => w.id === id)?.name || '';
};

let installed = false;
/** Install the global click interceptor once (idempotent). */
export const installTaskRefClicks = (): void => {
    if (installed) return;
    installed = true;
    document.addEventListener('click', onDocumentClick);
};

/**
 * Consume a /{project}/tasks/{number} deep link from the current location, if
 * present, opening the referenced task and cleaning the URL back to root.
 * Returns true when a permalink was handled.
 */
export const consumeDeepLink = (): boolean => {
    const ref = parseTaskPermalink(window.location.pathname);
    if (!ref) return false;
    void openTaskByRef(ref.project, ref.number);
    window.history.replaceState(null, '', '/');
    return true;
};
