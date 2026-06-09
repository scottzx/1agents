import { h } from 'preact';
import { useRef } from 'preact/hooks';

interface ComposerProps {
    onSend: (text: string) => void;
    disabled?: boolean;
    placeholder?: string;
}

export function Composer({ onSend, disabled, placeholder }: ComposerProps) {
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
        el.style.height = Math.min(el.scrollHeight, 200) + 'px';
    };

    return (
        <div class="chat-composer">
            <textarea
                ref={ref}
                class="chat-composer-input"
                placeholder={placeholder ?? '输入消息，Enter 发送，Shift+Enter 换行'}
                disabled={disabled}
                onKeyDown={handleKeyDown}
                onInput={handleInput}
                rows={1}
            />
            <button class="chat-composer-send" onClick={submit} disabled={disabled} title="发送 (Enter)">
                发送
            </button>
        </div>
    );
}
