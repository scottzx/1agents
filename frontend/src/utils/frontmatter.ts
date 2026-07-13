// Task cards author their content as YAML-frontmatter Markdown: a leading `---`
// fenced block of machine-recognizable keys (acceptance, …) followed by the
// free-form body (background / process / expected result). Frontmatter is the
// single source of truth for those structured keys.
//
// We parse only the tiny subset we author (inline scalar, `- ` list, `|`/`>`
// block scalar), so no YAML dependency is needed — and the Go side
// (backend/internal/meta/frontmatter.go) mirrors this exactly.
//
// `---` wears two hats in Markdown: it's both the YAML frontmatter fence and a
// thematic break. To distinguish them we never trust "the next `---` is the
// closing fence" alone — the block in between must also look like a YAML
// mapping (top-level `key: value` shape). A README that opens with a decorative
// `--- / # Title / ---` triple is then treated as a thematic break + H1 +
// thematic break, not as frontmatter.

export interface ParsedCard {
    /** acceptance criteria as discrete lines (list items, sans the "- " marker). */
    acceptance: string[];
    /** the prose body with the frontmatter block stripped. */
    body: string;
}

/**
 * True iff `lines` (a slice of a doc, with no leading `---` fence) could be a
 * YAML frontmatter body. Empty array is a valid (empty) frontmatter. Each
 * non-empty line must either be a top-level `key: value` mapping or an
 * indented continuation (block scalar / list item). Lines that look like
 * Markdown (headers `# …`, bare prose, top-level bullets `- …` that don't fit
 * the key/value shape) defeat the candidate. The check is intentionally
 * conservative — we'd rather refuse a real but weird frontmatter than swallow
 * actual Markdown body.
 */
export function looksLikeFrontmatterYaml(lines: string[]): boolean {
    for (const raw of lines) {
        const line = raw.replace(/\r$/, '');
        if (line.trim() === '') continue; // blank line is fine inside YAML.
        if (/^[ \t]/.test(line)) continue; // indented continuation: always allowed.
        // Top-level mapping: a bare key followed by ":" (optionally with the
        // value inline). Reject Markdown-shaped top-level lines.
        if (!/^[A-Za-z_][\w-]*\s*:/.test(line)) return false;
    }
    return true;
}

/**
 * Split a document into its frontmatter (raw, without the `---` fences) and
 * the prose body. When no YAML-shaped `---…---` block opens the doc, returns
 * ("", doc) — the caller then sees the original content as body so rendering
 * can still treat the leading `---` as a thematic break.
 */
export function splitFrontmatter(doc: string): { fm: string; body: string } {
    const s = doc.replace(/^\uFEFF/, '');
    if (!s.startsWith('---\n') && !s.startsWith('---\r\n')) {
        return { fm: '', body: doc };
    }
    const nl = s.indexOf('\n');
    const rest = s.slice(nl + 1);
    const lines = rest.split('\n');

    // Locate the first `---` after the opening fence.
    let firstCloseIdx = -1;
    for (let i = 0; i < lines.length; i++) {
        if (lines[i].replace(/\r$/, '') === '---') {
            firstCloseIdx = i;
            break;
        }
    }
    if (firstCloseIdx < 0) {
        // No closing fence at all → preserve the original doc (legacy behavior;
        // genuine malformed input stays whole).
        return { fm: '', body: doc };
    }

    // Walk every `---` we find; accept the first whose leading block reads as
    // a YAML mapping. This lets us reject the `# Title` decoy used in many
    // README headers while still recognizing real frontmatter (possibly with
    // a thematic break earlier in the document — unusual but allowed).
    for (let i = firstCloseIdx; i < lines.length; i++) {
        if (lines[i].replace(/\r$/, '') !== '---') continue;
        const candidateLines = lines.slice(0, i);
        if (!looksLikeFrontmatterYaml(candidateLines)) continue;
        return {
            fm: candidateLines.join('\n'),
            body: lines
                .slice(i + 1)
                .join('\n')
                .replace(/^[\r\n]+/, ''),
        };
    }
    // A `---` was present but the block before it wasn't YAML-shaped — typical
    // decorative README header (`--- / # Title / ---`). Drop the opening fence
    // so both `---`s render as thematic breaks instead of leaking into frontmatter.
    return { fm: '', body: rest };
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
