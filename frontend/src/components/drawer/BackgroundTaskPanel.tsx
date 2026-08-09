// 副看板 (right panel) live view of the active chat session's background
// bash tasks. Subscribes to the same ChatBridgeManager state that drives the
// top-of-chat BackgroundTasks widget, but renders a fuller status view:
// running/completed/failed summary, a per-task elapsed timer that ticks every
// second while the task is running, and a collapsible output tail for long
// tasks. The tab is a normal 常驻 side-panel tab — it shows an empty state
// when no session/tasks are active.

import { h, Fragment } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { t } from '../../i18n';
import * as sess from '../../stores/sessionStore';
import * as ui from '../../stores/uiStore';
import { isChat } from '../types';
import { globalBridgeManager } from '../chat/hooks';
import type { BackgroundTask } from '../chat/hooks';
import { StatusIcon } from '../chat/BackgroundTasks';

const TICK_MS = 1000;
// Cap the rendered output tail so long-running scrollback stays cheap.
const OUTPUT_TAIL_LINES = 200;

function formatElapsed(ms: number): string {
    const total = Math.max(0, Math.floor(ms / 1000));
    const s = total % 60;
    const m = Math.floor(total / 60) % 60;
    const hh = Math.floor(total / 3600);
    if (hh > 0) return `${hh}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
}

/** Runtimes may send startedAt in epoch seconds or milliseconds — normalize. */
function toEpochMs(value: number): number {
    return value < 1e12 ? value * 1000 : value;
}

function TaskCard({
    task,
    now,
    startAt,
    language,
}: {
    task: BackgroundTask;
    now: number;
    startAt?: number;
    language: typeof ui.language.value;
}) {
    const outputOpen = useSignal(false);
    const preRef = useRef<HTMLPreElement | null>(null);

    const running = task.status === 'running';
    const startMs = startAt !== undefined ? toEpochMs(startAt) : undefined;
    const elapsed = running
        ? startMs !== undefined
            ? formatElapsed(now - startMs)
            : task.duration ?? ''
        : task.duration ?? (startMs !== undefined ? formatElapsed(now - startMs) : '');

    const output = task.output ?? '';
    const lines = output.split('\n');
    const truncated = lines.length > OUTPUT_TAIL_LINES;
    const tail = truncated ? lines.slice(-OUTPUT_TAIL_LINES) : lines;

    // Keep the tail pinned to the newest lines while output streams in.
    useEffect(() => {
        if (outputOpen.value && preRef.current) {
            preRef.current.scrollTop = preRef.current.scrollHeight;
        }
    }, [outputOpen.value, output]);

    return (
        <div class="background-task-card" data-status={task.status}>
            <div class="background-task-card-head">
                <span class="chat-background-task-item-icon">
                    <StatusIcon status={task.status} />
                </span>
                <div class="background-task-card-command" title={task.command}>
                    {task.command}
                </div>
                <span class="background-task-card-elapsed" data-running={running ? 'true' : undefined}>
                    {elapsed}
                </span>
            </div>
            <div class="background-task-card-meta">
                <span class="background-task-card-id">#{task.id}</span>
                {task.exitCode !== undefined && (
                    <span class="background-task-card-exit" data-error={task.exitCode !== 0 ? 'true' : undefined}>
                        {t('backgroundPanel.exitCode', language, { code: task.exitCode })}
                    </span>
                )}
            </div>
            {output && (
                <Fragment>
                    <button
                        type="button"
                        class="background-task-card-toggle"
                        onClick={() => {
                            outputOpen.value = !outputOpen.value;
                        }}
                    >
                        {outputOpen.value
                            ? t('backgroundPanel.hideOutput', language)
                            : t('backgroundPanel.showOutput', language)}
                    </button>
                    {outputOpen.value && (
                        <Fragment>
                            {truncated && (
                                <div class="background-task-card-truncated">
                                    {t('backgroundPanel.truncated', language, { n: lines.length - OUTPUT_TAIL_LINES })}
                                </div>
                            )}
                            <pre class="background-task-card-output" ref={preRef}>
                                {tail.join('\n')}
                            </pre>
                        </Fragment>
                    )}
                </Fragment>
            )}
        </div>
    );
}

export function BackgroundTaskPanel({ language }: { language: typeof ui.language.value }) {
    // Re-render via a signal (same pattern as useBridge): the panel lives in a
    // static-ish subtree, so a plain useState forceUpdate may silently fail.
    const rev = useSignal(0);
    const session = sess.activeSession.value;
    const chat = session && isChat(session) ? session : null;

    useEffect(() => {
        if (!chat) return;
        const state = globalBridgeManager.getOrCreate(chat);
        const bump = () => {
            rev.value++;
        };
        state.listeners.add(bump);
        bump();
        return () => {
            state.listeners.delete(bump);
        };
    }, [chat?.id, chat?.workspaceId]);

    // eslint-disable-next-line no-unused-expressions
    rev.value;

    const state = chat ? globalBridgeManager.getOrCreate(chat) : null;
    const tasks = state?.backgroundTasks ?? null;

    // Live ticking clock while any task is running: bump the same `rev`
    // signal the render already subscribes to, so `now` recomputes each second.
    useEffect(() => {
        if (!tasks?.some(task => task.status === 'running')) return;
        const id = window.setInterval(() => {
            rev.value++;
        }, TICK_MS);
        return () => window.clearInterval(id);
    }, [tasks]);

    // Fallback start times for runtimes that don't send startedAt.
    const firstSeen = useSignal<Record<string, number>>({});

    const running = tasks?.filter(task => task.status === 'running') ?? [];
    const completed = tasks?.filter(task => task.status === 'completed') ?? [];
    const failed =
        tasks?.filter(task => task.status === 'failed' || task.status === 'cancelled' || task.status === 'killed') ??
        [];
    const total = tasks?.length ?? 0;

    const now = Date.now();
    for (const task of running) {
        if (typeof task.startedAt === 'number') continue;
        if (!firstSeen.value[task.id]) firstSeen.value[task.id] = now;
    }

    if (!tasks || total === 0) {
        return (
            <div class="background-task-panel">
                <div class="background-task-empty">
                    <p class="background-task-empty-title">{t('backgroundPanel.empty', language)}</p>
                </div>
            </div>
        );
    }

    return (
        <div class="background-task-panel">
            <div class="background-task-summary">
                {running.length > 0 && (
                    <span class="chat-task-running-badge">
                        {t('backgroundPanel.runningCount', language, { n: running.length })}
                    </span>
                )}
                {completed.length > 0 && (
                    <span class="background-task-done-badge">
                        {t('backgroundPanel.completedCount', language, { n: completed.length })}
                    </span>
                )}
                {failed.length > 0 && (
                    <span class="chat-task-failed-badge">
                        {t('backgroundPanel.failedCount', language, { n: failed.length })}
                    </span>
                )}
                <span class="background-task-summary-total">
                    {t('backgroundPanel.totalCount', language, { n: total })}
                </span>
            </div>
            <div class="background-task-panel-list">
                {tasks.map(task => (
                    <TaskCard
                        key={task.id}
                        task={task}
                        now={now}
                        startAt={task.startedAt ?? firstSeen.value[task.id]}
                        language={language}
                    />
                ))}
            </div>
        </div>
    );
}
