import { h, Fragment, type ComponentChildren } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { MessageBubble, GroupedChatItem, GroupedToolCall, ToolGroupElement } from './MessageBubble';
import type { ChatItem } from './hooks';
import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import { groupHistoricalTurns } from './turns';

interface MessageListProps {
    items: ChatItem[];
    /**
     * True while a turn is running. Drives the tool-call status icons
     * (spinner vs. neutral "incomplete" for history replays of
     * cancelled turns) and the persistent typing wave at the bottom
     * of the message list while the turn is in progress.
     */
    typing?: boolean;
    emptyHint?: string;
    /**
     * When true, render a centered spinner placeholder instead of the
     * normal empty hint. Used during the bridge's `ensure_session` window
     * for new chats so users see "preparing session" rather than a hint
     * that implies they can type immediately.
     */
    loading?: boolean;
    loadingHint?: string;
    /**
     * Per-queue-prompt cancel. Wired to the X button on queued user
     * bubbles — distinct from the global "stop the current turn" cancel
     * which only stops `activeTurn` and leaves the queue running.
     */
    onCancelQueued?: (queueRequestId: string) => void;
    /** Optional references rendered inside the same scroll timeline. */
    timelineFooter?: ComponentChildren;
    /** One-shot request to reveal a persisted Turn after cross-navigation. */
    focusTurn?: { turnId: string; nonce: number } | null;
}

function isCallRenderable(call: GroupedToolCall): boolean {
    // A call is renderable as soon as we know *something* concrete about it:
    //   - it has a toolCallId (the runtime committed to this call — render
    //     a streaming placeholder even before the arguments JSON arrives)
    //   - it has an attached permission request (pending or resolved)
    //   - it already produced an output
    //   - it has parseable input
    // This fixes the "invisible tool card until arguments stream in" bug.
    if (call.toolCallId) return true;
    if (call.output !== undefined) return true;
    if (call.permission) return true;
    if (call.askUser) return true;
    if (call.exitPlan) return true;
    if (!call.input || !call.input.trim()) return false;
    try {
        const parsed = JSON.parse(call.input);
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
            return Object.keys(parsed as Record<string, unknown>).length > 0;
        }
        return true;
    } catch {
        // Non-JSON but has content — render as raw.
        return true;
    }
}

