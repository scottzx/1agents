import { h } from 'preact';
import { useState } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import type { SourceAccount } from '@1agents/core/services/sourceService';
import { SourceAuthZone } from './SourceAuthZone';
import { ScheduleList } from './ScheduleList';
import { TaskRunsGrid } from './TaskRunsGrid';
import { SourceDataZone } from './SourceDataZone';
import { SourceSilverZone } from './SourceSilverZone';

// SourcePanel — the single, source-agnostic panel for every 数据源. It replaces
// the per-vendor FeishuSourcePanel / AppleSourcePanel / SourceInstancePanel: one
// tab set (认证 · 配置 · 数据) that only renders data, driven by the account +
// its authKind. Vendor specifics live inside their zone (auth by authKind; the
// 群/iMessage extras inside those zones), never in this shell.
//
//   认证 → SourceAuthZone (authKind → oauth / cli / credentials)
//   配置 → 定时任务: ScheduleList (采集项 + 触发状态) | 执行情况: TaskRunsGrid (工单运行)
//   数据 → SourceDataZone 卡片 → SourceDetail 多维表格 (via onOpenData)
//   治理 → SourceSilverZone 卡片 → SilverDetail 多维表格 (via onOpenSilver);
//          silver 单表治理,跟随本源定时同步增量修正
export function sourceTabs(language: Lang): ShellTab[] {
    return [
        { id: 'auth', label: t('datasource.zone.auth', language) },
        { id: 'config', label: t('datasource.tab.config', language) },
        { id: 'data', label: t('datasource.data.title', language) },
        { id: 'silver', label: t('datasource.zone.silver', language) },
    ];
}

// dataSourcesFor maps a vendor to the bronze source(s) its data tab reads.
// Apple's account holds iCloud (CardDAV) contacts while iMessage lands under the
// machine-shared "apple" source.
function dataSourcesFor(vendor: string): { sources: string[]; sharedSources?: string[] } {
    if (vendor === 'icloud') return { sources: ['icloud'], sharedSources: ['apple'] };
    return { sources: [vendor] };
}

export function SourcePanel({
    account,
    authKind,
    tab,
    onOpenData,
    onOpenSilver,
}: {
    account: SourceAccount;
    authKind: string;
    tab: string;
    onOpenData: (source: string, kind: string, title: string, account?: string) => void;
    onOpenSilver: (domain: string, title: string) => void;
}) {
    const language = ui.language.value;
    const data = dataSourcesFor(account.vendor);

    return (
        <div class="source-panel">
            {tab === 'auth' && <SourceAuthZone account={account} authKind={authKind} />}

            {tab === 'config' && <SourceConfigTab source={account.vendor} language={language} />}

            {tab === 'data' && (
                <SourceDataZone
                    sources={data.sources}
                    sharedSources={data.sharedSources}
                    account={account.id}
                    onOpen={onOpenData}
                />
            )}

            {tab === 'silver' && <SourceSilverZone vendor={account.vendor} onOpen={onOpenSilver} />}
        </div>
    );
}

// SourceConfigTab hosts the two config subpages behind a segmented switch:
// 定时任务 (schedule definitions + trigger status) and 任务执行情况 (single-run
// history). This 定时任务/多维表格 pattern is meant to recur across the app, so it
// is built from the reusable ScheduleList / TaskRunsGrid rather than inline.
function SourceConfigTab({ source, language }: { source: string; language: Lang }) {
    const [sub, setSub] = useState<'schedule' | 'runs'>('schedule');
    return (
        <div class="source-config-tab">
            <div class="datasource-subnav">
                <button
                    class={`datasource-subnav-tab${sub === 'schedule' ? ' is-active' : ''}`}
                    onClick={() => setSub('schedule')}
                >
                    {t('datasource.config.sub.schedule', language)}
                </button>
                <button
                    class={`datasource-subnav-tab${sub === 'runs' ? ' is-active' : ''}`}
                    onClick={() => setSub('runs')}
                >
                    {t('datasource.config.sub.runs', language)}
                </button>
            </div>
            {sub === 'schedule' ? (
                <ScheduleList source={source} language={language} />
            ) : (
                <TaskRunsGrid source={source} language={language} />
            )}
        </div>
    );
}
