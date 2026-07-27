import { h } from 'preact';
import { useState } from 'preact/hooks';
import type { RefObject } from 'preact';
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { liveSessionStatus } from '../../stores/sessionStore';
import { BriefInspector } from './BriefInspector';
import { RoundtableSidebarView } from './RoundtableSidebarView';

interface RoundtableSidebarProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
    turns: RoundtableTurn[];
    loading?: boolean;
    activeTab: 'topic' | 'participants';
    onTabChange: (tab: 'topic' | 'participants') => void;
    inspectorRef?: RefObject<HTMLElement>;
    onRoomUpdate?: (room: RoundtableRoom) => void | Promise<void>;
    onReload?: () => void | Promise<void>;
}

/** Connects live seat behavior to the presentational Inspector. */
export function RoundtableSidebar({
    room,
    seats,
    turns,
    loading,
    activeTab,
    onTabChange,
    inspectorRef,
    onRoomUpdate,
    onReload,
}: RoundtableSidebarProps) {
    const [openingId, setOpeningId] = useState<string | null>(null);

    const onSeatClick = async (seat: RoundtableSeat) => {
        if (!seat.session_id?.trim() || openingId) return;
        setOpeningId(seat.id);
        try {
            const { openSeatSession } = await import('./openSeatSession');
            await openSeatSession(seat, { roomId: room.id, roomTitle: room.title });
        } finally {
            setOpeningId(null);
        }
    };

    return (
        <RoundtableSidebarView
            room={room}
            seats={seats}
            turns={turns}
            activeTab={activeTab}
            onTabChange={onTabChange}
            liveMap={liveSessionStatus.value}
            openingId={openingId}
            onSeatClick={onSeatClick}
            briefInspector={
                <BriefInspector
                    room={room}
                    loading={loading}
                    sectionRef={inspectorRef}
                    onRoomUpdate={onRoomUpdate}
                    onReload={onReload}
                />
            }
        />
    );
}
