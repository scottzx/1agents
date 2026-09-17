import type {
  TurnChangeReport,
  TurnChangeFile,
  TurnChangeOp,
  ToolCallInfo,
  GroupedChatItem,
  TurnContentItem,
} from './types';

function opFromTool(call: { toolName: string; input?: string; kind?: string }): TurnChangeOp | null {
  const kind = (call.kind || '').toLowerCase();
  const name = (call.toolName || '').toLowerCase();
  if (kind === 'read' || kind === 'search' || kind === 'think' || kind === 'fetch') return null;
  if (kind === 'delete' || /(delete|remove|\brm\b|unlink)/.test(name)) {
    return 'deleted';
  }
  if (kind === 'edit' || /(write|create_file|apply_patch|multiedit|str_replace|\bedit\b)/.test(name)) {
    return /(write|create_file)/.test(name) ? 'added' : 'modified';
  }
  return null;
}

function pathsFromCall(call: ToolCallInfo): string[] {
  const paths = [
    ...(call.locations ?? []).map(location => location.path),
    ...(call.diffs ?? []).map(diff => diff.path),
  ];
  if (call.input) {
    try {
      const parsed = JSON.parse(call.input) as Record<string, unknown>;
      for (const key of ['path', 'file_path', 'filePath', 'file', 'target_file', 'targetFile']) {
        const value = parsed[key];
        if (typeof value === 'string' && value.trim()) paths.push(value.trim());
      }
    } catch {}
  }
  const seen = new Set<string>();
  const out: string[] = [];
  for (const p of paths) {
    const trimmed = p.trim();
    if (!trimmed || seen.has(trimmed) || trimmed.startsWith('http://') || trimmed.startsWith('https://')) continue;
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}

export function inferFilesFromTurnItems(items: Array<{ kind: string; calls?: ToolCallInfo[] }>): TurnChangeFile[] {
  const seen = new Map<string, TurnChangeFile>();
  for (const item of items) {
    if (!item.calls?.length) continue;
    for (const call of item.calls) {
      const op = opFromTool(call);
      if (!op) continue;
      for (const path of pathsFromCall(call)) {
        seen.set(path, { path, op });
      }
    }
  }
  return [...seen.values()];
}

export function mergeChangeReport(
  report: TurnChangeReport | undefined,
  inferred: TurnChangeFile[]
): TurnChangeReport | undefined {
  const seen = new Map<string, TurnChangeFile>();
  if (report?.files) {
    for (const file of report.files) {
      seen.set(file.path, file);
    }
  }
  for (const file of inferred) {
    const prev = seen.get(file.path);
    if (!prev || file.op === 'deleted' || prev.op !== 'deleted') {
      seen.set(file.path, file);
    }
  }
  const files = [...seen.values()];
  if (files.length === 0) return report;

  let addedCount = 0;
  let deletedCount = 0;
  let modifiedCount = 0;
  for (const file of files) {
    if (file.op === 'added') addedCount++;
    else if (file.op === 'deleted') deletedCount++;
    else modifiedCount++;
  }
  return {
    turnId: report?.turnId || '',
    recipeVersion: report?.recipeVersion || 0,
    addedCount,
    deletedCount,
    modifiedCount,
    files,
    source: report?.source || 'live',
    computedAt: report?.computedAt || new Date().toISOString(),
  };
}

function hasChangeCounts(report: TurnChangeReport | undefined): report is TurnChangeReport {
  return !!report && (report.addedCount > 0 || report.deletedCount > 0 || report.modifiedCount > 0);
}

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
    const report = mergeChangeReport(rawReport, inferFilesFromTurnItems(turnItems as any));
    if (!hasChangeCounts(report)) return items;
  }

  const result: GroupedChatItem[] = items.slice(0, turnStarts[0]);

  for (let turnIndex = 0; turnIndex < turnStarts.length; turnIndex++) {
    const start = turnStarts[turnIndex];
    const end = turnStarts[turnIndex + 1] ?? items.length;
    const user = items[start];
    const rawReport = user.kind === 'user' ? user.changeReport : undefined;
    const turnItems = items.slice(start + 1, end) as TurnContentItem[];
    const report = mergeChangeReport(rawReport, inferFilesFromTurnItems(turnItems as any));
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
