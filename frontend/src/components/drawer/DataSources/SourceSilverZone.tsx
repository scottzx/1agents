import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SilverSummary } from '@1agents/core/services/sourceService';

// SourceSilverZone — the "已治理数据" zone of a source panel, sibling to the raw
// "已获取原始数据" zone. Governance is bespoke per table (一事一议、一表一议): each
// governed table turns its source's raw JSON into readable structured fields,
// preserving what's there for the later fusion step. A governed table is its own
// entity — sometimes a MERGE of several inputs (飞书联系人 = group roster + users
// mined from messages), not a mechanical 1:1 of one bronze kind — so cards are
// labeled by the governed table, not by an abstract 联系人/消息/日历/待办 domain.
// A source's (domain) maps 1:1 to one silver table, so (vendor, domain) keys it.

// (vendor, silver domain) → the governed table's label key. Most tables reuse the
// source kind's name; 飞书联系人 is a merge and gets its own name. Mirrors the
// backend silver registry (internal/data/silver_*.go) — a new source adds a line
// here just as it registers a silverTableDef there.
const SILVER_TABLE_LABEL: Record<string, Record<string, string>> = {
    feishu: {
        contacts: 'datasource.silver.feishuContacts',
        messages: 'datasource.kind.feishu_message',
        events: 'datasource.kind.feishu_calendar_event',
    },
    icloud: { contacts: 'datasource.kind.contact' },
    microsoft: {
        messages: 'datasource.kind.ms_mail',
        events: 'datasource.kind.ms_event',
        todos: 'datasource.kind.ms_todo',
    },
    agentmail: { messages: 'datasource.kind.agentmail_mail' },
};

function tableLabel(vendor: string, domain: string, language: Lang): string {
    const key = SILVER_TABLE_LABEL[vendor]?.[domain] ?? `datasource.silver.domain.${domain}`;
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
                        const title = tableLabel(vendor, r.domain, language);
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
