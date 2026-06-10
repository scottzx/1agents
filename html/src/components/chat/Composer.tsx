import { h } from 'preact';
import { useRef } from 'preact/hooks';

interface ComposerProps {
    onSend: (text: string) => void;
    onCancel?: () => void;
    isRunning?: boolean;
    disabled?: boolean;
    placeholder?: string;
}

export function Composer({ onSend, onCancel, isRunning, disabled, placeholder }: ComposerProps) {
    const ref = useRef<HTMLTextAreaElement | null>(null);

    const handleKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            submit();
        }
    };

    const submit = () => {
        const el = ref.current;
        if (!el) return;
        const text = el.value.trim();
        if (!text) return;
        onSend(text);
        el.value = '';
        // Reset height
        el.style.height = 'auto';
    };

    const handleInput = () => {
        const el = ref.current;
        if (!el) return;
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 320) + 'px';
    };

    return (
        <div class="chat-composer">
            <textarea
                ref={ref}
                class="chat-composer-input"
                placeholder={placeholder ?? '输入消息，支持 Markdown，Enter 发送，Shift+Enter 换行'}
                disabled={disabled}
                onKeyDown={handleKeyDown}
                onInput={handleInput}
                rows={1}
                wrap="soft"
            />
            {isRunning ? (
                <button class="chat-composer-stop" onClick={onCancel} title="停止">
                    停止
                </button>
            ) : (
                <button class="chat-composer-send" onClick={submit} disabled={disabled} title="发送 (Enter)">
                    发送
                </button>
            )}
        </div>
    );
}
