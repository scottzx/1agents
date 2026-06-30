/**
 * CRM data store (#339-343) — signals + fetch helpers over the backend
 * /api/crm/* endpoints. Kept framework-light so it can later move into the
 * shared @1agents/core package (multi-end contract).
 */

import { signal } from '@preact/signals';

export type LeadStage = 'new' | 'contacted' | 'qualified' | 'won' | 'lost' | 'dropped';

export interface Contact {
    id: string;
    name: string;
    company?: string;
    title?: string;
    email?: string;
    phone?: string;
    source?: string;
    createdAt: string;
}

export interface TaskBrief {
    id: string;
    title: string;
    status: string;
    executor: string;
}

export interface Lead {
    id: string;
    contactId: string;
    stage: LeadStage;
    score: number;
    owner?: string;
    businessRef?: string;
    notes?: string;
    createdAt: string;
    updatedAt: string;
    tasks?: TaskBrief[];
}

export const contacts = signal<Contact[]>([]);
export const leads = signal<Lead[]>([]);
export const loading = signal(false);
export const errorMsg = signal<string | null>(null);

async function jsonFetch<T>(url: string, init?: RequestInit): Promise<T> {
    const res = await fetch(url, init);
    if (!res.ok) {
        const text = await res.text().catch(() => res.statusText);
        throw new Error(text || `HTTP ${res.status}`);
    }
    return res.json() as Promise<T>;
}

export async function loadContacts(): Promise<void> {
    const data = await jsonFetch<{ contacts: Contact[] }>('/api/crm/contacts');
    contacts.value = data.contacts ?? [];
}

export async function loadLeads(): Promise<void> {
    const data = await jsonFetch<{ leads: Lead[] }>('/api/crm/leads');
    leads.value = data.leads ?? [];
}

export async function refreshAll(): Promise<void> {
    loading.value = true;
    errorMsg.value = null;
    try {
        await Promise.all([loadContacts(), loadLeads()]);
    } catch (e) {
        errorMsg.value = e instanceof Error ? e.message : String(e);
    } finally {
        loading.value = false;
    }
}

export async function createContact(c: Partial<Contact>): Promise<void> {
    await jsonFetch<Contact>('/api/crm/contacts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(c),
    });
    await loadContacts();
}

export async function parseCard(text: string, save = true): Promise<Contact> {
    const c = await jsonFetch<Contact>('/api/crm/contacts/parse-card', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ text, save }),
    });
    if (save) await loadContacts();
    return c;
}

export async function ingestInbox(): Promise<number> {
    const r = await jsonFetch<{ ingested: number }>('/api/crm/ingest', { method: 'POST' });
    await loadContacts();
    return r.ingested;
}

export async function createLead(contactId: string, owner?: string, notes?: string): Promise<void> {
    await jsonFetch<Lead>('/api/crm/leads', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ contactId, owner, notes }),
    });
    await loadLeads();
}

/** Dispatch a lead action (score | enrich | follow | drop) and refresh leads. */
export async function leadAction(
    leadId: string,
    action: 'score' | 'enrich' | 'follow' | 'drop',
    workspacePath: string,
    context?: string
): Promise<void> {
    await jsonFetch<{ taskId: string }>(`/api/crm/leads/${encodeURIComponent(leadId)}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspacePath, context }),
    });
    await loadLeads();
}

export const STAGE_LABELS: Record<LeadStage, string> = {
    new: '新建',
    contacted: '已联系',
    qualified: '已验证',
    won: '赢单',
    lost: '丢单',
    dropped: '已放弃',
};

/** The ordered funnel columns shown in the L1 page. */
export const FUNNEL_STAGES: LeadStage[] = ['new', 'contacted', 'qualified', 'won', 'lost'];

export function contactById(id: string): Contact | undefined {
    return contacts.value.find(c => c.id === id);
}
