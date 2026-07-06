import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, type Lang } from '../../i18n';
import { skillService, type SkillPushPreview, type SkillDiffFile } from '@1agents/core/services/skillService';

interface PushPreviewModalProps {
    preview: SkillPushPreview;
    workspaceId: string;
    skillRef: string;
    onClose: () => void;
    onDone: (result: 'created' | 'main' | 'fork') => void;
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
 * Push-preview dialog (issue #379 follow-up): shown on every "推送到母体" click
 * instead of pushing immediately. Renders the read-only preview's diff so the
 * user can see exactly what changed before choosing to update 母体 in place,
 * fork it, or (first-time) just add it.
 */
export function PushPreviewModal({ preview, workspaceId, skillRef, onClose, onDone, language }: PushPreviewModalProps) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');

    const { isNew, diverged, files } = preview;

    const onCreate = async () => {
        setBusy(true);
        setError('');
        try {
            await skillService.pushSkill(workspaceId, skillRef);
            onDone('created');
        } catch {
            setError(t('assistant.push.previewFailed', language));
        } finally {
            setBusy(false);
        }
    };

    const onPush = async () => {
        setBusy(true);
        setError('');
        try {
            const res = await skillService.pushSkill(workspaceId, skillRef);
            if (res.status === 'conflict') {
                setError(t('assistant.push.conflictStaged', language));
            } else {
                onDone('main');
            }
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
                    {diverged && preview.target && (
                        <p class="push-preview-banner">
                            {t('assistant.push.divergedBanner', language, {
                                storeVersion: preview.target.storeVersion,
                                baseVersion: preview.target.baseVersion,
                            })}
                        </p>
                    )}
                    <div class="push-preview-diff">
                        {(!files || files.length === 0) && (
                            <p class="push-preview-empty">{t('assistant.push.noChange', language)}</p>
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
                    {isNew ? (
                        <button class="ws-modal-confirm" onClick={() => void onCreate()} disabled={busy}>
                            {t('assistant.push.addToStore', language)}
                        </button>
                    ) : (
                        <button class="ws-modal-confirm" onClick={() => void onPush()} disabled={busy}>
                            {t('assistant.push.submitCopy', language)}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
