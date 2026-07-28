import assert from 'node:assert/strict';
import test from 'node:test';

import { parseMarkdownFileLink } from './fileLinks';
import { renderMarkdown } from './markdown';

test('assistant Markdown renders headings, lists, and explicit links', () => {
    const html = renderMarkdown(
        '## PRD 与设计文档\n\n- [验收标准](/Users/scott/project/docs/features/turn-model/prd.md:561)'
    );
    assert.match(html, /<h2>PRD 与设计文档<\/h2>/);
    assert.match(html, /<ul>/);
    assert.match(html, /href="\/Users\/scott\/project\/docs\/features\/turn-model\/prd\.md:561"/);
});

test('parses an absolute Markdown file link with a target line', () => {
    assert.deepEqual(
        parseMarkdownFileLink(
            '/Users/scott/Documents/01-%E5%BC%80%E5%8F%91%E9%A1%B9%E7%9B%AE/1agents/docs/features/turn-model/prd.md:416'
        ),
        {
            path: '/Users/scott/Documents/01-开发项目/1agents/docs/features/turn-model/prd.md',
            line: 416,
        }
    );
});

test('parses relative file links and line ranges', () => {
    assert.deepEqual(parseMarkdownFileLink('../docs/design.md:14-20'), {
        path: '../docs/design.md',
        line: 14,
        lineEnd: 20,
    });
});

test('does not mistake web or application links for files', () => {
    assert.equal(parseMarkdownFileLink('https://example.com/readme.md:40'), null);
    assert.equal(parseMarkdownFileLink('/project/tasks/40'), null);
    assert.equal(parseMarkdownFileLink('#section'), null);
});
