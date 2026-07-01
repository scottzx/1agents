import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import {
    channelService,
    type ChatInfo,
    type TrackedChat,
    type ChannelStatus,
} from '@1agents/core/services/contactService';
import { sourceService } from '@1agents/core/services/sourceService';

// 飞书群消息子模块 — browse the user's Feishu groups via lark-cli, pick which to
// track, and sync manually or automatically. Extracted from the old ChannelsTab so
// the 渠道 page can group it under the 飞书 channel alongside other sub-modules. All
// fetch/dedup reuses the existing SyncChat watermark loop server-side (no AI).

const INTERVALS = [60, 180, 360, 720, 1440];

function chatLabel(c: { name?: string; description?: string; chatId: string }): string {
    return c.name?.trim() || c.description?.trim() || c.chatId;
}

function avatarInitial(label: string): string {
    const n = label.trim();
    if (!n) return '#';
    if (/[一-龥]/.test(n)) return n.slice(0, 1);
    return n[0].toUpperCase();
}

export function FeishuSection() {
    const language = ui.language.value;
    const [status, setStatus] = useState<ChannelStatus | null>(null);
    const [tracked, setTracked] = useState<TrackedChat[]>([]);
    const [available, setAvailable] = useState<ChatInfo[]>([]);
    const [error, setError] = useState('');
    const [toast, setToast] = useState('');
    const [loadingChats, setLoadingChats] = useState(false);
    const [chatsLoaded, setChatsLoaded] = useState(false);
    const [syncingAll, setSyncingAll] = useState(false);
    const [busyChat, setBusyChat] = useState<string | null>(null);

    const refreshTracked = useCallback(async () => {
        setError('');
        try {
            setTracked(await channelService.trackedChats());
        } catch (err) {
            setError((err as Error).message);
        }
    }, []);

    const refreshStatus = useCallback(async () => {
        try {
            setStatus(await channelService.status());
        } catch (err) {
            setError((err as Error).message);
        }
    }, []);

    useEffect(() => {
        refreshStatus();
        refreshTracked();
    }, [refreshStatus, refreshTracked]);

    const loadAvailable = async () => {
        setLoadingChats(true);
        setError('');
        try {
            setAvailable(await channelService.availableChats());
            setChatsLoaded(true);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoadingChats(false);
        }
    };

    const trackedIds = new Set(tracked.map(c => c.chatId));

    const track = async (c: ChatInfo) => {
        setBusyChat(c.chatId);
        setError('');
        try {
            await channelService.trackChat({
                chatId: c.chatId,
                chatName: chatLabel(c),
                avatar: c.avatar,
                external: c.external,
            });
            await refreshTracked();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setBusyChat(null);
        }
    };

    const untrack = async (chatId: string) => {
        setBusyChat(chatId);
        setError('');
        try {
            await channelService.untrackChat(chatId);
            await refreshTracked();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setBusyChat(null);
        }
    };

    const toggleAutoSync = async (chatId: string, on: boolean) => {
        setError('');
        try {
            await channelService.setChatAutoSync(chatId, on);
            await refreshTracked();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    // Both per-chat and all-chats sync now dispatch the feishu_message work-order
    // task — one loop for everything. The task's function handler pulls raw
    // messages into bronze (source_records) AND drives the proven digest sync
    // (unified_messages + 二度联系人), so both views stay in step. The task runs
    // async; we refresh the tracked list after a short delay so the last-synced
    // timestamps and any new bronze rows are visible.
    // Both sync buttons dispatch the same work-order task — the handler pulls
    // raw messages into bronze AND drives the proven digest sync
    // (unified_messages + 二度联系人), so bronze and the message UI stay in step.
    // The per-chat button intentionally triggers a full-tracked-set pass (small
    // over-fetch to keep a single sync path).
    const dispatchMessageSync = async () => {
        await sourceService.syncNow('feishu', 'feishu_message');
        window.setTimeout(() => refreshTracked(), 6500);
    };

    const syncOne = async (chatId: string) => {
        setBusyChat(chatId);
        setError('');
        try {
            await dispatchMessageSync();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            window.setTimeout(() => setBusyChat(null), 6500);
        }
    };

    const syncAll = async () => {
        setSyncingAll(true);
        setError('');
        setToast('');
        try {
            await dispatchMessageSync();
            setToast(t('contacts.channels.syncDispatched', language));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            window.setTimeout(() => setSyncingAll(false), 6500);
        }
    };

    const setConfig = async (enabled: boolean, intervalMinutes: number) => {
        setError('');
        try {
            const cfg = await channelService.setSyncConfig(enabled, intervalMinutes);
            setStatus(s => (s ? { ...s, config: cfg } : s));
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const cfg = status?.config;
    const lastSynced = (ms: number) =>
        ms > 0 ? new Date(ms).toLocaleString(language) : t('contacts.channels.neverSynced', language);

    return (
        <div class="contacts-channels-body">
            <div class={`contacts-channels-banner ${status?.connected ? 'ok' : 'bad'}`}>
                <span class="contacts-channels-dot" />
                <span class="contacts-channels-banner-text">
                    {status?.connected
                        ? t('contacts.channels.connected', language)
                        : t('contacts.channels.disconnected', language)}
                </span>
                {status && !status.connected && status.error && (
                    <span class="contacts-channels-banner-err">{status.error}</span>
                )}
                {status?.checks?.map(c => (
                    <span key={c.name} class={`contacts-channels-check ${c.status}`}>
                        {c.name}: {c.message || c.status}
                    </span>
                ))}
            </div>

            {error && <div class="contacts-error">{error}</div>}
            {toast && <div class="contacts-channels-toast">{toast}</div>}

            {cfg && (
                <div class="contacts-channels-config">
                    <label class="contacts-channels-toggle">
                        <input
                            type="checkbox"
                            checked={cfg.enabled}
                            onChange={(e: Event) =>
                                setConfig((e.target as HTMLInputElement).checked, cfg.intervalMinutes)
                            }
                        />
                        <span>{t('contacts.channels.autoSync', language)}</span>
                    </label>
                    <label class="contacts-channels-interval">
                        <span>{t('contacts.channels.interval', language)}</span>
                        <select
                            value={String(cfg.intervalMinutes)}
                            onChange={(e: Event) =>
                                setConfig(cfg.enabled, Number((e.target as HTMLSelectElement).value))
                            }
                        >
                            {INTERVALS.map(m => (
                                <option key={m} value={String(m)}>
                                    {t(`contacts.channels.interval.${m}`, language)}
                                </option>
                            ))}
                        </select>
                    </label>
                    <button class="contacts-btn contacts-btn-primary" disabled={syncingAll} onClick={syncAll}>
                        {syncingAll
                            ? t('contacts.channels.syncing', language)
                            : t('contacts.channels.syncAll', language)}
                    </button>
                </div>
            )}

            <div class="contacts-section">
                <div class="contacts-section-head">
                    <span class="contacts-section-title">{t('contacts.channels.tracked', language)}</span>
                </div>
                {tracked.length === 0 && (
                    <div class="contacts-empty">{t('contacts.channels.trackedEmpty', language)}</div>
                )}
                {tracked.map(c => (
                    <div key={c.chatId} class="contacts-channels-row">
                        <span class="contacts-avatar">{avatarInitial(c.chatName || c.chatId)}</span>
                        <div class="contacts-channels-row-main">
                            <div class="contacts-channels-row-top">
                                <span class="contacts-channels-name">{c.chatName || c.chatId}</span>
                                <span class={`contacts-channels-badge ${c.external ? 'ext' : 'int'}`}>
                                    {c.external
                                        ? t('contacts.channels.external', language)
                                        : t('contacts.channels.internal', language)}
                                </span>
                            </div>
                            <div class="contacts-channels-row-sub">
                                {t('contacts.channels.lastSynced', language)}: {lastSynced(c.lastSyncedAt)}
                                {' · '}
                                {t('contacts.channels.memberCount', language)}: {c.memberTotal || c.memberCount || 0}
                                {(c.memberTotal ?? 0) > (c.memberCount ?? 0) && (
                                    <span>
                                        {' '}
                                        ({t('contacts.channels.ingested', language)} {c.memberCount ?? 0})
                                    </span>
                                )}
                            </div>
                        </div>
                        <label class="contacts-channels-toggle contacts-channels-toggle-sm">
                            <input
                                type="checkbox"
                                checked={c.autoSync}
                                onChange={(e: Event) =>
                                    toggleAutoSync(c.chatId, (e.target as HTMLInputElement).checked)
                                }
                            />
                            <span>{t('contacts.channels.autoSync', language)}</span>
                        </label>
                        <button
                            class="contacts-btn contacts-btn-sm"
                            disabled={busyChat === c.chatId}
                            onClick={() => syncOne(c.chatId)}
                        >
                            {t('contacts.channels.syncNow', language)}
                        </button>
                        <button
                            class="contacts-btn contacts-btn-sm contacts-btn-danger"
                            disabled={busyChat === c.chatId}
                            onClick={() => untrack(c.chatId)}
                        >
                            {t('contacts.channels.untrack', language)}
                        </button>
                    </div>
                ))}
            </div>

            <div class="contacts-section">
                <div class="contacts-section-head">
                    <span class="contacts-section-title">{t('contacts.channels.available', language)}</span>
                    <button class="contacts-btn" disabled={loadingChats} onClick={loadAvailable}>
                        {loadingChats
                            ? t('contacts.channels.loadingChats', language)
                            : t('contacts.channels.refresh', language)}
                    </button>
                </div>
                {!chatsLoaded && !loadingChats && (
                    <div class="contacts-empty">{t('contacts.channels.availableHint', language)}</div>
                )}
                {chatsLoaded && available.length === 0 && !loadingChats && (
                    <div class="contacts-empty">{t('contacts.channels.availableEmpty', language)}</div>
                )}
                {available.map(c => {
                    const isTracked = trackedIds.has(c.chatId);
                    return (
                        <div key={c.chatId} class="contacts-channels-row">
                            <span class="contacts-avatar">{avatarInitial(chatLabel(c))}</span>
                            <div class="contacts-channels-row-main">
                                <div class="contacts-channels-row-top">
                                    <span class="contacts-channels-name">{chatLabel(c)}</span>
                                    <span class={`contacts-channels-badge ${c.external ? 'ext' : 'int'}`}>
                                        {c.external
                                            ? t('contacts.channels.external', language)
                                            : t('contacts.channels.internal', language)}
                                    </span>
                                </div>
                            </div>
                            {isTracked ? (
                                <button class="contacts-btn contacts-btn-sm" disabled>
                                    {t('contacts.channels.tracked.badge', language)}
                                </button>
                            ) : (
                                <button
                                    class="contacts-btn contacts-btn-sm contacts-btn-primary"
                                    disabled={busyChat === c.chatId}
                                    onClick={() => track(c)}
                                >
                                    {t('contacts.channels.track', language)}
                                </button>
                            )}
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
