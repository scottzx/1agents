// Regression tests for the frontmatter splitter (#83). The `--- / # Title / ---`
// triple used as a decorative README header used to be eaten as YAML
// frontmatter, swallowing `# Title` and the body. It must now be treated as a
// thematic break + H1 + thematic break, while legitimate `key: value`
// frontmatter (the case cards actually author) keeps working.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { splitFrontmatter, parseFrontmatter } from './frontmatter';

const DOC_HEADER_TITLE = '---\n# Title\n---\n\nbody content here';
const DOC_HEADER_TITLE_BODY = '---\n# Title\n---\n\nbody content here';

test('splitFrontmatter: decorative `--- / # Title / ---` triple is NOT frontmatter', () => {
    const { fm, body } = splitFrontmatter(DOC_HEADER_TITLE);
    assert.equal(fm, '');
    // The opening `---` is dropped as decoration; the rest stays in body so the
    // closing `---` still renders as a thematic break.
    assert.equal(body, '# Title\n---\n\nbody content here');
});

test('parseFrontmatter: acceptance is empty when # Title decoy is present', () => {
    const { acceptance, body } = parseFrontmatter(DOC_HEADER_TITLE_BODY);
    assert.deepEqual(acceptance, []);
    assert.equal(body, '# Title\n---\n\nbody content here');
});

test('splitFrontmatter: legitimate frontmatter is still recognized', () => {
    const { fm, body } = splitFrontmatter('---\nacceptance: foo\n---\nbody');
    assert.equal(fm, 'acceptance: foo');
    assert.equal(body, 'body');
});

test('splitFrontmatter: block-scalar acceptance with `|`', () => {
    const { fm, body } = splitFrontmatter('---\nacceptance: |\n  one\n  two\n---\nbody');
    assert.equal(fm, 'acceptance: |\n  one\n  two');
    assert.equal(body, 'body');
});

test('splitFrontmatter: list-style acceptance', () => {
    const { fm, body } = splitFrontmatter('---\nacceptance:\n  - a\n  - b\n---\nbody');
    assert.equal(fm, 'acceptance:\n  - a\n  - b');
    assert.equal(body, 'body');
});

test('parseFrontmatter: acceptance extraction (list)', () => {
    const { acceptance, body } = parseFrontmatter('---\nacceptance:\n  - 分页正确\n  - 空态有提示\n---\n## 背景\n正文');
    assert.deepEqual(acceptance, ['分页正确', '空态有提示']);
    assert.equal(body, '## 背景\n正文');
});

test('splitFrontmatter: multi-key frontmatter is preserved verbatim', () => {
    const doc = '---\npriority: high\nacceptance: foo\n---\nbody';
    const { fm, body } = splitFrontmatter(doc);
    assert.equal(fm, 'priority: high\nacceptance: foo');
    assert.equal(body, 'body');
});

test('splitFrontmatter: opening `---` with no closing fence → whole thing is body', () => {
    const { fm, body } = splitFrontmatter('---\nacceptance: still body');
    assert.equal(fm, '');
    assert.equal(body, '---\nacceptance: still body');
});

test('splitFrontmatter: no frontmatter at all', () => {
    const { fm, body } = splitFrontmatter('just text');
    assert.equal(fm, '');
    assert.equal(body, 'just text');
});

test('splitFrontmatter: empty frontmatter `--- / ---` is fine', () => {
    const { fm, body } = splitFrontmatter('---\n---\nbody');
    assert.equal(fm, '');
    assert.equal(body, 'body');
});

test('splitFrontmatter: thematic break deeper in the doc is not promoted', () => {
    // The first `---` opens a candidate that fails the YAML-shape check, so the
    // splitter keeps scanning. No later `---` closes a YAML block, so the entire
    // doc returns as body.
    const doc = '---\n# Title\n---\n\n# Heading 2\n\n---\n\nbody';
    const { fm, body } = splitFrontmatter(doc);
    assert.equal(fm, '');
    assert.equal(body.startsWith('# Title\n---\n\n# Heading 2'), true);
});

test('splitFrontmatter: malformed YAML inside `--- / ---` falls through to body', () => {
    // Real prose between the fences — not a YAML mapping — must NOT be parsed
    // as frontmatter.
    const { fm, body } = splitFrontmatter('---\nreal prose\n---\nreal body');
    assert.equal(fm, '');
    assert.equal(body, 'real prose\n---\nreal body');
});

test('splitFrontmatter: leading BOM is tolerated', () => {
    const { fm, body } = splitFrontmatter('﻿---\nacceptance: x\n---\nbody');
    assert.equal(fm, 'acceptance: x');
    assert.equal(body, 'body');
});
