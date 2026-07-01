import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal, useSignalEffect } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as taskNav from '../../../stores/taskNavStore';
import { t } from '../../../i18n';
import { ShellNav, type ShellTab } from '../../platform/ShellNav';
import { SourceList } from './SourceList';
import { SourceDetail } from './SourceDetail';
import { ManagePanel } from './ManagePanel';
import { AddSource } from './AddSource';

// 数据源管理 (Data Source Management) — a full-page section (sidebar, above 系统
// 设置). Uses the shared ShellNav (breadcrumb + tab bar) so it reads like the
// project detail page: breadcrumb 概览 › <record type>, level-2 tabs 已获取数据 /
// 配置数据源 / 新增数据源. The section title (数据源) is the workspace-header.
type Tab = 'data' | 'config' | 'add';
type Detail = { source: string; kind: string; title: string };

export function DataSourcesPane() {
    const language = ui.language.value;
    const tab = useSignal<Tab>('data');
    const detail = useSignal<Detail | null>(null);

    const overview = () => (detail.value = null);
    const setTab = (id: string) => {
        tab.value = id as Tab;
        if (id !== 'data') detail.value = null;
    };

    // The drill breadcrumb lives in the global WorkspaceHeader now (数据源 /
    // 数据源 › <record>), not in a second bar here. Publish it on state change,
    // and clear it on unmount so switching to another full-page module resets.
    useSignalEffect(() => {
        taskNav.headerCrumbs.value = detail.value
            ? [{ label: t('header.title.datasources', language), onClick: overview }, { label: detail.value.title }]
            : [{ label: t('header.title.datasources', language) }];
    });
    useEffect(() => () => void (taskNav.headerCrumbs.value = null), []);

    const tabs: ShellTab[] = [
        { id: 'data', label: t('datasource.tab.data', language) },
        { id: 'config', label: t('datasource.tab.config', language) },
        { id: 'add', label: t('datasource.tab.add', language) },
    ];

    return (
        <div class="datasource-pane">
            <ShellNav tabs={tabs} activeTab={tab.value} onSelectTab={setTab} />

            <div class="datasource-tab-body">
                {tab.value === 'data' &&
                    (detail.value ? (
                        <SourceDetail
                            source={detail.value.source}
                            kind={detail.value.kind}
                            title={detail.value.title}
                            onBack={overview}
                        />
                    ) : (
                        <SourceList onOpen={(source, kind, title) => (detail.value = { source, kind, title })} />
                    ))}
                {tab.value === 'config' && <ManagePanel />}
                {tab.value === 'add' && <AddSource onGoConfig={() => (tab.value = 'config')} />}
            </div>
        </div>
    );
}
