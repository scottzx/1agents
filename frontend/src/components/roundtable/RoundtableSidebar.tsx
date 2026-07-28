import { h } from 'preact';
import { useState } from 'preact/hooks';
import type { RoundtableRoom, RoundtableSeat } from '@1agents/core/services/roundtableService';
import { liveSessionStatus } from '../../stores/sessionStore';
import { RoundtableSidebarView } from './RoundtableSidebarView';

interface RoundtableSidebarProps {
    room: RoundtableRoom;
}

/** Connects live seat behavior to the presentational role panel. */
export function RoundtableSidebar({ room }: RoundtableSidebarProps) {
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
            seats={room.seats || []}
            liveMap={liveSessionStatus.value}
            openingId={openingId}
            onSeatClick={onSeatClick}
        />
    );
}
