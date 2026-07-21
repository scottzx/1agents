import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, type Lang } from '../../i18n';
import { skillService, type SkillPushPreview, type SkillDiffFile } from '@1agents/core/services/skillService';

interface PushPreviewModalProps {
    preview: SkillPushPreview;
    workspaceId: string;
    skillRef: string;
    onClose: () => void;
    onDone: (result: 'submitted' | 'unchanged') => void;
    language: Lang;
}

const STATUS_KEY: Record<SkillDiffFile['status'], string> = {
    added: 'assistant.push.statusAdded',
    removed: 'assistant.push.statusRemoved',
    modified: 'assistant.push.statusModified',
};

/** Maps one unified-diff line to the class that colors it. */
function diffLineClass(line: string): string {
    if (line.startsWith('+++') || line.startsWith('---')) return 'skill-diff-line--meta';
    if (line.startsWith('@@')) return 'skill-diff-line--hunk';
    if (line.startsWith('+')) return 'skill-diff-line--add';
    if (line.startsWith('-')) return 'skill-diff-line--del';
    return 'skill-diff-line--ctx';
}

function DiffBlock({ diff }: { diff: string }) {
    const lines = diff.split('\n');
    return (
        <pre class="skill-diff-pre">
            {lines.map((line, i) => (
                <span key={i} class={`skill-diff-line ${diffLineClass(line)}`}>
                    {line || ' '}
                    {'\n'}
                </span>
            ))}
        </pre>
    );
}

/**
 * Push-preview dialog: project side only *submits* a snapshot to Skills Manager.
 * Adoption (create / update / conflict resolve) happens in the manager inbox —
 * this modal never decides whether the skill enters the shared store.
 */
export function PushPreviewModal({ preview, workspaceId, skillRef, onClose, onDone, language }: PushPreviewModalProps) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const { isNew, diverged, files } = preview;
    const hasChanges = Boolean(isNew || (files && files.length > 0) || diverged);

    const onSubmit = async () => {
        setBusy(true);
        setError('');
        try {
            const res = await skillService.pushSkill(workspaceId, skillRef);
            if (res.status === 'exists') {
                onDone('unchanged');
                return;
            }
            // pending (create / update / conflict) — staged for manager adoption
            onDone('submitted');
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
                    <span>{t('assistant.push.previewTitle', language)}</span>
                    <button class="ws-modal-close" onClick={onClose}>
                        ✕
                    </button>
                </div>
                <div class="ws-modal-body">
                    <p class="push-preview-hint">{t('assistant.push.submitHint', language)}</p>
                    {diverged && preview.target && (
                        <p class="push-preview-banner">
                            {t('assistant.push.divergedBanner', language, {
                                storeVersion: preview.target.storeVersion,
                                baseVersion: preview.target.baseVersion,
                            })}
                        </p>
                    )}
                    <div class="push-preview-diff">
                        {(!files || files.length === 0) && !isNew && (
                            <p class="push-preview-empty">{t('assistant.push.noChange', language)}</p>
                        )}
                        {isNew && (!files || files.length === 0) && (
                            <p class="push-preview-empty">{t('assistant.push.newSkillHint', language)}</p>
                        )}
                        {files && files.length > 0 && (
                            <div class="skill-diff-files">
                                {files.map(f => (
                                    <div key={f.path} class="skill-diff-file">
                                        <div class="skill-diff-file-head">
                                            <span class="skill-diff-file-path">{f.path}</span>
                                            <span class={`skill-diff-status-chip is-${f.status}`}>
                                                {t(STATUS_KEY[f.status], language)}
                                            </span>
                                        </div>
                                        <DiffBlock diff={f.diff} />
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>
                    {error && <p class="skill-conflict-error">{error}</p>}
                </div>
                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose} disabled={busy}>
                        {t('assistant.push.cancel', language)}
                    </button>
                    <button class="ws-modal-confirm" onClick={() => void onSubmit()} disabled={busy || !hasChanges}>
                        {t('assistant.push.submitToManager', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
