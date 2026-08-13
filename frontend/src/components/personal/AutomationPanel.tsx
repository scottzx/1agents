/**
 * Automation recipe surface. One personal-shell entry replaces 定时任务 +
 * 工作聚合: recipes (default), editor, runs, calendar.
 */
import { h } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';

import { executionService } from '@1agents/core/services/executionService';
import { projectItemService } from '@1agents/core/services/taskService';
import type { ExecutionJob } from '@1agents/core/types/execution';
import type { ProjectItem } from '@1agents/core/types/task';

import { t, type Lang } from '../../i18n';
import * as modal from '../../stores/modalStore';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import { AgentProfilePicker } from '../chat/AgentProfilePicker';
import { RemindersPane } from '../drawer/Reminders';
import { PersonalAggregatePanel } from './PersonalAggregatePanel';

export type AutomationView = 'recipes' | 'editor' | 'runs' | 'calendar';

const AUTOMATION_PREFIX = 'automation:';
const SCRIPT_PREFIX = 'script:';

const TEMPLATES: readonly { id: string; titleKey: string; descKey: string; instructionsKey: string }[] = [
    {
        id: 'email',
        titleKey: 'automation.template.email.title',
        descKey: 'automation.template.email.desc',
        instructionsKey: 'automation.template.email.body',
    },
    {
        id: 'stock',
        titleKey: 'automation.template.stock.title',
        descKey: 'automation.template.stock.desc',
        instructionsKey: 'automation.template.stock.body',
    },
    {
        id: 'tasks',
        titleKey: 'automation.template.tasks.title',
        descKey: 'automation.template.tasks.desc',
        instructionsKey: 'automation.template.tasks.body',
    },
];

interface RecipeDraft {
    jobId?: string;
    itemId?: string;
    projectId: string;
    title: string;
    instructions: string;
    profileId: string;
    cwd: string;
    usePreamble: boolean;
    scriptPath: string;
    triggerKind: '' | 'at' | 'recurrence';
    triggerAt: string;
    everyMinutes: string;
}

function emptyDraft(projectId = ''): RecipeDraft {
    return {
        projectId,
        title: '',
        instructions: '',
        profileId: '',
        cwd: '',
        usePreamble: false,
        scriptPath: 'automation.py',
        triggerKind: '',
        triggerAt: '',
        everyMinutes: '60',
    };
}

function scriptFromCaps(caps?: string[]): string {
    const found = (caps || []).find(cap => cap.startsWith(SCRIPT_PREFIX));
    return found ? found.slice(SCRIPT_PREFIX.length) : 'automation.py';
}

function isRecipe(job: ExecutionJob): boolean {
    return (job.businessRef || '').startsWith(AUTOMATION_PREFIX);
}

