import { h } from 'preact';
// eslint-disable-next-line @typescript-eslint/no-unused-vars
import { Fragment } from 'preact';
import { useState } from 'preact/hooks';

import { AGENT_OPTIONS, TYPE_ACCEPTANCE_TEMPLATES } from './constants';
import { taskService } from '@1agents/core/services/taskService';
import type { Task, TaskPriority, TaskRecurrence, TaskType } from './types';

// Set of all type templates, so switching type can recognise an untouched
// template (and replace it) versus criteria the user actually wrote.
const ACCEPTANCE_TEMPLATE_VALUES = new Set(Object.values(TYPE_ACCEPTANCE_TEMPLATES).filter(Boolean));

interface CreateTaskFormProps {
    workspaceId: string;
    tasks: Task[];
    onCreated: () => void;
}

type RecurFreq = '' | 'daily' | 'weekly' | 'monthly';
type MonthMode = 'day' | 'weekday';
type RecurEnd = 'never' | 'until' | 'count';

const WEEKDAYS = ['日', '一', '二', '三', '四', '五', '六'];

export function CreateTaskForm({ workspaceId, tasks, onCreated }: CreateTaskFormProps) {
    const [title, setTitle] = useState('');
    const [type, setType] = useState<TaskType>('task');
    const [description, setDescription] = useState('');
    const [acceptance, setAcceptance] = useState('');
    const [priority, setPriority] = useState<TaskPriority>('medium');
    const [assignee, setAssignee] = useState('claudecode');
    const [labelsInput, setLabelsInput] = useState('');
    const [parentId, setParentId] = useState('');
    // Recurrence (superset): interval + multi-weekday + relative month + until/count.
    const [recurFreq, setRecurFreq] = useState<RecurFreq>('');
    const [recurInterval, setRecurInterval] = useState(1);
    const [recurDays, setRecurDays] = useState<number[]>([1]);
    const [recurMonthMode, setRecurMonthMode] = useState<MonthMode>('day');
    const [recurMonthday, setRecurMonthday] = useState(1);
    const [recurWeekIndex, setRecurWeekIndex] = useState(1);
    const [recurMonthWeekday, setRecurMonthWeekday] = useState(1);
    const [recurAt, setRecurAt] = useState('09:00');
    const [recurEnd, setRecurEnd] = useState<RecurEnd>('never');
    const [recurUntil, setRecurUntil] = useState('');
    const [recurCount, setRecurCount] = useState(10);
    const [checklist, setChecklist] = useState<string[]>([]);
    const [plannedStart, setPlannedStart] = useState('');
    const [plannedEnd, setPlannedEnd] = useState('');
    const [scheduleType, setScheduleType] = useState<'immediate' | 'scheduled'>('immediate');
    const [scheduledAt, setScheduledAt] = useState('');
    const [dependsOn, setDependsOn] = useState<string[]>([]);

    const resetForm = () => {
        setTitle('');
        setType('task');
        setDescription('');
        setAcceptance('');
        setPriority('medium');
        setAssignee('claudecode');
        setLabelsInput('');
        setParentId('');
        setRecurFreq('');
        setRecurInterval(1);
        setRecurDays([1]);
        setRecurMonthMode('day');
        setRecurEnd('never');
        setRecurUntil('');
        setChecklist([]);
        setPlannedStart('');
        setPlannedEnd('');
        setScheduleType('immediate');
        setScheduledAt('');
        setDependsOn([]);
    };

    // Switching type swaps in that type's acceptance template, but only while the
    // field is empty or still holds an untouched template — never clobber criteria
    // the user has actually edited.
    const handleTypeChange = (next: TaskType) => {
        setType(next);
        if (acceptance.trim() === '' || ACCEPTANCE_TEMPLATE_VALUES.has(acceptance)) {
            setAcceptance(TYPE_ACCEPTANCE_TEMPLATES[next] ?? '');
        }
    };

    const handleToggleDependency = (taskId: string) => {
        setDependsOn(prev => (prev.includes(taskId) ? prev.filter(id => id !== taskId) : [...prev, taskId]));
    };

    const toggleRecurDay = (d: number) => {
        setRecurDays(prev => (prev.includes(d) ? prev.filter(x => x !== d) : [...prev, d].sort((a, b) => a - b)));
    };

    const buildRecurrence = (): TaskRecurrence | null => {
        if (recurFreq === '') return null;
        const r: TaskRecurrence = { freq: recurFreq, at: recurAt };
        if (recurInterval > 1) r.interval = recurInterval;
        if (recurFreq === 'weekly') r.daysOfWeek = recurDays.length ? recurDays : [1];
        if (recurFreq === 'monthly') {
            if (recurMonthMode === 'weekday') {
                r.weekIndex = recurWeekIndex;
                r.daysOfWeek = [recurMonthWeekday];
            } else {
                r.monthday = recurMonthday;
            }
        }
        if (recurEnd === 'until' && recurUntil) r.until = recurUntil;
        if (recurEnd === 'count' && recurCount > 0) r.count = recurCount;
        return r;
    };

    const handleSubmit = async (e: Event) => {
        e.preventDefault();
        if (!title.trim()) return;

        try {
            const items = checklist.map(s => s.trim()).filter(Boolean);
            await taskService.create({
                workspace_id: workspaceId,
                title: title.trim(),
                type,
                description: description.trim(),
                acceptanceCriteria: acceptance.trim(),
                priority,
                assignee,
                labels: labelsInput
                    .split(/[,，]/)
                    .map(s => s.trim())
                    .filter(Boolean),
                parentId,
                recurrence: buildRecurrence(),
                checklist: items.length ? items.map(text => ({ text, done: false })) : undefined,
                scheduleType,
                scheduledAt: scheduleType === 'scheduled' && scheduledAt ? new Date(scheduledAt).toISOString() : null,
                plannedStart: plannedStart ? new Date(plannedStart).toISOString() : null,
                plannedEnd: plannedEnd ? new Date(plannedEnd).toISOString() : null,
                dependsOn,
            });
            resetForm();
            onCreated();
        } catch (err) {
            alert((err as Error).message);
        }
    };

    return (
        <form class="create-task-form" onSubmit={handleSubmit}>
            <div class="form-row">
                <div class="form-group" style={{ flex: 1 }}>
                    <label>标题</label>
                    <input
                        type="text"
                        placeholder="如: 完成新模块开发"
                        value={title}
                        onInput={(e: Event) => setTitle((e.target as HTMLInputElement).value)}
                        required
                    />
                </div>
                <div class="form-group">
                    <label>类型</label>
                    <select
                        value={type}
                        onChange={(e: Event) => handleTypeChange((e.target as HTMLSelectElement).value as TaskType)}
                    >
                        <option value="task">任务</option>
                        <option value="requirement">需求</option>
                        <option value="bug">缺陷</option>
                    </select>
                </div>
            </div>

            <div class="form-group">
                <label>描述（即交给 agent 的工作指令，支持 Markdown）</label>
                <textarea
                    rows={3}
                    placeholder="任务背景、目标、注意事项 —— 时间一到 agent 会按这段描述自动执行..."
                    value={description}
                    onInput={(e: Event) => setDescription((e.target as HTMLTextAreaElement).value)}
                />
            </div>

            <div class="form-group">
                <label>验收标准（必填，agent 完成后对照自查；留空则任务标记为「未就绪」，不进入调度队列）</label>
                <textarea
                    rows={3}
                    placeholder="如：hello.txt 存在且内容为 hello；所有测试通过..."
                    value={acceptance}
                    onInput={(e: Event) => setAcceptance((e.target as HTMLTextAreaElement).value)}
                />
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label>优先级</label>
                    <select
                        value={priority}
                        onChange={(e: Event) => setPriority((e.target as HTMLSelectElement).value as TaskPriority)}
                    >
                        <option value="urgent">紧急</option>
                        <option value="high">高</option>
                        <option value="medium">中</option>
                        <option value="low">低</option>
                    </select>
                </div>
                <div class="form-group">
                    <label>执行者</label>
                    <select
                        value={assignee}
                        onChange={(e: Event) => setAssignee((e.target as HTMLSelectElement).value)}
                    >
                        <option value="user">我自己（待办 / 提醒）</option>
                        {AGENT_OPTIONS.map(a => (
                            <option key={a} value={a}>
                                {a}
                            </option>
                        ))}
                    </select>
                </div>
                <div class="form-group">
                    <label>标签（逗号分隔）</label>
                    <input
                        type="text"
                        placeholder="如: 文档,高风险"
                        value={labelsInput}
                        onInput={(e: Event) => setLabelsInput((e.target as HTMLInputElement).value)}
                    />
                </div>
            </div>

            <div class="form-group">
                <label>清单（任务内子项，执行时逐条勾选）</label>
                <div class="task-checklist-editor">
                    {checklist.map((item, i) => (
                        <div class="task-checklist-row" key={i}>
                            <input
                                type="text"
                                placeholder={`子项 ${i + 1}`}
                                value={item}
                                onInput={(e: Event) =>
                                    setChecklist(prev =>
                                        prev.map((v, j) => (j === i ? (e.target as HTMLInputElement).value : v))
                                    )
                                }
                            />
                            <button
                                type="button"
                                class="task-checklist-remove"
                                onClick={() => setChecklist(prev => prev.filter((_, j) => j !== i))}
                            >
                                ×
                            </button>
                        </div>
                    ))}
                    <button
                        type="button"
                        class="task-checklist-add"
                        onClick={() => setChecklist(prev => [...prev, ''])}
                    >
                        + 添加子项
                    </button>
                </div>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label>父任务（子任务全部完成后父任务才执行）</label>
                    <select
                        value={parentId}
                        onChange={(e: Event) => setParentId((e.target as HTMLSelectElement).value)}
                    >
                        <option value="">无（顶层任务）</option>
                        {tasks
                            .filter(t => !t.parentId)
                            .map(t => (
                                <option key={t.id} value={t.id}>
                                    {t.title}
                                </option>
                            ))}
                    </select>
                </div>
                <div class="form-group">
                    <label>重复</label>
                    <select
                        value={recurFreq}
                        onChange={(e: Event) => setRecurFreq((e.target as HTMLSelectElement).value as RecurFreq)}
                    >
                        <option value="">不重复</option>
                        <option value="daily">每天</option>
                        <option value="weekly">每周</option>
                        <option value="monthly">每月</option>
                    </select>
                </div>
                {recurFreq !== '' && (
                    <div class="form-group">
                        <label>间隔（每 N 个周期）</label>
                        <input
                            type="number"
                            min={1}
                            value={recurInterval}
                            onChange={(e: Event) =>
                                setRecurInterval(Math.max(1, Number((e.target as HTMLInputElement).value)))
                            }
                        />
                    </div>
                )}
                {recurFreq !== '' && (
                    <div class="form-group">
                        <label>时间</label>
                        <input
                            type="time"
                            value={recurAt}
                            onChange={(e: Event) => setRecurAt((e.target as HTMLInputElement).value)}
                        />
                    </div>
                )}
            </div>

            {recurFreq === 'weekly' && (
                <div class="form-group">
                    <label>星期几（可多选，如「六 日」= 每周末）</label>
                    <div class="recur-weekday-toggles">
                        {WEEKDAYS.map((d, i) => (
                            <label key={d} class={`recur-weekday-toggle${recurDays.includes(i) ? ' active' : ''}`}>
                                <input
                                    type="checkbox"
                                    checked={recurDays.includes(i)}
                                    onChange={() => toggleRecurDay(i)}
                                />
                                <span>周{d}</span>
                            </label>
                        ))}
                    </div>
                </div>
            )}

            {recurFreq === 'monthly' && (
                <div class="form-row">
                    <div class="form-group">
                        <label>按</label>
                        <select
                            value={recurMonthMode}
                            onChange={(e: Event) =>
                                setRecurMonthMode((e.target as HTMLSelectElement).value as MonthMode)
                            }
                        >
                            <option value="day">按几号</option>
                            <option value="weekday">按第几个星期几</option>
                        </select>
                    </div>
                    {recurMonthMode === 'day' ? (
                        <div class="form-group">
                            <label>几号</label>
                            <input
                                type="number"
                                min={1}
                                max={31}
                                value={recurMonthday}
                                onChange={(e: Event) => setRecurMonthday(Number((e.target as HTMLInputElement).value))}
                            />
                        </div>
                    ) : (
                        <>
                            <div class="form-group">
                                <label>第几个</label>
                                <select
                                    value={String(recurWeekIndex)}
                                    onChange={(e: Event) =>
                                        setRecurWeekIndex(Number((e.target as HTMLSelectElement).value))
                                    }
                                >
                                    <option value="1">第一个</option>
                                    <option value="2">第二个</option>
                                    <option value="3">第三个</option>
                                    <option value="4">第四个</option>
                                    <option value="-1">最后一个</option>
                                </select>
                            </div>
                            <div class="form-group">
                                <label>星期几</label>
                                <select
                                    value={String(recurMonthWeekday)}
                                    onChange={(e: Event) =>
                                        setRecurMonthWeekday(Number((e.target as HTMLSelectElement).value))
                                    }
                                >
                                    {WEEKDAYS.map((d, i) => (
                                        <option key={d} value={String(i)}>
                                            周{d}
                                        </option>
                                    ))}
                                </select>
                            </div>
                        </>
                    )}
                </div>
            )}

            {recurFreq !== '' && (
                <div class="form-row">
                    <div class="form-group">
                        <label>结束方式</label>
                        <select
                            value={recurEnd}
                            onChange={(e: Event) => setRecurEnd((e.target as HTMLSelectElement).value as RecurEnd)}
                        >
                            <option value="never">永不</option>
                            <option value="until">到某日止</option>
                            <option value="count">共 N 次</option>
                        </select>
                    </div>
                    {recurEnd === 'until' && (
                        <div class="form-group">
                            <label>截止日期</label>
                            <input
                                type="date"
                                value={recurUntil}
                                onChange={(e: Event) => setRecurUntil((e.target as HTMLInputElement).value)}
                            />
                        </div>
                    )}
                    {recurEnd === 'count' && (
                        <div class="form-group">
                            <label>次数</label>
                            <input
                                type="number"
                                min={1}
                                value={recurCount}
                                onChange={(e: Event) =>
                                    setRecurCount(Math.max(1, Number((e.target as HTMLInputElement).value)))
                                }
                            />
                        </div>
                    )}
                </div>
            )}

            <div class="form-row">
                <div class="form-group">
                    <label>计划开始（即自动执行触发时间）</label>
                    <input
                        type="date"
                        value={plannedStart}
                        onChange={(e: Event) => setPlannedStart((e.target as HTMLInputElement).value)}
                    />
                </div>
                <div class="form-group">
                    <label>计划完成</label>
                    <input
                        type="date"
                        value={plannedEnd}
                        onChange={(e: Event) => setPlannedEnd((e.target as HTMLInputElement).value)}
                    />
                </div>
            </div>

            <div class="form-row">
                <div class="form-group">
                    <label>调度方式</label>
                    <select
                        value={scheduleType}
                        onChange={(e: Event) =>
                            setScheduleType((e.target as HTMLSelectElement).value as 'immediate' | 'scheduled')
                        }
                    >
                        <option value="immediate">立即排队</option>
                        <option value="scheduled">定时排队</option>
                    </select>
                </div>

                {scheduleType === 'scheduled' && (
                    <div class="form-group">
                        <label>执行时间</label>
                        <input
                            type="datetime-local"
                            value={scheduledAt}
                            onChange={(e: Event) => setScheduledAt((e.target as HTMLInputElement).value)}
                            required
                        />
                    </div>
                )}
            </div>

            {tasks.length > 0 && (
                <div class="form-group">
                    <label>前置依赖任务</label>
                    <div class="dependency-checklist">
                        {tasks.map(t => (
                            <label key={t.id} class="checkbox-label">
                                <input
                                    type="checkbox"
                                    checked={dependsOn.includes(t.id)}
                                    onChange={() => handleToggleDependency(t.id)}
                                />
                                <span>{t.title}</span>
                            </label>
                        ))}
                    </div>
                </div>
            )}

            <button type="submit" class="submit-task-btn">
                创建任务
            </button>
        </form>
    );
}
