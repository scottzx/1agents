import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SourceSummary, type SourceAccount } from '@1agents/core/services/sourceService';

// SourceHome — the 数据源 landing page, 源为中心: one Bento card per registered
// account (厂家 + 账号 = 一个源), so google+账号A and google+账号B show as two
// separate cards, plus an "添加数据源" card. Each card rolls up its vendor's bronze
// (record total + last fetch). Picking a card drills into that source.

const VENDOR_UI: Record<string, { icon: string; color: string; descKey: string }> = {
    icloud: { icon: '🍎', color: '#555', descKey: 'datasource.home.appleDesc' },
    feishu: { icon: '💬', color: '#3370ff', descKey: 'datasource.home.feishuDesc' },
    microsoft: { icon: '🪟', color: '#2f6feb', descKey: 'datasource.src.microsoftDesc' },
    google: { icon: '🔎', color: '#ea4335', descKey: 'datasource.src.googleDesc' },
};

function rollup(summaries: SourceSummary[], accountId: string): { count: number; lastFetchedAt: number } {
    let count = 0;
    let lastFetchedAt = 0;
    for (const s of summaries) {
        if (s.accountId !== accountId) continue;
        count += s.count;
        if (s.lastFetchedAt > lastFetchedAt) lastFetchedAt = s.lastFetchedAt;
    }
    return { count, lastFetchedAt };
}

function fmtTime(ms: number, language: Lang): string {
    const d = new Date(ms);
    return Number.isNaN(d.getTime()) ? '' : d.toLocaleString(language);
}

export function SourceHome({
    accounts,
    onPick,
    onAdd,
    onDelete,
}: {
    accounts: SourceAccount[];
    onPick: (account: SourceAccount) => void;
    onAdd: () => void;
    onDelete: (account: SourceAccount) => void;
}) {
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

    const stats = (accountId: string) => {
        if (summaries === null) return null;
        const { count, lastFetchedAt } = rollup(summaries, accountId);
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
                {accounts.map(a => {
                    const meta = VENDOR_UI[a.vendor] ?? { icon: '🔌', color: '#888', descKey: '' };
                    return (
                        <button key={a.id} class="bento-card source-home-card" onClick={() => onPick(a)}>
                            <div class="bento-zone-header">
                                <div
                                    class="bento-card-icon source-home-icon"
                                    style={`background-color:${meta.color}1a;color:${meta.color};`}
                                >
                                    {meta.icon}
                                </div>
                                <span
                                    class="source-home-del"
                                    role="button"
                                    title={t('datasource.account.delete', language)}
                                    onClick={e => {
                                        e.stopPropagation();
                                        if (window.confirm(t('datasource.account.deleteConfirm', language)))
                                            onDelete(a);
                                    }}
                                >
                                    ✕
                                </span>
                            </div>
                            <div class="bento-zone-body">
                                <h3 class="bento-card-title">{a.label}</h3>
                                <p class="bento-card-desc">
                                    <span class="datasource-card-badge muted">
                                        {t(`datasource.region.${a.region}`, language) || a.region}
                                    </span>{' '}
                                    {meta.descKey ? t(meta.descKey, language) : ''}
                                </p>
                                {stats(a.id)}
                            </div>
                            <div class="bento-zone-footer">
                                <span class="card-action-text">{t('datasource.home.manage', language)} →</span>
                            </div>
                        </button>
                    );
                })}
                <button class="bento-card source-home-card source-home-add" onClick={onAdd}>
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
