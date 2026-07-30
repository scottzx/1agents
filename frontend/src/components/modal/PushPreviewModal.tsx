import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, type Lang } from '../../i18n';
import { skillService, type SkillReindexPreview } from '@1agents/core/services/skillService';

interface PushPreviewModalProps {
    preview: SkillReindexPreview;
    workspaceId: string;
    extensionId: string;
    onClose: () => void;
    onDone: (result: 'indexed') => void;
    language: Lang;
}

/**
 * Confirms an in-place HarnessKit reindex. This intentionally has no
 * create/update/fork choices: project files remain the source of truth.
 */
export function PushPreviewModal({
    preview,
    workspaceId,
    extensionId,
    onClose,
    onDone,
    language,
}: PushPreviewModalProps) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const onSubmit = async () => {
        setBusy(true);
        setError('');
        try {
            await skillService.reindexSkill(workspaceId, extensionId);
            onDone('indexed');
        } catch {
            setError(t('assistant.push.previewFailed', language));
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="ws-modal-overlay" onClick={onClose}>
            <div class="ws-modal push-preview-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>{t('assistant.push.reindexTitle', language)}</span>
                    <button class="ws-modal-close" onClick={onClose}>
                        ✕
                    </button>
                </div>
                <div class="ws-modal-body">
                    <p class="push-preview-hint">{t('assistant.push.reindexHint', language, { name: preview.name })}</p>
                    <p class="push-preview-empty">{t('assistant.push.inPlaceMeaning', language)}</p>
                    {error && <p class="skill-conflict-error">{error}</p>}
                </div>
                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose} disabled={busy}>
                        {t('assistant.push.cancel', language)}
                    </button>
                    <button class="ws-modal-confirm" onClick={() => void onSubmit()} disabled={busy}>
                        {busy ? t('assistant.detail.indexing', language) : t('assistant.detail.reindex', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
