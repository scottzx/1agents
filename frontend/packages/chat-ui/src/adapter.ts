import type { ChatItem, ToolCallInfo, TurnChangeReport } from './types';

export interface SessionReaderTurn {
  no?: number;
  turn?: number;
  status?: string;
  prompt?: string;
  userPrompt?: string;
  outcome?: string;
  assistantReply?: string;
  thinking?: string;
  thinkingBlocks?: string[];
  toolCalls?: Array<{
    id?: string;
    toolName: string;
    args?: any;
    result?: any;
    exitCode?: number;
    isError?: boolean;
  }>;
  files?: string[];
  commands?: number;
  errors?: number;
  startedAt?: string;
  endedAt?: string;
  durationMs?: number;
}

function parseTime(timestamp?: string, fallback = Date.now()): number {
  if (!timestamp) return fallback;
  const t = Date.parse(timestamp);
  return isNaN(t) ? fallback : t;
}

export function turnsToChatItems(turns: SessionReaderTurn[]): ChatItem[] {
  if (!turns || !Array.isArray(turns)) return [];
  const items: ChatItem[] = [];

  for (let i = 0; i < turns.length; i++) {
    const t = turns[i]!;
    const turnNo = t.turn ?? t.no ?? (i + 1);
    const turnId = String(turnNo);
    const startTime = parseTime(t.startedAt);
    const endTime = parseTime(t.endedAt, startTime + (t.durationMs || 1000));
    const isTurnError = t.status === 'error' || t.status === 'failed';
    const turnStatus = isTurnError ? 'failed' : 'completed';

    // 1. User Message
    const userPrompt = t.userPrompt || t.prompt || '';
    if (userPrompt) {
      let changeReport: TurnChangeReport | undefined;
      if (t.files && t.files.length > 0) {
        changeReport = {
          turnId,
          addedCount: 0,
          modifiedCount: t.files.length,
          deletedCount: 0,
          files: t.files.map(p => ({ path: p, op: 'modified' })),
        };
      }

      items.push({
        id: `user-${turnNo}`,
        kind: 'user',
        content: userPrompt,
        createdAt: startTime,
        turnId,
        turnStatus,
        changeReport,
      });
    }

    // 2. Thinking
    const thinkings = t.thinkingBlocks && t.thinkingBlocks.length > 0
      ? t.thinkingBlocks
      : (t.thinking && t.thinking.trim() ? [t.thinking] : []);

    for (let idx = 0; idx < thinkings.length; idx++) {
      const th = thinkings[idx]!;
      if (th.trim()) {
        items.push({
          id: `thinking-${turnNo}-${idx}`,
          kind: 'thinking',
          content: th,
          createdAt: startTime + 10 + idx,
          turnId,
          turnStatus,
        });
      }
    }

    // 3. Tool Calls
    if (t.toolCalls && t.toolCalls.length > 0) {
      const calls: ToolCallInfo[] = t.toolCalls.map((tc, idx) => {
        const callId = tc.id || `tc-${turnNo}-${idx}`;
        const inputStr = typeof tc.args === 'string' ? tc.args : (tc.args ? JSON.stringify(tc.args, null, 2) : '');
        const outputStr = typeof tc.result === 'string' ? tc.result : (tc.result !== undefined ? JSON.stringify(tc.result, null, 2) : undefined);
        return {
          id: callId,
          toolCallId: callId,
          toolName: tc.toolName || 'tool',
          input: inputStr,
          output: outputStr,
          isError: tc.isError,
          status: tc.isError ? 'failed' : 'completed',
        };
      });

      items.push({
        id: `tool-group-${turnNo}`,
        kind: 'tool_use',
        toolName: calls[0]?.toolName || 'tool',
        input: calls[0]?.input || '',
        calls,
        createdAt: startTime + 50,
        turnId,
        turnStatus,
      });
    }

    // 4. Assistant Reply
    const asstText = t.assistantReply || t.outcome || '';
    if (asstText && asstText.trim()) {
      items.push({
        id: `assistant-${turnNo}`,
        kind: 'assistant_text',
        content: asstText,
        createdAt: endTime,
        streaming: false,
        turnId,
        turnStatus,
      });
    }
  }

  return items;
}
