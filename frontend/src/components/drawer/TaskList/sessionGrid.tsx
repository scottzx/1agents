import { h } from 'preact';
import type { VNode } from 'preact';

import { t, type Lang } from '../../../i18n';
import { AGENT_TYPE_LABELS, type ChatSession } from '../../types';
import type { CellHelpers, GridColumn } from './DataGrid';
import { fmtDate } from './utils';

type RoleKey = 'pmo' | 'pm' | 'executor' | 'verifier' | 'general';
const ROLE_KEYS: readonly RoleKey[] = ['pmo', 'pm', 'executor', 'verifier', 'general'];

export function getRoleLabel(role: string, lang: Lang): string {
    if (!ROLE_KEYS.includes(role as RoleKey)) return role;
    return t(`session.role.${role as RoleKey}`, lang);
}

const agentLabel = (s: ChatSession) => AGENT_TYPE_LABELS[s.agentType] ?? s.agentType;
const roleOf = (s: ChatSession) => s.role || 'general';

export function getSessionColumns(lang: Lang): GridColumn[] {
    return [
        { key: 'name', label: t('session.col.name', lang), width: 260, locked: true },
        { key: 'agentType', label: 'Agent', width: 120, groupable: true },
        { key: 'role', label: t('session.col.role', lang), width: 104, groupable: true },
        { key: 'archived', label: t('session.col.status', lang), width: 96, groupable: true },
        { key: 'createdAt', label: t('session.col.created', lang), width: 124 },
        { key: 'lastEventAt', label: t('session.col.lastActivity', lang), width: 124 },
        { key: 'taskId', label: t('session.col.task', lang), width: 130, sortable: false },
    ];
}

export function getSessionGroupOptions(lang: Lang): Array<[string, string]> {
    return [
        ['none', t('session.group.none', lang)],
        ['archived', t('session.col.status', lang)],
        ['role', t('session.col.role', lang)],
        ['agentType', 'Agent'],
    ];
}

const ts = (iso?: string): number => (iso ? new Date(iso).getTime() : 0);

export function compareSessions(a: ChatSession, b: ChatSession, key: string): number {
    switch (key) {
        case 'name':
            return a.name.localeCompare(b.name);
        case 'agentType':
            return agentLabel(a).localeCompare(agentLabel(b));
        case 'role':
            return roleOf(a).localeCompare(roleOf(b));
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

export function sessionGroupValue(s: ChatSession, key: string, lang: Lang): string {
    switch (key) {
        case 'archived':
            return s.archived ? t('session.status.archived', lang) : t('session.status.active', lang);
        case 'role':
            return getRoleLabel(roleOf(s), lang);
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
export function renderSessionCell(
    s: ChatSession,
    col: GridColumn,
    helpers: CellHelpers,
    ctx: SessionCellCtx,
    lang: Lang
): VNode {
    switch (col.key) {
        case 'name':
            return (
                <td class="col-session-name">
                    <button
                        class="session-name-link"
                        title={t('session.openTitle', lang)}
                        onClick={(e: Event) => {
                            e.stopPropagation();
                            helpers.openDetail();
                        }}
                    >
                        {s.name || t('session.unnamed', lang)}
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
                        <span class={`session-role-badge role-${role}`}>{getRoleLabel(role, lang)}</span>
                    )}
                </td>
            );
        }
        case 'archived':
            return (
                <td class="col-session-state">
                    <span class={`session-state-badge ${s.archived ? 'archived' : 'active'}`}>
                        {s.archived ? t('session.status.archived', lang) : t('session.status.active', lang)}
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
                                title={t('session.taskTitle', lang)}
                                onClick={(e: Event) => {
                                    e.stopPropagation();
                                    ctx.onSelectTask!(s.taskId!);
                                }}
                            >
                                {t('session.taskLink', lang)}
                            </button>
                        ) : (
                            <span class="session-task-badge">{t('session.taskBadge', lang)}</span>
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
