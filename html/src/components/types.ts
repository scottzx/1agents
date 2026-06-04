import type { Lang } from './i18n';

/** A terminal session — mirrors a tmux window, belongs to a workspace. */
export interface Session {
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
    active: boolean;
    workspaceId: string;
    cwd: string;
    status?: string;
    waitingFor?: string;
    agent?: string;
}

export type RightDrawerTab = 'files' | 'git' | 'channels' | 'providers' | 'settings' | 'discovery' | 'skills' | 'none';

export function isFullPageTab(tab: RightDrawerTab): boolean {
    return tab === 'providers' || tab === 'discovery' || tab === 'skills' || tab === 'settings';
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
