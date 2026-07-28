export interface MarkdownFileLink {
    path: string;
    line?: number;
    lineEnd?: number;
}

const FILE_LIKE_PATH_RE = /(?:^|\/)[^/]+\.[A-Za-z][A-Za-z0-9]{0,15}$/;
const WINDOWS_ABSOLUTE_PATH_RE = /^[A-Za-z]:[\\/]/;
const LINE_SUFFIX_RE = /:(\d+)(?:-(\d+))?$/;

/**
 * Parse a Markdown anchor href that points at a local file.
 *
 * Explicit Markdown links are not handled by the custom bare-file renderer,
 * so `[doc](/abs/path/doc.md:40)` arrives here as an ordinary href. Web URLs,
 * task routes, and extensionless application routes intentionally do not
 * match.
 */
export function parseMarkdownFileLink(href: string): MarkdownFileLink | null {
    const raw = href.trim();
    if (
        !raw ||
        raw.startsWith('#') ||
        raw.startsWith('//') ||
        /^[A-Za-z][A-Za-z0-9+.-]*:\/\//.test(raw) ||
        /^(?:mailto|tel|javascript):/i.test(raw)
    ) {
        return null;
    }

    const withoutFragment = raw.split('#', 1)[0].split('?', 1)[0];
    let decoded: string;
    try {
        decoded = decodeURIComponent(withoutFragment);
    } catch {
        decoded = withoutFragment;
    }

    const lineMatch = LINE_SUFFIX_RE.exec(decoded);
    const path = lineMatch ? decoded.slice(0, lineMatch.index) : decoded;
    const isSupportedPath =
        path.startsWith('/') ||
        path.startsWith('~/') ||
        path.startsWith('./') ||
        path.startsWith('../') ||
        WINDOWS_ABSOLUTE_PATH_RE.test(path) ||
        FILE_LIKE_PATH_RE.test(path);
    if (!isSupportedPath || !FILE_LIKE_PATH_RE.test(path.replace(/\\/g, '/'))) {
        return null;
    }

    const line = lineMatch ? Number.parseInt(lineMatch[1], 10) : undefined;
    const lineEnd = lineMatch?.[2] ? Number.parseInt(lineMatch[2], 10) : undefined;
    return {
        path,
        ...(line ? { line } : {}),
        ...(lineEnd ? { lineEnd } : {}),
    };
}
