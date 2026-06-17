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

import { Marked, type TokenizerAndRendererExtension } from 'marked';

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

const instance = new Marked({ gfm: true, breaks: true });
instance.use({ extensions: [taskRefExtension] });

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
