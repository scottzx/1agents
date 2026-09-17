import { h } from 'preact';
import { useState } from 'preact/hooks';
import type { ToolCallDiff, ToolCallLocation } from './types';

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

type DiffLine = { type: 'ctx' | 'add' | 'del'; text: string };

export function deriveDiffsFromInput(toolName: string, args: Record<string, unknown>): ToolCallDiff[] {
  const name = (toolName || '').toLowerCase();
  const path =
    typeof args.file_path === 'string' ? args.file_path : typeof args.path === 'string' ? args.path : undefined;
  if (!path) return [];

  const oldStr = args.old_string ?? args.oldText ?? args.old_str;
  const newStr = args.new_string ?? args.newText ?? args.new_str;
  if (typeof oldStr === 'string' && typeof newStr === 'string') {
    return [{ path, oldText: oldStr, newText: newStr }];
  }

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

  if ((name.includes('write') || name.includes('create')) && typeof args.content === 'string') {
    return [{ path, newText: args.content }];
  }
  return [];
}

function computeLineDiff(oldText: string, newText: string): DiffLine[] {
  const oldLines = oldText.length ? oldText.split('\n') : [];
  const newLines = newText.length ? newText.split('\n') : [];

  let start = 0;
  while (start < oldLines.length && start < newLines.length && oldLines[start] === newLines[start]) {
    start++;
  }
  let endOld = oldLines.length;
  let endNew = newLines.length;
  while (endOld > start && endNew > start && oldLines[endOld - 1] === newLines[endNew - 1]) {
    endOld--;
    endNew--;
  }

  const lines: DiffLine[] = [];
  const CTX = 2;
  for (let i = Math.max(0, start - CTX); i < start; i++) lines.push({ type: 'ctx', text: oldLines[i] });
  for (let i = start; i < endOld; i++) lines.push({ type: 'del', text: oldLines[i] });
  for (let i = start; i < endNew; i++) lines.push({ type: 'add', text: newLines[i] });
  for (let i = endOld; i < Math.min(oldLines.length, endOld + CTX); i++)
    lines.push({ type: 'ctx', text: oldLines[i] });
  return lines;
}

const DIFF_COLLAPSE_THRESHOLD = 40;
const DIFF_PREVIEW_LINES = 20;

function DiffBlock({ diff }: { diff: ToolCallDiff }) {
  const lines: DiffLine[] =
    diff.oldText === undefined
      ? diff.newText.split('\n').map(text => ({ type: 'add', text }))
      : computeLineDiff(diff.oldText, diff.newText);

  const isLarge = lines.length > DIFF_COLLAPSE_THRESHOLD;
  const [collapsed, setCollapsed] = useState(isLarge);
  const displayed = collapsed ? lines.slice(0, DIFF_PREVIEW_LINES) : lines;
  const hiddenCount = lines.length - DIFF_PREVIEW_LINES;

  return (
    <div class="chat-tool-diff">
      <div class="chat-tool-diff-path">{diff.path}</div>
      <pre class="chat-tool-diff-body">
        {displayed.map((line, i) => (
          <div key={i} class={`chat-tool-diff-line is-${line.type}`}>
            <span class="chat-tool-diff-gutter" aria-hidden="true">
              {line.type === 'add' ? '+' : line.type === 'del' ? '-' : ' '}
            </span>
            <span class="chat-tool-diff-text">{line.text || ' '}</span>
          </div>
        ))}
        {isLarge && collapsed && (
          <div class="chat-tool-diff-fold">
            <button
              type="button"
              class="chat-tool-diff-fold-btn"
              onClick={() => setCollapsed(false)}
            >
              展开剩余 {hiddenCount} 行…
            </button>
          </div>
        )}
      </pre>
      {isLarge && !collapsed && (
        <div class="chat-tool-diff-fold-footer">
          <button
            type="button"
            class="chat-tool-diff-fold-btn"
            onClick={() => setCollapsed(true)}
          >
            折叠
          </button>
        </div>
      )}
    </div>
  );
}

export function ToolDiffView({ diffs }: { diffs: ToolCallDiff[] }) {
  if (!diffs || !diffs.length) return null;
  return (
    <div class="chat-tool-diffs">
      {diffs.map((diff, i) => (
        <DiffBlock key={i} diff={diff} />
      ))}
    </div>
  );
}
