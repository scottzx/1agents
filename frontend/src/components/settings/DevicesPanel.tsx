/**
 * 设备管理面板(issue #113,多设备 mesh 计划 #115 的 Phase 5)。
 *
 * 消费后端 #110 暴露的本地设备注册表:
 *  - GET    /api/devices          列出已知设备(含派生的 active 在线态)
 *  - POST   /api/devices          upsert(后端按字段合并 → 用于重命名)
 *  - DELETE /api/devices          移除设备(body: {id})
 *  - POST   /api/devices/refresh  Tailscale 扫描 + 合并新发现的节点 → 返回全量列表
 *
 * 前端只连宿主机(本地代理模型),所有调用都打同源 /api/devices*。
 */
import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { t, type Lang } from '../../i18n';

/** 后端 system.Device 的 JSON 形状(backend/internal/system/devices.go)。 */
interface Device {
    id: string;
    name: string;
    os?: string;
    arch?: string;
    address?: string;
    tailscaleIp?: string;
    version?: string;
    self?: boolean;
    lastSeen?: number;
    active: boolean;
}

/** Mac / Linux / Windows 的内联 SVG 图标(走 currentColor,适配明暗主题)。 */
function OsIcon({ os }: { os?: string }) {
    const kind = (os ?? '').toLowerCase();
    const common = {
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': '2',
        'stroke-linecap': 'round' as const,
        'stroke-linejoin': 'round' as const,
        width: 18,
        height: 18,
    };
    if (kind.includes('darwin') || kind.includes('mac') || kind.includes('ios')) {
        // Apple
        return (
            <svg {...common}>
                <path d="M12 20.94c1.5 0 2.75 1.06 4 1.06 3 0 6-8 6-12.22A4.91 4.91 0 0 0 17 5c-2.22 0-4 1.44-5 2-1-.56-2.78-2-5-2a4.9 4.9 0 0 0-5 4.78C2 14 5 22 8 22c1.25 0 2.5-1.06 4-1.06Z" />
                <path d="M10 2c1 .5 2 2 2 5" />
            </svg>
        );
    }
    if (kind.includes('windows')) {
        return (
            <svg {...common}>
                <rect x="3" y="4" width="7" height="7" rx="0.5" />
                <rect x="14" y="4" width="7" height="7" rx="0.5" />
                <rect x="3" y="13" width="7" height="7" rx="0.5" />
                <rect x="14" y="13" width="7" height="7" rx="0.5" />
            </svg>
        );
    }
    if (kind.includes('linux')) {
        // 终端/服务器风格 — Linux 没有简洁的单笔画企鹅,用服务器箱体表示
        return (
            <svg {...common}>
                <rect x="2" y="2" width="20" height="8" rx="2" />
                <rect x="2" y="14" width="20" height="8" rx="2" />
                <line x1="6" y1="6" x2="6.01" y2="6" />
                <line x1="6" y1="18" x2="6.01" y2="18" />
            </svg>
        );
    }
    // 未知 OS — 通用显示器图标
    return (
        <svg {...common}>
            <rect x="2" y="3" width="20" height="14" rx="2" />
            <line x1="8" y1="21" x2="16" y2="21" />
            <line x1="12" y1="17" x2="12" y2="21" />
        </svg>
    );
}

/** 把 unix 毫秒时间戳格式化为「N 秒/分钟/小时前」。 */
function relativeTime(lastSeen: number | undefined, lang: Lang): string {
    if (!lastSeen) return t('settings.devices.never', lang);
    const diff = Date.now() - lastSeen;
    if (diff < 60_000) return t('settings.devices.justNow', lang);
    const mins = Math.floor(diff / 60_000);
    if (mins < 60) return t('settings.devices.minsAgo', lang, { n: mins });
    const hours = Math.floor(mins / 60);
    if (hours < 24) return t('settings.devices.hoursAgo', lang, { n: hours });
    const days = Math.floor(hours / 24);
    return t('settings.devices.daysAgo', lang, { n: days });
}

