import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import {
    contactService,
    channelService,
    type Contact,
    type ContactChannel,
    type FeishuMessage,
    type SessionSummary,
    type TrackedChat,
} from '@1agents/core/services/contactService';
import { ChannelsTab } from './ChannelsTab';
import { DataGrid } from '../TaskList/DataGrid';
import {
    getContactColumns,
    getContactGroupOptions,
    compareContacts,
    contactDefaultCompare,
    contactGroupValue,
    renderContactCell,
} from './contactGrid';
import { GroupChatBubbles } from './GroupChatBubbles';

// 联系人聚合 (Contacts aggregation): a user-curated address book over channel
// identities auto-discovered from synced Feishu messages. The 联系人 tab is a
// 多维表格 (DataGrid) whose rows open a detail MODAL; the 消息 tab keeps its own
// master-detail; 渠道 is the channel-config tab.

type Tab = 'contacts' | 'messages' | 'channels';

function initials(name: string): string {
    const n = name.trim();
    if (!n) return '?';
    // CJK: first char; latin: first letter of each word, max 2.
    if (/[一-龥]/.test(n)) return n.slice(0, 1);
    return n
        .split(/\s+/)
        .map(w => w[0])
        .join('')
        .slice(0, 2)
        .toUpperCase();
}

function shortId(id: string): string {
    return id.length > 10 ? `…${id.slice(-8)}` : id;
}

// orgLabel renders a member's tenant_key as a readable org hint via the company
// map (tenantKey → short company name). Falls back to a short tenant id when the
// tenant isn't mapped to any company. 飞书官方 is now seeded company data, not a
// hardcoded constant.
function orgLabel(tenantKey: string, companyMap: Record<string, string>): string {
    if (!tenantKey) return '';
    return companyMap[tenantKey] || shortId(tenantKey);
}

function msgText(m: FeishuMessage): string {
    if (m.MsgType === 'text') {
        try {
            const t = JSON.parse(m.Content) as { text?: string };
            if (t.text) return t.text.replace(/\n/g, ' ');
        } catch {
            /* fall through */
        }
    }
    return `[${m.MsgType}]`;
}

export function ContactsPane() {
    const language = ui.language.value;
    const tab = useSignal<Tab>('contacts');

    return (
        <div class="contacts-pane">
            <div class="contacts-view-switcher">
                {(
                    [
                        ['contacts', t('contacts.tab.contacts', language)],
                        ['messages', t('contacts.tab.messages', language)],
                        ['channels', t('contacts.tab.channels', language)],
                    ] as Array<[Tab, string]>
                ).map(([key, label]) => (
                    <button key={key} class={tab.value === key ? 'active' : ''} onClick={() => (tab.value = key)}>
                        {label}
                    </button>
                ))}
            </div>
            {tab.value === 'contacts' && <ContactsTab />}
            {tab.value === 'messages' && <MessagesTab />}
            {tab.value === 'channels' && <ChannelsTab />}
        </div>
    );
}

// ── Contacts tab ─────────────────────────────────────────────────────────────

