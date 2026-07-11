// Banner shown above the chat Composer when the bridge reports an error
// (e.g. auth failure, upstream API 4xx). Page-persistent: the banner survives
// reconnects and re-renders, and is only cleared by the × button or a full
// page reload, per spec. Newer errors replace older ones — never stack.

import { h } from 'preact';

export interface ChatErrorBannerProps {
    message: string;
    code?: string;
    onDismiss: () => void;
}

export function ChatErrorBanner({ message, code, onDismiss }: ChatErrorBannerProps) {
    return (
        <div class="chat-error-banner" role="alert" aria-live="assertive">
            <span class="chat-error-banner__icon" aria-hidden="true">
                ⚠
            </span>
            <span class="chat-error-banner__text">
                {code ? <span class="chat-error-banner__code">[{code}]</span> : null} {message}
            </span>
            <button
                type="button"
                class="chat-error-banner__dismiss"
                onClick={onDismiss}
                aria-label="关闭提示"
                title="关闭"
            >
                ×
            </button>
        </div>
    );
}
