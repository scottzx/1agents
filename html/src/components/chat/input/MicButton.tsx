import { h } from 'preact';

interface MicButtonProps {
    /** Base class(es) for the site; decides the non-recording look. */
    className: string;
    /** Active recording state — appends `recording` (red pulse styling). */
    recording: boolean;
    onClick: () => void;
    title?: string;
    ariaLabel?: string;
    disabled?: boolean;
}

/**
 * Shared voice-input button. Renders the standard mic icon + recording-state
 * class so the three inputs (NewChatHome, Composer, terminal panel) don't each
 * carry a copy of the SVG. Visibility gating (IS_DESKTOP / speech.available /
 * isHttps) stays with each caller — this only renders the button.
 */
export function MicButton({ className, recording, onClick, title, ariaLabel, disabled }: MicButtonProps) {
    return (
        <button
            type="button"
            class={`${className} ${recording ? 'recording' : ''}`}
            onClick={onClick}
            title={title}
            aria-label={ariaLabel}
            disabled={disabled}
        >
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="2"
                stroke-linecap="round"
                stroke-linejoin="round"
                aria-hidden="true"
            >
                <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
                <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
                <line x1="12" y1="19" x2="12" y2="23" />
                <line x1="8" y1="23" x2="16" y2="23" />
            </svg>
        </button>
    );
}
