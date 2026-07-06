import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SilverSummary } from '@1agents/core/services/sourceService';

// SourceSilverZone — the "已治理数据" zone of a source panel, sibling to the raw
// "已获取原始数据" zone. Silver is single-table governance (one bronze table = one
// cleaning scheme, re-run incrementally after this source's scheduled sync), so
// here it is scoped to ONE source: one Bento card per governed domain this source
// contributes (飞书 → 消息 + 联系人; icloud → 联系人; …), clicking through to the
// cleaned 多维表格 (SilverDetail).

function domainLabel(domain: string, language: Lang): string {
    const key = `datasource.silver.domain.${domain}`;
    const val = t(key, language);
    return val !== key ? val : domain;
}

function fmtTime(ms: number, language: Lang): string {
    if (!ms) return '';
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

export function SourceSilverZone({
    vendor,
    onOpen,
}: {
    vendor: string;
    onOpen: (domain: string, title: string) => void;
}) {
    const language = ui.language.value;
    // null = still loading — renders nothing instead of flashing the empty hint.
    const [rows, setRows] = useState<SilverSummary[] | null>(null);
    const [error, setError] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .silverSummary()
            .then(all => active && setRows(all.filter(r => r.source === vendor)))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [vendor]);

    return (
        <div class="fscard-zone">
            <div class="fscard-zone-title">{t('datasource.zone.silver', language)}</div>
            {error && <div class="fscard-error">{error}</div>}
            {rows !== null && rows.length === 0 && !error && (
                <div class="contacts-empty">{t('datasource.silver.zoneEmpty', language)}</div>
            )}
            {rows !== null && rows.length > 0 && (
                <div class="bento-grid fscard-data-grid">
                    {rows.map(r => {
                        const title = domainLabel(r.domain, language);
                        return (
                            <button
                                key={r.domain}
                                class="bento-card fscard-data-card"
                                onClick={() => onOpen(r.domain, title)}
                            >
                                <div class="bento-zone-header">
                                    <span class="bento-card-title">{title}</span>
                                    <span class="fscard-data-count">
                                        {t('datasource.data.records', language, { n: r.count })}
                                    </span>
                                </div>
                                <div class="bento-zone-footer">
                                    <span class="fscard-data-time">
                                        {r.lastUpdated > 0
                                            ? `${t('datasource.data.lastFetched', language)} ${fmtTime(r.lastUpdated, language)}`
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
