import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { icloudService, type ICloudStatus } from '@1agents/core/services/contactService';

// iCloud 通讯录 section (lives at the top of the 渠道 tab). One-time guided setup:
// the user generates an app-specific password at Apple, pastes it once (stored
// in the local Keychain server-side), and contacts pull over CardDAV. No Mac
// Contacts permission / Full Disk Access needed — just the credential.

// Apple ID sign-in entry; App-Specific Passwords live under its Sign-In and
// Security section. Use the classic appleid.apple.com host — it handles the
// sign-in/redirect cleanly (deep-linking straight to /account/manage can 502).
const APPLE_ACCOUNT_URL = 'https://appleid.apple.com';

export function ICloudSection() {
    const language = ui.language.value;
    const [status, setStatus] = useState<ICloudStatus | null>(null);
    const [appleId, setAppleId] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');
    const [busy, setBusy] = useState(false);

    useEffect(() => {
        icloudService
            .status()
            .then(setStatus)
            .catch(e => setError((e as Error).message));
    }, []);

    const runSync = async () => {
        const r = await icloudService.sync();
        setToast(t('contacts.icloud.syncDone', language, { created: r.created, updated: r.updated }));
    };

    const saveAndSync = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            setStatus(await icloudService.setCredentials(appleId.trim(), password.trim()));
            setPassword('');
            await runSync();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const syncNow = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            await runSync();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    const clear = async () => {
        setError('');
        setToast('');
        setBusy(true);
        try {
            await icloudService.clearCredentials();
            setStatus({ configured: false, appleId: '' });
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="contacts-section contacts-icloud">
            <div class="contacts-section-head">
                <span class="contacts-section-title">{t('contacts.icloud.title', language)}</span>
                {status?.configured && <span class="contacts-channels-badge int">{status.appleId}</span>}
            </div>

            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}

            {status?.configured ? (
                <div class="contacts-icloud-connected">
                    <span class="contacts-icloud-hint">{t('contacts.icloud.connected', language)}</span>
                    <button class="contacts-btn contacts-btn-primary contacts-btn-sm" disabled={busy} onClick={syncNow}>
                        {busy ? t('contacts.icloud.syncing', language) : t('contacts.icloud.syncNow', language)}
                    </button>
                    <button class="contacts-btn contacts-btn-sm contacts-btn-danger" disabled={busy} onClick={clear}>
                        {t('contacts.icloud.clear', language)}
                    </button>
                </div>
            ) : (
                <div class="contacts-icloud-setup">
                    <ol class="contacts-icloud-steps">
                        <li>
                            {t('contacts.icloud.step1', language)}{' '}
                            <a
                                class="contacts-icloud-link"
                                href={APPLE_ACCOUNT_URL}
                                target="_blank"
                                rel="noopener noreferrer"
                            >
                                {t('contacts.icloud.openApple', language)}
                            </a>
                        </li>
                        <li>{t('contacts.icloud.step2', language)}</li>
                    </ol>
                    <label class="contacts-field">
                        <span>{t('contacts.icloud.appleId', language)}</span>
                        <input
                            type="email"
                            autocomplete="off"
                            placeholder="you@icloud.com"
                            value={appleId}
                            onInput={(e: Event) => setAppleId((e.target as HTMLInputElement).value)}
                        />
                    </label>
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
                            disabled={busy || !appleId.trim() || !password.trim()}
                            onClick={saveAndSync}
                        >
                            {busy ? t('contacts.icloud.saving', language) : t('contacts.icloud.saveAndSync', language)}
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
