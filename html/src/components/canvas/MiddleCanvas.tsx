import { h } from 'preact';
import { Terminal } from '../terminal';
import type { ITerminalOptions } from '@xterm/xterm';
import type { ClientOptions, FlowControl } from '../terminal/xterm';
import { t, type Lang } from '../i18n';
import type { ChatSession } from '../types';
import { ChatPanel } from '../chat/ChatPanel';

interface MiddleCanvasProps {
    activeTab: 'terminal' | 'agents' | 'console' | 'folders';
    wsUrl: string;
    tokenUrl: string;
    clientOptions: ClientOptions;
    termOptions: ITerminalOptions;
    flowControl: FlowControl;
    isMobile: boolean;
    onMobileDetect?: (isMobile: boolean) => void;
    onKeyboardStateChange?: (visible: boolean) => void;
    tmuxMouseOn?: boolean;
    onTmuxMouseToggle?: () => void;
    language: Lang;
    /** The currently-active chat session (when activeTab === 'agents'). */
    activeChatSession?: ChatSession | null;
    pendingInitialMessage?: string | null;
    onClearPendingInitialMessage?: () => void;
}

export function MiddleCanvas({
    activeTab,
    wsUrl,
    tokenUrl,
    clientOptions,
    termOptions,
    flowControl,
    isMobile,
    onMobileDetect,
    onKeyboardStateChange,
    tmuxMouseOn,
    onTmuxMouseToggle,
    language,
    activeChatSession,
    pendingInitialMessage,
    onClearPendingInitialMessage,
}: MiddleCanvasProps) {
    return (
        <main class="middle-canvas">
            {/* ── Terminal / agents canvas ────────────────────────────────────── */}
            <div class="terminal-card">
                {activeTab === 'terminal' ? (
                    <Terminal
                        id="terminal-container"
                        wsUrl={wsUrl}
                        tokenUrl={tokenUrl}
                        clientOptions={clientOptions}
                        termOptions={termOptions}
                        flowControl={flowControl}
                        isMobile={isMobile}
                        onMobileDetect={onMobileDetect}
                        onKeyboardStateChange={onKeyboardStateChange}
                        tmuxMouseOn={tmuxMouseOn}
                        onTmuxMouseToggle={onTmuxMouseToggle}
                        language={language}
                    />
                ) : activeTab === 'agents' ? (
                    activeChatSession ? (
                        <ChatPanel
                            session={activeChatSession}
                            pendingInitialMessage={pendingInitialMessage}
                            onClearPendingInitialMessage={onClearPendingInitialMessage}
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
                                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                            </svg>
                            <h3 class="placeholder-title">选择一个聊天会话</h3>
                            <p class="placeholder-desc">点击左侧工作空间旁的 +，选择"新建聊天"以开始一个会话。</p>
                        </div>
                    )
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
                        <h3 class="placeholder-title">{t('canvas.terminalReady', language)}</h3>
                        <p class="placeholder-desc">{t('canvas.terminalReadyDesc', language)}</p>
                    </div>
                )}
            </div>
        </main>
    );
}
