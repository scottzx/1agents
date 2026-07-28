import { h } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { roundtableService, type RoomState, type RoundtableRoom } from '@1agents/core/services/roundtableService';
import { STAGES, stageIndexFromState, stateLabel } from './stage';

export interface RoomListProps {
    onOpenRoom: (roomId: string) => void;
    onCreate: () => void;
    /** Bump to force reload (e.g. after creating a room then backing out). */
    refreshKey?: number;
}

/**
 * Responsive topic-card grid.
 * Cards open rooms normally and toggle selection while selection mode is active.
 */
export function RoomList({ onOpenRoom, onCreate, refreshKey = 0 }: RoomListProps) {
    const [rooms, setRooms] = useState<RoundtableRoom[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [selectionMode, setSelectionMode] = useState(false);
    const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
    const [deleting, setDeleting] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const list = await roundtableService.listRooms(100);
            setRooms(list);
            setError(null);
            setSelectedIds(new Set());
            setSelectionMode(false);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setLoading(false);
        }
    }, []);

    const deleteSelected = useCallback(
        async (ids: string[]) => {
            setDeleting(true);
            try {
                await Promise.all(ids.map(id => roundtableService.deleteRoom(id)));
                await load();
            } catch (e) {
                const message = e instanceof Error ? e.message : String(e);
                await load();
                setError(message);
            } finally {
                setDeleting(false);
            }
        },
        [load]
    );

    useEffect(() => {
        void load();
    }, [load, refreshKey]);

    const allSelected = rooms.length > 0 && selectedIds.size === rooms.length;
    const exitSelectionMode = () => {
        setSelectionMode(false);
        setSelectedIds(new Set());
    };

    return (
        <div class="rt-room rt-room-list">
            <header class="rt-list-header">
                <div class="rt-list-title-block">
                    <span class="rt-list-kicker">我的议题</span>
                    <h1 class="rt-list-title">圆桌讨论</h1>
                    <p class="rt-list-desc">从你的问题出发，优先继续标有“等待我操作”的讨论。</p>
                </div>
                <div class="rt-list-actions">
                    {selectionMode ? (
                        <button
                            type="button"
                            class="rt-btn rt-btn-primary"
                            onClick={exitSelectionMode}
                            disabled={deleting}
                        >
                            完成
                        </button>
                    ) : (
                        <div class="rt-list-default-actions">
                            <button
                                type="button"
                                class="rt-btn rt-btn-ghost"
                                disabled={loading}
                                onClick={() => void load()}
                                aria-label="刷新圆桌清单"
                            >
                                刷新
                            </button>
                            {rooms.length > 0 ? (
                                <button
                                    type="button"
                                    class="rt-btn rt-btn-ghost"
                                    onClick={() => setSelectionMode(true)}
                                >
                                    选择
                                </button>
                            ) : null}
                            <button type="button" class="rt-btn rt-btn-primary" onClick={onCreate}>
                                新建圆桌
                            </button>
                        </div>
                    )}
                </div>
            </header>

            {error && (
                <div class="rt-error rt-list-error" role="alert">
                    {error}
                </div>
            )}

            {selectionMode ? (
                <section class="rt-selection-toolbar" aria-label="圆桌批量选择">
                    <div class="rt-selection-summary">
                        <strong>选择圆桌</strong>
                        <span role="status" aria-live="polite">
                            已选择 {selectedIds.size} / {rooms.length} 项
                        </span>
                    </div>
                    <div class="rt-selection-actions">
                        <button
                            type="button"
                            class="rt-btn rt-btn-ghost"
                            disabled={deleting}
                            onClick={() =>
                                setSelectedIds(allSelected ? new Set() : new Set(rooms.map(room => room.id)))
                            }
                        >
                            {allSelected ? '取消全选' : '全选'}
                        </button>
                        <button
                            type="button"
                            class="rt-btn rt-btn-danger"
                            disabled={selectedIds.size === 0 || deleting}
                            onClick={() => {
                                const ids = Array.from(selectedIds);
                                if (window.confirm(`确定删除 ${ids.length} 个圆桌及其所有内容吗？此操作不可恢复。`)) {
                                    void deleteSelected(ids);
                                }
                            }}
                        >
                            {deleting ? '删除中…' : `删除所选${selectedIds.size > 0 ? ` (${selectedIds.size})` : ''}`}
                        </button>
                    </div>
                </section>
            ) : null}

            {loading && rooms.length === 0 ? (
                <div class="rt-topic-grid" aria-busy="true" aria-label="加载中">
                    {[0, 1, 2, 3].map(i => (
                        <div key={i} class="rt-topic-card rt-topic-skeleton" aria-hidden="true">
                            <div class="rt-skel rt-skel-badge" />
                            <div class="rt-skel rt-skel-title" />
                            <div class="rt-skel rt-skel-line" />
                            <div class="rt-skel rt-skel-line short" />
                            <div class="rt-skel rt-skel-foot" />
                        </div>
                    ))}
                </div>
            ) : rooms.length === 0 ? (
                <div class="rt-list-empty-wrap">
                    <div class="rt-list-empty-card">
                        <div class="rt-list-empty-icon" aria-hidden="true">
                            <svg
                                viewBox="0 0 24 24"
                                width="28"
                                height="28"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="1.5"
                            >
                                <circle cx="12" cy="8" r="2.5" />
                                <circle cx="6.5" cy="15" r="2.5" />
                                <circle cx="17.5" cy="15" r="2.5" />
                                <path d="M9.5 9.5 7.5 13M14.5 9.5l2 3.5M8.5 15.5h7" stroke-linecap="round" />
                            </svg>
                        </div>
                        <h2 class="rt-list-empty-title">还没有要讨论的问题</h2>
                        <p class="rt-list-empty-desc">写下一个真实问题，六个职能会先独立判断，再共同收敛行动建议。</p>
                        <button type="button" class="rt-btn rt-btn-primary" onClick={onCreate}>
                            新建第一局
                        </button>
                    </div>
                </div>
            ) : (
                <div
                    class={`rt-topic-grid${selectionMode ? ' is-selecting' : ''}`}
                    role="list"
                    aria-label={selectionMode ? '选择圆桌话题' : '圆桌话题'}
                >
                    {!selectionMode ? (
                        <button
                            type="button"
                            class="rt-topic-card rt-topic-card-new"
                            role="listitem"
                            onClick={onCreate}
                        >
                            <span class="rt-topic-new-plus" aria-hidden="true">
                                +
                            </span>
                            <span class="rt-topic-new-label">新建圆桌</span>
                            <span class="rt-topic-new-hint">从一个需要决策的问题开始</span>
                        </button>
                    ) : null}

                    {rooms.map(room => (
                        <TopicCard
                            key={room.id}
                            room={room}
                            onOpen={() => onOpenRoom(room.id)}
                            selectionMode={selectionMode}
                            selected={selectedIds.has(room.id)}
                            onToggle={() => {
                                setSelectedIds(current => {
                                    const next = new Set(current);
                                    if (next.has(room.id)) {
                                        next.delete(room.id);
                                    } else {
                                        next.add(room.id);
                                    }
                                    return next;
                                });
                            }}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

function TopicCard({
    room,
    onOpen,
    selectionMode,
    selected,
    onToggle,
}: {
    room: RoundtableRoom;
    onOpen: () => void;
    selectionMode: boolean;
    selected: boolean;
    onToggle: () => void;
}) {
    const stageIdx = stageIndexFromState(room.state);
    const tone = stateTone(room.state);
    const needsAction = waitsForUser(room);

    return (
        <div
            class={`rt-topic-card is-${tone}${selectionMode ? ' is-selecting' : ''}${selected ? ' is-selected' : ''}`}
            role="listitem"
        >
            <button
                type="button"
                class="rt-topic-card-content"
                onClick={selectionMode ? onToggle : onOpen}
                aria-label={
                    selectionMode ? `${selected ? '取消选择' : '选择'}：${room.title || '未命名议题'}` : undefined
                }
                aria-pressed={selectionMode ? selected : undefined}
            >
                <div class="rt-topic-card-top">
                    <span class={`rt-topic-badge is-${needsAction ? 'wait' : tone}`}>{roomCardStatus(room)}</span>
                    {selectionMode ? (
                        <span class="rt-topic-selection-mark" aria-hidden="true">
                            {selected ? (
                                <svg viewBox="0 0 16 16" width="12" height="12" fill="none">
                                    <path
                                        d="m3.5 8 3 3 6-6"
                                        stroke="currentColor"
                                        stroke-width="1.8"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    />
                                </svg>
                            ) : null}
                        </span>
                    ) : (
                        <span class="rt-topic-enter" aria-hidden="true">
                            进入
                            <svg class="rt-topic-enter-arrow" viewBox="0 0 16 16" width="14" height="14" fill="none">
                                <path
                                    d="M3 8h9M8.5 4.5 12 8l-3.5 3.5"
                                    stroke="currentColor"
                                    stroke-width="1.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                />
                            </svg>
                        </span>
                    )}
                </div>

                <h2 class="rt-topic-card-title">{room.title || '未命名议题'}</h2>

                {room.brief?.question ? (
                    <p class="rt-topic-question">{room.brief.question}</p>
                ) : (
                    <p class="rt-topic-question is-muted">进入后先和主持人把问题、约束与成功标准说清楚。</p>
                )}

                <div
                    class="rt-topic-stages"
                    aria-label={`讨论进度：${stageIdx >= 0 ? STAGES[stageIdx].label : stateLabel(room.state)}`}
                >
                    <div class="rt-topic-stage-heading">
                        <span>讨论进度</span>
                        <span>
                            {stageIdx >= 0
                                ? `${stageIdx + 1}/${STAGES.length} · ${STAGES[stageIdx].label}`
                                : '需要处理'}
                        </span>
                    </div>
                    <div class="rt-topic-stage-track">
                        {STAGES.map((s, i) => (
                            <span
                                key={s.id}
                                class={`rt-topic-stage-dot${
                                    stageIdx < 0
                                        ? i === 0
                                            ? ' is-error'
                                            : ''
                                        : i < stageIdx
                                          ? ' is-done'
                                          : i === stageIdx
                                            ? ' is-active'
                                            : ''
                                }`}
                                title={s.label}
                            />
                        ))}
                    </div>
                    <div class="rt-topic-stage-labels">
                        {STAGES.map((s, i) => (
                            <span
                                key={s.id}
                                class={`rt-topic-stage-lab${i === stageIdx ? ' is-active' : ''}${
                                    stageIdx >= 0 && i < stageIdx ? ' is-done' : ''
                                }`}
                            >
                                {s.label}
                            </span>
                        ))}
                    </div>
                </div>

                <div class="rt-topic-card-foot">
                    <span class="rt-topic-time">更新于 {formatTime(room.updated_at)}</span>
                    {room.summary_r3 || room.summary_r2 ? <span class="rt-topic-meta">已有阶段结论</span> : null}
                </div>
            </button>
        </div>
    );
}

function waitsForUser(room: RoundtableRoom): boolean {
    if (['confirm_brief', 'start_r2', 'start_r3', 'inspect_failure'].includes(room.next_action)) return true;
    return room.state === 'drafting_brief' || room.state === 'waiting_r2' || room.state === 'waiting_r3';
}

export function roomCardStatus(room: RoundtableRoom): string {
    return waitsForUser(room) ? '等待我操作' : stateLabel(room.state);
}

type StateTone = 'active' | 'wait' | 'done' | 'error' | 'idle';

function stateTone(state: RoomState | string | undefined): StateTone {
    switch (state) {
        case 'drafting_brief':
        case 'summarizing_r2':
        case 'summarizing_r3':
            return 'active';
        case 'waiting_r2':
        case 'waiting_r3':
            return 'wait';
        case 'done':
            return 'done';
        case 'failed':
            return 'error';
        default:
            return 'idle';
    }
}

function formatTime(iso: string | undefined): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return iso;
    try {
        return d.toLocaleString(undefined, {
            month: 'short',
            day: 'numeric',
            hour: '2-digit',
            minute: '2-digit',
        });
    } catch {
        return iso;
    }
}
