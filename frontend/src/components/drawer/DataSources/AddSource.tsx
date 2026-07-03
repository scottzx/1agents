import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import {
    sourceService,
    type VendorSpec,
    type SourceAccount,
    type CreateAccountInput,
} from '@1agents/core/services/sourceService';
import { VendorIcon } from './vendorIcons';

// 添加数据源 (Add data source) — 源为中心: a vendor gallery, then per-vendor a
// region choice (国际/大陆, limited by vendor capability) + an account form.
// Submitting registers one account = one source (厂家 + 账号). Multi-account
// vendors (Apple/微软/谷歌) can be added repeatedly; single-account vendors (飞书)
// are blocked once one exists.

const VENDOR_UI: Record<string, { descKey: string }> = {
    icloud: { descKey: 'datasource.src.appleDesc' },
    feishu: { descKey: 'datasource.src.feishuDesc' },
    microsoft: { descKey: 'datasource.src.microsoftDesc' },
    google: { descKey: 'datasource.src.googleDesc' },
    agentmail: { descKey: 'datasource.src.agentmailDesc' },
};

function regionLabel(region: string, language: Lang): string {
    return t(`datasource.region.${region}`, language) || region;
}

export function AddSource({
    picked,
    onPick,
    onCreated,
}: {
    picked: VendorSpec | null;
    onPick: (vendor: VendorSpec) => void;
    onCreated: (account: SourceAccount) => void;
}) {
    const language = ui.language.value;
    const [vendors, setVendors] = useState<VendorSpec[] | null>(null);
    const [accounts, setAccounts] = useState<SourceAccount[]>([]);

    useEffect(() => {
        let active = true;
        Promise.all([sourceService.vendors(), sourceService.accounts()])
            .then(([v, a]) => {
                if (!active) return;
                setVendors(v);
                setAccounts(a);
            })
            .catch(() => active && setVendors([]));
        return () => {
            active = false;
        };
    }, []);

    const countFor = (vendor: string) => accounts.filter(a => a.vendor === vendor).length;

    if (picked) {
        return <VendorForm vendor={picked} language={language} onCreated={onCreated} />;
    }

    return (
        <div class="datasource-list">
            <div class="datasource-head-hint">{t('datasource.add.multiHint', language)}</div>
            <div class="datasource-grid bento-grid">
                {(vendors ?? []).map(v => {
                    const uiMeta = VENDOR_UI[v.vendor] ?? { descKey: '' };
                    const full = !v.multiAccount && countFor(v.vendor) > 0;
                    return (
                        <div key={v.vendor} class="datasource-card">
                            <div class="datasource-card-top">
                                <span class="datasource-card-icon">
                                    <VendorIcon vendor={v.vendor} />
                                </span>
                                <span class="datasource-card-title">{v.label}</span>
                                {!v.multiAccount && (
                                    <span class="datasource-card-badge muted">
                                        {t('datasource.add.singleAccountNote', language)}
                                    </span>
                                )}
                            </div>
                            <div class="datasource-card-body">
                                <span class="datasource-card-sub">
                                    {uiMeta.descKey ? t(uiMeta.descKey, language) : ''}
                                </span>
                                <button
                                    class="contacts-btn contacts-btn-sm datasource-src-btn"
                                    disabled={full}
                                    onClick={() => onPick(v)}
                                >
                                    {full
                                        ? t('datasource.add.singleAccountReached', language)
                                        : t('datasource.tab.add', language)}
                                </button>
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
}

function VendorForm({
    vendor,
    language,
    onCreated,
}: {
    vendor: VendorSpec;
    language: Lang;
    onCreated: (account: SourceAccount) => void;
}) {
    const [region, setRegion] = useState(vendor.regions[0] ?? 'intl');
    const [appleId, setAppleId] = useState('');
    const [password, setPassword] = useState('');
    const [label, setLabel] = useState('');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const submit = async () => {
        setBusy(true);
        setError('');
        const input: CreateAccountInput = { vendor: vendor.vendor, region };
        if (vendor.authKind === 'credentials') {
            input.appleId = appleId.trim();
            input.password = password;
        } else if (label.trim()) {
            input.label = label.trim();
        }
        try {
            const account = await sourceService.createAccount(input);
            onCreated(account);
        } catch (e) {
            setError(`${t('datasource.add.failed', language)}: ${(e as Error).message}`);
            setBusy(false);
        }
    };

    return (
        <div class="datasource-add-form">
            <h3 class="datasource-card-title">{vendor.label}</h3>

            {vendor.regions.length > 1 && (
                <div class="datasource-region-choices">
                    <span class="datasource-form-label">{t('datasource.add.pickRegion', language)}</span>
                    {vendor.regions.map(r => (
                        <button
                            key={r}
                            class={`contacts-btn contacts-btn-sm${r === region ? ' contacts-btn-primary' : ''}`}
                            onClick={() => setRegion(r)}
                        >
                            {regionLabel(r, language)}
                        </button>
                    ))}
                </div>
            )}

            {vendor.authKind === 'credentials' && (
                <Fragment>
                    <label class="contacts-field">
                        <span>{t('datasource.add.appleId', language)}</span>
                        <input
                            type="email"
                            autocomplete="off"
                            placeholder="you@icloud.com"
                            value={appleId}
                            onInput={e => setAppleId((e.target as HTMLInputElement).value)}
                        />
                    </label>
                    <label class="contacts-field">
                        <span>{t('datasource.add.password', language)}</span>
                        <input
                            type="password"
                            autocomplete="off"
                            placeholder="xxxx-xxxx-xxxx-xxxx"
                            value={password}
                            onInput={e => setPassword((e.target as HTMLInputElement).value)}
                        />
                    </label>
                </Fragment>
            )}

            {vendor.authKind === 'oauth' && (
                <Fragment>
                    <div class="datasource-head-hint">
                        {vendor.vendor === 'microsoft'
                            ? t('datasource.add.oauthMicrosoft', language)
                            : t('datasource.add.oauthStub', language)}
                    </div>
                    <label class="contacts-field">
                        <span>{t('datasource.add.accountLabel', language)}</span>
                        <input
                            type="email"
                            autocomplete="off"
                            value={label}
                            onInput={e => setLabel((e.target as HTMLInputElement).value)}
                        />
                    </label>
                </Fragment>
            )}

            {vendor.authKind === 'cli' && (
                <div class="datasource-head-hint">{t('datasource.add.cliNote', language)}</div>
            )}

            {error && <div class="contacts-error">{error}</div>}

            <button class="contacts-btn contacts-btn-primary" disabled={busy} onClick={submit}>
                {busy ? t('datasource.add.submitting', language) : t('datasource.add.submit', language)}
            </button>
        </div>
    );
}
