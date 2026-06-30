/**
 * CrmPage (#343) — the CRM L1 global page.
 *
 * Three zones:
 *   1. Lead funnel — stage columns (new → contacted → qualified → won/lost), each
 *      lead card shows score, inline task state, and 跟进/放弃 decision buttons.
 *   2. Contact library — table of sunk contacts; 立项为线索 promotes a contact to a lead.
 *   3. Intake bar — 名片解析 + 从 Inbox 汇入 (#340 source aggregation reuse).
 *
 * The 跟进/放弃 decision creates a `human` executor task via the North API; on
 * completion the backend hook advances the lead's stage (#343). Inline execution
 * state comes from ListTasksForBusiness (tasks[] on each lead).
 */

import { h, Fragment } from 'preact';
import { useEffect, useState } from 'preact/hooks';
import { setCopilotAppContext, clearCopilotAppContext } from '../../stores/appManifestStore';
import { workspaces, activeWorkspaceId } from '../../stores/workspaceStore';
import {
    contacts,
    leads,
    loading,
    errorMsg,
    refreshAll,
    createLead,
    parseCard,
    ingestInbox,
    leadAction,
    contactById,
    STAGE_LABELS,
    FUNNEL_STAGES,
    type Lead,
    type LeadStage,
} from './store';

function activeWorkspacePath(): string {
    const ws = workspaces.value.find(w => w.id === activeWorkspaceId.value);
    return ws?.path ?? '';
}

export function CrmPage() {
    const [cardText, setCardText] = useState('');
    const [busy, setBusy] = useState<string | null>(null);

    useEffect(() => {
        void refreshAll();
        setCopilotAppContext({ appId: 'crm', namespace: 'crm', connectors: [] });
        return () => clearCopilotAppContext();
    }, []);

    const ws = activeWorkspacePath();
    const noWorkspace = ws === '';

    async function run(label: string, fn: () => Promise<unknown>) {
        setBusy(label);
        try {
            await fn();
        } catch (e) {
            errorMsg.value = e instanceof Error ? e.message : String(e);
        } finally {
            setBusy(null);
        }
    }

    return (
        <div class="crm-page">
            <header class="crm-header">
                <h1 class="crm-title">CRM</h1>
                <p class="crm-subtitle">联系人沉淀 · 潜客挖掘 · 漏斗决策</p>
            </header>

            {errorMsg.value && (
                <div class="crm-banner crm-banner-error" role="alert">
                    {errorMsg.value}
                    <button class="crm-banner-close" onClick={() => (errorMsg.value = null)}>
                        ×
                    </button>
                </div>
            )}

            {/* ── Intake bar (#340) ── */}
            <section class="crm-intake">
                <textarea
                    class="crm-card-input"
                    placeholder="粘贴名片文本(姓名 / 公司 / 职位 / 邮箱 / 电话)…"
                    value={cardText}
                    onInput={e => setCardText((e.target as HTMLTextAreaElement).value)}
                />
                <div class="crm-intake-actions">
                    <button
                        class="crm-btn crm-btn-accent"
                        disabled={busy !== null || cardText.trim() === ''}
                        onClick={() =>
                            run('card', async () => {
                                await parseCard(cardText);
                                setCardText('');
                            })
                        }
                    >
                        {busy === 'card' ? '解析中…' : '解析名片'}
                    </button>
                    <button
                        class="crm-btn"
                        disabled={busy !== null}
                        onClick={() =>
                            run('ingest', async () => {
                                const n = await ingestInbox();
                                errorMsg.value = `从 Inbox 汇入 ${n} 个联系人`;
                            })
                        }
                    >
                        {busy === 'ingest' ? '汇入中…' : '从 Inbox 汇入'}
                    </button>
                </div>
            </section>

            {loading.value ? (
                <div class="crm-empty">加载中…</div>
            ) : (
                <Fragment>
                    {/* ── Lead funnel (#343) ── */}
                    <section class="crm-section">
                        <h2 class="crm-section-title">线索漏斗</h2>
                        {noWorkspace && (
                            <p class="crm-hint">未选中项目工作区 — 跟进/打分/富集任务需要一个工作区作为执行目录。</p>
                        )}
                        <div class="crm-funnel">
                            {FUNNEL_STAGES.map(stage => (
                                <FunnelColumn
                                    key={stage}
                                    stage={stage}
                                    leads={leads.value.filter(l => l.stage === stage)}
                                    workspacePath={ws}
                                    disabled={noWorkspace || busy !== null}
                                    onAction={(id, action) => run(action, () => leadAction(id, action, ws))}
                                />
                            ))}
                        </div>
                    </section>

                    {/* ── Contact library (#343) ── */}
                    <section class="crm-section">
                        <h2 class="crm-section-title">联系人库</h2>
                        <ContactLibrary onPromote={cid => run('lead', () => createLead(cid))} busy={busy !== null} />
                    </section>
                </Fragment>
            )}
        </div>
    );
}

