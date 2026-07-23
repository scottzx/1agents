// Props contract + readOnly behaviour for EmbeddedChat helpers.
// Run: cd frontend && npx tsx --test src/components/chat/embeddedChatUtils.test.ts

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { formatMaxHeight, resolveEmbeddedSession, shouldShowComposer } from './embeddedChatUtils';
import type { ChatSession } from '../types';

const baseSession = (over: Partial<ChatSession> = {}): ChatSession => ({
    kind: 'chat',
    id: 'sess-1',
    workspaceId: 'ws-1',
    name: 'Demo',
    agentType: 'grok-build',
    ccProject: 'p',
    ccSessionId: 'cc-1',
    sessionKey: 'k',
    status: 'idle',
    active: true,
    ...over,
});

test('resolveEmbeddedSession prefers full session object', () => {
    const session = baseSession({ id: 'full' });
    const got = resolveEmbeddedSession({ session, sessionId: 'other' });
    assert.equal(got, session);
    assert.equal(got?.id, 'full');
});

test('resolveEmbeddedSession builds stub from sessionId + workspaceId', () => {
    const got = resolveEmbeddedSession({
        sessionId: ' seat-sess ',
        workspaceId: ' tmp-ws ',
        agentType: 'grok-build',
        acpSessionId: 'acp-9',
    });
    assert.ok(got);
    assert.equal(got!.kind, 'chat');
    assert.equal(got!.id, 'seat-sess');
    assert.equal(got!.workspaceId, 'tmp-ws');
    assert.equal(got!.agentType, 'grok-build');
    assert.equal(got!.acpSessionId, 'acp-9');
    assert.equal(got!.active, false);
});

test('resolveEmbeddedSession returns null without id or workspace for stub', () => {
    assert.equal(resolveEmbeddedSession({}), null);
    assert.equal(resolveEmbeddedSession({ sessionId: 'x' }), null);
    assert.equal(resolveEmbeddedSession({ workspaceId: 'w' }), null);
});

test('resolveEmbeddedSession reuses sessionStore row when present', () => {
    const store = [baseSession({ id: 's-store', name: 'From store', agentType: 'codex' })];
    const got = resolveEmbeddedSession({ sessionId: 's-store' }, store);
    assert.ok(got);
    assert.equal(got!.name, 'From store');
    assert.equal(got!.agentType, 'codex');
    assert.equal(got!.workspaceId, 'ws-1');
});

test('resolveEmbeddedSession store row can override workspace/agent via props', () => {
    const store = [baseSession({ id: 's-store' })];
    const got = resolveEmbeddedSession(
        { sessionId: 's-store', workspaceId: 'ws-override', agentType: 'gemini' },
        store
    );
    assert.equal(got!.workspaceId, 'ws-override');
    assert.equal(got!.agentType, 'gemini');
});

test('shouldShowComposer: readOnly always hides', () => {
    assert.equal(shouldShowComposer(true), false);
    assert.equal(shouldShowComposer(true, true), false);
    assert.equal(shouldShowComposer(true, false), false);
});

test('shouldShowComposer: non-readOnly defaults to visible', () => {
    assert.equal(shouldShowComposer(false), true);
    assert.equal(shouldShowComposer(undefined), true);
    assert.equal(shouldShowComposer(false, true), true);
    assert.equal(shouldShowComposer(false, false), false);
});

test('formatMaxHeight defaults and units', () => {
    assert.equal(formatMaxHeight(), '280px');
    assert.equal(formatMaxHeight(240), '240px');
    assert.equal(formatMaxHeight('40vh'), '40vh');
    assert.equal(formatMaxHeight(''), '280px');
});
