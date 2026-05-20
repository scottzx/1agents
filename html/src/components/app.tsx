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
    background: '#0d1117',
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

interface WorkspaceFolder {
    id: string;
    name: string;
    expanded: boolean;
    children: Array<{
        id: string;
        title: string;
        time: string;
        active?: boolean;
    }>;
}

interface ProjectFile {
    path: string;
    name: string;
    size: string;
    type: 'file' | 'folder';
    indent: number;
    content: string;
}

const projectFiles: ProjectFile[] = [
    {
        path: 'README.md',
        name: 'README.md',
        size: '5.4 KB',
        type: 'file',
        indent: 0,
        content:
            '# ttyd - Share your terminal over the web\n\nttyd is a simple command-line tool for sharing terminal over the web.\n\n## Features\n- Built on top of libuv and WebGL2 for speed\n- Fully-featured terminal with CJK and IME support\n- ZMODEM / trzsz file transfer support\n- Sixel image output support\n- SSL support based on OpenSSL / Mbed TLS\n- Run any custom command with options',
    },
    {
        path: 'package.json',
        name: 'package.json',
        size: '2.1 KB',
        type: 'file',
        indent: 0,
        content:
            '{\n  "private": true,\n  "name": "ttyd",\n  "version": "1.0.0",\n  "description": "Share your terminal over the web",\n  "scripts": {\n    "start": "webpack serve",\n    "build": "webpack && gulp",\n    "fix": "gts fix"\n  },\n  "dependencies": {\n    "@xterm/xterm": "^5.5.0",\n    "preact": "^10.19.6",\n    "trzsz": "^1.1.5"\n  }\n}',
    },
    {
        path: 'CMakeLists.txt',
        name: 'CMakeLists.txt',
        size: '4.4 KB',
        type: 'file',
        indent: 0,
        content:
            'cmake_minimum_required(VERSION 3.10)\nproject(ttyd C)\n\nset(CMAKE_C_STANDARD 99)\nset(CMAKE_C_STANDARD_REQUIRED ON)\n\nfind_package(Libwebsockets REQUIRED)\nfind_package(Libuv REQUIRED)\nfind_package(OpenSSL REQUIRED)\n\nadd_executable(ttyd src/main.c src/utils.c)\ntarget_link_libraries(ttyd Libwebsockets::websockets Libuv::uv OpenSSL::SSL)',
    },
    {
        path: 'html/src/components',
        name: 'html/src/components',
        size: '',
        type: 'folder',
        indent: 0,
        content: '',
    },
    {
        path: 'html/src/components/app.tsx',
        name: 'app.tsx',
        size: '25.9 KB',
        type: 'file',
        indent: 1,
        content:
            'import { h, Component } from \'preact\';\nimport { Terminal } from \'./terminal\';\n\nexport class App extends Component {\n    render() {\n        return (\n            <div class="app-container">\n                <header class="global-header">...</header>\n                <main class="workspace-body">...</main>\n            </div>\n        );\n    }\n}',
    },
    {
        path: 'html/src/style/index.scss',
        name: 'index.scss',
        size: '8.2 KB',
        type: 'file',
        indent: 1,
        content:
            'html, body {\n  height: 100%;\n  margin: 0;\n  overflow: hidden;\n}\n\n.app-container {\n  display: flex;\n  flex-direction: column;\n  height: 100vh;\n}',
    },
];

type RightDrawerTab = 'files' | 'tasks' | 'settings' | 'none';

interface AppState {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    activeDrawerTab: RightDrawerTab;
    theme: 'light' | 'dark';
    hostname: string;
    leftSidebarOpen: boolean;
    folders: WorkspaceFolder[];
    selectedFile: ProjectFile | null;
}

