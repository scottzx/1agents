import { h, Fragment } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
    roundtableService,
    type RoundtableRoom as Room,
    type RoundtableSeat,
    type RoundtableTurn,
} from '@1agents/core/services/roundtableService';
import { StageBar } from './StageBar';
import { SeatBar } from './SeatBar';
import { TurnCard } from './TurnCard';
import { RoundtableSidebar } from './RoundtableSidebar';
import { LaunchWizard } from './LaunchWizard';
import { RoomList } from './RoomList';
import { isTerminalState, pollIntervalMs } from './stage';
import { persistListView, persistRoomView, readStoredRoomId, resolveInitialNav } from './navState';
import { roundtableBreadcrumbs } from './breadcrumbs';
import * as taskNav from '../../stores/taskNavStore';
import * as tabsStore from '../../stores/tabsStore';
import * as stage from '../../stores/stageStore';

export interface RoundtableRoomProps {
    /** When set, open this room directly. */
    roomId?: string;
    /** Optional title seed when creating a room. */
    defaultTitle?: string;
    /**
     * @deprecated Prefer last-view restore. When true and no stored room view,
     * still lands on the topic list (create via「新建」).
     */
    preferWizard?: boolean;
}

type ShellView = 'list' | 'room' | 'create';

/**
 * Roundtable app shell:
 * - list: topic cards (4 per row)
 * - create: launch wizard
 * - room: active timeline + seats
 * Last list/room position is restored when re-entering from the sidebar.
 */
