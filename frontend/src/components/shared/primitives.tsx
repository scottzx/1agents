import { h, ComponentChildren } from 'preact';
import { useSignal } from '@preact/signals';
import { useRef, useEffect } from 'preact/hooks';

// ─────────────────────────────────────────────────────────────────────────────
// Shared UI Primitives — docs/design_rules/component-patterns.md
// Usable across Chat, Task, DataSources, Git/QA/Deployment, Settings.
// All icon buttons expose title + aria-label. No nested large cards or
// marketing visuals. Uses only tokens defined in style/index.scss :root.
// ─────────────────────────────────────────────────────────────────────────────

// ── ProcessBlock ──────────────────────────────────────────────────────────────
// Collapsible agentic process wrapper (chat tool groups, sync steps, deploy
// runs, QA records). Auto-expands when waiting/error, auto-collapses on success.

export type ProcessStatus = 'running' | 'waiting' | 'success' | 'error' | 'incomplete' | 'idle';

interface ProcessBlockProps {
    title: string;
    status: ProcessStatus;
    /** Short label shown in the header — e.g. "完成", "3 errors", "等待确认" */
    statusLabel?: string;
    children?: ComponentChildren;
    /**
     * Override the default expand logic. By default: expanded when
     * running/waiting/error, collapsed when success/idle/incomplete.
     */
    defaultExpanded?: boolean;
}

export function ProcessBlock({ title, status, statusLabel, children, defaultExpanded }: ProcessBlockProps) {
    const initial =
        defaultExpanded !== undefined
            ? defaultExpanded
            : status === 'running' || status === 'waiting' || status === 'error';
    const isExpanded = useSignal(initial);

    // Respond to status transitions: force-open on waiting/error, collapse on success.
    const prevStatus = useRef(status);
    useEffect(() => {
        if (prevStatus.current === status) return;
        prevStatus.current = status;
        if (status === 'waiting' || status === 'error') {
            isExpanded.value = true;
        } else if (status === 'success') {
            isExpanded.value = false;
        }
    }, [status]);

    const toggle = () => {
        isExpanded.value = !isExpanded.value;
    };
    const expanded = isExpanded.value;

    return (
        <div class={`sp-process-block sp-ps-${status}${expanded ? ' is-expanded' : ' is-collapsed'}`}>
            <div
                class="sp-process-header"
                role="button"
                tabIndex={0}
                aria-expanded={expanded}
                onClick={toggle}
                onKeyDown={e => {
                    if (e.key === 'Enter' || e.key === ' ') {
                        e.preventDefault();
                        toggle();
                    }
                }}
            >
                <span class="sp-process-caret" aria-hidden="true">
                    {expanded ? '▾' : '▸'}
                </span>
                <span class="sp-process-title">{title}</span>
                {statusLabel && <span class={`sp-process-status-label sp-ps-label-${status}`}>{statusLabel}</span>}
                <span class={`sp-dot sp-dot-${status}`} aria-hidden="true" />
            </div>
            {expanded && children && <div class="sp-process-body">{children}</div>}
        </div>
    );
}

// ── StatusRow ─────────────────────────────────────────────────────────────────
// Single-row status object for lists (sessions, tasks, data sources, devices).

interface StatusRowProps {
    icon?: ComponentChildren;
    status?: ProcessStatus;
    title: string;
    summary?: string;
    badge?: ComponentChildren;
    time?: string;
    actions?: ComponentChildren;
    onClick?: () => void;
    selected?: boolean;
}

