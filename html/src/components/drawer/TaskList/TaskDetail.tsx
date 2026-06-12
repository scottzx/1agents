import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';

import { agentService } from '../../../services/agentService';
import type { AgentType, ChatSession, Session } from '../../types';
import { PRIORITY_LABELS, STATUS_LABELS } from './constants';
import type { Reply, ReplyMode, Task } from './types';
import { fmtDate, fmtDateOnly, recurrenceLabel } from './utils';

interface TaskDetailProps {
    workspaceId: string;
    taskId: string;
    allTasks: Task[];
    onBack: () => void;
    onDelete: (taskId: string) => void;
    onSelectSession?: (session: Session) => void;
}

export function TaskDetail({ workspaceId, taskId, allTasks, onBack, onDelete, onSelectSession }: TaskDetailProps) {
    const [task, setTask] = useState<Task | null>(null);
    const [error, setError] = useState('');

    // Description editing
    const [editingDesc, setEditingDesc] = useState(false);
    const [descDraft, setDescDraft] = useState('');

    // Acceptance criteria editing
    const [editingAccept, setEditingAccept] = useState(false);
    const [acceptDraft, setAcceptDraft] = useState('');

    // Reply composer
    const [replyText, setReplyText] = useState('');
    const [replyMode, setReplyMode] = useState<ReplyMode>('new');
    const [followUpTarget, setFollowUpTarget] = useState('');
    const [submitting, setSubmitting] = useState(false);

    const fetchTask = useCallback(async () => {
        try {
            const res = await fetch(`/api/agent/tasks/${encodeURIComponent(taskId)}`);
            if (!res.ok) {
                throw new Error(`Failed to load task: ${res.statusText}`);
            }
            setTask(await res.json());
            setError('');
        } catch (err) {
            setError((err as Error).message);
        }
    }, [taskId]);

    useEffect(() => {
        fetchTask();
        const timer = setInterval(fetchTask, 5000);
        return () => clearInterval(timer);
    }, [fetchTask]);

    const patchTask = async (patch: {
        description?: string;
        issueState?: 'open' | 'closed';
        acceptanceCriteria?: string;
    }) => {
        const res = await fetch(`/api/agent/tasks/${encodeURIComponent(taskId)}`, {
            method: 'PATCH',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(patch),
        });
        if (!res.ok) {
            throw new Error(await res.text());
        }
        setTask(await res.json());
    };

    const saveDescription = async () => {
        try {
            await patchTask({ description: descDraft });
            setEditingDesc(false);
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const saveAcceptance = async () => {
        try {
            await patchTask({ acceptanceCriteria: acceptDraft });
            setEditingAccept(false);
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const toggleIssueState = async () => {
        if (!task) return;
        const next = task.issueState === 'closed' ? 'open' : 'closed';
        try {
            await patchTask({ issueState: next });
        } catch (err) {
            alert((err as Error).message);
        }
    };

    // Open an EXISTING session (timeline link / follow-up). Resolves the
    // indexed record first so the chat resumes with its real identity
    // (name, acpSessionId) and shows up in the sidebar session list.
    const openSession = async (sessionId: string, agentType: string, replyId?: string) => {
        if (!onSelectSession || !task) return;
        let rec: ChatSession | null = null;
        try {
            rec = await agentService.get(sessionId);
        } catch {
            // fall through to the synthetic shape below
        }
        if (rec) {
            onSelectSession({
                ...rec,
                taskId: rec.taskId || task.id,
                replyId,
                active: true,
            });
            return;
        }
        // Legacy sessions that never got an index record: open with a
        // synthetic shape (the backend resolves resume state server-side).
        onSelectSession({
            kind: 'chat',
            id: sessionId,
            workspaceId,
            taskId: task.id,
            replyId,
            name: `${task.title} - 智能体`,
            agentType: (agentType || 'claudecode') as AgentType,
            ccProject: '',
            ccSessionId: '',
            sessionKey: '',
            status: 'idle',
            active: true,
        });
    };

    // Spawn a NEW session for a mode=new reply: index it first so it exists
    // in the sidebar immediately (with the task badge), then open it.
    const openNewSession = async (replyId: string) => {
        if (!onSelectSession || !task) return;
        const rec = await agentService.index({
            workspace_id: workspaceId,
            name: `${task.title} - 智能体`,
            agent_type: 'claudecode',
            task_id: task.id,
        });
        onSelectSession({ ...rec, taskId: task.id, replyId, active: true });
    };

    const submitReply = async (e: Event) => {
        e.preventDefault();
        if (!task || !replyText.trim() || submitting) return;
        setSubmitting(true);
        try {
            // Follow-up: link the new reply to the last agent reply of the
            // target session, so the timeline threads correctly.
            let inReplyTo = '';
            if (replyMode === 'follow_up' && followUpTarget) {
                const prior = [...(task.replies || [])]
                    .reverse()
                    .find(r => r.author.kind === 'agent' && r.sessionRef === followUpTarget);
                inReplyTo = prior?.id || '';
            }

            const res = await fetch(`/api/agent/tasks/${encodeURIComponent(taskId)}/replies`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    text: replyText.trim(),
                    mode: replyMode,
                    inReplyTo,
                }),
            });
            if (!res.ok) {
                throw new Error(await res.text());
            }
            const reply = (await res.json()) as Reply;
            setReplyText('');

            if (replyMode === 'new') {
                await openNewSession(reply.id);
            } else if (replyMode === 'follow_up' && followUpTarget) {
                const sess = task.sessions.find(s => s.id === followUpTarget);
                await openSession(followUpTarget, sess?.agentType || 'claudecode', reply.id);
            }
            fetchTask();
        } catch (err) {
            alert((err as Error).message);
        } finally {
            setSubmitting(false);
        }
    };

    if (!task) {
        return (
            <div class="task-dashboard-container">
                <div class="task-detail-header">
                    <button class="task-back-btn" onClick={onBack}>
                        ← 返回列表
                    </button>
                </div>
                {error ? <div class="task-error">{error}</div> : <div class="task-loading">载入任务...</div>}
            </div>
        );
    }

    const closed = task.issueState === 'closed';
    const deps = allTasks.filter(t => task.dependsOn?.includes(t.id));
    const subtasks = allTasks.filter(t => t.parentId === task.id);
    const replies = task.replies || [];

    return (
        <div class="task-dashboard-container task-detail-view">
            <div class="task-detail-header">
                <button class="task-back-btn" onClick={onBack}>
                    ← 返回列表
                </button>
                <div class="task-detail-title-group">
                    <h3 class="task-detail-title">
                        {'\u{1F4CB}'} {task.title}
                    </h3>
                    <span class="task-issue-icon" title={closed ? 'closed' : 'open'}>
                        {closed ? '\u{1F512}' : '\u{1F513}'}
                    </span>
                    <span class={`task-status-badge ${task.status}`}>
                        {task.status === 'running' && <span class="pulse-indicator" />}
                        {STATUS_LABELS[task.status] || task.status}
                    </span>
                </div>
                <div class="task-detail-actions">
                    <button class="task-issue-toggle-btn" onClick={toggleIssueState}>
                        {closed ? '\u{1F513} 重新打开' : '\u{1F512} 关闭 Issue'}
                    </button>
                </div>
            </div>

            <div class="task-detail-scroller">
                {/* Description */}
                <div class="task-desc-section">
                    <div class="task-section-header">
                        <h5>{'\u{1F4DD}'} 描述</h5>
                        {!editingDesc && (
                            <button
                                class="task-desc-edit-btn"
                                onClick={() => {
                                    setDescDraft(task.description || '');
                                    setEditingDesc(true);
                                }}
                            >
                                编辑
                            </button>
                        )}
                    </div>
                    {editingDesc ? (
                        <div class="task-desc-editor">
                            <textarea
                                rows={5}
                                value={descDraft}
                                onInput={(e: Event) => setDescDraft((e.target as HTMLTextAreaElement).value)}
                            />
                            <div class="task-desc-editor-actions">
                                <button onClick={saveDescription}>保存</button>
                                <button onClick={() => setEditingDesc(false)}>取消</button>
                            </div>
                        </div>
                    ) : (
                        <div class="task-desc-body">
                            {task.description ? (
                                <pre class="task-desc-text">{task.description}</pre>
                            ) : (
                                <span class="task-desc-empty">（暂无描述，点击编辑补充任务背景）</span>
                            )}
                        </div>
                    )}
                </div>

                {/* Acceptance criteria */}
                <div class="task-desc-section task-accept-section">
                    <div class="task-section-header">
                        <h5>✅ 验收标准</h5>
                        {!editingAccept && (
                            <button
                                class="task-desc-edit-btn"
                                onClick={() => {
                                    setAcceptDraft(task.acceptanceCriteria || '');
                                    setEditingAccept(true);
                                }}
                            >
                                编辑
                            </button>
                        )}
                    </div>
                    {editingAccept ? (
                        <div class="task-desc-editor">
                            <textarea
                                rows={3}
                                value={acceptDraft}
                                onInput={(e: Event) => setAcceptDraft((e.target as HTMLTextAreaElement).value)}
                            />
                            <div class="task-desc-editor-actions">
                                <button onClick={saveAcceptance}>保存</button>
                                <button onClick={() => setEditingAccept(false)}>取消</button>
                            </div>
                        </div>
                    ) : (
                        <div class="task-desc-body">
                            {task.acceptanceCriteria ? (
                                <pre class="task-desc-text">{task.acceptanceCriteria}</pre>
                            ) : (
                                <span class="task-desc-empty">
                                    （未设置 —— agent 执行完会按此自查，建议补充可验证的标准）
                                </span>
                            )}
                        </div>
                    )}
                </div>

                {/* Meta info */}
                <div class="task-meta-section">
                    <span class={`priority-badge priority-${task.priority || 'medium'}`}>
                        {PRIORITY_LABELS[task.priority || 'medium']}
                    </span>
                    <span>执行: {task.assignee || 'claudecode'}</span>
                    {(task.labels || []).map(l => (
                        <span key={l} class="task-label-tag">
                            {l}
                        </span>
                    ))}
                    {task.milestone && <span>🏁 {task.milestone}</span>}
                    {task.recurrence && <span>🔁 {recurrenceLabel(task.recurrence)}</span>}
                    {(task.retryCount ?? 0) > 0 && (
                        <span>
                            重试 {task.retryCount}/{task.maxRetries ?? 1}
                        </span>
                    )}
                    <span>
                        计划: {fmtDateOnly(task.plannedStart)} → {fmtDateOnly(task.plannedEnd)}
                    </span>
                    <span>
                        实际: {fmtDateOnly(task.startedAt)} → {fmtDateOnly(task.completedAt)}
                    </span>
                    {deps.length > 0 && (
                        <span class="task-meta-deps">
                            前置:{' '}
                            {deps.map(d => (
                                <span key={d.id} class="dep-tag">
                                    {d.status === 'completed' ? '✓ ' : ''}
                                    {d.title}
                                </span>
                            ))}
                        </span>
                    )}
                    <button class="task-delete-link" onClick={() => onDelete(task.id)}>
                        删除任务
                    </button>
                </div>

                {/* Subtasks */}
                {subtasks.length > 0 && (
                    <div class="task-subtasks-section">
                        <div class="task-section-header">
                            <h5>
                                子任务 ({subtasks.filter(s => s.status === 'completed').length}/{subtasks.length})
                            </h5>
                        </div>
                        {subtasks.map(st => (
                            <div key={st.id} class="task-subtask-row">
                                <span class={`task-status-badge ${st.status}`}>
                                    {STATUS_LABELS[st.status] || st.status}
                                </span>
                                <span class="task-subtask-title">{st.title}</span>
                            </div>
                        ))}
                    </div>
                )}

                {/* Timeline */}
                <div class="task-timeline-section">
                    <div class="task-section-header">
                        <h5>时间线 ({replies.length})</h5>
                    </div>
                    {replies.length === 0 ? (
                        <div class="no-sessions-hint">还没有回复 —— 在下方写第一条，开始这个话题。</div>
                    ) : (
                        <div class="task-timeline">
                            {replies.map(rp => {
                                const isAgent = rp.author.kind === 'agent';
                                const sess = rp.sessionRef
                                    ? task.sessions.find(s => s.id === rp.sessionRef)
                                    : undefined;
                                return (
                                    <div key={rp.id} class={`timeline-reply ${isAgent ? 'agent' : 'user'}`}>
                                        <div class="timeline-reply-meta">
                                            <span class="timeline-author">
                                                {isAgent ? '\u{1F916}' : '\u{1F4AC}'} {rp.author.name || rp.author.kind}
                                            </span>
                                            <span class="timeline-time">{fmtDate(rp.createdAt)}</span>
                                        </div>
                                        <div class="timeline-reply-text">{rp.text}</div>
                                        {rp.sessionRef && (
                                            <button
                                                class="timeline-session-link"
                                                onClick={() =>
                                                    openSession(
                                                        rp.sessionRef!,
                                                        sess?.agentType || rp.agentType || 'claudecode'
                                                    )
                                                }
                                            >
                                                {'\u{1F916}'} {isAgent ? '查看完整转录' : '查看会话'} →
                                            </button>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </div>

            {/* Reply composer */}
            <form class="task-reply-composer" onSubmit={submitReply}>
                <textarea
                    rows={3}
                    placeholder={
                        closed ? 'Issue 已关闭，仅可评论...' : '写回复：评论、布置新一轮工作，或追问已有会话...'
                    }
                    value={replyText}
                    onInput={(e: Event) => setReplyText((e.target as HTMLTextAreaElement).value)}
                />
                <div class="task-reply-controls">
                    <label class={`reply-mode-option${replyMode === 'pure_comment' ? ' active' : ''}`}>
                        <input
                            type="radio"
                            name="replyMode"
                            checked={replyMode === 'pure_comment'}
                            onChange={() => setReplyMode('pure_comment')}
                        />
                        {'\u{1F4AC}'} 纯评论
                    </label>
                    <label
                        class={`reply-mode-option${replyMode === 'new' ? ' active' : ''}${closed ? ' disabled' : ''}`}
                        title={closed ? 'Issue 已关闭，先重新打开' : ''}
                    >
                        <input
                            type="radio"
                            name="replyMode"
                            checked={replyMode === 'new'}
                            disabled={closed}
                            onChange={() => setReplyMode('new')}
                        />
                        {'\u{1F680}'} 启动新会话
                    </label>
                    <label
                        class={`reply-mode-option${replyMode === 'follow_up' ? ' active' : ''}${
                            closed || task.sessions.length === 0 ? ' disabled' : ''
                        }`}
                        title={
                            closed ? 'Issue 已关闭，先重新打开' : task.sessions.length === 0 ? '还没有会话可追问' : ''
                        }
                    >
                        <input
                            type="radio"
                            name="replyMode"
                            checked={replyMode === 'follow_up'}
                            disabled={closed || task.sessions.length === 0}
                            onChange={() => {
                                setReplyMode('follow_up');
                                if (!followUpTarget && task.sessions.length > 0) {
                                    setFollowUpTarget(task.sessions[task.sessions.length - 1].id);
                                }
                            }}
                        />
                        ↩️ 追问会话
                    </label>
                    {replyMode === 'follow_up' && (
                        <select
                            class="follow-up-target"
                            value={followUpTarget}
                            onChange={(e: Event) => setFollowUpTarget((e.target as HTMLSelectElement).value)}
                        >
                            {task.sessions.map((s, i) => (
                                <option key={s.id} value={s.id}>
                                    #{i + 1} {s.agentType} · {fmtDate(s.createdAt)}
                                </option>
                            ))}
                        </select>
                    )}
                    <button type="submit" class="task-reply-submit" disabled={submitting || !replyText.trim()}>
                        {submitting ? '提交中...' : '提交'}
                    </button>
                </div>
            </form>
        </div>
    );
}
