import type { DiffLine } from './types';

/** Parse unified diff text into numbered rows. Pure — safe to memoize by content. */
export function parseDiffLines(content: string): DiffLine[] {
    if (!content) return [];
    const lines = content.split('\n');
    let oldLine = 0;
    let newLine = 0;
    const result: DiffLine[] = [];

    for (let i = 0; i < lines.length; i++) {
        const line = lines[i];
        if (i === lines.length - 1 && line === '') continue;

        if (line.startsWith('@@ ')) {
            const match = line.match(/^@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
            if (match) {
                oldLine = parseInt(match[1], 10);
                newLine = parseInt(match[2], 10);
            }
            result.push({ oldLineNum: '', newLineNum: '', type: 'hunk', text: line });
        } else if (line.startsWith('+++ ') || line.startsWith('--- ')) {
            result.push({ oldLineNum: '', newLineNum: '', type: 'header', text: line });
        } else if (line.startsWith('+')) {
            result.push({ oldLineNum: '', newLineNum: newLine++, type: 'add', text: line });
        } else if (line.startsWith('-')) {
            result.push({ oldLineNum: oldLine++, newLineNum: '', type: 'del', text: line });
        } else if (line.startsWith(' ')) {
            result.push({ oldLineNum: oldLine++, newLineNum: newLine++, type: 'ctx', text: line });
        } else {
            result.push({ oldLineNum: '', newLineNum: '', type: 'header', text: line });
        }
    }
    return result;
}
