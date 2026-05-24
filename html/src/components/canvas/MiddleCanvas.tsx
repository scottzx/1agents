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
    onKeyboardStateChange?: (visible: boolean) => void;
}

export function MiddleCanvas({
    activeTab,
    wsUrl,
    tokenUrl,
    clientOptions,
    termOptions,
    flowControl,
    onMobileDetect,
    onKeyboardStateChange,
}: MiddleCanvasProps) {
    return (
        <main class="middle-canvas">
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
                        onKeyboardStateChange={onKeyboardStateChange}
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
