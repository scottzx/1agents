import { h } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import type { RefObject } from 'preact';
import {
    BriefVersionConflictError,
    roundtableService,
    type BriefStatus,
    type ProductKind,
    type RoundtableBrief,
    type RoundtableBriefVersion,
    type RoundtableRoom,
} from '@1agents/core/services/roundtableService';

export type BriefInspectorMode = 'loading' | 'empty' | 'ready' | 'saving' | 'conflict' | 'error' | 'confirmed';

interface BriefInspectorProps {
    room: RoundtableRoom;
    loading?: boolean;
    readOnly?: boolean;
    sectionRef?: RefObject<HTMLElement>;
    onRoomUpdate?: (room: RoundtableRoom) => void | Promise<void>;
    onReload?: () => void | Promise<void>;
}

interface ResolveBriefInspectorModeInput {
    loading?: boolean;
    saving?: boolean;
    conflict?: boolean;
    error?: boolean;
    confirmed?: boolean;
    hasVersion?: boolean;
}

const EMPTY_BRIEF: RoundtableBrief = {
    title: '',
    question: '',
    constraints: '',
    success_criteria: '',
};

export function resolveBriefInspectorMode(input: ResolveBriefInspectorModeInput): BriefInspectorMode {
    if (input.loading && !input.hasVersion) return 'loading';
    if (input.saving) return 'saving';
    if (input.conflict) return 'conflict';
    if (input.error) return 'error';
    if (input.confirmed) return 'confirmed';
    return input.hasVersion ? 'ready' : 'empty';
}

export function isBriefContentComplete(brief: RoundtableBrief): boolean {
    const fields = [brief.title, brief.question, brief.constraints, brief.success_criteria];
    return fields.every(isBriefValueComplete);
}

function isBriefValueComplete(value: string): boolean {
    const trimmed = value.trim();
    return Boolean(trimmed) && !['—', '-', '–', 'TBD', 'tbd', 'N/A', 'n/a'].includes(trimmed);
}

function currentBrief(room: RoundtableRoom): RoundtableBriefVersion | null {
    if (room.current_brief) return room.current_brief;
    if (!room.brief) return null;

    const version = room.current_brief_version || room.confirmed_brief_version || 1;
    const confirmed = room.confirmed_brief_version === version || room.state !== 'drafting_brief';
    return {
        room_id: room.id,
        version,
        status: confirmed ? 'confirmed' : 'draft',
        content: room.brief,
        proposed_by: 'user',
        created_at: room.created_at,
        updated_at: room.updated_at,
        ...(confirmed ? { confirmed_at: room.updated_at } : {}),
    };
}

function statusLabel(status: BriefStatus): string {
    switch (status) {
        case 'draft':
            return '草稿';
        case 'proposed':
            return '待确认';
        case 'confirmed':
            return '已确认';
        case 'superseded':
            return '已被替代';
    }
}

function sameBrief(a: RoundtableBrief, b: RoundtableBrief): boolean {
    return (
        a.title === b.title &&
        a.question === b.question &&
        a.constraints === b.constraints &&
        a.success_criteria === b.success_criteria &&
        (a.product_kind || '') === (b.product_kind || '')
    );
}

