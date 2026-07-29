import { h } from 'preact';
import { useSignal } from '@preact/signals';

import type { Milestone } from './types';

export type MilestoneBump = 'patch' | 'minor' | 'major';

export interface MilestoneFields {
    description: string;
    targetDate: string | null;
    bump?: MilestoneBump;
    name?: string;
    predecessorId?: string;
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
    const bump = useSignal<MilestoneBump>('minor');
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
        if (initial?.isLegacy && !trimmed) {
            error.value = '名称不能为空';
            return;
        }
        saving.value = true;
        error.value = '';
        try {
            const fields: MilestoneFields = {
                description: desc.value.trim(),
                targetDate: dateInputToRFC(date.value),
            };
            if (!initial) fields.bump = bump.value;
            if (initial?.isLegacy) {
                fields.name = trimmed;
                fields.predecessorId = predecessorId.value;
            }
            await onSubmit(fields);
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
            {!initial && (
                <fieldset class="milestone-bump-field">
                    <legend>版本变更</legend>
                    <div class="milestone-bump-options">
                        {(
                            [
                                ['patch', 'Patch', '修复与小范围补充'],
                                ['minor', 'Minor', '向后兼容的新功能'],
                                ['major', 'Major', '重大或不兼容变更'],
                            ] as Array<[MilestoneBump, string, string]>
                        ).map(([value, label, hint]) => (
                            <label key={value} class={bump.value === value ? 'selected' : ''}>
                                <input
                                    type="radio"
                                    name="milestone-bump"
                                    value={value}
                                    checked={bump.value === value}
                                    onChange={() => (bump.value = value)}
                                />
                                <span>
                                    <strong>{label}</strong>
                                    <small>{hint}</small>
                                </span>
                            </label>
                        ))}
                    </div>
                    <p>版本号和前序版本由服务端自动生成。</p>
                </fieldset>
            )}
            {initial &&
                (initial.isLegacy ? (
                    <input
                        class="milestone-form-input"
                        aria-label="历史里程碑名称"
                        value={name.value}
                        onInput={e => (name.value = (e.target as HTMLInputElement).value)}
                    />
                ) : (
                    <div class="milestone-version-readonly">
                        <span>版本</span>
                        <strong>{initial.version || initial.name}</strong>
                        <small>版本号和前序版本由系统维护，不可修改</small>
                    </div>
                ))}
            <textarea
                class="milestone-form-textarea"
                placeholder="说明（可选）"
                value={desc.value}
                onInput={e => (desc.value = (e.target as HTMLTextAreaElement).value)}
            />
            {initial?.isLegacy && (
                <label class="milestone-form-field">
                    前置里程碑
                    <select
                        value={predecessorId.value}
                        onChange={e => (predecessorId.value = (e.target as HTMLSelectElement).value)}
                    >
                        <option value="">（无 · 作为起点）</option>
                        {candidates.map(m => (
                            <option key={m.id} value={m.id}>
                                {m.version || m.name}
                            </option>
                        ))}
                    </select>
                </label>
            )}
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
