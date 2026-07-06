import { h } from 'preact';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';

// 数据融合 (gold) — the cross-source fused view. The engine (entity merge / thread
// stitching / fingerprint dedup) is tracked in #400; until it lands this layer is
// a placeholder so the medallion switch stays complete (接入 → 治理 → 融合).
export function GoldView() {
    const language = ui.language.value;
    return (
        <div class="datasource-detail">
            <div class="silver-tabs">
                <span class="silver-tab active">{t('datasource.gold.title', language)}</span>
            </div>
            <div class="contacts-empty">{t('datasource.gold.empty', language)}</div>
        </div>
    );
}
