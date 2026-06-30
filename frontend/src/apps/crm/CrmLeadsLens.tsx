/**
 * CrmLeadsLens — the 关联线索 lens (manifest mount type=lens, scope=project).
 *
 * A横切 overlay that shows related leads with their score and inline task state.
 * It is read-only (the funnel page owns the decisions); the lens just surfaces
 * CRM context wherever it is叠加 over a project.
 */

import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { leads, contactById, refreshAll, STAGE_LABELS } from './store';

export function CrmLeadsLens() {
    useEffect(() => {
        void refreshAll();
    }, []);

    if (leads.value.length === 0) {
        return <div class="crm-lens crm-lens-empty">暂无关联线索</div>;
    }

    return (
        <div class="crm-lens">
            <div class="crm-lens-head">关联线索 · {leads.value.length}</div>
            <ul class="crm-lens-list">
                {leads.value.map(lead => {
                    const c = contactById(lead.contactId);
                    const running = (lead.tasks ?? []).filter(
                        t => t.status !== 'completed' && t.status !== 'failed' && t.status !== 'cancelled'
                    ).length;
                    return (
                        <li class="crm-lens-item" key={lead.id}>
                            <span class="crm-lens-name">{c?.name ?? lead.contactId}</span>
                            <span class="crm-lens-stage">{STAGE_LABELS[lead.stage]}</span>
                            <span class="crm-lens-score">{lead.score}</span>
                            {running > 0 && <span class="crm-lens-running">●{running}</span>}
                        </li>
                    );
                })}
            </ul>
        </div>
    );
}
