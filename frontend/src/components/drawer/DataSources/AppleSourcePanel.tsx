import { h, Fragment } from 'preact';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import { ICloudSection } from './ICloudSection';
import { IMessageSection } from './IMessageSection';
import { useChannelModules, ConsentSubmodule } from './ConsentGate';
import { SourceDataZone } from './SourceDataZone';

// AppleSourcePanel — the Apple source (local iCloud contacts via CardDAV +
// iMessage via chat.db), one zone per top-nav tab:
//   认证与授权 → privacy-gated credential / Full-Disk-Access setup
//   数据 → SourceDataZone over the icloud/apple bronze records

const MOD_ICLOUD = 'icloud.contacts';
const MOD_IMESSAGE = 'apple.imessage';

// appleTabs are the top-nav tabs for the Apple source.
export function appleTabs(language: Lang): ShellTab[] {
    return [
        { id: 'auth', label: t('datasource.zone.auth', language) },
        { id: 'data', label: t('datasource.data.title', language) },
    ];
}

export function AppleSourcePanel({
    tab,
    onOpenData,
}: {
    tab: string;
    onOpenData: (source: string, kind: string, title: string) => void;
}) {
    const language = ui.language.value;
    const consent = useChannelModules();

    return (
        <div class="source-panel">
            {tab === 'auth' && (
                <Fragment>
                    <div class="contacts-privacy-banner">
                        <span class="contacts-privacy-icon" aria-hidden="true">
                            🔒
                        </span>
                        <span>{t('contacts.privacy.notice', language)}</span>
                    </div>
                    {consent.error && <div class="contacts-error">{consent.error}</div>}
                    <ConsentSubmodule
                        id={MOD_ICLOUD}
                        title={t('contacts.sub.contacts', language)}
                        hint="CardDAV"
                        consent={consent}
                        render={() => <ICloudSection />}
                    />
                    <ConsentSubmodule
                        id={MOD_IMESSAGE}
                        title={t('contacts.sub.imessage', language)}
                        hint="chat.db"
                        consent={consent}
                        render={m => <IMessageSection module={m} onChange={consent.refresh} />}
                    />
                </Fragment>
            )}

            {tab === 'data' && <SourceDataZone sources={['icloud', 'apple']} onOpen={onOpenData} />}
        </div>
    );
}
