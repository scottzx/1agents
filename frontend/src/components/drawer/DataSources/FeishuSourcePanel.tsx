import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import { CliZone, CollectionsZone, HistoryZone } from './FeishuSourceCard';
import { SourceDataZone } from './SourceDataZone';

// FeishuSourcePanel — the 飞书 source, one zone per top-nav tab:
//   认证 → CLI 生命周期 (CliZone)
//   采集配置 → CollectionsZone. The 群 pieces live in ChatScopeModal, opened
//   from the rows: 群列表 browses the bronze cache (refresh is explicit), and
//   群消息 picks its scope from the same cache — one pull serves both, no
//   duplicate lark-cli fetching, one unified cadence at the kind level.
//   数据与历史 → SourceDataZone (raw records) + HistoryZone (work-order runs)
// The panel keeps its own state (toast / history refresh) so switching tabs
// doesn't lose it — index.tsx keeps this one instance mounted and just flips `tab`.

// feishuTabs are the top-nav tabs for the 飞书 source.
export function feishuTabs(language: Lang): ShellTab[] {
    return [
        { id: 'auth', label: t('datasource.zone.auth', language) },
        { id: 'config', label: t('datasource.tab.config', language) },
        { id: 'data', label: t('datasource.data.title', language) },
    ];
}

export function FeishuSourcePanel({
    tab,
    onOpenData,
}: {
    tab: string;
    onOpenData: (source: string, kind: string, title: string) => void;
}) {
    const language = ui.language.value;
    const [toast, setToast] = useState('');
    const [historyTick, setHistoryTick] = useState(0);

    const showToast = (msg: string) => {
        setToast(msg);
        window.setTimeout(() => setToast(''), 3000);
    };
    const onSyncDispatched = () => {
        // Give the backend a moment to record the run before HistoryZone re-fetches.
        window.setTimeout(() => setHistoryTick(n => n + 1), 800);
    };

    return (
        <div class="source-panel">
            {toast && <div class="fscard-toast">{toast}</div>}

            {tab === 'auth' && <CliZone language={language} />}

            {tab === 'config' && (
                <CollectionsZone language={language} onSyncDispatched={onSyncDispatched} onToast={showToast} />
            )}

            {tab === 'data' && (
                <Fragment>
                    <SourceDataZone sources={['feishu']} onOpen={onOpenData} />
                    <HistoryZone language={language} refreshTick={historyTick} />
                </Fragment>
            )}
        </div>
    );
}
