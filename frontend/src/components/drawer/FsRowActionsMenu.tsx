import { h, Fragment } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import { t, type Lang } from '../i18n';
import type { FsEntry } from '../types';

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
 * row. Click-outside / Escape closes the dropdown.
 *
 * The menu is rendered as a sibling of the trigger inside `.fb-row-actions`.
 * `.fb-actions-menu` is `position: fixed`, so the row's `display: none` on
 * un-hover doesn't hide the menu visually — but it WOULD unmount the menu
 * (parent display:none tears down all descendants). So we also keep the
 * row-actions container visible via the `:has(.fb-actions-menu)` CSS rule.
 */
export function FsRowActionsMenu({ entry, items, language }: { entry: FsEntry; items: FsRowAction[]; language: Lang }) {
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
                class="fb-row-action-btn fb-actions-trigger"
                title={t('fileBrowser.actionsMenu', language)}
                aria-label={t('fileBrowser.actionsMenu', language)}
                data-entry-path={entry.path}
                onClick={(e: MouseEvent) => {
                    e.stopPropagation();
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
            {open && pos && (
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
                </div>
            )}
        </Fragment>
    );
}