export function StatusRow({ icon, status, title, summary, badge, time, actions, onClick, selected }: StatusRowProps) {
    const cls = ['sp-status-row', onClick ? 'is-clickable' : '', selected ? 'is-selected' : '']
        .filter(Boolean)
        .join(' ');

    return (
        <div
            class={cls}
            onClick={onClick}
            role={onClick ? 'button' : undefined}
            tabIndex={onClick ? 0 : undefined}
            onKeyDown={
                onClick
                    ? e => {
                          if (e.key === 'Enter') onClick();
                      }
                    : undefined
            }
        >
            <div class="sp-status-row-left">
                {icon ? (
                    <span class="sp-status-row-icon" aria-hidden="true">
                        {icon}
                    </span>
                ) : status ? (
                    <span class={`sp-dot sp-dot-${status}`} aria-hidden="true" />
                ) : null}
                <div class="sp-status-row-content">
                    <span class="sp-status-row-title">{title}</span>
                    {summary && <span class="sp-status-row-summary">{summary}</span>}
                </div>
            </div>
            {(badge !== undefined || time !== undefined || actions !== undefined) && (
                <div class="sp-status-row-right">
                    {badge}
                    {time && <span class="sp-status-row-time">{time}</span>}
                    {actions && (
                        <span class="sp-status-row-actions" onClick={e => e.stopPropagation()}>
                            {actions}
                        </span>
                    )}
                </div>
            )}
        </div>
    );
}

// ── InlineBadge ───────────────────────────────────────────────────────────────
// Row-inline metadata label (file path, tool name, agent type, status tag…).

export type BadgeVariant = 'default' | 'mono' | 'success' | 'danger' | 'warning' | 'accent' | 'muted' | 'purple';

interface InlineBadgeProps {
    children: ComponentChildren;
    variant?: BadgeVariant;
    title?: string;
    onClick?: () => void;
}

export function InlineBadge({ children, variant = 'default', title, onClick }: InlineBadgeProps) {
    const cls = `sp-badge sp-badge-${variant}${onClick ? ' is-clickable' : ''}`;
    if (onClick) {
        return (
            <button type="button" class={cls} title={title} aria-label={title} onClick={onClick}>
                {children}
            </button>
        );
    }
    return (
        <span class={cls} title={title}>
            {children}
        </span>
    );
}

// ── TerminalLiteBlock ─────────────────────────────────────────────────────────
// Command + log output. Shell command is shown as a `$ cmd` line. Output scrolls.
// Copy button copies both the command and output to clipboard.

interface TerminalLiteBlockProps {
    command?: string;
    output?: string;
    isError?: boolean;
    /** Max height of the output area in px (default 240). */
    maxHeight?: number;
}

export function TerminalLiteBlock({ command, output, isError, maxHeight = 240 }: TerminalLiteBlockProps) {
    const copied = useSignal(false);

    const copyAll = () => {
        const parts = [command ? `$ ${command}` : null, output ?? null].filter(Boolean);
        void navigator.clipboard.writeText(parts.join('\n')).then(() => {
            copied.value = true;
            setTimeout(() => {
                copied.value = false;
            }, 1500);
        });
    };

    const hasCopyable = command !== undefined || output !== undefined;

    return (
        <div class={`sp-terminal-block${isError ? ' has-error' : ''}`}>
            <div class="sp-terminal-cmd">
                <span class="sp-terminal-prompt" aria-hidden="true">
                    $
                </span>
                <span class="sp-terminal-cmd-text">{command ?? ''}</span>
                {hasCopyable && (
                    <button
                        type="button"
                        class={`sp-terminal-copy-btn${copied.value ? ' is-copied' : ''}`}
                        title={copied.value ? '已复制' : '复制'}
                        aria-label={copied.value ? '已复制' : '复制日志'}
                        onClick={copyAll}
                    >
                        {copied.value ? '✓' : '⎘'}
                    </button>
                )}
            </div>
            {output !== undefined && (
                <pre class="sp-terminal-output" style={{ maxHeight: `${maxHeight}px` }}>
                    {output || ''}
                </pre>
            )}
        </div>
    );
}

// ── PermissionDecision ────────────────────────────────────────────────────────
// Human-in-the-loop confirmation widget. Pending shows action buttons;
// resolved collapses to a one-line receipt. Always visible when pending.

