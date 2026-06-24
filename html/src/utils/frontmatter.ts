// Task cards author their content as YAML-frontmatter Markdown: a leading `---`
// fenced block of machine-recognizable keys (acceptance, …) followed by the
// free-form body (background / process / expected result). Frontmatter is the
// single source of truth for those structured keys.
//
// We parse only the tiny subset we author (inline scalar, `- ` list, `|`/`>`
// block scalar), so no YAML dependency is needed — and the Go side
// (backend/internal/meta/frontmatter.go) mirrors this exactly.

export interface ParsedCard {
    /** acceptance criteria as discrete lines (list items, sans the "- " marker). */
    acceptance: string[];
    /** the prose body with the frontmatter block stripped. */
    body: string;
}

function splitFrontmatter(doc: string): { fm: string; body: string } {
    const s = doc.replace(/^\uFEFF/, '');
    if (!s.startsWith('---\n') && !s.startsWith('---\r\n')) {
        return { fm: '', body: doc };
    }
    const nl = s.indexOf('\n');
    const lines = s.slice(nl + 1).split('\n');
    for (let i = 0; i < lines.length; i++) {
        if (lines[i].replace(/\r$/, '') === '---') {
            return {
                fm: lines.slice(0, i).join('\n'),
                body: lines
                    .slice(i + 1)
                    .join('\n')
                    .replace(/^[\r\n]+/, ''),
            };
        }
    }
    return { fm: '', body: doc }; // no closing fence — malformed, treat as body
}

function parseAcceptance(fm: string): string[] {
    if (!fm) return [];
    const lines = fm.split('\n');
    for (let i = 0; i < lines.length; i++) {
        const line = lines[i].replace(/\r$/, '');
        if (/^[ \t-]/.test(line) || !line.startsWith('acceptance:')) continue;
        const rest = line.slice('acceptance:'.length).trim();
        // Inline scalar.
        if (rest && !['|', '>', '|-', '>-'].includes(rest)) {
            return [rest.replace(/^["']|["']$/g, '')];
        }
        // List / block scalar: collect indented lines.
        const items: string[] = [];
        const block: string[] = [];
        for (let j = i + 1; j < lines.length; j++) {
            const ln = lines[j].replace(/\r$/, '');
            if (ln.trim() === '') {
                if (rest === '') break;
                continue;
            }
            if (!/^[ \t]/.test(ln)) break;
            const trimmed = ln.trim();
            if (trimmed.startsWith('- ')) {
                items.push(
                    trimmed
                        .slice(2)
                        .trim()
                        .replace(/^["']|["']$/g, '')
                );
            } else {
                block.push(trimmed);
            }
        }
        if (items.length) return items;
        if (block.length) return block;
        return [];
    }
    return [];
}

export function parseFrontmatter(doc: string | undefined): ParsedCard {
    const { fm, body } = splitFrontmatter(doc || '');
    return { acceptance: parseAcceptance(fm), body };
}
