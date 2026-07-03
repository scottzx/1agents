// Lightweight syntax highlighting for the file preview code view.
// Uses highlight.js core with an explicit language allow-list to keep the
// bundle small. Output is split back into per-line HTML so it plugs into the
// existing line-number gutter (.fb-code-row / .fb-code-text) unchanged.

import hljs from 'highlight.js/lib/core';

import json from 'highlight.js/lib/languages/json';
import bash from 'highlight.js/lib/languages/bash';
import python from 'highlight.js/lib/languages/python';
import javascript from 'highlight.js/lib/languages/javascript';
import typescript from 'highlight.js/lib/languages/typescript';
import xml from 'highlight.js/lib/languages/xml';
import css from 'highlight.js/lib/languages/css';
import scss from 'highlight.js/lib/languages/scss';
import yaml from 'highlight.js/lib/languages/yaml';
import go from 'highlight.js/lib/languages/go';
import rust from 'highlight.js/lib/languages/rust';
import cpp from 'highlight.js/lib/languages/cpp';
import c from 'highlight.js/lib/languages/c';
import ini from 'highlight.js/lib/languages/ini';
import sql from 'highlight.js/lib/languages/sql';
import dockerfile from 'highlight.js/lib/languages/dockerfile';
import markdown from 'highlight.js/lib/languages/markdown';
import diff from 'highlight.js/lib/languages/diff';
import java from 'highlight.js/lib/languages/java';
import ruby from 'highlight.js/lib/languages/ruby';
import php from 'highlight.js/lib/languages/php';
import shell from 'highlight.js/lib/languages/shell';

hljs.registerLanguage('json', json);
hljs.registerLanguage('bash', bash);
hljs.registerLanguage('shell', shell);
hljs.registerLanguage('python', python);
hljs.registerLanguage('javascript', javascript);
hljs.registerLanguage('typescript', typescript);
hljs.registerLanguage('xml', xml);
hljs.registerLanguage('css', css);
hljs.registerLanguage('scss', scss);
hljs.registerLanguage('yaml', yaml);
hljs.registerLanguage('go', go);
hljs.registerLanguage('rust', rust);
hljs.registerLanguage('cpp', cpp);
hljs.registerLanguage('c', c);
hljs.registerLanguage('ini', ini);
hljs.registerLanguage('sql', sql);
hljs.registerLanguage('dockerfile', dockerfile);
hljs.registerLanguage('markdown', markdown);
hljs.registerLanguage('diff', diff);
hljs.registerLanguage('java', java);
hljs.registerLanguage('ruby', ruby);
hljs.registerLanguage('php', php);

/** Map a filename (by extension, then a few well-known basenames) to an hljs language. */
const EXT_LANG: Record<string, string> = {
    json: 'json',
    jsonc: 'json',
    sh: 'bash',
    bash: 'bash',
    zsh: 'bash',
    py: 'python',
    pyw: 'python',
    js: 'javascript',
    mjs: 'javascript',
    cjs: 'javascript',
    jsx: 'javascript',
    ts: 'typescript',
    tsx: 'typescript',
    html: 'xml',
    htm: 'xml',
    xml: 'xml',
    svg: 'xml',
    vue: 'xml',
    css: 'css',
    scss: 'scss',
    sass: 'scss',
    less: 'css',
    yaml: 'yaml',
    yml: 'yaml',
    go: 'go',
    rs: 'rust',
    cpp: 'cpp',
    cxx: 'cpp',
    cc: 'cpp',
    hpp: 'cpp',
    c: 'c',
    h: 'c',
    toml: 'ini',
    ini: 'ini',
    conf: 'ini',
    cfg: 'ini',
    sql: 'sql',
    md: 'markdown',
    markdown: 'markdown',
    diff: 'diff',
    patch: 'diff',
    java: 'java',
    rb: 'ruby',
    php: 'php',
};

const BASENAME_LANG: Record<string, string> = {
    dockerfile: 'dockerfile',
    makefile: 'bash',
    '.gitignore': 'bash',
    '.env': 'ini',
    '.bashrc': 'bash',
    '.zshrc': 'bash',
};

/** Resolve the hljs language id for a filename, or null when unsupported. */
export function detectCodeLang(filename: string): string | null {
    const base = filename.toLowerCase();
    if (BASENAME_LANG[base]) return BASENAME_LANG[base];
    const ext = base.includes('.') ? base.split('.').pop()! : '';
    return EXT_LANG[ext] ?? null;
}

/**
 * Highlight `code` for the given hljs language and return one HTML string per
 * source line, with any spans that straddle a newline (block comments,
 * multi-line strings) correctly closed at the line end and reopened on the next
 * line. The returned array length always equals `code.split('\n').length`.
 */
export function highlightToLines(code: string, lang: string): string[] {
    let html: string;
    try {
        html = hljs.highlight(code, { language: lang, ignoreIllegals: true }).value;
    } catch {
        return escapeToLines(code);
    }

    const lines: string[] = [];
    const openTags: string[] = [];
    let cur = '';
    let i = 0;
    while (i < html.length) {
        const ch = html[i];
        if (ch === '<') {
            const close = html.indexOf('>', i);
            if (close === -1) {
                cur += html.slice(i);
                break;
            }
            const tag = html.slice(i, close + 1);
            cur += tag;
            if (tag[1] === '/') openTags.pop();
            else openTags.push(tag);
            i = close + 1;
        } else if (ch === '\n') {
            cur += '</span>'.repeat(openTags.length);
            lines.push(cur);
            cur = openTags.join('');
            i++;
        } else {
            let j = i;
            while (j < html.length && html[j] !== '<' && html[j] !== '\n') j++;
            cur += html.slice(i, j);
            i = j;
        }
    }
    lines.push(cur);
    return lines;
}

/** Fallback: HTML-escape each line so the gutter view still renders safely. */
function escapeToLines(code: string): string[] {
    return code.split('\n').map(l => l.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;'));
}
