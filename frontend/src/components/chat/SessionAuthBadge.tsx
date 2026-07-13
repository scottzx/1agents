// ChatHeader auth badge — three states driven by the bridge's `authState`:
//
//   - authenticated: badge hidden entirely (acceptance #6 — agents that
//     never require auth add zero visual noise)
//   - auth_required: red dot + "重新认证" button → opens ReauthModal
//   - logged_out:   gray dot + "登录" button → opens ReauthModal
//
// The badge subscribes to the bridge through the chat store mirror
// (`liveSessionAuthState`) so it stays correct across tab switches and
// backgrounded sessions — no active `useBridge` listener required.

import { h } from 'preact';
import { useSignal } from '@preact/signals';
import type { AuthState } from '@1agents/core/protocol/types';
import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import { liveSessionAuthState } from '../../stores/sessionStore';
import { openAuthRequiredModal } from '../../stores/modalStore';

export interface SessionAuthBadgeProps {
    sessionId: string;
    /** Called by the user-menu logout entry on the right of the header. */
    onLogout?: () => void;
}

export function SessionAuthBadge({ sessionId, onLogout }: SessionAuthBadgeProps) {
    // Read the per-session mirror. liveSessionAuthState is the source of
    // truth across the whole header — reading it inside the component
    // subscribes this render so the badge repaints on bridge events.
    const authMap = liveSessionAuthState.value;
    const auth: AuthState | null = authMap[sessionId] ?? null;
    // Local "menu open" toggle for the user avatar dropdown. lives on a
    // signal (not useState) per the codebase convention — useState silently
    // fails to repaint under @preact/signals v2.
    const menuOpen = useSignal(false);
    const language = ui.language.value;

    if (!auth) return null;

    const handleAction = () => {
        openAuthRequiredModal(sessionId, auth.methods, auth.message);
    };

    // Hide the badge (and its button) entirely when the session is healthy.
    // acceptance #1 says "no badge or green checkmark"; we go with no badge
    // to honor #6 ("agents that don't require auth add zero visual noise").
    if (auth.status === 'authenticated') return null;

    const isAuthRequired = auth.status === 'auth_required';
    const labelKey = isAuthRequired ? 'chat.auth.status.auth_required' : 'chat.auth.status.logged_out';
    const buttonKey = isAuthRequired ? 'chat.auth.reauth' : 'chat.auth.login';
    const dotClass = isAuthRequired ? 'chat-auth-badge__dot--required' : 'chat-auth-badge__dot--logged-out';

    return (
        <div class="chat-auth-badge" data-status={auth.status}>
            <span class={`chat-auth-badge__dot ${dotClass}`} aria-hidden="true" />
            <span class="chat-auth-badge__label">{t(labelKey, language)}</span>
            <button type="button" class="chat-auth-badge__action" onClick={handleAction}>
                {t(buttonKey, language)}
            </button>
            {/* User avatar / menu — kept next to the badge so the logout
                entry lives where the spec asked for it (in the avatar menu)
                even though there is no global account menu elsewhere in the
                app yet. Click-outside closes it; toggle on the button itself. */}
            <div class="chat-header-user-menu">
                <button
                    type="button"
                    class="chat-header-user-menu__trigger"
                    aria-haspopup="menu"
                    aria-expanded={menuOpen.value}
                    title={t('chat.auth.logout', language)}
                    onClick={e => {
                        e.stopPropagation();
                        menuOpen.value = !menuOpen.value;
                    }}
                >
                    <span aria-hidden="true">●</span>
                </button>
                {menuOpen.value && (
                    <div class="chat-header-user-menu__panel" role="menu" onClick={e => e.stopPropagation()}>
                        <button
                            type="button"
                            class="chat-header-user-menu__item"
                            role="menuitem"
                            onClick={() => {
                                menuOpen.value = false;
                                onLogout?.();
                            }}
                            disabled={!onLogout}
                        >
                            {t('chat.auth.logout', language)}
                        </button>
                    </div>
                )}
            </div>
        </div>
    );
}
