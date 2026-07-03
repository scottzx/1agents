import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal, useSignalEffect } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as taskNav from '../../../stores/taskNavStore';
import { t } from '../../../i18n';
import { ShellNav } from '../../platform/ShellNav';
import { sourceService, type SourceAccount, type VendorSpec } from '@1agents/core/services/sourceService';
import { SourceDetail } from './SourceDetail';
import { SourceHome } from './SourceHome';
import { FeishuSourcePanel, feishuTabs } from './FeishuSourcePanel';
import { AppleSourcePanel, appleTabs } from './AppleSourcePanel';
import { SourceInstancePanel, instanceTabs } from './SourceInstancePanel';
import { AddSource } from './AddSource';

// 数据源管理 (Data Source Management) — organized 源为中心: the landing page is a
// grid of account cards (厂家 + 账号 = 一个源). Picking one drills into that source
// (breadcrumb gains a second level, the source's zones show as top-nav tabs);
// a data card drills further into the schema-free 多维表格 (SourceDetail). "添加
// 数据源" opens the vendor → region → account flow.
type Detail = { source: string; kind: string; title: string; account?: string };
// The add flow's picked vendor lives in the view (not inside AddSource) so the
// second breadcrumb level (添加数据源 → <vendor>) drives back-navigation instead of
// a bespoke in-form back button.
type View = { kind: 'home' } | { kind: 'add'; vendor?: VendorSpec } | { kind: 'account'; account: SourceAccount };

function defaultTab(vendor: string): string {
    if (vendor === 'feishu') return 'config';
    return 'auth';
}

export function DataSourcesPane() {
    const language = ui.language.value;
    const view = useSignal<View>({ kind: 'home' });
    const accounts = useSignal<SourceAccount[]>([]);
    const subTab = useSignal<string>('auth');
    const detail = useSignal<Detail | null>(null);

    const loadAccounts = () => {
        sourceService
            .accounts()
            .then(list => (accounts.value = list))
            .catch(() => (accounts.value = []));
    };
    useEffect(loadAccounts, []);

    const goHome = () => {
        view.value = { kind: 'home' };
        detail.value = null;
        loadAccounts();
    };
    const openAccount = (a: SourceAccount) => {
        view.value = { kind: 'account', account: a };
        subTab.value = defaultTab(a.vendor);
        detail.value = null;
    };
    const openAdd = () => {
        view.value = { kind: 'add' };
        detail.value = null;
    };
    const pickVendor = (v: VendorSpec) => (view.value = { kind: 'add', vendor: v });
    const backToVendors = () => (view.value = { kind: 'add' });
    const onCreated = (a: SourceAccount) => {
        loadAccounts();
        openAccount(a);
    };
    const onDelete = (a: SourceAccount) => {
        sourceService.deleteAccount(a.id).then(loadAccounts).catch(loadAccounts);
    };
    const clearDetail = () => (detail.value = null);
    const openData = (s: string, kind: string, title: string, account?: string) => {
        detail.value = { source: s, kind, title, account };
    };

    // Publish the drill breadcrumb into the global WorkspaceHeader; clear on unmount.
    useSignalEffect(() => {
        const home = { label: t('header.title.datasources', language), onClick: goHome };
        const v = view.value;
        if (v.kind === 'home') {
            taskNav.headerCrumbs.value = [{ label: t('header.title.datasources', language) }];
            return;
        }
        const crumbs: { label: string; onClick?: () => void }[] = [home];
        if (v.kind === 'add') {
            crumbs.push({ label: t('datasource.tab.add', language), onClick: v.vendor ? backToVendors : undefined });
            if (v.vendor) crumbs.push({ label: v.vendor.label });
        } else {
            crumbs.push({
                label: v.account.label,
                onClick: detail.value ? clearDetail : undefined,
            });
            if (detail.value) crumbs.push({ label: detail.value.title });
        }
        taskNav.headerCrumbs.value = crumbs;
    });
    useEffect(() => () => void (taskNav.headerCrumbs.value = null), []);

    const v = view.value;
    const zoneTabs =
        v.kind === 'account'
            ? v.account.vendor === 'feishu'
                ? feishuTabs(language)
                : v.account.vendor === 'icloud'
                  ? appleTabs(language)
                  : instanceTabs(language)
            : [];

    return (
        <div class="datasource-pane">
            {v.kind === 'home' ? (
                <div class="datasource-tab-body">
                    <SourceHome accounts={accounts.value} onPick={openAccount} onAdd={openAdd} onDelete={onDelete} />
                </div>
            ) : v.kind === 'add' ? (
                <div class="datasource-tab-body">
                    <AddSource picked={v.vendor ?? null} onPick={pickVendor} onCreated={onCreated} />
                </div>
            ) : detail.value ? (
                <div class="datasource-tab-body">
                    <SourceDetail
                        source={detail.value.source}
                        kind={detail.value.kind}
                        title={detail.value.title}
                        account={detail.value.account}
                    />
                </div>
            ) : (
                <Fragment>
                    <ShellNav tabs={zoneTabs} activeTab={subTab.value} onSelectTab={id => (subTab.value = id)} />
                    <div class="datasource-tab-body">
                        {v.account.vendor === 'feishu' ? (
                            <FeishuSourcePanel tab={subTab.value} onOpenData={openData} />
                        ) : v.account.vendor === 'icloud' ? (
                            <AppleSourcePanel account={v.account} tab={subTab.value} onOpenData={openData} />
                        ) : (
                            <SourceInstancePanel account={v.account} tab={subTab.value} onOpenData={openData} />
                        )}
                    </div>
                </Fragment>
            )}
        </div>
    );
}