export function AutomationPanel({ initialView = 'recipes' }: { initialView?: AutomationView }) {
    const language = ui.language.value;
    const [view, setView] = useState<AutomationView>(initialView);
    const [jobs, setJobs] = useState<ExecutionJob[]>([]);
    const [items, setItems] = useState<Record<string, ProjectItem>>({});
    const [loading, setLoading] = useState(true);
    const [draft, setDraft] = useState<RecipeDraft>(emptyDraft());
    const [saving, setSaving] = useState(false);

    const workspaces = wsStore.workspaces.value.filter(ws => !ws.builtin);

    const load = useCallback(async () => {
        setLoading(true);
        try {
            const all = await executionService.listJobs();
            const recipes = all.filter(isRecipe);
            setJobs(recipes);
            const next: Record<string, ProjectItem> = {};
            await Promise.all(
                recipes.map(async job => {
                    try {
                        next[job.workItemId] = await projectItemService.get(job.workItemId);
                    } catch {
                        /* item may have been deleted */
                    }
                })
            );
            setItems(next);
        } catch (err) {
            ui.showToast(String(err));
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    const openNew = (preset?: { title?: string; instructions?: string }) => {
        const projectId = wsStore.activeWorkspaceId.value || workspaces[0]?.id || '';
        const cwd = workspaces.find(ws => ws.id === projectId)?.path || '';
        setDraft({
            ...emptyDraft(projectId),
            title: preset?.title || t('automation.newName', language),
            instructions: preset?.instructions || '',
            cwd,
        });
        setView('editor');
    };

    const openEdit = (job: ExecutionJob) => {
        const item = items[job.workItemId];
        const trigger = job.trigger;
        let triggerAt = '';
        if (trigger?.kind === 'at' && typeof trigger.spec.at === 'string') {
            triggerAt = trigger.spec.at.slice(0, 16);
        }
        setDraft({
            jobId: job.id,
            itemId: job.workItemId,
            projectId: job.projectId,
            title: item?.title || t('automation.untitled', language),
            instructions: item?.description || '',
            profileId: job.profileId || '',
            cwd: job.cwd || '',
            usePreamble: Boolean(job.preambleFunctionType),
            scriptPath: scriptFromCaps(job.capabilities),
            triggerKind: trigger?.kind || '',
            triggerAt,
            everyMinutes: String(trigger?.spec.everyMinutes || 60),
        });
        setView('editor');
    };

    const save = async () => {
        if (!draft.title.trim() || !draft.projectId) {
            ui.showToast(t('automation.needTitleProject', language));
            return;
        }
        setSaving(true);
        try {
            let itemId = draft.itemId;
            if (!itemId) {
                const created = await projectItemService.create({
                    workspace_id: draft.projectId,
                    title: draft.title.trim(),
                    description: draft.instructions,
                    type: 'task',
                    executor: 'agent',
                });
                itemId = created.id;
            } else {
                await projectItemService.patch(itemId, {
                    title: draft.title.trim(),
                    description: draft.instructions,
                });
            }
            const capabilities = draft.usePreamble
                ? [`${SCRIPT_PREFIX}${draft.scriptPath.trim() || 'automation.py'}`]
                : [];
            let jobId = draft.jobId;
            if (!jobId) {
                const job = await executionService.createJob({
                    projectId: draft.projectId,
                    workItemId: itemId,
                    businessRef: `${AUTOMATION_PREFIX}${itemId}`,
                    executorKind: 'agent',
                    profileId: draft.profileId || undefined,
                    preambleFunctionType: draft.usePreamble ? 'core.script' : '',
                    cwd: draft.cwd,
                    capabilities,
                    maxAttempts: 1,
                });
                jobId = job.id;
            } else {
                await executionService.updateJob(jobId, {
                    profileId: draft.profileId || undefined,
                    preambleFunctionType: draft.usePreamble ? 'core.script' : '',
                    cwd: draft.cwd,
                    capabilities,
                });
            }
            if (!draft.triggerKind) {
                if (draft.jobId) {
                    try {
                        await executionService.deleteTrigger(jobId);
                    } catch {
                        /* no trigger */
                    }
                }
            } else if (draft.triggerKind === 'at' && draft.triggerAt) {
                await executionService.upsertTrigger(jobId, {
                    kind: 'at',
                    spec: { at: new Date(draft.triggerAt).toISOString() },
                    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
                    misfirePolicy: 'run_once',
                    overlapPolicy: 'forbid',
                });
            } else if (draft.triggerKind === 'recurrence') {
                const every = Number(draft.everyMinutes);
                if (!Number.isInteger(every) || every < 1) {
                    throw new Error(t('automation.invalidMinutes', language));
                }
                await executionService.upsertTrigger(jobId, {
                    kind: 'recurrence',
                    spec: { everyMinutes: every },
                    timezone: Intl.DateTimeFormat().resolvedOptions().timeZone,
                    misfirePolicy: 'skip',
                    overlapPolicy: 'forbid',
                });
            }
            ui.showToast(t('automation.saved', language));
            await load();
            setView('recipes');
        } catch (err) {
            ui.showToast(String(err));
        } finally {
            setSaving(false);
        }
    };

    return (
        <div class="automation-pane">
            <div class="automation-header">
                <h2 class="automation-title">{t('sidebar.navCtrl.automation', language)}</h2>
                <div class="automation-tabs" role="tablist">
                    {(['recipes', 'runs', 'calendar'] as const).map(id => (
                        <button
                            key={id}
                            type="button"
                            role="tab"
                            class={`automation-tab${view === id || (view === 'editor' && id === 'recipes') ? ' is-active' : ''}`}
                            onClick={() => setView(id)}
                        >
                            {t(`automation.view.${id}`, language)}
                        </button>
                    ))}
                </div>
            </div>

            {view === 'recipes' && (
                <RecipeList
                    language={language}
                    loading={loading}
                    jobs={jobs}
                    items={items}
                    onNew={() => openNew()}
                    onEdit={openEdit}
                    onTemplate={tpl =>
                        openNew({
                            title: t(tpl.titleKey, language),
                            instructions: t(tpl.instructionsKey, language),
                        })
                    }
                    onRefresh={() => void load()}
                />
            )}
            {view === 'editor' && (
                <RecipeEditor
                    language={language}
                    draft={draft}
                    saving={saving}
                    workspaces={workspaces}
                    onChange={setDraft}
                    onCancel={() => setView('recipes')}
                    onSave={() => void save()}
                />
            )}
            {view === 'runs' && (
                <div class="automation-embed">
                    <PersonalAggregatePanel />
                </div>
            )}
            {view === 'calendar' && (
                <div class="automation-embed">
                    <RemindersPane />
                </div>
            )}
        </div>
    );
}

function RecipeList({
    language,
    loading,
    jobs,
    items,
    onNew,
    onEdit,
    onTemplate,
    onRefresh,
}: {
    language: Lang;
    loading: boolean;
    jobs: ExecutionJob[];
    items: Record<string, ProjectItem>;
    onNew: () => void;
    onEdit: (job: ExecutionJob) => void;
    onTemplate: (tpl: (typeof TEMPLATES)[number]) => void;
    onRefresh: () => void;
}) {
    const paused = useMemo(() => jobs.filter(job => job.status === 'paused').length, [jobs]);
    return (
        <div class="automation-body">
            <div class="automation-toolbar">
                <span class="automation-meta">
                    {t('automation.count', language, { n: jobs.length })}
                    {paused > 0 ? ` · ${t('automation.pausedCount', language, { n: paused })}` : ''}
                </span>
                <div class="automation-toolbar-actions">
                    <button type="button" class="automation-refresh" onClick={onRefresh}>
                        ↻
                    </button>
                    <button type="button" class="automation-primary" onClick={onNew}>
                        {t('automation.new', language)}
                    </button>
                </div>
            </div>

            {loading ? (
                <div class="automation-empty">{t('automation.loading', language)}</div>
            ) : jobs.length === 0 ? (
                <div class="automation-empty">{t('automation.empty', language)}</div>
            ) : (
                <ul class="automation-list bento-grid">
                    {jobs.map(job => {
                        const item = items[job.workItemId];
                        return (
                            <li key={job.id}>
                                <button type="button" class="automation-card" onClick={() => onEdit(job)}>
                                    <div class="bento-zone-header">
                                        <div class="automation-card-title">
                                            {item?.title || t('automation.untitled', language)}
                                        </div>
                                        <span
                                            class={`automation-card-badge${job.status === 'paused' ? ' is-paused' : ''}`}
                                        >
                                            {job.status === 'paused'
                                                ? t('automation.status.paused', language)
                                                : t('automation.status.active', language)}
                                        </span>
                                    </div>
                                    <div class="bento-zone-body">
                                        <div class="automation-card-desc">
                                            {(item?.description || '').split('\n')[0] ||
                                                t('automation.noInstructions', language)}
                                        </div>
                                    </div>
                                    <div class="automation-card-meta">
                                        {job.preambleFunctionType && (
                                            <span>{t('automation.hasPreamble', language)}</span>
                                        )}
                                        {job.trigger?.nextRunAt && (
                                            <span>
                                                {t('automation.nextRun', language, {
                                                    at: new Date(job.trigger.nextRunAt).toLocaleString(),
                                                })}
                                            </span>
                                        )}
                                    </div>
                                </button>
                            </li>
                        );
                    })}
                </ul>
            )}

            <div class="automation-suggested">
                <h3>{t('automation.suggested', language)}</h3>
                <div class="automation-suggested-grid">
                    {TEMPLATES.map(tpl => (
                        <button key={tpl.id} type="button" class="automation-card" onClick={() => onTemplate(tpl)}>
                            <div class="automation-card-title">{t(tpl.titleKey, language)}</div>
                            <div class="automation-card-desc">{t(tpl.descKey, language)}</div>
                            <span class="automation-add">{t('automation.add', language)}</span>
                        </button>
                    ))}
                </div>
            </div>
        </div>
    );
}

function RecipeEditor({
    language,
    draft,
    saving,
    workspaces,
    onChange,
    onCancel,
    onSave,
}: {
    language: Lang;
    draft: RecipeDraft;
    saving: boolean;
    workspaces: { id: string; name: string; path?: string }[];
    onChange: (next: RecipeDraft) => void;
    onCancel: () => void;
    onSave: () => void;
}) {
    const patch = (partial: Partial<RecipeDraft>) => onChange({ ...draft, ...partial });
    return (
        <div class="automation-editor">
            <label class="automation-field">
                <span>{t('automation.field.name', language)}</span>
                <input
                    value={draft.title}
                    onInput={e => patch({ title: (e.currentTarget as HTMLInputElement).value })}
                />
            </label>
            <label class="automation-field">
                <span>{t('automation.field.project', language)}</span>
                <select
                    value={draft.projectId}
                    disabled={Boolean(draft.jobId)}
                    onChange={e => {
                        const projectId = (e.currentTarget as HTMLSelectElement).value;
                        const ws = workspaces.find(item => item.id === projectId);
                        patch({ projectId, cwd: draft.cwd || ws?.path || '' });
                    }}
                >
                    <option value="">{t('automation.field.projectPlaceholder', language)}</option>
                    {workspaces.map(ws => (
                        <option key={ws.id} value={ws.id}>
                            {ws.name}
                        </option>
                    ))}
                </select>
            </label>
            <label class="automation-field">
                <span>{t('automation.field.trigger', language)}</span>
                <select
                    value={draft.triggerKind}
                    onChange={e =>
                        patch({
                            triggerKind: (e.currentTarget as HTMLSelectElement).value as RecipeDraft['triggerKind'],
                        })
                    }
                >
                    <option value="">{t('automation.trigger.manual', language)}</option>
                    <option value="at">{t('automation.trigger.at', language)}</option>
                    <option value="recurrence">{t('automation.trigger.recurrence', language)}</option>
                </select>
            </label>
            {draft.triggerKind === 'at' && (
                <label class="automation-field">
                    <span>{t('automation.field.at', language)}</span>
                    <input
                        type="datetime-local"
                        value={draft.triggerAt}
                        onInput={e => patch({ triggerAt: (e.currentTarget as HTMLInputElement).value })}
                    />
                </label>
            )}
            {draft.triggerKind === 'recurrence' && (
                <label class="automation-field">
                    <span>{t('automation.field.everyMinutes', language)}</span>
                    <input
                        type="number"
                        min="1"
                        value={draft.everyMinutes}
                        onInput={e => patch({ everyMinutes: (e.currentTarget as HTMLInputElement).value })}
                    />
                </label>
            )}
            <label class="automation-field">
                <span>{t('automation.field.instructions', language)}</span>
                <textarea
                    rows={8}
                    value={draft.instructions}
                    onInput={e => patch({ instructions: (e.currentTarget as HTMLTextAreaElement).value })}
                />
            </label>
            <label class="automation-check">
                <input
                    type="checkbox"
                    checked={draft.usePreamble}
                    onChange={e => patch({ usePreamble: (e.currentTarget as HTMLInputElement).checked })}
                />
                <span>{t('automation.field.preamble', language)}</span>
            </label>
            {draft.usePreamble && (
                <label class="automation-field">
                    <span>{t('automation.field.script', language)}</span>
                    <input
                        value={draft.scriptPath}
                        onInput={e => patch({ scriptPath: (e.currentTarget as HTMLInputElement).value })}
                    />
                </label>
            )}
            <label class="automation-field">
                <span>{t('automation.field.profile', language)}</span>
                <AgentProfilePicker value={draft.profileId} onChange={profileId => patch({ profileId })} />
            </label>
            <label class="automation-field">
                <span>{t('automation.field.cwd', language)}</span>
                <div class="automation-cwd">
                    <input
                        value={draft.cwd}
                        onInput={e => patch({ cwd: (e.currentTarget as HTMLInputElement).value })}
                    />
                    <button
                        type="button"
                        onClick={() =>
                            modal.openDirPicker(
                                path => patch({ cwd: path }),
                                t('automation.field.cwd', language),
                                draft.cwd
                            )
                        }
                    >
                        {t('automation.browse', language)}
                    </button>
                </div>
            </label>
            <div class="automation-editor-actions">
                <button type="button" onClick={onCancel} disabled={saving}>
                    {t('common.cancel', language)}
                </button>
                <button type="button" class="automation-primary" onClick={onSave} disabled={saving}>
                    {saving ? t('automation.saving', language) : t('common.save', language)}
                </button>
            </div>
        </div>
    );
}
