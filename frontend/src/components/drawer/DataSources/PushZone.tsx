import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import { sourceService, type SourceAccount, type PushKindInfo } from '@1agents/core/services/sourceService';

// PushZone — the 认证 + 配置 surfaces for a 推送式 (authKind=push) 数据源. A push
// source is never crawled: a local Agent POSTs processed records to
// /api/data/push/{source}/{kind}, so its 认证 zone shows the endpoint + a curl
// example + the shared push key (stored via the bearer store), and its 配置 zone
// shows the declared table schema instead of a 定时任务 list (push has no schedule).

// pushEndpoint builds the absolute push URL a local Agent posts to. The backend is
// same-origin for the local H5, so window.location.origin is the right host.
function pushEndpoint(source: string, kind: string): string {
    const origin = typeof window !== 'undefined' ? window.location.origin : '';
    return `${origin}/api/data/push/${source}/${kind}`;
}

function curlExample(source: string, kind: string, language: Lang, info?: PushKindInfo): string {
    const sample: Record<string, unknown> = {};
    if (info) {
        for (const f of info.schema) {
            sample[f.name] =
                f.type === 'number' ? 0 : f.type === 'bool' ? true : f.type === 'array' ? [] : `<${f.name}>`;
        }
    }
    const body = JSON.stringify([Object.keys(sample).length ? sample : { id: '<id>' }]);
    const keyLabel = t('datasource.push.key', language);
    return `curl -X POST ${pushEndpoint(source, kind)} \\\n  -H "Authorization: Bearer <${keyLabel}>" \\\n  -d '${body}'`;
}

// ── 认证 → push endpoint + curl + shared key ──────────────────────────────────

export function PushAuthZone({ account, language }: { account: SourceAccount; language: Lang }) {
    const source = account.vendor;
    const [kinds, setKinds] = useState<PushKindInfo[]>([]);
    const [configured, setConfigured] = useState<boolean | null>(null);
    const [key, setKey] = useState('');
    const [reentry, setReentry] = useState(false);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .pushInfo(source)
            .then(ks => active && setKinds(ks))
            .catch(e => active && setError((e as Error).message));
        sourceService
            .bearerStatus(source, account.id)
            .then(s => active && setConfigured(s.configured))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [source, account.id]);

    const save = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            const s = await sourceService.setBearerToken(source, key.trim(), account.id);
            setConfigured(s.configured);
            setKey('');
            setReentry(false);
            setToast(t('datasource.push.keySaved', language));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const firstKind = kinds[0];

    return (
        <div class="contacts-section">
            <div class="contacts-privacy-banner">
                <span class="contacts-privacy-icon" aria-hidden="true">
                    📥
                </span>
                <span>{t('datasource.push.hint', language)}</span>
            </div>
            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}

            {/* Endpoints per kind */}
            <div class="push-endpoints">
                <div class="contacts-section-title">{t('datasource.push.endpoint', language)}</div>
                {kinds.map(k => (
                    <div key={k.kind} class="push-endpoint-row">
                        <span class="datasource-card-badge muted">{k.label || k.kind}</span>
                        <code class="push-endpoint-url">{pushEndpoint(source, k.kind)}</code>
                    </div>
                ))}
            </div>

            {/* curl example */}
            {firstKind && (
                <div class="push-example">
                    <div class="contacts-section-title">{t('datasource.push.example', language)}</div>
                    <pre class="chat-tool-output-box push-curl">
                        {curlExample(source, firstKind.kind, language, firstKind)}
                    </pre>
                </div>
            )}

            {/* shared push key (reuses the bearer store) */}
            <div class="push-key">
                <div class="contacts-section-head">
                    <span class="contacts-section-title">{t('datasource.push.key', language)}</span>
                </div>
                <p class="bento-card-desc">{t('datasource.push.keyHint', language)}</p>
                {configured && !reentry ? (
                    <div class="contacts-icloud-connected">
                        <span class="contacts-icloud-hint">{t('datasource.push.keyConfigured', language)}</span>
                        <button class="contacts-btn contacts-btn-sm" disabled={busy} onClick={() => setReentry(true)}>
                            {t('datasource.push.keyReplace', language)}
                        </button>
                    </div>
                ) : (
                    <div class="contacts-icloud-setup">
                        <label class="contacts-field">
                            <span>{t('datasource.push.key', language)}</span>
                            <input
                                type="password"
                                autocomplete="off"
                                spellcheck={false}
                                placeholder="push_..."
                                value={key}
                                onInput={(e: Event) => setKey((e.target as HTMLInputElement).value)}
                            />
                        </label>
                        <div class="contacts-modal-actions">
                            <button
                                class="contacts-btn contacts-btn-primary"
                                disabled={busy || !key.trim()}
                                onClick={save}
                            >
                                {busy
                                    ? t('datasource.push.keySaving', language)
                                    : t('datasource.push.keySave', language)}
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
        </div>
    );
}

// ── 配置 → declared table schema (no schedule for push) ───────────────────────

export function PushSchemaZone({ source, language }: { source: string; language: Lang }) {
    const [kinds, setKinds] = useState<PushKindInfo[]>([]);
    const [error, setError] = useState('');

    useEffect(() => {
        let active = true;
        sourceService
            .pushInfo(source)
            .then(ks => active && setKinds(ks))
            .catch(e => active && setError((e as Error).message));
        return () => {
            active = false;
        };
    }, [source]);

    return (
        <div class="push-schema-zone">
            <div class="contacts-privacy-banner">
                <span class="contacts-privacy-icon" aria-hidden="true">
                    ⏸️
                </span>
                <span>{t('datasource.push.noSchedule', language)}</span>
            </div>
            {error && <div class="contacts-error">{error}</div>}
            {kinds.map(k => (
                <div key={k.kind} class="bento-card sys-settings-card push-schema-card">
                    <div class="bento-zone-body">
                        <div class="contacts-section-head">
                            <span class="contacts-section-title">{k.label || k.kind}</span>
                            {k.uidField && (
                                <span class="datasource-card-badge muted">
                                    {t('datasource.push.uid', language)}: {k.uidField}
                                </span>
                            )}
                        </div>
                        {k.schema.length === 0 ? (
                            <p class="bento-card-desc">{t('datasource.push.noSchema', language)}</p>
                        ) : (
                            <table class="push-schema-table">
                                <thead>
                                    <tr>
                                        <th>{t('datasource.push.field', language)}</th>
                                        <th>{t('datasource.push.type', language)}</th>
                                        <th>{t('datasource.push.required', language)}</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {k.schema.map(f => (
                                        <tr key={f.name}>
                                            <td>
                                                <code>{f.name}</code>
                                            </td>
                                            <td>{f.type || 'any'}</td>
                                            <td>{f.required ? '✓' : ''}</td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        )}
                    </div>
                </div>
            ))}
        </div>
    );
}
