import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import type { ShellTab } from '../../platform/ShellNav';
import {
    sourceService,
    type CollectionView,
    type SourceAccount,
    type MSOAuthStatus,
} from '@1agents/core/services/sourceService';
import { SourceDataZone } from './SourceDataZone';

// SourceInstancePanel — the generic panel for OAuth-style multi-account sources
// (microsoft / google). One zone per tab:
//   认证 → Microsoft: the real OAuth (PKCE) connect flow; Google: 占位 until wired
//   采集配置 → the roadmap of crawlable kinds (per-kind implemented flag)
//   数据 → SourceDataZone over the vendor's bronze
// Apple/飞书 keep their bespoke panels; this covers the Graph/Google sources.

export function instanceTabs(language: Lang): ShellTab[] {
    return [
        { id: 'auth', label: t('datasource.zone.auth', language) },
        { id: 'config', label: t('datasource.tab.config', language) },
        { id: 'data', label: t('datasource.data.title', language) },
    ];
}

export function SourceInstancePanel({
    account,
    tab,
    onOpenData,
}: {
    account: SourceAccount;
    tab: string;
    onOpenData: (source: string, kind: string, title: string, account?: string) => void;
}) {
    const language = ui.language.value;
    const vendor = account.vendor;

    return (
        <div class="source-panel">
            {tab === 'auth' &&
                (vendor === 'microsoft' ? (
                    <MicrosoftAuthZone account={account} language={language} />
                ) : (
                    <div class="contacts-privacy-banner">
                        <span class="contacts-privacy-icon" aria-hidden="true">
                            🔐
                        </span>
                        <span>{t('datasource.instance.authStub', language)}</span>
                    </div>
                ))}

            {tab === 'config' && <SourceConfigZone vendor={vendor} language={language} />}

            {tab === 'data' && (
                <Fragment>
                    <SourceDataZone sources={[vendor]} account={account.id} onOpen={onOpenData} />
                </Fragment>
            )}
        </div>
    );
}

