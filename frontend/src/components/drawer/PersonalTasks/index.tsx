import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import * as wsStore from '../../../stores/workspaceStore';
import { t } from '../../../i18n';
import { personalTaskService } from '@1agents/core/services/personalTaskService';
import { workspaceService } from '@1agents/core/services/workspaceService';
import type { Task } from '@1agents/core/types/task';

// 个人任务 + 立项 (#67): the no-project backlog. Lightweight personal tasks land
// here (无 project_id, 不强制归口, scheduler 跳过) — captured straight, or funneled
// from the Inbox (#60). When one accrues enough weight it 立项 (incubates) into a
// real long-term Project via the modal below. This sits a sibling to Inbox /
// 定时任务 in the left-rail nav; it is NOT the agenda calendar (that's Reminders).
export function PersonalTasksPane() {
    const language = ui.language.value;
    const [tasks, setTasks] = useState<Task[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [draft, setDraft] = useState('');
    const [capturing, setCapturing] = useState(false);
    const [incubateTarget, setIncubateTarget] = useState<Task | null>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setTasks(await personalTaskService.list());
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    const capture = async () => {
        const title = draft.trim();
        if (!title || capturing) return;
        setCapturing(true);
        try {
            await personalTaskService.capture({ title });
            setDraft('');
            await refresh();
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setCapturing(false);
        }
    };

    const onKeyDown = (e: KeyboardEvent) => {
        if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
            e.preventDefault();
            capture();
        }
    };

    const isFromInbox = (task: Task) => (task.labels || []).some(l => l.startsWith('captured-from:'));

    return (
        <div class="personal-pane">
            <div class="personal-header">
                <h2 class="personal-title">{t('personal.title', language)}</h2>
            </div>
            <p class="personal-desc">{t('personal.desc', language)}</p>

            <div class="personal-capture">
                <input
                    class="personal-capture-input"
                    type="text"
                    placeholder={t('personal.capturePlaceholder', language)}
                    value={draft}
                    onInput={(e: Event) => setDraft((e.target as HTMLInputElement).value)}
                    onKeyDown={onKeyDown}
                />
                <button class="personal-capture-btn" onClick={capture} disabled={!draft.trim() || capturing}>
                    {t('personal.captureBtn', language)}
                </button>
            </div>

            {error && <div class="personal-error">{error}</div>}

            <div class="personal-list">
                {!loading && tasks.length === 0 && <div class="personal-empty">{t('personal.empty', language)}</div>}
                {tasks.map(task => (
                    <div key={task.id} class="personal-item">
                        <div class="personal-item-main">
                            <div class="personal-item-meta">
                                {typeof task.number === 'number' && (
                                    <span class="personal-item-num">#{task.number}</span>
                                )}
                                {isFromInbox(task) && (
                                    <span class="personal-from-inbox">{t('personal.fromInbox', language)}</span>
                                )}
                                <span class="personal-item-time">
                                    {new Date(task.createdAt).toLocaleString(language)}
                                </span>
                            </div>
                            <div class="personal-item-title">{task.title}</div>
                            {task.description && <div class="personal-item-desc">{task.description}</div>}
                        </div>
                        <div class="personal-item-actions">
                            <button class="personal-incubate-btn" onClick={() => setIncubateTarget(task)}>
                                {t('personal.incubate', language)}
                            </button>
                        </div>
                    </div>
                ))}
            </div>

            {incubateTarget && (
                <IncubateModal
                    task={incubateTarget}
                    onClose={() => setIncubateTarget(null)}
                    onDone={() => {
                        setIncubateTarget(null);
                        refresh();
                    }}
                />
            )}
        </div>
    );
}

interface IncubateModalProps {
    task: Task;
    onClose: () => void;
    onDone: () => void;
}

// The 立项 gate. Promotes the personal task into a fresh Project, then registers
// that project as a workspace — meta `projects` (where Incubate writes) and the
// `workspaces_dir.json` registry (where the sidebar reads) are decoupled, so
// without this the task would vanish from here with no visible project to land in.
function IncubateModal({ task, onClose, onDone }: IncubateModalProps) {
    const language = ui.language.value;
    const [projectName, setProjectName] = useState(task.title);
    const [workspacePath, setWorkspacePath] = useState('');
    const [milestonesText, setMilestonesText] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [error, setError] = useState('');

    const submit = async () => {
        const name = projectName.trim();
        const path = workspacePath.trim();
        if (!name || !path || submitting) return;
        setSubmitting(true);
        setError('');
        try {
            const milestones = milestonesText
                .split('\n')
                .map(s => s.trim())
                .filter(Boolean);
            const { project } = await personalTaskService.incubate(task.id, {
                projectName: name,
                workspacePath: path,
                milestones,
            });

            // The server-side incubation has already committed; surface registration
            // problems via toast but never re-throw (a retry would hit "path exists").
            try {
                await workspaceService.create({
                    id: project.id,
                    name: project.name,
                    path: project.workspacePath,
                    status: 'active',
                });
                const list = await wsStore.loadWorkspaces(true);
                const newWs = list.find(w => w.id === project.id);
                if (newWs) await wsStore.selectWorkspace(newWs);
            } catch (regErr) {
                ui.showToast(t('personal.incubate.registerFailed', language, { err: String(regErr) }));
            }

            ui.showToast(t('personal.incubate.success', language, { name }));
            onDone();
        } catch (err) {
            setError((err as Error).message);
            setSubmitting(false);
        }
    };

    return (
        <div class="personal-modal-overlay" onClick={onClose}>
            <div class="personal-modal" onClick={(e: Event) => e.stopPropagation()}>
                <h3 class="personal-modal-title">{t('personal.incubate.title', language)}</h3>
                <p class="personal-modal-from">
                    {t('personal.incubate.from', language)} {task.title}
                </p>

                <label class="personal-modal-field">
                    <span class="personal-modal-label">{t('personal.incubate.projectName', language)}</span>
                    <input
                        class="personal-modal-input"
                        type="text"
                        value={projectName}
                        onInput={(e: Event) => setProjectName((e.target as HTMLInputElement).value)}
                    />
                </label>

                <label class="personal-modal-field">
                    <span class="personal-modal-label">{t('personal.incubate.workspacePath', language)}</span>
                    <input
                        class="personal-modal-input"
                        type="text"
                        placeholder="/Users/me/projects/my-project"
                        value={workspacePath}
                        onInput={(e: Event) => setWorkspacePath((e.target as HTMLInputElement).value)}
                    />
                    <span class="personal-modal-hint">{t('personal.incubate.workspacePathHint', language)}</span>
                </label>

                <label class="personal-modal-field">
                    <span class="personal-modal-label">{t('personal.incubate.milestones', language)}</span>
                    <textarea
                        class="personal-modal-input"
                        rows={3}
                        placeholder={t('personal.incubate.milestonesPlaceholder', language)}
                        value={milestonesText}
                        onInput={(e: Event) => setMilestonesText((e.target as HTMLTextAreaElement).value)}
                    />
                </label>

                {error && <div class="personal-error">{error}</div>}

                <div class="personal-modal-actions">
                    <button class="personal-modal-cancel" onClick={onClose} disabled={submitting}>
                        {t('personal.incubate.cancel', language)}
                    </button>
                    <button
                        class="personal-modal-submit"
                        onClick={submit}
                        disabled={!projectName.trim() || !workspacePath.trim() || submitting}
                    >
                        {t('personal.incubate.submit', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
