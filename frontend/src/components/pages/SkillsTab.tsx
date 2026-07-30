import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { App } from '../app';
import type { FsEntry } from '../types';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as fs from '../../stores/fsStore';
import { fsService } from '../../services/fsService';
import { FilePreviewPane } from '../shared/WorkspacePanes';
import { skillService, type WorkspaceSkillStatus, type AvailableSkill } from '@1agents/core/services/skillService';
import * as modal from '../../stores/modalStore';
import * as taskNav from '../../stores/taskNavStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabs from '../../stores/tabsStore';
import type { Crumb } from '../platform/ShellNav';

/**
 * 技能 tab of the 助理 详情. A skill is a *folder* under
 * <ws>/.claude/skills/<dir>, so the grid lists one card per skill (4 per row);
 * opening a card drills into that skill: metadata header on top, then a
 * two-pane file browser (the skill folder's files on the left, live preview on
 * the right — reusing the workspace file preview).
 */
const BADGE: Record<WorkspaceSkillStatus['state'], { cls: string; key: string }> = {
    active: { cls: 'is-synced', key: 'assistant.detail.extensionActive' },
    disabled: { cls: 'is-local', key: 'assistant.detail.extensionDisabled' },
};

export function SkillsTab({
    workspaceId,
    app,
    language,
    /**
     * Override the parent crumb prepended to the global breadcrumb trail.
     * Default (omitted) = the 助理 crumb that the 助理 detail uses. Pass a
     * Crumb to integrate with a different host page (e.g. 项目详情 uses
     * { 项目总览, onClick=projectOverview }) — when set, SkillsTab clears the
     * trail on unmount so the host's own breadcrumb (customCrumbs) reclaims it.
     */
    crumbsParent,
}: {
    workspaceId: string;
    app: App;
    language: Lang;
    crumbsParent?: Crumb;
}) {
    const [skills, setSkills] = useState<WorkspaceSkillStatus[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [selected, setSelected] = useState<WorkspaceSkillStatus | null>(null);
    const [pushing, setPushing] = useState(false);
    const [pulling, setPulling] = useState(false);
    const [flash, setFlash] = useState('');
    // Add-from-global HarnessKit inventory picker + project-remove confirm.
    const [pickerOpen, setPickerOpen] = useState(false);
    const [available, setAvailable] = useState<AvailableSkill[]>([]);
    const [pickerBusy, setPickerBusy] = useState(false);
    const [pickerQuery, setPickerQuery] = useState('');
    const [confirmRemove, setConfirmRemove] = useState(false);
    const [removing, setRemoving] = useState(false);

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

    const wsName = wsStore.workspaces.value.find(w => w.id === workspaceId)?.name ?? '';

    // Drive back-navigation through the global header breadcrumb (助理/项目总览 ›
    // <name> › <skill>) instead of an in-pane back button: clicking <name>
    // returns to the skill list. On unmount, the global trail is either reset
    // (default 助理 crumb, if still in the assistant detail) or cleared
    // (custom parent — host reclaims via its own customCrumbs).
    useEffect(() => {
        const parentCrumb: Crumb = crumbsParent ?? {
            label: t('sidebar.assistants', language),
            onClick: () => {
                tabs.assistantDetailId.value = null;
            },
        };
        taskNav.headerCrumbs.value = selected
            ? [parentCrumb, { label: wsName, onClick: () => setSelected(null) }, { label: selected.name }]
            : [parentCrumb, { label: wsName }];
        return () => {
            if (crumbsParent !== undefined) {
                // Custom parent: clear so the host page's customCrumbs wins.
                taskNav.headerCrumbs.value = null;
                return;
            }
            if (tabs.assistantDetailId.value === workspaceId) {
                taskNav.headerCrumbs.value = [
                    {
                        label: t('sidebar.assistants', language),
                        onClick: () => {
                            tabs.assistantDetailId.value = null;
                        },
                    },
                    { label: wsName },
                ];
            }
        };
    }, [selected, wsName, language, workspaceId, crumbsParent]);

    useEffect(() => {
        if (!selected) return;
        return taskNav.registerHeaderBackAction(
            `skill-detail:${workspaceId}`,
            () => setSelected(null),
            taskNav.HEADER_BACK_PRIORITY.detail
        );
    }, [selected, workspaceId]);

    const openPicker = async () => {
        setPickerOpen(true);
        setPickerQuery('');
        setPickerBusy(true);
        try {
            setAvailable(await skillService.listAvailableSkills(workspaceId));
        } catch {
            setAvailable([]);
        } finally {
            setPickerBusy(false);
        }
    };

    const onAdd = async (extensionId: string) => {
        setPickerBusy(true);
        try {
            await skillService.addSkill(workspaceId, extensionId);
            setPickerOpen(false);
            setFlash(t('assistant.detail.added', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.addFailed', language));
        } finally {
            setPickerBusy(false);
        }
    };

    const onRemove = async () => {
        if (!selected) return;
        setRemoving(true);
        try {
            await skillService.removeSkill(workspaceId, selected.extensionId);
            setConfirmRemove(false);
            setSelected(null);
            setFlash(t('assistant.detail.removed', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.removeFailed', language));
        } finally {
            setRemoving(false);
        }
    };

    const skillDir = selected?.path ?? '';

    // On opening a skill, preview its SKILL.md by default.
    useEffect(() => {
        if (!selected || !skillDir) return;
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
            const preview = await skillService.previewReindex(workspaceId, selected.extensionId);
            modal.openPushPreviewModal(preview, workspaceId, selected.extensionId, () => {
                setFlash(t('assistant.push.indexed', language));
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
            await skillService.updateSkill(workspaceId, selected.extensionId);
            setFlash(t('assistant.pull.updated', language));
            void load();
        } catch {
            setFlash(t('assistant.pull.failed', language));
        } finally {
            setPulling(false);
        }
    };

    // ── Skill detail (folder browser + preview) ──────────────────────────────
    if (selected) {
        const canPull = selected.canUpdate;
        return (
            <div class="skill-detail">
                {/* Row 1: title + status on the left, quick actions on the right.
                    Back-navigation lives in the header breadcrumb (no back button). */}
                <div class="skill-detail-head">
                    <div class="skill-detail-row1">
                        <div class="skill-detail-titlewrap">
                            <div class="skill-detail-title">
                                <span class="skill-detail-name">{selected.name}</span>
                                <span class={`assistant-skill-badge ${BADGE[selected.state].cls}`}>
                                    {t(BADGE[selected.state].key, language)}
                                </span>
                            </div>
                        </div>
                        <div class="skill-detail-actions">
                            {flash && <span class="assistant-flash">{flash}</span>}
                            {canPull && (
                                <button
                                    class="assistant-btn assistant-btn-ghost"
                                    disabled={pulling}
                                    onClick={() => void onPull()}
                                >
                                    {pulling
                                        ? t('assistant.pull.pulling', language)
                                        : t('assistant.pull.pull', language)}
                                </button>
                            )}
                            <button
                                class="assistant-btn assistant-btn-ghost"
                                disabled={pushing}
                                onClick={() => void onPush()}
                            >
                                {pushing
                                    ? t('assistant.detail.indexing', language)
                                    : t('assistant.detail.reindex', language)}
                            </button>
                            <button
                                class="assistant-btn assistant-btn-danger"
                                disabled={removing}
                                onClick={() => setConfirmRemove(true)}
                            >
                                {t('assistant.detail.remove', language)}
                            </button>
                        </div>
                    </div>
                    {selected.description && <p class="skill-detail-desc">{selected.description}</p>}
                </div>
                {skillDir && (
                    <div class="file-split">
                        <div class="file-split-list">
                            <SkillFileList rootDir={skillDir} />
                        </div>
                        <div class="file-split-preview">
                            <FilePreviewPane app={app} language={language} hideBack />
                        </div>
                    </div>
                )}
                {confirmRemove && (
                    <div class="ws-modal-overlay" onClick={() => !removing && setConfirmRemove(false)}>
                        <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                            <div class="ws-modal-header">
                                <span>{t('assistant.detail.removeConfirmTitle', language)}</span>
                                <button class="ws-modal-close" onClick={() => setConfirmRemove(false)}>
                                    ✕
                                </button>
                            </div>
                            <div class="ws-modal-body">
                                <p>
                                    {t('assistant.detail.removeConfirmBody', language, {
                                        name: selected.name,
                                    })}
                                </p>
                            </div>
                            <div class="ws-modal-footer">
                                <button
                                    class="ws-modal-cancel"
                                    onClick={() => setConfirmRemove(false)}
                                    disabled={removing}
                                >
                                    {t('assistant.detail.cancel', language)}
                                </button>
                                <button
                                    class="ws-modal-confirm ws-modal-danger"
                                    onClick={() => void onRemove()}
                                    disabled={removing}
                                >
                                    {removing
                                        ? t('assistant.detail.removing', language)
                                        : t('assistant.detail.confirmRemove', language)}
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    // ── Card grid ────────────────────────────────────────────────────────────
    return (
        <div class="skills-tab">
            <p class="assistant-hint">{t('assistant.detail.skillsHint', language)}</p>
            {flash && <span class="assistant-flash">{flash}</span>}
            {loading && <div class="assistant-empty-row">…</div>}
            {!loading && error && (
                <div class="assistant-empty-row">
                    {t('assistant.detail.skillsLoadFailed', language)}: {error}
                </div>
            )}
            {!loading && !error && (
                <div class="skills-grid">
                    {/* Add from the global HarnessKit inventory — always first. */}
                    <button class="skill-card skill-add-card" onClick={() => void openPicker()}>
                        <span class="skill-add-plus">＋</span>
                        <span class="skill-add-label">{t('assistant.detail.addSkill', language)}</span>
                    </button>
                    {skills.map(s => (
                        <button key={s.extensionId} class="skill-card" onClick={() => setSelected(s)}>
                            <div class="skill-card-head">
                                <span class="skill-card-name">{s.name}</span>
                                <span class={`assistant-skill-badge ${BADGE[s.state].cls}`}>
                                    {t(BADGE[s.state].key, language)}
                                </span>
                            </div>
                            {s.description && <p class="skill-card-desc">{s.description}</p>}
                        </button>
                    ))}
                </div>
            )}
            {pickerOpen && (
                <div class="ws-modal-overlay" onClick={() => !pickerBusy && setPickerOpen(false)}>
                    <div class="ws-modal skill-picker-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                        <div class="ws-modal-header">
                            <span>{t('assistant.detail.addSkillTitle', language)}</span>
                            <button class="ws-modal-close" onClick={() => setPickerOpen(false)}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            <input
                                class="ws-modal-input skill-picker-search"
                                type="search"
                                placeholder={t('assistant.detail.addSkillSearch', language)}
                                value={pickerQuery}
                                onInput={(e: Event) => setPickerQuery((e.target as HTMLInputElement).value)}
                            />
                            {(() => {
                                const q = pickerQuery.trim().toLowerCase();
                                const filtered = q
                                    ? available.filter(a =>
                                          [a.name, a.description, a.sourceAgent]
                                              .filter(Boolean)
                                              .some(s => (s as string).toLowerCase().includes(q))
                                      )
                                    : available;
                                if (pickerBusy && available.length === 0)
                                    return <div class="assistant-empty-row">…</div>;
                                if (available.length === 0)
                                    return (
                                        <div class="assistant-empty-row">
                                            {t('assistant.detail.addSkillEmpty', language)}
                                        </div>
                                    );
                                if (filtered.length === 0)
                                    return (
                                        <div class="assistant-empty-row">
                                            {t('assistant.detail.addSkillNoMatch', language)}
                                        </div>
                                    );
                                return (
                                    <div class="skill-picker-grid">
                                        {filtered.map(a => (
                                            <button
                                                key={a.extensionId}
                                                class="skill-card skill-picker-card"
                                                disabled={pickerBusy || !a.canInstall}
                                                onClick={() => void onAdd(a.extensionId)}
                                            >
                                                <div class="skill-card-head">
                                                    <span class="skill-card-name">{a.name}</span>
                                                </div>
                                                {a.description && <p class="skill-card-desc">{a.description}</p>}
                                            </button>
                                        ))}
                                    </div>
                                );
                            })()}
                        </div>
                    </div>
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
