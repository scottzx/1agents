// Platform-agnostic shared data shapes + re-export hub.
//
// Holds the wire shapes returned by the backend file/workspace/terminal APIs
// (moved verbatim out of components/types.ts, Phase 0 carve) and re-exports the
// protocol session/permission types so consumers have a single core entry point.
// components/types.ts re-exports everything here, keeping existing importers
// unchanged.

export * from '../protocol/session';
export * from '../protocol/permission';
export * from './task';

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

/** Mirrors the backend Workspace struct stored in meta.db projects table. */
export interface Workspace {
    id: string;
    name: string;
    path: string;
    status: string;
    terminalDir?: string;
    chatChannel?: string;
    defaultAgent?: import('../protocol/session').AgentType;
    builtin?: boolean;
    /**
     * "workforce" | "project" — splits the sidebar into 助理 vs 项目 lists
     * (Epic #184 / #190). Code kind is workforce; Chinese UI still says「助理」.
     * Empty value from a legacy row is treated as "project".
     */
    kind?: 'workforce' | 'project';
    /** Avatar URL served by GET /avatars/ (preset or uploaded image). */
    avatar?: string;
    /**
     * 所属远程设备 id(多设备项目视图,#114)。client-only:由 workspaceService.list(deviceId)
     * 在拉取远程设备项目时打标;本机项目此字段为空。点击带 deviceId 的项目时切到代理路由。
     */
    deviceId?: string;
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
