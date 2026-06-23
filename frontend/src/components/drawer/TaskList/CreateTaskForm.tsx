import { h } from 'preact';
import { useState } from 'preact/hooks';

import { AGENT_OPTIONS } from './constants';
import type { Task, TaskPriority, TaskType } from './types';

interface CreateTaskFormProps {
    workspaceId: string;
    tasks: Task[];
    onCreated: () => void;
}

type RecurFreq = '' | 'daily' | 'weekly' | 'monthly';

export function CreateTaskForm({ workspaceId, tasks, onCreated }: CreateTaskFormProps) {
    const [title, setTitle] = useState('');
    const [type, setType] = useState<TaskType>('task');
    const [description, setDescription] = useState('');
    const [acceptance, setAcceptance] = useState('');
    const [priority, setPriority] = useState<TaskPriority>('medium');
    const [assignee, setAssignee] = useState('claudecode');
    const [labelsInput, setLabelsInput] = useState('');
    const [parentId, setParentId] = useState('');
    const [recurFreq, setRecurFreq] = useState<RecurFreq>('');
    const [recurWeekday, setRecurWeekday] = useState(1);
    const [recurMonthday, setRecurMonthday] = useState(1);
    const [recurAt, setRecurAt] = useState('09:00');
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
        setPlannedStart('');
        setPlannedEnd('');
        setScheduleType('immediate');
        setScheduledAt('');
        setDependsOn([]);
    };

    const handleToggleDependency = (taskId: string) => {
        setDependsOn(prev => (prev.includes(taskId) ? prev.filter(id => id !== taskId) : [...prev, taskId]));
    };

    const handleSubmit = async (e: Event) => {
        e.preventDefault();
        if (!title.trim()) return;

        try {
            const recurrence =
                recurFreq === ''
                    ? null
                    : {
                          freq: recurFreq,
                          ...(recurFreq === 'weekly' ? { weekday: recurWeekday } : {}),
                          ...(recurFreq === 'monthly' ? { monthday: recurMonthday } : {}),
                          at: recurAt,
                      };
            const res = await fetch('/api/agent/tasks', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
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
                    recurrence,
                    scheduleType,
                    scheduledAt:
                        scheduleType === 'scheduled' && scheduledAt ? new Date(scheduledAt).toISOString() : null,
                    plannedStart: plannedStart ? new Date(plannedStart).toISOString() : null,
                    plannedEnd: plannedEnd ? new Date(plannedEnd).toISOString() : null,
                    dependsOn,
                }),
            });
            if (!res.ok) {
                throw new Error('Failed to create task');
            }
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
                        onChange={(e: Event) => setType((e.target as HTMLSelectElement).value as TaskType)}
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
                <label>验收标准（agent 完成后对照自查）</label>
                <textarea
                    rows={2}
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
                    <label>执行 Agent</label>
                    <select
                        value={assignee}
                        onChange={(e: Event) => setAssignee((e.target as HTMLSelectElement).value)}
                    >
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
                {recurFreq === 'weekly' && (
                    <div class="form-group">
                        <label>周几</label>
                        <select
                            value={String(recurWeekday)}
                            onChange={(e: Event) => setRecurWeekday(Number((e.target as HTMLSelectElement).value))}
                        >
                            {['日', '一', '二', '三', '四', '五', '六'].map((d, i) => (
                                <option key={d} value={String(i)}>
                                    周{d}
                                </option>
                            ))}
                        </select>
                    </div>
                )}
                {recurFreq === 'monthly' && (
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
