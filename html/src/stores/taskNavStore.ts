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
import { parseTaskPermalink } from '../utils/markdown';

/**
 * A pending request to open a task, consumed by the mounted <TaskList> for the
 * matching workspace. A signal (not a direct call) bridges the async gap: the
 * project switch + view open may mount TaskList a tick later, and its effect
 * picks the request up whenever it lands.
 */
export const pendingTaskNav = signal<{ workspaceId: string; taskId: string } | null>(null);

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
        const res = await fetch(
            `/api/agent/tasks/resolve?project=${encodeURIComponent(project)}&number=${number}`
        );
        if (!res.ok) {
            ui.showToast(`未找到任务：${project}#${number}`);
            return;
        }
        const data = (await res.json()) as { workspaceId: string; task: { id: string } };
        openTaskById(data.workspaceId, data.task.id);
    } catch {
        ui.showToast(`无法打开任务：${project}#${number}`);
    }
};

/** Build the shareable permalink for a task (absolute, GitHub-style). */
export const taskPermalink = (projectName: string, number: number): string =>
    `${window.location.origin}/${encodeURIComponent(projectName)}/tasks/${number}`;

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
