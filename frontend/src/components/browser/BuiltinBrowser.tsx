import { h, Component } from 'preact';
import { t, type Lang } from '../../i18n';
import type { Tab } from '../../stores/tabsStore';

/** base64url (no padding) for ASCII origins — matches Go RawURLEncoding. */
function b64urlEncode(str: string): string {
    return btoa(str).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/g, '');
}

function b64urlDecode(s: string): string {
    let b64 = s.replace(/-/g, '+').replace(/_/g, '/');
    while (b64.length % 4) b64 += '=';
    return atob(b64);
}

/**
 * /api/webproxy/{b64origin}/TalkingHeadComposition
 * last path segment = Remotion composition id.
 */
function toWebProxyIframeSrc(targetUrl: string): string {
    const u = new URL(targetUrl);
    const origin = u.origin;
    const path = u.pathname || '/';
    return `${window.location.origin}/api/webproxy/${b64urlEncode(origin)}${path}${u.search}${u.hash}`;
}

function decodeWebProxyPath(url: URL): string | null {
    const prefix = '/api/webproxy/';
    if (!url.pathname.startsWith(prefix)) return null;
    const rest = url.pathname.slice(prefix.length);
    if (!rest) return null;
    const slash = rest.indexOf('/');
    const b64 = slash < 0 ? rest : rest.slice(0, slash);
    const path = slash < 0 ? '/' : rest.slice(slash) || '/';
    try {
        return `${b64urlDecode(b64)}${path}${url.search}${url.hash}`;
    } catch {
        return null;
    }
}

export interface BuiltinBrowserProps {
    tab: Tab;
    active: boolean;
    onUrlChange: (tabId: string, url: string) => void;
    language: Lang;
}

export interface BuiltinBrowserState {
    iframeSrc: string;
    /** Local draft for the address bar — must not bind live typing to tab.url. */
    addressBar: string;
}

export class BuiltinBrowser extends Component<BuiltinBrowserProps, BuiltinBrowserState> {
    private inputRef: HTMLInputElement | null = null;
    private iframeRef: HTMLIFrameElement | null = null;
    private lastLoadedUrl: string = '';

    state: BuiltinBrowserState = {
        iframeSrc: this.getIframeUrl(this.props.tab.url || ''),
        addressBar: !this.props.tab.url || this.props.tab.url === 'about:blank' ? '' : this.props.tab.url,
    };

    componentDidMount() {
        window.addEventListener('message', this.handleIframeMessage);
    }

    componentWillUnmount() {
        window.removeEventListener('message', this.handleIframeMessage);
    }

    componentWillReceiveProps(nextProps: BuiltinBrowserProps) {
        // Inactive pane: drop iframe document to free memory (Remotion / SPAs).
        if (!nextProps.active && this.props.active) {
            this.setState({ iframeSrc: 'about:blank' });
            return;
        }
        if (nextProps.active && !this.props.active) {
            const nextUrl = nextProps.tab.url || '';
            this.lastLoadedUrl = '';
            this.setState({
                iframeSrc: this.getIframeUrl(nextUrl),
                addressBar: !nextUrl || nextUrl === 'about:blank' ? '' : nextUrl,
            });
            return;
        }
        // Tab id change = different workspace session.
        if (nextProps.tab.id !== this.props.tab.id) {
            const nextUrl = nextProps.tab.url || '';
            this.lastLoadedUrl = '';
            this.setState({
                iframeSrc: nextProps.active ? this.getIframeUrl(nextUrl) : 'about:blank',
                addressBar: !nextUrl || nextUrl === 'about:blank' ? '' : nextUrl,
            });
            return;
        }
        if (nextProps.tab.url !== this.props.tab.url) {
            const nextUrl = nextProps.tab.url || '';
            const display = !nextUrl || nextUrl === 'about:blank' ? '' : nextUrl;
            // Committed navigation only — never fight the user while typing.
            if (nextUrl !== this.lastLoadedUrl) {
                this.setState({
                    iframeSrc: nextProps.active ? this.getIframeUrl(nextUrl) : 'about:blank',
                    addressBar: display,
                });
            } else {
                // URL settled to what we just loaded; keep bar in sync (normalized http:// etc.)
                this.setState({ addressBar: display });
            }
        }
    }

