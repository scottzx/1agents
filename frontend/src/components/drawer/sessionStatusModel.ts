import type {
    AgentTurn,
    AgentTurnStatus,
    TurnChangeOp,
    TurnChangeReport,
} from '@1agents/core/services/activityService';
import type { ChatItem, ToolCallInfo } from '@1agents/core/protocol/types';

export type SessionTurnStatus = AgentTurnStatus | 'live';

export interface SessionTurnRef {
    id: string;
    aliases: string[];
    promptText: string;
    status: SessionTurnStatus;
    createdAt: number;
    completedAt?: number;
    errorText?: string;
    changeReport?: TurnChangeReport;
    itemRange: { start: number; end: number };
}

export interface SessionSubagent {
    id: string;
    label: string;
    thinking: string;
    output: string;
    calls: ToolCallInfo[];
    streaming: boolean;
    createdAt: number;
    status: 'running' | 'completed' | 'failed';
}

export interface SessionFileEntry {
    path: string;
    name: string;
    op: TurnChangeOp;
    kind: 'code' | 'artifact';
}

export interface SessionUpload {
    path: string;
    name: string;
    isImage: boolean;
}

const ARTIFACT_NAME_RE =
    /^(implementation_plan|walkthrough|plan|report|design|architecture|summary|readme|changelog)\b/i;
const ARTIFACT_EXT_RE = /\.(md|mdx|txt|rst|adoc|pdf|html|docx)$/i;
const IMAGE_EXT_RE = /\.(png|jpe?g|gif|webp|svg|bmp|heic|heif)$/i;
const UPLOAD_LINE_RE = /^(?:\/|~\/|[A-Za-z]:[\\/]).+\.[A-Za-z0-9]{1,16}$/;

export function displayFileName(path: string): string {
    const normalized = path.replace(/\\/g, '/');
    const parts = normalized.split('/').filter(Boolean);
    return parts[parts.length - 1] || path;
}

export function displayFilePath(path: string): string {
    const normalized = path.replace(/\\/g, '/');
    const parts = normalized.split('/').filter(Boolean);
    if (parts.length <= 2) return parts.join('/') || path;
    return parts.slice(-2).join('/');
}

export function isArtifactPath(path: string): boolean {
    const name = displayFileName(path);
    return ARTIFACT_NAME_RE.test(name) || ARTIFACT_EXT_RE.test(name);
}

export function isImagePath(path: string): boolean {
    return IMAGE_EXT_RE.test(displayFileName(path));
}

export function turnOwnsId(turn: SessionTurnRef, id?: string): boolean {
    return !!id && (turn.id === id || turn.aliases.includes(id));
}

function aliasesFor(turn: AgentTurn): string[] {
    return [turn.id, turn.clientRequestId, turn.runtimeRequestId].filter((id): id is string => !!id);
}

function resolvePersistedTurn(
    user: Extract<ChatItem, { kind: 'user' }>,
    persisted: AgentTurn[],
    unused: Set<string>
): AgentTurn | undefined {
    if (user.turnId) {
        const reserved = persisted.find(
            turn =>
                turn.id === user.turnId || turn.clientRequestId === user.turnId || turn.runtimeRequestId === user.turnId
        );
        if (reserved) {
            unused.delete(reserved.id);
            return reserved;
        }
    }
    const exact = persisted.find(turn => unused.has(turn.id) && turn.promptText === user.content);
    if (exact) {
        unused.delete(exact.id);
        return exact;
    }
    return undefined;
}

function toEpoch(value?: string): number | undefined {
    if (!value) return undefined;
    const ms = Date.parse(value);
    return Number.isFinite(ms) ? ms : undefined;
}

/**
 * Build the session's turn list. User messages (non-queued) are the
 * switcher source of truth; persisted AgentTurns enrich status / reports.
 * When the live transcript is empty, fall back to persisted turns.
 */
export function collectSessionTurns(items: ChatItem[], persisted: AgentTurn[]): SessionTurnRef[] {
    const starts: number[] = [];
    for (let index = 0; index < items.length; index++) {
        const item = items[index];
        if (item.kind === 'user' && item.queueStatus !== 'queued') starts.push(index);
    }

    if (starts.length === 0) {
        return [...persisted]
            .sort((a, b) => Date.parse(a.createdAt) - Date.parse(b.createdAt) || a.id.localeCompare(b.id))
            .map(turn => ({
                id: turn.id,
                aliases: aliasesFor(turn),
                promptText: turn.promptText ?? '',
                status: turn.status,
                createdAt: toEpoch(turn.createdAt) ?? 0,
                completedAt: toEpoch(turn.completedAt),
                errorText: turn.errorText,
                changeReport: turn.changeReport,
                itemRange: { start: 0, end: 0 },
            }));
    }

    const unused = new Set(persisted.map(turn => turn.id));
    for (const start of starts) {
        const user = items[start];
        if (user.kind !== 'user' || !user.turnId) continue;
        const reserved = persisted.find(
            turn =>
                turn.id === user.turnId || turn.clientRequestId === user.turnId || turn.runtimeRequestId === user.turnId
        );
        if (reserved) unused.delete(reserved.id);
    }

    return starts.map((start, index) => {
        const end = starts[index + 1] ?? items.length;
        const user = items[start] as Extract<ChatItem, { kind: 'user' }>;
        const persistedTurn = resolvePersistedTurn(user, persisted, unused);
        const id = persistedTurn?.id || user.turnId || user.id;
        const aliases = new Set<string>(
            [id, user.id, user.turnId, user.clientRequestId].filter((alias): alias is string => !!alias)
        );
        if (persistedTurn) aliasesFor(persistedTurn).forEach(alias => aliases.add(alias));
        return {
            id,
            aliases: [...aliases].filter((alias): alias is string => !!alias),
            promptText: persistedTurn?.promptText || user.content,
            status: persistedTurn?.status || (user.turnStatus ?? 'live'),
            createdAt: persistedTurn ? toEpoch(persistedTurn.createdAt) ?? user.createdAt : user.createdAt,
            completedAt: toEpoch(persistedTurn?.completedAt),
            errorText: persistedTurn?.errorText,
            changeReport: persistedTurn?.changeReport || user.changeReport,
            itemRange: { start, end },
        };
    });
}