export function DevicesPanel({ language }: { language: Lang }) {
    const devices = useSignal<Device[]>([]);
    const loading = useSignal(false);
    const scanning = useSignal(false);
    const msg = useSignal('');
    const editingId = useSignal<string | null>(null);
    const editDraft = useSignal('');

    const load = async () => {
        loading.value = true;
        try {
            const res = await fetch('/api/devices');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            devices.value = (await res.json()) ?? [];
            msg.value = '';
        } catch (e) {
            msg.value =
                '❌ ' + t('settings.devices.loadFailed', language) + ': ' + ((e as Error)?.message ?? String(e));
        } finally {
            loading.value = false;
        }
    };

    useEffect(() => {
        void load();
    }, []);

    const scan = async () => {
        scanning.value = true;
        msg.value = t('settings.devices.scanning', language);
        try {
            const before = devices.value.length;
            const res = await fetch('/api/devices/refresh', { method: 'POST' });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            devices.value = (await res.json()) ?? [];
            const added = devices.value.length - before;
            msg.value =
                added > 0
                    ? '✅ ' + t('settings.devices.scanFound', language, { n: added })
                    : '✅ ' + t('settings.devices.scanNone', language);
        } catch (e) {
            msg.value =
                '❌ ' + t('settings.devices.scanFailed', language) + ': ' + ((e as Error)?.message ?? String(e));
        } finally {
            scanning.value = false;
        }
    };

    const startEdit = (d: Device) => {
        editingId.value = d.id;
        editDraft.value = d.name;
    };

    const saveEdit = async (d: Device) => {
        const name = editDraft.value.trim();
        editingId.value = null;
        if (!name || name === d.name) return;
        try {
            // upsert 只带 id + name,后端 mergeDevice 保留其余字段。
            const res = await fetch('/api/devices', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: d.id, name }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            await load();
        } catch (e) {
            msg.value =
                '❌ ' + t('settings.devices.renameFailed', language) + ': ' + ((e as Error)?.message ?? String(e));
        }
    };

    const remove = async (d: Device) => {
        try {
            const res = await fetch('/api/devices', {
                method: 'DELETE',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ id: d.id }),
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            await load();
        } catch (e) {
            msg.value =
                '❌ ' + t('settings.devices.removeFailed', language) + ': ' + ((e as Error)?.message ?? String(e));
        }
    };

    return (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.devices', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.devices.desc', language)}</div>

            {/* 扫描 / 刷新工具条 */}
            <div class="sys-settings-action-row" style="margin-bottom: 4px;">
                <button class="sys-settings-btn primary" disabled={scanning.value} onClick={scan}>
                    <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <circle cx="11" cy="11" r="8" />
                        <line x1="21" y1="21" x2="16.65" y2="16.65" />
                    </svg>
                    {scanning.value
                        ? t('settings.devices.scanning', language)
                        : t('settings.devices.scanBtn', language)}
                </button>
                <button class="sys-settings-btn ghost" disabled={loading.value} onClick={load}>
                    <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <polyline points="23 4 23 10 17 10" />
                        <polyline points="1 20 1 14 7 14" />
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                    </svg>
                    {t('settings.devices.refreshBtn', language)}
                </button>
            </div>

            {/* 设备卡片网格(Bento) */}
            {devices.value.length === 0 && !loading.value ? (
                <div class="sys-settings-card">
                    <span style="font-size: 13px; color: var(--text-muted);">
                        {t('settings.devices.empty', language)}
                    </span>
                </div>
            ) : (
                <div class="bento-grid">
                    {devices.value.map(d => {
                        const isEditing = d.id === editingId.value;
                        return (
                            <div class="bento-card" key={d.id}>
                                <div class="bento-zone-header">
                                    <div class="bento-card-icon">
                                        <OsIcon os={d.os} />
                                    </div>
                                    <div style="flex: 1; min-width: 0;">
                                        {isEditing ? (
                                            <input
                                                class="sys-settings-input"
                                                style="width: 100%; padding: 4px 8px; font-size: 14px;"
                                                value={editDraft.value}
                                                autoFocus
                                                onInput={e => (editDraft.value = (e.target as HTMLInputElement).value)}
                                                onKeyDown={e => {
                                                    if (e.key === 'Enter') void saveEdit(d);
                                                    if (e.key === 'Escape') editingId.value = null;
                                                }}
                                                onBlur={() => void saveEdit(d)}
                                            />
                                        ) : (
                                            <div
                                                class="bento-card-title"
                                                style="display: flex; align-items: center; gap: 6px;"
                                            >
                                                <span style="overflow: hidden; text-overflow: ellipsis; white-space: nowrap;">
                                                    {d.name || d.id}
                                                </span>
                                                {d.self && (
                                                    <span style="font-size: 10px; padding: 1px 6px; border-radius: 999px; background: var(--accent-bg); color: var(--accent-fg); flex-shrink: 0;">
                                                        {t('settings.devices.selfBadge', language)}
                                                    </span>
                                                )}
                                            </div>
                                        )}
                                        <div
                                            class="bento-card-desc"
                                            style="display: flex; align-items: center; gap: 6px; margin-top: 2px;"
                                        >
                                            <span
                                                style={`display: inline-block; width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0; background: ${
                                                    d.active ? 'var(--success-emphasis)' : 'var(--text-muted)'
                                                };`}
                                                aria-hidden="true"
                                            />
                                            <span
                                                style={`font-size: 12px; color: ${
                                                    d.active ? 'var(--success-fg)' : 'var(--text-muted)'
                                                };`}
                                            >
                                                {d.active
                                                    ? t('settings.devices.online', language)
                                                    : t('settings.devices.offline', language)}
                                            </span>
                                            <span style="font-size: 11px; color: var(--text-muted);">
                                                · {relativeTime(d.lastSeen, language)}
                                            </span>
                                        </div>
                                    </div>
                                </div>

                                <div class="bento-zone-body">
                                    <dl style="display: flex; flex-direction: column; gap: 4px; margin: 0; font-size: 12px;">
                                        {(d.tailscaleIp || d.address) && (
                                            <div style="display: flex; justify-content: space-between; gap: 8px;">
                                                <dt style="color: var(--text-muted);">
                                                    {t('settings.devices.address', language)}
                                                </dt>
                                                <dd
                                                    style="margin: 0; color: var(--text-secondary); font-family: var(--font-mono); overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                                                    title={d.tailscaleIp || d.address}
                                                >
                                                    {d.tailscaleIp || d.address}
                                                </dd>
                                            </div>
                                        )}
                                        {(d.os || d.arch) && (
                                            <div style="display: flex; justify-content: space-between; gap: 8px;">
                                                <dt style="color: var(--text-muted);">
                                                    {t('settings.devices.system', language)}
                                                </dt>
                                                <dd style="margin: 0; color: var(--text-secondary);">
                                                    {[d.os, d.arch].filter(Boolean).join(' / ')}
                                                </dd>
                                            </div>
                                        )}
                                        {d.version && (
                                            <div style="display: flex; justify-content: space-between; gap: 8px;">
                                                <dt style="color: var(--text-muted);">
                                                    {t('settings.devices.version', language)}
                                                </dt>
                                                <dd style="margin: 0; color: var(--text-secondary); font-family: var(--font-mono);">
                                                    {d.version}
                                                </dd>
                                            </div>
                                        )}
                                    </dl>
                                </div>

                                <div
                                    class="bento-zone-footer"
                                    style="display: flex; gap: 6px; justify-content: flex-end;"
                                >
                                    {isEditing ? (
                                        <button class="sys-settings-option-btn" onClick={() => void saveEdit(d)}>
                                            {t('common.save', language)}
                                        </button>
                                    ) : (
                                        <Fragment>
                                            <button class="sys-settings-option-btn" onClick={() => startEdit(d)}>
                                                {t('settings.devices.rename', language)}
                                            </button>
                                            {!d.self && (
                                                <button class="sys-settings-option-btn" onClick={() => void remove(d)}>
                                                    {t('settings.devices.remove', language)}
                                                </button>
                                            )}
                                        </Fragment>
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}

            {msg.value && (
                <div class="sys-settings-card" style="font-size: 13px; word-break: break-all; margin-top: 4px;">
                    {msg.value}
                </div>
            )}
        </div>
    );
}
