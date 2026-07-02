import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import { sourceService, type CollectionView } from '@1agents/core/services/sourceService';
import { SourceDataZone } from './SourceDataZone';

// SourceInstancePanel — the generic panel for OAuth-style multi-account sources
// (microsoft / google) whose real pulls are not wired yet. One zone per tab:
//   认证 → OAuth 占位 (the account is already registered; connect flow ships later)
//   采集配置 → the roadmap of crawlable kinds (all not-yet-implemented)
//   数据 → SourceDataZone over the vendor's bronze
// Apple/飞书 keep their bespoke panels; this covers the framework skeletons.

export function instanceTabs(language: Lang): ShellTab[] {
    return [
        { id: 'auth', label: t('datasource.zone.auth', language) },
        { id: 'config', label: t('datasource.tab.config', language) },
        { id: 'data', label: t('datasource.data.title', language) },
    ];
}

export function SourceInstancePanel({
    vendor,
    tab,
    onOpenData,
}: {
    vendor: string;
    tab: string;
    onOpenData: (source: string, kind: string, title: string) => void;
}) {
    const language = ui.language.value;
    const [collections, setCollections] = useState<CollectionView[] | null>(null);

    useEffect(() => {
        let active = true;
        sourceService
            .collections(vendor)
            .then(list => active && setCollections(list))
            .catch(() => active && setCollections([]));
        return () => {
            active = false;
        };
    }, [vendor]);

    return (
        <div class="source-panel">
            {tab === 'auth' && (
                <div class="contacts-privacy-banner">
                    <span class="contacts-privacy-icon" aria-hidden="true">
                        🔐
                    </span>
                    <span>{t('datasource.instance.authStub', language)}</span>
                </div>
            )}

            {tab === 'config' && (
                <div class="source-instance-config">
                    <div class="datasource-head-hint">{t('datasource.instance.roadmap', language)}</div>
                    {collections === null ? (
                        <div class="datasource-head-hint">…</div>
                    ) : (
                        <div class="bento-grid">
                            {collections.map(c => (
                                <div key={c.kind} class="bento-card sys-settings-card">
                                    <div class="bento-zone-body">
                                        <h3 class="bento-card-title">{c.label || c.kind}</h3>
                                        <p class="bento-card-desc">{c.domain}</p>
                                        <span class="datasource-card-badge warn">
                                            {t('datasource.config.notImplemented', language)}
                                        </span>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            )}

            {tab === 'data' && (
                <Fragment>
                    <SourceDataZone sources={[vendor]} onOpen={onOpenData} />
                </Fragment>
            )}
        </div>
    );
}
