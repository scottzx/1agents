import { h } from 'preact';
import { Terminal } from '../terminal';
import type { ITerminalOptions } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from '../terminal/xterm';
import type { TmuxWindow, Workspace } from '../types';

interface MiddleCanvasProps {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    wsUrl: string;
    tokenUrl: string;
    clientOptions: ClientOptions;
    termOptions: ITerminalOptions;
    flowControl: FlowControl;
    terminalWindows: TmuxWindow[];
    terminalWindowsLoading: boolean;
    activeWorkspaceId: string;
    workspaces: Workspace[];
    onTerminalCreate: (workspaceId: string, cwd: string) => void;
    onTerminalSwitch: (windowIndex: number) => void;
    onTerminalKill: (windowIndex: number) => void;
}

export function MiddleCanvas({
    activeTab,
    wsUrl,
    tokenUrl,
    clientOptions,
    termOptions,
    flowControl,
    terminalWindows,
    activeWorkspaceId,
    workspaces,
    onTerminalCreate,
    onTerminalSwitch,
    onTerminalKill,
}: MiddleCanvasProps) {
    // Find the active workspace to get its path for new terminal creation
    const activeWs = workspaces.find(w => w.id === activeWorkspaceId);

    const handleAddTab = () => {
        if (!activeWorkspaceId) return;
        onTerminalCreate(activeWorkspaceId, activeWs?.path || '');
    };

    const handleKillActive = () => {
        const activeWin = terminalWindows.find(w => w.active);
        if (activeWin) {
            onTerminalKill(activeWin.index);
        }
    };

    return (
        <main class="middle-canvas">
            {/* ── Session bar (tmux windows) ─────────────────────────────────── */}
            <div class="terminal-tab-bar">
                <div class="tab-tabs">
                    {terminalWindows.map(win => (
                        <div
                            key={win.index}
                            class={`tab-item${win.active ? ' tab-active' : ''}`}
                            onClick={() => onTerminalSwitch(win.index)}
                            title={`${win.workspaceId} — 会话 #${win.index}`}
                        >
                            <span class="tab-ws-badge">{win.workspaceId}</span>
                            <span class="tab-num">#{win.index}</span>
                        </div>
                    ))}
                </div>

                <div class="tab-actions">
                    <button
                        class="tab-btn"
                        onClick={handleAddTab}
                        disabled={!activeWorkspaceId}
                        title="在当前工作空间新建会话"
                    >
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                        >
                            <path d="M5 12h14M12 5v14" />
                        </svg>
                    </button>
                    <button
                        class="tab-btn tab-btn-danger"
                        onClick={handleKillActive}
                        disabled={terminalWindows.length <= 1}
                        title="关闭当前活跃会话"
                    >
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                        >
                            <line x1="18" x2="6" y1="6" y2="18" />
                            <line x1="6" x2="18" y1="6" y2="18" />
                        </svg>
                    </button>
                </div>
            </div>

            {/* ── Toolbar ────────────────────────────────────────────────────── */}
            <div class="terminal-toolbar">
                <div class="toolbar-left">
                    <h2 class="page-title">系统主控制终端</h2>
                </div>
                <div class="toolbar-right">
                    <div class="shell-selector" title="当前 Shell: tmux">
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
                        <span>tmux</span>
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
                </div>
            </div>

            {/* ── Terminal canvas ─────────────────────────────────────────────── */}
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
                    <div class="placeholder-view" style="margin: 0; border: none; border-radius: 0; height: 100%;">
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
    );
}
