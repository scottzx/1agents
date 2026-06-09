import { h, Component } from 'preact';
import { t, type Lang } from '../../i18n';
import type { Tab } from '../app';

export interface BuiltinBrowserProps {
    tab: Tab;
    active: boolean;
    onUrlChange: (tabId: string, url: string) => void;
    language: Lang;
}

export interface BuiltinBrowserState {
    iframeSrc: string;
}

export class BuiltinBrowser extends Component<BuiltinBrowserProps, BuiltinBrowserState> {
    private inputRef: HTMLInputElement | null = null;
    private iframeRef: HTMLIFrameElement | null = null;
    private lastLoadedUrl: string = '';

    state: BuiltinBrowserState = {
        iframeSrc: this.getIframeUrl(this.props.tab.url || ''),
    };

    componentDidMount() {
        window.addEventListener('message', this.handleIframeMessage);
    }

    componentWillUnmount() {
        window.removeEventListener('message', this.handleIframeMessage);
    }

    componentWillReceiveProps(nextProps: BuiltinBrowserProps) {
        if (nextProps.tab.url !== this.props.tab.url) {
            if (nextProps.tab.url !== this.lastLoadedUrl) {
                this.setState({
                    iframeSrc: this.getIframeUrl(nextProps.tab.url || ''),
                });
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
            const url = new URL(urlStr);
            if (url.pathname === '/api/proxy') {
                const target = url.searchParams.get('url');
                if (target) return target;
            }
            return urlStr;
        } catch (e) {
            return urlStr;
        }
    };

    handleIframeLoad = () => {
        if (!this.iframeRef || !this.iframeRef.contentWindow) return;
        try {
            const iframeUrl = this.iframeRef.contentWindow.location.href;
            if (iframeUrl && iframeUrl !== 'about:blank') {
                const targetUrl = this.getOriginalUrl(iframeUrl);
                if (targetUrl && targetUrl !== this.props.tab.url) {
                    this.lastLoadedUrl = targetUrl;
                    this.props.onUrlChange(this.props.tab.id, targetUrl);
                }
            }
        } catch (e) {
            // Expected cross-origin error when loading non-proxied localhost/intranet sites
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

    isLocalUrl(urlStr: string): boolean {
        try {
            const url = new URL(urlStr);
            const hostname = url.hostname.toLowerCase();
            return (
                hostname === 'localhost' ||
                hostname === '127.0.0.1' ||
                hostname === '::1' ||
                hostname.startsWith('192.168.') ||
                hostname.startsWith('10.') ||
                hostname.startsWith('172.')
            );
        } catch (e) {
            const lower = urlStr.toLowerCase();
            return lower.includes('localhost') || lower.includes('127.0.0.1') || lower.includes('::1');
        }
    }

    getIframeUrl(urlStr: string): string {
        if (!urlStr || urlStr === 'about:blank') {
            return 'about:blank';
        }
        if (this.isLocalUrl(urlStr)) {
            return urlStr;
        }
        // Don't double-wrap an already-proxied URL — breaks the feedback loop
        // if tab.url is transiently a /api/proxy?url=... string
        if (urlStr.startsWith(`${window.location.origin}/api/proxy?url=`)) {
            return urlStr;
        }
        return `${window.location.origin}/api/proxy?url=${encodeURIComponent(urlStr)}`;
    }

    handleKeyPress = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && this.inputRef) {
            let url = this.inputRef.value.trim();
            if (url) {
                if (!/^https?:\/\//i.test(url) && !url.startsWith('about:')) {
                    url = 'http://' + url;
                }
                this.lastLoadedUrl = '';
                this.props.onUrlChange(this.props.tab.id, url);
            }
        }
    };

    handleRefresh = () => {
        if (this.iframeRef && this.iframeRef.contentWindow) {
            try {
                this.iframeRef.contentWindow.location.reload();
            } catch (e) {
                this.iframeRef.src = this.state.iframeSrc;
            }
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
                        value={tab.url === 'about:blank' ? '' : tab.url}
                        ref={el => {
                            this.inputRef = el;
                        }}
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
