// Run: cd frontend && npx tsx --test src/components/drawer/sessionStatusModel.test.ts

import assert from 'node:assert/strict';
import { test } from 'node:test';

import type { ChatItem } from '@1agents/core/protocol/types';
import type { AgentTurn } from '@1agents/core/services/activityService';
import {
    collectSessionTurns,
    collectSubagents,
    collectTurnFiles,
    collectUploads,
    displayWorkdirPath,
    isArtifactPath,
    promptSnippet,
    splitTurnFiles,
} from './sessionStatusModel';

const user = (id: string, content: string, extra: Partial<Extract<ChatItem, { kind: 'user' }>> = {}): ChatItem => ({
    id,
    kind: 'user',
    content,
    createdAt: 1,
    ...extra,
});

const answer = (id: string, extra: Partial<Extract<ChatItem, { kind: 'assistant_text' }>> = {}): ChatItem => ({
    id,
    kind: 'assistant_text',
    content: id,
    createdAt: 2,
    streaming: false,
    ...extra,
});

const persisted = (id: string, promptText: string, extra: Partial<AgentTurn> = {}): AgentTurn => ({
    id,
    projectId: 'p1',
    sessionId: 's1',
    status: 'completed',
    promptText,
    createdAt: '2026-01-01T00:00:00Z',
    updatedAt: '2026-01-01T00:00:00Z',
    ...extra,
});

test('queued user messages do not become turns', () => {
    const turns = collectSessionTurns(
        [user('u1', 'go'), answer('a1'), user('u2', 'later', { queueStatus: 'queued' })],
        []
    );
    assert.equal(turns.length, 1);
    assert.equal(turns[0].promptText, 'go');
});

test('attaches persisted turn identity and change reports via aliases', () => {
    const turns = collectSessionTurns(
        [user('u1', 'implement login', { turnId: 'req-1' }), answer('a1', { turnId: 'req-1' })],
        [
            persisted('turn-1', 'implement login', {
                clientRequestId: 'req-1',
                changeReport: {
                    turnId: 'turn-1',
                    recipeVersion: 1,
                    addedCount: 1,
                    deletedCount: 0,
                    modifiedCount: 1,
                    files: [
                        { path: 'src/auth.ts', op: 'added' },
                        { path: 'docs/walkthrough.md', op: 'added' },
                    ],
                    source: 'live',
                    computedAt: '2026-01-01T00:00:00Z',
                },
            }),
        ]
    );

    assert.equal(turns.length, 1);
    assert.equal(turns[0].id, 'turn-1');
    assert.ok(turns[0].aliases.includes('req-1'));
    const files = collectTurnFiles(
        [user('u1', 'implement login', { turnId: 'req-1' }), answer('a1', { turnId: 'req-1' })],
        turns[0]
    );
    const split = splitTurnFiles(files);
    assert.deepEqual(
        split.code.map(file => file.path),
        ['src/auth.ts']
    );
    assert.deepEqual(
        split.artifacts.map(file => file.path),
        ['docs/walkthrough.md']
    );
});

test('collects subagents only from the selected turn slice', () => {
    const items: ChatItem[] = [
        user('u1', 'first'),
        {
            id: 'sub-old',
            kind: 'subagent_turn',
            agentTurnId: 'old',
            label: 'researcher',
            thinking: 'old thought',
            output: '',
            calls: [],
            streaming: false,
            createdAt: 2,
        },
        answer('a1'),
        user('u2', 'second'),
        {
            id: 'sub-new',
            kind: 'subagent_turn',
            agentTurnId: 'new',
            label: 'explorer',
            thinking: 'new thought',
            output: 'done',
            calls: [{ toolName: 'list_dir', input: '{}', status: 'completed' }],
            streaming: false,
            createdAt: 4,
        },
    ];
    const turns = collectSessionTurns(items, []);
    assert.equal(turns.length, 2);
    assert.deepEqual(
        collectSubagents(items, turns[0]).map(sub => sub.id),
        ['old']
    );
    assert.deepEqual(
        collectSubagents(items, turns[1]).map(sub => sub.id),
        ['new']
    );
    assert.equal(collectSubagents(items, turns[1])[0].status, 'completed');
});

