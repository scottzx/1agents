// Regression: bare `---` must render as <hr>, not a setext H2 underline.
// Chat / file preview users write section dividers without blank lines.

import assert from 'node:assert/strict';
import test from 'node:test';
import { normalizeMarkdownThematicBreaks, renderMarkdown } from './markdown';

test('normalize: inserts blank line so text\\n--- is not setext', () => {
    const out = normalizeMarkdownThematicBreaks('上一段\n---\n下一段');
    assert.equal(out, '上一段\n\n---\n下一段');
});

test('normalize: leaves already-blank-surrounded hr alone', () => {
    const src = 'a\n\n---\n\nb';
    assert.equal(normalizeMarkdownThematicBreaks(src), src);
});

test('normalize: does not touch --- inside fenced code', () => {
    const src = '```\nfoo\n---\nbar\n```';
    assert.equal(normalizeMarkdownThematicBreaks(src), src);
});

test('normalize: preserves YAML frontmatter fences', () => {
    const src = '---\nname: demo\n---\n\nbody';
    // Blank line inserted before closing fence is still YAML-shaped.
    const out = normalizeMarkdownThematicBreaks(src);
    assert.match(out, /^---\nname: demo\n\n---\n/);
});

test('renderMarkdown: text\\n---\\ntext emits <hr>', () => {
    const html = renderMarkdown('section A\n---\nsection B');
    assert.match(html, /<hr\s*\/?>/i);
    assert.doesNotMatch(html, /<h2>section A<\/h2>/i);
    assert.match(html, /section A/);
    assert.match(html, /section B/);
});

test('renderMarkdown: blank-surrounded --- still emits <hr>', () => {
    const html = renderMarkdown('a\n\n---\n\nb');
    assert.match(html, /<hr\s*\/?>/i);
});

test('renderMarkdown: frontmatter still rendered as yaml block', () => {
    const html = renderMarkdown('---\ntitle: hello\n---\n\n# Body');
    assert.match(html, /md-yaml-frontmatter/);
    assert.match(html, /<h1>Body<\/h1>/);
});