// SourceConfigZone lists a source's crawlable kinds and, for the implemented
// ones, lets the user enable collection and trigger an immediate sync. Kinds
// still on the roadmap show a "not implemented" badge. (Reflects the backend's
// per-kind `implemented` flag instead of hard-coding "coming soon".)
function SourceConfigZone({ vendor, language }: { vendor: string; language: Lang }) {
    const [collections, setCollections] = useState<CollectionView[] | null>(null);
    const [busyKind, setBusyKind] = useState('');
    const [msg, setMsg] = useState('');

    const load = () =>
        sourceService
            .collections(vendor)
            .then(setCollections)
            .catch(() => setCollections([]));

    useEffect(() => {
        let active = true;
        sourceService
            .collections(vendor)
            .then(l => active && setCollections(l))
            .catch(() => active && setCollections([]));
        return () => {
            active = false;
        };
    }, [vendor]);

    const setEnabled = async (c: CollectionView, enabled: boolean) => {
        setBusyKind(c.kind);
        setMsg('');
        try {
            await sourceService.setCollection(vendor, {
                kind: c.kind,
                enabled,
                initialLookbackDays: c.initialLookbackDays || 0,
                incrementalMinutes: c.incrementalMinutes || 60,
                pageSize: c.pageSize || 50,
            });
            await load();
        } catch (e) {
            setMsg((e as Error).message);
        }
        setBusyKind('');
    };

    const syncNow = async (c: CollectionView) => {
        setBusyKind(c.kind);
        setMsg('');
        try {
            await sourceService.syncNow(vendor, c.kind);
            setMsg(t('datasource.config.syncStarted', language));
        } catch (e) {
            setMsg((e as Error).message);
        }
        setBusyKind('');
    };

    if (collections === null) {
        return <div class="datasource-head-hint">…</div>;
    }

    return (
        <div class="source-instance-config">
            {msg && <div class="datasource-head-hint">{msg}</div>}
            <div class="bento-grid">
                {collections.map(c => (
                    <div key={c.kind} class="bento-card sys-settings-card">
                        <div class="bento-zone-body">
                            <h3 class="bento-card-title">{c.label || c.kind}</h3>
                            <p class="bento-card-desc">{c.domain}</p>
                            {c.implemented ? (
                                <div class="datasource-region-choices">
                                    <button
                                        class={`contacts-btn contacts-btn-sm${c.enabled ? ' contacts-btn-primary' : ''}`}
                                        disabled={busyKind === c.kind}
                                        onClick={() => setEnabled(c, !c.enabled)}
                                    >
                                        {c.enabled
                                            ? t('datasource.config.enabled', language)
                                            : t('datasource.config.enable', language)}
                                    </button>
                                    {c.enabled && (
                                        <button
                                            class="contacts-btn contacts-btn-sm"
                                            disabled={busyKind === c.kind}
                                            onClick={() => syncNow(c)}
                                        >
                                            {t('datasource.config.syncNow', language)}
                                        </button>
                                    )}
                                </div>
                            ) : (
                                <span class="datasource-card-badge warn">
                                    {t('datasource.config.notImplemented', language)}
                                </span>
                            )}
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}

// MicrosoftAuthZone drives the real Microsoft Graph OAuth (PKCE) connect for one
// account. It reads the per-account status, opens the region-correct sign-in
// (大陆/21Vianet vs 国际) in a popup, and polls until the callback stores a token.
function MicrosoftAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const [status, setStatus] = useState<MSOAuthStatus | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    // In-UI app registration (so the user never hand-edits microsoft_oauth.json).
    const [clientId, setClientId] = useState('');
    const [tenant, setTenant] = useState('');
    const [savingCfg, setSavingCfg] = useState(false);

    const refresh = () =>
        sourceService
            .msOAuthStatus(account.id)
            .then(setStatus)
            .catch(e => setError((e as Error).message));

    useEffect(() => {
        let active = true;
        sourceService
            .msOAuthStatus(account.id)
            .then(s => active && setStatus(s))
            .catch(e => active && setError((e as Error).message));
        // Prefill the app-registration form for this account's region.
        sourceService
            .msOAuthGetConfig(account.region)
            .then(cfg => {
                if (!active) return;
                setClientId(cfg.clientId);
                setTenant(cfg.tenant);
            })
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [account.id, account.region]);

    const saveConfigAndConnect = async () => {
        setSavingCfg(true);
        setError('');
        try {
            await sourceService.msOAuthSetConfig({ region: account.region, clientId, tenant });
            const s = await sourceService.msOAuthStatus(account.id);
            setStatus(s);
            setSavingCfg(false);
            if (s.configured) await connect(); // 保存即连接
        } catch (e) {
            setError((e as Error).message);
            setSavingCfg(false);
        }
    };

    const connect = async () => {
        setBusy(true);
        setError('');
        try {
            const { authUrl } = await sourceService.msOAuthStart(account.id);
            const popup = window.open(authUrl, 'ms-oauth', 'width=520,height=680');
            if (!popup) {
                setError(t('datasource.ms.popupBlocked', language));
                setBusy(false);
                return;
            }
            // Poll status until the callback attaches a token (or we give up).
            let tries = 0;
            const timer = window.setInterval(async () => {
                tries += 1;
                try {
                    const s = await sourceService.msOAuthStatus(account.id);
                    setStatus(s);
                    if (s.connected || tries > 60) {
                        window.clearInterval(timer);
                        setBusy(false);
                    }
                } catch {
                    /* keep polling */
                }
            }, 2000);
        } catch (e) {
            setError((e as Error).message);
            setBusy(false);
        }
    };

    const disconnect = async () => {
        setBusy(true);
        try {
            await sourceService.msOAuthDisconnect(account.id);
            await refresh();
        } catch (e) {
            setError((e as Error).message);
        }
        setBusy(false);
    };

    const regionLabel =
        account.region === 'cn' ? t('datasource.ms.regionCN', language) : t('datasource.ms.regionIntl', language);

    if (status === null) {
        return <div class="datasource-head-hint">…</div>;
    }

    return (
        <div class="source-instance-auth">
            {!status.configured && (
                <div class="bento-card sys-settings-card">
                    <div class="bento-zone-body">
                        <h3 class="bento-card-title">{t('datasource.ms.appConfigTitle', language)}</h3>
                        <p class="bento-card-desc">{t('datasource.ms.appConfigHint', language)}</p>
                        <label class="contacts-field">
                            <span>{t('datasource.ms.clientId', language)}</span>
                            <input
                                type="text"
                                autocomplete="off"
                                spellcheck={false}
                                placeholder="00000000-0000-0000-0000-000000000000"
                                value={clientId}
                                onInput={e => setClientId((e.target as HTMLInputElement).value)}
                            />
                        </label>
                        <label class="contacts-field">
                            <span>{t('datasource.ms.tenant', language)}</span>
                            <input
                                type="text"
                                autocomplete="off"
                                spellcheck={false}
                                placeholder={account.region === 'cn' ? 'xxx.partner.onmschina.cn / 租户GUID' : 'common'}
                                value={tenant}
                                onInput={e => setTenant((e.target as HTMLInputElement).value)}
                            />
                        </label>
                        <button
                            class="contacts-btn contacts-btn-primary"
                            disabled={savingCfg || !clientId.trim()}
                            onClick={saveConfigAndConnect}
                        >
                            {savingCfg
                                ? t('datasource.ms.connecting', language)
                                : t('datasource.ms.saveAndConnect', language)}
                        </button>
                        {error && <div class="contacts-error">{error}</div>}
                    </div>
                </div>
            )}

            {status.configured && (
                <div class="bento-card sys-settings-card">
                    <div class="bento-zone-body">
                        <h3 class="bento-card-title">Microsoft · {regionLabel}</h3>
                        {status.connected ? (
                            <Fragment>
                                <span class="datasource-card-badge">{t('datasource.ms.connected', language)}</span>
                                <p class="bento-card-desc">
                                    {account.label}
                                    {status.expiresAt > 0 && (
                                        <Fragment>
                                            {' · '}
                                            {t('datasource.ms.expires', language)}{' '}
                                            {new Date(status.expiresAt * 1000).toLocaleString()}
                                        </Fragment>
                                    )}
                                </p>
                                <div class="datasource-region-choices">
                                    <button
                                        class="contacts-btn contacts-btn-sm"
                                        disabled={busy || !status.configured}
                                        onClick={connect}
                                    >
                                        {t('datasource.ms.reconnect', language)}
                                    </button>
                                    <button class="contacts-btn contacts-btn-sm" disabled={busy} onClick={disconnect}>
                                        {t('datasource.ms.disconnect', language)}
                                    </button>
                                </div>
                            </Fragment>
                        ) : (
                            <Fragment>
                                <p class="bento-card-desc">{t('datasource.ms.connectHint', language)}</p>
                                <button
                                    class="contacts-btn contacts-btn-primary"
                                    disabled={busy || !status.configured}
                                    onClick={connect}
                                >
                                    {busy
                                        ? t('datasource.ms.connecting', language)
                                        : t('datasource.ms.connect', language)}
                                </button>
                            </Fragment>
                        )}
                        {error && <div class="contacts-error">{error}</div>}
                    </div>
                </div>
            )}
        </div>
    );
}