    handleIframeMessage = (e: MessageEvent) => {
        if (this.iframeRef && e.source === this.iframeRef.contentWindow) {
            // Reject cross-origin messages so a misbehaving page can't poison the URL bar
            if (e.origin !== window.location.origin) return;
            const data = e.data;
            if (data && data.type === 'iframe_navigate' && typeof data.url === 'string') {
                // Strip /api/proxy?url= wrapper — mirrors handleIframeLoad's extraction
                const newUrl = this.getOriginalUrl(data.url);
                if (newUrl && newUrl !== this.props.tab.url) {
                    this.lastLoadedUrl = newUrl;
                    this.props.onUrlChange(this.props.tab.id, newUrl);
                }
            }
        }
    };

    getOriginalUrl = (urlStr: string): string => {
        try {
            const url = new URL(urlStr, window.location.origin);
            // Legacy query proxy
            if (url.pathname === '/api/proxy') {
                const target = url.searchParams.get('url');
                if (target) return target;
            }
            // Path proxy: /api/webproxy/{b64origin}/{path...}
            const decoded = decodeWebProxyPath(url);
            if (decoded) return decoded;
            return urlStr;
        } catch (e) {
            return urlStr;
        }
    };

    handleIframeLoad = () => {
        if (!this.iframeRef || !this.iframeRef.contentWindow) return;
        try {
            const iframeUrl = this.iframeRef.contentWindow.location.href;
            if (!iframeUrl || iframeUrl === 'about:blank') return;
            // After inject, iframe replaceState's to a clean path ("/TalkingHeadComposition").
            // That is NOT the target site URL — trust postMessage / proxy wrappers only.
            try {
                const u = new URL(iframeUrl);
                if (u.origin === window.location.origin) {
                    const isProxy = u.pathname === '/api/proxy' || u.pathname.startsWith('/api/webproxy/');
                    if (!isProxy) return;
                }
            } catch {
                /* fall through */
            }
            const targetUrl = this.getOriginalUrl(iframeUrl);
            if (targetUrl && targetUrl !== this.props.tab.url) {
                this.lastLoadedUrl = targetUrl;
                this.props.onUrlChange(this.props.tab.id, targetUrl);
            }
        } catch (e) {
            // Cross-origin reads can still fail for non-HTML / edge proxy responses
        }
    };

    private invokeTauri = async (command: string, args: Record<string, unknown> = {}): Promise<unknown> => {
        const tauri = (
            window as unknown as {
                __TAURI__?: { core: { invoke: (cmd: string, args?: Record<string, unknown>) => Promise<unknown> } };
            }
        ).__TAURI__;
        if (tauri) {
            try {
                return await tauri.core.invoke(command, args);
            } catch (e) {
                console.error(`Failed to invoke Tauri command ${command}:`, e);
            }
        }
        return null;
    };

    /**
     * Always load through the host Go path-proxy so URL semantics match the
     * 1agents host — not the browser client (LAN phone / Happy Relay).
     *
     * Shape: /api/webproxy/{base64url(origin)}/TalkingHeadComposition
     *
     * Remotion Studio takes composition id from the last path segment; this
     * keeps "TalkingHeadComposition" as that segment (unlike ?url=… where
     * pathname is always "/api/proxy" → error "Composition with ID api/proxy").
     * Inject also replaceState's to a clean "/TalkingHeadComposition" path.
     */
    getIframeUrl(urlStr: string): string {
        if (!urlStr || urlStr === 'about:blank') {
            return 'about:blank';
        }
        // Already a proxied URL — don't double-wrap
        if (
            urlStr.startsWith(`${window.location.origin}/api/proxy?url=`) ||
            urlStr.startsWith(`${window.location.origin}/api/webproxy/`) ||
            urlStr.startsWith('/api/webproxy/') ||
            urlStr.startsWith('/api/proxy?url=')
        ) {
            if (urlStr.startsWith('/')) {
                return `${window.location.origin}${urlStr}`;
            }
            return urlStr;
        }
        // Unwrap if tab.url somehow holds a proxy wrapper
        try {
            const u = new URL(urlStr, window.location.origin);
            if (u.pathname === '/api/proxy') {
                const target = u.searchParams.get('url');
                if (target) return toWebProxyIframeSrc(target);
            }
            const decoded = decodeWebProxyPath(u);
            if (decoded) return toWebProxyIframeSrc(decoded);
        } catch {
            /* fall through */
        }
        return toWebProxyIframeSrc(urlStr);
    }

    handleAddressInput = (e: Event) => {
        const value = (e.target as HTMLInputElement).value;
        this.setState({ addressBar: value });
    };

