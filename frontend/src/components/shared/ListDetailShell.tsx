import { h, ComponentChildren } from 'preact';

interface ListDetailShellProps {
    /** The master list / board — fills the pane and scrolls on its own. */
    list: ComponentChildren;
    /** The inspector body. When null/undefined the inspector is collapsed to 0. */
    detail?: ComponentChildren | null;
    /** Inspector width in px when open (default 320). */
    detailWidth?: number;
    /** Header title shown above the inspector body. */
    detailTitle?: string;
    /** Close handler — renders an × in the inspector header when provided. */
    onCloseDetail?: () => void;
}

/**
 * The focus-mode master-detail frame (#redesign). A full-width list on the
 * left with a non-modal inspector that slides in from the right — the list
 * stays visible so the user can scan-and-drill continuously (the chosen
 * "右侧内嵌抽屉" pattern). Panes that already own a modal / popover keep those;
 * this is for list panes that want a docked detail (待办 today).
 */
export function ListDetailShell({ list, detail, detailWidth = 320, detailTitle, onCloseDetail }: ListDetailShellProps) {
    const open = detail !== null && detail !== undefined;
    return (
        <div class="list-detail-shell">
            <div class="list-detail-main">{list}</div>
            <div class={`list-detail-inspector${open ? ' open' : ''}`} style={{ width: open ? detailWidth : 0 }}>
                {open && (
                    <div class="list-detail-inspector-inner" style={{ width: detailWidth }}>
                        <div class="list-detail-inspector-head">
                            <span class="list-detail-inspector-title">{detailTitle}</span>
                            {onCloseDetail && (
                                <button
                                    class="list-detail-inspector-close"
                                    onClick={onCloseDetail}
                                    aria-label="close detail"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <line x1="18" y1="6" x2="6" y2="18" />
                                        <line x1="6" y1="6" x2="18" y2="18" />
                                    </svg>
                                </button>
                            )}
                        </div>
                        <div class="list-detail-inspector-body">{detail}</div>
                    </div>
                )}
            </div>
        </div>
    );
}