export class App extends Component<{}, AppState> {
    constructor() {
        super();
        this.state = {
            activeTab: 'terminal',
            activeDrawerTab: 'none', // Collapsed right drawer panel by default
            theme: 'light',
            hostname: 'Ashley Walker',
            leftSidebarOpen: true,
            selectedFile: projectFiles[0],
            folders: [
                {
                    id: 'remote-agents',
                    name: 'remote-agents',
                    expanded: true,
                    children: [
                        { id: 'f-custom', title: 'Frontend Customization G...', time: '2m', active: false },
                        { id: 'a-terminal', title: 'Analyzing Web Terminal...', time: '11m', active: true },
                    ],
                },
                {
                    id: 'bee-write-back',
                    name: 'bee-write-back',
                    expanded: false,
                    children: [{ id: 'a-bee', title: 'Analyzing Bee Write Back', time: '3h' }],
                },
                {
                    id: 'cc-connect',
                    name: 'cc-connect',
                    expanded: false,
                    children: [{ id: 'a-cc', title: '帮我分析一下这个项目。理...', time: '4h' }],
                },
                {
                    id: 'html-slides',
                    name: 'html-slides',
                    expanded: false,
                    children: [
                        { id: 'a-slide-1', title: 'Designing Agent Collabor...', time: '18h' },
                        { id: 'a-slide-2', title: 'Designing Agent Collabor...', time: '18h' },
                    ],
                },
                {
                    id: 'html-anything',
                    name: 'html-anything',
                    expanded: false,
                    children: [{ id: 'a-anything', title: 'Querying LLM Usage', time: '1d' }],
                },
            ],
        };
    }

    componentDidMount() {
        const savedTheme = localStorage.getItem('remote-agents-theme') as 'light' | 'dark' | null;
        const theme = savedTheme || 'light';
        this.setState({ theme });
        document.documentElement.setAttribute('data-theme', theme);
        this.setState({ hostname: window.location.hostname || 'Ashley Walker' });
    }

    toggleTheme = (themeMode?: 'light' | 'dark') => {
        const targetTheme = themeMode || (this.state.theme === 'light' ? 'dark' : 'light');
        this.setState({ theme: targetTheme });
        document.documentElement.setAttribute('data-theme', targetTheme);
        localStorage.setItem('remote-agents-theme', targetTheme);
        this.triggerTerminalFit();
    };

    triggerTerminalFit = () => {
        setTimeout(() => {
            const term = (window as unknown as { term?: { fit?: () => void } }).term;
            if (term && term.fit) {
                term.fit();
            }
        }, 150);
    };

    setActiveTab = (tab: 'terminal' | 'agents' | 'console' | 'folders') => {
        this.setState({ activeTab: tab });
        this.triggerTerminalFit();
    };

    // Coze click shortcut toggle dynamic drawer logic
    toggleDrawerTab = (tab: RightDrawerTab) => {
        if (this.state.activeDrawerTab === tab) {
            this.setState({ activeDrawerTab: 'none' });
        } else {
            this.setState({ activeDrawerTab: tab });
        }
        this.triggerTerminalFit();
    };

    toggleLeftSidebar = () => {
        this.setState({ leftSidebarOpen: !this.state.leftSidebarOpen });
        this.triggerTerminalFit();
    };

    toggleFolder = (folderId: string) => {
        this.setState({
            folders: this.state.folders.map(f => (f.id === folderId ? { ...f, expanded: !f.expanded } : f)),
        });
    };

    selectFile = (file: ProjectFile) => {
        if (file.type === 'file') {
            this.setState({ selectedFile: file });
        }
    };

