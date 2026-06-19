// Platform-agnostic permission protocol types.
//
// Moved verbatim out of components/types.ts (Phase 0 — core carve).
// components/types.ts re-exports these so existing importers stay unchanged.

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