// ── Funnel column ─────────────────────────────────────────────────────────────

interface FunnelColumnProps {
    stage: LeadStage;
    leads: Lead[];
    workspacePath: string;
    disabled: boolean;
    onAction: (leadId: string, action: 'score' | 'enrich' | 'follow' | 'drop') => void;
}

function FunnelColumn({ stage, leads: items, disabled, onAction }: FunnelColumnProps) {
    return (
        <div class={`crm-funnel-col crm-stage-${stage}`}>
            <div class="crm-funnel-col-head">
                <span class="crm-stage-label">{STAGE_LABELS[stage]}</span>
                <span class="crm-stage-count">{items.length}</span>
            </div>
            <div class="crm-funnel-col-body">
                {items.length === 0 && <div class="crm-funnel-empty">—</div>}
                {items.map(lead => (
                    <LeadCard key={lead.id} lead={lead} disabled={disabled} onAction={onAction} />
                ))}
            </div>
        </div>
    );
}

// ── Lead card ─────────────────────────────────────────────────────────────────

interface LeadCardProps {
    lead: Lead;
    disabled: boolean;
    onAction: (leadId: string, action: 'score' | 'enrich' | 'follow' | 'drop') => void;
}

function LeadCard({ lead, disabled, onAction }: LeadCardProps) {
    const contact = contactById(lead.contactId);
    const running = (lead.tasks ?? []).filter(
        t => t.status !== 'completed' && t.status !== 'failed' && t.status !== 'cancelled'
    );
    return (
        <article class="crm-lead-card">
            <div class="crm-lead-head">
                <span class="crm-lead-name">{contact?.name ?? lead.contactId}</span>
                <span class={`crm-score crm-score-${scoreBucket(lead.score)}`} title="评分">
                    {lead.score}
                </span>
            </div>
            {contact?.company && <div class="crm-lead-company">{contact.company}</div>}
            {lead.notes && <div class="crm-lead-notes">{lead.notes}</div>}

            {(lead.tasks ?? []).length > 0 && (
                <ul class="crm-task-list">
                    {(lead.tasks ?? []).map(t => (
                        <li key={t.id} class={`crm-task crm-task-${t.status}`}>
                            <span class="crm-task-exec">{t.executor}</span>
                            <span class="crm-task-title">{t.title}</span>
                            <span class="crm-task-status">{t.status}</span>
                        </li>
                    ))}
                </ul>
            )}

            <div class="crm-lead-actions">
                <button class="crm-btn crm-btn-sm" disabled={disabled} onClick={() => onAction(lead.id, 'score')}>
                    打分
                </button>
                <button class="crm-btn crm-btn-sm" disabled={disabled} onClick={() => onAction(lead.id, 'enrich')}>
                    富集
                </button>
                <button
                    class="crm-btn crm-btn-sm crm-btn-success"
                    disabled={disabled}
                    onClick={() => onAction(lead.id, 'follow')}
                >
                    跟进
                </button>
                <button
                    class="crm-btn crm-btn-sm crm-btn-danger"
                    disabled={disabled}
                    onClick={() => onAction(lead.id, 'drop')}
                >
                    放弃
                </button>
            </div>
            {running.length > 0 && <div class="crm-lead-running">执行中 {running.length}…</div>}
        </article>
    );
}

function scoreBucket(score: number): 'low' | 'mid' | 'high' {
    if (score >= 70) return 'high';
    if (score >= 40) return 'mid';
    return 'low';
}

// ── Contact library ───────────────────────────────────────────────────────────

function ContactLibrary({ onPromote, busy }: { onPromote: (contactId: string) => void; busy: boolean }) {
    if (contacts.value.length === 0) {
        return <div class="crm-empty">暂无联系人 — 用上方名片解析或从 Inbox 汇入。</div>;
    }
    const leadContactIds = new Set(leads.value.map(l => l.contactId));
    return (
        <div class="bento-grid crm-contact-grid">
            {contacts.value.map(c => (
                <div class="bento-card crm-contact-card" key={c.id}>
                    <div class="bento-zone-header">
                        <div class="bento-card-title">{c.name}</div>
                        {c.source && <span class="crm-source-badge">{c.source}</span>}
                    </div>
                    <div class="bento-zone-body">
                        {c.company && (
                            <div class="crm-contact-line">
                                {c.title ? `${c.title} · ` : ''}
                                {c.company}
                            </div>
                        )}
                        {c.email && <div class="crm-contact-line crm-mono">{c.email}</div>}
                        {c.phone && <div class="crm-contact-line crm-mono">{c.phone}</div>}
                    </div>
                    <div class="bento-zone-footer">
                        {leadContactIds.has(c.id) ? (
                            <span class="crm-tag">已转线索</span>
                        ) : (
                            <button class="crm-btn crm-btn-sm" disabled={busy} onClick={() => onPromote(c.id)}>
                                立项为线索
                            </button>
                        )}
                    </div>
                </div>
            ))}
        </div>
    );
}
