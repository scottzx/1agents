import { h } from 'preact';
import { t, getLang } from '../../i18n';
import type { Lang } from '../../i18n';
import type { SessionUsage } from '@1agents/core/protocol/types';

// Composer-side token/context + cost readout, fed by the bridge `usage` event
// (ACP usage_update). Strictly presentational and fail-soft: any field the
// adapter didn't report is simply omitted, and a usage object with nothing
// renderable hides the whole badge. The per-turn token breakdown rides along
// as a native title tooltip (the "hover 展开明细" the issue asks for) — no
// popover state to manage.

function formatCost(cost: SessionUsage['cost']): string | null {
    if (!cost || cost.amount == null) return null;
    const currency = cost.currency || 'USD';
    // Intl handles the symbol + minor-unit rounding for whatever currency the
    // adapter reports; fall back to a plain "$" prefix if the code is unknown.
    try {
        return new Intl.NumberFormat(undefined, {
            style: 'currency',
            currency,
            maximumFractionDigits: cost.amount < 1 ? 4 : 2,
        }).format(cost.amount);
    } catch {
        return `$${cost.amount.toFixed(cost.amount < 1 ? 4 : 2)}`;
    }
}

function contextPercent(usage: SessionUsage): number | null {
    if (usage.used == null || usage.size == null || usage.size <= 0) return null;
    return Math.min(100, Math.round((usage.used / usage.size) * 100));
}

/** Compact number: 12_345 → "12.3k", 1_200_000 → "1.2M". */
function compact(n: number): string {
    if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
    if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
    return String(n);
}

function buildTooltip(usage: SessionUsage, lang: Lang): string {
    const lines: string[] = [];
    const pct = contextPercent(usage);
    if (pct != null && usage.used != null && usage.size != null) {
        lines.push(
            t('chat.usage.context', lang, {
                percent: String(pct),
                used: compact(usage.used),
                size: compact(usage.size),
            })
        );
    }
    const cost = formatCost(usage.cost);
    if (cost) lines.push(t('chat.usage.cost', lang, { amount: cost }));

    const b = usage.breakdown;
    if (b) {
        const rows: Array<[string, number | undefined]> = [
            ['chat.usage.input', b.inputTokens],
            ['chat.usage.output', b.outputTokens],
            ['chat.usage.cacheRead', b.cachedReadTokens],
            ['chat.usage.cacheWrite', b.cachedWriteTokens],
            ['chat.usage.reasoning', b.thoughtTokens],
            ['chat.usage.total', b.totalTokens],
        ];
        for (const [key, val] of rows) {
            if (val != null) lines.push(`${t(key, lang)}: ${compact(val)}`);
        }
    }
    return lines.join('\n');
}

interface UsageBadgeProps {
    usage: SessionUsage;
}

export function UsageBadge({ usage }: UsageBadgeProps) {
    const lang = getLang();
    const pct = contextPercent(usage);
    const cost = formatCost(usage.cost);

    // Nothing worth showing (no context %, no cost) → render nothing rather
    // than an empty chip.
    if (pct == null && !cost) return null;

    // Context gauge tints toward warning as the window fills; > 90% is red.
    const level = pct == null ? undefined : pct >= 90 ? 'high' : pct >= 70 ? 'mid' : 'low';

    return (
        <span
            class="chat-usage-badge"
            data-level={level}
            title={buildTooltip(usage, lang)}
            aria-label={t('chat.usage.label', lang)}
        >
            {pct != null && (
                <span class="chat-usage-context">
                    <svg viewBox="0 0 24 24" width="12" height="12" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
                        <circle cx="12" cy="12" r="9" opacity="0.3" />
                        <path d="M12 3a9 9 0 0 1 9 9" stroke-linecap="round" />
                    </svg>
                    <span>{pct}%</span>
                </span>
            )}
            {cost && <span class="chat-usage-cost">{cost}</span>}
        </span>
    );
}
