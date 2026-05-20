import { h, Component } from 'preact';

import { Terminal } from './terminal';

import type { ITerminalOptions, ITheme } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from './terminal/xterm';

const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
const path = window.location.pathname.replace(/[/]+$/, '');
const wsUrl = [protocol, '//', window.location.host, path, '/ws', window.location.search].join('');
const tokenUrl = [window.location.protocol, '//', window.location.host, path, '/token'].join('');

const clientOptions = {
    rendererType: 'webgl',
    disableLeaveAlert: false,
    disableResizeOverlay: false,
    enableZmodem: false,
    enableTrzsz: false,
    enableSixel: false,
    closeOnDisconnect: false,
    isWindows: false,
    unicodeVersion: '11',
} as ClientOptions;

const flowControl = {
    limit: 100000,
    highWater: 10,
    lowWater: 4,
} as FlowControl;

// High-fidelity terminal theme options
const lightTermTheme = {
    foreground: '#1f2328',
    background: '#fafafa',
    cursor: '#1f2328',
    black: '#1f2328',
    red: '#cf222e',
    green: '#1a7f37',
    yellow: '#9a6700',
    blue: '#0969da',
    magenta: '#8250df',
    cyan: '#1b7c83',
    white: '#ffffff',
    brightBlack: '#6e7781',
    brightRed: '#d1242f',
    brightGreen: '#2da44e',
    brightYellow: '#b48600',
    brightBlue: '#2188ff',
    brightMagenta: '#a371f7',
    brightCyan: '#31929a',
    brightWhite: '#ffffff',
} as ITheme;

const darkTermTheme = {
    foreground: '#d2d2d2',
    background: '#0d1117', // Soft dark palette matching modern SaaS dark-mode
    cursor: '#adadad',
    black: '#000000',
    red: '#d81e00',
    green: '#5ea702',
    yellow: '#cfae00',
    blue: '#427ab3',
    magenta: '#89658e',
    cyan: '#00a7aa',
    white: '#dbded8',
    brightBlack: '#686a66',
    brightRed: '#f54235',
    brightGreen: '#99e343',
    brightYellow: '#fdeb61',
    brightBlue: '#84b0d8',
    brightMagenta: '#bc94b7',
    brightCyan: '#37e6e8',
    brightWhite: '#f1f1f0',
} as ITheme;

const baseTermOptions = {
    fontSize: 13,
    fontFamily: 'JetBrains Mono, Consolas, Liberation Mono, Menlo, monospace',
    allowProposedApi: true,
} as ITerminalOptions;

interface AppState {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders' | 'settings';
    theme: 'light' | 'dark';
    hostname: string;
}

export class App extends Component<{}, AppState> {
    constructor() {
        super();
        this.state = {
            activeTab: 'terminal',
            theme: 'light', // Default to light mode to present reference image styles beautifully
            hostname: 'Ashley Walker',
        };
    }

    componentDidMount() {
        const savedTheme = localStorage.getItem('remote-agents-theme') as 'light' | 'dark' | null;
        const theme = savedTheme || 'light';
        this.setState({ theme });
        document.documentElement.setAttribute('data-theme', theme);

        // Fetch local host details or context title
        this.setState({ hostname: window.location.hostname || 'Ashley Walker' });
    }

    toggleTheme = () => {
        const newTheme = this.state.theme === 'light' ? 'dark' : 'light';
        this.setState({ theme: newTheme });
        document.documentElement.setAttribute('data-theme', newTheme);
        localStorage.setItem('remote-agents-theme', newTheme);

        // Dynamically trigger resize/fit inside xterm
        setTimeout(() => {
            if (window.term && window.term.fit) {
                window.term.fit();
            }
        }, 100);
    };

    setActiveTab = (tab: 'terminal' | 'agents' | 'console' | 'folders' | 'settings') => {
        this.setState({ activeTab: tab });

        // If switching back to terminal, trigger fit
        if (tab === 'terminal') {
            setTimeout(() => {
                if (window.term && window.term.fit) {
                    window.term.fit();
                }
            }, 50);
        }
    };

