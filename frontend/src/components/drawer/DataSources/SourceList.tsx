import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { sourceService, type SourceSummary } from '@1agents/core/services/sourceService';

// 数据源列表 (data-source overview) — sources grouped by 大类 (Apple / 飞书), each
// with a card per data type (联系人 / 待办 / 日历 / 群消息). A 'live' card carries a
// real bronze rollup (count + last sync) and opens the 多维表格 detail; types not
// yet ingested show a roadmap badge. Top-right opens 管理数据源.

type Status = 'live' | 'phase2' | 'phase3' | 'managed';

interface TypeEntry {
    source: string;
    kind: string;
    labelKey: string;
    icon: string;
    status: Status;
}

interface Category {
    key: string;
    labelKey: string;
    entries: TypeEntry[];
}

// Roadmap-driven scaffold: everything shows so the shape is visible, but only
// 'live' entries (already in bronze) are interactive. todo/event = #359 Phase 2;
// feishu/message = Phase 3; imessage data isn't in bronze yet (managed only).
const CATEGORIES: Category[] = [
    {
        key: 'apple',
        labelKey: 'datasource.cat.apple',
        entries: [
            { source: 'icloud', kind: 'contact', labelKey: 'datasource.type.contacts', icon: '📇', status: 'live' },
            { source: 'icloud', kind: 'todo', labelKey: 'datasource.type.todos', icon: '✅', status: 'phase2' },
            { source: 'icloud', kind: 'event', labelKey: 'datasource.type.calendar', icon: '📅', status: 'phase2' },
            { source: 'apple', kind: 'imessage', labelKey: 'datasource.type.imessage', icon: '💬', status: 'managed' },
        ],
    },
    {
        key: 'feishu',
        labelKey: 'datasource.cat.feishu',
        entries: [
            {
                source: 'feishu',
                kind: 'message',
                labelKey: 'datasource.type.feishuMessages',
                icon: '💬',
                status: 'phase3',
            },
        ],
    },
];

function fmtTime(ms: number, lang: string): string {
    if (!ms) return '';
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(lang);
}

export function SourceList({ onOpen }: { onOpen: (source: string, kind: string, title: string) => void }) {
    const language = ui.language.value;
    const [summaries, setSummaries] = useState<Record<string, SourceSummary>>({});
    const [error, setError] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .summary()
            .then(list => {
                if (!active) return;
                const map: Record<string, SourceSummary> = {};
                for (const s of list) map[`${s.source}/${s.kind}`] = s;
                setSummaries(map);
            })
            .catch(err => active && setError((err as Error).message));
        return () => {
            active = false;
        };
    }, []);

    const badgeFor = (status: Status): { text: string; cls: string } | null => {
        switch (status) {
            case 'phase2':
                return { text: t('datasource.phase2', language), cls: 'warn' };
            case 'phase3':
                return { text: t('datasource.phase3', language), cls: 'warn' };
            case 'managed':
                return { text: t('datasource.managed', language), cls: 'muted' };
            default:
                return null;
        }
    };

    const renderCard = (e: TypeEntry) => {
        const label = t(e.labelKey, language);
        const sum = summaries[`${e.source}/${e.kind}`];
        const badge = badgeFor(e.status);
        const clickable = e.status === 'live';
        const count = sum?.count ?? 0;
        const last = sum ? fmtTime(sum.lastFetchedAt, language) : '';

        return (
            <div
                key={`${e.source}/${e.kind}`}
                class={`datasource-card${clickable ? ' clickable' : ' disabled'}`}
                onClick={clickable ? () => onOpen(e.source, e.kind, label) : undefined}
            >
                <div class="datasource-card-top">
                    <span class="datasource-card-icon" aria-hidden="true">
                        {e.icon}
                    </span>
                    <span class="datasource-card-title">{label}</span>
                    {badge && <span class={`datasource-card-badge ${badge.cls}`}>{badge.text}</span>}
                </div>
                {clickable ? (
                    <div class="datasource-card-body">
                        <span class="datasource-card-count">{t('datasource.records', language, { n: count })}</span>
                        <span class="datasource-card-sub">
                            {last ? `${t('datasource.lastSync', language)} ${last}` : t('datasource.never', language)}
                        </span>
                    </div>
                ) : (
                    <div class="datasource-card-body">
                        <span class="datasource-card-sub">{t('datasource.notReady', language)}</span>
                    </div>
                )}
            </div>
        );
    };

    return (
        <div class="datasource-list">
            <div class="datasource-head-hint">{t('datasource.overviewHint', language)}</div>

            {error && <div class="contacts-error">{error}</div>}

            {CATEGORIES.map(cat => (
                <Fragment key={cat.key}>
                    <div class="datasource-cat-head">{t(cat.labelKey, language)}</div>
                    <div class="datasource-grid bento-grid">{cat.entries.map(renderCard)}</div>
                </Fragment>
            ))}
        </div>
    );
}
