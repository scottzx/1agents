import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal, useSignalEffect } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as taskNav from '../../../stores/taskNavStore';
import { t } from '../../../i18n';
import { ShellNav } from '../../platform/ShellNav';
import { SourceDetail } from './SourceDetail';
import { SourceHome } from './SourceHome';
import { FeishuSourcePanel, feishuTabs } from './FeishuSourcePanel';
import { AppleSourcePanel, appleTabs } from './AppleSourcePanel';
import { AddSource } from './AddSource';

// 数据源管理 (Data Source Management) — organized **by data source**. The landing
// page is a grid of source cards (飞书 / Apple / 添加). Picking one drills in: the
// breadcrumb gains a second level (数据源 › 飞书) and the source's zones — 认证 /
// 采集配置 / 数据与历史 — show as top-nav tabs. A data card drills one level
// further into the schema-free 多维表格 (SourceDetail).
type SourceId = 'feishu' | 'apple' | 'add';
type Detail = { source: string; kind: string; title: string };

const SOURCE_LABEL: Record<SourceId, string> = { feishu: '飞书', apple: 'Apple', add: '' };

export function DataSourcesPane() {
    const language = ui.language.value;
    const source = useSignal<SourceId | null>(null); // null = home
    const subTab = useSignal<string>('config');
    const detail = useSignal<Detail | null>(null);

    const goHome = () => {
        source.value = null;
        detail.value = null;
    };
    const enterSource = (id: string) => {
        const sid = id as SourceId;
        source.value = sid;
        subTab.value = sid === 'apple' ? 'auth' : 'config';
        detail.value = null;
    };
    const clearDetail = () => (detail.value = null);
    const openData = (s: string, kind: string, title: string) => {
        detail.value = { source: s, kind, title };
    };

    // Publish the drill breadcrumb into the global WorkspaceHeader; clear on unmount.
    useSignalEffect(() => {
        const home = { label: t('header.title.datasources', language), onClick: goHome };
        if (source.value === null) {
            taskNav.headerCrumbs.value = [{ label: t('header.title.datasources', language) }];
            return;
        }
        const crumbs: { label: string; onClick?: () => void }[] = [home];
        if (source.value === 'add') {
            crumbs.push({ label: t('datasource.tab.add', language) });
        } else {
            crumbs.push({
                label: SOURCE_LABEL[source.value],
                onClick: detail.value ? clearDetail : undefined,
            });
            if (detail.value) crumbs.push({ label: detail.value.title });
        }
        taskNav.headerCrumbs.value = crumbs;
    });
    useEffect(() => () => void (taskNav.headerCrumbs.value = null), []);

    const zoneTabs =
        source.value === 'feishu' ? feishuTabs(language) : source.value === 'apple' ? appleTabs(language) : [];

    return (
        <div class="datasource-pane">
            {source.value === null ? (
                <div class="datasource-tab-body">
                    <SourceHome onPick={enterSource} />
                </div>
            ) : source.value === 'add' ? (
                <div class="datasource-tab-body">
                    <AddSource onGoConfig={() => enterSource('feishu')} />
                </div>
            ) : detail.value ? (
                <div class="datasource-tab-body">
                    <SourceDetail
                        source={detail.value.source}
                        kind={detail.value.kind}
                        title={detail.value.title}
                        onBack={clearDetail}
                    />
                </div>
            ) : (
                <Fragment>
                    <ShellNav tabs={zoneTabs} activeTab={subTab.value} onSelectTab={id => (subTab.value = id)} />
                    <div class="datasource-tab-body">
                        {source.value === 'feishu' ? (
                            <FeishuSourcePanel tab={subTab.value} onOpenData={openData} />
                        ) : (
                            <AppleSourcePanel tab={subTab.value} onOpenData={openData} />
                        )}
                    </div>
                </Fragment>
            )}
        </div>
    );
}