function groupChatItems(items: ChatItem[]): GroupedChatItem[] {
    const grouped: GroupedChatItem[] = [];
    const pendingCalls: GroupedToolCall[] = [];
    let currentProcessGroup: Extract<GroupedChatItem, { kind: 'tool_group' }> | null = null;

    const ensureProcessGroup = (
        id: string,
        createdAt: number,
        turnId?: string,
        turnStatus?: ChatItem['turnStatus']
    ): Extract<GroupedChatItem, { kind: 'tool_group' }> => {
        if (currentProcessGroup && turnId && currentProcessGroup.turnId && currentProcessGroup.turnId !== turnId) {
            currentProcessGroup = null;
        }
        if (currentProcessGroup) return currentProcessGroup;
        currentProcessGroup = {
            id: `group-${id}`,
            kind: 'tool_group',
            calls: [],
            thinkingBlocks: [],
            elements: [],
            createdAt,
            turnId,
            turnStatus,
        };
        grouped.push(currentProcessGroup);
        return currentProcessGroup;
    };

    for (const item of items) {
        if (item.kind === 'tool_use') {
            const lastGroup = ensureProcessGroup(item.id, item.createdAt, item.turnId, item.turnStatus);

            if (!lastGroup.thinkingBlocks) lastGroup.thinkingBlocks = [];
            if (!lastGroup.elements) lastGroup.elements = [];

            for (const call of item.calls) {
                const callId = call.toolCallId;
                const existingCall = callId ? lastGroup.calls.find(c => c.toolCallId === callId) : null;
                if (existingCall) {
                    existingCall.toolName = call.toolName;
                    existingCall.input = call.input;
                    existingCall.output = call.output;
                    existingCall.isError = call.isError;
                    if (call.permission) existingCall.permission = call.permission;
                    if (call.askUser) existingCall.askUser = call.askUser;
                    if (call.exitPlan) existingCall.exitPlan = call.exitPlan;
                    // Metadata (Phase 6) + ACP status arrive on later updates — merge, don't clear.
                    if (call.kind) existingCall.kind = call.kind;
                    if (call.status) existingCall.status = call.status;
                    if (call.locations) existingCall.locations = call.locations;
                    if (call.diffs) existingCall.diffs = call.diffs;
                } else {
                    // Key falls back to the group-relative position when the
                    // runtime didn't supply a toolCallId — stable across
                    // re-renders, unlike Math.random() which remounted the
                    // row (and dropped its expand state) on every render.
                    const newCall: GroupedToolCall = {
                        id: `call-${callId || lastGroup.calls.length}`,
                        toolCallId: callId,
                        toolName: call.toolName,
                        input: call.input,
                        output: call.output,
                        isError: call.isError,
                        ...(call.kind ? { kind: call.kind } : {}),
                        ...(call.status ? { status: call.status } : {}),
                        ...(call.locations ? { locations: call.locations } : {}),
                        ...(call.diffs ? { diffs: call.diffs } : {}),
                        ...(call.permission ? { permission: call.permission } : {}),
                        ...(call.askUser ? { askUser: call.askUser } : {}),
                        ...(call.exitPlan ? { exitPlan: call.exitPlan } : {}),
                    };
                    lastGroup.calls.push(newCall);
                    lastGroup.elements.push({ kind: 'call', call: newCall });
                }
            }
        } else if (item.kind === 'thinking') {
            const lastGroup = ensureProcessGroup(item.id, item.createdAt, item.turnId, item.turnStatus);

            if (!lastGroup.thinkingBlocks) lastGroup.thinkingBlocks = [];
            if (!lastGroup.elements) lastGroup.elements = [];

            const existingElement = lastGroup.elements.find(el => el.kind === 'thinking' && el.id === item.id);
            if (existingElement && existingElement.kind === 'thinking') {
                existingElement.content = item.content;
            } else {
                lastGroup.elements.push({
                    kind: 'thinking',
                    id: item.id,
                    content: item.content,
                });
            }

            lastGroup.thinkingBlocks = lastGroup.elements
                .filter(el => el.kind === 'thinking')
                .map(el => (el as Extract<ToolGroupElement, { kind: 'thinking' }>).content);
        } else if (item.kind === 'tool_result') {
            const callId = item.toolCallId;
            let matchedCall: GroupedToolCall | null = null;
            let matchedGroup: Extract<GroupedChatItem, { kind: 'tool_group' }> | null = null;

            // Search backward for the most recent non-pending group
            // that contains this callId. The pending group is skipped
            // — it's just a holding pen for unmatched entries, not a
            // legitimate target for new attachments.
            if (callId) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group' && !g.pending) {
                        const c = g.calls.find(call => call.toolCallId === callId);
                        if (c) {
                            matchedCall = c;
                            matchedGroup = g;
                            break;
                        }
                    }
                }
            }

            // Fallback: if not found by callId, attach to the most
            // recent non-pending group's last call (mirrors realtime
            // tool_result's reverse-scan fallback).
            if (!matchedCall) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group' && !g.pending) {
                        matchedGroup = g;
                        break;
                    }
                }
                if (matchedGroup && matchedGroup.calls.length > 0) {
                    matchedCall = matchedGroup.calls.find(c => c.output === undefined) || null;
                    if (!matchedCall) {
                        matchedCall = matchedGroup.calls[matchedGroup.calls.length - 1];
                    }
                }
            }

            if (matchedCall) {
                matchedCall.output = item.content;
                matchedCall.isError = item.isError;
                matchedCall.status = item.isError ? 'failed' : 'completed';
            } else {
                // No tool_use matched — park the result in the
                // pending pool. A later tool_use with the right
                // toolCallId (or a history reload) will attach it.
                pendingCalls.push({
                    id: `pending-result-${item.id}`,
                    toolCallId: callId,
                    toolName: item.toolName || 'tool',
                    input: '',
                    output: item.content,
                    isError: item.isError,
                    status: item.isError ? 'failed' : 'completed',
                });
            }
        } else if (item.kind === 'permission_request') {
            const callId = item.toolCallId;
            let matchedCall: GroupedToolCall | null = null;

            if (callId) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group' && !g.pending) {
                        const c = g.calls.find(call => call.toolCallId === callId);
                        if (c) {
                            matchedCall = c;
                            break;
                        }
                    }
                }
            }

            if (matchedCall) {
                matchedCall.permission = {
                    requestId: item.requestId,
                    toolName: item.toolName,
                    input: item.input,
                    options: item.options,
                    ...(item.resolved ? { resolved: item.resolved } : {}),
                };
            } else {
                pendingCalls.push({
                    id: `pending-permission-${item.id}`,
                    toolCallId: callId,
                    toolName: item.toolName,
                    // Surface the permission's tool input as the call input —
                    // the row's args section is the only place the user can
                    // inspect what the orphan request wants to run.
                    input: item.input,
                    output: undefined,
                    isError: undefined,
                    permission: {
                        requestId: item.requestId,
                        toolName: item.toolName,
                        input: item.input,
                        options: item.options,
                        ...(item.resolved ? { resolved: item.resolved } : {}),
                    },
                });
            }
        } else if (item.kind === 'ask_user_question') {
            const callId = item.toolCallId;
            let matchedCall: GroupedToolCall | null = null;

            if (callId) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group' && !g.pending) {
                        const c = g.calls.find(call => call.toolCallId === callId);
                        if (c) {
                            matchedCall = c;
                            break;
                        }
                    }
                }
            }

            const askUser = {
                requestId: item.requestId,
                toolCallId: callId,
                mode: item.mode,
                questions: item.questions,
                ...(item.resolved ? { resolved: item.resolved } : {}),
                ...(item.answers ? { answers: item.answers } : {}),
            };

            if (matchedCall) {
                matchedCall.askUser = askUser;
            } else {
                pendingCalls.push({
                    id: `pending-ask-user-${item.id}`,
                    toolCallId: callId,
                    toolName: 'ask_user_question',
                    input: JSON.stringify({ questions: item.questions }, null, 2),
                    askUser,
                });
            }
        } else if (item.kind === 'exit_plan_mode') {
            const callId = item.toolCallId;
            let matchedCall: GroupedToolCall | null = null;

            if (callId) {
                for (let i = grouped.length - 1; i >= 0; i--) {
                    const g = grouped[i];
                    if (g.kind === 'tool_group' && !g.pending) {
                        const c = g.calls.find(call => call.toolCallId === callId);
                        if (c) {
                            matchedCall = c;
                            break;
                        }
                    }
                }
            }

            const exitPlan = {
                requestId: item.requestId,
                toolCallId: callId,
                planContent: item.planContent,
                ...(item.resolved ? { resolved: item.resolved } : {}),
                ...(item.comments ? { comments: item.comments } : {}),
            };

            if (matchedCall) {
                matchedCall.exitPlan = exitPlan;
            } else {
                pendingCalls.push({
                    id: `pending-exit-plan-${item.id}`,
                    toolCallId: callId,
                    toolName: 'exit_plan_mode',
                    input: item.planContent?.slice(0, 500) || '',
                    exitPlan,
                });
            }
        } else {
            // Assistant replies, user messages, and errors live outside the
            // tool group. Closing the group keeps chronological order when
            // more tools arrive after an intermediate assistant reply.
            if (
                item.kind === 'user' ||
                item.kind === 'error' ||
                item.kind === 'assistant_text' ||
                item.kind === 'turn_receipt'
            ) {
                currentProcessGroup = null;
            }
            grouped.push(item as GroupedChatItem);
        }
    }

    if (pendingCalls.length > 0) {
        grouped.push({
            id: 'pending-group',
            kind: 'tool_group',
            calls: pendingCalls,
            thinkingBlocks: [],
            elements: pendingCalls.map(c => ({ kind: 'call', call: c })),
            createdAt: Date.now(),
            pending: true,
        });
    }

    return grouped;
}