    handleKeyPress = (e: KeyboardEvent) => {
        if (e.key === 'Enter') {
            let url = this.state.addressBar.trim();
            if (url) {
                if (!/^https?:\/\//i.test(url) && !url.startsWith('about:')) {
                    url = 'http://' + url;
                }
                // Allow componentWillReceiveProps to refresh iframeSrc for this commit.
                this.lastLoadedUrl = '';
                this.props.onUrlChange(this.props.tab.id, url);
            }
        }
    };

    handleRefresh = () => {
        // Always re-set iframeSrc (webproxy URL). Do NOT location.reload() the
        // iframe document — after SPA history changes the real path may no longer
        // be a proxy path and reload would hit 1agents 404 (e.g. /TalkingHeadComposition).
        const src = this.getIframeUrl(this.props.tab.url || '');
        if (this.iframeRef) {
            // Cache-bust so the browser actually re-fetches
            const sep = src.includes('?') ? '&' : '?';
            this.iframeRef.src = src === 'about:blank' ? src : `${src}${sep}_r=${Date.now()}`;
            this.setState({ iframeSrc: src });
        }
    };

    handleOpenExternal = () => {
        const { tab } = this.props;
        if (!tab.url || tab.url === 'about:blank') return;

        const isDesktopEnv =
            IS_DESKTOP || (typeof window !== 'undefined' && !!(window as unknown as { __TAURI__?: object }).__TAURI__);
        if (isDesktopEnv) {
            this.invokeTauri('open_in_external_browser', { url: tab.url });
        } else {
            window.open(tab.url, '_blank');
        }
    };

    render() {
        const { tab, active } = this.props;
        const { language } = this.props;
        const isHome = !tab.url || tab.url === 'about:blank';

        return (
            <div
                class="builtin-browser"
                style={{ display: active ? 'flex' : 'none', height: '100%', flexDirection: 'column' }}
            >
                <div class="browser-nav-bar">
                    <button
                        class="browser-refresh-btn"
                        onClick={this.handleRefresh}
                        title={t('app.browser.refresh', this.props.language)}
                        disabled={isHome}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.72 2.78L21 8" />
                            <polyline points="21 3 21 8 16 8" />
                        </svg>
                    </button>
                    <input
                        type="text"
                        class="browser-url-input"
                        placeholder={t('app.browser.placeholder', this.props.language)}
                        value={this.state.addressBar}
                        ref={el => {
                            this.inputRef = el;
                        }}
                        onInput={this.handleAddressInput}
                        onKeyDown={this.handleKeyPress}
                    />
                    <button
                        class="browser-open-external-btn"
                        onClick={this.handleOpenExternal}
                        title={t('app.browser.openExternal', this.props.language)}
                        disabled={isHome}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6" />
                            <polyline points="15 3 21 3 21 9" />
                            <line x1="10" y1="14" x2="21" y2="3" />
                        </svg>
                    </button>
                </div>
                <div class="browser-iframe-wrapper" style="flex: 1; position: relative; width: 100%; height: 100%;">
                    {isHome && (
                        <div
                            class="browser-welcome-page"
                            style="position: absolute; top: 0; left: 0; width: 100%; height: 100%; z-index: 1;"
                        >
                            <div class="welcome-card">
                                <svg
                                    class="welcome-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="1.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="10" />
                                    <line x1="2" y1="12" x2="22" y2="12" />
                                    <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                                </svg>
                                <h3 class="welcome-title">{t('app.browser.title', language)}</h3>
                                <p class="welcome-desc">{t('app.browser.welcomeDesc', language)}</p>
                                <div class="welcome-tips">
                                    <div class="tip-item">
                                        <strong>{t('app.browser.tipProxyLabel', language)}</strong>
                                        <span>{t('app.browser.tipProxyDesc', language)}</span>
                                    </div>
                                    <div class="tip-item">
                                        <strong>{t('app.browser.tipExternalLabel', language)}</strong>
                                        <span>{t('app.browser.tipExternalDesc', language)}</span>
                                    </div>
                                </div>
                            </div>
                        </div>
                    )}
                    {!isHome && (
                        <iframe
                            ref={el => {
                                this.iframeRef = el;
                            }}
                            src={this.state.iframeSrc}
                            class="browser-iframe"
                            style="width: 100%; height: 100%; border: none; background: #fff;"
                            onLoad={this.handleIframeLoad}
                        />
                    )}
                </div>
            </div>
        );
    }
}
