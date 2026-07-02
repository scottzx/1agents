import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
import { sourceService, type CachedChat } from '@1agents/core/services/sourceService';
import { channelService, channelModuleService } from '@1agents/core/services/contactService';

// ChatScopeModal — the 群 modal for the 飞书 source, two modes over ONE cached
// list (bronze feishu_chat rows; opening it never calls lark-cli):
//   view — 群列表 row: browse the cached groups; 刷新 dispatches a 群列表 sync
//          work-order task and re-reads the cache.
//   pick — 群消息 row: check which groups are in the message-sync scope
//          (feishu_tracked_chats). Privacy-gated: third-party chat content, so
//          the feishu.groups consent must be granted before picking.
// Cadence is NOT here — 更新机制统一 at the kind level (采集配置的增量频率).

const MOD_FEISHU = 'feishu.groups';

function fmtCachedAt(ms: number, language: Lang): string {
    if (!ms) return t('datasource.chats.neverSynced', language);
    return `${t('datasource.chats.cachedAt', language)} ${new Date(ms).toLocaleString(language)}`;
}

export function ChatScopeModal({ mode, onClose }: { mode: 'view' | 'pick'; onClose: () => void }) {
    const language = ui.language.value;
    const [chats, setChats] = useState<CachedChat[]>([]);
    const [cachedAt, setCachedAt] = useState(0);
    const [error, setError] = useState('');
    const [refreshing, setRefreshing] = useState(false);
    const [busyChat, setBusyChat] = useState<string | null>(null);
    // pick mode only: null = loading, false = gate, true = granted.
    const [consented, setConsented] = useState<boolean | null>(mode === 'pick' ? null : true);

    const load = useCallback(async () => {
        setError('');
        try {
            const resp = await sourceService.feishuChats();
            setChats(resp.chats);
            setCachedAt(resp.cachedAt);
        } catch (e) {
            setError((e as Error).message);
        }
    }, []);

    useEffect(() => {
        load();
        if (mode === 'pick') {
            channelModuleService
                .list()
                .then(mods => setConsented(mods.find(m => m.id === MOD_FEISHU)?.consented ?? false))
                .catch(e => setError((e as Error).message));
        }
    }, [load, mode]);

    // 刷新 = dispatch the 群列表 sync work-order task, then re-read the cache
    // once the scheduler (5s tick) has run it.
    const refresh = async () => {
        setRefreshing(true);
        setError('');
        try {
            await sourceService.syncNow('feishu', 'feishu_chat');
            window.setTimeout(async () => {
                await load();
                setRefreshing(false);
            }, 7000);
        } catch (e) {
            setError((e as Error).message);
            setRefreshing(false);
        }
    };

    const toggle = async (chat: CachedChat) => {
        setBusyChat(chat.chatId);
        setError('');
        try {
            if (chat.tracked) {
                await channelService.untrackChat(chat.chatId);
            } else {
                await channelService.trackChat({
                    chatId: chat.chatId,
                    chatName: chat.name || chat.description,
                    avatar: chat.avatar,
                    external: chat.external,
                });
            }
            setChats(prev => prev.map(c => (c.chatId === chat.chatId ? { ...c, tracked: !c.tracked } : c)));
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setBusyChat(null);
        }
    };

    const grant = async () => {
        setError('');
        try {
            await channelModuleService.consent(MOD_FEISHU);
            setConsented(true);
        } catch (e) {
            setError((e as Error).message);
        }
    };

    const pickedCount = chats.filter(c => c.tracked).length;
    const title =
        mode === 'pick' ? t('datasource.chats.pickTitle', language) : t('datasource.chats.viewTitle', language);

    return (
        <div class="ds-record-overlay" onClick={onClose}>
            <div
                class="ds-record-modal ds-chats-modal"
                role="dialog"
                aria-modal="true"
                onClick={(e: Event) => e.stopPropagation()}
            >
                <div class="ds-chats-head">
                    <span class="ds-chats-title">{title}</span>
                    {mode === 'pick' && (
                        <span class="ds-chats-count">
                            {t('datasource.chats.picked', language, { n: pickedCount, total: chats.length })}
                        </span>
                    )}
                    <span class="ds-chats-cached">{fmtCachedAt(cachedAt, language)}</span>
                    <button class="contacts-btn contacts-btn-sm" disabled={refreshing} onClick={refresh}>
                        {refreshing
                            ? t('datasource.chats.refreshing', language)
                            : t('datasource.chats.refresh', language)}
                    </button>
                    <button class="ds-chats-close" onClick={onClose} aria-label="close">
                        ✕
                    </button>
                </div>

                {error && <div class="contacts-error">{error}</div>}

                {mode === 'pick' && consented === false ? (
                    <div class="contacts-consent-gate">
                        <p>{t('contacts.privacy.moduleNotice', language)}</p>
                        <button class="contacts-btn contacts-btn-primary contacts-btn-sm" onClick={grant}>
                            {t('contacts.privacy.authorize', language)}
                        </button>
                    </div>
                ) : (
                    <div class="ds-chats-list">
                        {chats.length === 0 && !error && (
                            <div class="contacts-empty">{t('datasource.chats.empty', language)}</div>
                        )}
                        {chats.map(chat => (
                            <div key={chat.chatId} class="ds-chat-row">
                                {mode === 'pick' && (
                                    <input
                                        type="checkbox"
                                        checked={chat.tracked}
                                        disabled={busyChat === chat.chatId || consented !== true}
                                        onChange={() => toggle(chat)}
                                    />
                                )}
                                {chat.avatar ? (
                                    <img class="ds-chat-avatar" src={chat.avatar} alt="" />
                                ) : (
                                    <span class="ds-chat-avatar ds-chat-avatar-fallback">
                                        {(chat.name || chat.description || '#').slice(0, 1)}
                                    </span>
                                )}
                                <div class="ds-chat-info">
                                    <span class="ds-chat-name">{chat.name || chat.description || chat.chatId}</span>
                                    {chat.description && chat.name && (
                                        <span class="ds-chat-desc">{chat.description}</span>
                                    )}
                                </div>
                                {chat.external && (
                                    <span class="fscard-badge warn">{t('contacts.channels.external', language)}</span>
                                )}
                                {mode === 'view' && chat.tracked && (
                                    <span class="fscard-badge ok">{t('datasource.chats.inScope', language)}</span>
                                )}
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}
