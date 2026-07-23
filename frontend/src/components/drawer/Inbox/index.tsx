import { h } from 'preact';
import { useState, useEffect, useCallback, useMemo } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import * as wsStore from '../../../stores/workspaceStore';
import { t } from '../../../i18n';
import { inboxService, type InboxItem, type InboxTarget } from '@1agents/core/services/inboxService';
import { InboxList } from './InboxList';
import { InboxDetail } from './InboxDetail';

// Workspace Inbox (#210): list + side drawer detail.
// Human triage: accept → requirement (toast, stay) | send_mail forward | archive.
// PMO cross-project dispatch removed from this UI.
export function InboxPane(props: { workspaceId?: string } = {}) {
    const language = ui.language.value;
    const workspaceId = props.workspaceId || wsStore.activeWorkspaceId.value || 'default';
    const workspaceName =
        wsStore.workspaces.value.find(w => w.id === workspaceId)?.name ||
        wsStore.findWorkspaceAnyStatus(workspaceId)?.name ||
        workspaceId;

    const [items, setItems] = useState<InboxItem[]>([]);
    const [unread, setUnread] = useState(0);
    const [showArchived, setShowArchived] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [draft, setDraft] = useState('');
    const [capturing, setCapturing] = useState(false);
    const [accepting, setAccepting] = useState<string | null>(null);
    const [forwarding, setForwarding] = useState(false);
    const [selectedId, setSelectedId] = useState<string | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            const res = await inboxService.list(showArchived, workspaceId);
            setItems(res.items || []);
            setUnread(res.unread);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [showArchived, workspaceId]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    // Clear selection when workspace / archive filter changes.
    useEffect(() => {
        setSelectedId(null);
    }, [workspaceId, showArchived]);

    // Keep selection bound to a live list row after refresh (status may change).
    const selectedItem = useMemo(
        () => (selectedId ? items.find(i => i.id === selectedId) || null : null),
        [items, selectedId]
    );

    useEffect(() => {
        if (selectedId && !selectedItem) {
            setSelectedId(null);
        }
    }, [selectedId, selectedItem]);

    const capture = async () => {
        const text = draft.trim();
        if (!text || capturing) return;
        setCapturing(true);
        setError('');
        try {
            const isUrl = /^https?:\/\/\S+$/i.test(text);
            await inboxService.capture(
                isUrl ? { workspaceId, url: text, source: 'manual' } : { workspaceId, content: text, source: 'manual' }
            );
            setDraft('');
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setCapturing(false);
        }
    };

    const act = async (id: string, action: 'archive' | 'read' | 'unread') => {
        setError('');
        try {
            await inboxService.setStatus(id, action, workspaceId);
            await refresh();
        } catch (err) {
            const msg = (err as Error).message;
            setError(msg);
            ui.showToast(msg);
        }
    };

    // Accept into the current Workspace requirement pool; toast + stay on Inbox (#212).
    const accept = async (item: InboxItem) => {
        if (accepting) return;
        setAccepting(item.id);
        setError('');
        try {
            await inboxService.accept(item.id, { workspaceId });
            ui.showToast(t('inbox.acceptDone', language));
            await refresh();
        } catch (err) {
            const msg = (err as Error).message;
            setError(msg);
            ui.showToast(t('inbox.acceptFailed', language, { err: msg }));
        } finally {
            setAccepting(null);
        }
    };

    // send_mail: deliver envelope to target Workspace (#213). Original stays, marked read.
    // Returns true on success so the detail picker can close.
    const forward = async (item: InboxItem, target: InboxTarget): Promise<boolean> => {
        if (forwarding) return false;
        // Guard: never deliver back into the current box (UI also filters this).
        if (target.projectId === workspaceId) return false;
        setForwarding(true);
        setError('');
        try {
            // Backend requires title | content | url; fall back summary → title when needed.
            const title = item.title || (!item.content && !item.url ? item.summary : undefined) || undefined;
            await inboxService.deliver({
                workspaceId: target.projectId,
                fromWorkspaceId: workspaceId,
                source: 'manual',
                title,
                content: item.content || undefined,
                url: item.url || undefined,
                summary: item.summary || undefined,
                tags: item.tags,
            });
            // Stay in this box; mark original read; do not archive.
            if (item.status === 'unread') {
                await inboxService.setStatus(item.id, 'read', workspaceId);
            }
            ui.showToast(t('inbox.forwardDone', language, { name: target.name }));
            await refresh();
            return true;
        } catch (err) {
            const msg = (err as Error).message;
            setError(msg);
            ui.showToast(t('inbox.forwardFailed', language, { err: msg }));
            return false;
        } finally {
            setForwarding(false);
        }
    };

    const loadTargets = useCallback(() => inboxService.listTargets(), []);

    const onKeyDown = (e: KeyboardEvent) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            capture();
        }
    };

    return (
        <div class="inbox-pane">
            <div class="inbox-header">
                <h2 class="inbox-title">
                    {t('inbox.title', language)}
                    <span class="inbox-workspace-tag" title={workspaceId}>
                        {workspaceName}
                    </span>
                    {unread > 0 && (
                        <span class="inbox-unread-pill">
                            {unread} {t('inbox.unreadBadge', language)}
                        </span>
                    )}
                </h2>
                <button type="button" class="inbox-archive-toggle" onClick={() => setShowArchived(v => !v)}>
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
                <button type="button" class="inbox-capture-btn" onClick={capture} disabled={!draft.trim() || capturing}>
                    {t('inbox.captureBtn', language)}
                </button>
            </div>

            {error && <div class="inbox-error">{error}</div>}

            <InboxList
                items={items}
                loading={loading}
                showArchived={showArchived}
                selectedId={selectedId}
                language={language}
                onSelect={item => setSelectedId(item.id)}
            />

            {selectedItem && (
                <InboxDetail
                    item={selectedItem}
                    language={language}
                    accepting={accepting}
                    forwarding={forwarding}
                    workspaceId={workspaceId}
                    onClose={() => setSelectedId(null)}
                    onAccept={accept}
                    onAct={act}
                    onForward={forward}
                    loadTargets={loadTargets}
                />
            )}
        </div>
    );
}