test('deploy bash is not a file change', () => {
    const items: ChatItem[] = [
        user('u1', 'deploy'),
        {
            id: 'tool-bash',
            kind: 'tool_use',
            toolName: 'bash',
            input: '',
            calls: [
                {
                    toolName: 'Bash',
                    input: JSON.stringify({
                        command: 'cd /opt/deploy && make -C frontend && rsync -a dist/ /var/www/app',
                    }),
                    kind: 'execute',
                    locations: [{ path: '/opt/deploy' }, { path: '/var/www/app' }],
                    status: 'completed',
                },
            ],
            createdAt: 2,
        },
    ];
    const turns = collectSessionTurns(items, []);
    assert.deepEqual(collectTurnFiles(items, turns[0]), []);
});

test('shell rm is a deleted file change', () => {
    const items: ChatItem[] = [
        user('u1', 'delete'),
        {
            id: 'tool-rm',
            kind: 'tool_use',
            toolName: 'bash',
            input: '',
            calls: [
                {
                    toolName: 'Bash',
                    input: JSON.stringify({ command: 'rm -f .tmp/1acp-turn-smoke.txt' }),
                    kind: 'execute',
                    status: 'completed',
                },
            ],
            createdAt: 2,
        },
    ];
    const turns = collectSessionTurns(items, []);
    const files = collectTurnFiles(items, turns[0]);
    assert.equal(files.length, 1);
    assert.equal(files[0].path, '.tmp/1acp-turn-smoke.txt');
    assert.equal(files[0].op, 'deleted');
});

test('live tool calls fill files when no change report exists', () => {
    const items: ChatItem[] = [
        user('u1', 'edit'),
        {
            id: 'tool-1',
            kind: 'tool_use',
            toolName: 'write',
            input: '',
            calls: [
                {
                    toolName: 'Write',
                    input: JSON.stringify({ path: 'src/app.ts' }),
                    kind: 'edit',
                    locations: [{ path: 'src/app.ts' }],
                    status: 'completed',
                },
            ],
            createdAt: 2,
        },
    ];
    const turns = collectSessionTurns(items, []);
    const files = collectTurnFiles(items, turns[0]);
    assert.equal(files.length, 1);
    assert.equal(files[0].path, 'src/app.ts');
    assert.equal(files[0].op, 'added');
    assert.equal(files[0].kind, 'code');
});

test('extracts standalone upload paths and media from the user prompt', () => {
    const uploads = collectUploads(
        '请看这张图\n/tmp/shot.png\n/tmp/notes.txt\n不要把 https://example.com/a.png 算进去'
    );
    assert.deepEqual(
        uploads.map(item => [item.name, item.isImage]),
        [
            ['shot.png', true],
            ['notes.txt', false],
        ]
    );
});

test('artifact classifier recognizes plan and markdown deliverables', () => {
    assert.equal(isArtifactPath('implementation_plan.md'), true);
    assert.equal(isArtifactPath('src/main.go'), false);
    assert.equal(promptSnippet('hello world\n/tmp/a.png'), 'hello world');
});

test('displayWorkdirPath hides the current pwd and keeps outside paths absolute', () => {
    const pwd = '/Users/scott/proj';
    assert.equal(displayWorkdirPath('/Users/scott/proj/src/app.ts', pwd), 'src/app.ts');
    assert.equal(displayWorkdirPath('/Users/scott/proj/', pwd), '.');
    assert.equal(displayWorkdirPath('src/app.ts', pwd), 'src/app.ts');
    assert.equal(displayWorkdirPath('/tmp/other.txt', pwd), '/tmp/other.txt');
    assert.equal(displayWorkdirPath('/Users/scott/proj-other/a.ts', pwd), '/Users/scott/proj-other/a.ts');
});
