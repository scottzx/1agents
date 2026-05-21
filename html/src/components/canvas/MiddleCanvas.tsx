import { h } from 'preact';
import { Terminal } from '../terminal';
import type { ITerminalOptions } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from '../terminal/xterm';

interface MiddleCanvasProps {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    wsUrl: string;
    tokenUrl: string;
    clientOptions: ClientOptions;
    termOptions: ITerminalOptions;
    flowControl: FlowControl;
    onMobileDetect?: (isMobile: boolean) => void;
    tmuxMouseOn: boolean;
    onTmuxMouseToggle: () => void;
}

export function MiddleCanvas({
    activeTab,
    wsUrl,
    tokenUrl,
    clientOptions,
    termOptions,
    flowControl,
    onMobileDetect,
    tmuxMouseOn,
    onTmuxMouseToggle,
}: MiddleCanvasProps) {
    return (
        <main class="middle-canvas">
            {/* ── Toolbar ────────────────────────────────────────────────────── */}
            <div class="terminal-toolbar">
                <div class="toolbar-left">
                    <h2 class="page-title">系统主控制终端</h2>
                </div>
                <div class="toolbar-right">
                    <button
                        class={`tmux-mouse-toggle ${tmuxMouseOn ? 'active' : ''}`}
                        onClick={onTmuxMouseToggle}
                        title={
                            tmuxMouseOn
                                ? '当前模式：滚轮滑动（点击切换为选择复制）'
                                : '当前模式：选择复制（点击切换为滚轮滑动）'
                        }
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect x="5" y="2" width="14" height="20" rx="7" />
                            <path d="M12 2v6" />
                            <path d="M5 10h14" />
                        </svg>
                        <span>{tmuxMouseOn ? '滚轮滑动' : '选择复制'}</span>
                    </button>

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
                        onMobileDetect={onMobileDetect}
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
