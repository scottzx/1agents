import type { Lang } from './i18n';
import type { ChatSession } from '@1agents/core/types';

// Shared wire shapes (AgentType, ChatSession, ChatStatus, SessionRole,
// PermissionMode/Decision, FsEntry, Workspace, TmuxWindow, …) moved to the
// platform-agnostic core (Phase 0 carve). Re-exported here so existing
// `../types` importers stay unchanged.
export * from '@1agents/core/types';

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

export type RightDrawerTab =
    | 'files'
    | 'browser'
    | 'git'
    | 'channels'
    | 'providers'
    | 'settings'
    | 'discovery'
    | 'skills'
    | 'tasks'
    | 'terminal'
    | 'reminders'
    | 'assistants'
    | 'contacts'
    | 'datasources'
    | 'inbox'
    | 'aggregate'
    | 'none';

export function isFullPageTab(tab: RightDrawerTab): boolean {
    return (
        tab === 'providers' ||
        tab === 'discovery' ||
        tab === 'skills' ||
        tab === 'settings' ||
        tab === 'reminders' ||
        tab === 'assistants' ||
        tab === 'contacts' ||
        tab === 'datasources' ||
        tab === 'inbox' ||
        tab === 'aggregate'
    );
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
