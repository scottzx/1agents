// Shared Markdown renderer for task content (descriptions, acceptance
// criteria, timeline replies, chat bubbles).
//
// On top of GFM it adds GitHub-style task references that render as clickable
// permalinks pointing at /{project}/tasks/{number}:
//
//   #90            → same-project reference (the active project)
//   `项目名#90`     → cross-project reference (backtick-delimited so the project
//                     name's left boundary is unambiguous; split on the LAST #)
//
// The reference token only matches `#` followed by digits — `#bug`, `#todo`
// and the like are never touched. Same-project numbers are validated against
// the caller-supplied set of known numbers (link only if the task exists, just
// like GitHub); cross-project references are linked optimistically and fall
// back to a friendly not-found when followed. A plain `` `#2` `` (no project
// name) stays an ordinary code span, which doubles as the escape hatch.

import { Marked, type RendererObject, type TokenizerAndRendererExtension } from 'marked';
import hljs from 'highlight.js/lib/core';
import yaml from 'highlight.js/lib/languages/yaml';
import { looksLikeFrontmatterYaml } from './frontmatter';

hljs.registerLanguage('yaml', yaml);

export interface MarkdownContext {
    /** Active project (display name) used to build same-project `#N` links. */
    projectName?: string;
    /** Existing task numbers in the active project; when provided, a bare `#N`
     *  links only if N is present (GitHub-style existence check). */
    knownNumbers?: Set<number>;
}

// Set synchronously right before each parse() and read by the renderer. Safe
// because parse() is synchronous and JS is single-threaded — no interleaving.
let ctx: MarkdownContext = {};

const escapeHtml = (s: string): string =>
    s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

/** Build the permalink anchor markup for a resolved (project, number) ref. */
const refAnchor = (project: string, number: number, display: string): string => {
    const href = project ? `/${encodeURIComponent(project)}/tasks/${number}` : `/tasks/${number}`;
    return (
        `<a class="task-ref" href="${escapeHtml(href)}" data-task-ref` +
        ` data-project="${escapeHtml(project)}" data-number="${number}">${escapeHtml(display)}</a>`
    );
};

const taskRefExtension: TokenizerAndRendererExtension = {
    name: 'taskRef',
    level: 'inline',
    start(src: string) {
        const m = src.match(/[`#]/);
        return m ? m.index : undefined;
    },
    tokenizer(src: string) {
        // Cross-project: `<name>#<digits>` — split on the LAST '#' so names may
        // contain anything except a backtick/newline.
        let m = /^`([^`\n]+?)#(\d+)`/.exec(src);
        if (m) {
            return {
                type: 'taskRef',
                raw: m[0],
                project: m[1].trim(),
                number: parseInt(m[2], 10),
                crossProject: true,
            };
        }
        // Same-project: bare #<digits>.
        m = /^#(\d+)\b/.exec(src);
        if (m) {
            return {
                type: 'taskRef',
                raw: m[0],
                project: '',
                number: parseInt(m[1], 10),
                crossProject: false,
            };
        }
        return undefined;
    },
    renderer(token) {
        const number = token.number as number;
        if (token.crossProject) {
            const project = token.project as string;
            return refAnchor(project, number, `${project}#${number}`);
        }
        // Same-project: honor the existence check when we know the numbers.
        if (ctx.knownNumbers && !ctx.knownNumbers.has(number)) {
            return escapeHtml(token.raw as string);
        }
        return refAnchor(ctx.projectName || '', number, `#${number}`);
    },
};

/** Build the file-ref anchor markup for a resolved path with optional line range. */
const fileRefAnchor = (path: string, line?: number, lineEnd?: number): string => {
    const display = line ? `${path}:${line}${lineEnd && lineEnd !== line ? `-${lineEnd}` : ''}` : path;
    const lineAttr = line ? ` data-line="${line}"` : '';
    const lineEndAttr = lineEnd ? ` data-line-end="${lineEnd}"` : '';
    return `<a class="file-ref" href="#" data-file-ref data-path="${escapeHtml(path)}"${lineAttr}${lineEndAttr}>${escapeHtml(display)}</a>`;
};

// Backtick-wrapped path: `path/to/file.ext` or `path/to/file.ext:line-lineEnd`
// No spaces, no '#' (avoids conflict with task refs like `project#90`).
const FILE_PATH_RE = /^`([^\s`\n#]+\.[a-zA-Z][a-zA-Z0-9]{0,7})(?::(\d+)(?:-(\d+))?)?`/;

