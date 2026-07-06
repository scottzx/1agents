import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { App } from '../app';
import type { FsEntry } from '../types';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as fs from '../../stores/fsStore';
import { fsService } from '../../services/fsService';
import { FilePreviewPane } from '../shared/WorkspacePanes';
import { skillService, type WorkspaceSkillStatus } from '@1agents/core/services/skillService';
import * as modal from '../../stores/modalStore';

/**
 * 技能 tab of the 助理 详情. A skill is a *folder* under
 * <ws>/.claude/skills/<dir>, so the grid lists one card per skill (4 per row);
 * opening a card drills into that skill: metadata header on top, then a
 * two-pane file browser (the skill folder's files on the left, live preview on
 * the right — reusing the workspace file preview).
 */
const BADGE: Record<WorkspaceSkillStatus['state'], { cls: string; key: string }> = {
    synced: { cls: 'is-synced', key: 'assistant.detail.synced' },
    modified: { cls: 'is-modified', key: 'assistant.detail.modified' },
    local: { cls: 'is-local', key: 'assistant.detail.local' },
    'update-available': { cls: 'is-update', key: 'assistant.detail.updateAvailable' },
};

export function SkillsTab({ workspaceId, app, language }: { workspaceId: string; app: App; language: Lang }) {
    const [skills, setSkills] = useState<WorkspaceSkillStatus[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [selected, setSelected] = useState<WorkspaceSkillStatus | null>(null);
    const [pushing, setPushing] = useState(false);
    const [pulling, setPulling] = useState(false);
    const [flash, setFlash] = useState('');

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            setSkills(await skillService.listWorkspaceSkills(workspaceId));
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [workspaceId]);
    useEffect(() => {
        void load();
    }, [load]);

    const skillDir = selected ? `.claude/skills/${selected.dir}` : '';

    // On opening a skill, preview its SKILL.md by default.
    useEffect(() => {
        if (!selected) return;
        void fs.openFileDetail({
            name: 'SKILL.md',
            path: `${skillDir}/SKILL.md`,
            isDir: false,
            size: 0,
            modTime: 0,
        });
    }, [selected, skillDir]);

    const onPush = async () => {
        if (!selected) return;
        setPushing(true);
        try {
            // Read-only preview first (issue #379 follow-up) — nothing is written
            // until the user picks a resolution in the dialog.
            const preview = await skillService.previewPush(workspaceId, selected.skillRef);
            modal.openPushPreviewModal(preview, workspaceId, selected.skillRef, result => {
                setFlash(
                    result === 'created'
                        ? t('assistant.detail.pushedCreated', language)
                        : t('assistant.push.pushed', language)
                );
                void load();
            });
        } catch {
            setFlash(t('assistant.push.previewFailed', language));
        } finally {
            setPushing(false);
        }
    };

    const onPull = async () => {
        if (!selected) return;
        setPulling(true);
        try {
            const res = await skillService.pullSkill(workspaceId, selected.skillRef);
            setFlash(
                res.status === 'dirty' ? t('assistant.pull.dirty', language) : t('assistant.pull.pulled', language)
            );
            void load();
        } catch {
            setFlash(t('assistant.pull.failed', language));
        } finally {
            setPulling(false);
        }
    };

    // ── Skill detail (folder browser + preview) ──────────────────────────────
    if (selected) {
        const canPush = selected.state !== 'synced';
        const canPull = selected.state === 'update-available';
        return (
            <div class="skill-detail">
                <div class="skill-detail-head">
                    <button class="assistant-btn assistant-btn-ghost" onClick={() => setSelected(null)}>
                        ← {t('assistant.detail.skillsTitle', language)}
                    </button>
                    <div class="skill-detail-meta">
                        <div class="skill-detail-title">
                            <span class="skill-detail-name">{selected.name || selected.dir}</span>
                            <span class={`assistant-skill-badge ${BADGE[selected.state].cls}`}>
                                {t(BADGE[selected.state].key, language)}
                            </span>
                            {selected.version > 0 && <span class="assistant-skill-version">v{selected.version}</span>}
                        </div>
                        {selected.primaryTag || selected.secondaryTag ? (
                            <div
                                class="skill-detail-tags"
                                style="display:flex; gap:6px; margin-top:4px; margin-bottom:8px; flex-wrap:wrap;"
                            >
                                {selected.primaryTag && (
                                    <span
                                        class="tag-badge"
                                        style="background:rgba(0,200,255,0.15); color:#00c8ff; font-size:10px; padding:2px 6px; border-radius:3px; font-weight:bold;"
                                    >
                                        {selected.primaryTag}
                                    </span>
                                )}
                                {selected.secondaryTag && (
                                    <span
                                        class="tag-badge"
                                        style="background:rgba(255,200,0,0.15); color:#ffc800; font-size:10px; padding:2px 6px; border-radius:3px; font-weight:bold;"
                                    >
                                        {selected.secondaryTag}
                                    </span>
                                )}
                            </div>
                        ) : null}
                        {selected.description && <p class="skill-detail-desc">{selected.description}</p>}
                    </div>
                    <div class="assistant-section-actions">
                        {flash && <span class="assistant-flash">{flash}</span>}
                        {canPull && (
                            <button
                                class="assistant-btn assistant-btn-ghost"
                                disabled={pulling}
                                onClick={() => void onPull()}
                            >
                                {pulling ? t('assistant.pull.pulling', language) : t('assistant.pull.pull', language)}
                            </button>
                        )}
                        {canPush && (
                            <button
                                class="assistant-btn assistant-btn-ghost"
                                disabled={pushing}
                                onClick={() => void onPush()}
                            >
                                {pushing
                                    ? t('assistant.detail.pushing', language)
                                    : selected.state === 'local'
                                      ? t('assistant.detail.pushCreate', language)
                                      : t('assistant.detail.push', language)}
                            </button>
                        )}
                    </div>
                </div>
                <div class="file-split">
                    <div class="file-split-list">
                        <SkillFileList rootDir={skillDir} />
                    </div>
                    <div class="file-split-preview">
                        <FilePreviewPane app={app} language={language} />
                    </div>
                </div>
            </div>
        );
    }

    // ── Card grid ────────────────────────────────────────────────────────────
    return (
        <div class="skills-tab">
            <p class="assistant-hint">{t('assistant.detail.skillsHint', language)}</p>
            {loading && <div class="assistant-empty-row">…</div>}
            {!loading && error && (
                <div class="assistant-empty-row">
                    {t('assistant.detail.skillsLoadFailed', language)}: {error}
                </div>
            )}
            {!loading && !error && skills.length === 0 && (
                <div class="assistant-empty-row">{t('assistant.detail.skillsEmpty', language)}</div>
            )}
            {!loading && !error && skills.length > 0 && (
                <div class="skills-grid">
                    {skills.map(s => (
                        <button key={s.skillRef} class="skill-card" onClick={() => setSelected(s)}>
                            <div class="skill-card-head">
                                <span class="skill-card-name">{s.name || s.dir}</span>
                                <span class={`assistant-skill-badge ${BADGE[s.state].cls}`}>
                                    {t(BADGE[s.state].key, language)}
                                </span>
                                {s.version > 0 && <span class="assistant-skill-version">v{s.version}</span>}
                            </div>
                            {s.description && <p class="skill-card-desc">{s.description}</p>}
                            {s.primaryTag || s.secondaryTag ? (
                                <div
                                    class="skill-card-tags"
                                    style="display:flex; gap:6px; margin-top:8px; flex-wrap:wrap;"
                                >
                                    {s.primaryTag && (
                                        <span
                                            class="tag-badge"
                                            style="background:rgba(0,200,255,0.15); color:#00c8ff; font-size:10px; padding:2px 6px; border-radius:3px; font-weight:bold;"
                                        >
                                            {s.primaryTag}
                                        </span>
                                    )}
                                    {s.secondaryTag && (
                                        <span
                                            class="tag-badge"
                                            style="background:rgba(255,200,0,0.15); color:#ffc800; font-size:10px; padding:2px 6px; border-radius:3px; font-weight:bold;"
                                        >
                                            {s.secondaryTag}
                                        </span>
                                    )}
                                </div>
                            ) : null}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

/**
 * Minimal folder navigator scoped to a skill package. Lists the current dir
 * (dirs first), lets you drill into subfolders (with an "up" row), and opens
 * files into the shared fs preview. Highlights the currently-previewed file.
 */
function SkillFileList({ rootDir }: { rootDir: string }) {
    const [curDir, setCurDir] = useState(rootDir);
    const [entries, setEntries] = useState<FsEntry[]>([]);
    const [loading, setLoading] = useState(false);
    const selectedPath = fs.selectedFsEntry.value?.path;

    useEffect(() => {
        setCurDir(rootDir);
    }, [rootDir]);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        fsService
            .list(curDir)
            .then(list => {
                if (cancelled) return;
                setEntries(
                    [...list].sort((a, b) => {
                        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
                        return a.name.localeCompare(b.name);
                    })
                );
            })
            .catch(() => !cancelled && setEntries([]))
            .finally(() => !cancelled && setLoading(false));
        return () => {
            cancelled = true;
        };
    }, [curDir]);

    const parentDir = curDir.slice(0, curDir.lastIndexOf('/'));
    const canGoUp = curDir !== rootDir && curDir.startsWith(rootDir);

    return (
        <div class="skill-file-list">
            {canGoUp && (
                <button class="skill-file-row is-dir" onClick={() => setCurDir(parentDir)}>
                    <span class="skill-file-icon">⤴</span>
                    <span class="skill-file-name">..</span>
                </button>
            )}
            {loading && <div class="assistant-empty-row">…</div>}
            {!loading &&
                entries.map(e =>
                    e.isDir ? (
                        <button key={e.path} class="skill-file-row is-dir" onClick={() => setCurDir(e.path)}>
                            <span class="skill-file-icon">📁</span>
                            <span class="skill-file-name">{e.name}</span>
                        </button>
                    ) : (
                        <button
                            key={e.path}
                            class={`skill-file-row${selectedPath === e.path ? ' is-active' : ''}`}
                            onClick={() => void fs.openFileDetail(e)}
                        >
                            <span class="skill-file-icon">📄</span>
                            <span class="skill-file-name">{e.name}</span>
                        </button>
                    )
                )}
        </div>
    );
}
