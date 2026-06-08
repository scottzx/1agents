/**
 * postMessage contract between the host and module iframes.
 *
 * Two directions:
 *   host  →  iframe:  THEME_CHANGE, LANG_CHANGE, NAVIGATE
 *   iframe →  host:   READY (mount completed), NAV_CHANGE (route changed)
 *
 * The host is the single writer of THEME/LANG — modules never echo them
 * back. The iframe is the single writer of NAV_CHANGE — the host mirrors
 * it into the URL but does not push a NAVIGATE for its own clicks (those
 * update host state first, then send a NAVIGATE so the iframe's router
 * follows).
 */

export type ModuleMessage = ThemeChangeMessage | LangChangeMessage | NavigateMessage;

export type ModuleInboundMessage = ReadyMessage | NavChangeMessage;

export interface ThemeChangeMessage {
    type: 'THEME_CHANGE';
    theme: 'light' | 'dark';
}

export interface LangChangeMessage {
    type: 'LANG_CHANGE';
    /** BCP-47 — matches the host's `Lang` type. */
    lang: string;
}

export interface NavigateMessage {
    type: 'NAVIGATE';
    /** Target route inside the module, e.g. "/skills/use". */
    to: string;
}

export interface ReadyMessage {
    type: 'READY';
    /** Manifest version the iframe expects (currently always 1). */
    manifestVersion: 1;
}

export interface NavChangeMessage {
    type: 'NAV_CHANGE';
    /** Path the iframe is currently displaying, e.g. "/skills/use". */
    path: string;
}

/**
 * Posts a typed message to a module iframe. The `*` target origin is
 * acceptable here because messages are scoped to known iframe ids and
 * payload data is not security-sensitive; cross-frame validation is the
 * receiver's job.
 */
export function postToModule(iframe: HTMLIFrameElement | null, msg: ModuleMessage): void {
    if (!iframe || !iframe.contentWindow) return;
    try {
        iframe.contentWindow.postMessage(msg, '*');
    } catch (e) {
        console.error('[module] postMessage failed', e);
    }
}

/** Type guard for messages sent by modules to the host. */
export function isModuleInboundMessage(data: unknown): data is ModuleInboundMessage {
    if (typeof data !== 'object' || data === null) return false;
    const msg = data as Record<string, unknown>;
    if (msg.type === 'READY' && msg.manifestVersion === 1) return true;
    if (msg.type === 'NAV_CHANGE' && typeof msg.path === 'string') return true;
    return false;
}
