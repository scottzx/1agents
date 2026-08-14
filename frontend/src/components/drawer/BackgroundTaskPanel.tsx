// Session status panel (right side panel).
// Monitors the active chat session's execution state, resources, and
// deliverables — subagents, file changes, artifacts, uploads, and
// background bash tasks — scoped to the selected Agent Turn.

import { h, Fragment, type ComponentChildren } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import type { AgentTurn } from '@1agents/core/services/activityService';
import { activityService } from '@1agents/core/services/activityService';
import { fsService } from '../../services/fsService';
import { t } from '../../i18n';
import * as sess from '../../stores/sessionStore';
import * as ui from '../../stores/uiStore';
import * as tabsStore from '../../stores/tabsStore';
import { openFsNewItemModal } from '../../stores/modalStore';
import { requestTurnFocus } from '../../stores/turnFocusStore';
import { isChat } from '../types';
import { globalBridgeManager } from '../chat/hooks';
import type { BackgroundTask, ChatItem } from '../chat/hooks';
import { StatusIcon } from '../chat/BackgroundTasks';
import {
    collectSessionTurns,
    collectSubagents,
    collectTurnFiles,
    collectUploads,
    displayFilePath,
    itemsForTurn,
    promptSnippet,
    splitTurnFiles,
    type SessionFileEntry,
    type SessionSubagent,
    type SessionTurnRef,
    type SessionUpload,
} from './sessionStatusModel';

const TICK_MS = 1000;
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

function formatClock(ms?: number): string {
    if (!ms) return '—';
    return new Date(ms).toLocaleString();
}

