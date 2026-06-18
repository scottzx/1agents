import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import { t } from '../../../i18n';
import { agentService } from '../../../services/agentService';
import type { AgentType, ChatSession, Session } from '../../types';
import { getLinkRelLabels, getPriorityLabels, getStatusLabels } from './constants';
import type { LinkRel, Reply, ReplyMode, SessionMetadata, Task, TaskLink } from './types';
import { fmtDate, fmtDateOnly, recurrenceLabel } from './utils';
import { renderMarkdown, type MarkdownContext } from '../../../utils/markdown';
import { taskPermalink } from '../../../stores/taskNavStore';
import * as wsStore from '../../../stores/workspaceStore';
import * as ui from '../../../stores/uiStore';

interface TaskDetailProps {
    workspaceId: string;
    taskId: string;
    allTasks: Task[];
    onBack?: () => void;
    onDelete: (taskId: string) => void;
    onNavigate?: (taskId: string) => void;
    onSelectSession?: (session: Session) => void;
}

export function TaskDetail({
    workspaceId,
    taskId,
    allTasks,
    onBack,
    onDelete,
    onNavigate,
    onSelectSession,
}: TaskDetailProps) {
    const [task, setTask] = useState<Task | null>(null);
    const [error, setError] = useState('');

    // Add-link form
    const [linkTarget, setLinkTarget] = useState('');
    const [linkRel, setLinkRel] = useState<LinkRel>('relates');

    // Description editing
    const editingDesc = useSignal(false);
    const [descDraft, setDescDraft] = useState('');

    // Acceptance criteria editing
    const editingAccept = useSignal(false);
    const [acceptDraft, setAcceptDraft] = useState('');

    // Reply composer (top-level: pure comment or new session only)
    const [replyText, setReplyText] = useState('');
    const [replyMode, setReplyMode] = useState<ReplyMode>('new');
    const [submitting, setSubmitting] = useState(false);

    // Per-branch inline follow-up: which session's composer is open, its draft,
    // and a busy flag. Only one branch composer is open at a time.
    const followUpOpen = useSignal('');
    const followUpText = useSignal('');
    const followUpBusy = useSignal(false);

    // GitHub detail view tabs & preview state
    const [activeTab, setActiveTab] = useState<'conversation' | 'subtasks' | 'relations'>('conversation');
    const [composerTab, setComposerTab] = useState<'write' | 'preview'>('write');

    // Sidebar collapse toggle
    const sidebarCollapsed = useSignal(false);

    const getInitials = (name: string) => {
        if (!name) return '?';
        const clean = name.trim();
        if (clean.length === 0) return '?';
        if (clean.length <= 2) return clean.toUpperCase();
        return clean.slice(0, 2).toUpperCase();
    };

    // A single reply bubble, reused for standalone comments and branch children.
    const renderReplyCard = (rp: Reply) => {
        const isAgent = rp.author.kind === 'agent';
        return (
            <div key={rp.id} class={`gh-comment-card ${isAgent ? 'is-agent' : 'is-user'}`}>
                <div class="gh-comment-header">
                    <div class="gh-comment-header-left">
                        <span class="gh-avatar">{getInitials(rp.author.name || rp.author.kind)}</span>
                        <span class="gh-author-name">{rp.author.name || rp.author.kind}</span>
                        <span>{fmtDate(rp.createdAt)}</span>
                    </div>
                    <div class="gh-comment-actions">
                        <span class="gh-role-badge">{isAgent ? 'Agent' : 'User'}</span>
                    </div>
                </div>
                <div class="gh-comment-body">
                    <div
                        class="markdown-body timeline-reply-text"
                        dangerouslySetInnerHTML={{ __html: renderMarkdown(rp.text, mdCtx) }}
                    />
                </div>
            </div>
        );
    };

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
        links?: TaskLink[];
        status?: Task['status'];
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
            editingDesc.value = false;
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const saveAcceptance = async () => {
        try {
            await patchTask({ acceptanceCriteria: acceptDraft });
            editingAccept.value = false;
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

    const addLink = async () => {
        if (!task || !linkTarget) return;
        const links = task.links || [];
        if (links.some(l => l.target === linkTarget && l.rel === linkRel)) return; // already linked
        try {
            await patchTask({ links: [...links, { target: linkTarget, rel: linkRel }] });
            setLinkTarget('');
            setLinkRel('relates');
        } catch (err) {
            alert((err as Error).message);
        }
    };

    const removeLink = async (link: TaskLink) => {
        if (!task) return;
        const links = (task.links || []).filter(l => !(l.target === link.target && l.rel === link.rel));
        try {
            await patchTask({ links });
        } catch (err) {
            alert((err as Error).message);
        }
    };

    // Open an EXISTING session (timeline link / follow-up). Resolves the
    // indexed record first so the chat resumes with its real identity
    // (name, acpSessionId) and shows up in the sidebar session list.
    const openSession = async (sessionId: string, agentType: string, replyId?: string, initialMessage?: string) => {
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
                initialMessage,
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
            initialMessage,
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
    // in the sidebar immediately (with the task badge), then open it and
    // auto-send the reply text as the first prompt. The user turn itself is
    // recorded to the timeline server-side when the prompt runs.
    const openNewSession = async (initialMessage?: string) => {
        if (!onSelectSession || !task) return;
        const rec = await agentService.index({
            workspace_id: workspaceId,
            name: `${task.title} - 智能体`,
            agent_type: 'claudecode',
            task_id: task.id,
        });
        onSelectSession({ ...rec, taskId: task.id, initialMessage, active: true });
    };

    // Top-level composer: a pure comment (standalone timeline entry, no chat)
    // or the root of a new session branch. Follow-ups live inside each branch
    // (submitBranchFollowUp). For chat-driven modes the user turn is recorded
    // server-side (writeUserReply), so no reply is POSTed here.
    const submitReply = async (e: Event) => {
        e.preventDefault();
        if (!task || !replyText.trim() || submitting) return;
        const text = replyText.trim();
        setSubmitting(true);
        try {
            if (replyMode === 'new') {
                setReplyText('');
                await openNewSession(text);
            } else {
                const res = await fetch(`/api/agent/tasks/${encodeURIComponent(taskId)}/replies`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ text, mode: 'pure_comment' }),
                });
                if (!res.ok) {
                    throw new Error(await res.text());
                }
                setReplyText('');
            }
            fetchTask();
        } catch (err) {
            alert((err as Error).message);
        } finally {
            setSubmitting(false);
        }
    };

    // Follow up inside an existing session branch (issue-model: 追加会话一定
    //发生在已有会话下). Opens the live chat and auto-sends the text; the user
    // turn and the agent reply are both recorded server-side with SessionRef,
    // so they thread under this branch on the next poll.
    const submitBranchFollowUp = async (session: SessionMetadata) => {
        if (!task || !followUpText.value.trim() || followUpBusy.value) return;
        const text = followUpText.value.trim();
        followUpBusy.value = true;
        try {
            followUpText.value = '';
            followUpOpen.value = '';
            await openSession(session.id, session.agentType || 'claudecode', undefined, text);
            fetchTask();
        } catch (err) {
            alert((err as Error).message);
        } finally {
            followUpBusy.value = false;
        }
    };

    const lang = ui.language.value;
    const priorityLabels = getPriorityLabels(lang);
    const statusLabels = getStatusLabels(lang);
    const linkRelLabels = getLinkRelLabels(lang);

    if (!task) {
        return (
            <div class="task-dashboard-container">
                {error ? (
                    <div class="task-error">{error}</div>
                ) : (
                    <div class="task-loading">{t('task.detail.loading', lang)}</div>
                )}
            </div>
        );
    }

    const closed = task.issueState === 'closed';
    // Markdown context for this project: bare #N links resolve to this project
    // and link only when the number exists (GitHub-style existence check).
    const projectName = wsStore.workspaces.value.find(w => w.id === workspaceId)?.name || '';
    const mdCtx: MarkdownContext = {
        projectName,
        knownNumbers: new Set(allTasks.map(t => t.number).filter((n): n is number => !!n)),
    };
    const copyPermalink = () => {
        if (!projectName || !task.number) return;
        const url = taskPermalink(projectName, task.number);
        void navigator.clipboard?.writeText(url);
        ui.showToast(t('task.detail.permalinkCopied', lang).replace('{url}', url));
    };
    const deps = allTasks.filter(t => task.dependsOn?.includes(t.id));
    const subtasks = allTasks.filter(t => t.parentId === task.id);
    const replies = task.replies || [];

    // Group the flat reply list into a two-level tree: each session becomes a
    // top-level branch holding its replies in order; pure comments and replies
    // not yet linked to a session are standalone top-level nodes. Drill-down is
    // exactly one level — branch children are never themselves nested.
    type TimelineNode =
        | { kind: 'branch'; session: SessionMetadata; num: number; children: Reply[]; anchor: string }
        | { kind: 'comment'; reply: Reply; anchor: string };

    const sessionsById = new Map(task.sessions.map(s => [s.id, s]));
    const childrenBySession = new Map<string, Reply[]>();
    const looseReplies: Reply[] = [];
    for (const rp of replies) {
        if (rp.sessionRef && sessionsById.has(rp.sessionRef)) {
            const arr = childrenBySession.get(rp.sessionRef) || [];
            arr.push(rp);
            childrenBySession.set(rp.sessionRef, arr);
        } else {
            looseReplies.push(rp);
        }
    }
    const timelineNodes: TimelineNode[] = [];
    task.sessions.forEach((s, i) => {
        const children = (childrenBySession.get(s.id) || [])
            .slice()
            .sort((a, b) => a.createdAt.localeCompare(b.createdAt));
        if (children.length === 0) return; // session not linked to any reply yet
        timelineNodes.push({
            kind: 'branch',
            session: s,
            num: i + 1,
            children,
            anchor: children[0].createdAt,
        });
    });
    for (const rp of looseReplies) {
        timelineNodes.push({ kind: 'comment', reply: rp, anchor: rp.createdAt });
    }
    timelineNodes.sort((a, b) => a.anchor.localeCompare(b.anchor));

    // Peer cross-references (#N links). Outgoing come from this task; backlinks
    // are other tasks that reference this one. Both resolve titles via allTasks.
    const taskById = new Map(allTasks.map(t => [t.id, t]));
    const outgoing = task.links || [];
    const backlinks = allTasks.filter(t => t.id !== task.id && (t.links || []).some(l => l.target === task.id));
    const linkOptions = allTasks.filter(t => t.id !== task.id);
    const linkLabel = (tgt?: Task) =>
        tgt ? `${tgt.number ? `#${tgt.number} ` : ''}${tgt.title}` : t('task.detail.unknownTask', lang);

    // Subtask checks calculation
    const totalSubtasks = subtasks.length;
    const completedSubtasks = subtasks.filter(s => s.status === 'completed').length;
    const allSubtasksDone = totalSubtasks > 0 && completedSubtasks === totalSubtasks;

    // Acceptance criteria check
    const hasAcceptance = !!task.acceptanceCriteria;

    // Dependencies check
    const pendingDeps = deps.filter(d => d.status !== 'completed').length;
    const allDepsDone = pendingDeps === 0;

    return (
        <div class="task-dashboard-container task-detail-view">
            {/* GitHub style title header */}
            <div class="gh-header-top">
                {onBack && (
                    <button class="task-back-btn" onClick={onBack}>
                        {t('task.detail.back', lang)}
                    </button>
                )}
                <h3 class="gh-title">
                    {task.title} <span class="gh-number">#{task.number || ''}</span>
                </h3>
                <div class="gh-actions">
                    {task.number ? (
                        <button
                            class="task-permalink-btn"
                            title={t('task.detail.permalink', lang)}
                            onClick={copyPermalink}
                        >
                            🔗 链接
                        </button>
                    ) : null}
                    <button class="task-issue-toggle-btn" onClick={toggleIssueState}>
                        {closed ? t('task.detail.reopen', lang) : t('task.detail.close', lang)}
                    </button>
                </div>
            </div>

            <div class="gh-header-meta">
                <span class={`gh-status-badge ${closed ? 'closed' : 'open'}`}>{closed ? 'Closed' : 'Open'}</span>
                <span class="gh-meta-text">
                    <strong>{task.createdBy || 'scottzx'}</strong>{' '}
                    {closed ? t('task.detail.closedBy', lang) : t('task.detail.createdBy', lang)} · {replies.length}
                </span>
            </div>

            {/* GitHub style tab navigation */}
            <div class="gh-detail-tabs">
                <button
                    class={`gh-tab-btn ${activeTab === 'conversation' ? 'active' : ''}`}
                    onClick={() => setActiveTab('conversation')}
                >
                    💬 Conversation <span class="gh-tab-badge">{replies.length + (task.description ? 1 : 0)}</span>
                </button>
                <button
                    class={`gh-tab-btn ${activeTab === 'subtasks' ? 'active' : ''}`}
                    onClick={() => setActiveTab('subtasks')}
                >
                    📋 Checklist / Subtasks <span class="gh-tab-badge">{subtasks.length}</span>
                </button>
                <button
                    class={`gh-tab-btn ${activeTab === 'relations' ? 'active' : ''}`}
                    onClick={() => setActiveTab('relations')}
                >
                    🔗 Relations <span class="gh-tab-badge">{outgoing.length + backlinks.length}</span>
                </button>
                <button
                    class={`gh-sidebar-toggle-btn${sidebarCollapsed.value ? ' is-collapsed' : ''}`}
                    title={
                        sidebarCollapsed.value
                            ? t('task.detail.sidebarExpand', lang)
                            : t('task.detail.sidebarCollapse', lang)
                    }
                    onClick={() => (sidebarCollapsed.value = !sidebarCollapsed.value)}
                >
                    <span />
                    <span />
                    <span />
                </button>
            </div>

            <div class={`task-detail-scroller${sidebarCollapsed.value ? ' sidebar-collapsed' : ''}`}>
                <div class="task-detail-main">
                    {activeTab === 'conversation' && (
                        <div>
                            {/* Description Card */}
                            <div class="gh-comment-card is-user">
                                <div class="gh-comment-header">
                                    <div class="gh-comment-header-left">
                                        <span class="gh-avatar">{getInitials(task.createdBy || 'scottzx')}</span>
                                        <span class="gh-author-name">{task.createdBy || 'scottzx'}</span>
                                        <span>{t('task.detail.createdTask', lang)}</span>
                                    </div>
                                    <div class="gh-comment-actions">
                                        <span class="gh-role-badge">Author</span>
                                        {!editingDesc.value && (
                                            <button
                                                class="task-desc-edit-btn"
                                                onClick={() => {
                                                    setDescDraft(task.description || '');
                                                    editingDesc.value = true;
                                                }}
                                            >
                                                {t('common.edit', lang)}
                                            </button>
                                        )}
                                    </div>
                                </div>
                                <div class="gh-comment-body">
                                    {editingDesc.value ? (
                                        <div class="task-desc-editor">
                                            <textarea
                                                rows={5}
                                                value={descDraft}
                                                onInput={(e: Event) =>
                                                    setDescDraft((e.target as HTMLTextAreaElement).value)
                                                }
                                            />
                                            <div class="task-desc-editor-actions">
                                                <button onClick={saveDescription}>{t('common.save', lang)}</button>
                                                <button onClick={() => (editingDesc.value = false)}>
                                                    {t('common.cancel', lang)}
                                                </button>
                                            </div>
                                        </div>
                                    ) : task.description ? (
                                        <div
                                            class="markdown-body task-desc-md"
                                            dangerouslySetInnerHTML={{
                                                __html: renderMarkdown(task.description, mdCtx),
                                            }}
                                        />
                                    ) : (
                                        <span class="task-desc-empty">{t('task.detail.descEmpty', lang)}</span>
                                    )}
                                </div>
                            </div>

                            {/* Pinned Acceptance Criteria Card */}
                            <div class="gh-comment-card is-user">
                                <div class="gh-comment-header">
                                    <div class="gh-comment-header-left">
                                        <span>
                                            ✅ <strong>验收标准 (Acceptance Criteria)</strong>
                                        </span>
                                    </div>
                                    <div class="gh-comment-actions">
                                        {!editingAccept.value && (
                                            <button
                                                class="task-desc-edit-btn"
                                                onClick={() => {
                                                    setAcceptDraft(task.acceptanceCriteria || '');
                                                    editingAccept.value = true;
                                                }}
                                            >
                                                {t('common.edit', lang)}
                                            </button>
                                        )}
                                    </div>
                                </div>
                                <div class="gh-comment-body">
                                    {editingAccept.value ? (
                                        <div class="task-desc-editor">
                                            <textarea
                                                rows={3}
                                                value={acceptDraft}
                                                onInput={(e: Event) =>
                                                    setAcceptDraft((e.target as HTMLTextAreaElement).value)
                                                }
                                            />
                                            <div class="task-desc-editor-actions">
                                                <button onClick={saveAcceptance}>{t('common.save', lang)}</button>
                                                <button onClick={() => (editingAccept.value = false)}>
                                                    {t('common.cancel', lang)}
                                                </button>
                                            </div>
                                        </div>
                                    ) : task.acceptanceCriteria ? (
                                        <div
                                            class="markdown-body task-desc-md"
                                            dangerouslySetInnerHTML={{
                                                __html: renderMarkdown(task.acceptanceCriteria, mdCtx),
                                            }}
                                        />
                                    ) : (
                                        <span class="task-desc-empty">{t('task.detail.acceptanceEmpty', lang)}</span>
                                    )}
                                </div>
                            </div>

                            {/* Timeline: standalone comments + session branches */}
                            <div class="task-timeline">
                                {timelineNodes.map(node => {
                                    if (node.kind === 'comment') {
                                        return renderReplyCard(node.reply);
                                    }
                                    const { session, num, children } = node;
                                    const lastChildId = children[children.length - 1]?.id;
                                    const running = session.status === 'running';
                                    return (
                                        <div key={session.id} class="task-branch">
                                            <div class="task-branch-header">
                                                <span class="task-branch-badge">
                                                    {t('task.detail.sessionBadge', lang).replace('{num}', String(num))}
                                                </span>
                                                <span class="task-branch-agent">{session.agentType}</span>
                                                <span class={`task-branch-status${running ? ' running' : ''}`}>
                                                    {running
                                                        ? t('task.detail.sessionRunning', lang)
                                                        : t('task.detail.sessionIdle', lang)}
                                                </span>
                                            </div>
                                            <div class="task-branch-children">
                                                {children.map(rp => renderReplyCard(rp))}
                                            </div>
                                            <div class="task-branch-actions">
                                                {!closed && (
                                                    <button
                                                        type="button"
                                                        class="task-branch-followup-btn"
                                                        onClick={() => {
                                                            followUpOpen.value =
                                                                followUpOpen.value === session.id ? '' : session.id;
                                                            followUpText.value = '';
                                                        }}
                                                    >
                                                        {t('task.detail.followupBtn', lang)}
                                                    </button>
                                                )}
                                                <button
                                                    type="button"
                                                    class="timeline-session-link"
                                                    onClick={() =>
                                                        openSession(
                                                            session.id,
                                                            session.agentType || 'claudecode',
                                                            lastChildId
                                                        )
                                                    }
                                                >
                                                    {t('task.detail.openSession', lang)}
                                                </button>
                                            </div>
                                            {followUpOpen.value === session.id && (
                                                <div class="task-branch-followup">
                                                    <textarea
                                                        rows={3}
                                                        placeholder={t('task.detail.followupPlaceholder', lang).replace(
                                                            '{num}',
                                                            String(num)
                                                        )}
                                                        value={followUpText.value}
                                                        onInput={(e: Event) =>
                                                            (followUpText.value = (
                                                                e.target as HTMLTextAreaElement
                                                            ).value)
                                                        }
                                                    />
                                                    <div class="task-branch-followup-actions">
                                                        <button
                                                            type="button"
                                                            class="gh-close-btn"
                                                            onClick={() => (followUpOpen.value = '')}
                                                        >
                                                            {t('common.cancel', lang)}
                                                        </button>
                                                        <button
                                                            type="button"
                                                            class="gh-submit-btn"
                                                            disabled={followUpBusy.value || !followUpText.value.trim()}
                                                            onClick={() => submitBranchFollowUp(session)}
                                                        >
                                                            {followUpBusy.value
                                                                ? t('task.detail.followupSubmitting', lang)
                                                                : t('task.detail.followupRun', lang)}
                                                        </button>
                                                    </div>
                                                </div>
                                            )}
                                        </div>
                                    );
                                })}
                            </div>

                            {/* Merge / Checks Status Box */}
                            <div class="gh-merge-box">
                                <div class={`gh-merge-icon-col status-${task.status}`}>
                                    {task.status === 'completed' && '✓'}
                                    {task.status === 'running' && '●'}
                                    {task.status === 'failed' && '✗'}
                                    {(task.status === 'pending' || task.status === 'queued') && '◷'}
                                    {task.status === 'cancelled' && '⊘'}
                                    {task.status === 'blocked' && '⚠'}
                                </div>
                                <div class="gh-merge-content">
                                    <h4 class="gh-merge-title">
                                        {task.status === 'completed' && t('task.detail.mergeTitle.completed', lang)}
                                        {task.status === 'running' && t('task.detail.mergeTitle.running', lang)}
                                        {task.status === 'failed' && t('task.detail.mergeTitle.failed', lang)}
                                        {(task.status === 'pending' || task.status === 'queued') &&
                                            t('task.detail.mergeTitle.queued', lang)}
                                        {task.status === 'cancelled' && t('task.detail.mergeTitle.cancelled', lang)}
                                        {task.status === 'blocked' && t('task.detail.mergeTitle.blocked', lang)}
                                    </h4>
                                    <p class="gh-merge-desc">{t('task.detail.checksDesc', lang)}</p>

                                    <div class="gh-check-item">
                                        <span
                                            class={`gh-check-status ${allSubtasksDone || totalSubtasks === 0 ? 'pass' : 'warn'}`}
                                        >
                                            {allSubtasksDone || totalSubtasks === 0 ? '✓' : '⚠'}
                                        </span>
                                        <span>
                                            {t('task.detail.subtaskCheck', lang)
                                                .replace('{done}', String(completedSubtasks))
                                                .replace('{total}', String(totalSubtasks))}
                                        </span>
                                    </div>

                                    <div class="gh-check-item">
                                        <span class={`gh-check-status ${hasAcceptance ? 'pass' : 'warn'}`}>
                                            {hasAcceptance ? '✓' : 'warn'}
                                        </span>
                                        <span>
                                            {t('task.detail.acceptanceLabel', lang)}
                                            {hasAcceptance
                                                ? t('task.detail.acceptanceDefined', lang)
                                                : t('task.detail.acceptanceNotSet', lang)}
                                        </span>
                                    </div>

                                    <div class="gh-check-item">
                                        <span class={`gh-check-status ${allDepsDone ? 'pass' : 'fail'}`}>
                                            {allDepsDone ? '✓' : 'fail'}
                                        </span>
                                        <span>
                                            {t('task.detail.depsLabel', lang)}
                                            {allDepsDone
                                                ? t('task.detail.depsOk', lang)
                                                : t('task.detail.depsPending', lang).replace(
                                                      '{n}',
                                                      String(pendingDeps)
                                                  )}
                                        </span>
                                    </div>
                                </div>
                                <div class="gh-merge-actions">
                                    {task.status === 'completed' && (
                                        <button class="gh-merge-btn btn-todo" onClick={toggleIssueState}>
                                            {t('task.detail.reopenTask', lang)}
                                        </button>
                                    )}
                                    {task.status === 'running' && (
                                        <button
                                            class="gh-merge-btn btn-running"
                                            onClick={() => patchTask({ status: 'cancelled' })}
                                        >
                                            {t('task.detail.cancelExec', lang)}
                                        </button>
                                    )}
                                    {task.status === 'failed' && (
                                        <button class="gh-merge-btn btn-todo" onClick={() => openNewSession('retry')}>
                                            {t('task.detail.retryExec', lang)}
                                        </button>
                                    )}
                                    {(task.status === 'pending' || task.status === 'queued') && (
                                        <button class="gh-merge-btn" onClick={() => openNewSession('start')}>
                                            {t('task.detail.startAgent', lang)}
                                        </button>
                                    )}
                                </div>
                            </div>

                            {/* GitHub style composer */}
                            <div class="gh-composer-card">
                                <div class="gh-composer-tabs">
                                    <button
                                        class={`gh-composer-tab ${composerTab === 'write' ? 'active' : ''}`}
                                        type="button"
                                        onClick={() => setComposerTab('write')}
                                    >
                                        Write
                                    </button>
                                    <button
                                        class={`gh-composer-tab ${composerTab === 'preview' ? 'active' : ''}`}
                                        type="button"
                                        onClick={() => setComposerTab('preview')}
                                    >
                                        Preview
                                    </button>
                                </div>

                                <div class="gh-composer-body">
                                    {composerTab === 'write' ? (
                                        <textarea
                                            rows={4}
                                            placeholder={
                                                closed
                                                    ? 'Issue 已关闭，仅可评论...'
                                                    : '写回复：评论、布置新一轮工作，或追问已有会话...'
                                            }
                                            value={replyText}
                                            onInput={(e: Event) =>
                                                setReplyText((e.target as HTMLTextAreaElement).value)
                                            }
                                        />
                                    ) : replyText.trim() ? (
                                        <div
                                            class="gh-preview-box markdown-body"
                                            dangerouslySetInnerHTML={{ __html: renderMarkdown(replyText, mdCtx) }}
                                        />
                                    ) : (
                                        <div class="gh-preview-box">Nothing to preview</div>
                                    )}
                                </div>

                                <div class="gh-composer-footer">
                                    <div class="gh-composer-options">
                                        <label class={`gh-opt-label ${replyMode === 'pure_comment' ? 'active' : ''}`}>
                                            <input
                                                type="radio"
                                                name="replyMode"
                                                style={{ display: 'none' }}
                                                checked={replyMode === 'pure_comment'}
                                                onChange={() => setReplyMode('pure_comment')}
                                            />
                                            {t('task.detail.commentMode', lang)}
                                        </label>
                                        <label
                                            class={`gh-opt-label ${replyMode === 'new' ? 'active' : ''} ${closed ? 'disabled' : ''}`}
                                            title={closed ? 'Issue 已关闭，先重新打开' : ''}
                                        >
                                            <input
                                                type="radio"
                                                name="replyMode"
                                                style={{ display: 'none' }}
                                                checked={replyMode === 'new'}
                                                disabled={closed}
                                                onChange={() => setReplyMode('new')}
                                            />
                                            {t('task.detail.newSessionMode', lang)}
                                        </label>
                                        <span class="gh-opt-hint">{t('task.detail.followupHint', lang)}</span>
                                    </div>
                                    <div class="gh-composer-actions">
                                        <button type="button" class="gh-close-btn" onClick={toggleIssueState}>
                                            {closed ? 'Reopen Issue' : 'Close Issue'}
                                        </button>
                                        <button
                                            type="button"
                                            class="gh-submit-btn"
                                            disabled={submitting || !replyText.trim()}
                                            onClick={submitReply}
                                        >
                                            {submitting ? 'Submitting...' : 'Comment'}
                                        </button>
                                    </div>
                                </div>
                            </div>
                        </div>
                    )}

                    {activeTab === 'subtasks' && (
                        <div class="gh-subtasks-tab-content">
                            <h4 style={{ margin: '0 0 16px 0', fontSize: '15px' }}>
                                Checklist ({completedSubtasks}/{totalSubtasks})
                            </h4>
                            {subtasks.length === 0 ? (
                                <div class="task-desc-empty">{t('task.detail.subtasksEmpty', lang)}</div>
                            ) : (
                                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                                    {subtasks.map(st => (
                                        <div
                                            key={st.id}
                                            class="gh-comment-card"
                                            style={{ marginBottom: '8px', padding: '12px' }}
                                        >
                                            <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                                                <input type="checkbox" checked={st.status === 'completed'} disabled />
                                                <span
                                                    class={`priority-badge priority-${st.priority || 'medium'}`}
                                                    style={{ fontSize: '10px' }}
                                                >
                                                    {priorityLabels[st.priority || 'medium']}
                                                </span>
                                                <span
                                                    style={{
                                                        fontSize: '13.5px',
                                                        textDecoration:
                                                            st.status === 'completed' ? 'line-through' : 'none',
                                                    }}
                                                >
                                                    {st.title}
                                                </span>
                                                <span
                                                    class={`task-status-badge ${st.status}`}
                                                    style={{ marginLeft: 'auto', fontSize: '10.5px' }}
                                                >
                                                    {statusLabels[st.status] || st.status}
                                                </span>
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}

                    {activeTab === 'relations' && (
                        <div class="gh-relations-tab-content">
                            <h4 style={{ margin: '0 0 16px 0', fontSize: '15px' }}>
                                {t('task.detail.relationsTitle', lang)}
                            </h4>

                            <div style={{ marginBottom: '24px' }}>
                                <h5 style={{ margin: '0 0 8px 0', fontSize: '12.5px', color: 'var(--text-secondary)' }}>
                                    {t('task.detail.outgoingTitle', lang)}
                                </h5>
                                {outgoing.length === 0 ? (
                                    <div class="task-desc-empty" style={{ marginBottom: '12px' }}>
                                        {t('task.detail.outgoingEmpty', lang)}
                                    </div>
                                ) : (
                                    <div
                                        style={{
                                            display: 'flex',
                                            flexDirection: 'column',
                                            gap: '8px',
                                            marginBottom: '16px',
                                        }}
                                    >
                                        {outgoing.map(link => {
                                            const tgt = taskById.get(link.target);
                                            return (
                                                <div
                                                    key={`${link.target}-${link.rel}`}
                                                    class="task-link-row"
                                                    style={{
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        gap: '8px',
                                                        padding: '8px',
                                                        border: '1px solid var(--border-color)',
                                                        borderRadius: '6px',
                                                    }}
                                                >
                                                    <span
                                                        class={`task-link-rel rel-${link.rel}`}
                                                        style={{
                                                            fontSize: '11px',
                                                            padding: '2px 6px',
                                                            borderRadius: '4px',
                                                        }}
                                                    >
                                                        {linkRelLabels[link.rel] || link.rel}
                                                    </span>
                                                    <button
                                                        class="task-link-target"
                                                        disabled={!tgt || !onNavigate}
                                                        onClick={() => tgt && onNavigate && onNavigate(tgt.id)}
                                                        style={{
                                                            background: 'none',
                                                            border: 'none',
                                                            color: 'var(--accent-color)',
                                                            cursor: 'pointer',
                                                            textAlign: 'left',
                                                            fontWeight: '500',
                                                        }}
                                                    >
                                                        {linkLabel(tgt)}
                                                    </button>
                                                    <button
                                                        class="task-link-remove"
                                                        title="移除关联"
                                                        onClick={() => removeLink(link)}
                                                        style={{
                                                            background: 'none',
                                                            border: 'none',
                                                            color: 'var(--text-muted)',
                                                            cursor: 'pointer',
                                                            fontSize: '16px',
                                                            marginLeft: 'auto',
                                                        }}
                                                    >
                                                        ×
                                                    </button>
                                                </div>
                                            );
                                        })}
                                    </div>
                                )}
                            </div>

                            <div style={{ marginBottom: '24px' }}>
                                <h5 style={{ margin: '0 0 8px 0', fontSize: '12.5px', color: 'var(--text-secondary)' }}>
                                    {t('task.detail.backlinkTitle', lang)}
                                </h5>
                                {backlinks.length === 0 ? (
                                    <div class="task-desc-empty" style={{ marginBottom: '12px' }}>
                                        {t('task.detail.backlinkEmpty', lang)}
                                    </div>
                                ) : (
                                    <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                                        {backlinks.map(src => {
                                            const link = (src.links || []).find(l => l.target === task.id);
                                            return (
                                                <div
                                                    key={src.id}
                                                    class="task-link-row"
                                                    style={{
                                                        display: 'flex',
                                                        alignItems: 'center',
                                                        gap: '8px',
                                                        padding: '8px',
                                                        border: '1px solid var(--border-color)',
                                                        borderRadius: '6px',
                                                    }}
                                                >
                                                    <span
                                                        class={`task-link-rel rel-${link?.rel || 'relates'}`}
                                                        style={{
                                                            fontSize: '11px',
                                                            padding: '2px 6px',
                                                            borderRadius: '4px',
                                                        }}
                                                    >
                                                        {linkRelLabels[link?.rel || 'relates']}
                                                    </span>
                                                    <button
                                                        class="task-link-target"
                                                        disabled={!onNavigate}
                                                        onClick={() => onNavigate && onNavigate(src.id)}
                                                        style={{
                                                            background: 'none',
                                                            border: 'none',
                                                            color: 'var(--accent-color)',
                                                            cursor: 'pointer',
                                                            textAlign: 'left',
                                                            fontWeight: '500',
                                                        }}
                                                    >
                                                        {linkLabel(src)}
                                                    </button>
                                                </div>
                                            );
                                        })}
                                    </div>
                                )}
                            </div>

                            {/* Add link form */}
                            <div class="gh-comment-card" style={{ padding: '16px' }}>
                                <h5 style={{ margin: '0 0 12px 0', fontSize: '12.5px' }}>
                                    {t('task.detail.addLinkTitle', lang)}
                                </h5>
                                <div style={{ display: 'flex', gap: '10px', flexWrap: 'wrap' }}>
                                    <select
                                        class="task-link-target-select"
                                        value={linkTarget}
                                        onChange={(e: Event) => setLinkTarget((e.target as HTMLSelectElement).value)}
                                        style={{
                                            flex: 1,
                                            minWidth: '150px',
                                            padding: '6px',
                                            borderRadius: '6px',
                                            border: '1px solid var(--border-color)',
                                            backgroundColor: 'var(--bg-card)',
                                            color: 'var(--text-main)',
                                        }}
                                    >
                                        <option value="">{t('task.detail.linkTargetPlaceholder', lang)}</option>
                                        {linkOptions.map(tgt => (
                                            <option key={tgt.id} value={tgt.id}>
                                                {linkLabel(tgt)}
                                            </option>
                                        ))}
                                    </select>
                                    <select
                                        class="task-link-rel-select"
                                        value={linkRel}
                                        onChange={(e: Event) =>
                                            setLinkRel((e.target as HTMLSelectElement).value as LinkRel)
                                        }
                                        style={{
                                            padding: '6px',
                                            borderRadius: '6px',
                                            border: '1px solid var(--border-color)',
                                            backgroundColor: 'var(--bg-card)',
                                            color: 'var(--text-main)',
                                        }}
                                    >
                                        <option value="relates">{t('task.link.relates', lang)}</option>
                                        <option value="closes">{t('task.link.closes', lang)}</option>
                                    </select>
                                    <button
                                        class="gh-submit-btn"
                                        disabled={!linkTarget}
                                        onClick={addLink}
                                        style={{
                                            padding: '6px 14px',
                                            borderRadius: '6px',
                                            backgroundColor: 'var(--accent-color)',
                                            color: '#fff',
                                            border: 'none',
                                            fontWeight: '600',
                                            cursor: 'pointer',
                                        }}
                                    >
                                        {t('task.detail.addLink', lang)}
                                    </button>
                                </div>
                            </div>
                        </div>
                    )}
                </div>

                <div class={`task-detail-sidebar${sidebarCollapsed.value ? ' is-collapsed' : ''}`}>
                    {/* Assignees */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Assignees</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                                <span class="gh-avatar">{getInitials(task.assignee || 'claudecode')}</span>
                                <span>{task.assignee || 'claudecode'}</span>
                            </div>
                        </div>
                    </div>

                    {/* Labels */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Labels</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {(task.labels || []).length === 0 ? (
                                <span class="gh-no-item">None yet</span>
                            ) : (
                                <div style={{ display: 'flex', gap: '4px', flexWrap: 'wrap' }}>
                                    {(task.labels || []).map(l => (
                                        <span key={l} class="task-label-tag">
                                            {l}
                                        </span>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>

                    {/* Milestone */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Milestone</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {task.milestone ? (
                                <div>🏁 {task.milestone}</div>
                            ) : (
                                <span class="gh-no-item">No milestone</span>
                            )}
                        </div>
                    </div>

                    {/* Priority */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Priority</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            <span class={`priority-badge priority-${task.priority || 'medium'}`}>
                                {priorityLabels[task.priority || 'medium']}
                            </span>
                        </div>
                    </div>

                    {/* Dates & Schedule */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Dates & Schedule</span>
                        </div>
                        <div
                            class="gh-sidebar-body"
                            style={{
                                display: 'flex',
                                flexDirection: 'column',
                                gap: '6px',
                                fontSize: '12px',
                                color: 'var(--text-secondary)',
                            }}
                        >
                            {task.recurrence && <div>🔁 {recurrenceLabel(task.recurrence)}</div>}
                            {(task.retryCount ?? 0) > 0 && (
                                <div>
                                    {t('task.detail.dateRetry', lang)
                                        .replace('{count}', String(task.retryCount))
                                        .replace('{max}', String(task.maxRetries ?? 1))}
                                </div>
                            )}
                            <div>
                                {t('task.detail.datePlanned', lang)} {fmtDateOnly(task.plannedStart)} →{' '}
                                {fmtDateOnly(task.plannedEnd)}
                            </div>
                            <div>
                                {t('task.detail.dateActual', lang)} {fmtDateOnly(task.startedAt)} →{' '}
                                {fmtDateOnly(task.completedAt)}
                            </div>
                        </div>
                    </div>

                    {/* Development sessions */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>Development</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {task.sessions.length === 0 ? (
                                <span class="gh-no-item">No sessions active</span>
                            ) : (
                                <div style={{ display: 'flex', flexDirection: 'column', gap: '6px' }}>
                                    {task.sessions.map((s, idx) => (
                                        <button
                                            key={s.id}
                                            onClick={() => openSession(s.id, s.agentType)}
                                            style={{
                                                background: 'none',
                                                border: '1px solid var(--border-color)',
                                                borderRadius: '6px',
                                                padding: '6px',
                                                textAlign: 'left',
                                                cursor: 'pointer',
                                                fontSize: '12.5px',
                                                color: 'var(--text-main)',
                                                display: 'flex',
                                                alignItems: 'center',
                                                justifyContent: 'space-between',
                                                width: '100%',
                                            }}
                                        >
                                            <span>
                                                #{idx + 1} {s.agentType}
                                            </span>
                                            <span
                                                class={`task-status-badge ${s.status === 'running' ? 'running' : 'completed'}`}
                                                style={{ fontSize: '10px' }}
                                            >
                                                {s.status === 'running'
                                                    ? t('task.detail.sessionRunningBadge', lang)
                                                    : t('task.detail.sessionIdleBadge', lang)}
                                            </span>
                                        </button>
                                    ))}
                                </div>
                            )}
                        </div>
                    </div>

                    {/* Actions */}
                    <div class="gh-sidebar-panel" style={{ borderBottom: 'none' }}>
                        <div class="gh-sidebar-head">
                            <span>Danger Zone</span>
                        </div>
                        <div class="gh-sidebar-body">
                            <button class="task-delete-link" onClick={() => onDelete(task.id)}>
                                {t('task.detail.deleteBtn', lang)}
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    );
}
