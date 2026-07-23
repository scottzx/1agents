import { h, Fragment } from 'preact';
import type { ComponentChildren } from 'preact';
import { useEffect, useMemo, useState } from 'preact/hooks';

import { t, type Lang } from '../../../i18n';
import type { InboxItem, InboxTarget } from '@1agents/core/services/inboxService';
import { renderMarkdown } from '../../../utils/markdown';
import { sourceLabel } from './InboxList';

interface InboxDetailProps {
    item: InboxItem;
    language: Lang;
    accepting: string | null;
    forwarding?: boolean;
    workspaceId: string;
    onClose: () => void;
    onAccept: (item: InboxItem) => void;
    onAct: (id: string, action: 'archive' | 'read' | 'unread') => void;
    onForward: (item: InboxItem, target: InboxTarget) => Promise<boolean> | boolean;
    loadTargets: () => Promise<InboxTarget[]>;
}

function MetaRow({ label, children }: { label: string; children: ComponentChildren }) {
    if (children === null || children === undefined || children === '') return null;
    return (
        <div class="inbox-detail-meta-row">
            <span class="inbox-detail-meta-key">{label}</span>
            <span class="inbox-detail-meta-val">{children}</span>
        </div>
    );
}

export function InboxDetail({
    item,
    language,
    accepting,
    forwarding = false,
    workspaceId,
    onClose,
    onAccept,
    onAct,
    onForward,
    loadTargets,
}: InboxDetailProps) {
    const [forwardOpen, setForwardOpen] = useState(false);
    const [targets, setTargets] = useState<InboxTarget[]>([]);
    const [targetsLoading, setTargetsLoading] = useState(false);

    // Esc closes forward picker first, then clears selection.
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key !== 'Escape') return;
            if (forwardOpen) {
                setForwardOpen(false);
                return;
            }
            onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose, forwardOpen]);

    useEffect(() => {
        setForwardOpen(false);
        setTargets([]);
    }, [item.id]);

    const summaryHtml = useMemo(() => (item.summary ? renderMarkdown(item.summary) : ''), [item.summary]);
    const contentHtml = useMemo(() => (item.content ? renderMarkdown(item.content) : ''), [item.content]);

    const openForward = async () => {
        setForwardOpen(true);
        setTargetsLoading(true);
        try {
            const list = await loadTargets();
            setTargets(list.filter(tgt => tgt.projectId !== workspaceId));
        } catch {
            setTargets([]);
        } finally {
            setTargetsLoading(false);
        }
    };

    const pickTarget = async (tgt: InboxTarget) => {
        if (forwarding || tgt.projectId === workspaceId) return;
        const ok = await onForward(item, tgt);
        if (ok) setForwardOpen(false);
    };

    const archived = item.status === 'archived';
    const title = (item.title || '').trim() || '—';
    const hasBody = !!(item.summary || item.content || item.url);

    return (
        <div class="inbox-detail-panel" aria-label={t('inbox.detailAria', language)}>
            <header class="inbox-detail-head">
                <div class="inbox-detail-head-main">
                    <div class="inbox-detail-badges">
                        <span class={`inbox-source-tag source-${item.source}`}>
                            {sourceLabel(item.source, language)}
                        </span>
                        {item.status === 'unread' && (
                            <span class="inbox-detail-status-pill is-unread">{t('inbox.unreadBadge', language)}</span>
                        )}
                        {archived && (
                            <span class="inbox-detail-status-pill is-archived">{t('inbox.archive', language)}</span>
                        )}
                    </div>
                    <h3 class="inbox-detail-title">{title}</h3>
                </div>
                <button
                    type="button"
                    class="inbox-detail-close"
                    aria-label={t('common.close', language)}
                    onClick={onClose}
                >
                    ×
                </button>
            </header>

            <div class="inbox-detail-scroll">
                {item.url && (
                    <section class="inbox-detail-section">
                        <div class="inbox-detail-section-label">{t('inbox.field.url', language)}</div>
                        <a class="inbox-detail-link" href={item.url} target="_blank" rel="noopener noreferrer">
                            {item.url}
                        </a>
                    </section>
                )}

                {item.summary && (
                    <section class="inbox-detail-section">
                        <div class="inbox-detail-section-label">{t('inbox.field.summary', language)}</div>
                        <div class="inbox-detail-md markdown-body" dangerouslySetInnerHTML={{ __html: summaryHtml }} />
                    </section>
                )}

                {item.content && (
                    <section class="inbox-detail-section inbox-detail-section-body">
                        <div class="inbox-detail-section-label">{t('inbox.field.content', language)}</div>
                        <div class="inbox-detail-md markdown-body" dangerouslySetInnerHTML={{ __html: contentHtml }} />
                    </section>
                )}

                {!hasBody && <div class="inbox-detail-empty-body">{t('inbox.detailNoBody', language)}</div>}

                <section class="inbox-detail-section inbox-detail-section-meta">
                    <div class="inbox-detail-section-label">{t('inbox.field.meta', language)}</div>
                    <div class="inbox-detail-meta-list">
                        <MetaRow label={t('inbox.field.source', language)}>
                            {sourceLabel(item.source, language)}
                        </MetaRow>
                        <MetaRow label={t('inbox.fromWorkspace', language)}>{item.fromWorkspaceId || null}</MetaRow>
                        <MetaRow label={t('inbox.field.fromRef', language)}>{item.fromRef || null}</MetaRow>
                        <MetaRow label={t('inbox.field.createdAt', language)}>
                            {new Date(item.createdAt).toLocaleString(language)}
                        </MetaRow>
                        {item.tags && item.tags.length > 0 && (
                            <MetaRow label={t('inbox.field.tags', language)}>{item.tags.join(', ')}</MetaRow>
                        )}
                    </div>
                </section>

                {forwardOpen && !archived && (
                    <section class="inbox-detail-section inbox-forward-picker">
                        {targetsLoading ? (
                            <span class="inbox-forward-hint">{t('inbox.forwardLoading', language)}</span>
                        ) : targets.length === 0 ? (
                            <span class="inbox-forward-hint">{t('inbox.forwardNoTargets', language)}</span>
                        ) : (
                            <Fragment>
                                <span class="inbox-forward-hint">{t('inbox.forwardPickTarget', language)}</span>
                                <div class="inbox-forward-targets">
                                    {targets.map(tgt => (
                                        <button
                                            key={tgt.projectId}
                                            type="button"
                                            class="inbox-action"
                                            disabled={forwarding}
                                            onClick={() => pickTarget(tgt)}
                                        >
                                            {tgt.name}
                                        </button>
                                    ))}
                                </div>
                            </Fragment>
                        )}
                        <button
                            type="button"
                            class="inbox-action"
                            disabled={forwarding}
                            onClick={() => setForwardOpen(false)}
                        >
                            {t('inbox.forwardCancel', language)}
                        </button>
                    </section>
                )}
            </div>

            <footer class="inbox-detail-actions">
                {!archived && (
                    <button
                        type="button"
                        class="inbox-action inbox-action-primary"
                        onClick={() => onAccept(item)}
                        disabled={accepting === item.id || forwarding}
                    >
                        {t('inbox.accept', language)}
                    </button>
                )}
                {!archived && (
                    <button type="button" class="inbox-action" onClick={openForward} disabled={forwarding}>
                        {t('inbox.forward', language)}
                    </button>
                )}
                {item.status === 'unread' && (
                    <button type="button" class="inbox-action" onClick={() => onAct(item.id, 'read')}>
                        {t('inbox.markRead', language)}
                    </button>
                )}
                {item.status === 'read' && (
                    <button type="button" class="inbox-action" onClick={() => onAct(item.id, 'unread')}>
                        {t('inbox.markUnread', language)}
                    </button>
                )}
                <button
                    type="button"
                    class="inbox-action"
                    onClick={() => onAct(item.id, archived ? 'unread' : 'archive')}
                >
                    {t(archived ? 'inbox.unarchive' : 'inbox.archive', language)}
                </button>
            </footer>
        </div>
    );
}

/** Empty right pane when nothing is selected. */
export function InboxDetailEmpty({ language }: { language: Lang }) {
    return (
        <div class="inbox-detail-panel inbox-detail-panel-empty">
            <div class="inbox-detail-empty-hint">{t('inbox.detailEmpty', language)}</div>
        </div>
    );
}
