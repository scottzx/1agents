/**
 * Personal Shell registration (#328, design §8 / D7).
 *
 * The CURRENT default workbench IS the `personal` Product Shell. A Product
 * Shell is a UX composition layer, not a data layer: the personal shell does
 * not own workspaces, tasks or sessions — it is the set of built-in surfaces
 * the app already renders, shared with every other shell. This module is the
 * frontend half of the registration: the backend registers the `personal`
 * ProductShellManifest (appregistry/shell.go); here we declare which built-in
 * entries compose it, so the shell's composition is canonical, diffable, and
 * addressable by stable deep-link ids.
 *
 * The presales / commerce shells contribute through declarative app mount
 * points (MountPoint.shells). The personal shell is different: its entries
 * are the host's built-in navigation, declared here instead of being
 * re-implemented as mount points. `builtinEntriesForShell` therefore returns
 * these entries only for `personal` — every other shell composes from mounts.
 */

import { SHELL_IDS } from '../services/productShellService';

/** The registered id of the Personal Shell (stable; used in deep links). */
export const PERSONAL_SHELL_ID: string = SHELL_IDS.personal;

/**
 * One built-in entry of the personal shell. `id` is stable and safe to use in
 * deep links; `labelKey` is an i18n key resolved with `t(labelKey, language)`.
 */
export interface PersonalShellEntry {
    /** Stable entry id — must not change casually (deep links depend on it). */
    id: string;
    /** i18n key for the display label. */
    labelKey: string;
}

/**
 * The built-in workbench entries the personal shell retains (#328):
 * 项目 / 任务 / Chat / 终端 / 文件 / Inbox / 日程 / Agent / Function / 浏览器,
 * plus the right multi-tab side panel. Order is the canonical navigation
 * order. "Function" maps to the 百宝箱/Toolbox (installable functions/tools);
 * "日程" maps to the scheduled-tasks surface.
 */
export const PERSONAL_SHELL_ENTRIES: readonly PersonalShellEntry[] = [
    { id: 'projects', labelKey: 'projectHome.title' },
    { id: 'tasks', labelKey: 'sidebar.section.tasks' },
    { id: 'chat', labelKey: 'header.mobile.session' },
    { id: 'terminal', labelKey: 'sidePanel.tab.terminal' },
    { id: 'files', labelKey: 'header.mobile.files' },
    { id: 'inbox', labelKey: 'inbox.title' },
    { id: 'schedule', labelKey: 'sidebar.navCtrl.automation' },
    { id: 'agents', labelKey: 'sidebar.navCtrl.assistantOverview' },
    { id: 'functions', labelKey: 'header.title.skills' },
    { id: 'browser', labelKey: 'header.mobile.browser' },
    { id: 'side-panel', labelKey: 'personalShell.entry.sidePanel' },
];

/** True when shellId is the registered personal shell. */
export function isPersonalShell(shellId: string): boolean {
    return shellId === PERSONAL_SHELL_ID;
}

/**
 * The built-in entries a shell contributes. Only the personal shell has
 * built-in entries; other shells compose from declarative app mount points,
 * so they return an empty list here.
 */
export function builtinEntriesForShell(shellId: string): readonly PersonalShellEntry[] {
    return isPersonalShell(shellId) ? PERSONAL_SHELL_ENTRIES : [];
}
