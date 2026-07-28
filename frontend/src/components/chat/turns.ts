import type { GroupedChatItem, TurnContentItem } from './MessageBubble';

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

    if (turnStarts.length < 2) return items;

    const result: GroupedChatItem[] = items.slice(0, turnStarts[0]);

    for (let turnIndex = 0; turnIndex < turnStarts.length; turnIndex++) {
        const start = turnStarts[turnIndex];
        const end = turnStarts[turnIndex + 1] ?? items.length;
        const user = items[start];
        result.push(user);

        const turnItems = items.slice(start + 1, end) as TurnContentItem[];
        const isLatestTurn = turnIndex === turnStarts.length - 1;
        if (isLatestTurn || turnItems.length === 0) {
            result.push(...turnItems);
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
                if (turnItems[index].kind === 'error') {
                    outcomeId = turnItems[index].id;
                    break;
                }
            }
        }

        const hasHiddenProcess = turnItems.some(item => item.id !== outcomeId);
        if (!hasHiddenProcess) {
            result.push(...turnItems);
            continue;
        }

        result.push({
            id: `turn-${user.id}`,
            kind: 'turn',
            items: turnItems,
            outcomeId,
            createdAt: user.createdAt,
        });
    }

    return result;
}