    renderHighlightedCode(content: string) {
        const lines = content.split('\n');
        return lines.map((line, idx) => {
            const renderedText: Array<h.JSX.Element | string> = [];
            const parts = line.split(/(\s+)/);
            parts.forEach((part, pIdx) => {
                if (
                    /^(import|export|class|const|return|function|public|private|type|interface|void|async|await|let|var|set)$/.test(
                        part
                    )
                ) {
                    renderedText.push(
                        <span key={pIdx} class="kw">
                            {part}
                        </span>
                    );
                } else if (/^("[^"]*"|'[^']*'|`[^`]*`)$/.test(part)) {
                    renderedText.push(
                        <span key={pIdx} class="str">
                            {part}
                        </span>
                    );
                } else if (/^\/\/.*$/.test(part) || /^\/\*.*$/.test(part) || /^#.*$/.test(part)) {
                    renderedText.push(
                        <span key={pIdx} class="cm">
                            {part}
                        </span>
                    );
                } else if (/^(<[^>]+>)$/.test(part)) {
                    renderedText.push(
                        <span key={pIdx} class="tag">
                            {part}
                        </span>
                    );
                } else {
                    renderedText.push(part);
                }
            });

            return (
                <div key={idx} class="code-line">
                    <span class="line-number">{idx + 1}</span>
                    <span class="line-text">{renderedText}</span>
                </div>
            );
        });
    }

    renderDrawerTitle(tab: RightDrawerTab) {
        switch (tab) {
            case 'files':
                return '文件浏览器 (Files)';
            case 'tasks':
                return '任务调试看板 (Tasks)';
            case 'settings':
                return '系统终端设置 (Settings)';
            default:
                return '';
        }
    }

