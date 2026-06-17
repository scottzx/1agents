import { h } from 'preact';
import { useSignal } from '@preact/signals';

import type { Milestone } from './types';

export interface MilestoneFields {
    name: string;
    description: string;
    targetDate: string | null;
    predecessorId: string;
}

interface MilestoneFormProps {
    milestones: Milestone[];
    initial?: Milestone; // present = edit mode
    onSubmit: (fields: MilestoneFields) => Promise<void>;
    onClose: () => void;
}

// dateInputToRFC turns a <input type="date"> value (YYYY-MM-DD) into RFC3339;
// '' clears the date (null).
function dateInputToRFC(v: string): string | null {
    return v ? new Date(`${v}T00:00:00Z`).toISOString() : null;
}

// descendantIds returns id + all milestones reachable downward via predecessor,
// so the predecessor dropdown can exclude them (no cycles / no self-parent).
function descendantIds(rootId: string, milestones: Milestone[]): Set<string> {
    const out = new Set<string>([rootId]);
    let grew = true;
    while (grew) {
        grew = false;
        for (const m of milestones) {
            if (m.predecessorId && out.has(m.predecessorId) && !out.has(m.id)) {
                out.add(m.id);
                grew = true;
            }
        }
    }
    return out;
}

export function MilestoneForm({ milestones, initial, onSubmit, onClose }: MilestoneFormProps) {
    const isEdit = !!initial;
    const name = useSignal(initial?.name ?? '');
    const desc = useSignal(initial?.description ?? '');
    const date = useSignal(initial?.targetDate ? initial.targetDate.slice(0, 10) : '');
    const predecessorId = useSignal(initial?.predecessorId ?? '');
    const error = useSignal('');
    const saving = useSignal(false);

    // Candidates for the 前置里程碑 dropdown: every other milestone, minus the
    // edited node's own subtree (which would create a cycle).
    const blocked = initial ? descendantIds(initial.id, milestones) : new Set<string>();
    const candidates = milestones.filter(m => !blocked.has(m.id));

    async function submit() {
        const trimmed = name.value.trim();
        if (!trimmed) {
            error.value = '名称不能为空';
            return;
        }
        saving.value = true;
        error.value = '';
        try {
            await onSubmit({
                name: trimmed,
                description: desc.value.trim(),
                targetDate: dateInputToRFC(date.value),
                predecessorId: predecessorId.value,
            });
            onClose();
        } catch (err) {
            error.value = (err as Error).message || '保存失败';
        } finally {
            saving.value = false;
        }
    }

    return (
        <div class="milestone-form">
            <div class="milestone-form-title">{isEdit ? '编辑里程碑' : '新建里程碑'}</div>
            <input
                class="milestone-form-input"
                placeholder="名称（如：M3 支付接入）"
                value={name.value}
                onInput={e => (name.value = (e.target as HTMLInputElement).value)}
            />
            <textarea
                class="milestone-form-textarea"
                placeholder="说明（可选）"
                value={desc.value}
                onInput={e => (desc.value = (e.target as HTMLTextAreaElement).value)}
            />
            <label class="milestone-form-field">
                前置里程碑
                <select
                    value={predecessorId.value}
                    onChange={e => (predecessorId.value = (e.target as HTMLSelectElement).value)}
                >
                    <option value="">（无 · 作为起点）</option>
                    {candidates.map(m => (
                        <option key={m.id} value={m.id}>
                            {m.name}
                        </option>
                    ))}
                </select>
            </label>
            <label class="milestone-form-field">
                目标日期
                <input
                    type="date"
                    value={date.value}
                    onInput={e => (date.value = (e.target as HTMLInputElement).value)}
                />
            </label>
            {error.value && <div class="milestone-form-error">{error.value}</div>}
            <div class="milestone-form-actions">
                <button class="milestone-form-save" disabled={saving.value} onClick={submit}>
                    {saving.value ? '保存中…' : '保存'}
                </button>
                <button class="milestone-form-cancel" onClick={onClose}>
                    取消
                </button>
            </div>
        </div>
    );
}
