import { h } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import type { ChatSession, Session } from '../../types';
import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';
import { DataGrid } from './DataGrid';
import {
    getSessionColumns,
    getSessionGroupOptions,
    compareSessions,
    renderSessionCell,
    sessionDefaultCompare,
    sessionGroupValue,
} from './sessionGrid';
import { chatSessions, loadChatSessions } from '../../../stores/sessionStore';

interface SessionsViewProps {
    workspaceId: string;
    /** May be async — selectSession unarchives when needed. */
    onSelectSession?: (session: Session) => void | Promise<void>;
    /** Jump to a linked task's timeline (sets the TaskList selection). */
    onSelectTask?: (taskId: string) => void;
}

const RestoreIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
        <path d="M3 12a9 9 0 1 0 3-6.7L3 8" />
        <polyline points="3 3 3 8 8 8" />
    </svg>
);

const OpenIcon = () => (
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <polyline points="9 18 15 12 9 6" />
    </svg>
);

// 会话归档：以多维表格（复用任务模块的 DataGrid）列出本项目下所有会话（含已
// 归档）。关闭侧边栏会话 = 归档（软删除），数据保留，在此可检索、排序、分组。
// 「恢复对话」：未归档会话直接打开；已归档会话先取消归档再打开，于是下次在侧
// 边栏关闭即是关闭一个活跃会话，而非二次关闭。
export function SessionsView({ workspaceId, onSelectSession, onSelectTask }: SessionsViewProps) {
    const sessions = chatSessions.value.filter(s => s.workspaceId === workspaceId);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const query = useSignal('');
    const language = ui.language.value;

    const fetchSessions = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            await loadChatSessions(workspaceId);
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, [workspaceId]);

    useEffect(() => {
        fetchSessions();
    }, [fetchSessions]);

    // Open / restore: selectSession itself unarchives when needed (single path
    // for sidebar SoT + backend PATCH). Avoid a second restore here.
    const handleOpen = useCallback(
        async (s: ChatSession) => {
            if (!onSelectSession) return;
            try {
                await onSelectSession({ ...s, active: true });
            } catch (err) {
                setError((err as Error).message);
            }
        },
        [onSelectSession]
    );

    const q = query.value.trim().toLowerCase();
    const filtered = q ? sessions.filter(s => s.name.toLowerCase().includes(q)) : sessions;

    return (
        <div class="sessions-view">
            <div class="sessions-view-toolbar">
                <input
                    class="sessions-search"
                    type="search"
                    placeholder={t('sessions.searchPlaceholder', language)}
                    value={query.value}
                    onInput={e => (query.value = (e.target as HTMLInputElement).value)}
                />
                <span class="sessions-count">{`${filtered.length} / ${sessions.length}`}</span>
            </div>

            {error && <div class="task-error">{error}</div>}

            <DataGrid<ChatSession>
                workspaceId={workspaceId}
                prefsSurface="sessions"
                rows={filtered}
                totalCount={sessions.length}
                columns={getSessionColumns(language)}
                groupOptions={getSessionGroupOptions(language)}
                getRowKey={s => s.id}
                loading={loading}
                emptyAll={t('sessions.emptyAll', language)}
                emptyFiltered={t('sessions.emptyFiltered', language)}
                compare={compareSessions}
                defaultCompare={sessionDefaultCompare}
                groupValue={(s, key) => sessionGroupValue(s, key, language)}
                rowClass={s => `task-row session-grid-row${s.archived ? ' archived' : ''}`}
                onOpenRow={handleOpen}
                renderCell={(s, col, helpers) => renderSessionCell(s, col, helpers, { onSelectTask }, language)}
                renderActions={s => (
                    <button
                        class="session-restore-btn"
                        title={s.archived ? t('sessions.restoreTitle', language) : t('session.openTitle', language)}
                        onClick={() => handleOpen(s)}
                    >
                        {s.archived ? <RestoreIcon /> : <OpenIcon />}
                    </button>
                )}
            />
        </div>
    );
}
