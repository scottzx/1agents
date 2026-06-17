import { h } from 'preact';
import type { VNode } from 'preact';

import { AGENT_TYPE_LABELS, type ChatSession } from '../../types';
import type { CellHelpers, GridColumn } from './DataGrid';
import { fmtDate } from './utils';

// Human-readable labels for the conversation role declared at creation.
export const ROLE_LABELS: Record<string, string> = {
    pmo: '总管',
    pm: '项目经理',
    executor: '执行者',
    verifier: '验收者',
    general: '对话',
};

const agentLabel = (s: ChatSession) => AGENT_TYPE_LABELS[s.agentType] ?? s.agentType;
const roleOf = (s: ChatSession) => s.role || 'general';

export const SESSION_COLUMNS: GridColumn[] = [
    { key: 'name', label: '会话标题', width: 260, locked: true },
    { key: 'agentType', label: 'Agent', width: 120, groupable: true },
    { key: 'role', label: '角色', width: 104, groupable: true },
    { key: 'archived', label: '状态', width: 96, groupable: true },
    { key: 'createdAt', label: '创建', width: 124 },
    { key: 'lastEventAt', label: '最近活动', width: 124 },
    { key: 'taskId', label: '关联任务', width: 130, sortable: false },
];

export const SESSION_GROUP_OPTIONS: Array<[string, string]> = [
    ['none', '不分组'],
    ['archived', '状态'],
    ['role', '角色'],
    ['agentType', 'Agent'],
];

const ts = (iso?: string): number => (iso ? new Date(iso).getTime() : 0);

export function compareSessions(a: ChatSession, b: ChatSession, key: string): number {
    switch (key) {
        case 'name':
            return a.name.localeCompare(b.name);
        case 'agentType':
            return agentLabel(a).localeCompare(agentLabel(b));
        case 'role':
            return (ROLE_LABELS[roleOf(a)] ?? roleOf(a)).localeCompare(ROLE_LABELS[roleOf(b)] ?? roleOf(b));
        case 'archived':
            return Number(!!a.archived) - Number(!!b.archived);
        case 'createdAt':
            return ts(a.createdAt) - ts(b.createdAt);
        case 'lastEventAt':
            return ts(a.lastEventAt) - ts(b.lastEventAt);
        default:
            return 0;
    }
}

// Default order: active sessions first, then most-recent activity (fall back
// to creation time) — mirrors the prior card list.
export function sessionDefaultCompare(a: ChatSession, b: ChatSession): number {
    const t = (s: ChatSession) => ts(s.lastEventAt) || ts(s.createdAt);
    return Number(!!a.archived) - Number(!!b.archived) || t(b) - t(a);
}

export function sessionGroupValue(s: ChatSession, key: string): string {
    switch (key) {
        case 'archived':
            return s.archived ? '已归档' : '活跃';
        case 'role':
            return ROLE_LABELS[roleOf(s)] ?? roleOf(s);
        case 'agentType':
            return agentLabel(s);
        default:
            return '';
    }
}

interface SessionCellCtx {
    /** Jump to a linked task's timeline. */
    onSelectTask?: (taskId: string) => void;
}

// Read-only cell renderer for the session DataGrid. Each branch returns a full
// <td>, mirroring TaskGridCell's contract.
export function renderSessionCell(s: ChatSession, col: GridColumn, helpers: CellHelpers, ctx: SessionCellCtx): VNode {
    switch (col.key) {
        case 'name':
            return (
                <td class="col-session-name">
                    <button
                        class="session-name-link"
                        title="打开会话"
                        onClick={(e: Event) => {
                            e.stopPropagation();
                            helpers.openDetail();
                        }}
                    >
                        {s.name || '(未命名会话)'}
                    </button>
                </td>
            );
        case 'agentType':
            return (
                <td class="col-session-agent">
                    <span class="session-agent-badge">{agentLabel(s)}</span>
                </td>
            );
        case 'role': {
            const role = roleOf(s);
            return (
                <td class="col-session-role">
                    {role === 'general' ? (
                        '—'
                    ) : (
                        <span class={`session-role-badge role-${role}`}>{ROLE_LABELS[role] ?? role}</span>
                    )}
                </td>
            );
        }
        case 'archived':
            return (
                <td class="col-session-state">
                    <span class={`session-state-badge ${s.archived ? 'archived' : 'active'}`}>
                        {s.archived ? '已归档' : '活跃'}
                    </span>
                </td>
            );
        case 'createdAt':
            return <td class="col-session-date">{fmtDate(s.createdAt)}</td>;
        case 'lastEventAt':
            return <td class="col-session-date">{fmtDate(s.lastEventAt)}</td>;
        case 'taskId':
            return (
                <td class="col-session-task">
                    {s.taskId ? (
                        ctx.onSelectTask ? (
                            <button
                                class="session-task-badge clickable"
                                title="查看关联任务时间轴"
                                onClick={(e: Event) => {
                                    e.stopPropagation();
                                    ctx.onSelectTask!(s.taskId!);
                                }}
                            >
                                关联任务 ↗
                            </button>
                        ) : (
                            <span class="session-task-badge">关联任务</span>
                        )
                    ) : (
                        '—'
                    )}
                </td>
            );
        default:
            return <td />;
    }
}
