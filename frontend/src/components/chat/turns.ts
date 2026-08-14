import type { TurnChangeReport } from '@1agents/core/services/activityService';
import type { GroupedChatItem, TurnContentItem } from './MessageBubble';
import { inferFilesFromTurnItems, mergeChangeReport } from '../drawer/sessionStatusModel';

/**
 * Collapse every completed turn except the newest one into a single render
 * item. A turn starts at a non-queued user message and runs until the next
 * non-queued user message. Queued prompts are deliberately not boundaries:
 * they have not started a turn yet.
 *
 * The user bubble stays in the outer timeline. The synthesized turn item owns
 * the agent-side activity and identifies the last assistant response so its
 * renderer can keep that response visible while folding thoughts/tools.
 */
export function groupHistoricalTurns(items: GroupedChatItem[]): GroupedChatItem[] {
    const turnStarts: number[] = [];
    for (let index = 0; index < items.length; index++) {
        const item = items[index];
        if (item.kind === 'user' && item.queueStatus !== 'queued') {
            turnStarts.push(index);
        }
    }

    if (turnStarts.length === 0) return items;

    if (turnStarts.length === 1) {
        const start = turnStarts[0];
        const user = items[start];
        const turnItems = items.slice(start + 1) as TurnContentItem[];
        const rawReport = user.kind === 'user' ? user.changeReport : undefined;
        const report = mergeChangeReport(rawReport, inferFilesFromTurnItems(turnItems));
        if (!hasChangeCounts(report)) return items;
    }

    const result: GroupedChatItem[] = items.slice(0, turnStarts[0]);

    for (let turnIndex = 0; turnIndex < turnStarts.length; turnIndex++) {
        const start = turnStarts[turnIndex];
        const end = turnStarts[turnIndex + 1] ?? items.length;
        const user = items[start];
        const rawReport = user.kind === 'user' ? user.changeReport : undefined;
        const turnItems = items.slice(start + 1, end) as TurnContentItem[];
        const report = mergeChangeReport(rawReport, inferFilesFromTurnItems(turnItems));
        const isLatestTurn = turnIndex === turnStarts.length - 1;
        if (isLatestTurn || turnItems.length === 0) {
            result.push(user.kind === 'user' && user.changeReport ? { ...user, changeReport: undefined } : user);
            pushFlatTurnItems(result, user, turnItems, report);
            continue;
        }

        let outcomeId: string | undefined;
        for (let index = turnItems.length - 1; index >= 0; index--) {
            if (turnItems[index].kind === 'assistant_text') {
                outcomeId = turnItems[index].id;
                break;
            }
        }

        // Cancelled/failed turns may not have a final assistant response. Keep
        // their terminal error visible instead of reducing the turn to only a
        // process header.
        if (!outcomeId) {
            for (let index = turnItems.length - 1; index >= 0; index--) {
                const item = turnItems[index];
                if (item.kind === 'error' || (item.kind === 'turn_receipt' && item.status !== 'succeeded')) {
                    outcomeId = item.id;
                    break;
                }
            }
        }

        const hasHiddenProcess = turnItems.some(item => item.id !== outcomeId);
        if (!hasHiddenProcess) {
            result.push(user.kind === 'user' && user.changeReport ? { ...user, changeReport: undefined } : user);
            pushFlatTurnItems(result, user, turnItems, report);
            continue;
        }

        result.push(user.kind === 'user' && user.changeReport ? { ...user, changeReport: undefined } : user);
        result.push({
            id: `turn-${user.turnId || user.id}`,
            kind: 'turn',
            items: attachChangeReport(turnItems, report),
            outcomeId,
            createdAt: user.createdAt,
            turnId: user.turnId,
            turnStatus: user.turnStatus,
            changeReport: report,
        });
    }

    return result;
}

function hasChangeCounts(report: TurnChangeReport | undefined): report is TurnChangeReport {
    return !!report && (report.addedCount > 0 || report.deletedCount > 0 || report.modifiedCount > 0);
}

/**
 * Pin the file list to the last assistant answer so it renders at the
 * bottom of that block. Turns without an answer keep a trailing sibling.
 */
function pushFlatTurnItems(
    result: GroupedChatItem[],
    user: GroupedChatItem,
    turnItems: TurnContentItem[],
    report: TurnChangeReport | undefined
): void {
    const annotated = attachChangeReport(turnItems, report);
    result.push(...annotated);
    if (hasChangeCounts(report) && !annotated.some(item => item.kind === 'assistant_text' && item.changeReport)) {
        result.push({
            id: `turn-changes-${user.turnId || user.id}`,
            kind: 'turn_changes',
            createdAt: user.createdAt,
            turnId: user.turnId,
            turnStatus: user.kind === 'user' ? user.turnStatus : undefined,
            changeReport: report,
        });
    }
}

function attachChangeReport(items: TurnContentItem[], report: TurnChangeReport | undefined): TurnContentItem[] {
    const stripped = items.map(item =>
        item.kind === 'assistant_text' && item.changeReport ? { ...item, changeReport: undefined } : item
    );
    if (!hasChangeCounts(report)) return stripped;
    for (let index = stripped.length - 1; index >= 0; index--) {
        const item = stripped[index];
        if (item.kind !== 'assistant_text') continue;
        const next = stripped.slice();
        next[index] = { ...item, changeReport: report };
        return next;
    }
    return stripped;
}
