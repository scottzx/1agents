import { h } from 'preact';
import { SystemSettings } from '../settings/SystemSettings';
import type { SettingsCategory } from '../../modules/settings-manifest';
import type { App, AppState } from '../app';
import * as ui from '../../stores/uiStore';
import * as sess from '../../stores/sessionStore';

interface SystemSettingsHostProps {
    app: App;
    state: AppState;
    /** Desktop: App-level settings category; mobile: local menu selection. */
    activeCategory: SettingsCategory;
}

/**
 * Shared SystemSettings wiring used by both DesktopAppLayout and
 * MobileAppLayout. Theme/language toggles, tmux mouse and access-token
 * plumbing are identical on both platforms; only the active category
 * source diverges and is passed in.
 */
export function SystemSettingsHost({ app, state, activeCategory }: SystemSettingsHostProps) {
    return (
        <SystemSettings
            theme={ui.theme.value}
            toggleTheme={ui.toggleTheme}
            language={ui.language.value}
            toggleLanguage={ui.toggleLanguage}
            tmuxMouseOn={sess.tmuxMouseOn.value}
            onTmuxMouseToggle={sess.toggleTmuxMouse}
            accessTokenExists={state.accessAuthRequired}
            onGenerateAccessToken={app.generateAccessToken}
            onRevokeAccessToken={app.revokeAccessToken}
            activeCategory={activeCategory}
        />
    );
}