export function itemsForTurn(items: ChatItem[], turn: SessionTurnRef): ChatItem[] {
    if (turn.itemRange.end > turn.itemRange.start) {
        return items.slice(turn.itemRange.start, turn.itemRange.end);
    }
    return items.filter(item => turnOwnsId(turn, item.turnId));
}

function subagentStatus(card: Extract<ChatItem, { kind: 'subagent_turn' }>): SessionSubagent['status'] {
    if (card.streaming || card.calls.some(call => call.status === 'pending' || call.status === 'in_progress')) {
        return 'running';
    }
    if (card.calls.some(call => call.status === 'failed' || call.isError)) return 'failed';
    return 'completed';
}

export function collectSubagents(items: ChatItem[], turn: SessionTurnRef): SessionSubagent[] {
    return itemsForTurn(items, turn)
        .filter((item): item is Extract<ChatItem, { kind: 'subagent_turn' }> => item.kind === 'subagent_turn')
        .map(item => ({
            id: item.agentTurnId,
            label: item.label || 'subagent',
            thinking: item.thinking,
            output: item.output,
            calls: item.calls,
            streaming: item.streaming,
            createdAt: item.createdAt,
            status: subagentStatus(item),
        }));
}

function stampFile(path: string, op: TurnChangeOp): SessionFileEntry {
    return {
        path,
        name: displayFileName(path),
        op,
        kind: isArtifactPath(path) ? 'artifact' : 'code',
    };
}

function filesFromReport(report?: TurnChangeReport): SessionFileEntry[] {
    if (!report?.files.length) return [];
    const seen = new Map<string, SessionFileEntry>();
    for (const file of report.files) {
        seen.set(file.path, stampFile(file.path, file.op));
    }
    return [...seen.values()];
}

function opFromTool(call: ToolCallInfo): TurnChangeOp | null {
    const kind = (call.kind || '').toLowerCase();
    const name = call.toolName.toLowerCase();
    if (kind === 'read' || kind === 'search' || kind === 'think' || kind === 'fetch') return null;
    if (kind === 'delete' || /(delete|remove|\brm\b|unlink)/.test(name)) return 'deleted';
    if (kind === 'edit' || /(write|create_file|apply_patch|multiedit|str_replace|\bedit\b)/.test(name)) {
        return /(write|create_file)/.test(name) ? 'added' : 'modified';
    }
    if (kind === 'execute' || /(bash|shell|\brun\b|execute|command|terminal|exec)/.test(name)) return 'modified';
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
        } catch {
            // Tool input is a display string, not JSON.
        }
    }
    const seen = new Set<string>();
    const out: string[] = [];
    for (const path of paths) {
        const trimmed = path.trim();
        if (!trimmed || seen.has(trimmed) || trimmed.startsWith('http://') || trimmed.startsWith('https://')) continue;
        seen.add(trimmed);
        out.push(trimmed);
    }
    return out;
}

function filesFromLiveItems(slice: ChatItem[]): SessionFileEntry[] {
    const seen = new Map<string, SessionFileEntry>();
    const visitCalls = (calls: ToolCallInfo[]) => {
        for (const call of calls) {
            const op = opFromTool(call);
            if (!op) continue;
            for (const path of pathsFromCall(call)) {
                seen.set(path, stampFile(path, op));
            }
        }
    };
    for (const item of slice) {
        if (item.kind === 'tool_use') visitCalls(item.calls);
        if (item.kind === 'subagent_turn') visitCalls(item.calls);
    }
    return [...seen.values()];
}

export function collectTurnFiles(items: ChatItem[], turn: SessionTurnRef): SessionFileEntry[] {
    const fromReport = filesFromReport(turn.changeReport);
    if (fromReport.length > 0) return fromReport;
    return filesFromLiveItems(itemsForTurn(items, turn));
}

export function splitTurnFiles(files: SessionFileEntry[]): { code: SessionFileEntry[]; artifacts: SessionFileEntry[] } {
    return {
        code: files.filter(file => file.kind === 'code'),
        artifacts: files.filter(file => file.kind === 'artifact'),
    };
}

export function collectUploads(prompt: string): SessionUpload[] {
    const seen = new Set<string>();
    const out: SessionUpload[] = [];
    for (const raw of prompt.split('\n')) {
        const line = raw.trim();
        if (!UPLOAD_LINE_RE.test(line) && !line.startsWith('/tmp/')) continue;
        if (!/\.[A-Za-z0-9]{1,16}$/.test(line)) continue;
        if (seen.has(line)) continue;
        seen.add(line);
        out.push({
            path: line,
            name: displayFileName(line),
            isImage: isImagePath(line),
        });
    }
    return out;
}

export function promptSnippet(prompt: string, max = 72): string {
    const first = prompt
        .split('\n')
        .map(line => line.trim())
        .find(line => line && !UPLOAD_LINE_RE.test(line) && !line.startsWith('/tmp/'));
    const text = first || prompt.trim();
    if (text.length <= max) return text;
    return `${text.slice(0, max - 1)}…`;
}
