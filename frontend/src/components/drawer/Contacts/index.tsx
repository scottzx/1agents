import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import * as ui from '../../../stores/uiStore';
import { t, type Lang } from '../../../i18n';
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

// 联系人聚合 (Contacts aggregation): a user-curated address book over channel
// identities auto-discovered from synced Feishu messages. Two tabs — 联系人 and
// 消息 — share one responsive master-detail shell (desktop: list + detail side
// by side; mobile: list, then detail full-screen with a back button).

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

// Feishu's official tenant — degree-2 members carrying this tenant_key are
// Feishu/Lark official operations staff (verified across multiple groups).
const FEISHU_OFFICIAL_TENANT = '736588c9260f175d';

// orgLabel renders a member's tenant_key as a readable org hint: the known
// Feishu-official tenant gets a friendly label, others show a short id. Distinct
// tenant_keys are how two same-named people (e.g. two "但妮") are told apart.
function orgLabel(tenantKey: string, language: Lang): string {
    if (!tenantKey) return '';
    if (tenantKey === FEISHU_OFFICIAL_TENANT) return t('contacts.org.feishuOfficial', language);
    return shortId(tenantKey);
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
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [search, setSearch] = useState('');
    // Degree filter: 0 = all, 1 = first-degree, 2 = second-degree (group roster).
    const [degree, setDegree] = useState(0);
    const selectedId = useSignal<string | null>(null);
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
    const isMobile = ui.isMobile.value;
    const showDetailOnly = isMobile && selectedId.value !== null;

    const detail = selected ? (
        <ContactDetail
            key={selected.id}
            contact={selected}
            onBack={() => (selectedId.value = null)}
            onEdit={() => setEditing(selected)}
            onChanged={refresh}
        />
    ) : (
        <div class="contacts-detail-placeholder">{t('contacts.selectContact', language)}</div>
    );

    const list = (
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
            <div class="contacts-list">
                {!loading && filtered.length === 0 && <div class="contacts-empty">{t('contacts.empty', language)}</div>}
                {filtered.map(c => {
                    const feishuChans = (c.channels || []).filter(ch => ch.platform === 'feishu');
                    const feishuCount = feishuChans.length;
                    // org of the first feishu identity — distinguishes same-named
                    // people (e.g. two "但妮" from different tenants).
                    const org = feishuChans.find(ch => ch.tenantKey)?.tenantKey;
                    return (
                        <div
                            key={c.id}
                            class={`contacts-list-item${selectedId.value === c.id ? ' selected' : ''}`}
                            onClick={() => (selectedId.value = c.id)}
                        >
                            <span class="contacts-avatar">{initials(c.name || c.phone)}</span>
                            <div class="contacts-list-item-main">
                                <div class="contacts-list-item-top">
                                    <span class="contacts-list-name">
                                        {c.name || t('contacts.field.name', language)}
                                    </span>
                                    <span class={`contacts-degree-badge deg-${c.degree === 2 ? 2 : 1}`}>
                                        {c.degree === 2
                                            ? t('contacts.degree.second', language)
                                            : t('contacts.degree.first', language)}
                                    </span>
                                    {c.phone && <span class="contacts-list-phone">{c.phone}</span>}
                                    {feishuCount > 0 && (
                                        <span class="contacts-channel-badge">
                                            {t('contacts.platform.feishu', language)}×{feishuCount}
                                        </span>
                                    )}
                                    {org && (
                                        <span
                                            class={`contacts-channel-tenant${
                                                org === FEISHU_OFFICIAL_TENANT ? ' official' : ''
                                            }`}
                                            title={org}
                                        >
                                            {orgLabel(org, language)}
                                        </span>
                                    )}
                                </div>
                                {(c.company || c.title) && (
                                    <div class="contacts-list-sub">
                                        {[c.company, c.title].filter(Boolean).join(' · ')}
                                    </div>
                                )}
                            </div>
                        </div>
                    );
                })}
            </div>
        </Fragment>
    );

    return (
        <Fragment>
            <div class="contacts-split">
                {!showDetailOnly && <div class="contacts-list-col">{list}</div>}
                {!isMobile && <div class="contacts-detail-col">{detail}</div>}
                {showDetailOnly && <div class="contacts-detail-col">{detail}</div>}
            </div>
            {editing && (
                <ContactForm
                    initial={editing === 'new' ? null : editing}
                    onClose={() => setEditing(null)}
                    onSaved={async c => {
                        setEditing(null);
                        await refresh();
                        selectedId.value = c.id;
                    }}
                />
            )}
        </Fragment>
    );
}

