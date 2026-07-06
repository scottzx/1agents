import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal, useSignalEffect } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import * as taskNav from '../../../stores/taskNavStore';
import { t } from '../../../i18n';
import { ShellNav } from '../../platform/ShellNav';
import { sourceService, type SourceAccount, type VendorSpec } from '@1agents/core/services/sourceService';
import { SourceDetail } from './SourceDetail';
import { SilverDetail } from './SilverDetail';
import { SourceHome } from './SourceHome';
import { SourcePanel, sourceTabs } from './SourcePanel';
import { AddSource } from './AddSource';
import { GoldView } from './GoldView';

// 数据源管理 (Data Source Management) — two medallion layers switch at the top:
//   数据接入 (bronze) — 源为中心 landing grid → source drill → 原始/已治理 zones
//   数据融合 (gold)   — 跨源融合视图 (placeholder until #400 lands)
// Silver is no longer a top layer: it merged into bronze as the 已治理数据 zone
// tab (single-table governance, one bronze table = one cleaning scheme, re-run
// incrementally after that source's scheduled sync). Only bronze drills (home →
// account → 原始/治理 detail); gold is a single screen. Each layer keeps its own
// state, so switching away and back is seamless.
type Layer = 'bronze' | 'gold';
// The drill target discriminates by stage: bronze opens a raw (source, kind)
// table; silver opens a governed (source, domain) table. Both carry a title so
// the breadcrumb's "push detail.title" logic works unchanged.
type Detail =
    | { stage: 'bronze'; source: string; kind: string; title: string; account?: string }
    | { stage: 'silver'; domain: string; source: string; title: string };
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
    const layer = useSignal<Layer>('bronze');
    const view = useSignal<View>({ kind: 'home' });
    const accounts = useSignal<SourceAccount[]>([]);
    // vendor → authKind, so the unified SourcePanel drives its 认证 zone off the
    // backend capability table instead of per-vendor branching.
    const vendorAuth = useSignal<Record<string, string>>({});
    const subTab = useSignal<string>('auth');
    const detail = useSignal<Detail | null>(null);

    const loadAccounts = () => {
        sourceService
            .accounts()
            .then(list => (accounts.value = list))
            .catch(() => (accounts.value = []));
    };
    useEffect(loadAccounts, []);
    useEffect(() => {
        sourceService
            .vendors()
            .then(list => (vendorAuth.value = Object.fromEntries(list.map(v => [v.vendor, v.authKind]))))
            .catch(() => (vendorAuth.value = {}));
    }, []);

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
        detail.value = { stage: 'bronze', source: s, kind, title, account };
    };
    const openSilver = (domain: string, title: string) => {
        if (view.value.kind !== 'account') return;
        detail.value = { stage: 'silver', domain, source: view.value.account.vendor, title };
    };

    // Publish the drill breadcrumb into the global WorkspaceHeader; clear on unmount.
    // Only bronze drills — silver/gold are single screens the layer switch already
    // labels, so they show just the root crumb.
    useSignalEffect(() => {
        const home = { label: t('header.title.datasources', language), onClick: goHome };
        if (layer.value !== 'bronze') {
            taskNav.headerCrumbs.value = [{ label: t('header.title.datasources', language) }];
            return;
        }
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

    const layers: { id: Layer; label: string }[] = [
        { id: 'bronze', label: t('datasource.layer.bronze', language) },
        { id: 'gold', label: t('datasource.layer.gold', language) },
    ];

    const v = view.value;
    const zoneTabs = layer.value === 'bronze' && v.kind === 'account' ? sourceTabs(language) : [];

    return (
        <div class="datasource-pane">
            <div class="datasource-layer-switch">
                {layers.map(l => (
                    <button
                        key={l.id}
                        class={`datasource-subnav-tab${layer.value === l.id ? ' is-active' : ''}`}
                        onClick={() => (layer.value = l.id)}
                    >
                        {l.label}
                    </button>
                ))}
            </div>

            {layer.value === 'gold' ? (
                <div class="datasource-tab-body">
                    <GoldView />
                </div>
            ) : v.kind === 'home' ? (
                <div class="datasource-tab-body">
                    <SourceHome accounts={accounts.value} onPick={openAccount} onAdd={openAdd} onDelete={onDelete} />
                </div>
            ) : v.kind === 'add' ? (
                <div class="datasource-tab-body">
                    <AddSource picked={v.vendor ?? null} onPick={pickVendor} onCreated={onCreated} />
                </div>
            ) : detail.value ? (
                <div class="datasource-tab-body">
                    {detail.value.stage === 'silver' ? (
                        <SilverDetail
                            domain={detail.value.domain}
                            source={detail.value.source}
                            title={detail.value.title}
                        />
                    ) : (
                        <SourceDetail
                            source={detail.value.source}
                            kind={detail.value.kind}
                            title={detail.value.title}
                            account={detail.value.account}
                        />
                    )}
                </div>
            ) : (
                <Fragment>
                    <ShellNav tabs={zoneTabs} activeTab={subTab.value} onSelectTab={id => (subTab.value = id)} />
                    <div class="datasource-tab-body">
                        <SourcePanel
                            account={v.account}
                            authKind={vendorAuth.value[v.account.vendor] ?? ''}
                            tab={subTab.value}
                            onOpenData={openData}
                            onOpenSilver={openSilver}
                        />
                    </div>
                </Fragment>
            )}
        </div>
    );
}
