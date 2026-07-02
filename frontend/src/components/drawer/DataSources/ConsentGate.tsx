import { h, type ComponentChildren } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { channelModuleService, type ChannelModule } from '@1agents/core/services/contactService';

// Shared privacy-consent gate for source sub-modules. Every sensitive sub-module
// (iCloud contacts, iMessage, Feishu groups) requires explicit user consent
// before any deterministic Go syncer runs — the "代码化 · AI 不参与" promise. This
// is extracted from the old ManagePanel so each source-centric panel can reuse it.

export interface ConsentModules {
    modules: Record<string, ChannelModule>;
    error: string;
    refresh: () => Promise<void>;
}

// useChannelModules loads the consent/rules state for every channel sub-module.
export function useChannelModules(): ConsentModules {
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

    return { modules, error, refresh };
}

// ConsentSubmodule renders one consent-gated sub-module: a header (title + hint +
// revoke), then either the authorize gate or the sub-module body once consented.
export function ConsentSubmodule({
    id,
    title,
    hint,
    consent,
    render,
}: {
    id: string;
    title: string;
    hint: string;
    consent: ConsentModules;
    render: (m: ChannelModule) => ComponentChildren;
}) {
    const language = ui.language.value;
    const m = consent.modules[id];

    const body = () => {
        if (!m) return <div class="contacts-empty">…</div>;
        if (!m.consented) {
            return (
                <div class="contacts-consent-gate">
                    <p>{t('contacts.privacy.moduleNotice', language)}</p>
                    <button
                        class="contacts-btn contacts-btn-primary contacts-btn-sm"
                        onClick={async () => {
                            await channelModuleService.consent(id);
                            await consent.refresh();
                        }}
                    >
                        {t('contacts.privacy.authorize', language)}
                    </button>
                </div>
            );
        }
        return render(m);
    };

    return (
        <div class="contacts-submodule">
            <div class="contacts-submodule-head">
                <span class="contacts-submodule-title">{title}</span>
                <span class="contacts-submodule-hint">{hint}</span>
                {m?.consented && (
                    <button
                        class="contacts-linkbtn"
                        onClick={async () => {
                            await channelModuleService.revoke(id);
                            await consent.refresh();
                        }}
                    >
                        {t('contacts.privacy.revoke', language)}
                    </button>
                )}
            </div>
            {body()}
        </div>
    );
}
