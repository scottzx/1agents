/**
 * Agents 圆桌 — app view registration (design §6 / §6.3).
 * Manifest id: agents-roundtable · discovery → 应用.
 * Entry restores last view (topic list or open room) via localStorage.
 */
import { registerAppView } from '../../modules/appViewRegistry';
import type { AppViewProps } from '../../modules/appViewRegistry';
import { h } from 'preact';
import { RoundtableRoomView } from '../../components/roundtable';

function RoundtableAppView(_props: AppViewProps) {
    void _props;
    // No preferWizard: RoomList is home; create via「新建圆桌」; re-open last room if left mid-session.
    return <RoundtableRoomView />;
}

registerAppView('RoundtableRoom', RoundtableAppView);
registerAppView('AgentsRoundtable', RoundtableAppView);
