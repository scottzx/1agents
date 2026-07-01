import { h, type ComponentChildren } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { channelModuleService, type ChannelModule } from '@1agents/core/services/contactService';
import { ICloudSection } from './ICloudSection';
import { IMessageSection } from './IMessageSection';
import { FeishuSection } from './FeishuSection';

// 渠道 tab — data sources grouped by channel (Apple / 飞书), each with sub-modules.
// Every sub-module is privacy-gated: it requires explicit user consent before any
// sync, and all crawling is done by deterministic Go syncers (the "代码化 · AI 不
// 参与" badge), never by an AI. WeChat-class sources stay outside the binary,
// writing the same schema via the neutral import path.

const MOD_ICLOUD = 'icloud.contacts';
const MOD_IMESSAGE = 'apple.imessage';
const MOD_FEISHU = 'feishu.groups';

export function ChannelsTab() {
    const language = ui.language.value;
    const [modules, setModules] = useState<Record<string, ChannelModule>>({});
    const [error, setError] = useState('');

    const refresh = useCallback(async () => {
        try {
            const list = await channelModuleService.list();
            const map: Record<string, ChannelModule> = {};
            for (const m of list) map[m.id] = m;
            setModules(map);
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const gate = (id: string, render: (m: ChannelModule) => ComponentChildren) => {
        const m = modules[id];
        if (!m) return <div class="contacts-empty">…</div>;
        if (!m.consented) {
            return (
                <div class="contacts-consent-gate">
                    <p>{t('contacts.privacy.moduleNotice', language)}</p>
                    <button
                        class="contacts-btn contacts-btn-primary contacts-btn-sm"
                        onClick={async () => {
                            await channelModuleService.consent(id);
                            await refresh();
                        }}
                    >
                        {t('contacts.privacy.authorize', language)}
                    </button>
                </div>
            );
        }
        return render(m);
    };

    const submodule = (id: string, title: string, hint: string, render: (m: ChannelModule) => ComponentChildren) => (
        <div class="contacts-submodule">
            <div class="contacts-submodule-head">
                <span class="contacts-submodule-title">{title}</span>
                <span class="contacts-submodule-hint">{hint}</span>
                {modules[id]?.consented && (
                    <button
                        class="contacts-linkbtn"
                        onClick={async () => {
                            await channelModuleService.revoke(id);
                            await refresh();
                        }}
                    >
                        {t('contacts.privacy.revoke', language)}
                    </button>
                )}
            </div>
            {gate(id, render)}
        </div>
    );

    return (
        <div class="contacts-channels">
            <div class="contacts-privacy-banner">
                <span class="contacts-privacy-icon" aria-hidden="true">
                    🔒
                </span>
                <span>{t('contacts.privacy.notice', language)}</span>
            </div>

            {error && <div class="contacts-error">{error}</div>}

            <div class="contacts-channel-group">
                <div class="contacts-channel-head">
                    <span class="contacts-channel-name">Apple</span>
                    <span class="contacts-code-badge">{t('contacts.channels.codeBadge', language)}</span>
                </div>
                {submodule(MOD_ICLOUD, t('contacts.sub.contacts', language), 'CardDAV', () => (
                    <ICloudSection />
                ))}
                {submodule(MOD_IMESSAGE, t('contacts.sub.imessage', language), 'chat.db', m => (
                    <IMessageSection module={m} onChange={refresh} />
                ))}
            </div>

            <div class="contacts-channel-group">
                <div class="contacts-channel-head">
                    <span class="contacts-channel-name">{t('contacts.channels.group.feishu', language)}</span>
                    <span class="contacts-code-badge">{t('contacts.channels.codeBadge', language)}</span>
                </div>
                {submodule(MOD_FEISHU, t('contacts.sub.feishuGroups', language), 'lark-cli', () => (
                    <FeishuSection />
                ))}
            </div>

            <div class="contacts-channel-placeholder">
                {t('contacts.channels.wechatPlaceholder', language)}
                <span class="contacts-channels-badge int">{t('contacts.channels.comingSoon', language)}</span>
            </div>
        </div>
    );
}
