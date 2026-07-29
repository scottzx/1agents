import { h } from 'preact';
import { useEffect, useState } from 'preact/hooks';

import { activityService, type TaskRun } from '@1agents/core/services/activityService';
import { agentService } from '@1agents/core/services/agentService';
import * as sessionStore from '../../stores/sessionStore';
import { requestTurnFocus } from '../../stores/turnFocusStore';
import { showToast } from '../../stores/uiStore';

export function TaskRunAuditTrail({ taskId }: { taskId: string }) {
    const [runs, setRuns] = useState<TaskRun[]>([]);
    const [error, setError] = useState('');

    useEffect(() => {
        let cancelled = false;
        setError('');
        void activityService
            .listTaskRuns(taskId)
            .then(items => {
                if (!cancelled) setRuns(items);
            })
            .catch(err => {
                if (!cancelled) setError((err as Error).message);
            });
        return () => {
            cancelled = true;
        };
    }, [taskId]);

    const openSession = async (sessionId: string, turnId?: string) => {
        try {
            const session = await agentService.get(sessionId);
            if (!session) throw new Error('Session 不存在或已清理');
            if (turnId) requestTurnFocus(sessionId, turnId);
            await sessionStore.selectSession(session);
        } catch (err) {
            showToast(`打开 TaskRun Session 失败：${(err as Error).message}`);
        }
    };

    if (runs.length === 0 && !error) return null;

    return (
        <section aria-label="TaskRun 完成审计" style={{ display: 'grid', gap: '8px', padding: '4px 0 16px' }}>
            <div style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-secondary)' }}>TaskRun / 完成审计</div>
            {error && (
                <div role="alert" style={{ color: 'var(--danger-color, #c0392b)', fontSize: '12px' }}>
                    {error}
                </div>
            )}
            {runs.map(run => (
                <details
                    key={run.id}
                    style={{
                        border: '1px solid var(--border-color)',
                        borderRadius: '8px',
                        padding: '8px 10px',
                        background: 'var(--bg-secondary)',
                    }}
                >
                    <summary style={{ cursor: 'pointer', fontSize: '12px' }}>
                        {run.kind === 'verification' ? '核验' : '执行'} #{run.attempt} · {run.status}
                        {run.closedBy ? ' · 已通过完成门' : ''}
                    </summary>
                    <div style={{ display: 'grid', gap: '5px', marginTop: '8px', fontSize: '12px' }}>
                        <span>TaskRun: {run.id}</span>
                        {run.originTurnId && <span>Origin Turn: {run.originTurnId}</span>}
                        {run.evidence.map(evidence => (
                            <span key={evidence.id}>
                                Evidence · {evidence.kind}: {evidence.summary}
                            </span>
                        ))}
                        {run.verdict && (
                            <span>
                                Verdict:{' '}
                                {run.verdict.pass ? 'passed' : run.verdict.needsHuman ? 'needs-human' : 'failed'}
                                {run.verdict.summary ? ` · ${run.verdict.summary}` : ''}
                            </span>
                        )}
                        {run.closedBy && (
                            <span>
                                ClosedBy: {run.closedBy.kind} · {run.closedBy.verdict}
                            </span>
                        )}
                        {run.originSessionId && run.originTurnId && (
                            <button
                                type="button"
                                onClick={() => void openSession(run.originSessionId!, run.originTurnId)}
                                style={{
                                    justifySelf: 'start',
                                    border: 0,
                                    padding: 0,
                                    background: 'transparent',
                                    color: 'var(--accent-color)',
                                    cursor: 'pointer',
                                    fontSize: '12px',
                                }}
                            >
                                打开 Origin Session / Turn
                            </button>
                        )}
                        {run.sessionId && (
                            <button
                                type="button"
                                onClick={() => void openSession(run.sessionId!)}
                                style={{
                                    justifySelf: 'start',
                                    border: 0,
                                    padding: 0,
                                    background: 'transparent',
                                    color: 'var(--accent-color)',
                                    cursor: 'pointer',
                                    fontSize: '12px',
                                }}
                            >
                                打开执行 Session
                            </button>
                        )}
                    </div>
                </details>
            ))}
        </section>
    );
}
