import { h } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import { t } from '../../../i18n';
import { agentService } from '../../../services/agentService';
import { projectItemService } from '@1agents/core/services/taskService';
import type { AgentType, ChatSession, Session } from '../../types';
import { AGENT_OPTIONS, getLinkRelLabels, getPriorityLabels, getStatusLabels } from './constants';
import type { ChecklistItem, Reply, SessionMetadata, ProjectItem, TaskLink } from './types';
import { fmtDate, fmtDateOnly, recurrenceLabel } from './utils';
import { renderMarkdown, type MarkdownContext } from '../../../utils/markdown';
import { renderMermaidBlocks } from '../../../utils/mermaid';
import { parseFrontmatter } from '../../../utils/frontmatter';
import { taskPermalink } from '../../../stores/taskNavStore';
import * as wsStore from '../../../stores/workspaceStore';
import * as sessionStore from '../../../stores/sessionStore';
import * as ui from '../../../stores/uiStore';

// Tab / control icons — feather-style outline, inherit color via currentColor
// (matches the inline-SVG convention in SessionsView/TaskTable).
const ConversationIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z" />
    </svg>
);

const SubtasksIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <polyline points="9 11 12 14 22 4" />
        <path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11" />
    </svg>
);

const RelationsIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71" />
        <path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71" />
    </svg>
);

const PropertiesIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <rect x="3" y="3" width="18" height="18" rx="2" />
        <line x1="15" y1="3" x2="15" y2="21" />
    </svg>
);

const PermalinkIcon = () => (
    <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
    >
        <path d="M9 17H7A5 5 0 0 1 7 7h2" />
        <path d="M15 7h2a5 5 0 0 1 0 10h-2" />
        <line x1="8" y1="12" x2="16" y2="12" />
    </svg>
);

// Issue status icon — open = ring with a dot (in progress), closed = check ring.
const StatusIcon = ({ closed }: { closed: boolean }) =>
    closed ? (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="12" cy="12" r="9" />
            <path d="M8.5 12.5l2.5 2.5 4.5-5" />
        </svg>
    ) : (
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="12" cy="12" r="9" />
            <circle cx="12" cy="12" r="3" fill="currentColor" stroke="none" />
        </svg>
    );

interface TaskDetailProps {
    workspaceId: string;
    taskId: string;
    allTasks: ProjectItem[];
    onBack?: () => void;
    onDelete: (taskId: string) => void;
    onNavigate?: (taskId: string) => void;
    onSelectSession?: (session: Session) => void;
}

// Card content is YAML-frontmatter Markdown: the frontmatter holds the
// machine-recognizable acceptance criteria, the body is free prose. These
// templates seed an empty editor so the author (or AI) fills the right shape —
// the bug body's 复现/期望/实际 sections also satisfy the confirm gate's check.
const REQ_DESC_TEMPLATE = ['---', 'acceptance:', '  - ', '---', '## 背景', '', '## 过程', '', '## 预期结果', ''].join(
    '\n'
);

const BUG_DESC_TEMPLATE = [
    '---',
    'acceptance:',
    '  - ',
    '---',
    '## 现象',
    '',
    '## 复现步骤',
    '1. ',
    '2. ',
    '',
    '## 期望结果',
    '',
    '## 实际结果',
    '',
].join('\n');

