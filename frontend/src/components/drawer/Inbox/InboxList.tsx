import { h } from 'preact';

import { t, type Lang } from '../../../i18n';
import type { InboxItem, InboxSource } from '@1agents/core/services/inboxService';

export function sourceLabel(source: InboxSource | string, language: Lang): string {
    return t(`inbox.source.${source}`, language) || source;
}

/** One-line preview for list scan: summary → title → content → url. */
export function itemPreview(item: InboxItem): string {
    return (item.summary || item.title || item.content || item.url || '').trim();
}

interface InboxListProps {
    items: InboxItem[];
    loading: boolean;
    showArchived: boolean;
    selectedId: string | null;
    language: Lang;
    onSelect: (item: InboxItem) => void;
}

export function InboxList({ items, loading, showArchived, selectedId, language, onSelect }: InboxListProps) {
    return (
        <div class="inbox-list" role="list">
            {!loading && items.length === 0 && (
                <div class="inbox-empty">{t(showArchived ? 'inbox.emptyArchived' : 'inbox.empty', language)}</div>
            )}
            {items.map(item => {
                const archived = item.status === 'archived';
                const unread = item.status === 'unread';
                const selected = selectedId === item.id;
                const preview = itemPreview(item);

                return (
                    <button
                        key={item.id}
                        type="button"
                        role="listitem"
                        class={`inbox-item${unread ? ' is-unread' : ''}${
                            archived ? ' is-archived' : ''
                        }${selected ? ' is-selected' : ''}`}
                        onClick={() => onSelect(item)}
                    >
                        <div class="inbox-item-main">
                            <div class="inbox-item-meta">
                                <span class={`inbox-source-tag source-${item.source}`}>
                                    {sourceLabel(item.source, language)}
                                </span>
                                <span class="inbox-item-time">{new Date(item.createdAt).toLocaleString(language)}</span>
                            </div>
                            <div class="inbox-item-summary">{preview || '—'}</div>
                        </div>
                        {unread && <span class="inbox-item-unread-dot" aria-hidden="true" />}
                    </button>
                );
            })}
        </div>
    );
}
