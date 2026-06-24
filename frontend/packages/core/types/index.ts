// Platform-agnostic shared data shapes + re-export hub.
//
// Holds the wire shapes returned by the backend file/workspace/terminal APIs
// (moved verbatim out of components/types.ts, Phase 0 carve) and re-exports the
// protocol session/permission types so consumers have a single core entry point.
// components/types.ts re-exports everything here, keeping existing importers
// unchanged.

export * from '../protocol/session';
export * from '../protocol/permission';

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

/** Mirrors the backend Workspace struct stored in workspaces_dir.json */
export interface Workspace {
    id: string;
    name: string;
    path: string;
    status: string;
    terminalDir?: string;
    chatChannel?: string;
    defaultAgent?: import('../protocol/session').AgentType;
    builtin?: boolean;
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
