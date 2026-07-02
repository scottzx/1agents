import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import { CliZone, CollectionsZone, HistoryZone } from './FeishuSourceCard';
import { FeishuSection } from './FeishuSection';
import { useChannelModules, ConsentSubmodule } from './ConsentGate';
import { SourceDataZone } from './SourceDataZone';

// FeishuSourcePanel — the 飞书 source, one zone per top-nav tab:
//   认证 → CLI 生命周期 (CliZone)
//   采集配置 → CollectionsZone + 群选择 (consent-gated FeishuSection)
//   数据与历史 → SourceDataZone (raw records) + HistoryZone (work-order runs)
// The panel keeps its own state (toast / history refresh) so switching tabs
// doesn't lose it — index.tsx keeps this one instance mounted and just flips `tab`.

const MOD_FEISHU = 'feishu.groups';

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
    const consent = useChannelModules();

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
                <Fragment>
                    <CollectionsZone language={language} onSyncDispatched={onSyncDispatched} onToast={showToast} />
                    <div class="fscard-zone">
                        <div class="fscard-zone-title">{t('datasource.zone.groups', language)}</div>
                        {consent.error && <div class="contacts-error">{consent.error}</div>}
                        <ConsentSubmodule
                            id={MOD_FEISHU}
                            title={t('contacts.sub.feishuGroups', language)}
                            hint="lark-cli"
                            consent={consent}
                            render={() => <FeishuSection />}
                        />
                    </div>
                </Fragment>
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
