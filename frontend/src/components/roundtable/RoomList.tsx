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
 * Topic card grid — 4 columns (codex-minimal / bento).
 * Click a card to enter that room.
 */
export function RoomList({ onOpenRoom, onCreate, refreshKey = 0 }: RoomListProps) {
    const [rooms, setRooms] = useState<RoundtableRoom[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const list = await roundtableService.listRooms(100);
            setRooms(list);
            setError(null);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load, refreshKey]);

    return (
        <div class="rt-room rt-room-list">
            <header class="rt-list-header">
                <div class="rt-list-title-block">
                    <span class="rt-list-kicker">Agents · Roundtable</span>
                    <h1 class="rt-list-title">圆桌话题</h1>
                    <p class="rt-list-desc">真多 session 编排 · 裁判 + 五职能 · 固定三轮。点卡片进入，或新建一局。</p>
                </div>
                <div class="rt-list-actions">
                    <button
                        type="button"
                        class="rt-btn rt-btn-ghost"
                        disabled={loading}
                        onClick={() => void load()}
                        aria-label="刷新列表"
                    >
                        刷新
                    </button>
                    <button type="button" class="rt-btn rt-btn-primary" onClick={onCreate}>
                        新建圆桌
                    </button>
                </div>
            </header>

            {error && (
                <div class="rt-error rt-list-error" role="alert">
                    {error}
                </div>
            )}

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
                        <h2 class="rt-list-empty-title">还没有圆桌话题</h2>
                        <p class="rt-list-empty-desc">创建一局后会出现在这里。一行最多四张卡片，随时从侧栏回来继续。</p>
                        <button type="button" class="rt-btn rt-btn-primary" onClick={onCreate}>
                            新建第一局
                        </button>
                    </div>
                </div>
            ) : (
                <div class="rt-topic-grid" role="list" aria-label="圆桌话题">
                    <button type="button" class="rt-topic-card rt-topic-card-new" role="listitem" onClick={onCreate}>
                        <span class="rt-topic-new-plus" aria-hidden="true">
                            +
                        </span>
                        <span class="rt-topic-new-label">新建圆桌</span>
                        <span class="rt-topic-new-hint">议题草稿 · 固定 6 席</span>
                    </button>

                    {rooms.map(room => (
                        <TopicCard key={room.id} room={room} onOpen={() => onOpenRoom(room.id)} />
                    ))}
                </div>
            )}
        </div>
    );
}

function TopicCard({ room, onOpen }: { room: RoundtableRoom; onOpen: () => void }) {
    const stageIdx = stageIndexFromState(room.state);
    const tone = stateTone(room.state);

    return (
        <button type="button" class={`rt-topic-card is-${tone}`} role="listitem" onClick={onOpen}>
            <div class="rt-topic-card-top">
                <span class={`rt-topic-badge is-${tone}`}>{stateLabel(room.state)}</span>
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
            </div>

            <h2 class="rt-topic-card-title">{room.title || '未命名议题'}</h2>

            {room.brief?.question ? (
                <p class="rt-topic-question">{room.brief.question}</p>
            ) : (
                <p class="rt-topic-question is-muted">尚未确认 Brief · 可在 R1 与裁判澄清</p>
            )}

            <div class="rt-topic-stages" aria-hidden="true">
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
                <span class="rt-topic-time">{formatTime(room.updated_at)}</span>
                {room.summary_r3 || room.summary_r2 ? <span class="rt-topic-meta">有总结</span> : null}
            </div>
        </button>
    );
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
