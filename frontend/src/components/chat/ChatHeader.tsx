// Per-chat header rendered above the message list. Today the only thing it
// owns is the auth badge + logout entry, but this is the natural home for
// future per-session chrome (connection indicator, model picker, etc.) —
// kept as its own component so adding more pieces doesn't grow ChatPanel.

import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import type { ChatSession } from '../types';
import { SessionAuthBadge } from './SessionAuthBadge';
import { useBridge } from './hooks';
import { openAuthRequiredModal, closeAuthRequiredModal } from '../../stores/modalStore';

export interface ChatHeaderProps {
    session: ChatSession;
}

/**
 * Functional component so it can call `useBridge`. Rendered above
 * MessageList by ChatPanel — see ChatPanel for layout context.
 */
export function ChatHeader({ session }: ChatHeaderProps) {
    const { authState, logout } = useBridge(session);

    // Auto-open the ReauthModal when the bridge pushes `auth_required` so
    // the user lands on the form the moment the agent demands credentials
    // (acceptance #2). Re-derived from `authState.status` rather than the
    // event itself so re-renders after the modal closes (e.g. user dismissed
    // the modal but the bridge is still auth_required) don't re-pop it.
    useEffect(() => {
        if (authState?.status === 'auth_required' && authState.methods.length > 0) {
            openAuthRequiredModal(session.id, authState.methods, authState.message);
        } else if (authState?.status === 'authenticated' || authState?.status === 'logged_out') {
            // The user successfully authenticated or explicitly closed the
            // modal — make sure no stale modal is still up.
            closeAuthRequiredModal();
        }
    }, [authState?.status, authState?.methods, authState?.message, session.id]);

    return (
        <div class="chat-header">
            <SessionAuthBadge sessionId={session.id} onLogout={logout} />
            {/* Spacer keeps the header row tall enough to be tappable on
                touch devices; the badge itself is right-aligned via flex
                so an absolute-positioned dropdown menu has somewhere to anchor. */}
            <span class="chat-header-spacer" aria-hidden="true" />
        </div>
    );
}
