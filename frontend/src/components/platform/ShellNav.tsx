import { h, Fragment } from 'preact';
import type { ComponentChildren } from 'preact';

// ShellNav — the generic section navigation bar: a breadcrumb row over a level
// tab bar (+ optional right-aligned actions). Extracted from the project detail
// page (breadcrumb 项目总览 › <project> + 动态/计划/任务/资产 + gear) so any
// full-page section (项目管理, 数据源, …) reuses one nav. Emits the existing
// `.project-detail-breadcrumb` / `.project-shell-tabbar` classes so the visual
// is identical everywhere.

export interface Crumb {
    label: string;
    /** Present on ancestor crumbs (rendered as clickable links); omit on the
     *  current/leaf crumb (rendered bold, non-interactive). */
    onClick?: () => void;
}

export interface ShellTab {
    id: string;
    label: string;
    title?: string;
}

interface ShellNavProps {
    crumbs?: Crumb[];
    tabs?: ShellTab[];
    activeTab?: string;
    onSelectTab?: (id: string) => void;
    /** Right-aligned controls on the tab row (e.g. a settings gear). */
    actions?: ComponentChildren;
}

function ChevronSep() {
    return (
        <svg
            class="project-crumb-sep"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="9 6 15 12 9 18" />
        </svg>
    );
}

export function ShellNav({ crumbs, tabs, activeTab, onSelectTab, actions }: ShellNavProps) {
    return (
        <Fragment>
            {crumbs && crumbs.length > 0 && (
                <div class="project-detail-breadcrumb">
                    {crumbs.map((c, i) => (
                        <Fragment key={i}>
                            {i > 0 && <ChevronSep />}
                            {c.onClick ? (
                                <button class="project-crumb-link" onClick={c.onClick}>
                                    {c.label}
                                </button>
                            ) : (
                                <span class="project-crumb-current">{c.label}</span>
                            )}
                        </Fragment>
                    ))}
                </div>
            )}
            {((tabs && tabs.length > 0) || actions) && (
                <div class="project-shell-tabbar">
                    <div class="project-shell-tabs">
                        {tabs?.map(tb => (
                            <button
                                key={tb.id}
                                class={`project-shell-tab${activeTab === tb.id ? ' is-active' : ''}`}
                                onClick={() => onSelectTab?.(tb.id)}
                                title={tb.title}
                            >
                                {tb.label}
                            </button>
                        ))}
                    </div>
                    {actions}
                </div>
            )}
        </Fragment>
    );
}
