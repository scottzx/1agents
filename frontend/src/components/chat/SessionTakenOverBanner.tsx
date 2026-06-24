// Banner shown when this session was taken over by a newer connection
// (another tab/browser opened the same session). The taken-over tab stops
// auto-reconnecting — the user either dismisses this banner or hits 重试 to
// reclaim ownership (which hands the banner to the other tab instead).

import { h } from 'preact';

export interface SessionTakenOverBannerProps {
    onRetry: () => void;
    onDismiss: () => void;
}

export function SessionTakenOverBanner({ onRetry, onDismiss }: SessionTakenOverBannerProps) {
    return (
        <div class="chat-session-takeover-banner" role="alert" aria-live="assertive">
            <span class="chat-session-takeover-banner__icon" aria-hidden="true">
                ⚠
            </span>
            <span class="chat-session-takeover-banner__text">会话已在其他窗口打开,此处连接已断开。</span>
            <button type="button" class="chat-session-takeover-banner__action" onClick={onRetry}>
                重试
            </button>
            <button
                type="button"
                class="chat-session-takeover-banner__dismiss"
                onClick={onDismiss}
                aria-label="关闭提示"
            >
                ×
            </button>
        </div>
    );
}