    render() {
        const { activeTab, activeDrawerTab, theme, hostname, leftSidebarOpen, folders, selectedFile } = this.state;
        const currentTheme = theme === 'light' ? lightTermTheme : darkTermTheme;
        const termOptions = {
            ...baseTermOptions,
            theme: currentTheme,
        } as ITerminalOptions;

        return (
            <div class="app-container">
                {/* [COLUMN 1]: LEFT Workspaces Tree Sidebar (直通顶部 100vh) */}
                <aside class={`left-sidebar ${leftSidebarOpen ? '' : 'collapsed'}`}>
                    <div class="sidebar-header">
                        <div class="coze-brand">
                            <div class="brand-left">
                                <div class="brand-icon">扣</div>
                                <span>扣子终端</span>
                            </div>
                            <div class="sidebar-close-btn" onClick={this.toggleLeftSidebar} title="折叠侧边栏">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="15 18 9 12 15 6" />
                                </svg>
                            </div>
                        </div>

                        <button class="new-conv-btn">
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
                            <span>新建会话</span>
                        </button>

                        <div class="history-title-container">
                            <span>历史会话</span>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="10" />
                                <polyline points="12 6 12 12 16 14" />
                            </svg>
                        </div>
                    </div>

                    <div class="sidebar-scroll">
                        <div class="workspace-section">
                            <div class="section-header">
                                <span>工作空间 Workspaces</span>
                                <div class="header-actions">
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <path d="M3 16h10M3 12h18M3 8h18" />
                                    </svg>
                                </div>
                            </div>

                            {folders.map(folder => (
                                <div key={folder.id} class="project-node">
                                    <div
                                        class={`project-folder ${folder.expanded ? 'expanded' : ''}`}
                                        onClick={() => this.toggleFolder(folder.id)}
                                    >
                                        <svg
                                            class="chevron"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="9 18 15 12 9 6" />
                                        </svg>
                                        <svg
                                            class="folder-icon"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                        </svg>
                                        <span>{folder.name}</span>
                                    </div>

                                    {folder.expanded && (
                                        <div class="project-children">
                                            {folder.children.map(child => (
                                                <div key={child.id} class={`chat-item ${child.active ? 'active' : ''}`}>
                                                    <span class="chat-title" title={child.title}>
                                                        {child.title}
                                                    </span>
                                                    <span class="chat-time">{child.time}</span>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    </div>

                    <div class="sidebar-footer">
                        <div class="footer-item" onClick={() => this.toggleDrawerTab('settings')}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="3" />
                                <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                            </svg>
                            <span>Settings</span>
                        </div>
                        <div class="footer-item">
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
                            <span>Feedback</span>
                        </div>
                    </div>
                </aside>

                {/* [WORKSPACE MAIN CONTENT]: Occupies rest of screen */}
                <div class="workspace-main-content">
                    {/* [COZE PAGE HEADER]: Replaces top global header */}
                    <header class="workspace-header">
                        <div class="header-left">
                            {!leftSidebarOpen && (
                                <button
                                    class="sidebar-toggle-btn"
                                    onClick={this.toggleLeftSidebar}
                                    style="margin-right: 4px;"
                                    title="展开左侧栏"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <polyline points="9 18 15 12 9 6" />
                                    </svg>
                                </button>
                            )}
                            <div class="header-title-group">
                                <span class="title">{hostname}的智能体</span>
                                <div class="status-indicator">
                                    <div class="pulse-dot" />
                                    <span>运行中</span>
                                </div>
                            </div>
                        </div>

                        {/* Coze right shortcut buttons in red box */}
                        <div class="header-right">
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'files' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('files')}
                                title="文件浏览器"
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
                            </button>
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'tasks' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('tasks')}
                                title="任务追踪与调试"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 22c5.523 0 10-4.477 10-10S17.523 2 12 2 2 6.477 2 12s4.477 10 10 10z" />
                                    <path d="m9 12 2 2 4-4" />
                                </svg>
                            </button>
                            <button
                                class={`shortcut-btn ${activeDrawerTab === 'settings' ? 'active' : ''}`}
                                onClick={() => this.toggleDrawerTab('settings')}
                                title="系统设置"
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="3" />
                                    <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                                </svg>
                            </button>
                            <div class="divider" />
                            <button
                                class="shortcut-btn"
                                onClick={() => this.toggleTheme()}
                                title={theme === 'light' ? '深色主题' : '浅色主题'}
                            >
                                {theme === 'light' ? (
                                    <svg
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
                        </div>
                    </header>

                    {/* [WORKSPACE BODY CONTAINER]: terminal & drawers */}
                    <div class="workspace-body-container">
                        {/* [COLUMN 2]: MIDDLE main workspace Terminal container */}
                        <main class="middle-canvas">
                            <div class="terminal-toolbar">
                                <div class="toolbar-left">
                                    <h2 class="page-title">系统主控制终端</h2>
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
                                </div>
                            </div>

                            {/* Card wrapper containing the actual Web terminal canvas */}
                            <div class="terminal-card">
                                {activeTab === 'terminal' ? (
                                    <Terminal
                                        id="terminal-container"
                                        wsUrl={wsUrl}
                                        tokenUrl={tokenUrl}
                                        clientOptions={clientOptions}
                                        termOptions={termOptions}
                                        flowControl={flowControl}
                                    />
                                ) : (
                                    <div
                                        class="placeholder-view"
                                        style="margin: 0; border: none; border-radius: 0; height: 100%;"
                                    >
                                        <svg
                                            class="placeholder-icon"
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="1.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <rect width="20" height="16" x="2" y="4" rx="2" />
                                            <path d="m7 8 3 2-3 2" />
                                            <path d="M12 12h4" />
                                        </svg>
                                        <h3 class="placeholder-title">终端就绪</h3>
                                        <p class="placeholder-desc">在全局导航栏中点击【终端】以开始交互会话。</p>
                                    </div>
                                )}
                            </div>
                        </main>

                        {/* [COLUMN 3]: RIGHT side dynamic sliding drawer panel */}
                        <aside class={`right-panel ${activeDrawerTab === 'none' ? 'collapsed' : ''}`}>
                            <div class="panel-tabs-header">
                                <span class="panel-tab-title">{this.renderDrawerTitle(activeDrawerTab)}</span>
                                <div
                                    class="panel-close-btn"
                                    onClick={() => this.setState({ activeDrawerTab: 'none' })}
                                    title="收起面板"
                                >
                                    <svg
                                        viewBox="0 0 24 24"
                                        fill="none"
                                        stroke="currentColor"
                                        stroke-width="2.5"
                                        stroke-linecap="round"
                                        stroke-linejoin="round"
                                    >
                                        <line x1="18" x2="6" y1="6" y2="18" />
                                        <line x1="6" x2="18" y1="6" y2="18" />
                                    </svg>
                                </div>
                            </div>

                            <div class="panel-body-scroll">
                                {activeDrawerTab === 'files' && (
                                    <div style="display: flex; flex-direction: column; gap: 16px;">
                                        <div class="file-tree-container">
                                            {projectFiles.map(file => (
                                                <div
                                                    key={file.path}
                                                    class={`file-node indent-${file.indent} ${
                                                        selectedFile?.path === file.path ? 'active' : ''
                                                    }`}
                                                    onClick={() => this.selectFile(file)}
                                                >
                                                    {file.type === 'folder' ? (
                                                        <svg
                                                            class="folder-icon"
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                                        </svg>
                                                    ) : (
                                                        <svg
                                                            class="file-icon"
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <path d="M15 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V7Z" />
                                                            <path d="M14 2v4a2 2 0 0 0 2 2h4" />
                                                        </svg>
                                                    )}
                                                    <span class="file-name">{file.name}</span>
                                                    {file.size && <span class="file-size">{file.size}</span>}
                                                </div>
                                            ))}
                                        </div>

                                        {selectedFile && (
                                            <div class="code-preview-card">
                                                <div class="preview-header">
                                                    <span class="preview-title">{selectedFile.path}</span>
                                                    <div
                                                        class="preview-close"
                                                        onClick={() => this.setState({ selectedFile: null })}
                                                    >
                                                        <svg
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <line x1="18" x2="6" y1="6" y2="18" />
                                                            <line x1="6" x2="18" y1="6" y2="18" />
                                                        </svg>
                                                    </div>
                                                </div>
                                                <pre class="preview-content">
                                                    {this.renderHighlightedCode(selectedFile.content)}
                                                </pre>
                                            </div>
                                        )}
                                    </div>
                                )}

                                {activeDrawerTab === 'tasks' && (
                                    <div class="task-list-container">
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>移除了顶部全局导航栏以呈现 Coze 极简风格</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>整合会话头部标题栏，引入运行中动态绿色脉冲灯</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>引入 Coze 右上角快捷功能按钮栏 (文件树、任务控制、系统设置)</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>实现右侧滑出式抽屉面板 (Quick Drawer System) 及其缓动过渡</span>
                                        </div>
                                        <div class="task-item completed">
                                            <svg
                                                class="check-icon"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="12" cy="12" r="10" />
                                                <polyline points="12 8 12 12 14 14" />
                                            </svg>
                                            <span>完全兼容并无损保留移动端快捷同步键盘及输入面板</span>
                                        </div>
                                    </div>
                                )}

                                {activeDrawerTab === 'settings' && (
                                    <div class="settings-container">
                                        <div class="setting-group">
                                            <span class="setting-label">色彩主题样式 (Color Theme)</span>
                                            <div class="theme-options">
                                                <button
                                                    class={`theme-btn ${theme === 'light' ? 'active' : ''}`}
                                                    onClick={() => this.toggleTheme('light')}
                                                >
                                                    <svg
                                                        width="12"
                                                        height="12"
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
                                                    <span>浅色模式</span>
                                                </button>
                                                <button
                                                    class={`theme-btn ${theme === 'dark' ? 'active' : ''}`}
                                                    onClick={() => this.toggleTheme('dark')}
                                                >
                                                    <svg
                                                        width="12"
                                                        height="12"
                                                        viewBox="0 0 24 24"
                                                        fill="none"
                                                        stroke="currentColor"
                                                        stroke-width="2"
                                                        stroke-linecap="round"
                                                        stroke-linejoin="round"
                                                    >
                                                        <path d="M12 3a6 6 0 0 0 9 9 9 9 0 1 1-9-9Z" />
                                                    </svg>
                                                    <span>深色模式</span>
                                                </button>
                                            </div>
                                        </div>
                                    </div>
                                )}
                            </div>
                        </aside>
                    </div>
                </div>
            </div>
        );
    }
}
