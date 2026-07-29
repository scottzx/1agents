import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';

import {
    activityService,
    type ProjectActivityEntry,
    type ProjectActivityStatus,
} from '@1agents/core/services/activityService';
import { agentService } from '@1agents/core/services/agentService';
import * as sessionStore from '../../stores/sessionStore';
import { requestTurnFocus } from '../../stores/turnFocusStore';
import { showToast } from '../../stores/uiStore';

interface ProjectActivityTimelineProps {
    workspaceId: string;
    targetId?: string;
    compact?: boolean;
}

const STATUS_LABEL: Record<ProjectActivityStatus, string> = {
    succeeded: '成功',
    rejected: '已拒绝',
    failed: '失败',
};

export function ProjectActivityTimeline({ workspaceId, targetId, compact = false }: ProjectActivityTimelineProps) {
    const [entries, setEntries] = useState<ProjectActivityEntry[]>([]);
    const [cursor, setCursor] = useState<string | undefined>();
    const [hasMore, setHasMore] = useState(false);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');

    const load = async (nextCursor?: string) => {
        setLoading(true);
        setError('');
        try {
            const page = await activityService.listActivity(workspaceId, {
                targetType: targetId ? 'project_item' : undefined,
                targetId,
                cursor: nextCursor,
                limit: compact ? 20 : 50,
            });
            setEntries(current => (nextCursor ? [...current, ...page.items] : page.items));
            setCursor(page.nextCursor);
            setHasMore(page.hasMore);
        } catch (err) {
            setError((err as Error).message || '项目动态加载失败');
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        setEntries([]);
        setCursor(undefined);
        setHasMore(false);
        void load();
    }, [workspaceId, targetId]);

    const openTurn = async (entry: ProjectActivityEntry) => {
        if (!entry.sessionId) return;
        try {
            const session = await agentService.get(entry.sessionId);
            if (!session) throw new Error('Session 不存在或已清理');
            if (entry.turnId) requestTurnFocus(entry.sessionId, entry.turnId);
            // Keep the indexed Session untouched. In particular, a project-wide
            // PM Session intentionally has no taskId even when reached from one
            // Task Detail's reverse projection.
            await sessionStore.selectSession(session);
        } catch (err) {
            showToast(`打开 Turn 失败：${(err as Error).message}`);
        }
    };

    return (
        <section
            aria-label={targetId ? '相关 Agent Turn' : '项目动态时间轴'}
            style={{
                display: 'grid',
                gap: '12px',
                padding: compact ? '12px 0' : '24px',
                maxWidth: compact ? '100%' : '960px',
                margin: compact ? '0' : '0 auto',
            }}
        >
            {compact && (
                <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-secondary)' }}>相关 Agent Turn</div>
            )}
            {error && (
                <div role="alert" style={{ color: 'var(--danger-color, #c0392b)', fontSize: '13px' }}>
                    {error}
                </div>
            )}
            {!loading && entries.length === 0 && !error && (
                <div style={{ color: 'var(--text-tertiary)', fontSize: '13px', padding: '16px 0' }}>
                    {targetId ? '暂无关联 Turn' : '暂无项目动态'}
                </div>
            )}
            {entries.map(entry => (
                <article
                    key={entry.id}
                    style={{
                        border: '1px solid var(--border-color)',
                        borderRadius: '10px',
                        padding: compact ? '10px 12px' : '14px 16px',
                        background: 'var(--bg-secondary)',
                    }}
                >
                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px', flexWrap: 'wrap' }}>
                        <strong style={{ fontSize: '14px' }}>{entry.summary}</strong>
                        <span
                            style={{
                                fontSize: '11px',
                                color:
                                    entry.status === 'succeeded'
                                        ? 'var(--text-tertiary)'
                                        : 'var(--danger-color, #c0392b)',
                            }}
                        >
                            {STATUS_LABEL[entry.status]}
                        </span>
                        <time style={{ marginLeft: 'auto', fontSize: '12px', color: 'var(--text-tertiary)' }}>
                            {new Date(entry.createdAt).toLocaleString()}
                        </time>
                    </div>
                    <div style={{ marginTop: '6px', fontSize: '12px', color: 'var(--text-tertiary)' }}>
                        {entry.actorName || entry.actorKind} · {entry.origin}
                        {entry.turnId ? ` · Turn ${entry.turnId.slice(0, 8)}` : ''}
                    </div>
                    {entry.sessionId && (
                        <button
                            type="button"
                            onClick={() => void openTurn(entry)}
                            style={{
                                marginTop: '8px',
                                border: 0,
                                padding: 0,
                                background: 'transparent',
                                color: 'var(--accent-color)',
                                cursor: 'pointer',
                                fontSize: '12px',
                            }}
                        >
                            打开 Session{entry.turnId ? ' / Turn' : ''}
                        </button>
                    )}
                </article>
            ))}
            {hasMore && (
                <button type="button" disabled={loading} onClick={() => void load(cursor)}>
                    {loading ? '加载中…' : '加载更多'}
                </button>
            )}
            {loading && entries.length === 0 && (
                <div style={{ color: 'var(--text-tertiary)', fontSize: '13px' }}>正在加载项目动态…</div>
            )}
        </section>
    );
}
