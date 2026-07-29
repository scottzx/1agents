import { h, Fragment } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { t, type Lang } from '../../i18n';
import { type SettingsCategory, isRelayClientHost } from '../../modules/settings-manifest';
import { agentCatalog, agentCatalogLoading, loadAgentCatalog } from '../../stores/agentCatalogStore';
import { uiMode, setUiMode } from '../../stores/uiStore';
import { RelayPairingPanel } from './RelayPairingPanel';
import { SubscriptionPanel } from './SubscriptionPanel';
import { RelayDevicePanel } from './RelayDevicePanel';
import { LocalMachinePanel, isLocalMachineMode } from './LocalMachinePanel';
import { DevicesPanel } from './DevicesPanel';
import { CoffeePanel } from './CoffeePanel';
import { APP_VERSION, isNewer } from '../../version';
import { fetchManifest } from '../../ota/checker';
import type { RootManifest } from '../../ota/checker';
import { apply as applyFrontendUpdate } from '../../ota/applier';

export type { SettingsCategory };

// Model B（设备档案 / 扫码 bundle）为主路径；账户级配对(Model A)默认隐藏作回退。
const SHOW_LEGACY_ACCOUNT_PAIRING = false;

interface SystemSettingsProps {
    theme: 'light' | 'dark';
    toggleTheme: (themeMode?: 'light' | 'dark') => void;
    language: Lang;
    toggleLanguage: (lang: Lang) => void;
    accessTokenExists: boolean;
    onGenerateAccessToken: () => void;
    onRevokeAccessToken: () => void;
    /**
     * Active sub-category. The component is purely content — it doesn't
     * carry an internal sidebar. The host (workspace's left sidebar in
     * desktop state, the "more" menu in mobile state) renders the category
     * navigation in its own chrome and passes the active one down.
     */
    activeCategory: SettingsCategory;
}

/**
 * System settings — content view for the active sub-category.
 *
 * The category navigation lives outside this component (in the host's
 * own sidebar/header, mirroring the skill-management design). Switching
 * categories re-renders this component with a different `activeCategory`
 * prop; no internal state is needed for that.
 */
