import type { Lang } from './i18n';

/**
 * Agent plugin names registered in cc-connect. Keep in sync with
 * backend/internal/agent/types.go SupportedAgentTypes.
 */
export type AgentType =
    | 'claudecode'
    | 'codex'
    | 'acp'
    | 'gemini'
    | 'cursor'
    | 'devin'
    | 'iflow'
    | 'kimi'
    | 'opencode'
    | 'pi'
    | 'qoder'
    | 'tmux';

export const AGENT_TYPES: AgentType[] = [
    'claudecode',
    'codex',
    'acp',
    'gemini',
    'cursor',
    'devin',
    'iflow',
    'kimi',
    'opencode',
    'pi',
    'qoder',
    'tmux',
];

/** Human-readable labels for the agent-type picker. */
export const AGENT_TYPE_LABELS: Record<AgentType, string> = {
    claudecode: 'Claude Code',
    codex: 'Codex',
    acp: 'ACP (通用)',
    gemini: 'Gemini CLI',
    cursor: 'Cursor',
    devin: 'Devin',
    iflow: 'iFlow',
    kimi: 'Kimi',
    opencode: 'OpenCode',
    pi: 'Pi',
    qoder: 'Qoder',
    tmux: 'Tmux',
};

/** A terminal session — mirrors a tmux window, belongs to a workspace. */
export interface TerminalSession {
    kind: 'terminal';
    id: string;
    workspaceId: string;
    index: number;
    name: string;
    active: boolean;
    cwd?: string;
    status?: string;
    waitingFor?: string;
    agent?: string;
}

export type ChatStatus = 'idle' | 'streaming' | 'awaiting_permission' | 'error';

/**
 * Per-session permission policy mirrored from the backend
 * ChatSessionRecord.permission_mode and from the bridge-server's
 * activeSessions[sessionId].permissionMode. Empty string / undefined
 * means "use the bridge-server runtime default".
 */
export type PermissionMode = 'approve-reads' | 'approve-all' | 'deny-all';

export const PERMISSION_MODES: PermissionMode[] = ['approve-reads', 'approve-all', 'deny-all'];

/** Cycle order for the Composer toggle button. */
export function nextPermissionMode(mode: PermissionMode): PermissionMode {
    const idx = PERMISSION_MODES.indexOf(mode);
    return PERMISSION_MODES[(idx + 1) % PERMISSION_MODES.length];
}

/**
 * The full ACP permission decision set the user can pick from a
 * permission bubble. Mirrors `AcpPermissionDecision.outcome` in
 * modules/1acp/src/types.ts. `cancel` collapses the bubble without
 * picking a side (used by close affordances, currently unused by UI).
 */
export type PermissionDecision = 'allow_once' | 'allow_always' | 'reject_once' | 'reject_always' | 'cancel';

/**
 * A chat session — backed by a cc-connect session. The actual
 * conversation lives in cc-connect; this is the 1agents-side index.
 *
 * Wire shape: mirrors backend/internal/agent.ChatSessionRecord.
 */
export interface ChatSession {
    kind: 'chat';
    id: string; // 1agents uuid
    workspaceId: string;
    taskId?: string; // New: task ID this session belongs to
    name: string;
    agentType: AgentType;
    ccProject: string; // cc-connect project name
    ccSessionId: string; // cc-connect session id
    /**
     * ACP-side session id — the agent-managed identifier (e.g. Claude
     * Code's JSONL UUID). Populated by the bridge-server on first
     * session_ready and reused as resumeSessionId on subsequent opens.
     * Independent of ccSessionId, which is for the cc-connect / IM path.
     */
    acpSessionId?: string;
    sessionKey: string; // cc-connect bridge session_key
    status: ChatStatus;
    lastEventAt?: string; // ISO timestamp
    active: boolean;
    /** Per-session permission policy. Persisted via PATCH /api/agent/sessions/{id}. */
    permissionMode?: PermissionMode;
}

