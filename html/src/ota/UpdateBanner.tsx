// Non-blocking banner that nudges the user to apply a frontend OTA update.
// Renders nothing when `info.hasUpdate` is false. Dismissing records the
// version so we don't surface the same release twice (until a newer one
// lands).

import { h } from 'preact';
import { t, type Lang } from '../i18n';
import { apply } from './applier';
import { dismiss } from './checker';
import type { RootManifest, UpdateInfo } from './checker';

export interface UpdateBannerProps {
    info: UpdateInfo;
    language: Lang;
}

export function UpdateBanner({ info, language }: UpdateBannerProps) {
    if (!info.hasUpdate || !info.manifest) return null;

    const onApply = () => apply(info.manifest as RootManifest);
    const onDismiss = () => dismiss(info.latest);

    return (
        <div class="ota-banner" role="status" aria-live="polite">
            <span class="ota-banner__icon" aria-hidden="true">
                ↑
            </span>
            <span class="ota-banner__text">
                {t('app.ota.banner.body', language, { current: info.current, latest: info.latest })}
            </span>
            <button type="button" class="ota-banner__action" onClick={onApply}>
                {t('app.ota.banner.refresh', language)}
            </button>
            <button
                type="button"
                class="ota-banner__dismiss"
                onClick={onDismiss}
                aria-label={t('app.ota.banner.dismiss', language)}
            >
                ×
            </button>
        </div>
    );
}
