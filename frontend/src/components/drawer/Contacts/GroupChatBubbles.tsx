import { h } from 'preact';

import type { Lang } from '../../../i18n';
import type { FeishuMessage } from '@1agents/core/services/contactService';

// 群聊气泡 (group-chat bubbles) — a lightweight multi-sender replay renderer for a
// contact's group messages. Deliberately NOT the agent chat UI (which is
// protocol-coupled + single-sender): a group replay has many senders and no
// "me", so every bubble is left-aligned. Consecutive messages from the same
// sender collapse the avatar + name for a clean chat look. Visual reference is
// the .chat-bubble family, but the component is independent.

function initials(name: string): string {
    const n = name.trim();
    if (!n) return '?';
    if (/[一-龥]/.test(n)) return n.slice(0, 1);
    return n
        .split(/\s+/)
        .map(w => w[0])
        .join('')
        .slice(0, 2)
        .toUpperCase();
}

function shortId(id: string): string {
    return id.length > 10 ? `…${id.slice(-8)}` : id;
}

function msgText(m: FeishuMessage): string {
    if (m.MsgType === 'text') {
        try {
            const parsed = JSON.parse(m.Content) as { text?: string };
            if (parsed.text) return parsed.text;
        } catch {
            /* fall through */
        }
    }
    return `[${m.MsgType}]`;
}

// senderColorIndex maps a senderId deterministically onto one of N palette
// slots, so each sender keeps a stable avatar color across the replay. A simple
// summed-char hash is enough (collisions only mean two senders share a hue).
const PALETTE_SIZE = 8;
function senderColorIndex(senderId: string): number {
    let h = 0;
    for (let i = 0; i < senderId.length; i++) {
        h = (h * 31 + senderId.charCodeAt(i)) >>> 0;
    }
    return h % PALETTE_SIZE;
}

export function GroupChatBubbles({ messages, language }: { messages: FeishuMessage[]; language: Lang }) {
    return (
        <div class="contacts-chat-list">
            {messages.map((m, i) => {
                const prev = messages[i - 1];
                // First of a run: previous message is from a different sender.
                const runStart = !prev || prev.SenderID !== m.SenderID;
                const name = m.SenderName || shortId(m.SenderID);
                const colorIdx = senderColorIndex(m.SenderID || name);
                return (
                    <div key={m.MessageID} class={`contacts-chat-row${runStart ? ' run-start' : ''}`}>
                        <div class="contacts-chat-avatar-col">
                            {runStart ? (
                                <span class={`contacts-chat-avatar c${colorIdx}`}>{initials(name)}</span>
                            ) : (
                                <span class="contacts-chat-avatar-spacer" />
                            )}
                        </div>
                        <div class="contacts-chat-bubble-col">
                            {runStart && <span class="contacts-chat-sender">{name}</span>}
                            <div class="contacts-chat-bubble">
                                <span class="contacts-chat-text">{msgText(m)}</span>
                                <span class="contacts-chat-time">
                                    {new Date(m.CreateTime).toLocaleString(language)}
                                </span>
                            </div>
                        </div>
                    </div>
                );
            })}
        </div>
    );
}
