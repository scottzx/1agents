import { h } from 'preact';
import { useRef } from 'preact/hooks';

interface AttachButtonProps {
    /** Base class(es) so the button matches its host toolbar (mic/plus look). */
    className: string;
    /** Called with the user's picked files; upload is the caller's concern. */
    onSelect: (files: FileList) => void;
    /** True while an upload is in flight — disables re-picking. */
    uploading?: boolean;
    disabled?: boolean;
    title?: string;
    ariaLabel?: string;
}

/**
 * Shared attachment button: a `+` icon that opens the native file picker and
 * forwards the selection. Accepts any file type (no `accept` filter). Used by
 * NewChatHome and Composer so the upload trigger lives in one place.
 */
export function AttachButton({ className, onSelect, uploading, disabled, title, ariaLabel }: AttachButtonProps) {
    const inputRef = useRef<HTMLInputElement | null>(null);

    const handleChange = (e: Event) => {
        const input = e.target as HTMLInputElement;
        if (input.files && input.files.length) onSelect(input.files);
        // Clear so picking the same file again still fires onChange.
        input.value = '';
    };

    return (
        <button
            type="button"
            class={className}
            onClick={() => inputRef.current?.click()}
            disabled={disabled || uploading}
            title={title}
            aria-label={ariaLabel}
        >
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2.5"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
            >
                <line x1="12" y1="5" x2="12" y2="19" />
                <line x1="5" y1="12" x2="19" y2="12" />
            </svg>
            <input
                ref={inputRef}
                type="file"
                multiple
                style="display: none;"
                onChange={handleChange}
            />
        </button>
    );
}