export function SystemSettings(props: SystemSettingsProps) {
    const {
        theme,
        toggleTheme,
        language,
        toggleLanguage,
        accessTokenExists,
        onGenerateAccessToken,
        onRevokeAccessToken,
        activeCategory,
    } = props;

    const confirmReset = useSignal(false);
    // ── 重置本地数据 (server-side data wipe, preserves relay pairing) ─────────
    const resetDataModalOpen = useSignal(false);
    const resetDataAck = useSignal(false); // checkbox: user understands the wipe
    const resetDataBusy = useSignal(false);
    const resetDataError = useSignal('');
    // Agent type whose install command was just copied (transient checkmark).
    const copiedAgent = useSignal('');
    const creditsExpanded = useSignal(false);

    // ── Updates panel state ─────────────────────────────────────────────────
    const versionLoading = useSignal(false);
    const versionError = useSignal('');
    const manifest = useSignal<RootManifest | null>(null);
    const backendCurrent = useSignal('');
    const backendLatest = useSignal('');
    const backendUpdating = useSignal(false);
    const backendUpdateLog = useSignal('');
    const backendUpdateDone = useSignal(false);

    const loadVersionInfo = async () => {
        versionLoading.value = true;
        versionError.value = '';
        try {
            const [mfst, versionRes] = await Promise.all([
                fetchManifest(),
                fetch('/api/system/version').then(r => r.json()),
            ]);
            manifest.value = mfst;
            backendCurrent.value = versionRes.current ?? '';
            backendLatest.value = versionRes.latest ?? '';
        } catch (err) {
            versionError.value = String(err);
        } finally {
            versionLoading.value = false;
        }
    };

    useEffect(() => {
        if (activeCategory === 'updates') {
            loadVersionInfo();
        }
    }, [activeCategory]);

    const handleFrontendUpdate = () => {
        if (manifest.value) applyFrontendUpdate(manifest.value);
        else window.location.reload();
    };

    const handleBackendUpdate = async () => {
        backendUpdating.value = true;
        backendUpdateLog.value = '';
        backendUpdateDone.value = false;
        try {
            await fetch('/api/system/update', { method: 'POST' });
        } catch (_) {
            /* backend may already be restarting */
        }

        const poll = setInterval(async () => {
            try {
                const res = await fetch('/api/system/update/status');
                const data = await res.json();
                const lines: string[] = data.lines ?? [];
                backendUpdateLog.value = lines.join('\n');
                const last = lines[lines.length - 1] ?? '';
                if (
                    last.includes('done') ||
                    last.includes('restart') ||
                    last.includes('error') ||
                    last.includes('failed')
                ) {
                    clearInterval(poll);
                    backendUpdating.value = false;
                    backendUpdateDone.value = !last.includes('error') && !last.includes('failed');
                }
            } catch (_) {
                /* ignore transient errors during restart */
            }
        }, 1000);
    };

    const handleResetCache = () => {
        if (!confirmReset.value) {
            confirmReset.value = true;
            return;
        }
        try {
            localStorage.clear();
        } catch (_) {
            /* ignore */
        }
        window.location.reload();
    };

    // handleResetData wipes all local App data on the server (tasks/projects/
    // sessions/knowledge/digest), keeping the relay pairing identity, then
    // reloads to a clean state. Gated behind the modal's acknowledge checkbox.
    const handleResetData = async () => {
        if (!resetDataAck.value || resetDataBusy.value) return;
        resetDataBusy.value = true;
        resetDataError.value = '';
        try {
            const res = await fetch('/api/system/reset', { method: 'POST' });
            if (!res.ok) {
                const body = await res.json().catch(() => ({}));
                throw new Error(body.error || `HTTP ${res.status}`);
            }
            // Drop browser-side state too, then reload into the fresh App.
            try {
                localStorage.clear();
            } catch (_) {
                /* ignore */
            }
            window.location.reload();
        } catch (e) {
            resetDataError.value = e instanceof Error ? e.message : String(e);
            resetDataBusy.value = false;
        }
    };

    const copyInstall = (key: string, cmd: string) => {
        const done = () => {
            copiedAgent.value = key;
            window.setTimeout(() => {
                if (copiedAgent.value === key) copiedAgent.value = '';
            }, 1500);
        };
        if (navigator.clipboard?.writeText) {
            navigator.clipboard
                .writeText(cmd)
                .then(done)
                .catch(() => done());
        } else {
            done();
        }
    };

    const renderGeneral = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.general', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.general.desc', language)}</div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <circle cx="12" cy="12" r="10" />
                    <line x1="2" y1="12" x2="22" y2="12" />
                    <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                </svg>
                {t('settings.general.title', language)}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="10" />
                            <line x1="2" y1="12" x2="22" y2="12" />
                            <path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.general.uiLang', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.general.uiLangDesc', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${language === 'zh-CN' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('zh-CN')}
                    >
                        {t('settings.general.uiLangZh', language)}
                    </button>
                    <button
                        class={`sys-settings-option-btn ${language === 'en-US' ? 'active' : ''}`}
                        onClick={() => toggleLanguage('en-US')}
                    >
                        {t('settings.general.uiLangEn', language)}
                    </button>
                </div>
            </div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <circle cx="12" cy="12" r="4" />
                    <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                </svg>
                {t('settings.appearance.title', language)}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="10" />
                            <path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3" />
                            <line x1="12" y1="17" x2="12.01" y2="17" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.appearance.uiMode', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.appearance.uiModeDesc', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-toggle-group">
                    <button
                        class={`sys-settings-option-btn ${uiMode.value === 'beginner' ? 'active' : ''}`}
                        onClick={() => setUiMode('beginner')}
                    >
                        {t('settings.appearance.beginnerMode', language)}
                    </button>
                    <button
                        class={`sys-settings-option-btn ${uiMode.value === 'advanced' ? 'active' : ''}`}
                        onClick={() => setUiMode('advanced')}
                    >
                        {t('settings.appearance.advancedMode', language)}
                    </button>
                </div>
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="4" />
                            <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.appearance.colorTheme', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {t('settings.appearance.colorThemeDesc', language)}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-theme-grid">
                    <button
                        class={`sys-settings-theme-card ${theme === 'light' ? 'active' : ''}`}
                        onClick={() => toggleTheme('light')}
                    >
                        <div class="sys-settings-theme-preview light-preview">
                            <div class="preview-bar" />
                            <div class="preview-content">
                                <div class="preview-line" style="width:70%" />
                                <div class="preview-line" style="width:50%" />
                            </div>
                        </div>
                        <div class="sys-settings-theme-label">
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="4" />
                                <path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M6.34 17.66l-1.41 1.41M19.07 4.93l-1.41 1.41" />
                            </svg>
                            {t('settings.appearance.lightMode', language)}
                        </div>
                        {theme === 'light' && <div class="sys-settings-theme-check">✓</div>}
                    </button>
                    <button
                        class={`sys-settings-theme-card ${theme === 'dark' ? 'active' : ''}`}
                        onClick={() => toggleTheme('dark')}
                    >
                        <div class="sys-settings-theme-preview dark-preview">
                            <div class="preview-bar" />
                            <div class="preview-content">
                                <div class="preview-line" style="width:70%" />
                                <div class="preview-line" style="width:50%" />
                            </div>
                        </div>
                        <div class="sys-settings-theme-label">
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                            </svg>
                            {t('settings.appearance.darkMode', language)}
                        </div>
                        {theme === 'dark' && <div class="sys-settings-theme-check">✓</div>}
                    </button>
                </div>
            </div>
        </div>
    );

    const renderRelay = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.relay', language)}</div>
            <div class="sys-settings-section-desc">
                {isLocalMachineMode()
                    ? '本机模式：开启 Relay Daemon 并将凭据分发给客户端'
                    : t('settings.relay.desc', language)}
            </div>

            {/* 本机模式：启停 daemon + 展示 machine key */}
            {isLocalMachineMode() && <LocalMachinePanel />}

            {/* 本机密钥 — moved here from the former 安全设置 category */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect x="3" y="11" width="18" height="11" rx="2" ry="2" />
                            <path d="M7 11V7a5 5 0 0 1 10 0v4" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.security.token', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {accessTokenExists
                                ? t('settings.security.tokenSet', language)
                                : t('settings.security.tokenUnset', language)}
                        </div>
                    </div>
                </div>

                <div class="sys-settings-token-status">
                    <div class={`sys-settings-token-badge ${accessTokenExists ? 'active' : 'inactive'}`}>
                        {accessTokenExists ? (
                            <Fragment>
                                <svg
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                                {t('settings.security.active', language)}
                            </Fragment>
                        ) : (
                            <Fragment>
                                <svg
                                    width="12"
                                    height="12"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="10" />
                                    <line x1="12" y1="8" x2="12" y2="12" />
                                    <line x1="12" y1="16" x2="12.01" y2="16" />
                                </svg>
                                {t('settings.security.inactive', language)}
                            </Fragment>
                        )}
                    </div>
                </div>

                <div class="sys-settings-action-row">
                    {accessTokenExists ? (
                        <button class="sys-settings-btn danger" onClick={onRevokeAccessToken}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="3 6 5 6 21 6" />
                                <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
                                <path d="M10 11v6M14 11v6" />
                                <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
                            </svg>
                            {t('settings.security.revoke', language)}
                        </button>
                    ) : (
                        <button class="sys-settings-btn primary" onClick={onGenerateAccessToken}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M5 12h14M12 5v14" />
                            </svg>
                            {t('settings.security.generate', language)}
                        </button>
                    )}
                </div>
            </div>

            {/* 客户端主路径：Model B 设备档案 —— 扫机器端配置二维码 / 粘贴 bundle。
                订阅由客户端门禁控制；不依赖 listMachines 账户级列表。 */}
            {!isLocalMachineMode() && <RelayDevicePanel embedded onConnected={() => window.location.reload()} />}

            {/* 旧账户级配对(Model A) —— 默认隐藏；与 Model B 混用易账号错配。 */}
            {!isLocalMachineMode() && SHOW_LEGACY_ACCOUNT_PAIRING && <RelayPairingPanel embedded />}
        </div>
    );

    const renderAgents = () => {
        const list = agentCatalog.value;
        const installedCount = list.filter(a => a.installed).length;
        return (
            <div class="sys-settings-section">
                <div class="sys-settings-section-title">{t('settings.nav.agents', language)}</div>
                <div class="sys-settings-section-desc">{t('settings.agents.desc', language)}</div>

                <div class="sys-settings-card">
                    <div
                        class="sys-settings-action-row"
                        style="flex-direction: row; justify-content: space-between; align-items: center; border-bottom: 1px solid var(--border-color); padding-bottom: 12px; margin-bottom: 12px;"
                    >
                        <span class="sys-settings-card-subtitle" style="font-size: 12px; font-weight: 500;">
                            {t('settings.agents.summary', language, {
                                '0': installedCount,
                                '1': list.length,
                            })}
                        </span>
                        <button
                            class="sys-settings-btn ghost"
                            disabled={agentCatalogLoading.value}
                            onClick={() => loadAgentCatalog(true)}
                            style="height: 30px; padding: 0 12px; font-size: 11.5px;"
                        >
                            {agentCatalogLoading.value
                                ? t('settings.agents.refreshing', language)
                                : t('settings.agents.refresh', language)}
                        </button>
                    </div>

                    <div class="agent-catalog-list" style="margin-top: 0;">
                        {list.map(a => (
                            <div class="agent-catalog-row" key={a.type}>
                                <div class="agent-catalog-header">
                                    <div class="agent-catalog-main">
                                        <span
                                            class={`agent-catalog-dot ${a.installed ? 'installed' : 'missing'}`}
                                            aria-hidden="true"
                                        />
                                        <span class="agent-catalog-name">{a.label}</span>
                                    </div>
                                    <div class="agent-catalog-badges">
                                        {a.acpCapable && (
                                            <span class="agent-cap-badge acp">
                                                {t('settings.agents.capAcp', language)}
                                            </span>
                                        )}
                                        {a.cliCapable && (
                                            <span class="agent-cap-badge cli">
                                                {t('settings.agents.capCli', language)}
                                            </span>
                                        )}
                                    </div>
                                </div>

                                {a.installed && a.path && (
                                    <div class="agent-catalog-path-row">
                                        <code class="agent-catalog-path-text" title={a.path}>
                                            {a.path}
                                        </code>
                                    </div>
                                )}

                                {!a.installed && a.installCommand && (
                                    <div class="agent-catalog-path-row">
                                        <code class="agent-catalog-path-text" title={a.installCommand}>
                                            {a.installCommand}
                                        </code>
                                        <button
                                            class="agent-catalog-copy-btn"
                                            title={
                                                copiedAgent.value === a.type
                                                    ? t('settings.agents.copied', language)
                                                    : t('settings.agents.copy', language)
                                            }
                                            onClick={() => copyInstall(a.type, a.installCommand!)}
                                        >
                                            {copiedAgent.value === a.type ? (
                                                <svg
                                                    width="14"
                                                    height="14"
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    strokeWidth="2.5"
                                                    strokeLinecap="round"
                                                    strokeLinejoin="round"
                                                >
                                                    <polyline points="20 6 9 17 4 12" />
                                                </svg>
                                            ) : (
                                                <svg
                                                    width="14"
                                                    height="14"
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    strokeWidth="2"
                                                    strokeLinecap="round"
                                                    strokeLinejoin="round"
                                                >
                                                    <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                                                    <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                                                </svg>
                                            )}
                                        </button>
                                    </div>
                                )}
                            </div>
                        ))}
                    </div>
                </div>
            </div>
        );
    };

    /**
     * Curated list of major open-source projects we depend on. Kept short
     * intentionally — this is acknowledgement, not a full SBOM. License
     * field is the project's SPDX identifier; url points at the canonical
     * upstream repository or homepage.
     */
    const CREDITS_GROUPS: Array<{
        key: 'frontend' | 'bridge' | 'agents' | 'infra';
        items: Array<{ name: string; descKey: string; license: string; url: string }>;
    }> = [
        {
            key: 'frontend',
            items: [
                {
                    name: 'ttyd',
                    descKey: 'settings.credits.ttyd',
                    license: 'MIT',
                    url: 'https://github.com/tsl0922/ttyd',
                },
                {
                    name: 'xterm.js',
                    descKey: 'settings.credits.xterm',
                    license: 'MIT',
                    url: 'https://github.com/xtermjs/xterm.js',
                },
                {
                    name: 'Preact',
                    descKey: 'settings.credits.preact',
                    license: 'MIT',
                    url: 'https://github.com/preactjs/preact',
                },
                {
                    name: 'Marked',
                    descKey: 'settings.credits.marked',
                    license: 'MIT',
                    url: 'https://github.com/markedjs/marked',
                },
                {
                    name: 'trzsz',
                    descKey: 'settings.credits.trzsz',
                    license: 'MIT',
                    url: 'https://github.com/trzsz/trzsz.js',
                },
                {
                    name: 'webpack',
                    descKey: 'settings.credits.webpack',
                    license: 'MIT',
                    url: 'https://github.com/webpack/webpack',
                },
            ],
        },
        {
            key: 'bridge',
            items: [
                {
                    name: 'Bubble Tea',
                    descKey: 'settings.credits.bubbletea',
                    license: 'MIT',
                    url: 'https://github.com/charmbracelet/bubbletea',
                },
                {
                    name: 'discordgo',
                    descKey: 'settings.credits.discordgo',
                    license: 'BSD-3-Clause',
                    url: 'https://github.com/bwmarrin/discordgo',
                },
                {
                    name: 'go-telegram/bot',
                    descKey: 'settings.credits.telegram',
                    license: 'MIT',
                    url: 'https://github.com/go-telegram/bot',
                },
                {
                    name: 'slack-go',
                    descKey: 'settings.credits.slack',
                    license: 'BSD-2-Clause',
                    url: 'https://github.com/slack-go/slack',
                },
                {
                    name: 'line-bot-sdk-go',
                    descKey: 'settings.credits.line',
                    license: 'Apache-2.0',
                    url: 'https://github.com/line/line-bot-sdk-go',
                },
                {
                    name: 'larksuite/oapi-sdk-go',
                    descKey: 'settings.credits.feishu',
                    license: 'MIT',
                    url: 'https://github.com/larksuite/oapi-sdk-go',
                },
                {
                    name: 'dingtalk-stream-sdk-go',
                    descKey: 'settings.credits.dingtalk',
                    license: 'Apache-2.0',
                    url: 'https://github.com/open-dingtalk/dingtalk-stream-sdk-go',
                },
                {
                    name: 'gorilla/websocket',
                    descKey: 'settings.credits.websocket',
                    license: 'BSD-2-Clause',
                    url: 'https://github.com/gorilla/websocket',
                },
            ],
        },
        {
            key: 'agents',
            items: [
                {
                    name: 'cc-switch',
                    descKey: 'settings.credits.ccswitch',
                    license: 'MIT',
                    url: 'https://github.com/farion1231/cc-switch',
                },
                {
                    name: 'cc-switch-cli',
                    descKey: 'settings.credits.ccswitchcli',
                    license: 'MIT',
                    url: 'https://github.com/SaladDay/cc-switch-cli',
                },
                {
                    name: 'skill-manager',
                    descKey: 'settings.credits.skillmanager',
                    license: 'MIT',
                    url: 'https://github.com/mode-io/skill-manager',
                },
            ],
        },
        {
            key: 'infra',
            items: [
                {
                    name: 'modernc.org/sqlite',
                    descKey: 'settings.credits.sqlite',
                    license: 'BSD-3-Clause',
                    url: 'https://gitlab.com/cznic/sqlite',
                },
                {
                    name: 'BurntSushi/toml',
                    descKey: 'settings.credits.toml',
                    license: 'MIT',
                    url: 'https://github.com/BurntSushi/toml',
                },
                {
                    name: 'creack/pty',
                    descKey: 'settings.credits.pty',
                    license: 'MIT',
                    url: 'https://github.com/creack/pty',
                },
                {
                    name: 'robfig/cron',
                    descKey: 'settings.credits.cron',
                    license: 'MIT',
                    url: 'https://github.com/robfig/cron',
                },
            ],
        },
    ];

    const frontendLatest = manifest.value?.components?.frontend?.version ?? '';
    const frontendHasUpdate = !!frontendLatest && isNewer(frontendLatest, APP_VERSION);
    const backendHasUpdate =
        !!backendLatest.value && !!backendCurrent.value && isNewer(backendLatest.value, backendCurrent.value);

    const renderUpdates = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.updates', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.updates.desc', language)}</div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <polyline points="23 4 23 10 17 10" />
                    <polyline points="1 20 1 14 7 14" />
                    <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                </svg>
                {t('settings.updates.title', language)}
            </div>

            {versionLoading.value ? (
                <div class="sys-settings-card">
                    <span style="color: var(--text-muted); font-size: 13px;">
                        {t('settings.updates.checking', language)}
                    </span>
                </div>
            ) : versionError.value ? (
                <div class="sys-settings-card">
                    <span style="color: var(--danger-fg); font-size: 13px;">
                        {t('settings.updates.error', language)}: {versionError.value}
                    </span>
                </div>
            ) : (
                <div class="sys-settings-update-grid">
                    {/* Frontend */}
                    <div class="sys-settings-card sys-settings-update-card">
                        <div class="sys-settings-update-card-body">
                            <div class="sys-settings-update-icon">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="2" y="3" width="20" height="14" rx="2" ry="2" />
                                    <line x1="8" y1="21" x2="16" y2="21" />
                                    <line x1="12" y1="17" x2="12" y2="21" />
                                </svg>
                            </div>
                            <div class="sys-settings-update-info">
                                <div class="sys-settings-update-name">{t('settings.updates.frontend', language)}</div>
                                <div class="sys-settings-update-versions">
                                    <span class="sys-settings-version-chip">{APP_VERSION || '—'}</span>
                                    {frontendHasUpdate && frontendLatest && (
                                        <Fragment>
                                            <span class="sys-settings-update-arrow">→</span>
                                            <span class="sys-settings-version-chip new">{frontendLatest}</span>
                                        </Fragment>
                                    )}
                                    <span
                                        class={`sys-settings-update-badge ${frontendHasUpdate ? 'available' : 'uptodate'}`}
                                    >
                                        {frontendHasUpdate
                                            ? t('settings.updates.available', language)
                                            : t('settings.updates.upToDate', language)}
                                    </span>
                                </div>
                            </div>
                            {frontendHasUpdate && (
                                <button class="sys-settings-btn primary" onClick={handleFrontendUpdate}>
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="23 4 23 10 17 10" />
                                        <polyline points="1 20 1 14 7 14" />
                                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                                    </svg>
                                    {t('settings.updates.refreshBtn', language)}
                                </button>
                            )}
                        </div>
                    </div>

                    {/* Backend */}
                    <div class="sys-settings-card sys-settings-update-card">
                        <div class="sys-settings-update-card-body">
                            <div
                                class="sys-settings-update-icon"
                                style="background-color: rgba(var(--success-rgb), 0.1);"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    style="stroke: var(--success-fg);"
                                >
                                    <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
                                    <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
                                    <line x1="6" y1="6" x2="6.01" y2="6" />
                                    <line x1="6" y1="18" x2="6.01" y2="18" />
                                </svg>
                            </div>
                            <div class="sys-settings-update-info">
                                <div class="sys-settings-update-name">{t('settings.updates.backend', language)}</div>
                                <div class="sys-settings-update-versions">
                                    <span class="sys-settings-version-chip">{backendCurrent.value || '—'}</span>
                                    {backendHasUpdate && backendLatest.value && (
                                        <Fragment>
                                            <span class="sys-settings-update-arrow">→</span>
                                            <span class="sys-settings-version-chip new">{backendLatest.value}</span>
                                        </Fragment>
                                    )}
                                    <span
                                        class={`sys-settings-update-badge ${backendUpdating.value ? 'checking' : backendHasUpdate ? 'available' : 'uptodate'}`}
                                    >
                                        {backendUpdating.value
                                            ? t('settings.updates.checking', language)
                                            : backendHasUpdate
                                              ? t('settings.updates.available', language)
                                              : t('settings.updates.upToDate', language)}
                                    </span>
                                </div>
                            </div>
                            {backendHasUpdate && !backendUpdating.value && !backendUpdateDone.value && (
                                <button class="sys-settings-btn primary" onClick={handleBackendUpdate}>
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
                                        <polyline points="7 10 12 15 17 10" />
                                        <line x1="12" y1="15" x2="12" y2="3" />
                                    </svg>
                                    {t('settings.updates.updateBtn', language)}
                                </button>
                            )}
                        </div>
                        {(backendUpdating.value || backendUpdateLog.value) && (
                            <div class="sys-settings-update-log">
                                {backendUpdateLog.value || t('settings.updates.updating', language)}
                            </div>
                        )}
                    </div>
                </div>
            )}

            <div class="sys-settings-action-row" style="margin-top: 4px;">
                <button class="sys-settings-btn ghost" onClick={loadVersionInfo} disabled={versionLoading.value}>
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <polyline points="23 4 23 10 17 10" />
                        <polyline points="1 20 1 14 7 14" />
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                    </svg>
                    {t('settings.updates.checkBtn', language)}
                </button>
            </div>
        </div>
    );

    const renderAbout = () => (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">{t('settings.nav.about', language)}</div>
            <div class="sys-settings-section-desc">{t('settings.about.desc', language)}</div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <circle cx="12" cy="12" r="10" />
                    <line x1="12" y1="16" x2="12" y2="12" />
                    <line x1="12" y1="8" x2="12.01" y2="8" />
                </svg>
                {t('settings.about.title', language)}
            </div>

            <div class="sys-settings-card sys-settings-about-card">
                <div class="sys-settings-about-brand">
                    <img class="sys-settings-about-logo" src="/logo.png" alt="1agents logo" />
                    <div class="sys-settings-about-info">
                        <div class="sys-settings-about-name">1agents</div>
                        <div class="sys-settings-about-tagline">{t('settings.about.tagline', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-about-meta">
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.version', language)}</span>
                        <span class="meta-value">1.0.0</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.platform', language)}</span>
                        <span class="meta-value">Web / Desktop</span>
                    </div>
                    <div class="sys-settings-about-meta-row">
                        <span class="meta-label">{t('settings.about.project', language)}</span>
                        <a
                            class="meta-value meta-link"
                            href="https://github.com"
                            target="_blank"
                            rel="noopener noreferrer"
                        >
                            github.com/1agents
                        </a>
                    </div>
                </div>
            </div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
                </svg>
                {t('settings.feedback.title', language)}
            </div>

            <div class="sys-settings-card">
                <div style="display: flex; flex-direction: column; gap: 12px; width: 100%;">
                    <div class="sys-settings-card-header">
                        <div class="sys-settings-card-icon">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                                <polyline points="9 22 9 12 15 12 15 22" />
                            </svg>
                        </div>
                        <div>
                            <div class="sys-settings-card-title">{t('settings.feedback.company', language)}</div>
                            <div class="sys-settings-card-subtitle">{t('settings.feedback.companyName', language)}</div>
                        </div>
                    </div>

                    <div class="sys-settings-card-header">
                        <div class="sys-settings-card-icon">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M4 4h16c1.1 0 2 .9 2 2v12c0 1.1-.9 2-2 2H4c-1.1 0-2-.9-2-2V6c0-1.1.9-2 2-2z" />
                                <polyline points="22,6 12,13 2,6" />
                            </svg>
                        </div>
                        <div>
                            <div class="sys-settings-card-title">{t('settings.feedback.email', language)}</div>
                            <div class="sys-settings-card-subtitle">
                                <a
                                    href="mailto:xiaofengzeng93@outlook.com"
                                    class="meta-link"
                                    style="word-break: break-all;"
                                >
                                    xiaofengzeng93@outlook.com
                                </a>
                            </div>
                        </div>
                    </div>
                </div>
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.feedback.form', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.feedback.formDesc', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-action-row">
                    <a
                        class="sys-settings-btn primary"
                        href="https://my.feishu.cn/share/base/form/shrcn0OGqn5ZBCiPEpmJuJ3Djtc"
                        target="_blank"
                        rel="noopener noreferrer"
                        style="text-decoration: none; display: inline-flex; align-items: center; gap: 6px;"
                    >
                        <svg
                            width="14"
                            height="14"
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
                        {t('settings.feedback.open', language)}
                    </a>
                </div>
            </div>

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <path d="M19 11H5m14 0a2 2 0 0 1 2 2v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-6a2 2 0 0 1 2-2m14 0V9a2 2 0 0 0-2-2M5 11V9a2 2 0 0 1 2-2m0 0V5a2 2 0 0 1 2-2h6a2 2 0 0 1 2 2v2M7 7h10" />
                </svg>
                {t('settings.about.reset', language)}
            </div>

            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon danger-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <polyline points="1 4 1 10 7 10" />
                            <path d="M3.51 15a9 9 0 1 0 .49-3.51" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.about.reset', language)}</div>
                        <div class="sys-settings-card-subtitle">
                            {confirmReset.value
                                ? t('settings.about.resetWarning', language)
                                : t('settings.about.resetDesc', language)}
                        </div>
                    </div>
                </div>
                <div class="sys-settings-action-row">
                    {confirmReset.value ? (
                        <Fragment>
                            <button class="sys-settings-btn danger" onClick={handleResetCache}>
                                <svg
                                    width="14"
                                    height="14"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="20 6 9 17 4 12" />
                                </svg>
                                {t('settings.about.confirmReset', language)}
                            </button>
                            <button class="sys-settings-btn ghost" onClick={() => (confirmReset.value = false)}>
                                {t('common.cancel', language)}
                            </button>
                        </Fragment>
                    ) : (
                        <button class="sys-settings-btn warning" onClick={handleResetCache}>
                            <svg
                                width="14"
                                height="14"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="1 4 1 10 7 10" />
                                <path d="M3.51 15a9 9 0 1 0 .49-3.51" />
                            </svg>
                            {t('settings.about.resetBtn', language)}
                        </button>
                    )}
                </div>
            </div>

            <div class="sys-settings-card sys-settings-reset-data-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-icon danger-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M3 6h18" />
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                            <line x1="10" y1="11" x2="10" y2="17" />
                            <line x1="14" y1="11" x2="14" y2="17" />
                        </svg>
                    </div>
                    <div>
                        <div class="sys-settings-card-title">{t('settings.about.resetData', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.about.resetDataDesc', language)}</div>
                    </div>
                </div>
                <div class="sys-settings-action-row">
                    <button
                        class="sys-settings-btn danger"
                        onClick={() => {
                            resetDataAck.value = false;
                            resetDataError.value = '';
                            resetDataModalOpen.value = true;
                        }}
                    >
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M3 6h18" />
                            <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                        </svg>
                        {t('settings.about.resetData', language)}
                    </button>
                </div>
            </div>

            {resetDataModalOpen.value && (
                <div
                    class="sys-settings-modal-overlay"
                    onClick={() => {
                        if (!resetDataBusy.value) resetDataModalOpen.value = false;
                    }}
                >
                    <div class="sys-settings-modal sys-settings-reset-data-modal" onClick={e => e.stopPropagation()}>
                        <div class="sys-settings-modal-title">
                            <svg
                                width="18"
                                height="18"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="var(--danger-fg)"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M10.29 3.86 1.82 18a2 2 0 0 0 1.71 3h16.94a2 2 0 0 0 1.71-3L13.71 3.86a2 2 0 0 0-3.42 0z" />
                                <line x1="12" y1="9" x2="12" y2="13" />
                                <line x1="12" y1="17" x2="12.01" y2="17" />
                            </svg>
                            {t('settings.about.resetData', language)}
                        </div>
                        <p class="sys-settings-modal-warning">{t('settings.about.resetDataModalWarn', language)}</p>
                        <p class="sys-settings-modal-keep">{t('settings.about.resetDataModalKeep', language)}</p>
                        <p class="sys-settings-modal-backup">{t('settings.about.resetDataModalBackup', language)}</p>
                        {resetDataError.value && <p class="sys-settings-modal-error">{resetDataError.value}</p>}
                        <label class="sys-settings-modal-ack">
                            <input
                                type="checkbox"
                                checked={resetDataAck.value}
                                disabled={resetDataBusy.value}
                                onChange={e => (resetDataAck.value = (e.target as HTMLInputElement).checked)}
                            />
                            <span>{t('settings.about.resetDataModalAck', language)}</span>
                        </label>
                        <div class="sys-settings-modal-actions">
                            <button
                                class="sys-settings-btn ghost"
                                disabled={resetDataBusy.value}
                                onClick={() => (resetDataModalOpen.value = false)}
                            >
                                {t('common.cancel', language)}
                            </button>
                            <button
                                class="sys-settings-btn danger"
                                disabled={!resetDataAck.value || resetDataBusy.value}
                                onClick={handleResetData}
                            >
                                {resetDataBusy.value
                                    ? t('settings.about.resetDataModalBusy', language)
                                    : t('settings.about.resetDataModalConfirm', language)}
                            </button>
                        </div>
                    </div>
                </div>
            )}

            <div class="sys-settings-sub-title">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    style="width: 14px; height: 14px;"
                >
                    <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
                </svg>
                {t('settings.credits.title', language)}
            </div>

            <div class="sys-settings-card credits-card" style="flex-direction: column; align-items: stretch;">
                <div
                    class="sys-settings-card-header"
                    onClick={() => (creditsExpanded.value = !creditsExpanded.value)}
                    style="cursor: pointer; user-select: none; width: 100%;"
                >
                    <div class="sys-settings-card-icon">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M20.84 4.61a5.5 5.5 0 0 0-7.78 0L12 5.67l-1.06-1.06a5.5 5.5 0 0 0-7.78 7.78l1.06 1.06L12 21.23l7.78-7.78 1.06-1.06a5.5 5.5 0 0 0 0-7.78z" />
                        </svg>
                    </div>
                    <div style="flex: 1;">
                        <div class="sys-settings-card-title">{t('settings.credits.title', language)}</div>
                        <div class="sys-settings-card-subtitle">{t('settings.credits.desc', language)}</div>
                    </div>
                    <div
                        style={`transform: rotate(${creditsExpanded.value ? 180 : 0}deg); transition: transform 0.2s ease; display: flex; align-items: center; justify-content: center; opacity: 0.6;`}
                    >
                        <svg
                            width="18"
                            height="18"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <polyline points="6 9 12 15 18 9" />
                        </svg>
                    </div>
                </div>

                {creditsExpanded.value && (
                    <div
                        class="credits-expanded-content"
                        style="margin-top: 12px; border-top: 1px solid var(--border-color); padding-top: 16px; width: 100%;"
                    >
                        {CREDITS_GROUPS.map(group => (
                            <div key={group.key} style="margin-bottom: 20px;">
                                <div style="font-size: 13px; font-weight: 600; color: var(--text-main); margin-bottom: 6px; opacity: 0.9;">
                                    {t(`settings.credits.group.${group.key}`, language)}
                                </div>
                                <div style="font-size: 11.5px; color: var(--text-secondary); margin-bottom: 10px;">
                                    {t(`settings.credits.group.${group.key}.desc`, language)}
                                </div>
                                <ul style="margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: 8px;">
                                    {group.items.map(item => (
                                        <li
                                            key={item.url}
                                            style="display: flex; align-items: baseline; gap: 8px; flex-wrap: wrap;"
                                        >
                                            <a
                                                class="meta-link"
                                                href={item.url}
                                                target="_blank"
                                                rel="noopener noreferrer"
                                                style="font-weight: 600; font-size: 12.5px;"
                                            >
                                                {item.name}
                                            </a>
                                            <span style="font-size: 10px; padding: 1px 5px; border-radius: 4px; border: 1px solid currentColor; opacity: 0.6; font-family: var(--font-mono);">
                                                {item.license}
                                            </span>
                                            <span style="opacity: 0.75; font-size: 12px;">
                                                — {t(item.descKey, language)}
                                            </span>
                                        </li>
                                    ))}
                                </ul>
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );

    const renderContent = () => {
        switch (activeCategory) {
            case 'general':
                return renderGeneral();
            case 'agents':
                return renderAgents();
            case 'account':
                // Subscription is a Relay/client concept — on the local backend
                // (localhost) it's hidden from nav; guard deep-links too.
                return isRelayClientHost() ? <SubscriptionPanel /> : renderGeneral();
            case 'relay':
                return renderRelay();
            case 'devices':
                return <DevicesPanel language={language} />;
            case 'updates':
                return renderUpdates();
            case 'coffee':
                return <CoffeePanel language={language} theme={theme} />;
            case 'about':
                return renderAbout();
            default:
                return null;
        }
    };

    return (
        <div class="sys-settings-page sys-settings-page--bare">
            <div class="sys-settings-content">{renderContent()}</div>
        </div>
    );
}
