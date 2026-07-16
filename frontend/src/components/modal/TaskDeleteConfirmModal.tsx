import { h } from 'preact';
import { t, type Lang } from '../../i18n';

interface TaskDeleteConfirmModalProps {
    title?: string;
    onClose: () => void;
    onSubmit: () => void;
    language: Lang;
}

// Confirmation dialog used by both TaskTable.renderActions and TaskDetail's
// danger-zone delete button. Cancel is a no-op; submit forwards to the parent's
// actual delete call. All copy is i18n'd.
export function TaskDeleteConfirmModal({ title, onClose, onSubmit, language }: TaskDeleteConfirmModalProps) {
    return (
        <div class="ws-modal-overlay" onClick={onClose}>
            <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>{title || t('task.table.deleteConfirmTitle', language)}</span>
                    <button class="ws-modal-close" onClick={onClose}>
                        ✕
                    </button>
                </div>
                <div
                    class="ws-modal-body"
                    style="font-size: 13.5px; line-height: 1.6; color: var(--text-main); padding: 16px 20px;"
                >
                    {t('task.table.deleteConfirmMessage', language)}
                </div>
                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose}>
                        {t('common.cancel', language)}
                    </button>
                    <button class="ws-modal-confirm ws-modal-confirm-danger" onClick={onSubmit}>
                        {t('common.delete', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