// Pick the seed template for an empty card by type.
function cardTemplate(type: string | undefined): string {
    if (type === 'bug') return BUG_DESC_TEMPLATE;
    if (type === 'requirement') return REQ_DESC_TEMPLATE;
    return '';
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
    const [task, setTask] = useState<ProjectItem | null>(null);
    const [error, setError] = useState('');

    // Add-link form — disabled: links are created by agent only; UI shows results.
    // const [linkTarget, setLinkTarget] = useState('');
    // const [linkRel, setLinkRel] = useState<LinkRel>('relates');

    // Description editing
    const editingDesc = useSignal(false);
    const [descDraft, setDescDraft] = useState('');

    // Reply composer (top-level: always starts new session with chosen agent)
    const [replyText, setReplyText] = useState('');
    const [composerAgent, setComposerAgent] = useState<string>('grok');
    const [includeTimeline, setIncludeTimeline] = useState(true);
    const [submitting, setSubmitting] = useState(false);

    // Per-branch inline follow-up: which session's composer is open, its draft,
    // and a busy flag. Only one branch composer is open at a time.
    const followUpOpen = useSignal('');
    const followUpText = useSignal('');
    const followUpBusy = useSignal(false);

    // Which session branches are expanded. Default collapsed; running branches
    // auto-expand so in-progress work is always visible.
    const expandedBranches = useSignal(new Set<string>());
    const toggleBranch = (id: string) => {
        const next = new Set(expandedBranches.value);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        expandedBranches.value = next;
    };

    // GitHub detail view tabs
    const [activeTab, setActiveTab] = useState<'conversation' | 'subtasks' | 'relations'>('conversation');

    // Sidebar collapse toggle (desktop) / bottom-drawer open (mobile).
    const sidebarCollapsed = useSignal(true);
    const drawerOpen = useSignal(false);

    // One control, two behaviours: on narrow screens the properties panel is a
    // bottom sheet (toggle drawerOpen); on wide screens it's a collapsible
    // right column (toggle sidebarCollapsed).
    const toggleSidebar = () => {
        if (typeof window !== 'undefined' && window.matchMedia('(max-width: 899px)').matches) {
            drawerOpen.value = !drawerOpen.value;
        } else {
            sidebarCollapsed.value = !sidebarCollapsed.value;
        }
    };

    const containerRef = useRef<HTMLDivElement>(null);
    // Main column holding the markdown bodies (description, acceptance, timeline
    // replies, composer preview); used to draw any ```mermaid placeholders.
    const mainRef = useRef<HTMLDivElement>(null);
    // Raw bytes of the last task payload, for poll de-duplication (see fetchTask).
    const lastRawRef = useRef('');

    useEffect(() => {
        const el = containerRef.current;
        if (!el || !onBack) return;

        let startX = 0,
            startY = 0,
            active = false;

        const onDown = (e: PointerEvent) => {
            const rect = el.getBoundingClientRect();
            if (e.clientX - rect.left > 40) return;
            startX = e.clientX;
            startY = e.clientY;
            active = true;
            el.setPointerCapture(e.pointerId);
        };
        const onMove = (e: PointerEvent) => {
            if (!active) return;
            const dx = e.clientX - startX;
            const dy = Math.abs(e.clientY - startY);
            if (dy > Math.abs(dx) + 5) {
                active = false;
                return;
            }
            if (dx < 0) return;
            el.style.transform = `translateX(${dx}px)`;
            el.style.transition = 'none';
        };
        const onUp = (e: PointerEvent) => {
            if (!active) return;
            active = false;
            const dx = e.clientX - startX;
            if (dx > 80) {
                el.style.transition = 'transform 0.22s ease';
                el.style.transform = 'translateX(110%)';
                setTimeout(onBack!, 220);
            } else {
                el.style.transition = 'transform 0.25s ease';
                el.style.transform = '';
            }
        };

        el.addEventListener('pointerdown', onDown);
        el.addEventListener('pointermove', onMove);
        el.addEventListener('pointerup', onUp);
        el.addEventListener('pointercancel', onUp);
        return () => {
            el.removeEventListener('pointerdown', onDown);
            el.removeEventListener('pointermove', onMove);
            el.removeEventListener('pointerup', onUp);
            el.removeEventListener('pointercancel', onUp);
        };
    }, [onBack]);

    // Draw ```mermaid diagrams in any markdown body of the main column once its
    // HTML is in the DOM. renderMarkdown already emits .mermaid-block
    // placeholders; this swaps them for SVG (idempotent, re-runs on theme/tab/
    // content change). Reading the theme signal here re-renders on light/dark
    // toggle so diagrams repaint with the matching palette.
    const detailTheme = ui.theme.value;
    useEffect(() => {
        void renderMermaidBlocks(mainRef.current, detailTheme);
    }, [task, detailTheme, activeTab, replyText, editingDesc.value]);

    const getInitials = (name: string) => {
        if (!name) return '?';
        const clean = name.trim();
        if (clean.length === 0) return '?';
        if (clean.length <= 2) return clean.toUpperCase();
        return clean.slice(0, 2).toUpperCase();
    };

    const replyExcerpt = (text: string, max = 50) => {
        const plain = text
            .replace(/```[\s\S]*?```/g, ' ')
            .replace(/[#*_`>\-[\]()]/g, ' ')
            .replace(/\s+/g, ' ')
            .trim();
        return plain.length > max ? `${plain.slice(0, max)}...` : plain;
    };

    // A single reply bubble, reused for standalone comments and branch children.
    const renderReplyCard = (rp: Reply) => {
        const isAgent = rp.author.kind === 'agent';
        return (
            <div key={rp.id} class={`gh-comment-card topic-reply-card ${isAgent ? 'is-agent' : 'is-user'}`}>
                <div class="gh-comment-header">
                    <div class="gh-comment-header-left">
                        <span class="gh-avatar">{getInitials(rp.author.name || rp.author.kind)}</span>
                        <span class="gh-author-name">{rp.author.name || rp.author.kind}</span>
                        <span>{isAgent ? '回复并处理了这个话题' : '回复了这个话题'}</span>
                        <span class="topic-reply-time">{fmtDate(rp.createdAt)}</span>
                    </div>
                    <div class="gh-comment-actions">
                        <span class="gh-role-badge">{isAgent ? 'Agent' : t('task.detail.roleUser', lang)}</span>
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

    const renderBranchNode = (session: SessionMetadata, num: number, children: Reply[]) => {
        const lastChildId = children[children.length - 1]?.id;
        const running = session.status === 'running';
        // Running branches auto-expand so in-progress work is always visible.
        const isExpanded = expandedBranches.value.has(session.id) || running;
        const toggleFollowUp = () => {
            followUpOpen.value = followUpOpen.value === session.id ? '' : session.id;
            followUpText.value = '';
        };

        return (
            <div key={session.id} class={`task-branch${isExpanded ? ' is-expanded' : ''}`}>
                <button
                    type="button"
                    class="task-branch-header"
                    onClick={() => {
                        if (!running) toggleBranch(session.id);
                    }}
                >
                    <span class="task-branch-caret">{isExpanded ? '▾' : '▸'}</span>
                    <span class="task-branch-badge">#{num} Agent 回帖</span>
                    <span class="task-branch-agent">{session.agentType}</span>
                    <span class={`task-branch-status${running ? ' running' : ''}`}>
                        {running ? '正在处理' : '已处理'}
                    </span>
                    {children.length > 0 && <span class="task-branch-summary">{replyExcerpt(children[0].text)}</span>}
                </button>
                {isExpanded && children.length > 0 && (
                    <div class="task-branch-children">{children.map(rp => renderReplyCard(rp))}</div>
                )}
                {isExpanded && (
                    <div class="task-branch-actions">
                        {!closed && (
                            <button type="button" class="task-branch-followup-btn" onClick={toggleFollowUp}>
                                {t('task.detail.followupBtn', lang)}
                            </button>
                        )}
                        <button
                            type="button"
                            class="timeline-session-link"
                            onClick={() => openSession(session.id, session.agentType || 'claudecode', lastChildId)}
                        >
                            {t('task.detail.openSession', lang)}
                        </button>
                    </div>
                )}
                {isExpanded && followUpOpen.value === session.id && (
                    <div class="task-branch-followup">
                        <textarea
                            rows={3}
                            placeholder={t('task.detail.followupPlaceholder', lang).replace('{num}', String(num))}
                            value={followUpText.value}
                            onInput={(e: Event) => (followUpText.value = (e.target as HTMLTextAreaElement).value)}
                        />
                        <div class="task-branch-followup-actions">
                            <button type="button" class="gh-close-btn" onClick={() => (followUpOpen.value = '')}>
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
    };

    const renderExecutionBox = () => (
        <div class="gh-merge-box topic-execution-box">
            <div class={`gh-merge-icon-col status-${task!.status}`}>
                {task!.status === 'completed' && '✓'}
                {task!.status === 'running' && '●'}
                {task!.status === 'failed' && '✗'}
                {(task!.status === 'pending' || task!.status === 'queued') && '◷'}
                {task!.status === 'cancelled' && '⊘'}
                {task!.status === 'blocked' && '⚠'}
            </div>
            <div class="gh-merge-content">
                <h4 class="gh-merge-title">
                    {task!.status === 'completed' && t('task.detail.mergeTitle.completed', lang)}
                    {task!.status === 'running' && t('task.detail.mergeTitle.running', lang)}
                    {task!.status === 'failed' && t('task.detail.mergeTitle.failed', lang)}
                    {(task!.status === 'pending' || task!.status === 'queued') &&
                        t('task.detail.mergeTitle.queued', lang)}
                    {task!.status === 'cancelled' && t('task.detail.mergeTitle.cancelled', lang)}
                    {task!.status === 'blocked' && t('task.detail.mergeTitle.blocked', lang)}
                </h4>
                <p class="gh-merge-desc">{t('task.detail.checksDesc', lang)}</p>

                <div class="gh-check-item">
                    <span class={`gh-check-status ${allSubtasksDone || totalSubtasks === 0 ? 'pass' : 'warn'}`}>
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
                        {hasAcceptance ? '✓' : '⚠'}
                    </span>
                    <span>
                        {t('task.detail.acceptanceLabel', lang)}
                        {hasAcceptance
                            ? t('task.detail.acceptanceDefined', lang)
                            : t('task.detail.acceptanceNotSet', lang)}
                    </span>
                </div>

                <div class="gh-check-item">
                    <span class={`gh-check-status ${allDepsDone ? 'pass' : 'fail'}`}>{allDepsDone ? '✓' : '✕'}</span>
                    <span>
                        {t('task.detail.depsLabel', lang)}
                        {allDepsDone
                            ? t('task.detail.depsOk', lang)
                            : t('task.detail.depsPending', lang).replace('{n}', String(pendingDeps))}
                    </span>
                </div>
            </div>
            <div class="gh-merge-actions">
                {task!.status === 'completed' && (
                    <button class="gh-merge-btn btn-todo" onClick={toggleIssueState}>
                        {t('task.detail.reopenTask', lang)}
                    </button>
                )}
                {task!.status === 'running' && (
                    <button class="gh-merge-btn btn-running" onClick={() => patchTask({ status: 'cancelled' })}>
                        {t('task.detail.cancelExec', lang)}
                    </button>
                )}
                {task!.status === 'failed' && (
                    <button class="gh-merge-btn btn-todo" onClick={() => openNewSession('retry')}>
                        {t('task.detail.retryExec', lang)}
                    </button>
                )}
                {(task!.status === 'pending' || task!.status === 'queued') && (
                    <button class="gh-merge-btn" onClick={() => openNewSession('start')}>
                        {t('task.detail.startAgent', lang)}
                    </button>
                )}
            </div>
        </div>
    );

    const fetchTask = useCallback(async () => {
        try {
            // Skip the state update (and the full re-render + markdown re-parse it
            // triggers) when the polled payload is byte-identical to the last one.
            // Go's JSON encoding is deterministic, so unchanged state → same bytes.
            const text = await projectItemService.getText(taskId);
            setError('');
            if (text === lastRawRef.current) return;
            lastRawRef.current = text;
            setTask(JSON.parse(text));
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
        status?: ProjectItem['status'];
        userConfirm?: boolean;
        checklist?: ChecklistItem[];
        assignee?: string;
    }) => {
        setTask(await projectItemService.patch(taskId, patch));
    };

    // Checklist edits: toggle a single item's done, append, or remove — each
    // rewrites the whole array and persists via PATCH.
    const [newChecklistText, setNewChecklistText] = useState('');
    const updateChecklist = (next: ChecklistItem[]) => patchTask({ checklist: next });
    const toggleChecklistItem = (idx: number) => {
        const list = task?.checklist ?? [];
        updateChecklist(list.map((c, i) => (i === idx ? { ...c, done: !c.done } : c)));
    };
    const removeChecklistItem = (idx: number) => {
        const list = task?.checklist ?? [];
        updateChecklist(list.filter((_, i) => i !== idx));
    };
    const addChecklistItem = () => {
        const text = newChecklistText.trim();
        if (!text) return;
        updateChecklist([...(task?.checklist ?? []), { text, done: false }]);
        setNewChecklistText('');
    };

    const saveDescription = async () => {
        try {
            await patchTask({ description: descDraft });
            editingDesc.value = false;
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

    // Sync composer agent from task assignee (so properties change updates composer)
    useEffect(() => {
        if (task) {
            setComposerAgent(task.assignee || 'grok');
        }
    }, [task]);

    // Add-link form — disabled: links are created by agent only.
    // const addLink = async () => {
    //     if (!task || !linkTarget) return;
    //     const links = task.links || [];
    //     if (links.some(l => l.target === linkTarget && l.rel === linkRel)) return;
    //     try {
    //         await patchTask({ links: [...links, { target: linkTarget, rel: linkRel }] });
    //         setLinkTarget('');
    //         setLinkRel('relates');
    //     } catch (err) {
    //         alert((err as Error).message);
    //     }
    // };

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

    // Spawn a NEW session via unified SessionSetup (P1-3): same modal/skip as
    // the main path, agent prefilled from assignee or chosen composer agent, taskId bound,
    // optional initialMessage auto-sent after create. PM paths stay on createPMSession.
    const openNewSession = async (
        initialMessage?: string,
        agentType?: AgentType,
        taskId?: string,
        includeTimeline = true
    ) => {
        if (!task) return;

        // Build rich background summary for the seeded initial message
        let seedMessage = `任务背景摘要（${task.title}）\n\n`;
        seedMessage += `描述：${task.description || '(无描述)'}\n`;
        if (task.acceptanceCriteria) {
            seedMessage += `验收标准：${task.acceptanceCriteria.replace(/\n/g, '；')}\n`;
        }
        seedMessage += `类型：${task.type || '需求'}\n`;
        if (task.assignee) seedMessage += `负责人：${task.assignee}\n`;

        if (includeTimeline && task.replies && task.replies.length > 0) {
            seedMessage += '\n之前回复摘要（前 3 条）：\n';
            const recent = task.replies.slice(0, 3);
            recent.forEach((rp, i) => {
                const who = rp.sessionRef ? `agent (${rp.agentType || rp.author?.name})` : rp.author?.name || '用户';
                seedMessage += `[${i + 1}] ${who} @ ${rp.createdAt?.slice(0, 10) || '未知'}：${rp.text?.slice(0, 120) || '(无内容)'}\n`;
            });
            seedMessage += '...（完整时间线由后端自动注入）';
        } else {
            seedMessage += '\n（仅任务摘要，未加载完整时间线）';
        }

        void sessionStore.openSessionSetup({
            workspaceId,
            locked: true,
            defaultAgent: (agentType || task.assignee || 'claudecode') as AgentType,
            initialMessage: initialMessage || seedMessage,
            taskId: taskId || task.id,
        });
    };

    // 讨论需求：a discussion is deliberately fuzzy, so we never auto-flip its
    // type. Instead we open a PM conversation seeded with the discussion and let
    // the PM clarify the deliverable, then create the requirement when it's clear.
    const convertToRequirement = async () => {
        if (!task) return;
        // The discussion thread is injected as background by the backend, so the
        // seed stays short — and the whole conversation is recorded back to this
        // discussion's timeline (the session is linked via taskId).
        const prompt =
            '我们把这条讨论聊成一条清晰的需求吧。请先和我澄清：交付物到底是什么、验收标准是什么；聊清楚后用 create_task 建一条 requirement（必要时拆成任务）。如果发现还不够清晰，就先保留为讨论。';
        await sessionStore.createPMSession(workspaceId, `讨论需求：${task.title}`, prompt, task.id);
    };

    // 与 AI 讨论(需求/缺陷):open a PM session linked to this issue to clarify its
    // scope/boundary before it's confirmed for scheduling. The card stays a
    // requirement/bug; the conversation is recorded back to its timeline.
    const discussIssue = async () => {
        if (!task) return;
        const isBug = task.type === 'bug';
        const kind = isBug ? '缺陷' : '需求';
        const need = isBug
            ? '描述（现象 + 复现步骤 + 期望结果 vs 实际结果）、验收标准（怎样算修好）'
            : '描述（要解决什么 + 范围边界）、验收标准（怎样算做对、做完）';
        const prompt = `我们来把这条${kind}的边界聊清楚，并补全这些必填要素：${need}。\n\n请先问我还不清楚的地方；澄清后请帮我把这些内容完善好（后续你可用任务工具回填到这张卡片）。补全后我才能点「确认，可排期」，再由你拆成可执行的任务。`;
        await sessionStore.createPMSession(workspaceId, `讨论${kind}：${task.title}`, prompt, task.id);
    };

    // What's still missing before a requirement/bug can be confirmed: title +
    // description + acceptance, and for bugs the description must also cover
    // 复现/期望/实际 (kept as a Markdown template inside description, so this is
    // a keyword check — the real quality comes from the 与 AI 讨论 pass).
    const confirmBlockers = (): string[] => {
        if (!task) return [];
        const { acceptance, body } = parseFrontmatter(task.description);
        const miss: string[] = [];
        if (!task.title?.trim()) miss.push('标题');
        if (!body.trim()) miss.push('描述');
        if (!acceptance.some(a => a.trim())) miss.push('验收标准（frontmatter acceptance）');
        if (task.type === 'bug') {
            if (!body.includes('复现')) miss.push('复现步骤');
            if (!body.includes('期望')) miss.push('期望结果');
            if (!body.includes('实际')) miss.push('实际结果');
        }
        return miss;
    };

    // Toggle the user-confirmed gate. Confirmation is a lightweight signal, not a
    // hard gate: agents may decompose an agreed requirement into sub-tasks and
    // schedule them without a per-item confirmation round. If the essentials are
    // still thin we surface a skippable nudge rather than blocking the toggle.
    const toggleUserConfirm = async () => {
        if (!task) return;
        if (!task.userConfirm) {
            const miss = confirmBlockers();
            if (
                miss.length &&
                !confirm(`还差：${miss.join('、')}。\n可点「与 AI 讨论」让 AI 帮你完善。仍要标记为已确认吗？`)
            ) {
                return;
            }
        }
        await patchTask({ userConfirm: !task.userConfirm });
    };

    // Top-level composer: always starts new session with the chosen agent.
    // Follow-ups live inside each branch (submitBranchFollowUp).
    const submitReply = async (e: Event) => {
        e.preventDefault();
        if (!task || !replyText.trim() || submitting) return;
        const text = replyText.trim();
        setSubmitting(true);
        try {
            await openNewSession(text, composerAgent as AgentType, task.id);
            setReplyText('');
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
        if (children.length === 0 && s.status !== 'running') return;
        timelineNodes.push({
            kind: 'branch',
            session: s,
            num: i + 1,
            children,
            anchor: children[0]?.createdAt ?? s.createdAt,
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
    // const linkOptions = allTasks.filter(t => t.id !== task.id);
    const linkLabel = (tgt?: ProjectItem) =>
        tgt ? `${tgt.number ? `#${tgt.number} ` : ''}${tgt.title}` : t('task.detail.unknownTask', lang);

    // Subtask checks calculation
    const totalSubtasks = subtasks.length;
    const completedSubtasks = subtasks.filter(s => s.status === 'completed').length;
    const allSubtasksDone = totalSubtasks > 0 && completedSubtasks === totalSubtasks;

    // Card content is YAML-frontmatter Markdown: structured keys (acceptance)
    // come from the frontmatter, the prose body is what we render/edit-display.
    const parsed = parseFrontmatter(task.description);
    const acceptanceLines = parsed.acceptance.filter(a => a.trim());

    // Acceptance criteria check (frontmatter is the source; fall back to the
    // legacy column for pre-frontmatter rows).
    const hasAcceptance = acceptanceLines.length > 0 || !!task.acceptanceCriteria;

    // Dependencies check
    const pendingDeps = deps.filter(d => d.status !== 'completed').length;
    const allDepsDone = pendingDeps === 0;

    // A discussion is a free-form concept record, not an executable work item:
    // hide the task-only panels (acceptance criteria, execution/checks box,
    // assignee) so its detail page stays focused on the conversation.
    const isDiscussion = task.type === 'discussion';
    // Requirements and bugs are open/closed issue items too: like discussions
    // they are non-executable (the PM breaks confirmed ones into tasks), so they
    // share the discussion-style detail page — conversation + 与 AI 讨论, no
    // acceptance/checks/assignee/composer. isNonExecutable gates those panels.
    const isIssueItem = task.type === 'requirement' || task.type === 'bug';
    const isNonExecutable = isDiscussion || isIssueItem;
    const typeLabel =
        task.type === 'discussion'
            ? '讨论'
            : task.type === 'requirement'
              ? '需求'
              : task.type === 'bug'
                ? '缺陷'
                : '任务';
    const statusLabel = statusLabels[task.status] || task.status;
    const issueStateLabel = closed ? t('task.detail.statusClosed', lang) : t('task.detail.statusOpen', lang);

    return (
        <div class="task-dashboard-container task-detail-view" ref={containerRef}>
            {/* GitHub style title header — status icon · title · permalink */}
            <div class="gh-header-top">
                <span
                    class={`gh-status-icon ${closed ? 'closed' : 'open'}`}
                    title={closed ? t('task.detail.statusClosed', lang) : t('task.detail.statusOpen', lang)}
                >
                    <StatusIcon closed={closed} />
                </span>
                <div class="topic-title-group">
                    <div class="topic-kicker">
                        <span class="topic-type-chip">{typeLabel}</span>
                        <span>{issueStateLabel}</span>
                        <span class="topic-dot">·</span>
                        <span class={`task-status-badge ${task.status}`}>{statusLabel}</span>
                        {task.milestone && (
                            <span class="topic-meta-pair">
                                <span class="topic-dot">·</span>
                                <span>{task.milestone}</span>
                            </span>
                        )}
                        {task.number ? (
                            <button
                                class="task-permalink-btn"
                                title={t('task.detail.permalink', lang)}
                                onClick={copyPermalink}
                            >
                                <PermalinkIcon />
                            </button>
                        ) : null}
                    </div>
                    <h3 class="gh-title">
                        {task.title} <span class="gh-number">#{task.number || ''}</span>
                    </h3>
                </div>
                <div class="gh-actions">
                    {task.type === 'discussion' && (
                        <button class="task-convert-btn" onClick={convertToRequirement}>
                            {t('task.discussion.convert', lang)}
                        </button>
                    )}
                    {isIssueItem && (
                        <button class="task-convert-btn" onClick={discussIssue}>
                            与 AI 讨论
                        </button>
                    )}
                    {isIssueItem && (
                        <button
                            class={`task-confirm-btn${task.userConfirm ? ' confirmed' : ''}`}
                            onClick={toggleUserConfirm}
                            title="标记需求已和你对齐（非排期硬门槛，agent 可据此直接拆解排期）"
                        >
                            {task.userConfirm ? '已确认 ✓' : '确认，可排期'}
                        </button>
                    )}
                </div>
            </div>

            {/* GitHub style tab navigation */}
            <div class="gh-detail-tabs">
                <button
                    class={`gh-tab-btn ${activeTab === 'conversation' ? 'active' : ''}`}
                    onClick={() => setActiveTab('conversation')}
                >
                    <span class="gh-tab-icon">
                        <ConversationIcon />
                    </span>
                    <span class="gh-tab-label">{t('task.detail.tabConversation', lang)}</span>
                    <span class="gh-tab-badge">{replies.length + (task.description ? 1 : 0)}</span>
                </button>
                <button
                    class={`gh-tab-btn ${activeTab === 'subtasks' ? 'active' : ''}`}
                    onClick={() => setActiveTab('subtasks')}
                >
                    <span class="gh-tab-icon">
                        <SubtasksIcon />
                    </span>
                    <span class="gh-tab-label">{t('task.detail.tabSubtasks', lang)}</span>
                    <span class="gh-tab-badge">{subtasks.length}</span>
                </button>
                <button
                    class={`gh-tab-btn ${activeTab === 'relations' ? 'active' : ''}`}
                    onClick={() => setActiveTab('relations')}
                >
                    <span class="gh-tab-icon">
                        <RelationsIcon />
                    </span>
                    <span class="gh-tab-label">{t('task.detail.tabRelations', lang)}</span>
                    <span class="gh-tab-badge">{outgoing.length + backlinks.length}</span>
                </button>
                {/* Mobile-only: open the properties bottom sheet */}
                <button class="gh-properties-btn" onClick={() => (drawerOpen.value = true)}>
                    <PropertiesIcon /> {t('task.detail.propertiesBtn', lang)}
                </button>
                {/* Desktop-only: collapse the right column */}
                <button
                    class={`gh-sidebar-toggle-btn${sidebarCollapsed.value ? ' is-collapsed' : ''}`}
                    title={
                        sidebarCollapsed.value
                            ? t('task.detail.sidebarExpand', lang)
                            : t('task.detail.sidebarCollapse', lang)
                    }
                    onClick={toggleSidebar}
                >
                    <span />
                    <span />
                    <span />
                </button>
            </div>

            <div class={`task-detail-scroller${sidebarCollapsed.value ? ' sidebar-collapsed' : ''}`}>
                <div class="task-detail-main" ref={mainRef}>
                    {activeTab === 'conversation' && (
                        <div class="topic-stream">
                            {/* Main topic post */}
                            <div class="gh-comment-card topic-main-post is-user">
                                <div class="gh-comment-header">
                                    <div class="gh-comment-header-left">
                                        <span class="gh-avatar">{getInitials(task.createdBy || 'scottzx')}</span>
                                        <span class="gh-author-name">{task.createdBy || 'scottzx'}</span>
                                        <span>发布了主帖</span>
                                    </div>
                                    <div class="gh-comment-actions">
                                        <span class="gh-role-badge">楼主</span>
                                        {!editingDesc.value && (
                                            <button
                                                class="task-desc-edit-btn"
                                                onClick={() => {
                                                    // Seed the frontmatter template for an empty
                                                    // requirement/bug so acceptance + sections are
                                                    // ready to fill.
                                                    setDescDraft(task.description || cardTemplate(task.type));
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
                                    ) : parsed.body ? (
                                        <div
                                            class="markdown-body task-desc-md"
                                            dangerouslySetInnerHTML={{
                                                __html: renderMarkdown(parsed.body, mdCtx),
                                            }}
                                        />
                                    ) : (
                                        <span class="task-desc-empty">{t('task.detail.descEmpty', lang)}</span>
                                    )}
                                </div>
                            </div>

                            {/* Acceptance criteria — compact L1 section (edited via description frontmatter). */}
                            {!isDiscussion && (
                                <div class="task-l1-section">
                                    <div class="task-l1-section-header">
                                        <span class="task-l1-section-title">
                                            {t('task.detail.acceptanceTitle', lang)}
                                        </span>
                                        <span class="gh-acceptance-hint">在「描述」的 frontmatter 中编辑</span>
                                    </div>
                                    {acceptanceLines.length > 0 ? (
                                        <ul class="acceptance-list">
                                            {acceptanceLines.map((a, i) => (
                                                <li key={i}>{a}</li>
                                            ))}
                                        </ul>
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
                            )}

                            {/* Checklist — compact L1 section: tick / add / remove, persisted via PATCH. */}
                            {!isDiscussion && (
                                <div class="task-l1-section task-l1-checklist">
                                    <div class="task-l1-section-header">
                                        <span class="task-l1-section-title">清单</span>
                                        {(task.checklist?.length ?? 0) > 0 && (
                                            <span class="task-checklist-progress">
                                                {task.checklist!.filter(c => c.done).length}/{task.checklist!.length}
                                            </span>
                                        )}
                                    </div>
                                    {(task.checklist?.length ?? 0) > 0 && (
                                        <ul class="task-checklist-view">
                                            {task.checklist!.map((c, i) => (
                                                <li key={i} class={c.done ? 'done' : ''}>
                                                    <label>
                                                        <input
                                                            type="checkbox"
                                                            checked={c.done}
                                                            onChange={() => toggleChecklistItem(i)}
                                                        />
                                                        <span>{c.text}</span>
                                                    </label>
                                                    <button
                                                        type="button"
                                                        class="task-checklist-remove"
                                                        title="删除"
                                                        onClick={() => removeChecklistItem(i)}
                                                    >
                                                        ×
                                                    </button>
                                                </li>
                                            ))}
                                        </ul>
                                    )}
                                    <div class="task-checklist-add-row">
                                        <input
                                            type="text"
                                            placeholder="添加子项…"
                                            value={newChecklistText}
                                            onInput={(e: Event) =>
                                                setNewChecklistText((e.target as HTMLInputElement).value)
                                            }
                                            onKeyDown={(e: KeyboardEvent) => {
                                                if (e.key === 'Enter') {
                                                    e.preventDefault();
                                                    addChecklistItem();
                                                }
                                            }}
                                        />
                                        <button type="button" onClick={addChecklistItem}>
                                            添加
                                        </button>
                                    </div>
                                </div>
                            )}

                            {/* Timeline: standalone comments + session branches */}
                            <div class="topic-replies-head">
                                <span>讨论与执行</span>
                                <span>
                                    {replies.length} 条回复 · {task.sessions.length} 个执行分支
                                </span>
                            </div>
                            <div class="topic-replies">
                                {timelineNodes.map(node => {
                                    if (node.kind === 'comment') {
                                        return renderReplyCard(node.reply);
                                    }
                                    return renderBranchNode(node.session, node.num, node.children);
                                })}
                            </div>

                            {/* GitHub style composer (hidden for discussions — replies happen
                                via the PM conversation opened by 讨论需求, not an inline form) */}
                            {!isNonExecutable && (
                                <div class="gh-composer-card topic-composer-card">
                                    <div class="gh-composer-body">
                                        <textarea
                                            rows={4}
                                            placeholder={
                                                closed
                                                    ? t('task.detail.composerPlaceholderClosed', lang)
                                                    : t('task.detail.composerPlaceholder', lang)
                                            }
                                            value={replyText}
                                            onInput={(e: Event) =>
                                                setReplyText((e.target as HTMLTextAreaElement).value)
                                            }
                                        />
                                    </div>

                                    <div class="gh-composer-footer">
                                        <div class="gh-composer-options">
                                            <select
                                                class="gh-agent-select"
                                                value={composerAgent}
                                                onChange={(e: Event) =>
                                                    setComposerAgent((e.target as HTMLSelectElement).value)
                                                }
                                            >
                                                {AGENT_OPTIONS.map(a => (
                                                    <option key={a} value={a}>
                                                        {a}
                                                    </option>
                                                ))}
                                            </select>
                                            <span class="gh-opt-hint">选择智能体</span>

                                            <label class="timeline-toggle">
                                                <input
                                                    type="checkbox"
                                                    checked={includeTimeline}
                                                    onChange={(e: Event) =>
                                                        setIncludeTimeline((e.target as HTMLInputElement).checked)
                                                    }
                                                />
                                                <span>Include full timeline / previous replies</span>
                                            </label>
                                        </div>
                                        <div class="gh-composer-actions">
                                            <button
                                                type="button"
                                                class="gh-submit-btn"
                                                disabled={submitting || !replyText.trim()}
                                                onClick={submitReply}
                                            >
                                                {submitting
                                                    ? t('task.detail.commentSubmitting', lang)
                                                    : t('task.detail.commentSubmit', lang)}
                                            </button>
                                        </div>
                                    </div>
                                </div>
                            )}
                        </div>
                    )}

                    {activeTab === 'subtasks' && (
                        <div class="gh-subtasks-tab-content">
                            <h4 class="task-tab-title">
                                {t('task.detail.checklistTitle', lang)} ({completedSubtasks}/{totalSubtasks})
                            </h4>
                            {subtasks.length === 0 ? (
                                <div class="task-desc-empty">{t('task.detail.subtasksEmpty', lang)}</div>
                            ) : (
                                <div class="task-checklist">
                                    {subtasks.map(st => (
                                        <div key={st.id} class="task-checklist-item">
                                            <input type="checkbox" checked={st.status === 'completed'} disabled />
                                            <span class={`priority-badge priority-${st.priority || 'medium'}`}>
                                                {priorityLabels[st.priority || 'medium']}
                                            </span>
                                            <span
                                                class={`task-checklist-title${
                                                    st.status === 'completed' ? ' is-done' : ''
                                                }`}
                                            >
                                                {st.title}
                                            </span>
                                            <span class={`task-status-badge ${st.status}`}>
                                                {statusLabels[st.status] || st.status}
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}

                    {activeTab === 'relations' && (
                        <div class="gh-relations-tab-content">
                            <h4 class="task-tab-title">{t('task.detail.relationsTitle', lang)}</h4>

                            <div class="task-rel-group">
                                <h5 class="task-rel-group-title">{t('task.detail.outgoingTitle', lang)}</h5>
                                {outgoing.length === 0 ? (
                                    <div class="task-desc-empty">{t('task.detail.outgoingEmpty', lang)}</div>
                                ) : (
                                    <div class="task-rel-list">
                                        {outgoing.map(link => {
                                            const tgt = taskById.get(link.target);
                                            return (
                                                <div key={`${link.target}-${link.rel}`} class="task-link-row">
                                                    <span class={`task-link-rel rel-${link.rel}`}>
                                                        {linkRelLabels[link.rel] || link.rel}
                                                    </span>
                                                    <button
                                                        class="task-link-target"
                                                        disabled={!tgt || !onNavigate}
                                                        onClick={() => tgt && onNavigate && onNavigate(tgt.id)}
                                                    >
                                                        {linkLabel(tgt)}
                                                    </button>
                                                    <button
                                                        class="task-link-remove"
                                                        title={t('task.detail.removeLink', lang)}
                                                        onClick={() => removeLink(link)}
                                                    >
                                                        ×
                                                    </button>
                                                </div>
                                            );
                                        })}
                                    </div>
                                )}
                            </div>

                            <div class="task-rel-group">
                                <h5 class="task-rel-group-title">{t('task.detail.backlinkTitle', lang)}</h5>
                                {backlinks.length === 0 ? (
                                    <div class="task-desc-empty">{t('task.detail.backlinkEmpty', lang)}</div>
                                ) : (
                                    <div class="task-rel-list">
                                        {backlinks.map(src => {
                                            const link = (src.links || []).find(l => l.target === task.id);
                                            return (
                                                <div key={src.id} class="task-link-row">
                                                    <span class={`task-link-rel rel-${link?.rel || 'relates'}`}>
                                                        {linkRelLabels[link?.rel || 'relates']}
                                                    </span>
                                                    <button
                                                        class="task-link-target"
                                                        disabled={!onNavigate}
                                                        onClick={() => onNavigate && onNavigate(src.id)}
                                                    >
                                                        {linkLabel(src)}
                                                    </button>
                                                </div>
                                            );
                                        })}
                                    </div>
                                )}
                            </div>

                            {/* Add link form — disabled: links are created by agent only; UI shows results. */}
                            {/* <div class="task-addlink-card">
                                <h5 class="task-rel-group-title">{t('task.detail.addLinkTitle', lang)}</h5>
                                <div class="task-addlink-row">
                                    <select
                                        class="task-addlink-select task-link-target-select"
                                        value={linkTarget}
                                        onChange={(e: Event) => setLinkTarget((e.target as HTMLSelectElement).value)}
                                    >
                                        <option value="">{t('task.detail.linkTargetPlaceholder', lang)}</option>
                                        {linkOptions.map(tgt => (
                                            <option key={tgt.id} value={tgt.id}>
                                                {linkLabel(tgt)}
                                            </option>
                                        ))}
                                    </select>
                                    <select
                                        class="task-addlink-select task-link-rel-select"
                                        value={linkRel}
                                        onChange={(e: Event) =>
                                            setLinkRel((e.target as HTMLSelectElement).value as LinkRel)
                                        }
                                    >
                                        <option value="relates">{t('task.link.relates', lang)}</option>
                                    </select>
                                    <button class="gh-submit-btn" disabled={!linkTarget} onClick={addLink}>
                                        {t('task.detail.addLink', lang)}
                                    </button>
                                </div>
                            </div> */}
                        </div>
                    )}
                </div>

                {/* Backdrop: only visible while the mobile bottom-sheet is open */}
                <div
                    class={`task-detail-backdrop${drawerOpen.value ? ' is-open' : ''}`}
                    onClick={() => (drawerOpen.value = false)}
                />

                <div
                    class={`task-detail-sidebar${sidebarCollapsed.value ? ' is-collapsed' : ''}${
                        drawerOpen.value ? ' drawer-open' : ''
                    }`}
                >
                    {/* Drag handle + header — only rendered visually on the mobile sheet */}
                    <div class="task-drawer-handle">
                        <span class="task-drawer-grip" />
                        <span class="task-drawer-title">话题信息</span>
                        <button
                            class="task-drawer-close"
                            title={t('task.detail.drawerClose', lang)}
                            onClick={() => (drawerOpen.value = false)}
                        >
                            ×
                        </button>
                    </div>

                    {/* Status: issue meta + open/close toggle (moved out of the header) */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>话题状态</span>
                            <span class={`gh-status-icon ${closed ? 'closed' : 'open'}`}>
                                <StatusIcon closed={closed} />
                            </span>
                        </div>
                        <div class="gh-sidebar-body">
                            <div class="gh-meta-text">
                                <strong>{task.createdBy || 'scottzx'}</strong>{' '}
                                {closed ? t('task.detail.closedBy', lang) : t('task.detail.createdBy', lang)} ·{' '}
                                {replies.length} 条回复
                            </div>
                            <button class="task-issue-toggle-btn" onClick={toggleIssueState}>
                                {closed ? t('task.detail.reopen', lang) : t('task.detail.close', lang)}
                            </button>
                        </div>
                    </div>

                    {/* Execution checks live in the sidebar so the main thread stays conversation-first. */}
                    {!isNonExecutable && renderExecutionBox()}

                    {/* Assignees (hidden for discussions — nobody executes a discussion) */}
                    {!isNonExecutable && (
                        <div class="gh-sidebar-panel">
                            <div class="gh-sidebar-head">
                                <span>{t('task.detail.sideAssignees', lang)}</span>
                                <span class="gh-sidebar-edit-icon">⚙</span>
                            </div>
                            <div class="gh-sidebar-body">
                                <div class="gh-assignee-row">
                                    <span class="gh-avatar">{getInitials(task.assignee || 'claudecode')}</span>
                                    <select
                                        class="gh-assignee-select"
                                        value={task.assignee || 'claudecode'}
                                        onChange={(e: Event) =>
                                            patchTask({
                                                assignee: (e.target as HTMLSelectElement).value,
                                            }).catch(err => alert((err as Error).message))
                                        }
                                    >
                                        {AGENT_OPTIONS.map(a => (
                                            <option key={a} value={a}>
                                                {a}
                                            </option>
                                        ))}
                                    </select>
                                    <button
                                        type="button"
                                        class="gh-close-btn"
                                        onClick={toggleIssueState}
                                        title={
                                            closed
                                                ? t('task.detail.reopenIssue', lang)
                                                : t('task.detail.closeIssue', lang)
                                        }
                                    >
                                        {closed
                                            ? t('task.detail.reopenIssue', lang)
                                            : t('task.detail.closeIssue', lang)}
                                    </button>
                                </div>
                            </div>
                        </div>
                    )}

                    {/* Labels */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>{t('task.detail.sideLabels', lang)}</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {(task.labels || []).length === 0 ? (
                                <span class="gh-no-item">{t('task.detail.noLabels', lang)}</span>
                            ) : (
                                <div class="gh-label-list">
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
                            <span>{t('task.detail.sideMilestone', lang)}</span>
                            <span class="gh-sidebar-edit-icon">⚙</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {task.milestone ? (
                                <div>🏁 {task.milestone}</div>
                            ) : (
                                <span class="gh-no-item">{t('task.detail.noMilestone', lang)}</span>
                            )}
                        </div>
                    </div>

                    {/* Priority */}
                    <div class="gh-sidebar-panel">
                        <div class="gh-sidebar-head">
                            <span>{t('task.detail.sidePriority', lang)}</span>
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
                            <span>{t('task.detail.sideDates', lang)}</span>
                        </div>
                        <div class="gh-sidebar-body gh-dates-body">
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
                            <span>{t('task.detail.sideDevelopment', lang)}</span>
                        </div>
                        <div class="gh-sidebar-body">
                            {task.sessions.length === 0 ? (
                                <span class="gh-no-item">{t('task.detail.noSessions', lang)}</span>
                            ) : (
                                <div class="gh-session-list">
                                    {task.sessions.map((s, idx) => (
                                        <button
                                            key={s.id}
                                            class="gh-session-btn"
                                            onClick={() => openSession(s.id, s.agentType)}
                                        >
                                            <span>
                                                #{idx + 1} {s.agentType}
                                            </span>
                                            <span
                                                class={`task-status-badge ${s.status === 'running' ? 'running' : 'completed'}`}
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
                    <div class="gh-sidebar-panel gh-sidebar-panel-last">
                        <div class="gh-sidebar-head">
                            <span>{t('task.detail.sideDanger', lang)}</span>
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
