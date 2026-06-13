import { signal } from '@preact/signals';

/**
 * Stage layout model — the "舞台" abstraction.
 *
 * The desktop stage is `NavRail (left) + ContentArea`. The ContentArea is
 * no longer hardwired to "middle workbench + right drawer"; it holds an
 * ordered list of `Pane`s (max two), and each pane renders any one
 * `ContentView`. This unifies the three overlapping mechanisms that used
 * to decide what shows where (tabsStore.activeTab for the middle canvas,
 * activeDrawerTab for the right drawer, Tab.type for full-page overlays)
 * under one clear model.
 *
 * The two panes keep the original spatial roles so existing muscle memory
 * survives — the refactor is about clarity, not relearning:
 *   - PRIMARY pane (`panes[0]`, always present): the focused main page.
 *     Driven by the LEFT NAV — clicking a top-level / resident item
 *     (terminal, chat, tasks…) replaces the primary content. Maps to the
 *     old `setActiveTab`.
 *   - SECONDARY pane (`panes[1]`, optional): the lower-frequency "drawer".
 *     Driven by DRAWER items (files, git, channels…) — clicking one
 *     auto-splits it open on the right; clicking the same kind again
 *     collapses back to a single column. Maps to the old `toggleDrawerTab`.
 *
 * Phase 1: this store + `<ContentViewHost>` are additive scaffolding —
 * nothing mounts them yet, so existing behavior is untouched. Phase 2
 * swaps DesktopAppLayout's content area onto `panes` and routes the
 * left-nav / drawer actions through `openPrimary` / `toggleSecondary`.
 */

/**
 * A unit of content that can live in a pane. The `kind` selects the leaf
 * component; the payload carries the identity the renderer needs.
 *
 * Note on singletons: `terminal` carries no id because the app runs a
 * single shared xterm (`window.term`, `id="terminal-container"`) that
 * follows the active tmux window — two terminals side-by-side is a later
 * capability, not modeled here. `chat` accepts an optional `sessionId`
 * so panes can show independent conversations; when absent it falls back
 * to the globally-active chat session (today's behavior).
 */
export type ContentView =
    | { kind: 'chat'; sessionId?: string }
    | { kind: 'newChat' }
    | { kind: 'terminal' }
    | { kind: 'preview'; tabId: string }
    | { kind: 'browser'; tabId: string }
    | { kind: 'files' }
    | { kind: 'git' }
    | { kind: 'tasks' }
    | { kind: 'channels' }
    | { kind: 'providers' }
    | { kind: 'skills' }
    | { kind: 'discovery' }
    | { kind: 'settings' };

export type ContentViewKind = ContentView['kind'];

export interface Pane {
    id: string;
    view: ContentView;
}

let paneSeq = 0;
const nextPaneId = (): string => `pane-${++paneSeq}`;
const makePane = (view: ContentView): Pane => ({ id: nextPaneId(), view });

/**
 * The content area: always `[primary]`, optionally `[primary, secondary]`.
 * Rendered with a uniform `panes.map(...)`; the role is positional.
 */
export const panes = signal<Pane[]>([makePane({ kind: 'terminal' })]);
/** Fraction of the content-area width given to the primary pane when split. */
export const splitRatio = signal(0.6);

export const primaryPane = (): Pane => panes.value[0];
export const secondaryPane = (): Pane | undefined => panes.value[1];
export const isSplit = (): boolean => panes.value.length > 1;

/** Structural equality used to detect "clicking the same drawer item again". */
const sameView = (a: ContentView, b: ContentView): boolean => {
    if (a.kind !== b.kind) return false;
    if (a.kind === 'chat' && b.kind === 'chat') return a.sessionId === b.sessionId;
    if ((a.kind === 'preview' || a.kind === 'browser') && (b.kind === 'preview' || b.kind === 'browser')) {
        return a.tabId === b.tabId;
    }
    return true;
};

/**
 * LEFT-NAV action: render `view` in the primary (focused main) pane,
 * replacing its content. Leaves the secondary pane untouched.
 */
export const openPrimary = (view: ContentView): void => {
    panes.value = [{ ...panes.value[0], view }, ...panes.value.slice(1)];
};

/**
 * DRAWER action: open `view` in the secondary pane (auto-split). Clicking
 * the same kind again collapses back to a single column; a different kind
 * swaps the secondary content in place. Mirrors the old `toggleDrawerTab`.
 */
export const toggleSecondary = (view: ContentView): void => {
    const current = secondaryPane();
    if (current && sameView(current.view, view)) {
        closeSecondary();
        return;
    }
    if (current) {
        panes.value = [panes.value[0], { ...current, view }];
    } else {
        panes.value = [panes.value[0], makePane(view)];
    }
};

/** Collapse the content area back to the single primary pane. */
export const closeSecondary = (): void => {
    if (panes.value.length > 1) panes.value = [panes.value[0]];
};

/** Replace a specific pane's content in place (used by drag-to-swap later). */
export const setPaneView = (paneId: string, view: ContentView): void => {
    panes.value = panes.value.map(p => (p.id === paneId ? { ...p, view } : p));
};
