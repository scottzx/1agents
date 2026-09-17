export interface ParsedCard {
  acceptance: string[];
  body: string;
}

export function looksLikeFrontmatterYaml(lines: string[]): boolean {
  for (const raw of lines) {
    const line = raw.replace(/\r$/, '');
    if (line.trim() === '') continue;
    if (/^[ \t]/.test(line)) continue;
    if (!/^[A-Za-z_][\w-]*\s*:/.test(line)) return false;
  }
  return true;
}

export function splitFrontmatter(doc: string): { fm: string; body: string } {
  const s = doc.replace(/^\uFEFF/, '');
  if (!s.startsWith('---\n') && !s.startsWith('---\r\n')) {
    return { fm: '', body: doc };
  }
  const nl = s.indexOf('\n');
  const rest = s.slice(nl + 1);
  const lines = rest.split('\n');

  let firstCloseIdx = -1;
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].replace(/\r$/, '') === '---') {
      firstCloseIdx = i;
      break;
    }
  }
  if (firstCloseIdx < 0) {
    return { fm: '', body: doc };
  }

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
  return { fm: '', body: rest };
}
