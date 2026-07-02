import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import { icloudService } from '@1agents/core/services/contactService';
import type { SourceAccount } from '@1agents/core/services/sourceService';
import { IMessageSection } from './IMessageSection';
import { useChannelModules, ConsentSubmodule } from './ConsentGate';
import { SourceDataZone } from './SourceDataZone';

// AppleSourcePanel — one Apple 数据源 account (源为中心). iCloud contacts pull over
// CardDAV for THIS account's Apple ID + region; iMessage is machine-local (shared
// across accounts). One zone per top-nav tab:
//   认证与授权 → this account's iCloud credential + sync, and iMessage consent
//   数据 → SourceDataZone scoped to this account's bronze

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
    account,
    tab,
    onOpenData,
}: {
    account: SourceAccount;
    tab: string;
    onOpenData: (source: string, kind: string, title: string, account?: string) => void;
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
                        render={() => <ICloudAccountSection account={account} language={language} />}
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

            {tab === 'data' && (
                <SourceDataZone
                    sources={['icloud']}
                    account={account.id}
                    sharedSources={['apple']}
                    onOpen={onOpenData}
                />
            )}
        </div>
    );
}

// ICloudAccountSection — the credential + sync UI for ONE iCloud account. The
// Apple ID and region come from the 数据源 account (set when it was added); this
// only shows status, runs the per-account CardDAV pull, and lets the user
// re-enter the app-specific password (stored in the local Keychain server-side).
function ICloudAccountSection({ account, language }: { account: SourceAccount; language: Lang }) {
    const [configured, setConfigured] = useState<boolean | null>(null);
    const [password, setPassword] = useState('');
    const [reentry, setReentry] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');

    useEffect(() => {
        let active = true;
        icloudService
            .statusFor(account.id)
            .then(s => active && setConfigured(s.configured))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [account.id]);

    const regionText = t(`datasource.region.${account.region}`, language) || account.region;

    const runSync = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            const r = await icloudService.syncFor(account.id);
            setToast(t('contacts.icloud.syncDone', language, { created: r.created, updated: r.updated }));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const savePassword = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            const s = await icloudService.setPasswordFor(account.id, password.trim());
            setConfigured(s.configured);
            setPassword('');
            setReentry(false);
            await runSync();
        } catch (e) {
            setError((e as Error).message);
            setBusy(false);
        }
    };

    return (
        <div class="contacts-section contacts-icloud">
            <div class="contacts-section-head">
                <span class="contacts-section-title">{account.label}</span>
                <span class="datasource-card-badge muted">{regionText}</span>
            </div>

            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}

            {configured && !reentry ? (
                <div class="contacts-icloud-connected">
                    <span class="contacts-icloud-hint">{t('contacts.icloud.connected', language)}</span>
                    <button class="contacts-btn contacts-btn-primary contacts-btn-sm" disabled={busy} onClick={runSync}>
                        {busy ? t('contacts.icloud.syncing', language) : t('contacts.icloud.syncNow', language)}
                    </button>
                    <button class="contacts-btn contacts-btn-sm" disabled={busy} onClick={() => setReentry(true)}>
                        {t('contacts.icloud.password', language)}
                    </button>
                </div>
            ) : (
                <div class="contacts-icloud-setup">
                    <label class="contacts-field">
                        <span>{t('contacts.icloud.password', language)}</span>
                        <input
                            type="password"
                            autocomplete="off"
                            placeholder="xxxx-xxxx-xxxx-xxxx"
                            value={password}
                            onInput={(e: Event) => setPassword((e.target as HTMLInputElement).value)}
                        />
                    </label>
                    <div class="contacts-modal-actions">
                        <button
                            class="contacts-btn contacts-btn-primary"
                            disabled={busy || !password.trim()}
                            onClick={savePassword}
                        >
                            {busy ? t('contacts.icloud.saving', language) : t('contacts.icloud.saveAndSync', language)}
                        </button>
                        {reentry && (
                            <button
                                class="contacts-btn contacts-btn-sm"
                                disabled={busy}
                                onClick={() => setReentry(false)}
                            >
                                {t('datasource.add.back', language)}
                            </button>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
