import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type SourceAccount, type MSOAuthStatus } from '@1agents/core/services/sourceService';
import { icloudService } from '@1agents/core/services/contactService';
import { CliZone } from './FeishuSourceCard';
import { IMessageSection } from './IMessageSection';
import { useChannelModules, ConsentSubmodule } from './ConsentGate';
import { PushAuthZone } from './PushZone';

// SourceAuthZone — the single, authKind-driven 认证与授权 zone. Instead of each
// vendor shipping its own auth panel, the backend's VendorSpec.authKind
// (credentials | cli | oauth) selects one of three reference flows:
//   oauth        → 网页跳转 OAuth (Microsoft Graph PKCE)   → OAuthAuthZone
//   cli          → 命令行授权 (lark-cli 交互式 / agently-cli 拿链接) → CliZone
//   credentials  → 凭据表单 (Apple ID + 专用密码)          → CredentialsAuthZone
// Vendor-specific extras (iMessage 本地库, Microsoft App 注册表单) live inside the
// variant they belong to, so the dispatcher stays a pure authKind switch.

// VENDOR_CLI_TOOL maps a cli-auth vendor to the CLI its 认证 zone probes.
const VENDOR_CLI_TOOL: Record<string, string> = {
    feishu: 'lark-cli',
    agentmail: 'agently-cli',
};

export function SourceAuthZone({ account, authKind }: { account: SourceAccount; authKind: string }) {
    const language = ui.language.value;
    switch (authKind) {
        case 'oauth':
            return <OAuthAuthZone account={account} language={language} />;
        case 'cli':
            return <CliAuthZone account={account} language={language} />;
        case 'credentials':
            return <CredentialsAuthZone account={account} language={language} />;
        case 'bearer':
            return <BearerAuthZone account={account} language={language} />;
        case 'push':
            return <PushAuthZone account={account} language={language} />;
        default:
            return (
                <div class="contacts-privacy-banner">
                    <span class="contacts-privacy-icon" aria-hidden="true">
                        🔐
                    </span>
                    <span>{t('datasource.instance.authStub', language)}</span>
                </div>
            );
    }
}

// CliAuthZone resolves which CLI a source authorizes through, then renders its
// probe. Built-in vendors are in VENDOR_CLI_TOOL; a manifest CLI source carries its
// tool on the VendorSpec (cliTool), fetched here so a hot-added connector's 认证 zone
// works without a code change.
function CliAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const [tool, setTool] = useState<string | undefined>(VENDOR_CLI_TOOL[account.vendor]);
    useEffect(() => {
        if (VENDOR_CLI_TOOL[account.vendor]) return; // built-in, no lookup needed
        let active = true;
        sourceService
            .vendors()
            .then(vs => active && setTool(vs.find(v => v.vendor === account.vendor)?.cliTool))
            .catch(() => {});
        return () => {
            active = false;
        };
    }, [account.vendor]);
    return <CliZone tool={tool} language={language} />;
}

// ── credentials → Apple (currently the sole credentials vendor) ────────────────

const MOD_ICLOUD = 'icloud.contacts';
const MOD_IMESSAGE = 'apple.imessage';

// CredentialsAuthZone composes the Apple 认证 flow: a privacy banner, the
// consent-gated iCloud credential section (CardDAV), and the machine-local
// iMessage submodule. When a second credentials vendor appears, split the Apple
// specifics out; for now Apple is the only consumer (see VendorSpec.authKind).
function CredentialsAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const consent = useChannelModules();
    return (
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
                render={() => <ICloudCredentialsSection account={account} language={language} />}
            />
            <ConsentSubmodule
                id={MOD_IMESSAGE}
                title={t('contacts.sub.imessage', language)}
                hint="chat.db"
                consent={consent}
                render={m => <IMessageSection module={m} onChange={consent.refresh} />}
            />
        </Fragment>
    );
}

// ICloudCredentialsSection — credential + sync UI for ONE credentials account.
// The account label + region come from the 数据源 account; this shows status,
// runs the per-account CardDAV pull, and lets the user (re)enter the
// app-specific password (stored in the local Keychain server-side).
function ICloudCredentialsSection({ account, language }: { account: SourceAccount; language: Lang }) {
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

// ── bearer → manifest REST sources (静态 token) ───────────────────────────────

// BearerAuthZone stores the static Bearer token for a manifest-declared REST
// source (authKind=bearer, e.g. 训记). The token is written server-side (a 0600
// per-account file) and never echoed back — the UI only knows configured/not.
function BearerAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const [configured, setConfigured] = useState<boolean | null>(null);
    const [token, setToken] = useState('');
    const [reentry, setReentry] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .bearerStatus(account.vendor, account.id)
            .then(s => active && setConfigured(s.configured))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [account.vendor, account.id]);

    const save = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            const s = await sourceService.setBearerToken(account.vendor, token.trim(), account.id);
            setConfigured(s.configured);
            setToken('');
            setReentry(false);
            setToast(t('datasource.bearer.saved', language));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="contacts-section">
            <div class="contacts-section-head">
                <span class="contacts-section-title">{account.label}</span>
            </div>
            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}
            {configured && !reentry ? (
                <div class="contacts-icloud-connected">
                    <span class="contacts-icloud-hint">{t('datasource.bearer.configured', language)}</span>
                    <button class="contacts-btn contacts-btn-sm" disabled={busy} onClick={() => setReentry(true)}>
                        {t('datasource.bearer.replace', language)}
                    </button>
                </div>
            ) : (
                <div class="contacts-icloud-setup">
                    <label class="contacts-field">
                        <span>{t('datasource.bearer.token', language)}</span>
                        <input
                            type="password"
                            autocomplete="off"
                            spellcheck={false}
                            placeholder="xjllm_..."
                            value={token}
                            onInput={(e: Event) => setToken((e.target as HTMLInputElement).value)}
                        />
                    </label>
                    <div class="contacts-modal-actions">
                        <button
                            class="contacts-btn contacts-btn-primary"
                            disabled={busy || !token.trim()}
                            onClick={save}
                        >
                            {busy ? t('datasource.bearer.saving', language) : t('datasource.bearer.save', language)}
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

// ── oauth → Microsoft Graph (PKCE) ────────────────────────────────────────────

// OAuthAuthZone drives the real Microsoft Graph OAuth (PKCE) connect for one
// account. It reads the per-account status, opens the region-correct sign-in
// (大陆/21Vianet vs 国际) in a popup, and polls until the callback stores a token.
// The in-UI app-registration form (clientId/tenant) is part of this variant so
// the user never hand-edits microsoft_oauth.json.
function OAuthAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const [status, setStatus] = useState<MSOAuthStatus | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
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
