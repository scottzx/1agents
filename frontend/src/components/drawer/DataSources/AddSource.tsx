import { h, Fragment } from 'preact';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';

// 新增数据源 (Add data source) — a gallery of connectable source types. Connected
// ones (Apple / 飞书) route to 配置数据源; the rest are roadmap placeholders. Adding
// a source here means connecting an account/credential for that type; the actual
// per-module credential + sync config lives in 配置数据源.
type SrcStatus = 'connected' | 'soon' | 'external';

interface SrcType {
    key: string;
    nameKey: string;
    descKey: string;
    icon: string;
    status: SrcStatus;
}

const SOURCE_TYPES: SrcType[] = [
    {
        key: 'apple',
        nameKey: 'datasource.cat.apple',
        descKey: 'datasource.src.appleDesc',
        icon: '',
        status: 'connected',
    },
    {
        key: 'feishu',
        nameKey: 'datasource.cat.feishu',
        descKey: 'datasource.src.feishuDesc',
        icon: '💬',
        status: 'connected',
    },
    {
        key: 'microsoft',
        nameKey: 'datasource.src.microsoft',
        descKey: 'datasource.src.microsoftDesc',
        icon: '🪟',
        status: 'soon',
    },
    {
        key: 'google',
        nameKey: 'datasource.src.google',
        descKey: 'datasource.src.googleDesc',
        icon: '🔎',
        status: 'soon',
    },
    {
        key: 'wechat',
        nameKey: 'datasource.src.wechat',
        descKey: 'datasource.src.wechatDesc',
        icon: '🟢',
        status: 'external',
    },
];

export function AddSource({ onGoConfig }: { onGoConfig: () => void }) {
    const language = ui.language.value;

    const badge = (s: SrcStatus): { text: string; cls: string } => {
        switch (s) {
            case 'connected':
                return { text: t('datasource.add.connected', language), cls: 'ok' };
            case 'external':
                return { text: t('datasource.add.external', language), cls: 'muted' };
            default:
                return { text: t('datasource.add.soon', language), cls: 'warn' };
        }
    };

    return (
        <div class="datasource-list">
            <div class="datasource-head-hint">{t('datasource.add.hint', language)}</div>
            <div class="datasource-grid bento-grid">
                {SOURCE_TYPES.map(s => {
                    const b = badge(s.status);
                    return (
                        <div key={s.key} class="datasource-card">
                            <div class="datasource-card-top">
                                <span class="datasource-card-icon" aria-hidden="true">
                                    {s.icon || '🍎'}
                                </span>
                                <span class="datasource-card-title">{t(s.nameKey, language)}</span>
                                <span class={`datasource-card-badge ${b.cls}`}>{b.text}</span>
                            </div>
                            <div class="datasource-card-body">
                                <span class="datasource-card-sub">{t(s.descKey, language)}</span>
                                {s.status === 'connected' ? (
                                    <button
                                        class="contacts-btn contacts-btn-sm datasource-src-btn"
                                        onClick={onGoConfig}
                                    >
                                        {t('datasource.add.goConfig', language)}
                                    </button>
                                ) : (
                                    <Fragment />
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
