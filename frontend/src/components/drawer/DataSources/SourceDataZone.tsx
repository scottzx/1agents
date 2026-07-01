import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SourceSummary } from '@1agents/core/services/sourceService';

// SourceDataZone — the "已获取原始数据" zone of a source panel. It rolls up the
// bronze source_records for the given source(s) and renders one Bento card per
// (source, kind): record count + last-fetched time, clicking through to the
// schema-free 多维表格 (SourceDetail). This is zone 3 of the source-centric
// redesign, shared by every source panel.

// kindLabel resolves a friendly title for a (source, kind), via the
// datasource.kind.<kind> dict with a graceful fallback to the raw kind.
function kindLabel(kind: string, language: Lang): string {
    const key = `datasource.kind.${kind}`;
    const val = t(key, language);
    return val !== key ? val : kind;
}

function fmtTime(ms: number, language: Lang): string {
    if (!ms) return '';
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

export function SourceDataZone({
    sources,
    onOpen,
}: {
    sources: string[];
    onOpen: (source: string, kind: string, title: string) => void;
}) {
    const language = ui.language.value;
    const [rows, setRows] = useState<SourceSummary[]>([]);
    const [error, setError] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .summary()
            .then(all => {
                if (active) setRows(all.filter(s => sources.includes(s.source)));
            })
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [sources.join(',')]);

    return (
        <div class="fscard-zone">
            <div class="fscard-zone-title">{t('datasource.data.title', language)}</div>
            {error && <div class="fscard-error">{error}</div>}
            {rows.length === 0 && !error && <div class="contacts-empty">{t('datasource.data.empty', language)}</div>}
            {rows.length > 0 && (
                <div class="bento-grid fscard-data-grid">
                    {rows.map(r => {
                        const title = kindLabel(r.kind, language);
                        return (
                            <button
                                key={`${r.source}:${r.kind}`}
                                class="bento-card fscard-data-card"
                                onClick={() => onOpen(r.source, r.kind, title)}
                            >
                                <div class="bento-zone-header">
                                    <span class="bento-card-title">{title}</span>
                                    <span class="fscard-data-count">
                                        {t('datasource.data.records', language, { n: r.count })}
                                    </span>
                                </div>
                                <div class="bento-zone-footer">
                                    <span class="fscard-data-time">
                                        {r.lastFetchedAt > 0
                                            ? `${t('datasource.data.lastFetched', language)} ${fmtTime(r.lastFetchedAt, language)}`
                                            : ''}
                                    </span>
                                    <span class="fscard-data-open">{t('datasource.data.open', language)} →</span>
                                </div>
                            </button>
                        );
                    })}
                </div>
            )}
        </div>
    );
}