/** The single full Brief body in a room, bound to room.current_brief. */
export function BriefInspector({
    room,
    loading = false,
    readOnly = false,
    sectionRef,
    onRoomUpdate,
    onReload,
}: BriefInspectorProps) {
    const version = currentBrief(room);
    const [draft, setDraft] = useState<RoundtableBrief>(() => ({ ...(version?.content || EMPTY_BRIEF) }));
    const [savingAction, setSavingAction] = useState<'save' | 'confirm' | 'reload' | null>(null);
    const [conflict, setConflict] = useState<BriefVersionConflictError | null>(null);
    const [error, setError] = useState<string | null>(null);
    const titleRef = useRef<HTMLInputElement>(null);
    const questionRef = useRef<HTMLTextAreaElement>(null);
    const constraintsRef = useRef<HTMLTextAreaElement>(null);
    const successRef = useRef<HTMLTextAreaElement>(null);
    const conflictActionRef = useRef<HTMLButtonElement>(null);

    useEffect(() => {
        setDraft({ ...(version?.content || EMPTY_BRIEF) });
        setConflict(null);
        setError(null);
    }, [room.id, version?.version, version?.status]);

    useEffect(() => {
        if (conflict) conflictActionRef.current?.focus();
    }, [conflict]);

    const confirmed =
        Boolean(version) &&
        (version?.status === 'confirmed' ||
            room.confirmed_brief_version === version?.version ||
            room.state !== 'drafting_brief');
    const dirty = version ? !sameBrief(draft, version.content) : Object.values(draft).some(Boolean);
    const complete = isBriefContentComplete(draft);
    const mode = resolveBriefInspectorMode({
        loading,
        saving: Boolean(savingAction),
        conflict: Boolean(conflict),
        error: Boolean(error),
        confirmed,
        hasVersion: Boolean(version),
    });
    const status = version?.status || (confirmed ? 'confirmed' : 'draft');
    const expectedVersion = room.current_brief_version || version?.version || 0;
    const describedBy = `rt-brief-state-${room.id}`;

    const update = (field: keyof RoundtableBrief, value: string) => {
        setDraft(current => ({ ...current, [field]: value }));
        setError(null);
    };

    const focusFirstBriefField = () => {
        const fields = [
            { value: draft.title, ref: titleRef },
            { value: draft.question, ref: questionRef },
            { value: draft.constraints, ref: constraintsRef },
            { value: draft.success_criteria, ref: successRef },
        ];
        const target = fields.find(field => !isBriefValueComplete(field.value)) || fields[0];
        queueMicrotask(() => target.ref.current?.focus());
    };

    const focusInspector = () => {
        queueMicrotask(() =>
            (document.getElementById(`rt-brief-title-${room.id}`)?.closest('section') as HTMLElement | null)?.focus()
        );
    };

    const save = async () => {
        if (readOnly || !complete || !dirty || confirmed) return;
        setSavingAction('save');
        setConflict(null);
        setError(null);
        try {
            const next = await roundtableService.saveBriefDraft(room.id, {
                ...draft,
                expected_version: expectedVersion,
            });
            await onRoomUpdate?.(next);
        } catch (cause) {
            if (cause instanceof BriefVersionConflictError) {
                setConflict(cause);
            } else {
                setError(cause instanceof Error ? cause.message : String(cause));
                focusFirstBriefField();
            }
        } finally {
            setSavingAction(null);
        }
    };

    const confirm = async () => {
        if (readOnly || !version || !complete || dirty || confirmed) return;
        setSavingAction('confirm');
        setConflict(null);
        setError(null);
        try {
            const next = await roundtableService.confirmBrief(room.id, {
                version: version.version,
                expected_version: expectedVersion,
            });
            await onRoomUpdate?.(next);
            focusInspector();
        } catch (cause) {
            if (cause instanceof BriefVersionConflictError) {
                setConflict(cause);
            } else {
                setError(cause instanceof Error ? cause.message : String(cause));
                focusFirstBriefField();
            }
        } finally {
            setSavingAction(null);
        }
    };

    const reload = async () => {
        setSavingAction('reload');
        setError(null);
        try {
            if (onReload) {
                await onReload();
            } else {
                const next = await roundtableService.getRoom(room.id);
                await onRoomUpdate?.(next);
            }
            setConflict(null);
            focusFirstBriefField();
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : String(cause));
            focusFirstBriefField();
        } finally {
            setSavingAction(null);
        }
    };

    const stateText = useMemo(() => {
        switch (mode) {
            case 'loading':
                return '正在加载服务端 Brief…';
            case 'empty':
                return '尚无 Brief 版本。填写四个必填字段并保存，将创建 v1。';
            case 'saving':
                if (savingAction === 'confirm') return '正在确认当前版本…';
                if (savingAction === 'reload') return '正在加载最新版本…';
                return '正在保存新版本…';
            case 'conflict':
                return `保存基于 v${conflict?.expectedVersion ?? expectedVersion}，但服务端已是 v${
                    conflict?.currentVersion ?? '?'
                }。请加载最新版本后重新编辑。`;
            case 'error':
                return error || 'Brief 操作失败。';
            case 'confirmed':
                return `Brief v${version?.version || expectedVersion} 已确认，已锁定为后续讨论快照。`;
            case 'ready':
                return dirty ? '有未保存修改。保存会创建一个新版本。' : '当前内容已与服务端同步。';
        }
    }, [mode, savingAction, conflict, expectedVersion, error, version?.version, dirty]);

    if (mode === 'loading') {
        return (
            <section
                class="rt-side-section rt-brief-inspector is-loading"
                aria-labelledby={`rt-brief-title-${room.id}`}
                ref={sectionRef}
                tabIndex={-1}
            >
                <h3 id={`rt-brief-title-${room.id}`} class="rt-side-title">
                    Brief Inspector
                </h3>
                <div class="rt-brief-loading" role="status">
                    {stateText}
                </div>
            </section>
        );
    }

    return (
        <section
            class={`rt-side-section rt-brief-inspector is-${mode}`}
            aria-labelledby={`rt-brief-title-${room.id}`}
            ref={sectionRef}
            tabIndex={-1}
        >
            <div class="rt-brief-inspector-head">
                <h3 id={`rt-brief-title-${room.id}`} class="rt-side-title">
                    Brief Inspector
                </h3>
                <span class={`rt-brief-status is-${status}`}>
                    {version ? `v${version.version} · ${statusLabel(status)}` : '未创建'}
                </span>
            </div>

            <p
                id={describedBy}
                class={`rt-brief-state is-${mode}`}
                role={mode === 'error' || mode === 'conflict' ? 'alert' : 'status'}
            >
                {stateText}
            </p>

            {mode === 'conflict' && (
                <button
                    type="button"
                    class="rt-btn rt-btn-primary rt-brief-conflict-action"
                    ref={conflictActionRef}
                    disabled={Boolean(savingAction)}
                    onClick={() => void reload()}
                >
                    加载最新版本
                </button>
            )}

            <div class="rt-brief" aria-describedby={describedBy}>
                <label class="rt-field">
                    <span class="rt-field-label">标题</span>
                    <input
                        ref={titleRef}
                        class="rt-input"
                        value={draft.title}
                        disabled={readOnly || confirmed || Boolean(savingAction)}
                        onInput={event => update('title', (event.target as HTMLInputElement).value)}
                    />
                </label>
                <label class="rt-field">
                    <span class="rt-field-label">议题 / 问题</span>
                    <textarea
                        ref={questionRef}
                        class="rt-input rt-textarea"
                        rows={3}
                        value={draft.question}
                        disabled={readOnly || confirmed || Boolean(savingAction)}
                        onInput={event => update('question', (event.target as HTMLTextAreaElement).value)}
                    />
                </label>
                <label class="rt-field">
                    <span class="rt-field-label">约束</span>
                    <textarea
                        ref={constraintsRef}
                        class="rt-input rt-textarea"
                        rows={3}
                        value={draft.constraints}
                        disabled={readOnly || confirmed || Boolean(savingAction)}
                        onInput={event => update('constraints', (event.target as HTMLTextAreaElement).value)}
                    />
                </label>
                <label class="rt-field">
                    <span class="rt-field-label">成功标准</span>
                    <textarea
                        ref={successRef}
                        class="rt-input rt-textarea"
                        rows={3}
                        value={draft.success_criteria}
                        disabled={readOnly || confirmed || Boolean(savingAction)}
                        onInput={event => update('success_criteria', (event.target as HTMLTextAreaElement).value)}
                    />
                </label>
                <label class="rt-field">
                    <span class="rt-field-label">品类（可选）</span>
                    <select
                        class="rt-input"
                        value={draft.product_kind || ''}
                        disabled={readOnly || confirmed || Boolean(savingAction)}
                        onChange={event =>
                            update('product_kind', (event.target as HTMLSelectElement).value as ProductKind)
                        }
                    >
                        <option value="">未指定</option>
                        <option value="software">软件</option>
                        <option value="hardware">硬件</option>
                        <option value="hybrid">软硬一体</option>
                    </select>
                </label>
            </div>

            {!readOnly && !confirmed && (
                <div class="rt-brief-actions">
                    <button
                        type="button"
                        class="rt-btn"
                        disabled={!complete || !dirty || Boolean(savingAction)}
                        onClick={() => void save()}
                    >
                        {savingAction === 'save' ? '保存中…' : '保存新版本'}
                    </button>
                    <button
                        type="button"
                        class="rt-btn rt-btn-primary"
                        disabled={!version || !complete || dirty || Boolean(savingAction)}
                        title={dirty ? '请先保存当前修改' : undefined}
                        onClick={() => void confirm()}
                    >
                        {savingAction === 'confirm' ? '确认中…' : '确认并进入 R2'}
                    </button>
                </div>
            )}
        </section>
    );
}