function openPath(path: string) {
    const name = path.split('/').filter(Boolean).pop() || path;
    void tabsStore.openPreviewTab(path, name);
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

function Caret({ open }: { open: boolean }) {
    return (
        <svg
            class="session-status-caret"
            data-open={open ? 'true' : 'false'}
            viewBox="0 0 24 24"
            width="12"
            height="12"
            fill="none"
            stroke="currentColor"
            stroke-width="2.5"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
        >
            <path d="m9 18 6-6-6-6" />
        </svg>
    );
}

function StatusSection({
    title,
    count,
    children,
    empty,
}: {
    title: string;
    count: number;
    children: ComponentChildren;
    empty: string;
}) {
    const toggled = useSignal<boolean | null>(null);
    const open = toggled.value === null ? count > 0 : toggled.value;
    return (
        <section class="session-status-section" data-empty={count === 0 ? 'true' : undefined}>
            <button
                type="button"
                class="session-status-section-header"
                aria-expanded={open}
                onClick={() => {
                    toggled.value = !open;
                }}
            >
                <Caret open={open} />
                <span class="session-status-section-title">{title}</span>
                <span class="session-status-section-count">{count}</span>
            </button>
            {open && (
                <div class="session-status-section-body">
                    {count === 0 ? <div class="session-status-section-empty">{empty}</div> : children}
                </div>
            )}
        </section>
    );
}

function FileRow({ file }: { file: SessionFileEntry }) {
    return (
        <button type="button" class="session-status-file" title={file.path} onClick={() => openPath(file.path)}>
            <span class={`session-status-file-op is-${file.op}`}>
                {file.op === 'added' ? '+' : file.op === 'deleted' ? '−' : '~'}
            </span>
            <span class="session-status-file-path">{displayFilePath(file.path)}</span>
        </button>
    );
}

function SubagentRow({ item, language }: { item: SessionSubagent; language: typeof ui.language.value }) {
    const open = useSignal(false);
    const statusKey =
        item.status === 'running'
            ? 'sessionStatus.subagent.running'
            : item.status === 'failed'
              ? 'sessionStatus.subagent.failed'
              : 'sessionStatus.subagent.completed';
    return (
        <div class="session-status-subagent" data-status={item.status}>
            <button
                type="button"
                class="session-status-subagent-head"
                aria-expanded={open.value}
                onClick={() => {
                    open.value = !open.value;
                }}
            >
                <Caret open={open.value} />
                <span class="session-status-subagent-label">
                    {item.label}
                    <span class="session-status-subagent-id">{item.id.slice(0, 8)}</span>
                </span>
                <span class="session-status-subagent-meta" data-status={item.status}>
                    {t(statusKey, language)}
                    {item.calls.length > 0
                        ? ` · ${t('sessionStatus.subagent.tools', language, { n: item.calls.length })}`
                        : ''}
                </span>
            </button>
            {open.value && (
                <div class="session-status-subagent-body">
                    {item.thinking && <pre class="session-status-pre">{item.thinking}</pre>}
                    {item.output && <pre class="session-status-pre">{item.output}</pre>}
                    {item.calls.map((call, index) => (
                        <div class="session-status-tool" key={call.toolCallId ?? call.id ?? index}>
                            <span class="session-status-tool-name">{call.toolName}</span>
                            {call.status && <span class="session-status-tool-status">{call.status}</span>}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}

function UploadRow({ item }: { item: SessionUpload }) {
    return (
        <button type="button" class="session-status-upload" title={item.path} onClick={() => openPath(item.path)}>
            {item.isImage ? (
                <img class="session-status-upload-thumb" src={fsService.imageUrl(item.path)} alt={item.name} />
            ) : (
                <span class="session-status-upload-icon" aria-hidden="true">
                    <svg viewBox="0 0 24 24" width="14" height="14" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                        <polyline points="14 2 14 8 20 8" />
                    </svg>
                </span>
            )}
            <span class="session-status-upload-name">{item.name}</span>
        </button>
    );
}

function IconButton({ title, onClick, children }: { title: string; onClick: () => void; children: ComponentChildren }) {
    return (
        <button type="button" class="session-status-icon-btn" title={title} onClick={onClick}>
            {children}
        </button>
    );
}

export function BackgroundTaskPanel({ language }: { language: typeof ui.language.value }) {
    const rev = useSignal(0);
    const persistedTurns = useSignal<AgentTurn[]>([]);
    const selectedId = useSignal<string | null>(null);
    const followLatest = useSignal(true);
    const contextOpen = useSignal(false);
    const createOpen = useSignal(false);
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
    const items: ChatItem[] = state?.items ?? [];
    const tasks = state?.backgroundTasks ?? null;
    const typing = !!state?.typing;

    useEffect(() => {
        if (!chat) {
            persistedTurns.value = [];
            return;
        }
        let cancelled = false;
        let refreshing = false;
        const refresh = async () => {
            if (refreshing) return;
            refreshing = true;
            try {
                const page = await activityService.listTurns(chat.workspaceId, { sessionId: chat.id, limit: 100 });
                if (!cancelled) persistedTurns.value = page.items;
            } catch {
                if (!cancelled) persistedTurns.value = [];
            } finally {
                refreshing = false;
            }
        };
        void refresh();
        const timer = typing ? window.setInterval(() => void refresh(), 5000) : null;
        return () => {
            cancelled = true;
            if (timer) window.clearInterval(timer);
        };
    }, [chat?.id, chat?.workspaceId, typing, items.filter(item => item.kind === 'user').length]);

    useEffect(() => {
        if (!tasks?.some(task => task.status === 'running')) return;
        const id = window.setInterval(() => {
            rev.value++;
        }, TICK_MS);
        return () => window.clearInterval(id);
    }, [tasks]);

    const firstSeen = useSignal<Record<string, number>>({});
    const turns = collectSessionTurns(items, persistedTurns.value);
    const selectedIndex = (() => {
        if (turns.length === 0) return -1;
        if (followLatest.value) return turns.length - 1;
        const index = selectedId.value ? turns.findIndex(turn => turn.id === selectedId.value) : -1;
        return index >= 0 ? index : turns.length - 1;
    })();
    const selected: SessionTurnRef | undefined = selectedIndex >= 0 ? turns[selectedIndex] : undefined;

    const selectTurn = (index: number) => {
        if (index < 0 || index >= turns.length) return;
        selectedId.value = turns[index].id;
        followLatest.value = index === turns.length - 1;
    };

    const subagents = selected ? collectSubagents(items, selected) : [];
    const files = selected ? splitTurnFiles(collectTurnFiles(items, selected)) : { code: [], artifacts: [] };
    const uploads = selected ? collectUploads(selected.promptText) : [];
    const media = uploads.filter(item => item.isImage);
    const otherUploads = uploads.filter(item => !item.isImage);
    const turnItems = selected ? itemsForTurn(items, selected) : [];
    const latestAnswer = [...turnItems].reverse().find(item => item.kind === 'assistant_text');

    const now = Date.now();
    for (const task of tasks ?? []) {
        if (task.status !== 'running' || typeof task.startedAt === 'number') continue;
        if (!firstSeen.value[task.id]) firstSeen.value[task.id] = now;
    }

    if (!chat) {
        return (
            <div class="session-status-panel">
                <div class="session-status-empty">
                    <p class="session-status-empty-title">{t('sessionStatus.noSession', language)}</p>
                </div>
            </div>
        );
    }

    return (
        <div class="session-status-panel">
            <header class="session-status-toolbar">
                <IconButton
                    title={
                        contextOpen.value
                            ? t('sessionStatus.closeContext', language)
                            : t('sessionStatus.openContext', language)
                    }
                    onClick={() => {
                        contextOpen.value = !contextOpen.value;
                        if (chat && selected) requestTurnFocus(chat.id, selected.id, selected.aliases);
                    }}
                >
                    <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2">
                        <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z" />
                        <polyline points="14 2 14 8 20 8" />
                        <line x1="8" y1="13" x2="16" y2="13" />
                        <line x1="8" y1="17" x2="13" y2="17" />
                    </svg>
                </IconButton>
                <div class="session-status-turn">
                    <button
                        type="button"
                        class="session-status-turn-nav"
                        disabled={selectedIndex <= 0}
                        title={t('sessionStatus.prevTurn', language)}
                        onClick={() => selectTurn(selectedIndex - 1)}
                    >
                        ‹
                    </button>
                    <div class="session-status-turn-label">
                        <span>
                            {turns.length === 0
                                ? t('sessionStatus.turnEmpty', language)
                                : t('sessionStatus.turnLabel', language, {
                                      n: selectedIndex + 1,
                                      total: turns.length,
                                  })}
                        </span>
                        {selected && (
                            <span class="session-status-turn-prompt" title={selected.promptText}>
                                {promptSnippet(selected.promptText) || t('sessionStatus.turnPromptFallback', language)}
                            </span>
                        )}
                    </div>
                    <button
                        type="button"
                        class="session-status-turn-nav"
                        disabled={selectedIndex < 0 || selectedIndex >= turns.length - 1}
                        title={t('sessionStatus.nextTurn', language)}
                        onClick={() => selectTurn(selectedIndex + 1)}
                    >
                        ›
                    </button>
                </div>
                <div class="session-status-create">
                    <IconButton
                        title={t('sessionStatus.new', language)}
                        onClick={() => {
                            createOpen.value = !createOpen.value;
                        }}
                    >
                        <svg
                            viewBox="0 0 24 24"
                            width="16"
                            height="16"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                        >
                            <line x1="12" y1="5" x2="12" y2="19" />
                            <line x1="5" y1="12" x2="19" y2="12" />
                        </svg>
                    </IconButton>
                    {createOpen.value && (
                        <div class="session-status-create-menu">
                            <button
                                type="button"
                                onClick={() => {
                                    createOpen.value = false;
                                    openFsNewItemModal('file', null);
                                }}
                            >
                                {t('sessionStatus.newFile', language)}
                            </button>
                            <button
                                type="button"
                                onClick={() => {
                                    createOpen.value = false;
                                    openFsNewItemModal('folder', null);
                                }}
                            >
                                {t('sessionStatus.newFolder', language)}
                            </button>
                        </div>
                    )}
                </div>
            </header>

            {contextOpen.value && selected && (
                <section class="session-status-context">
                    <div class="session-status-context-title">{t('sessionStatus.context', language)}</div>
                    <dl class="session-status-context-grid">
                        <div>
                            <dt>{t('sessionStatus.context.status', language)}</dt>
                            <dd>{t(`sessionStatus.status.${selected.status}`, language)}</dd>
                        </div>
                        <div>
                            <dt>{t('sessionStatus.context.started', language)}</dt>
                            <dd>{formatClock(selected.createdAt)}</dd>
                        </div>
                        <div>
                            <dt>{t('sessionStatus.context.completed', language)}</dt>
                            <dd>{formatClock(selected.completedAt)}</dd>
                        </div>
                    </dl>
                    <div class="session-status-context-block">
                        <div class="session-status-context-kicker">{t('sessionStatus.context.prompt', language)}</div>
                        <pre class="session-status-pre">{selected.promptText || '—'}</pre>
                    </div>
                    {latestAnswer && latestAnswer.kind === 'assistant_text' && latestAnswer.content && (
                        <div class="session-status-context-block">
                            <div class="session-status-context-kicker">
                                {t('sessionStatus.context.answer', language)}
                            </div>
                            <pre class="session-status-pre">{promptSnippet(latestAnswer.content, 280)}</pre>
                        </div>
                    )}
                    {selected.errorText && (
                        <div class="session-status-context-block is-error">
                            <div class="session-status-context-kicker">
                                {t('sessionStatus.context.error', language)}
                            </div>
                            <pre class="session-status-pre">{selected.errorText}</pre>
                        </div>
                    )}
                    <button
                        type="button"
                        class="session-status-context-link"
                        onClick={() => requestTurnFocus(chat.id, selected.id, selected.aliases)}
                    >
                        {t('sessionStatus.context.viewInChat', language)}
                    </button>
                </section>
            )}

            <div class="session-status-sections">
                <StatusSection
                    title={t('sessionStatus.section.subagents', language)}
                    count={subagents.length}
                    empty={t('sessionStatus.section.empty', language)}
                >
                    {subagents.map(item => (
                        <SubagentRow key={item.id} item={item} language={language} />
                    ))}
                </StatusSection>

                <StatusSection
                    title={t('sessionStatus.section.files', language)}
                    count={files.code.length}
                    empty={t('sessionStatus.section.empty', language)}
                >
                    {files.code.map(file => (
                        <FileRow key={`${file.op}:${file.path}`} file={file} />
                    ))}
                </StatusSection>

                <StatusSection
                    title={t('sessionStatus.section.artifacts', language)}
                    count={files.artifacts.length}
                    empty={t('sessionStatus.section.empty', language)}
                >
                    {files.artifacts.map(file => (
                        <FileRow key={`${file.op}:${file.path}`} file={file} />
                    ))}
                </StatusSection>

                <StatusSection
                    title={t('sessionStatus.section.uploads', language)}
                    count={uploads.length}
                    empty={t('sessionStatus.section.empty', language)}
                >
                    {media.length > 0 && (
                        <div class="session-status-media">
                            <div class="session-status-media-title">
                                {t('sessionStatus.uploads.media', language)}
                                <span>{media.length}</span>
                            </div>
                            <div class="session-status-media-grid">
                                {media.map(item => (
                                    <UploadRow key={item.path} item={item} />
                                ))}
                            </div>
                        </div>
                    )}
                    {otherUploads.map(item => (
                        <UploadRow key={item.path} item={item} />
                    ))}
                </StatusSection>

                <StatusSection
                    title={t('sessionStatus.section.tasks', language)}
                    count={tasks?.length ?? 0}
                    empty={t('backgroundPanel.empty', language)}
                >
                    <div class="background-task-panel-list">
                        {(tasks ?? []).map(task => (
                            <TaskCard
                                key={task.id}
                                task={task}
                                now={now}
                                startAt={task.startedAt ?? firstSeen.value[task.id]}
                                language={language}
                            />
                        ))}
                    </div>
                </StatusSection>
            </div>
        </div>
    );
}
