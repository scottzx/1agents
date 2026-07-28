import { useEffect, useMemo, useState } from 'preact/hooks';

import type { ChatSession } from '../types';
import type { ChatItem } from './hooks';
import { activityService, type AgentTurn, type ProjectActivityEntry } from '@1agents/core/services/activityService';

export interface TurnProjectionSnapshot {
    turns: AgentTurn[];
    activity: ProjectActivityEntry[];
}

function chronological<T extends { createdAt: string; id: string }>(items: T[]): T[] {
    return [...items].sort((a, b) => {
        const byTime = Date.parse(a.createdAt) - Date.parse(b.createdAt);
        return byTime || a.id.localeCompare(b.id);
    });
}

/**
 * Attach persisted Turn identity to native chat history. Exact prompt matching
 * is preferred; chronological matching is only a fallback for legacy runtimes
 * that normalize prompt text in their transcript.
 */
export function projectChatTurns(
    items: ChatItem[],
    rawTurns: AgentTurn[],
    activity: ProjectActivityEntry[]
): ChatItem[] {
    if (rawTurns.length === 0) return items;

    const turns = chronological(rawTurns);
    const unused = new Set(turns.map(turn => turn.id));
    const activityByTurn = new Map<string, ProjectActivityEntry[]>();
    for (const entry of chronological(activity)) {
        if (!entry.turnId) continue;
        const entries = activityByTurn.get(entry.turnId) ?? [];
        entries.push(entry);
        activityByTurn.set(entry.turnId, entries);
    }

    const starts: number[] = [];
    for (let index = 0; index < items.length; index++) {
        if (items[index].kind === 'user' && items[index].queueStatus !== 'queued') starts.push(index);
    }
    if (starts.length === 0) return items;

    const output: ChatItem[] = items.slice(0, starts[0]);
    for (let segment = 0; segment < starts.length; segment++) {
        const start = starts[segment];
        const end = starts[segment + 1] ?? items.length;
        const user = items[start];
        if (user.kind !== 'user') continue;

        const exact = turns.find(turn => unused.has(turn.id) && turn.promptText === user.content);
        const turn = exact ?? turns.find(candidate => unused.has(candidate.id));
        if (!turn) {
            output.push(...items.slice(start, end));
            continue;
        }
        unused.delete(turn.id);

        for (const item of items.slice(start, end)) {
            output.push({ ...item, turnId: turn.id, turnStatus: turn.status });
        }

        for (const entry of activityByTurn.get(turn.id) ?? []) {
            output.push({
                id: `receipt-${entry.id}`,
                kind: 'turn_receipt',
                content: entry.summary,
                count: entry.count,
                status: entry.status,
                createdAt: Date.parse(entry.createdAt) || Date.now(),
                turnId: turn.id,
                turnStatus: turn.status,
            });
        }

        if (turn.status === 'failed' || turn.status === 'cancelled') {
            output.push({
                id: `turn-status-${turn.id}`,
                kind: 'turn_receipt',
                content:
                    turn.errorText ||
                    (turn.status === 'cancelled'
                        ? 'Turn 已取消'
                        : `Turn 执行失败${turn.errorCode ? `：${turn.errorCode}` : ''}`),
                count: 0,
                status: turn.status,
                createdAt: Date.parse(turn.completedAt || turn.updatedAt) || Date.now(),
                turnId: turn.id,
                turnStatus: turn.status,
            });
        }
    }
    return output;
}

export function useProjectedTurnItems(session: ChatSession, items: ChatItem[], typing: boolean): ChatItem[] {
    const [snapshot, setSnapshot] = useState<TurnProjectionSnapshot>({ turns: [], activity: [] });
    const userSignature = items
        .filter(item => item.kind === 'user')
        .map(item => `${item.id}:${item.queueStatus ?? 'started'}`)
        .join('|');

    useEffect(() => {
        let cancelled = false;

        const refresh = async () => {
            try {
                const [turnPage, activityPage] = await Promise.all([
                    activityService.listTurns(session.workspaceId, { sessionId: session.id, limit: 100 }),
                    activityService.listActivity(session.workspaceId, { sessionId: session.id, limit: 100 }),
                ]);
                if (!cancelled) setSnapshot({ turns: turnPage.items, activity: activityPage.items });
            } catch {
                // Compatibility with older backends: leave the native transcript
                // untouched and let MessageList's legacy grouping stay active.
            }
        };

        void refresh();
        const timer = typing ? setInterval(() => void refresh(), 1500) : null;
        return () => {
            cancelled = true;
            if (timer) clearInterval(timer);
        };
    }, [session.id, session.workspaceId, typing, userSignature]);

    return useMemo(
        () => projectChatTurns(items, snapshot.turns, snapshot.activity),
        [items, snapshot.turns, snapshot.activity]
    );
}