function ContactsTab() {
    const language = ui.language.value;
    const [contacts, setContacts] = useState<Contact[]>([]);
    const [companyMap, setCompanyMap] = useState<Record<string, string>>({});
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    // Degree filter: 0 = all, 1 = first-degree, 2 = second-degree (group roster).
    const [degree, setDegree] = useState(0);
    const selectedId = useSignal<string | null>(null);
    const modalOpen = useSignal(false);
    const [editing, setEditing] = useState<Contact | 'new' | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setContacts(await contactService.listContacts(degree));
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [degree]);

    useEffect(() => {
        refresh();
    }, [refresh]);

    // Load the tenantKey → short company name map once (replaces the old
    // hardcoded 飞书官方 constant). Best-effort: labels fall back to short ids.
    useEffect(() => {
        let active = true;
        contactService
            .companies()
            .then(rows => {
                if (!active) return;
                const map: Record<string, string> = {};
                for (const r of rows) map[r.tenantKey] = r.shortName || r.fullName;
                setCompanyMap(map);
            })
            .catch(() => {
                /* best-effort */
            });
        return () => {
            active = false;
        };
    }, []);

    const discover = async () => {
        setError('');
        try {
            const res = await contactService.discover();
            await refresh();
            alert(t('contacts.discoverDone', language, { discovered: res.discovered }));
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const q = search.trim().toLowerCase();
    const filtered = q
        ? contacts.filter(
              c =>
                  c.name.toLowerCase().includes(q) ||
                  c.phone.toLowerCase().includes(q) ||
                  c.company.toLowerCase().includes(q)
          )
        : contacts;

    const selected = contacts.find(c => c.id === selectedId.value) || null;

    const openRow = (c: Contact) => {
        selectedId.value = c.id;
        modalOpen.value = true;
    };
    const closeModal = () => {
        modalOpen.value = false;
        selectedId.value = null;
    };

    return (
        <Fragment>
            <div class="contacts-list-toolbar">
                <input
                    class="contacts-search"
                    placeholder={t('contacts.searchPlaceholder', language)}
                    value={search}
                    onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                />
                <div class="contacts-toolbar-actions">
                    <button class="contacts-btn" onClick={() => setEditing('new')}>
                        {t('contacts.newContact', language)}
                    </button>
                    <button class="contacts-btn" onClick={discover}>
                        {t('contacts.discover', language)}
                    </button>
                </div>
            </div>
            <div class="contacts-degree-filter">
                {(
                    [
                        [0, t('contacts.degree.all', language)],
                        [1, t('contacts.degree.first', language)],
                        [2, t('contacts.degree.second', language)],
                    ] as Array<[number, string]>
                ).map(([d, label]) => (
                    <button
                        key={d}
                        class={`contacts-degree-chip${degree === d ? ' active' : ''}`}
                        onClick={() => setDegree(d)}
                    >
                        {label}
                    </button>
                ))}
            </div>
            {error && <div class="contacts-error">{error}</div>}

            <DataGrid<Contact>
                rows={filtered}
                totalCount={contacts.length}
                columns={getContactColumns(language)}
                groupOptions={getContactGroupOptions(language)}
                getRowKey={c => c.id}
                loading={loading}
                emptyAll={t('contacts.empty', language)}
                emptyFiltered={t('contacts.emptyFiltered', language)}
                compare={(a, b, key) => compareContacts(a, b, key, companyMap)}
                defaultCompare={contactDefaultCompare}
                groupValue={(c, key) => contactGroupValue(c, key, language, companyMap)}
                rowClass={() => 'task-row contact-grid-row'}
                onOpenRow={openRow}
                renderCell={(c, col, helpers) => renderContactCell(c, col, helpers, language, companyMap)}
            />

            {modalOpen.value && selected && (
                <ContactDetailModal
                    key={selected.id}
                    contact={selected}
                    companyMap={companyMap}
                    onClose={closeModal}
                    onEdit={() => setEditing(selected)}
                    onChanged={refresh}
                />
            )}
            {editing && (
                <ContactForm
                    initial={editing === 'new' ? null : editing}
                    onClose={() => setEditing(null)}
                    onSaved={async c => {
                        setEditing(null);
                        await refresh();
                        selectedId.value = c.id;
                        modalOpen.value = true;
                    }}
                />
            )}
        </Fragment>
    );
}

// ContactDetailModal — the contact detail in a modal overlay (NOT a side panel).
// Backdrop click + × button + Esc all close. The body is keyed by contact id in
// the parent so switching contacts remounts (refreshes the per-group state).
function ContactDetailModal({
    contact,
    companyMap,
    onClose,
    onEdit,
    onChanged,
}: {
    contact: Contact;
    companyMap: Record<string, string>;
    onClose: () => void;
    onEdit: () => void;
    onChanged: () => Promise<void> | void;
}) {
    const language = ui.language.value;
    const isMobile = ui.isMobile.value;
    const [picking, setPicking] = useState(false);
    const [unlinked, setUnlinked] = useState<ContactChannel[]>([]);
    const [error, setError] = useState('');

    const channels = contact.channels || [];

    // Esc closes the modal (a11y).
    useEffect(() => {
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        window.addEventListener('keydown', onKey);
        return () => window.removeEventListener('keydown', onKey);
    }, [onClose]);

    const remove = async () => {
        if (!confirm(t('contacts.deleteConfirm', language))) return;
        try {
            await contactService.deleteContact(contact.id);
            onClose();
            await onChanged();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const openPicker = async () => {
        setError('');
        setPicking(true);
        try {
            setUnlinked(await contactService.listChannels({ unlinked: true }));
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const link = async (channelId: string) => {
        try {
            await contactService.linkChannel(channelId, contact.id);
            setPicking(false);
            await onChanged();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    const unlink = async (channelId: string) => {
        try {
            await contactService.unlinkChannel(channelId);
            await onChanged();
        } catch (err) {
            setError((err as Error).message);
        }
    };

    return (
        <div class="contacts-detail-modal-overlay" onClick={onClose}>
            <div
                class={`contacts-detail-modal${isMobile ? ' mobile' : ''}`}
                role="dialog"
                aria-modal="true"
                onClick={(e: Event) => e.stopPropagation()}
            >
                <button
                    class="contacts-detail-modal-close"
                    aria-label={t('contacts.close', language)}
                    onClick={onClose}
                >
                    ×
                </button>
                <div class="contacts-detail">
                    <div class="contacts-detail-header">
                        <span class="contacts-avatar contacts-avatar-lg">
                            {initials(contact.name || contact.phone)}
                        </span>
                        <div class="contacts-detail-headinfo">
                            <h3 class="contacts-detail-name">{contact.name || '—'}</h3>
                            {contact.phone && <div class="contacts-detail-phone">{contact.phone}</div>}
                            {(contact.company || contact.title) && (
                                <div class="contacts-detail-org">
                                    {[contact.company, contact.title].filter(Boolean).join(' · ')}
                                </div>
                            )}
                        </div>
                        <div class="contacts-detail-actions">
                            <button class="contacts-btn" onClick={onEdit}>
                                {t('contacts.edit', language)}
                            </button>
                            <button class="contacts-btn contacts-btn-danger" onClick={remove}>
                                {t('contacts.delete', language)}
                            </button>
                        </div>
                    </div>

                    {contact.tags.length > 0 && (
                        <div class="contacts-tag-row">
                            {contact.tags.map(tag => (
                                <span key={tag} class="contacts-tag">
                                    {tag}
                                </span>
                            ))}
                        </div>
                    )}
                    {contact.note && <div class="contacts-detail-note">{contact.note}</div>}

                    {error && <div class="contacts-error">{error}</div>}

                    <div class="contacts-section">
                        <div class="contacts-section-head">
                            <span class="contacts-section-title">{t('contacts.channels', language)}</span>
                            <button class="contacts-btn" onClick={openPicker}>
                                {t('contacts.bindChannel', language)}
                            </button>
                        </div>
                        {channels.length === 0 && (
                            <div class="contacts-empty">{t('contacts.channelsEmpty', language)}</div>
                        )}
                        {channels.map(ch => (
                            <div key={ch.id} class="contacts-channel-row">
                                <span class="contacts-channel-platform">
                                    {t(`contacts.platform.${ch.platform}`, language)}
                                </span>
                                <div class="contacts-channel-main">
                                    <span class="contacts-channel-nick">{ch.nickname || '—'}</span>
                                    <span class="contacts-channel-id">{shortId(ch.channelId)}</span>
                                    {ch.tenantKey && (
                                        <span class="contacts-channel-tenant" title={ch.tenantKey}>
                                            {orgLabel(ch.tenantKey, companyMap)}
                                        </span>
                                    )}
                                </div>
                                <button class="contacts-btn contacts-btn-sm" onClick={() => unlink(ch.id)}>
                                    {t('contacts.unbind', language)}
                                </button>
                            </div>
                        ))}
                        {picking && (
                            <div class="contacts-bind-picker">
                                {unlinked.length === 0 ? (
                                    <span class="contacts-empty">{t('contacts.bindEmpty', language)}</span>
                                ) : (
                                    unlinked.map(ch => (
                                        <button
                                            key={ch.id}
                                            class="contacts-btn contacts-btn-sm"
                                            onClick={() => link(ch.id)}
                                        >
                                            {ch.nickname || shortId(ch.channelId)}
                                        </button>
                                    ))
                                )}
                                <button class="contacts-btn contacts-btn-sm" onClick={() => setPicking(false)}>
                                    {t('contacts.cancel', language)}
                                </button>
                            </div>
                        )}
                    </div>

                    <ContactGroupMessages contactId={contact.id} />
                </div>
            </div>
        </div>
    );
}

// 所在群 + 消息 — the tracked groups a contact belongs to (from the roster, GET
// /contacts/{id}/groups), each clickable to show THIS contact's messages in THAT
// group. Messages are per-group by default; the "全部群聊" chip merges across all
// groups (contactId-only query). Group membership comes from the roster because a
// person in N groups has ONE open_id channel but N roster rows. Messages render
// as multi-sender group-chat bubbles.
function ContactGroupMessages({ contactId }: { contactId: string }) {
    const language = ui.language.value;
    const [names, setNames] = useState<Record<string, TrackedChat>>({});
    const [sessionIds, setSessionIds] = useState<string[]>([]);
    const [sel, setSel] = useState<string | null>(null); // null=none, '*'=all groups, else sessionId
    const [msgs, setMsgs] = useState<FeishuMessage[]>([]);
    const [loading, setLoading] = useState(false);

    useEffect(() => {
        let active = true;
        Promise.all([channelService.trackedChats(), contactService.contactGroups(contactId)])
            .then(([chats, sids]) => {
                if (!active) return;
                const map: Record<string, TrackedChat> = {};
                for (const c of chats) map[c.chatId] = c;
                setNames(map);
                setSessionIds(sids);
            })
            .catch(() => {
                /* best-effort: groups still render with id fallback */
            });
        return () => {
            active = false;
        };
    }, [contactId]);

    const groupName = (sid: string) => names[sid]?.chatName || shortId(sid);

    const select = async (s: string) => {
        setSel(s);
        setLoading(true);
        try {
            const opts = s === '*' ? { contactId, limit: 300 } : { contactId, sessionId: s, limit: 300 };
            setMsgs(await contactService.messages(opts));
        } finally {
            setLoading(false);
        }
    };

    return (
        <div class="contacts-section">
            <div class="contacts-section-head">
                <span class="contacts-section-title">{t('contacts.groups', language)}</span>
            </div>
            {sessionIds.length === 0 ? (
                <div class="contacts-empty">{t('contacts.groupsEmpty', language)}</div>
            ) : (
                <div class="contacts-group-chips">
                    {sessionIds.map(sid => (
                        <button
                            key={sid}
                            class={`contacts-group-chip${sel === sid ? ' active' : ''}`}
                            onClick={() => select(sid)}
                        >
                            {groupName(sid)}
                            {names[sid]?.external && (
                                <span class="contacts-channels-badge ext">
                                    {t('contacts.channels.external', language)}
                                </span>
                            )}
                        </button>
                    ))}
                    <button
                        class={`contacts-group-chip all${sel === '*' ? ' active' : ''}`}
                        onClick={() => select('*')}
                    >
                        {t('contacts.allGroups', language)}
                    </button>
                </div>
            )}
            {sel && (
                <div class="contacts-chat-panel">
                    <div class="contacts-section-title">
                        {sel === '*' ? t('contacts.allGroups', language) : groupName(sel)}
                    </div>
                    {loading && <div class="contacts-empty">{t('contacts.loading', language)}</div>}
                    {!loading && msgs.length === 0 && (
                        <div class="contacts-empty">{t('contacts.timelineEmpty', language)}</div>
                    )}
                    {!loading && msgs.length > 0 && <GroupChatBubbles messages={msgs} language={language} />}
                </div>
            )}
        </div>
    );
}

function ContactForm({
    initial,
    onClose,
    onSaved,
}: {
    initial: Contact | null;
    onClose: () => void;
    onSaved: (c: Contact) => void;
}) {
    const language = ui.language.value;
    const [phone, setPhone] = useState(initial?.phone || '');
    const [name, setName] = useState(initial?.name || '');
    const [company, setCompany] = useState(initial?.company || '');
    const [title, setTitle] = useState(initial?.title || '');
    const [note, setNote] = useState(initial?.note || '');
    const [tags, setTags] = useState((initial?.tags || []).join(', '));
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);

    const submit = async (e: Event) => {
        e.preventDefault();
        setSaving(true);
        setError('');
        const input = {
            phone: phone.trim(),
            name: name.trim(),
            company: company.trim(),
            title: title.trim(),
            note: note.trim(),
            tags: tags
                .split(',')
                .map(s => s.trim())
                .filter(Boolean),
        };
        try {
            const c = initial
                ? await contactService.updateContact(initial.id, input)
                : await contactService.createContact(input);
            onSaved(c);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setSaving(false);
        }
    };

    return (
        <div class="contacts-modal-backdrop" onClick={onClose}>
            <form class="contacts-modal" onClick={(e: Event) => e.stopPropagation()} onSubmit={submit}>
                <label class="contacts-field">
                    <span>{t('contacts.field.phone', language)}</span>
                    <input value={phone} onInput={(e: Event) => setPhone((e.target as HTMLInputElement).value)} />
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.field.name', language)}</span>
                    <input value={name} onInput={(e: Event) => setName((e.target as HTMLInputElement).value)} />
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.field.company', language)}</span>
                    <input value={company} onInput={(e: Event) => setCompany((e.target as HTMLInputElement).value)} />
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.field.title', language)}</span>
                    <input value={title} onInput={(e: Event) => setTitle((e.target as HTMLInputElement).value)} />
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.field.tags', language)}</span>
                    <input value={tags} onInput={(e: Event) => setTags((e.target as HTMLInputElement).value)} />
                </label>
                <label class="contacts-field">
                    <span>{t('contacts.field.note', language)}</span>
                    <textarea
                        value={note}
                        rows={3}
                        onInput={(e: Event) => setNote((e.target as HTMLTextAreaElement).value)}
                    />
                </label>
                {error && <div class="contacts-error">{error}</div>}
                <div class="contacts-modal-actions">
                    <button type="button" class="contacts-btn" onClick={onClose}>
                        {t('contacts.cancel', language)}
                    </button>
                    <button type="submit" class="contacts-btn contacts-btn-primary" disabled={saving}>
                        {t('contacts.save', language)}
                    </button>
                </div>
            </form>
        </div>
    );
}

// ── Messages tab ─────────────────────────────────────────────────────────────

function MessagesTab() {
    const language = ui.language.value;
    const [sessions, setSessions] = useState<SessionSummary[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const selectedId = useSignal<string | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setSessions(await contactService.sessions());
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const selected = sessions.find(s => s.SessionID === selectedId.value) || null;
    const isMobile = ui.isMobile.value;
    const showDetailOnly = isMobile && selectedId.value !== null;

    const detail = selected ? (
        <SessionTimeline session={selected} onBack={() => (selectedId.value = null)} />
    ) : (
        <div class="contacts-detail-placeholder">{t('contacts.selectSession', language)}</div>
    );

    const list = (
        <Fragment>
            {error && <div class="contacts-error">{error}</div>}
            <div class="contacts-list">
                {!loading && sessions.length === 0 && (
                    <div class="contacts-empty">{t('contacts.emptySessions', language)}</div>
                )}
                {sessions.map(s => (
                    <div
                        key={s.SessionID}
                        class={`contacts-list-item${selectedId.value === s.SessionID ? ' selected' : ''}`}
                        onClick={() => (selectedId.value = s.SessionID)}
                    >
                        <div class="contacts-list-item-main">
                            <div class="contacts-list-item-top">
                                <span class="contacts-list-name" title={s.SessionName}>
                                    {s.SessionName}
                                </span>
                                <span class="contacts-channel-badge">{s.Count}</span>
                            </div>
                            {s.LastPreview && <div class="contacts-list-sub">{s.LastPreview}</div>}
                            {s.LastTime > 0 && (
                                <div class="contacts-list-time">{new Date(s.LastTime).toLocaleString(language)}</div>
                            )}
                        </div>
                    </div>
                ))}
            </div>
        </Fragment>
    );

    return (
        <div class="contacts-split">
            {!showDetailOnly && <div class="contacts-list-col">{list}</div>}
            <div class="contacts-detail-col">{detail}</div>
        </div>
    );
}

function SessionTimeline({ session, onBack }: { session: SessionSummary; onBack: () => void }) {
    const language = ui.language.value;
    const [msgs, setMsgs] = useState<FeishuMessage[]>([]);
    const [loading, setLoading] = useState(false);
    const isMobile = ui.isMobile.value;

    useEffect(() => {
        let active = true;
        setLoading(true);
        contactService
            .messages({ sessionId: session.SessionID, limit: 300 })
            .then(m => active && setMsgs(m))
            .finally(() => active && setLoading(false));
        return () => {
            active = false;
        };
    }, [session.SessionID]);

    return (
        <div class="contacts-detail">
            {isMobile && (
                <button class="contacts-detail-back" onClick={onBack}>
                    ← {t('contacts.back', language)}
                </button>
            )}
            <h3 class="contacts-detail-name" title={session.SessionName}>
                {session.SessionName}
            </h3>
            <div class="contacts-timeline">
                {!loading && msgs.length === 0 && (
                    <div class="contacts-empty">{t('contacts.timelineEmpty', language)}</div>
                )}
                {msgs.map(m => (
                    <div key={m.MessageID} class="contacts-msg-line">
                        <span class="contacts-msg-time">{new Date(m.CreateTime).toLocaleString(language)}</span>
                        <span class="contacts-msg-sender">{m.SenderName || shortId(m.SenderID)}:</span>
                        <span class="contacts-msg-text">{msgText(m)}</span>
                    </div>
                ))}
            </div>
        </div>
    );
}
