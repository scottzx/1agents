/**
 * Module Slot Architecture — public contract types.
 *
 * Every module embedded by the main app (1skills, cc-connect, future modules)
 * communicates its navigation surface through a `ModuleManifest`. The host's
 * `LeftSidebar` renders the manifest inside its own column; the module iframe
 * is then loaded in `?bare=1` mode and renders only the active route's content.
 *
 * This file is the single source of truth for the contract. Module code may
 * serialize to this shape on the wire; the host consumes it via the manifest
 * registry and renders it via `<ModuleNav />`.
 */

/** Module identifier — used as a key in the registry and in URL paths. */
export type ModuleId = string;

/** A single navigation link inside a module. */
export interface ModuleNavLink {
    /** Unique within the module (used as React key). */
    key: string;
    /** Display text. May be a plain string or a translation key — host decides. */
    label: string;
    /** Route inside the module, e.g. "/skills/use". */
    to: string;
    /** Icon hint resolved by the host's icon registry. */
    iconKey?: string;
    /** Optional count badge (e.g. "needs review: 3"). */
    count?: number | null;
    /** Optional textual badge type. */
    badge?: 'review' | 'info' | null;
}

/** A collapsible group of links in a module's navigation. */
export interface ModuleNavGroup {
    /** Unique within the module. */
    key: string;
    /** Group label displayed in the sidebar. */
    label: string;
    /** Icon for the group header. */
    iconKey?: string;
    /** Optional group-level count. */
    count?: number | null;
    /** Child links. */
    links: ModuleNavLink[];
    /** Whether the group can be collapsed (default true). */
    collapsible?: boolean;
    /** Initial collapsed state. */
    defaultCollapsed?: boolean;
}

/** Action the host can render in the chrome (e.g. a Refresh button). */
export interface ModuleHeaderAction {
    key: string;
    label: string;
    iconKey?: string;
}

/** The full module manifest exchanged with the host. */
export interface ModuleManifest {
    moduleId: ModuleId;
    /** Schema version. Bump when breaking changes are made. */
    version: 1;
    /** Initial route when the module is first opened. */
    entryPath: string;
    /** Top-level (non-grouped) links — rendered above groups. */
    topLinks?: ModuleNavLink[];
    /** Navigation groups — collapsible, with optional counts. */
    groups: ModuleNavGroup[];
    /** Actions the host can surface in its own chrome. */
    headerActions?: ModuleHeaderAction[];
}