interface PermissionDecisionProps {
    description: string;
    scope?: string;
    resolved?: 'allow' | 'deny';
    onDenyAlways?: () => void;
    onDeny?: () => void;
    onAllow?: () => void;
    onAllowAlways?: () => void;
    /** Override button labels (e.g. for localisation). */
    labels?: {
        denyAlways?: string;
        deny?: string;
        allow?: string;
        allowAlways?: string;
        resolvedAllow?: string;
        resolvedDeny?: string;
    };
}

export function PermissionDecision({
    description,
    scope,
    resolved,
    onDenyAlways,
    onDeny,
    onAllow,
    onAllowAlways,
    labels = {},
}: PermissionDecisionProps) {
    if (resolved) {
        const resolvedLabel = resolved === 'allow' ? labels.resolvedAllow ?? '已允许' : labels.resolvedDeny ?? '已拒绝';
        return (
            <div class={`sp-permission is-resolved sp-permission-${resolved}`}>
                <span class="sp-permission-mark" aria-hidden="true">
                    {resolved === 'allow' ? '✓' : '✕'}
                </span>
                <span class="sp-permission-resolved-label">{resolvedLabel}</span>
                {scope && <span class="sp-permission-scope">{scope}</span>}
            </div>
        );
    }

    return (
        <div class="sp-permission is-pending">
            <div class="sp-permission-desc">{description}</div>
            {scope && <div class="sp-permission-scope">{scope}</div>}
            <div class="sp-permission-actions">
                {onDenyAlways && (
                    <button
                        type="button"
                        class="sp-permission-btn is-deny-always"
                        title={labels.denyAlways ?? '总是拒绝'}
                        onClick={onDenyAlways}
                    >
                        {labels.denyAlways ?? '总是拒绝'}
                    </button>
                )}
                {onDeny && (
                    <button
                        type="button"
                        class="sp-permission-btn is-deny"
                        title={labels.deny ?? '拒绝'}
                        onClick={onDeny}
                    >
                        {labels.deny ?? '拒绝'}
                    </button>
                )}
                {onAllow && (
                    <button
                        type="button"
                        class="sp-permission-btn is-allow"
                        title={labels.allow ?? '允许'}
                        onClick={onAllow}
                    >
                        {labels.allow ?? '允许'}
                    </button>
                )}
                {onAllowAlways && (
                    <button
                        type="button"
                        class="sp-permission-btn is-allow-always"
                        title={labels.allowAlways ?? '总是允许'}
                        onClick={onAllowAlways}
                    >
                        {labels.allowAlways ?? '总是允许'}
                    </button>
                )}
            </div>
        </div>
    );
}

// ── DetailSection ─────────────────────────────────────────────────────────────
// Labelled information group for detail/settings pages. Not a floating card —
// just a semantic section with a short title and optional description.

interface DetailSectionProps {
    title: string;
    description?: string;
    /** Danger zone styling (border-top in danger-fg). */
    danger?: boolean;
    children: ComponentChildren;
}

export function DetailSection({ title, description, danger, children }: DetailSectionProps) {
    return (
        <section class={`sp-detail-section${danger ? ' is-danger' : ''}`}>
            <div class="sp-detail-section-head">
                <h4 class="sp-detail-section-title">{title}</h4>
                {description && <p class="sp-detail-section-desc">{description}</p>}
            </div>
            <div class="sp-detail-section-body">{children}</div>
        </section>
    );
}

// ── ActionToolbar ─────────────────────────────────────────────────────────────
// Page-level or object-level action row. Icon-only buttons must always have a
// title/aria-label — enforced by the ToolbarAction type (title is required).

export interface ToolbarAction {
    icon?: ComponentChildren;
    label: string;
    /** Required — shown as tooltip and read by screen readers. */
    title: string;
    onClick?: () => void;
    /** Accent-filled primary action (at most one per toolbar). */
    primary?: boolean;
    /** Danger-tinted destructive action. */
    danger?: boolean;
    /** Show icon only; label becomes visually hidden but present as aria-label. */
    iconOnly?: boolean;
    disabled?: boolean;
}

