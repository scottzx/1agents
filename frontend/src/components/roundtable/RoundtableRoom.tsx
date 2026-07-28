import { h } from 'preact';
import { useCallback, useEffect, useRef, useState } from 'preact/hooks';
import {
    roundtableService,
    type RoundtableRoom as Room,
    type RoundtableSeat,
    type RoundtableTurn,
} from '@1agents/core/services/roundtableService';
import { R1Workbench } from './R1Workbench';
import { RoundtableRoomContent, type RoundtableMobilePane } from './RoundtableRoomContent';
import { RoundtableHeader } from './RoundtableHeader';
import { RoundRecoveryNotice } from './RoundRecoveryNotice';
import { RoundtableSidebar } from './RoundtableSidebar';
import { StageWorkbench } from './StageWorkbench';
import { LaunchWizard } from './LaunchWizard';
import { RoomList } from './RoomList';
import { isTerminalState, pollIntervalMs } from './stage';
import { primaryActionForRoom, type RoundtablePrimaryActionId } from './primaryAction';
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
 * - room: controller-driven R1/R2/R3/Done workbench + Inspector
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
    const [inspectorTab, setInspectorTab] = useState<'topic' | 'participants'>('topic');
    const [mobilePane, setMobilePane] = useState<RoundtableMobilePane>('discussion');
    const [listRefreshKey, setListRefreshKey] = useState(0);
    const pollRef = useRef<number | null>(null);
    const briefInspectorRef = useRef<HTMLElement>(null);
    const roomStatusRef = useRef<HTMLDivElement>(null);
    const pendingRecoveryFocusRef = useRef(false);

    const goList = useCallback(() => {
        setShell('list');
        setRoomId(undefined);
        setRoom(null);
        setTurns([]);
        setSeats([]);
        setError(null);
        setMobilePane('discussion');
        persistListView();
        setListRefreshKey(k => k + 1);
    }, []);

    const goRoom = useCallback((id: string) => {
        setError(null);
        setLoading(true);
        setMobilePane('discussion');
        setRoomId(id);
        setShell('room');
        persistRoomView(id);
    }, []);

    const goCreate = useCallback(() => {
        setShell('create');
        setRoomId(undefined);
        setRoom(null);
        setError(null);
        setMobilePane('discussion');
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

    const applyRoom = useCallback((next: Room) => {
        setRoom(next);
        setSeats(next.seats || []);
        setTurns(next.turns || []);
    }, []);

    const refresh = useCallback(async (id: string, opts?: { quiet?: boolean }): Promise<boolean> => {
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
            return true;
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
            return false;
        } finally {
            if (!opts?.quiet) setLoading(false);
        }
    }, []);

    useEffect(() => {
        if (!room || !pendingRecoveryFocusRef.current) return;
        pendingRecoveryFocusRef.current = false;
        queueMicrotask(() => roomStatusRef.current?.focus({ preventScroll: true }));
    }, [room]);

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

    const sendR1 = async (text: string) => {
        if (!roomId || busy) return;
        setBusy(true);
        setError(null);
        try {
            await roundtableService.chat(roomId, { text });
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const focusBriefInspector = useCallback(() => {
        setInspectorTab('topic');
        setMobilePane('brief');
        queueMicrotask(() => {
            const inspector = briefInspectorRef.current;
            if (!inspector) return;
            inspector.scrollIntoView({ behavior: 'smooth', block: 'start' });
            inspector.focus({ preventScroll: true });
        });
    }, []);

    const openSeatSession = async (seat: RoundtableSeat) => {
        if (!room) return;
        const { openSeatSession: open } = await import('./openSeatSession');
        await open(seat, { roomId: room.id, roomTitle: room.title });
    };

    const runR2 = async () => {
        if (!roomId) return;
        setBusy(true);
        setError(null);
        try {
            // Use legacy sync path to wait until the turn has ended, ensuring conclusions are written back to the room response before UI refresh.
            const result = await roundtableService.runR2Legacy(roomId);
            setMobilePane('discussion');
            applyRoom(result as unknown as Room); // legacy returns unknown, but contains full room with turns
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
            // Use legacy sync path to wait until the turn has ended, ensuring conclusions are written back to the room response before UI refresh.
            const result = await roundtableService.runR3Legacy(roomId);
            setMobilePane('discussion');
            applyRoom(result as unknown as Room); // legacy returns unknown, but contains full room with turns
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const recover = async (action: () => Promise<{ room: Room }>) => {
        if (!roomId || busy) return;
        setBusy(true);
        setError(null);
        try {
            const result = await action();
            pendingRecoveryFocusRef.current = true;
            applyRoom(result.room);
            await refresh(roomId, { quiet: true });
        } catch (e) {
            setError(e instanceof Error ? e.message : String(e));
        } finally {
            setBusy(false);
        }
    };

    const reloadAfterFailure = async () => {
        if (!roomId || busy) return;
        setBusy(true);
        pendingRecoveryFocusRef.current = true;
        const restored = await refresh(roomId, { quiet: true });
        if (!restored) pendingRecoveryFocusRef.current = false;
        setBusy(false);
    };

    const selectMobilePane = (pane: RoundtableMobilePane) => {
        setMobilePane(pane);
        if (pane === 'brief') setInspectorTab('topic');
        if (pane === 'participants') setInspectorTab('participants');
    };

    const retrySeat = (role: RoundtableSeat['role']) => {
        if (!roomId || !room?.active_run) return;
        return recover(() => roundtableService.retrySeat(roomId, room.active_run!.id, role));
    };

    const skipFailedSeats = () => {
        if (!roomId || !room?.active_run) return;
        return recover(() => roundtableService.skipFailedSeats(roomId, room.active_run!.id));
    };

    const retrySummary = () => {
        if (!roomId || !room?.active_run) return;
        return recover(() => roundtableService.retrySummary(roomId, room.active_run!.id));
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
                            <div class="rt-recovery is-room" role="alert">
                                <div>
                                    <strong>房间加载失败</strong>
                                    <p>{error || '无法从服务端恢复当前圆桌。'}</p>
                                </div>
                                <button type="button" class="rt-btn" onClick={() => void reloadAfterFailure()}>
                                    重新加载房间
                                </button>
                            </div>
                        )}
                    </div>
                </div>
            </div>
        );
    }

    const primaryAction = primaryActionForRoom(room);
    const invokePrimaryAction = (id: RoundtablePrimaryActionId) => {
        if (id === 'confirm_brief') return focusBriefInspector();
        if (id === 'start_r2') return void runR2();
        if (id === 'start_r3') return void runR3();
        if (id === 'inspect_failure') return void reloadAfterFailure();
    };
    const headerAction = !primaryAction ? null : primaryAction.kind === 'status' ? (
        <span class="rt-room-waiting" role="status">
            {primaryAction.label}
        </span>
    ) : (
        <button
            type="button"
            class={`rt-btn${primaryAction.primary ? ' rt-btn-primary' : ''}`}
            disabled={busy || loading}
            onClick={() => invokePrimaryAction(primaryAction.id)}
        >
            {primaryAction.label}
        </button>
    );

    const header = (
        <RoundtableHeader room={room} busy={busy} loading={loading} action={headerAction} statusRef={roomStatusRef} />
    );

    return (
        <RoundtableRoomContent
            room={room}
            seats={seats}
            turns={turns}
            header={header}
            mobilePane={mobilePane}
            onMobilePaneChange={selectMobilePane}
            notice={
                <div class="rt-room-notices">
                    {error && (
                        <div class="rt-error" role="alert">
                            {error}
                        </div>
                    )}
                    <RoundRecoveryNotice
                        room={room}
                        busy={busy}
                        onRetrySeat={retrySeat}
                        onSkip={skipFailedSeats}
                        onRetrySummary={retrySummary}
                        onReload={reloadAfterFailure}
                    />
                </div>
            }
            primaryContent={
                drafting ? (
                    <R1Workbench
                        room={room}
                        seats={seats}
                        turns={turns}
                        sending={busy}
                        onSend={sendR1}
                        onFocusBrief={focusBriefInspector}
                        onOpenReferee={openSeatSession}
                    />
                ) : (
                    <StageWorkbench room={room} seats={seats} turns={turns} onOpenSeat={openSeatSession} />
                )
            }
            sidebar={
                <RoundtableSidebar
                    room={room}
                    seats={seats}
                    turns={turns}
                    loading={loading}
                    activeTab={inspectorTab}
                    onTabChange={setInspectorTab}
                    inspectorRef={briefInspectorRef}
                    onRoomUpdate={applyRoom}
                    onReload={() => void refresh(roomId, { quiet: true })}
                />
            }
        />
    );
}
