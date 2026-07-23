import type { Workspace } from '../components/types';

/**
 * Frontend picker sentinel for「单次对话」. Submitting this value creates a
 * real kind=tmp workspace (id tmp-<sessionId>) — not a permanent projects row
 * named "oneshot".
 */
export const ONESHOT_WORKSPACE_ID = 'oneshot';

/** True for picker sentinel or minted tmp-/oneshot- workspace ids. */
export function isOneshotWorkspaceId(id: string | undefined | null): boolean {
    if (!id) return false;
    return id === ONESHOT_WORKSPACE_ID || id.startsWith('tmp-') || id.startsWith('oneshot-');
}

/** True when workspace kind is tmp (preferred over id prefix when kind is known). */
export function isTmpWorkspace(ws: Pick<Workspace, 'kind' | 'id'> | undefined | null): boolean {
    if (!ws) return false;
    if (ws.kind === 'tmp') return true;
    return isOneshotWorkspaceId(ws.id);
}

/** Chat session on a tmp / legacy oneshot workspace. */
export function isOneshotChatSession(session: { kind?: string; workspaceId?: string } | null | undefined): boolean {
    return Boolean(session && session.kind === 'chat' && isOneshotWorkspaceId(session.workspaceId));
}

/**
 * Side-pane workspace id: tmp sessions use their real workspace id (tmp-…).
 * Only the bare picker sentinel has no project row.
 */
export function paneWorkspaceIdFor(
    session: { kind?: string; workspaceId?: string } | null | undefined,
    activeWorkspaceId: string
): string {
    if (isOneshotChatSession(session) && session?.workspaceId && session.workspaceId !== ONESHOT_WORKSPACE_ID) {
        return session.workspaceId;
    }
    if (activeWorkspaceId === ONESHOT_WORKSPACE_ID) {
        return session?.workspaceId && session.workspaceId !== ONESHOT_WORKSPACE_ID
            ? session.workspaceId
            : activeWorkspaceId;
    }
    return activeWorkspaceId;
}

export function paneWorkspacePathFor(
    session: { kind?: string; workspaceId?: string; cwd?: string } | null | undefined,
    realWorkspacePath: string
): string {
    if (isOneshotChatSession(session)) {
        const cwd = (session?.cwd || '').trim();
        if (cwd) return cwd;
    }
    return realWorkspacePath || '.';
}
