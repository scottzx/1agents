import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { inboxService, type InboxItem, type InboxSource } from '@1agents/core/services/inboxService';
import { pmoService, type DispatchTarget } from '@1agents/core/services/pmoService';

// Inbox 统一信息收口层 (#60): the most-upstream layer that funnels scattered
// external context (manual capture today; IM / email / RSS later) into one
// intake list. Archiving never deletes — it flips status so the trail of
// "what did this become" survives. PMO 分发 (#61) is the downstream action
// surfaced per item: dispatch an item into a project's requirement pool.
export function InboxPane() {
    const language = ui.language.value;
    const [items, setItems] = useState<InboxItem[]>([]);
    const [unread, setUnread] = useState(0);
    const [showArchived, setShowArchived] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [draft, setDraft] = useState('');
    const [capturing, setCapturing] = useState(false);
    // PMO 分发 (#61): the item currently being dispatched, plus the project menu.
    const [dispatchFor, setDispatchFor] = useState<string | null>(null);
    const [targets, setTargets] = useState<DispatchTarget[]>([]);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            const res = await inboxService.list(showArchived);
            setItems(res.items || []);
            setUnread(res.unread);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [showArchived]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const capture = async () => {
        const text = draft.trim();
        if (!text || capturing) return;
        setCapturing(true);
        try {
            // A bare URL becomes the url field; anything else is free-form content.
            const isUrl = /^https?:\/\/\S+$/i.test(text);
            await inboxService.capture(isUrl ? { url: text } : { content: text });
            setDraft('');
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setCapturing(false);
        }
    };

    const act = async (id: string, action: 'archive' | 'read' | 'unread') => {
        try {
            await inboxService.setStatus(id, action);
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    // Open the project picker for an item, lazily loading the dispatch targets.
    const openDispatch = async (id: string) => {
        setError('');
        setDispatchFor(id);
        try {
            setTargets(await pmoService.targets());
        } catch (err) {
            setError((err as Error).message);
        }
    };

    // Dispatch an inbox item into a project's requirement pool: title = the item
    // text, fromInbox = the item id (backlink + marks the item read).
    const dispatch = async (item: InboxItem, projectId: string) => {
        const title = (item.title || item.content || item.url || '').trim();
        if (!title) return;
        try {
            await pmoService.dispatch({ projectId, title, fromInbox: item.id });
            setDispatchFor(null);
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const onKeyDown = (e: KeyboardEvent) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            capture();
        }
    };

    const sourceLabel = (s: InboxSource) => t(`inbox.source.${s}`, language) || s;

    return (
        <div class="inbox-pane">
            <div class="inbox-header">
                <h2 class="inbox-title">
                    {t('inbox.title', language)}
                    {unread > 0 && (
                        <span class="inbox-unread-pill">
                            {unread} {t('inbox.unreadBadge', language)}
                        </span>
                    )}
                </h2>
                <button class="inbox-archive-toggle" onClick={() => setShowArchived(v => !v)}>
                    {t(showArchived ? 'inbox.hideArchived' : 'inbox.showArchived', language)}
                </button>
            </div>

            <div class="inbox-capture">
                <textarea
                    class="inbox-capture-input"
                    placeholder={t('inbox.capturePlaceholder', language)}
                    value={draft}
                    onInput={(e: Event) => setDraft((e.target as HTMLTextAreaElement).value)}
                    onKeyDown={onKeyDown}
                    rows={2}
                />
                <button class="inbox-capture-btn" onClick={capture} disabled={!draft.trim() || capturing}>
                    {t('inbox.captureBtn', language)}
                </button>
            </div>

            {error && <div class="inbox-error">{error}</div>}

            <div class="inbox-list">
                {!loading && items.length === 0 && (
                    <div class="inbox-empty">{t(showArchived ? 'inbox.emptyArchived' : 'inbox.empty', language)}</div>
                )}
                {items.map(item => {
                    const archived = item.status === 'archived';
                    const text = item.title || item.content || item.url || '';
                    return (
                        <div
                            key={item.id}
                            class={`inbox-item${item.status === 'unread' ? ' is-unread' : ''}${
                                archived ? ' is-archived' : ''
                            }`}
                        >
                            <div class="inbox-item-main">
                                <div class="inbox-item-meta">
                                    <span class={`inbox-source-tag source-${item.source}`}>
                                        {sourceLabel(item.source)}
                                    </span>
                                    <span class="inbox-item-time">
                                        {new Date(item.createdAt).toLocaleString(language)}
                                    </span>
                                </div>
                                {item.url ? (
                                    <a
                                        class="inbox-item-text inbox-item-link"
                                        href={item.url}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                    >
                                        {text}
                                    </a>
                                ) : (
                                    <div class="inbox-item-text">{text}</div>
                                )}
                            </div>
                            <div class="inbox-item-actions">
                                {item.status === 'unread' && (
                                    <button class="inbox-action" onClick={() => act(item.id, 'read')}>
                                        {t('inbox.markRead', language)}
                                    </button>
                                )}
                                {!archived && (
                                    <button class="inbox-action" onClick={() => openDispatch(item.id)}>
                                        {t('inbox.dispatch', language)}
                                    </button>
                                )}
                                <button
                                    class="inbox-action"
                                    onClick={() => act(item.id, archived ? 'unread' : 'archive')}
                                >
                                    {t(archived ? 'inbox.unarchive' : 'inbox.archive', language)}
                                </button>
                            </div>
                            {dispatchFor === item.id && (
                                <div class="inbox-dispatch-picker">
                                    {targets.length === 0 ? (
                                        <span class="inbox-dispatch-empty">
                                            {t('inbox.dispatchNoProjects', language)}
                                        </span>
                                    ) : (
                                        <>
                                            <span class="inbox-dispatch-label">
                                                {t('inbox.dispatchPickProject', language)}
                                            </span>
                                            {targets.map(tgt => (
                                                <button
                                                    key={tgt.projectId}
                                                    class="inbox-action"
                                                    onClick={() => dispatch(item, tgt.projectId)}
                                                >
                                                    {tgt.name}
                                                </button>
                                            ))}
                                        </>
                                    )}
                                    <button class="inbox-action" onClick={() => setDispatchFor(null)}>
                                        {t('inbox.dispatchCancel', language)}
                                    </button>
                                </div>
                            )}
                        </div>
                    );
                })}
            </div>
        </div>
    );
}
