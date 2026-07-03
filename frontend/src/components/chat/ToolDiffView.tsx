import { h } from 'preact';
import type { ToolCallDiff, ToolCallLocation } from '@1agents/core/protocol/types';

// Derive the file(s) a tool touched from its parsed input, so the locations
// chips survive the post-turn history reload (history carries the tool input,
// not the ACP `locations`). Covers the common file_path/path/notebook_path
// shapes plus MultiEdit's per-edit paths.
export function deriveLocationsFromInput(args: Record<string, unknown>): ToolCallLocation[] {
    const paths = new Set<string>();
    for (const key of ['file_path', 'path', 'notebook_path', 'filePath']) {
        if (typeof args[key] === 'string') paths.add(args[key] as string);
    }
    if (Array.isArray(args.edits)) {
        for (const e of args.edits) {
            const p = e && typeof e === 'object' ? (e as Record<string, unknown>).file_path : undefined;
            if (typeof p === 'string') paths.add(p);
        }
    }
    return [...paths].map(path => ({ path }));
}

// Inline file-diff renderer for tool cards (Phase 6). Diffs come from two
// sources, unified here:
//   1. ACP `content:[{type:"diff"}]` blocks forwarded by the bridge (live).
//   2. Derived from edit-family tool INPUT (Edit/Write/MultiEdit old→new),
//      so the diff survives the post-turn history reload — which carries the
//      tool input but not the ACP content blocks.
// Rendering is a lightweight prefix/suffix-trimmed line diff — good enough for
// typical edits without pulling in a diff library.

type DiffLine = { type: 'ctx' | 'add' | 'del'; text: string };

/** Derive diffs from an edit-family tool's parsed input, or [] if N/A. */
export function deriveDiffsFromInput(
    toolName: string,
    args: Record<string, unknown>
): ToolCallDiff[] {
    const name = toolName.toLowerCase();
    const path =
        typeof args.file_path === 'string'
            ? args.file_path
            : typeof args.path === 'string'
              ? args.path
              : undefined;
    if (!path) return [];

    // Edit / str_replace: single old→new.
    const oldStr = args.old_string ?? args.oldText ?? args.old_str;
    const newStr = args.new_string ?? args.newText ?? args.new_str;
    if (typeof oldStr === 'string' && typeof newStr === 'string') {
        return [{ path, oldText: oldStr, newText: newStr }];
    }
    // MultiEdit: edits[] of old→new — fold into one diff per edit.
    if (Array.isArray(args.edits)) {
        const diffs: ToolCallDiff[] = [];
        for (const e of args.edits) {
            if (e && typeof e === 'object') {
                const eo = (e as Record<string, unknown>).old_string;
                const en = (e as Record<string, unknown>).new_string;
                if (typeof eo === 'string' && typeof en === 'string') {
                    diffs.push({ path, oldText: eo, newText: en });
                }
            }
        }
        return diffs;
    }
    // Write / create_file: whole-file content is a new-file diff (no oldText).
    if ((name.includes('write') || name.includes('create')) && typeof args.content === 'string') {
        return [{ path, newText: args.content }];
    }
    return [];
}

function computeLineDiff(oldText: string, newText: string): DiffLine[] {
    const oldLines = oldText.length ? oldText.split('\n') : [];
    const newLines = newText.length ? newText.split('\n') : [];

    // Trim common leading lines.
    let start = 0;
    while (start < oldLines.length && start < newLines.length && oldLines[start] === newLines[start]) {
        start++;
    }
    // Trim common trailing lines (not overlapping the leading run).
    let endOld = oldLines.length;
    let endNew = newLines.length;
    while (endOld > start && endNew > start && oldLines[endOld - 1] === newLines[endNew - 1]) {
        endOld--;
        endNew--;
    }

    const lines: DiffLine[] = [];
    const CTX = 2;
    // A little leading context.
    for (let i = Math.max(0, start - CTX); i < start; i++) lines.push({ type: 'ctx', text: oldLines[i] });
    for (let i = start; i < endOld; i++) lines.push({ type: 'del', text: oldLines[i] });
    for (let i = start; i < endNew; i++) lines.push({ type: 'add', text: newLines[i] });
    // A little trailing context.
    for (let i = endOld; i < Math.min(oldLines.length, endOld + CTX); i++)
        lines.push({ type: 'ctx', text: oldLines[i] });
    return lines;
}

function DiffBlock({ diff }: { diff: ToolCallDiff }) {
    // A brand-new file (no oldText) shows every line as an addition.
    const lines: DiffLine[] =
        diff.oldText === undefined
            ? diff.newText.split('\n').map(text => ({ type: 'add', text }))
            : computeLineDiff(diff.oldText, diff.newText);

    return (
        <div class="chat-tool-diff">
            <div class="chat-tool-diff-path">{diff.path}</div>
            <pre class="chat-tool-diff-body">
                {lines.map((line, i) => (
                    <div key={i} class={`chat-tool-diff-line is-${line.type}`}>
                        <span class="chat-tool-diff-gutter" aria-hidden="true">
                            {line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' '}
                        </span>
                        <span class="chat-tool-diff-text">{line.text || ' '}</span>
                    </div>
                ))}
            </pre>
        </div>
    );
}

export function ToolDiffView({ diffs }: { diffs: ToolCallDiff[] }) {
    if (!diffs.length) return null;
    return (
        <div class="chat-tool-diffs">
            {diffs.map((diff, i) => (
                <DiffBlock key={i} diff={diff} />
            ))}
        </div>
    );
}
