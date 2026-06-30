import { h } from 'preact';
import type { VNode } from 'preact';

import { t, type Lang } from '../../../i18n';
import type { Contact } from '@1agents/core/services/contactService';
import type { CellHelpers, GridColumn } from '../TaskList/DataGrid';

// 联系人多维表格 (Contacts DataGrid) config — mirrors sessionGrid.tsx. The grid is
// generic; this file supplies the contacts-specific columns, comparators, group
// values and cell renderer. Org names resolve through companyMap (tenantKey →
// short company name) — there is no hardcoded tenant here.

function shortId(id: string): string {
    return id.length > 10 ? `…${id.slice(-8)}` : id;
}

// firstFeishuTenant returns the tenant_key of a contact's first Feishu channel
// that carries one — the org used to label / distinguish the contact.
export function firstFeishuTenant(c: Contact): string {
    const ch = (c.channels || []).find(x => x.platform === 'feishu' && x.tenantKey);
    return ch?.tenantKey || '';
}

// contactDisplayName is the name to show: the contact's nickname, or — when it's
// empty (message-only senders beyond the roster cap have no nickname) — the
// Feishu open_id, then phone. Never blank so the row is always identifiable.
export function contactDisplayName(c: Contact): string {
    if (c.name) return c.name;
    const ch = (c.channels || []).find(x => x.platform === 'feishu' && x.channelId);
    return ch?.channelId || c.phone || '';
}

export function feishuCount(c: Contact): number {
    return (c.channels || []).filter(ch => ch.platform === 'feishu').length;
}

// orgName resolves a contact's org via the companyMap (tenantKey → short name).
// Falls back to a short tenant id when the tenant isn't mapped to any company,
// or '—' when the contact has no Feishu tenant at all.
export function orgName(c: Contact, companyMap: Record<string, string>): string {
    const tk = firstFeishuTenant(c);
    if (!tk) return '—';
    return companyMap[tk] || shortId(tk);
}

export function getContactColumns(lang: Lang): GridColumn[] {
    return [
        { key: 'name', label: t('contacts.col.name', lang), width: 200, locked: true, groupable: true },
        { key: 'degree', label: t('contacts.col.degree', lang), width: 96, groupable: true },
        { key: 'phone', label: t('contacts.col.phone', lang), width: 140 },
        { key: 'company', label: t('contacts.col.company', lang), width: 160, groupable: true },
        { key: 'feishuCount', label: t('contacts.col.feishuCount', lang), width: 90 },
        { key: 'groupCount', label: t('contacts.col.groupCount', lang), width: 90 },
        { key: 'createdAt', label: t('contacts.col.createdAt', lang), width: 124 },
    ];
}

export function getContactGroupOptions(lang: Lang): Array<[string, string]> {
    return [
        ['none', t('contacts.group.none', lang)],
        ['degree', t('contacts.col.degree', lang)],
        ['company', t('contacts.col.company', lang)],
    ];
}

const ts = (iso?: string): number => (iso ? new Date(iso).getTime() : 0);

export function compareContacts(a: Contact, b: Contact, key: string, companyMap: Record<string, string>): number {
    switch (key) {
        case 'name':
            return contactDisplayName(a).localeCompare(contactDisplayName(b));
        case 'degree':
            return a.degree - b.degree;
        case 'phone':
            return a.phone.localeCompare(b.phone);
        case 'company':
            return orgName(a, companyMap).localeCompare(orgName(b, companyMap));
        case 'feishuCount':
            return feishuCount(a) - feishuCount(b);
        case 'groupCount':
            return (a.groupCount || 0) - (b.groupCount || 0);
        case 'createdAt':
            return ts(a.createdAt) - ts(b.createdAt);
        default:
            return 0;
    }
}

// Default order: by name (then phone) — mirrors the prior list's ORDER BY name.
export function contactDefaultCompare(a: Contact, b: Contact): number {
    return contactDisplayName(a).localeCompare(contactDisplayName(b));
}

export function contactGroupValue(c: Contact, key: string, lang: Lang, companyMap: Record<string, string>): string {
    switch (key) {
        case 'degree':
            return c.degree === 2 ? t('contacts.degree.second', lang) : t('contacts.degree.first', lang);
        case 'company':
            return orgName(c, companyMap);
        default:
            return '';
    }
}

function fmtDate(iso?: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString();
}

// Read-only cell renderer for the contacts DataGrid. Each branch returns a full
// <td>, mirroring renderSessionCell's contract. The name cell is a button that
// opens the detail modal.
export function renderContactCell(
    c: Contact,
    col: GridColumn,
    helpers: CellHelpers,
    lang: Lang,
    companyMap: Record<string, string>
): VNode {
    switch (col.key) {
        case 'name':
            return (
                <td class="col-contact-name">
                    <button
                        class="contact-name-link"
                        onClick={(e: Event) => {
                            e.stopPropagation();
                            helpers.openDetail();
                        }}
                    >
                        {contactDisplayName(c) || t('contacts.field.name', lang)}
                    </button>
                </td>
            );
        case 'degree':
            return (
                <td class="col-contact-degree">
                    <span class={`contacts-degree-badge deg-${c.degree === 2 ? 2 : 1}`}>
                        {c.degree === 2 ? t('contacts.degree.second', lang) : t('contacts.degree.first', lang)}
                    </span>
                </td>
            );
        case 'phone':
            return <td class="col-contact-phone">{c.phone || '—'}</td>;
        case 'company': {
            const tk = firstFeishuTenant(c);
            return (
                <td class="col-contact-company" title={tk || undefined}>
                    {orgName(c, companyMap)}
                </td>
            );
        }
        case 'feishuCount':
            return <td class="col-contact-count">{feishuCount(c)}</td>;
        case 'groupCount':
            return <td class="col-contact-count">{c.groupCount || 0}</td>;
        case 'createdAt':
            return <td class="col-contact-date">{fmtDate(c.createdAt)}</td>;
        default:
            return <td />;
    }
}
