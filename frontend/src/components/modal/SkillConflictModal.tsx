import { h } from 'preact';
import { useState } from 'preact/hooks';
import { t, type Lang } from '../../i18n';
import { skillService, type SkillPushConflict } from '@1agents/core/services/skillService';

interface SkillConflictModalProps {
    conflict: SkillPushConflict;
    onClose: () => void;
    onResolved: (resolution: 'main' | 'fork') => void;
    language: Lang;
}

/**
 * Concurrent-edit conflict dialog (issue #379): shown when a workspace's skill
 * push lands on a store package that moved past the workspace's base version.
 * Nothing was written yet — the user picks whether their push becomes a new
 * fork (default, keeps both) or the new main (demoting the store's current
 * version to a fork), then we call resolvePush to actually land it.
 */
export function SkillConflictModal({ conflict, onClose, onResolved, language }: SkillConflictModalProps) {
    const [resolution, setResolution] = useState<'fork' | 'main'>('fork');
    const [forkName, setForkName] = useState('');
    const [resolving, setResolving] = useState(false);
    const [error, setError] = useState('');

    const onConfirm = async () => {
        setResolving(true);
        setError('');
        try {
            await skillService.resolvePush({
                sourcePath: conflict.sourcePath,
                baseId: conflict.id,
                resolution,
                name: forkName.trim() || undefined,
            });
            onResolved(resolution);
        } catch {
            setError(t('assistant.conflict.resolveFailed', language));
        } finally {
            setResolving(false);
        }
    };

    return (
        <div class="ws-modal-overlay" onClick={onClose}>
            <div class="ws-modal skill-conflict-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>{t('assistant.conflict.title', language)}</span>
                    <button class="ws-modal-close" onClick={onClose}>
                        ✕
                    </button>
                </div>
                <div class="ws-modal-body">
                    <p class="skill-conflict-desc">
                        {t('assistant.conflict.body', language, {
                            name: conflict.name,
                            storeVersion: conflict.storeVersion,
                            baseVersion: conflict.baseVersion,
                        })}
                    </p>
                    <div class="skill-conflict-choices">
                        <label class={`skill-conflict-choice${resolution === 'fork' ? ' is-active' : ''}`}>
                            <input
                                type="radio"
                                name="skill-conflict-resolution"
                                checked={resolution === 'fork'}
                                onChange={() => setResolution('fork')}
                            />
                            <span>{t('assistant.conflict.fork', language)}</span>
                        </label>
                        <label class={`skill-conflict-choice${resolution === 'main' ? ' is-active' : ''}`}>
                            <input
                                type="radio"
                                name="skill-conflict-resolution"
                                checked={resolution === 'main'}
                                onChange={() => setResolution('main')}
                            />
                            <span>{t('assistant.conflict.setMain', language)}</span>
                        </label>
                    </div>
                    <input
                        class="ws-modal-input"
                        placeholder={t('assistant.conflict.forkNamePlaceholder', language)}
                        value={forkName}
                        onInput={(e: Event) => setForkName((e.target as HTMLInputElement).value)}
                        onKeyDown={(e: KeyboardEvent) => {
                            if (e.key === 'Enter') void onConfirm();
                            else if (e.key === 'Escape') onClose();
                        }}
                    />
                    {error && <p class="skill-conflict-error">{error}</p>}
                </div>
                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose} disabled={resolving}>
                        {t('assistant.conflict.cancel', language)}
                    </button>
                    <button class="ws-modal-confirm" onClick={() => void onConfirm()} disabled={resolving}>
                        {resolving
                            ? t('assistant.conflict.resolving', language)
                            : t('assistant.conflict.confirm', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