    render() {
        const { activeTab, theme, hostname } = this.state;
        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
        } as ITerminalOptions;

        return (
            <div class="app-container">
                {/* 1. Global Header Bar */}
                <header class="global-header">
                    <div class="header-left">
                        <div class="home-btn" title="返回首页">
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="m3 9 9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
                                <polyline points="9 22 9 12 15 12 15 22" />
                            </svg>
                        </div>
                        <div class="divider" />
                        <span class="project-title" title={`${hostname}的智能体`}>
                            {hostname}的智能体
                        </span>
                        <div class="badge-ops">
                            <div class="pulse-dot" />
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
                                <polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2" />
                            </svg>
                            <span>AI 运维</span>
                        </div>
                    </div>

                    <div class="header-center">
                        <div
                            class={`nav-tab ${activeTab === 'agents' ? 'active' : ''}`}
                            onClick={() => this.setActiveTab('agents')}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M12 8V4H8" />
                                <rect width="16" height="12" x="4" y="8" rx="2" />
                                <path d="M2 14h2" />
                                <path d="M20 14h2" />
                                <path d="M15 13v2" />
                                <path d="M9 13v2" />
                            </svg>
                            <span>智能体</span>
                        </div>
                        <div
                            class={`nav-tab ${activeTab === 'console' ? 'active' : ''}`}
                            onClick={() => this.setActiveTab('console')}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect width="20" height="14" x="2" y="5" rx="2" />
                                <line x1="2" x2="22" y1="10" y2="10" />
                            </svg>
                            <span>控制台</span>
                        </div>
                        <div
                            class={`nav-tab ${activeTab === 'folders' ? 'active' : ''}`}
                            onClick={() => this.setActiveTab('folders')}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                            </svg>
                            <span>文件夹</span>
                        </div>
                        <div
                            class={`nav-tab ${activeTab === 'terminal' ? 'active' : ''}`}
                            onClick={() => this.setActiveTab('terminal')}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect width="20" height="16" x="2" y="4" rx="2" />
                                <path d="m7 8 3 2-3 2" />
                                <path d="M12 12h4" />
                            </svg>
                            <span>终端</span>
                        </div>
                        <div
                            class={`nav-tab ${activeTab === 'settings' ? 'active' : ''}`}
                            onClick={() => this.setActiveTab('settings')}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.1a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                                <circle cx="12" cy="12" r="3" />
                            </svg>
                            <span>设置</span>
                        </div>
                    </div>

                    <div class="header-right">
                        <button
                            class="theme-toggle-btn"
                            onClick={this.toggleTheme}
                            title={theme === 'light' ? '深色模式' : '浅色模式'}
                        >
                            {theme === 'light' ? (
                                <svg
                                    width="16"
                                    height="16"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                                </svg>
                            ) : (
                                <svg
                                    width="16"
                                    height="16"
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
                            )}
                        </button>
                        <button class="btn-primary">
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
                                <path d="M5 12h14M12 5v14" />
                            </svg>
                            <span>新建智能体</span>
                        </button>
                        <div class="more-options-btn">
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
                                <circle cx="12" cy="12" r="1" />
                                <circle cx="19" cy="12" r="1" />
                                <circle cx="5" cy="12" r="1" />
                            </svg>
                        </div>
                        <div class="user-avatar" title="个人主页">
                            <div class="avatar-placeholder" />
                        </div>
                    </div>
                </header>

                {/* 2. Page Content Switcher */}
                {activeTab === 'terminal' && (
                    <div style="display: flex; flex-direction: column; flex: 1; overflow: hidden;">
                        {/* Terminal Control Toolbar */}
                        <div class="terminal-toolbar">
                            <div class="toolbar-left">
                                <h2 class="page-title">终端</h2>
                            </div>
                            <div class="toolbar-right">
                                <div class="shell-selector" title="选择 Shell 终端">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="4 17 10 11 4 5" />
                                        <line x1="12" x2="20" y1="19" y2="19" />
                                    </svg>
                                    <span>bash</span>
                                    <svg
                                        width="10"
                                        height="10"
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="6 9 12 15 18 9" />
                                    </svg>
                                </div>
                                <button class="tool-btn" title="添加新标签页">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M5 12h14M12 5v14" />
                                    </svg>
                                </button>
                                <button class="tool-btn" title="分屏显示">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <rect width="18" height="18" x="3" y="3" rx="2" />
                                        <line x1="12" x2="12" y1="3" y2="21" />
                                    </svg>
                                </button>
                                <button class="tool-btn btn-danger" title="终止并清理当前终端">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M3 6h18M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2" />
                                        <line x1="10" x2="10" y1="11" y2="17" />
                                        <line x1="14" x2="14" y1="11" y2="17" />
                                    </svg>
                                </button>
                                <button class="tool-btn" title="更多终端设置">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <circle cx="12" cy="12" r="1" />
                                        <circle cx="19" cy="12" r="1" />
                                        <circle cx="5" cy="12" r="1" />
                                    </svg>
                                </button>
                            </div>
                        </div>

                        {/* Terminal Body wrapped in a floating premium card */}
                        <main class="terminal-card">
                            <Terminal
                                id="terminal-container"
                                wsUrl={wsUrl}
                                tokenUrl={tokenUrl}
                                clientOptions={clientOptions}
                                termOptions={termOptions}
                                flowControl={flowControl}
                            />
                        </main>
                    </div>
                )}

                {activeTab === 'agents' && (
                    <main class="placeholder-view">
                        <svg
                            class="placeholder-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M12 8V4H8" />
                            <rect width="16" height="12" x="4" y="8" rx="2" />
                            <path d="M2 14h2" />
                            <path d="M20 14h2" />
                            <path d="M15 13v2" />
                            <path d="M9 13v2" />
                        </svg>
                        <h3 class="placeholder-title">智能体工作空间</h3>
                        <p class="placeholder-desc">
                            这里是智能体编排与训练控制中心。您可以创建、编辑和编排专门的 AI 智能体（如当前的 AI
                            运维），配置它们的底层工具链和执行逻辑。
                        </p>
                    </main>
                )}

                {activeTab === 'console' && (
                    <main class="placeholder-view">
                        <svg
                            class="placeholder-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect width="20" height="14" x="2" y="5" rx="2" />
                            <line x1="2" x2="22" y1="10" y2="10" />
                        </svg>
                        <h3 class="placeholder-title">控制台与系统监控</h3>
                        <p class="placeholder-desc">
                            实时观测当前智能体的工作负载、网络延时和 CPU
                            占用率。可视化看板正在筹备中，目前核心的控制流可以直接通过 **终端** 页签输入相关命令执行。
                        </p>
                    </main>
                )}

                {activeTab === 'folders' && (
                    <main class="placeholder-view">
                        <svg
                            class="placeholder-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                        </svg>
                        <h3 class="placeholder-title">文件管理器</h3>
                        <p class="placeholder-desc">
                            查看并管理智能体的工作目录和生成产物。支持文件的在线拖拽和拖放上传。您还可以通过在 **终端**
                            内执行相关的文件传输指令（如 `trz/tsz`）进行更底层的交互。
                        </p>
                    </main>
                )}

                {activeTab === 'settings' && (
                    <main class="placeholder-view">
                        <svg
                            class="placeholder-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="1.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <path d="M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.1a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z" />
                        </svg>
                        <h3 class="placeholder-title">系统偏好设置</h3>
                        <p class="placeholder-desc">
                            在此配置终端渲染加速模式（Webgl/Canvas/DOM）、字体大小、默认 shell
                            以及连接重试逻辑。目前支持亮色/深色主题，可在右上角快速切换。
                        </p>
                    </main>
                )}
            </div>
        );
    }
}
