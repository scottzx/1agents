export interface WorkspaceFolderChild {
    id: string;
    title: string;
    time: string;
    active?: boolean;
}

export interface WorkspaceFolder {
    id: string;
    name: string;
    expanded: boolean;
    children: WorkspaceFolderChild[];
}

/** Mirrors the backend Workspace struct stored in workspaces_dir.json */
export interface Workspace {
    id: string;
    name: string;
    path: string;
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

export type RightDrawerTab = 'files' | 'git' | 'settings' | 'none';

export function getFileTag(name: string): 'doc' | 'img' | 'code' | 'other' {
    const ext = name.includes('.') ? name.split('.').pop()!.toLowerCase() : '';
    const docs = ['md', 'txt', 'pdf', 'doc', 'docx', 'xls', 'xlsx', 'ppt', 'pptx', 'csv'];
    const imgs = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'ico', 'bmp'];
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
    return 'other';
}

/** Format a byte count as a human-readable string (e.g. 12.3 KB) */
export function formatBytes(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