export type Session = TerminalSession | ChatSession;

export function isChat(s: Session): s is ChatSession {
    return s.kind === 'chat';
}

export function isTerminal(s: Session): s is TerminalSession {
    return s.kind === 'terminal';
}

export interface WorkspaceFolder {
    id: string;
    name: string;
    expanded: boolean;
    sessions: Session[];
}

/** Mirrors the backend Workspace struct stored in workspaces_dir.json */
export interface Workspace {
    id: string;
    name: string;
    path: string;
    status: string;
    terminalDir?: string;
    chatChannel?: string;
    defaultAgent?: AgentType;
}

export type WorkspaceStatus = 'active' | 'inactive' | 'planning' | 'archived';

export const WORKSPACE_STATUS_KEYS: { value: WorkspaceStatus; labelKey: string }[] = [
    { value: 'active', labelKey: 'workspace.status.active' },
    { value: 'inactive', labelKey: 'workspace.status.inactive' },
    { value: 'planning', labelKey: 'workspace.status.planning' },
    { value: 'archived', labelKey: 'workspace.status.archived' },
];

export function getStatusLabel(status: string, language: Lang, t: (key: string, lang: Lang) => string): string {
    const found = WORKSPACE_STATUS_KEYS.find(s => s.value === status);
    return found ? t(found.labelKey, language) : status;
}

/** A single file or directory entry returned by /api/fs/list */
export interface FsEntry {
    name: string;
    path: string; // relative to workdir root
    isDir: boolean;
    size: number;
    modTime: number;
    // client-only: children loaded on expand
    children?: FsEntry[];
    expanded?: boolean;
}

/** A tmux window returned by GET /api/terminal/list — unified Session model */
export interface TmuxWindow {
    index: number;
    name: string;
    customName?: string;
    active: boolean;
    workspaceId: string;
    cwd: string;
    status?: string;
    waitingFor?: string;
    agent?: string;
}

export type RightDrawerTab =
    | 'files'
    | 'git'
    | 'channels'
    | 'providers'
    | 'settings'
    | 'discovery'
    | 'skills'
    | 'tasks'
    | 'none';

export function isFullPageTab(tab: RightDrawerTab): boolean {
    return tab === 'providers' || tab === 'discovery' || tab === 'skills' || tab === 'settings';
}

/**
 * Module-backed drawer tab state. Sits next to `RightDrawerTab` (which we
 * keep untouched for migration safety). Modules contribute their own sub-path
 * that the host mirrors in the main app's URL.
 */
export interface RightDrawerState {
    tab: RightDrawerTab;
    /** Sub-path inside an active module, e.g. "/skills/use". Empty for non-module tabs. */
    modulePath: string;
}

// NOTE: Keep in sync with getFileTagFromExt in agent/internal/fs/handler.go
export function getFileTag(name: string): 'doc' | 'img' | 'code' | 'video' | 'audio' | 'other' {
    const ext = name.includes('.') ? name.split('.').pop()!.toLowerCase() : '';
    const docs = ['md', 'txt', 'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'csv'];
    const imgs = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp'];
    const videos = ['mp4', 'webm', 'ogg', 'mov', 'm4v', '3gp'];
    const audios = ['mp3', 'wav', 'm4a', 'flac', 'aac', 'ogg', 'oga'];
    const code = [
        'js',
        'jsx',
        'ts',
        'tsx',
        'html',
        'css',
        'scss',
        'json',
        'go',
        'py',
        'rs',
        'cpp',
        'c',
        'h',
        'sh',
        'yaml',
        'yml',
        'toml',
        'xml',
    ];
    if (docs.includes(ext)) return 'doc';
    if (imgs.includes(ext)) return 'img';
    if (code.includes(ext)) return 'code';
    if (videos.includes(ext)) return 'video';
    if (audios.includes(ext)) return 'audio';
    return 'other';
}

/** Format a byte count as a human-readable string (e.g. 12.3 KB) */
export function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