// Bare absolute path without backticks: /path/to/file.ext or ~/path/to/file.ext
// Requires ≥2 path segments (so single-segment `/api` won't match).
// The (?!\/) guard prevents matching // (URL protocol-relative references).
// The trailing lookahead anchors the path at whitespace or common punctuation.
const BARE_PATH_RE =
    /^(~?\/(?!\/)(?:[\w.\-_]+\/)+[\w.\-_]+\.[a-zA-Z][a-zA-Z0-9]{0,7})(?::(\d+)(?:-(\d+))?)?(?=[\s,，。、!！?？：:；;"'"'()[\]<>【】]|$)/;

const fileRefExtension: TokenizerAndRendererExtension = {
    name: 'fileRef',
    level: 'inline',
    start(src: string) {
        // Backtick starts a potential backtick-wrapped ref; / or ~ start a bare path.
        const m = src.match(/[`/~]/);
        return m ? m.index : undefined;
    },
    tokenizer(src: string) {
        // 1. Backtick-wrapped: `path/to/file.ext:line`
        let m = FILE_PATH_RE.exec(src);
        if (m) {
            return {
                type: 'fileRef',
                raw: m[0],
                path: m[1],
                line: m[2] ? parseInt(m[2], 10) : undefined,
                lineEnd: m[3] ? parseInt(m[3], 10) : undefined,
            };
        }
        // 2. Bare absolute path: /Users/scott/file.ts or ~/project/file.go
        m = BARE_PATH_RE.exec(src);
        if (m) {
            return {
                type: 'fileRef',
                raw: m[0],
                path: m[1],
                line: m[2] ? parseInt(m[2], 10) : undefined,
                lineEnd: m[3] ? parseInt(m[3], 10) : undefined,
            };
        }
        return undefined;
    },
    renderer(token) {
        return fileRefAnchor(
            token.path as string,
            token.line as number | undefined,
            token.lineEnd as number | undefined
        );
    },
};

const yamlLangs = new Set(['yaml', 'yml']);

function renderYamlBlock(src: string, frontmatter = false): string {
    let html = escapeHtml(src);
    try {
        html = hljs.highlight(src, { language: 'yaml', ignoreIllegals: true }).value;
    } catch {
        // Keep the escaped fallback above.
    }
    return (
        `<div class="md-yaml-block${frontmatter ? ' md-yaml-frontmatter' : ''}">` +
        `<div class="md-yaml-label">${frontmatter ? 'frontmatter' : 'yaml'}</div>` +
        `<pre><code class="hljs language-yaml">${html}</code></pre></div>`
    );
}

export const frontmatterExtension: TokenizerAndRendererExtension = {
    name: 'frontmatter',
    level: 'block',
    start(src: string) {
        return src.startsWith('---\n') || src.startsWith('---\r\n') ? 0 : undefined;
    },
    tokenizer(src: string) {
        // Match `--- / <text> / ---` only when the inner text looks like a YAML
        // mapping. Otherwise the `---`s are thematic breaks (very common in
        // README headers: `---\n# Title\n---\n`) and the rest of the parse must
        // see them as such.
        const openMatch = /^---(?:\r?\n|$)/.exec(src);
        if (!openMatch || openMatch[0].length === src.length) return undefined;
        const after = src.slice(openMatch[0].length);
        if (!after.length) return undefined;
        const lines = after.split('\n');
        for (let i = 0; i < lines.length; i++) {
            if (lines[i].replace(/\r$/, '') !== '---') continue;
            const candidate = lines.slice(0, i);
            if (!looksLikeFrontmatterYaml(candidate)) continue;
            const innerLines = candidate.join('\n');
            const raw = openMatch[0] + lines.slice(0, i + 1).join('\n');
            return { type: 'frontmatter', raw, text: innerLines };
        }
        return undefined;
    },
    renderer(token) {
        return renderYamlBlock(token.text as string, true);
    },
};

// A ```mermaid fenced block is emitted as an inert placeholder carrying the
// diagram source (URI-encoded so newlines survive the attribute). The raw code
// is kept inside as a <pre> fallback — that's what shows until a consumer with
// a live DOM (the chat bubble) lazy-loads mermaid and swaps in the SVG, and
// what stays put in contexts that never run that step (task descriptions etc.).
export const mermaidRenderer: RendererObject = {
    code(token) {
        const lang = (token.lang || '').trim().split(/\s+/)[0].toLowerCase();
        if (lang === 'mermaid') {
            const src = token.text;
            return (
                `<div class="mermaid-block" data-mermaid="${encodeURIComponent(src)}">` +
                `<pre class="mermaid-fallback"><code>${escapeHtml(src)}</code></pre></div>`
            );
        }
        if (yamlLangs.has(lang)) return renderYamlBlock(token.text);
        // Returning false defers to marked's default code renderer.
        return false;
    },
};

const instance = new Marked({ gfm: true, breaks: true });
// taskRef must be registered first so `project#N` is captured before fileRef
// can see the backtick.
instance.use({ extensions: [frontmatterExtension, taskRefExtension, fileRefExtension], renderer: mermaidRenderer });

/**
 * Render Markdown to an HTML string, autolinking task references using `c`.
 * Pass the active project name (and its known task numbers when available) so
 * bare `#N` references resolve and validate correctly.
 */
export function renderMarkdown(content: string, c: MarkdownContext = {}): string {
    ctx = c;
    try {
        return instance.parse(content, { async: false }) as string;
    } catch (err) {
        return `<pre class="md-parse-error">Markdown parse error: ${escapeHtml(String(err))}</pre>`;
    } finally {
        ctx = {};
    }
}

/** Pattern for a task-permalink pathname: /{project}/tasks/{number}. */
const PERMALINK_RE = /^\/([^/]+)\/tasks\/(\d+)\/?$/;

/**
 * Parse a permalink pathname into its (project, number) parts, or null when it
 * isn't a task permalink. Shared by the deep-link bootstrap and the in-app
 * click interceptor so both agree on the URL shape.
 */
export function parseTaskPermalink(pathname: string): { project: string; number: number } | null {
    const m = PERMALINK_RE.exec(pathname);
    if (!m) return null;
    return { project: decodeURIComponent(m[1]), number: parseInt(m[2], 10) };
}
