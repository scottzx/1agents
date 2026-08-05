/**
 * ProductShellPanel — the 产品壳 settings category (C0 Product Shell
 * Registry, design §8/D7).
 *
 * One card per shell with the tenant controls:
 *   - enable / disable (flag only — never deletes data),
 *   - choose the tenant default shell,
 *   - choose / clear the user's preferred shell (overrides the tenant
 *     default only while the shell stays enabled),
 *   - enter (switch the active shell for this UI).
 *
 * The panel composes state from productShellStore; the backend is the
 * source of truth for the product profile.
 */

import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import type { Lang } from '../../i18n';
import { t } from '../../i18n';
import * as shellStore from '../../stores/productShellStore';
import * as ui from '../../stores/uiStore';
import type { ProductShell } from '../../services/productShellService';

interface ProductShellPanelProps {
    language: Lang;
}

/** Small lucide-style icon per built-in shell id; generic grid fallback. */
function ShellIcon({ id }: { id: string }) {
    const common = {
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': 2,
        'stroke-linecap': 'round' as const,
        'stroke-linejoin': 'round' as const,
    };
    switch (id) {
        case 'personal':
            return (
                <svg {...common}>
                    <path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                    <polyline points="9 22 9 12 15 12 15 22" />
                </svg>
            );
        case 'presales':
            return (
                <svg {...common}>
                    <rect x="2" y="7" width="20" height="14" rx="2" />
                    <path d="M16 21V5a2 2 0 0 0-2-2h-4a2 2 0 0 0-2 2v16" />
                </svg>
            );
        case 'commerce':
            return (
                <svg {...common}>
                    <path d="M6 2 3 6v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V6l-3-4z" />
                    <line x1="3" y1="6" x2="21" y2="6" />
                    <path d="M16 10a4 4 0 0 1-8 0" />
                </svg>
            );
        default:
            return (
                <svg {...common}>
                    <rect x="3" y="3" width="7" height="7" rx="1" />
                    <rect x="14" y="3" width="7" height="7" rx="1" />
                    <rect x="3" y="14" width="7" height="7" rx="1" />
                    <rect x="14" y="14" width="7" height="7" rx="1" />
                </svg>
            );
    }
}

export function ProductShellPanel({ language }: ProductShellPanelProps) {
    useEffect(() => {
        void shellStore.loadShells();
    }, []);

    const shells = shellStore.productShells.value;
    const activeId = shellStore.activeShellId.value;
    const tenantDefault = shellStore.tenantDefaultShell.value;
    const preferred = shellStore.userPreferredShell.value;
    const loading = shellStore.shellsLoading.value;

    const onToggleEnabled = async (shell: ProductShell, enabled: boolean) => {
        const ok = await shellStore.toggleShell(shell.id, enabled);
        if (!ok) ui.showToast(enabled ? '启用产品壳失败' : '停用产品壳失败');
    };
    const onMakeDefault = async (id: string) => {
        const ok = await shellStore.chooseDefaultShell(id);
        if (!ok) ui.showToast('设置默认产品壳失败（需先启用该壳）');
    };
    const onMakePreferred = async (id: string) => {
        const ok = await shellStore.choosePreferredShell(id);
        if (!ok) ui.showToast('设置偏好产品壳失败');
    };
    const onClearPreferred = async () => {
        const ok = await shellStore.choosePreferredShell('');
        if (!ok) ui.showToast('清除偏好产品壳失败');
    };

    return (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.shells', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.shells.desc', language)}</div>

            {loading && shells.length === 0 && (
                <div class="sys-settings-card">
                    <div class="sys-settings-card-subtitle">正在加载产品壳…</div>
                </div>
            )}

            {shells.map(shell => {
                const isDefault = tenantDefault === shell.id;
                const isPreferred = preferred === shell.id;
                const isActive = activeId === shell.id;
                const statusParts = [
                    shell.enabled ? t('settings.shells.enabled', language) : t('settings.shells.disabled', language),
                    ...(isDefault ? [t('settings.shells.default', language)] : []),
                    ...(isPreferred ? [t('settings.shells.preference', language)] : []),
                    ...(isActive ? [t('settings.shells.active', language)] : []),
                ];
                return (
                    <div class="sys-settings-card" key={shell.id}>
                        <div class="sys-settings-card-header">
                            <div class="sys-settings-card-icon">
                                <ShellIcon id={shell.id} />
                            </div>
                            <div>
                                <div class="sys-settings-card-title">{shell.name}</div>
                                <div class="sys-settings-card-subtitle">{shell.description || shell.id}</div>
                                <div class="sys-settings-card-subtitle">{statusParts.join(' · ')}</div>
                            </div>
                        </div>
                        <div class="sys-settings-toggle-group">
                            <button
                                class={`sys-settings-option-btn ${shell.enabled ? 'active' : ''}`}
                                onClick={() => void onToggleEnabled(shell, !shell.enabled)}
                                title={t('settings.shells.disableHint', language)}
                            >
                                {shell.enabled
                                    ? t('settings.shells.disable', language)
                                    : t('settings.shells.enable', language)}
                            </button>
                            <button
                                class={`sys-settings-option-btn ${isDefault ? 'active' : ''}`}
                                disabled={!shell.enabled || isDefault}
                                onClick={() => void onMakeDefault(shell.id)}
                            >
                                {isDefault
                                    ? t('settings.shells.default', language)
                                    : t('settings.shells.makeDefault', language)}
                            </button>
                            {isPreferred ? (
                                <button class="sys-settings-option-btn active" onClick={() => void onClearPreferred()}>
                                    {t('settings.shells.clearPreference', language)}
                                </button>
                            ) : (
                                <button
                                    class="sys-settings-option-btn"
                                    disabled={!shell.enabled}
                                    onClick={() => void onMakePreferred(shell.id)}
                                >
                                    {t('settings.shells.makePreference', language)}
                                </button>
                            )}
                            <button
                                class={`sys-settings-option-btn ${isActive ? 'active' : ''}`}
                                disabled={!shell.enabled || isActive}
                                onClick={() => shellStore.setActiveShell(shell.id)}
                            >
                                {isActive
                                    ? t('settings.shells.active', language)
                                    : t('settings.shells.enter', language)}
                            </button>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}
