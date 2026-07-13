import { h, Fragment } from 'preact';
import { createPortal } from 'preact/compat';
import { useEffect, useRef, useState } from 'preact/hooks';
import { t, type Lang } from '../i18n';

export interface FsRowAction {
    id: string;
    labelKey: string;
    danger?: boolean;
    /** Inline icon (12px SVG). Required for visual consistency. */
    icon: preact.ComponentChildren;
    onSelect: () => void;
}

/**
 * Single "..." trigger button + dropdown menu used in every FlatFileBrowser
 * row, and (re-used) by the sidebar workspace / chat rows. Click-outside /
 * Escape closes the dropdown.
 *
 * The menu is portalled to `document.body` rather than rendered next to the
 * trigger. Two reasons:
 *  1. The trigger's parent (`display: none` on un-hover) would otherwise
 *     unmount the menu mid-open — see the `:has(.fb-actions-menu)` rule on
 *     the row actions container.
 *  2. Ancestor `transform`/`filter`/`perspective` (e.g. sidebar's
 *     `.project-node { transform: translateX(0) scale(1) }`) creates a
 *     containing block that pins `position: fixed` to the wrong box.
 *     Portalling out restores true viewport-relative positioning.
 */
export function FsRowActionsMenu({
    entry,
    items,
    language,
    triggerClassName,
}: {
    /**
     * Source row the menu is attached to. Only `path` is read (as a
     * `data-entry-path` attribute on the trigger) — Workspace and Session
     * are accepted alongside FsEntry so the same component can host
     * sidebar row actions without an adapter type.
     */
    entry: { path?: string };
    items: FsRowAction[];
    language: Lang;
    /** Extra class merged into the "..." trigger button — used by callers
     *  (sidebar rows, etc.) to size/style the trigger per their context. */
    triggerClassName?: string;
}) {
    const [open, setOpen] = useState(false);
    const triggerRef = useRef<HTMLButtonElement | null>(null);
    const menuRef = useRef<HTMLDivElement | null>(null);
    const [pos, setPos] = useState<{ top: number; right: number } | null>(null);

    const place = () => {
        const el = triggerRef.current;
        if (!el) return;
        const rect = el.getBoundingClientRect();

        // Each menu row is ~30px (6px padding × 2 + 18px text) plus a 1px
        // gap; the container itself has 4px padding × 2. We use this rough
        // estimate to decide whether the menu fits below the trigger or
        // should flip above it before we render — avoids a visible jump on
        // open when the trigger sits near the viewport bottom.
        const estimatedHeight = items.length * 31 + 8;
        const viewportHeight = window.innerHeight;
        const overflowsBottom = rect.bottom + estimatedHeight > viewportHeight;
        const roomAbove = rect.top >= estimatedHeight;
        const placeAbove = overflowsBottom && roomAbove;

        setPos({
            top: placeAbove ? rect.top - estimatedHeight : rect.bottom,
            right: window.innerWidth - rect.right,
        });
    };

    useEffect(() => {
        if (!open) return;
        place();

        const onDocDown = (e: MouseEvent) => {
            const target = e.target as Node;
            if (triggerRef.current?.contains(target)) return;
            if (menuRef.current?.contains(target)) return;
            setOpen(false);
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setOpen(false);
        };
        const onResizeOrScroll = () => {
            // Anchor stays correct only while viewport is stable; close so
            // the next "..." click computes a fresh position.
            setOpen(false);
        };

        document.addEventListener('mousedown', onDocDown);
        document.addEventListener('keydown', onKey);
        window.addEventListener('resize', onResizeOrScroll);
        window.addEventListener('scroll', onResizeOrScroll, true);
        return () => {
            document.removeEventListener('mousedown', onDocDown);
            document.removeEventListener('keydown', onKey);
            window.removeEventListener('resize', onResizeOrScroll);
            window.removeEventListener('scroll', onResizeOrScroll, true);
        };
    }, [open]);

    return (
        <Fragment>
            <button
                ref={triggerRef}
                type="button"
                class={`fb-row-action-btn fb-actions-trigger${triggerClassName ? ' ' + triggerClassName : ''}`}
                title={t('fileBrowser.actionsMenu', language)}
                aria-label={t('fileBrowser.actionsMenu', language)}
                data-entry-path={entry.path}
                onClick={(e: MouseEvent) => {
                    e.stopPropagation();
                    setPos(null);
                    setOpen(o => !o);
                }}
            >
                <svg
                    width="14"
                    height="14"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.4"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="5" cy="12" r="1.4" />
                    <circle cx="12" cy="12" r="1.4" />
                    <circle cx="19" cy="12" r="1.4" />
                </svg>
            </button>
            {open &&
                pos &&
                createPortal(
                    <div
                        ref={menuRef}
                        class="fb-actions-menu"
                        style={`position: fixed; top: ${pos.top}px; right: ${pos.right}px;`}
                        role="menu"
                        onMouseDown={(e: MouseEvent) => e.stopPropagation()}
                        onClick={(e: MouseEvent) => e.stopPropagation()}
                    >
                        {items.map(item => (
                            <button
                                key={item.id}
                                type="button"
                                class={`fb-actions-menu-item ${item.danger ? 'fb-actions-menu-item-danger' : ''}`}
                                role="menuitem"
                                onMouseDown={(e: MouseEvent) => e.stopPropagation()}
                                onClick={() => {
                                    setOpen(false);
                                    item.onSelect();
                                }}
                            >
                                <span class="fb-actions-menu-icon">{item.icon}</span>
                                <span class="fb-actions-menu-label">{t(item.labelKey, language)}</span>
                            </button>
                        ))}
                    </div>,
                    document.body
                )}
        </Fragment>
    );
}
