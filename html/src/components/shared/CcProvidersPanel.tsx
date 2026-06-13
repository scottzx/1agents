import { h } from 'preact';
import { extractCcToken, extractCcRedirect } from '../../modules/cc-token';
import * as ui from '../../stores/uiStore';

interface CcProvidersPanelProps {
    /** Non-empty providers URL; callers guard the loading state themselves. */
    ccProvidersUrl: string;
    /** Platform-specific sizing; passed through verbatim. */
    panelStyle: string | Record<string, string | number>;
}

/**
 * Shared <cc-connect-panel> mount used by both DesktopAppLayout (full-page
 * providers drawer tab) and MobileAppLayout (providers bottom tab). The
 * route/auth-token extraction from the providers URL is identical on both
 * platforms; only the container styling diverges and is passed in.
 */
export function CcProvidersPanel({ ccProvidersUrl, panelStyle }: CcProvidersPanelProps) {
    return (
        <cc-connect-panel
            id="cc-providers-panel"
            route={extractCcRedirect(ccProvidersUrl, '/providers')}
            theme={ui.theme.value}
            lang={ui.language.value}
            auth-token={extractCcToken(ccProvidersUrl)}
            style={panelStyle}
        />
    );
}
