import { Marked, type RendererObject, type TokenizerAndRendererExtension } from 'marked';
import hljs from 'highlight.js/lib/common';
import { looksLikeFrontmatterYaml } from './frontmatter';

export interface MarkdownContext {
  projectName?: string;
  knownNumbers?: Set<number>;
  onOpenFile?: (path: string) => void;
}

let ctx: MarkdownContext = {};

const escapeHtml = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');

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
    if (ctx.knownNumbers && !ctx.knownNumbers.has(number)) {
      return escapeHtml(token.raw as string);
    }
    return refAnchor(ctx.projectName || '', number, `#${number}`);
  },
};

const fileRefAnchor = (path: string, line?: number, lineEnd?: number): string => {
  const display = line ? `${path}:${line}${lineEnd && lineEnd !== line ? `-${lineEnd}` : ''}` : path;
  const lineAttr = line ? ` data-line="${line}"` : '';
  const lineEndAttr = lineEnd ? ` data-line-end="${lineEnd}"` : '';
  return `<a class="file-ref" href="#" data-file-ref data-path="${escapeHtml(path)}"${lineAttr}${lineEndAttr}>${escapeHtml(display)}</a>`;
};

const FILE_PATH_RE = /^`([^\s`\n#]+\.[a-zA-Z][a-zA-Z0-9]{0,7})(?::(\d+)(?:-(\d+))?)?`/;
const BARE_PATH_RE =
  /^(~?\/(?!\/)(?:[\w.\-_]+\/)+[\w.\-_]+\.[a-zA-Z][a-zA-Z0-9]{0,7})(?::(\d+)(?:-(\d+))?)?(?=[\s,，。、!！?？：:；;"'"'()[\]<>【】]|$)/;

const fileRefExtension: TokenizerAndRendererExtension = {
  name: 'fileRef',
  level: 'inline',
  start(src: string) {
    const m = src.match(/[`/~]/);
    return m ? m.index : undefined;
  },
  tokenizer(src: string) {
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

function renderYamlBlock(src: string, frontmatter = false): string {
  let html = escapeHtml(src);
  try {
    html = hljs.highlight(src, { language: 'yaml', ignoreIllegals: true }).value;
  } catch {}
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

export const codeRenderer: RendererObject = {
  code(token) {
    const lang = (token.lang || '').trim().split(/\s+/)[0].toLowerCase();
    if (lang === 'mermaid') {
      const src = token.text;
      return (
        `<div class="mermaid-block" data-mermaid="${encodeURIComponent(src)}">` +
        `<pre class="mermaid-fallback"><code>${escapeHtml(src)}</code></pre></div>`
      );
    }
    if (lang === 'yaml' || lang === 'yml') {
      return renderYamlBlock(token.text);
    }

    let highlighted = '';
    if (lang && hljs.getLanguage(lang)) {
      try {
        highlighted = hljs.highlight(token.text, { language: lang, ignoreIllegals: true }).value;
      } catch {
        highlighted = escapeHtml(token.text);
      }
    } else {
      highlighted = escapeHtml(token.text);
    }

    return (
      `<div class="chat-code-block">` +
      `<div class="chat-code-header">` +
      `<span class="chat-code-lang">${escapeHtml(lang || 'text')}</span>` +
      `<button type="button" class="chat-code-copy-btn" data-copy="${encodeURIComponent(token.text)}">复制</button>` +
      `</div>` +
      `<pre><code class="hljs ${lang ? `language-${lang}` : ''}">${highlighted}</code></pre>` +
      `</div>`
    );
  },
};

const THEMATIC_BREAK_LINE_RE = /^ {0,3}(?:-{3,}|\*{3,}|_{3,})[ \t]*\r?$/;

export function normalizeMarkdownThematicBreaks(src: string): string {
  if (!src || (!src.includes('-') && !src.includes('*') && !src.includes('_'))) {
    return src;
  }

  const lines = src.split('\n');
  const out: string[] = [];
  let fenceChar: '`' | '~' | null = null;
  let fenceLen = 0;

  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];

    if (fenceChar) {
      const close = /^( {0,3})(`{3,}|~{3,})[ \t]*\r?$/.exec(line);
      if (close && close[2][0] === fenceChar && close[2].length >= fenceLen) {
        fenceChar = null;
        fenceLen = 0;
      }
      out.push(line);
      continue;
    }

    const open = /^( {0,3})(`{3,}|~{3,})(.*)$/.exec(line);
    if (open) {
      fenceChar = open[2][0] as '`' | '~';
      fenceLen = open[2].length;
      out.push(line);
      continue;
    }

    if (THEMATIC_BREAK_LINE_RE.test(line) && out.length > 0) {
      const prev = out[out.length - 1];
      if (prev.replace(/\r$/, '') !== '' && !THEMATIC_BREAK_LINE_RE.test(prev)) {
        out.push('');
      }
    }

    out.push(line);
  }

  return out.join('\n');
}

export const markedInstance = new Marked({ gfm: true, breaks: true });
markedInstance.use({
  extensions: [frontmatterExtension, taskRefExtension, fileRefExtension],
  renderer: codeRenderer,
});

export function renderMarkdown(content: string, c: MarkdownContext = {}): string {
  ctx = c;
  try {
    const src = normalizeMarkdownThematicBreaks(content);
    return markedInstance.parse(src, { async: false }) as string;
  } catch (err) {
    return `<pre class="md-parse-error">Markdown parse error: ${escapeHtml(String(err))}</pre>`;
  } finally {
    ctx = {};
  }
}
