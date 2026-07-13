import { h } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';
import { useSignal } from '@preact/signals';

import type { ChatSession, Session } from '../../types';
import { t } from '../../../i18n';
import * as ui from '../../../stores/uiStore';
import { agentService } from '../../../services/agentService';
import { DataGrid } from './DataGrid';
import {
    getSessionColumns,
    getSessionGroupOptions,
    compareSessions,
    renderSessionCell,
    sessionDefaultCompare,
    sessionGroupValue,
} from './sessionGrid';
import {
    chatSessions,
    mergeSessionsIntoFolders,
    terminalWindows,
    loadChatSessions
} from '../../../stores/sessionStore';

interface SessionsViewProps {
    workspaceId: string;
    onSelectSession?: (session: Session) => void;
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

    // 恢复对话: open the conversation, un-archiving first when needed so the
    // sidebar treats the next close as a fresh archive, not a double-close.
    const handleOpen = useCallback(
        async (s: ChatSession) => {
            if (!onSelectSession) return;
            if (s.archived) {
                // Optimistic UI update: unarchive locally immediately
                s.archived = false;
                s.archivedAt = undefined;
                chatSessions.value = [...chatSessions.value];
                mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);

                try {
                    await agentService.setArchived(s.id, false);
                } catch (err) {
                    // Rollback on error
                    s.archived = true;
                    s.archivedAt = new Date().toISOString();
                    chatSessions.value = [...chatSessions.value];
                    mergeSessionsIntoFolders(terminalWindows.value, chatSessions.value);
                    setError((err as Error).message);
                    return;
                }
            }
            onSelectSession({ ...s, archived: false, archivedAt: undefined, active: true });
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