interface ActionToolbarProps {
    actions: ToolbarAction[];
    gap?: 'tight' | 'normal';
}

export function ActionToolbar({ actions, gap = 'normal' }: ActionToolbarProps) {
    return (
        <div class={`sp-action-toolbar sp-toolbar-gap-${gap}`}>
            {actions.map((action, i) => {
                const cls = [
                    'sp-toolbar-btn',
                    action.primary ? 'is-primary' : '',
                    action.danger ? 'is-danger' : '',
                    action.iconOnly ? 'is-icon-only' : '',
                ]
                    .filter(Boolean)
                    .join(' ');
                return (
                    <button
                        key={i}
                        type="button"
                        class={cls}
                        title={action.title}
                        aria-label={action.title}
                        onClick={action.onClick}
                        disabled={action.disabled}
                    >
                        {action.icon && (
                            <span class="sp-toolbar-btn-icon" aria-hidden="true">
                                {action.icon}
                            </span>
                        )}
                        {action.iconOnly ? (
                            <span class="sp-visually-hidden">{action.label}</span>
                        ) : (
                            <span class="sp-toolbar-btn-label">{action.label}</span>
                        )}
                    </button>
                );
            })}
        </div>
    );
}

// ── EmptyState ────────────────────────────────────────────────────────────────
// Minimal placeholder for empty lists or views. No large illustrations.

interface EmptyStateProps {
    title: string;
    description?: string;
    /** Optional small icon (SVG). Keep it compact — no hero illustrations. */
    icon?: ComponentChildren;
    action?: { label: string; title?: string; onClick: () => void };
}

export function EmptyState({ title, description, icon, action }: EmptyStateProps) {
    return (
        <div class="sp-empty-state">
            {icon && (
                <div class="sp-empty-state-icon" aria-hidden="true">
                    {icon}
                </div>
            )}
            <p class="sp-empty-state-title">{title}</p>
            {description && <p class="sp-empty-state-desc">{description}</p>}
            {action && (
                <button
                    type="button"
                    class="sp-empty-state-action"
                    title={action.title ?? action.label}
                    aria-label={action.title ?? action.label}
                    onClick={action.onClick}
                >
                    {action.label}
                </button>
            )}
        </div>
    );
}

// ── ErrorState ────────────────────────────────────────────────────────────────
// Error/failure display. Technical details are foldable. No large red fills.

interface ErrorStateProps {
    title: string;
    message?: string;
    /** Foldable technical detail (stack trace, raw error, etc.). */
    detail?: string;
    onRetry?: () => void;
    retryLabel?: string;
}

export function ErrorState({ title, message, detail, onRetry, retryLabel = '重试' }: ErrorStateProps) {
    const showDetail = useSignal(false);

    return (
        <div class="sp-error-state">
            <p class="sp-error-state-title">{title}</p>
            {message && <p class="sp-error-state-message">{message}</p>}
            {detail && (
                <div class="sp-error-state-detail-wrap">
                    <button
                        type="button"
                        class="sp-error-detail-toggle"
                        title={showDetail.value ? '收起技术详情' : '展开技术详情'}
                        aria-expanded={showDetail.value}
                        onClick={() => {
                            showDetail.value = !showDetail.value;
                        }}
                    >
                        <span aria-hidden="true">{showDetail.value ? '▾' : '▸'}</span>
                        技术详情
                    </button>
                    {showDetail.value && <pre class="sp-error-detail">{detail}</pre>}
                </div>
            )}
            {onRetry && (
                <button
                    type="button"
                    class="sp-error-retry"
                    title={retryLabel}
                    aria-label={retryLabel}
                    onClick={onRetry}
                >
                    {retryLabel}
                </button>
            )}
        </div>
    );
}
