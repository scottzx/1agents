import { h } from 'preact';
import type { FileAttachment } from '../../../hooks/useFileAttachments';

interface AttachmentPreviewProps {
    attachments: FileAttachment[];
    onRemove: (att: FileAttachment) => void;
}

/**
 * Chip strip shown above a chat input. Each uploaded file renders as a chip:
 * an image thumbnail (or a generic file icon) + the original name + a ✕ that
 * removes the chip and strips its path from the input text. Renders nothing
 * when there are no attachments.
 */
export function AttachmentPreview({ attachments, onRemove }: AttachmentPreviewProps) {
    if (!attachments.length) return null;
    return (
        <div class="composer-attachment-strip">
            {attachments.map(att => (
                <div class="composer-attachment-chip" key={att.path} title={att.path}>
                    {att.isImage && att.previewUrl ? (
                        <img class="composer-attachment-thumb" src={att.previewUrl} alt={att.name} />
                    ) : (
                        <svg
                            class="composer-attachment-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            aria-hidden="true"
                        >
                            <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                            <polyline points="14 2 14 8 20 8" />
                        </svg>
                    )}
                    <span class="composer-attachment-name">{att.name}</span>
                    <button
                        type="button"
                        class="composer-attachment-remove"
                        onClick={() => onRemove(att)}
                        title="移除"
                        aria-label="移除附件"
                    >
                        <svg
                            viewBox="0 0 24 24"
                            width="12"
                            height="12"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            aria-hidden="true"
                        >
                            <line x1="18" y1="6" x2="6" y2="18" />
                            <line x1="6" y1="6" x2="18" y2="18" />
                        </svg>
                    </button>
                </div>
            ))}
        </div>
    );
}