export function RoundtableRoomView({ roomId: roomIdProp, defaultTitle }: RoundtableRoomProps) {
    const initial = resolveInitialNav(roomIdProp);
    const [shell, setShell] = useState<ShellView>(() => (initial.view === 'room' ? 'room' : 'list'));
    const [roomId, setRoomId] = useState<string | undefined>(() => initial.roomId);
    const [room, setRoom] = useState<Room | null>(null);
    const [turns, setTurns] = useState<RoundtableTurn[]>([]);
    const [seats, setSeats] = useState<RoundtableSeat[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState(false);
    const [briefTitle, setBriefTitle] = useState('');
    const [briefQuestion, setBriefQuestion] = useState('');
    const [briefConstraints, setBriefConstraints] = useState('');
    const [briefSuccess, setBriefSuccess] = useState('');
    const [showBriefForm, setShowBriefForm] = useState(false);
    const [listRefreshKey, setListRefreshKey] = useState(0);
    const pollRef = useRef<number | null>(null);
    const timelineRef = useRef<HTMLDivElement>(null);

    const goList = useCallback(() => {
        setShell('list');
        setRoomId(undefined);
        setRoom(null);
        setTurns([]);
        setSeats([]);
        setError(null);
        persistListView();
        setListRefreshKey(k => k + 1);
    }, []);

    const goRoom = useCallback((id: string) => {
        setError(null);
        setLoading(true);
        setRoomId(id);
        setShell('room');
        persistRoomView(id);
    }, []);

    const goCreate = useCallback(() => {
        setShell('create');
        setRoomId(undefined);
        setRoom(null);
        setError(null);
        // Creating is transient; remember list so cancel/back returns to cards.
        persistListView();
    }, []);

    const leaveRoundtable = useCallback(() => {
        taskNav.headerCrumbs.value = null;
        taskNav.clearHeaderBackAction('roundtable-room');
        stage.exitL1App();
        tabsStore.selectDiscoveryCategory('apps');
    }, []);

    useEffect(() => {
        const backAction = shell === 'list' ? leaveRoundtable : goList;
        const crumbs = roundtableBreadcrumbs({
            view: shell,
            roomTitle: room?.title || defaultTitle,
            onList: goList,
        });
        taskNav.headerCrumbs.value = crumbs;
        const unregisterBack = taskNav.registerHeaderBackAction(
            'roundtable-room',
            backAction,
            taskNav.HEADER_BACK_PRIORITY.surface
        );
        return () => {
            if (taskNav.headerCrumbs.value === crumbs) {
                taskNav.headerCrumbs.value = null;
            }
            unregisterBack();
        };
    }, [shell, room?.title, defaultTitle, goList, leaveRoundtable]);

    const refresh = useCallback(async (id: string, opts?: { quiet?: boolean }) => {
        if (!opts?.quiet) setLoading(true);
        try {
            const r = await roundtableService.getRoom(id);
            setRoom(r);
            setSeats(r.seats || []);
            if (r.turns && r.turns.length) {
                setTurns(r.turns);
            } else {
                const t = await roundtableService.listTurns(id);
                setTurns(t);
            }
            setError(null);
            persistRoomView(id);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            if (!opts?.quiet) setLoading(false);
        }
    }, []);

    useEffect(() => {
        if (roomIdProp && roomIdProp !== roomId) {
            goRoom(roomIdProp);
        }
    }, [roomIdProp]);

    useEffect(() => {
        if (shell !== 'room' || !roomId) {
            if (shell !== 'room') {
                setRoom(null);
                setTurns([]);
                setSeats([]);
            }
            return;
        }
        void refresh(roomId);
    }, [shell, roomId, refresh]);

    const anySpeaking = seats.some(s => s.status === 'speaking');
    const drafting = room?.state === 'drafting_brief';

    useEffect(() => {
        if (pollRef.current) {
            window.clearInterval(pollRef.current);
            pollRef.current = null;
        }
        if (shell !== 'room' || !roomId || !room || isTerminalState(room.state)) return;
        const base = pollIntervalMs(room.state);
        if (base <= 0) return;
        const ms = anySpeaking || busy ? Math.min(base, 1500) : base;
        pollRef.current = window.setInterval(() => {
            void refresh(roomId, { quiet: true });
        }, ms);
        return () => {
            if (pollRef.current) {
                window.clearInterval(pollRef.current);
                pollRef.current = null;
            }
        };
    }, [shell, roomId, room?.state, anySpeaking, busy, refresh]);

    useEffect(() => {
        const el = timelineRef.current;
        if (!el) return;
        el.scrollTop = el.scrollHeight;
    }, [turns.length]);

    const createRoom = async (title: string) => {
        setBusy(true);
        setError(null);
        try {
            const r = await roundtableService.createRoom({
                title: title.trim() || '圆桌议题',
            });
            setRoomId(r.id);
            setRoom(r);
            setSeats(r.seats || []);
            setTurns(r.turns || []);
            setShell('room');
            persistRoomView(r.id);
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const briefFormReady =
        briefTitle.trim().length > 0 &&
        briefQuestion.trim().length > 0 &&
        briefConstraints.trim().length > 0 &&
        briefSuccess.trim().length > 0 &&
        !['—', '-', '–', 'TBD', 'tbd', 'N/A', 'n/a'].includes(briefConstraints.trim()) &&
        !['—', '-', '–', 'TBD', 'tbd', 'N/A', 'n/a'].includes(briefSuccess.trim());

    const confirmBrief = async () => {
        if (!roomId) return;
        const title = briefTitle.trim();
        const question = briefQuestion.trim();
        const constraints = briefConstraints.trim();
        const success = briefSuccess.trim();
        // Never silent-fill with room title / "—" — empty Brief must not enter R2.
        if (!title || !question || !constraints || !success) {
            setError('请完整填写 Brief 四字段（标题 / 议题 / 约束 / 成功标准），不可用「—」占位');
            return;
        }
        if (
            ['—', '-', '–', 'TBD', 'tbd', 'N/A', 'n/a'].includes(constraints) ||
            ['—', '-', '–', 'TBD', 'tbd', 'N/A', 'n/a'].includes(success)
        ) {
            setError('约束与成功标准不能使用占位符，请填写真实内容');
            return;
        }
        setBusy(true);
        setError(null);
        try {
            await roundtableService.confirmBrief(roomId, {
                title,
                question,
                constraints,
                success_criteria: success,
            });
            setShowBriefForm(false);
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const runR2 = async () => {
        if (!roomId) return;
        setBusy(true);
        setError(null);
        try {
            await roundtableService.runR2(roomId);
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const runR3 = async () => {
        if (!roomId) return;
        setBusy(true);
        setError(null);
        try {
            await roundtableService.runR3(roomId);
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    if (shell === 'list') {
        return <RoomList onOpenRoom={goRoom} onCreate={goCreate} refreshKey={listRefreshKey} />;
    }

    if (shell === 'create') {
        return (
            <LaunchWizard
                busy={busy}
                error={error}
                previousRoomId={readStoredRoomId()}
                defaultTitle={defaultTitle}
                onStart={createRoom}
                onContinue={id => goRoom(id)}
            />
        );
    }

    // shell === 'room'
    if (!roomId) {
        // Missing id while on room view — fall back to list without setState during render.
        queueMicrotask(() => goList());
        return (
            <div class="rt-room rt-room-empty">
                <div class="rt-empty-card bento-card">
                    <div class="bento-zone-body">
                        <div class="rt-room-loading">返回列表…</div>
                    </div>
                </div>
            </div>
        );
    }

    if (!room) {
        return (
            <div class="rt-room rt-room-empty">
                <div class="rt-empty-card bento-card">
                    <div class="bento-zone-body">
                        {loading ? (
                            <div class="rt-room-loading">加载圆桌…</div>
                        ) : (
                            <div class="rt-error">{error || '无法加载圆桌'}</div>
                        )}
                    </div>
                </div>
            </div>
        );
    }

    const showR2 = room.state === 'waiting_r2';
    const showR3 = room.state === 'waiting_r3';

    return (
        <div class="rt-room">
            <div class="rt-room-main">
                <header class="rt-room-header">
                    <div class="rt-room-title-row">
                        <h1 class="rt-room-title">{room.title || '圆桌'}</h1>
                        {loading && <span class="rt-room-loading">刷新中</span>}
                        <button
                            type="button"
                            class="rt-btn rt-btn-ghost"
                            disabled={busy || loading}
                            onClick={() => void refresh(roomId)}
                        >
                            刷新
                        </button>
                    </div>
                    <StageBar state={room.state} />
                    <SeatBar seats={seats} />
                    {(showR2 || showR3 || drafting) && (
                        <div class="rt-room-actions">
                            {drafting && (
                                <button
                                    type="button"
                                    class="rt-btn"
                                    disabled={busy}
                                    onClick={() => {
                                        setShowBriefForm(v => !v);
                                    }}
                                >
                                    {showBriefForm ? '收起 Brief' : '确认 Brief'}
                                </button>
                            )}
                            {showR2 && (
                                <button
                                    type="button"
                                    class="rt-btn rt-btn-primary"
                                    disabled={busy}
                                    onClick={() => void runR2()}
                                >
                                    启动 R2 首轮
                                </button>
                            )}
                            {showR3 && (
                                <button
                                    type="button"
                                    class="rt-btn rt-btn-primary"
                                    disabled={busy}
                                    onClick={() => void runR3()}
                                >
                                    启动 R3 次轮
                                </button>
                            )}
                        </div>
                    )}
                    {/* CLI-confirmed brief is already on room; show read-only card when past drafting. */}
                    {!drafting && room.brief && (
                        <div class="rt-brief-readonly bento-card" aria-label="已确认 Brief">
                            <div class="bento-zone-header">
                                <span class="bento-card-title">Brief（已确认）</span>
                            </div>
                            <div class="bento-zone-body">
                                <dl class="rt-brief-dl">
                                    <dt>标题</dt>
                                    <dd>{room.brief.title}</dd>
                                    <dt>议题</dt>
                                    <dd>{room.brief.question}</dd>
                                    <dt>约束</dt>
                                    <dd>{room.brief.constraints}</dd>
                                    <dt>成功标准</dt>
                                    <dd>{room.brief.success_criteria}</dd>
                                    {room.brief.product_kind ? (
                                        <>
                                            <dt>品类</dt>
                                            <dd>{room.brief.product_kind}</dd>
                                        </>
                                    ) : null}
                                </dl>
                            </div>
                        </div>
                    )}
                    {drafting && showBriefForm && (
                        <div class="rt-brief-form">
                            <p class="rt-brief-hint">
                                四字段均必填；请填真实内容。裁判也可在 seat cwd 用 <code>roundtable set-brief</code>{' '}
                                写入（开发环境用二进制绝对路径或 ONEAGENTS_CLI；刷新后可见）。
                            </p>
                            <label class="rt-field">
                                <span class="rt-field-label">标题</span>
                                <input
                                    class="rt-input"
                                    value={briefTitle}
                                    onInput={e => setBriefTitle((e.target as HTMLInputElement).value)}
                                    required
                                />
                            </label>
                            <label class="rt-field">
                                <span class="rt-field-label">议题 / 问题</span>
                                <input
                                    class="rt-input"
                                    value={briefQuestion}
                                    onInput={e => setBriefQuestion((e.target as HTMLInputElement).value)}
                                    required
                                />
                            </label>
                            <label class="rt-field">
                                <span class="rt-field-label">约束</span>
                                <input
                                    class="rt-input"
                                    value={briefConstraints}
                                    onInput={e => setBriefConstraints((e.target as HTMLInputElement).value)}
                                    required
                                />
                            </label>
                            <label class="rt-field">
                                <span class="rt-field-label">成功标准</span>
                                <input
                                    class="rt-input"
                                    value={briefSuccess}
                                    onInput={e => setBriefSuccess((e.target as HTMLInputElement).value)}
                                    required
                                />
                            </label>
                            <button
                                type="button"
                                class="rt-btn rt-btn-primary"
                                disabled={busy || !briefFormReady}
                                onClick={() => void confirmBrief()}
                            >
                                提交 Brief → waiting_r2
                            </button>
                        </div>
                    )}
                    {error && <div class="rt-error">{error}</div>}
                </header>

                <div class="rt-timeline" ref={timelineRef} role="log" aria-live="polite" aria-label="主时间线">
                    {turns.length === 0 ? (
                        <div class="rt-timeline-empty">
                            {drafting
                                ? '点击右侧「裁判」席位打开完整 ChatUI 澄清议题；确认 Brief 后进入 R2。'
                                : '尚无发言。'}
                        </div>
                    ) : (
                        <Fragment>
                            {turns.map(t => (
                                <TurnCard key={t.id} turn={t} seats={seats} />
                            ))}
                        </Fragment>
                    )}
                </div>
            </div>

            <RoundtableSidebar room={room} seats={seats} />
        </div>
    );
}