function ContactDetail({
    contact,
    onBack,
    onEdit,
    onChanged,
}: {
    contact: Contact;
    onBack: () => void;
    onEdit: () => void;
    onChanged: () => Promise<void> | void;
}) {
    const language = ui.language.value;
    const isMobile = ui.isMobile.value;
    const [picking, setPicking] = useState(false);
    const [unlinked, setUnlinked] = useState<ContactChannel[]>([]);
    const [error, setError] = useState('');

    const channels = contact.channels || [];

    const remove = async () => {
        if (!confirm(t('contacts.deleteConfirm', language))) return;
        try {
            await contactService.deleteContact(contact.id);
            onBack();
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
        <div class="contacts-detail">
            {isMobile && (
                <button class="contacts-detail-back" onClick={onBack}>
                    ← {t('contacts.back', language)}
                </button>
            )}
            <div class="contacts-detail-header">
                <span class="contacts-avatar contacts-avatar-lg">{initials(contact.name || contact.phone)}</span>
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
                {channels.length === 0 && <div class="contacts-empty">{t('contacts.channelsEmpty', language)}</div>}
                {channels.map(ch => (
                    <div key={ch.id} class="contacts-channel-row">
                        <span class="contacts-channel-platform">{t(`contacts.platform.${ch.platform}`, language)}</span>
                        <div class="contacts-channel-main">
                            <span class="contacts-channel-nick">{ch.nickname || '—'}</span>
                            <span class="contacts-channel-id">{shortId(ch.channelId)}</span>
                            {ch.tenantKey && (
                                <span
                                    class={`contacts-channel-tenant${
                                        ch.tenantKey === FEISHU_OFFICIAL_TENANT ? ' official' : ''
                                    }`}
                                    title={ch.tenantKey}
                                >
                                    {orgLabel(ch.tenantKey, language)}
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
                                <button key={ch.id} class="contacts-btn contacts-btn-sm" onClick={() => link(ch.id)}>
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
    );
}

// 所在群 + 消息 — the tracked groups a contact belongs to (from the roster, GET
// /contacts/{id}/groups), each clickable to show THIS contact's messages in THAT
// group. Messages are per-group by default; the "全部群聊" chip merges across all
// groups (contactId-only query). Group membership comes from the roster because a
// person in N groups has ONE open_id channel but N roster rows.
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
                <div class="contacts-timeline">
                    <div class="contacts-section-title">
                        {sel === '*' ? t('contacts.allGroups', language) : groupName(sel)}
                    </div>
                    {loading && <div class="contacts-empty">{t('contacts.loading', language)}</div>}
                    {!loading && msgs.length === 0 && (
                        <div class="contacts-empty">{t('contacts.timelineEmpty', language)}</div>
                    )}
                    {!loading &&
                        msgs.map(m => (
                            <div key={m.MessageID} class="contacts-msg-line">
                                <span class="contacts-msg-time">{new Date(m.CreateTime).toLocaleString(language)}</span>
                                <span class="contacts-msg-sender">{m.SenderName || shortId(m.SenderID)}:</span>
                                <span class="contacts-msg-text">{msgText(m)}</span>
                            </div>
                        ))}
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
                                <span class="contacts-list-name">{shortId(s.SessionName)}</span>
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
            <h3 class="contacts-detail-name">{shortId(session.SessionName)}</h3>
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
