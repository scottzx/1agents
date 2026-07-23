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

/** CJK / fullwidth-ish code points count as 2 units; Latin/ASCII as 1.
 *  Cap is 20 units → 中文 ≤10 字、英文 ≤20 字；混合按权重截断。 */
const CRUMB_CURRENT_MAX_UNITS = 20;

function charDisplayUnits(ch: string): number {
    const code = ch.codePointAt(0) ?? 0;
    // CJK Unified Ideographs, Extension A–F ranges, fullwidth forms, Hangul,
    // Hiragana/Katakana, and other wide East-Asian blocks commonly used in UI.
    if (
        (code >= 0x1100 && code <= 0x11ff) || // Hangul Jamo
        (code >= 0x2e80 && code <= 0x9fff) || // CJK radicals … CJK Unified
        (code >= 0xa960 && code <= 0xa97f) || // Hangul Jamo Extended-A
        (code >= 0xac00 && code <= 0xd7af) || // Hangul Syllables
        (code >= 0xf900 && code <= 0xfaff) || // CJK Compatibility Ideographs
        (code >= 0xfe10 && code <= 0xfe1f) || // Vertical forms
        (code >= 0xfe30 && code <= 0xfe4f) || // CJK Compatibility Forms
        (code >= 0xff00 && code <= 0xff60) || // Fullwidth Forms
        (code >= 0xffe0 && code <= 0xffe6) || // Fullwidth symbols
        (code >= 0x20000 && code <= 0x2ceaf) // CJK Extension B–F
    ) {
        return 2;
    }
    return 1;
}

/** Truncate a crumb current label: 中文 ≤10、英文 ≤20，超出以 ... 替代。 */
function truncateCrumbCurrent(label: string): string {
    let units = 0;
    let end = 0;
    for (const ch of label) {
        const next = units + charDisplayUnits(ch);
        if (next > CRUMB_CURRENT_MAX_UNITS) {
            return label.slice(0, end) + '...';
        }
        units = next;
        end += ch.length;
    }
    return label;
}

/**
 * The breadcrumb trail itself (link › link › current), with no bar wrapper.
 * Reusable so the WorkspaceHeader can render a breadcrumb inline where it used
 * to show a bare module title — same trail markup ShellNav wraps in a bar.
 */
export function CrumbTrail({ crumbs }: { crumbs: Crumb[] }) {
    return (
        <Fragment>
            {crumbs.map((c, i) => (
                <Fragment key={i}>
                    {i > 0 && <ChevronSep />}
                    {c.onClick ? (
                        <button class="project-crumb-link" onClick={c.onClick}>
                            {c.label}
                        </button>
                    ) : (
                        <span class="project-crumb-current" title={c.label}>
                            {truncateCrumbCurrent(c.label)}
                        </span>
                    )}
                </Fragment>
            ))}
        </Fragment>
    );
}

export function ShellNav({ crumbs, tabs, activeTab, onSelectTab, actions }: ShellNavProps) {
    return (
        <Fragment>
            {crumbs && crumbs.length > 0 && (
                <div class="project-detail-breadcrumb">
                    <CrumbTrail crumbs={crumbs} />
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
