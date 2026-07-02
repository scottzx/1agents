import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SourceSummary } from '@1agents/core/services/sourceService';

// SourceHome — the 数据源 landing page: one Bento card per data source (飞书 /
// Apple) plus an "添加数据源" card. Each card rolls up its bronze sources (record
// total + last fetch) so the landing page reflects live state, not just names.
// Picking a card drills into that source (the breadcrumb gains a second level
// and the source's zones show as top-nav tabs).

interface SourceEntry {
    id: string;
    nameKey: string;
    descKey: string;
    icon: string;
    color: string;
    /** bronze source ids this card rolls up (Apple spans icloud + apple). */
    bronzeIds: string[];
}

const SOURCES: SourceEntry[] = [
    {
        id: 'feishu',
        nameKey: 'datasource.cat.feishu',
        descKey: 'datasource.home.feishuDesc',
        icon: '💬',
        color: '#3370ff',
        bronzeIds: ['feishu'],
    },
    {
        id: 'apple',
        nameKey: 'datasource.cat.apple',
        descKey: 'datasource.home.appleDesc',
        icon: '',
        color: '#555',
        bronzeIds: ['icloud', 'apple'],
    },
];

function rollup(summaries: SourceSummary[], bronzeIds: string[]): { count: number; lastFetchedAt: number } {
    let count = 0;
    let lastFetchedAt = 0;
    for (const s of summaries) {
        if (!bronzeIds.includes(s.source)) continue;
        count += s.count;
        if (s.lastFetchedAt > lastFetchedAt) lastFetchedAt = s.lastFetchedAt;
    }
    return { count, lastFetchedAt };
}

function fmtTime(ms: number, language: Lang): string {
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

export function SourceHome({ onPick }: { onPick: (id: string) => void }) {
    const language = ui.language.value;
    const [summaries, setSummaries] = useState<SourceSummary[] | null>(null);

    useEffect(() => {
        let active = true;
        sourceService
            .summary()
            .then(list => active && setSummaries(list))
            .catch(() => active && setSummaries([])); // stats are decorative — cards stay usable
        return () => {
            active = false;
        };
    }, []);

    const stats = (s: SourceEntry) => {
        if (summaries === null) return null; // still loading — omit the line, no flash
        const { count, lastFetchedAt } = rollup(summaries, s.bronzeIds);
        if (count === 0) return <span class="source-home-stats muted">{t('datasource.never', language)}</span>;
        return (
            <span class="source-home-stats">
                {t('datasource.records', language, { n: count })}
                {lastFetchedAt > 0 &&
                    ` · ${t('datasource.data.lastFetched', language)} ${fmtTime(lastFetchedAt, language)}`}
            </span>
        );
    };

    return (
        <div class="source-home">
            <div class="bento-grid">
                {SOURCES.map(s => (
                    <button key={s.id} class="bento-card source-home-card" onClick={() => onPick(s.id)}>
                        <div class="bento-zone-header">
                            <div
                                class="bento-card-icon source-home-icon"
                                style={`background-color:${s.color}1a;color:${s.color};`}
                            >
                                {s.icon}
                            </div>
                        </div>
                        <div class="bento-zone-body">
                            <h3 class="bento-card-title">{t(s.nameKey, language)}</h3>
                            <p class="bento-card-desc">{t(s.descKey, language)}</p>
                            {stats(s)}
                        </div>
                        <div class="bento-zone-footer">
                            <span class="card-action-text">{t('datasource.home.manage', language)} →</span>
                        </div>
                    </button>
                ))}
                <button class="bento-card source-home-card source-home-add" onClick={() => onPick('add')}>
                    <div class="bento-zone-body source-home-add-body">
                        <span class="source-home-add-plus">+</span>
                        <h3 class="bento-card-title">{t('datasource.tab.add', language)}</h3>
                        <p class="bento-card-desc">{t('datasource.home.addDesc', language)}</p>
                    </div>
                </button>
            </div>
        </div>
    );
}