export function MessageList({
    items,
    typing,
    emptyHint,
    loading,
    loadingHint,
    onCancelQueued,
    timelineFooter,
    focusTurn,
}: MessageListProps) {
    const scrollRef = useRef<HTMLDivElement | null>(null);
    // Whether the user is currently stuck to the bottom. Tracked from
    // real scroll events (before content updates) rather than measured
    // after a render — a large streamed chunk can push the post-render
    // distance past the threshold and would otherwise break auto-follow.
    const pinnedRef = useRef(true);
    // A pointer is pressed inside the list. While true we suspend
    // auto-follow so a streamed token can't snap scrollTop between a
    // header's mousedown and mouseup and silently swallow the click.
    const interactingRef = useRef(false);
    const focusedNonceRef = useRef<number | null>(null);

    const handleScroll = () => {
        const el = scrollRef.current;
        if (!el) return;
        pinnedRef.current = el.scrollHeight - el.scrollTop - el.clientHeight < 120;
    };

    // Follow new content only when the user is pinned to the bottom and
    // not mid-click, so the scroll never yanks an element out from under
    // a pointer press.
    useEffect(() => {
        const el = scrollRef.current;
        if (!el) return;
        if (!pinnedRef.current || interactingRef.current) return;
        el.scrollTop = el.scrollHeight;
    }, [items, typing, timelineFooter]);

    useEffect(() => {
        if (!focusTurn || focusedNonceRef.current === focusTurn.nonce) return;
        const frame = requestAnimationFrame(() => {
            const el = scrollRef.current;
            if (!el) return;
            const anchors = Array.from(el.querySelectorAll<HTMLElement>('[data-turn-anchor]'));
            const anchor = anchors.find(node => node.dataset.turnAnchor === focusTurn.turnId);
            if (!anchor) return;
            focusedNonceRef.current = focusTurn.nonce;
            anchor.scrollIntoView({ block: 'start', behavior: 'smooth' });
        });
        return () => cancelAnimationFrame(frame);
    }, [focusTurn, items]);

    if (loading) {
        // Spinner takes priority over the empty hint: while the bridge is
        // spinning up the agent we don't want to advertise an "empty
        // conversation, send a message" prompt that the composer can't
        // honor yet (it would be disabled and the user would wonder why).
        return (
            <div class="chat-empty chat-loading">
                <div class="chat-loading-spinner" aria-hidden="true" />
                <p>{loadingHint ?? t('chat.initializing', ui.language.value)}</p>
            </div>
        );
    }

    if (items.length === 0 && !timelineFooter) {
        // Speaking / mid-turn with no bubbles yet: still show typing wave so
        // embed cards (roundtable seats) are never a blank "empty" void.
        if (typing) {
            return (
                <div class="chat-messages">
                    <div class="chat-messages-body" ref={scrollRef}>
                        <div class="chat-empty chat-empty-typing">
                            <p>{emptyHint ?? t('chat.empty.send', ui.language.value)}</p>
                        </div>
                        <div class="chat-typing-row" aria-label="thinking">
                            <span class="chat-typing-dot" />
                            <span class="chat-typing-dot" />
                            <span class="chat-typing-dot" />
                        </div>
                    </div>
                </div>
            );
        }
        return (
            <div class="chat-empty">
                <p>{emptyHint ?? t('chat.empty.send', ui.language.value)}</p>
            </div>
        );
    }

    const processGroupedItems: GroupedChatItem[] = [];
    for (const item of groupChatItems(items)) {
        if (item.kind !== 'tool_group') {
            processGroupedItems.push(item);
            continue;
        }
        // Drop empty calls so we don't briefly render "工具调用 1" with no
        // body while waiting for the streaming input to land. If everything
        // is empty, hide the whole group — it'll reappear once content
        // arrives.
        const hasThinking = item.thinkingBlocks && item.thinkingBlocks.length > 0;
        const renderable = item.calls.filter(isCallRenderable);
        if (renderable.length === 0 && !hasThinking) continue;
        if (renderable.length === item.calls.length) {
            processGroupedItems.push(item);
        } else {
            const renderableCallIds = new Set(renderable.map(c => c.id));
            const filteredElements = item.elements?.filter(el => {
                if (el.kind === 'thinking') return true;
                return renderableCallIds.has(el.call.id);
            });
            processGroupedItems.push({
                ...item,
                calls: renderable,
                elements: filteredElements,
            });
        }
    }

    // Turn is a presentation layer around the existing process groups. Older
    // completed turns fold their thoughts/tools and retain only their final
    // answer; the newest turn stays flat so live activity remains visible.
    const groupedItems = groupHistoricalTurns(processGroupedItems);

    // Always show the typing wave while a turn is running so users have
    // a persistent "still working" affordance even after thinking/text/
    // tool blocks start streaming.
    const showTyping = !!typing;
    let activeProcessIndex = -1;
    if (typing) {
        for (let i = groupedItems.length - 1; i >= 0; i--) {
            if (groupedItems[i].kind === 'user') break;
            if (groupedItems[i].kind === 'tool_group') {
                activeProcessIndex = i;
                break;
            }
        }
    }

    // Newest assistant body is unlimited; earlier ones keep max-height fold.
    let latestAssistantIndex = -1;
    for (let i = groupedItems.length - 1; i >= 0; i--) {
        if (groupedItems[i].kind === 'assistant_text') {
            latestAssistantIndex = i;
            break;
        }
    }

    // Single consolidated track: all tool groups, per-turn content, long text blocks
    // and nested elements now share one main scroll track (merged from multiple wheels).
    return (
        <div class="chat-messages">
            <div
                class="chat-message-track"
                ref={scrollRef}
                onScroll={handleScroll}
                onPointerDown={() => {
                    interactingRef.current = true;
                }}
                onPointerUp={() => {
                    interactingRef.current = false;
                }}
                onPointerCancel={() => {
                    interactingRef.current = false;
                }}
            >
                {groupedItems.map((item, index) => (
                    <Fragment key={item.id}>
                        {item.turnId && (
                            <span
                                data-turn-anchor={item.turnId}
                                aria-hidden="true"
                                style={{ display: 'block', height: 0 }}
                            />
                        )}
                        <MessageBubble
                            item={item}
                            isLast={index === groupedItems.length - 1}
                            active={typing && index === activeProcessIndex}
                            isLatestAssistant={index === latestAssistantIndex}
                            onCancelQueued={onCancelQueued}
                        />
                    </Fragment>
                ))}
                {showTyping && (
                    <div class="chat-typing-row" aria-label="thinking">
                        <span class="chat-typing-dot" />
                        <span class="chat-typing-dot" />
                        <span class="chat-typing-dot" />
                    </div>
                )}
                {timelineFooter}
            </div>
        </div>
    );
}
