import { h, Fragment } from 'preact';
import { useState } from 'preact/hooks';
import type {
  GroupedChatItem,
  GroupedToolCall,
  ToolGroupElement,
  TurnContentItem,
  TurnChangeReport,
} from './types';
import { renderMarkdown } from './markdown';
import { ToolDiffView, deriveDiffsFromInput, deriveLocationsFromInput } from './ToolDiffView';
import { ToolKindIcon, deriveToolKind } from './ToolKindIcon';
import { terminalCommandLine } from './terminalCommand';
import { formatToolOutput } from './formatToolOutput';

export { formatToolOutput };

interface MessageBubbleProps {
  item: GroupedChatItem;
  isLast?: boolean;
  active?: boolean;
  isLatestAssistant?: boolean;
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
  onToast?: (msg: string) => void;
  onCancelQueued?: (queueRequestId: string) => void;
}

function hasTurnChanges(report?: TurnChangeReport): boolean {
  return !!report && (report.addedCount > 0 || report.deletedCount > 0 || report.modifiedCount > 0);
}

export function TurnChangeCounts({ report, lang = 'zh-CN' }: { report?: TurnChangeReport; lang?: 'zh-CN' | 'en' }) {
  if (!hasTurnChanges(report) || !report) return null;
  const label =
    lang === 'zh-CN'
      ? `+${report.addedCount} ~${report.modifiedCount} -${report.deletedCount}`
      : `+${report.addedCount} ~${report.modifiedCount} -${report.deletedCount}`;
  return <span class="chat-turn-changes"> · {label}</span>;
}

export function TurnChangeFileList({
  report,
  onOpenFile,
  cwd,
}: {
  report?: TurnChangeReport;
  onOpenFile?: (path: string) => void;
  cwd?: string;
}) {
  if (!hasTurnChanges(report) || !report || !report.files || report.files.length === 0) return null;
  return (
    <div class="chat-turn-change-list">
      {report.files.map(file => {
        const op = file.op || 'modified';
        const displayPath = file.path.startsWith((cwd || '') + '/') ? file.path.slice((cwd?.length || 0) + 1) : file.path;
        return (
          <button
            key={`${file.op}:${file.path}`}
            type="button"
            class={`chat-turn-change-file is-${op}`}
            onClick={() => onOpenFile?.(file.path)}
            title={file.path}
          >
            <span class="chat-turn-change-op">{op === 'added' ? '+' : op === 'deleted' ? '-' : '~'}</span>
            <span class="chat-turn-change-path">{displayPath}</span>
          </button>
        );
      })}
    </div>
  );
}

function agenticGroupTitle(
  calls: GroupedToolCall[],
  thinkingBlocks: string[],
  pending: boolean,
  lang: 'zh-CN' | 'en' = 'zh-CN'
): string {
  if (pending) return lang === 'zh-CN' ? '准备中…' : 'Preparing…';

  const commandCount = calls.filter(c => (c.kind ?? deriveToolKind(c.toolName)) === 'execute').length;
  const readFileCount = calls.reduce((count, c) => {
    const kind = c.kind ?? deriveToolKind(c.toolName);
    if (kind !== 'read' && kind !== 'search') return count;
    if (c.locations && c.locations.length > 0) return count + c.locations.length;
    return count + 1;
  }, 0);

  if (thinkingBlocks.length > 0 && calls.length > 0) {
    if (lang === 'zh-CN') {
      const parts: string[] = [`思考 ${thinkingBlocks.length} 次`];
      if (readFileCount > 0) parts.push(`读取 ${readFileCount} 个文件`);
      if (commandCount > 0) parts.push(`运行 ${commandCount} 条命令`);
      parts.push(`使用 ${calls.length} 个工具`);
      return parts.join('，');
    }
    return `Thinking ${thinkingBlocks.length} times, read ${readFileCount} files, ran ${commandCount} commands, used ${calls.length} tools`;
  }
  if (calls.length > 0) {
    if (lang === 'zh-CN') {
      const parts: string[] = [];
      if (readFileCount > 0) parts.push(`读取 ${readFileCount} 个文件`);
      if (commandCount > 0) parts.push(`运行 ${commandCount} 条命令`);
      parts.push(`使用 ${calls.length} 个工具`);
      return parts.join('，');
    }
    return `Read ${readFileCount} files, ran ${commandCount} commands, used ${calls.length} tools`;
  }
  if (thinkingBlocks.length > 0) {
    return lang === 'zh-CN' ? `思考 ${thinkingBlocks.length} 次` : `Thought ${thinkingBlocks.length} times`;
  }
  return lang === 'zh-CN' ? '本轮过程' : 'Turn process';
}

function ToolCallRow({
  call,
  active,
  onOpenFile,
  cwd,
}: {
  call: GroupedToolCall;
  active?: boolean;
  onOpenFile?: (path: string) => void;
  cwd?: string;
}) {
  const [expanded, setExpanded] = useState(false);
  const kind = call.kind || deriveToolKind(call.toolName);

  let parsedArgs: Record<string, unknown> = {};
  if (call.input) {
    try {
      const parsed = JSON.parse(call.input);
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        parsedArgs = parsed as Record<string, unknown>;
      }
    } catch {}
  }

  const diffs = call.diffs ?? deriveDiffsFromInput(call.toolName, parsedArgs);
  const locations = call.locations ?? deriveLocationsFromInput(parsedArgs);
  const cmd = terminalCommandLine(parsedArgs);
  const hasOutput = call.output !== undefined;
  const isError = !!call.isError;
  const outputText = call.output ? formatToolOutput(call.output) : '';

  return (
    <div class={`chat-tool-row ${call.status || ''} ${isError ? 'is-error' : ''}`}>
      <div class="chat-tool-row-header" onClick={() => setExpanded(!expanded)}>
        <span class="chat-tool-caret">{expanded ? '▾' : '▸'}</span>
        <ToolKindIcon kind={kind} />
        <span class="chat-tool-badge">{call.toolName}</span>
        {cmd && <span class="chat-tool-cmd-preview">{cmd}</span>}
        <span class="chat-tool-status">
          {isError ? '失败' : hasOutput ? '已完成' : active ? '执行中…' : '已完成'}
        </span>
      </div>

      {expanded && (
        <div class="chat-tool-row-body">
          {locations.length > 0 && (
            <div class="chat-tool-locations">
              {locations.map((loc, i) => (
                <span
                  key={i}
                  class="chat-tool-loc-chip"
                  onClick={() => onOpenFile?.(loc.path)}
                  title={loc.path}
                >
                  📄 {loc.path}{loc.line ? `:${loc.line}` : ''}
                </span>
              ))}
            </div>
          )}

          {diffs.length > 0 && <ToolDiffView diffs={diffs} />}

          {call.input && !cmd && (
            <div class="chat-tool-field">
              <div class="chat-tool-field-label">输入:</div>
              <pre class="chat-tool-pre"><code>{call.input}</code></pre>
            </div>
          )}

          {cmd && (
            <div class="chat-tool-field">
              <div class="chat-tool-field-label">命令:</div>
              <pre class="chat-tool-pre"><code>{cmd}</code></pre>
            </div>
          )}

          {hasOutput && (
            <div class="chat-tool-field">
              <div class="chat-tool-field-label">输出:</div>
              <pre class="chat-tool-pre"><code>{outputText}</code></pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function GroupedThinkingItem({
  content,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
}: {
  content: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const preview = content.trim().replace(/\s+/g, ' ');
  const previewText = preview.length > 60 ? `${preview.slice(0, 60)}…` : preview;

  return (
    <div class={`chat-tool-row chat-tool-row-thinking ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <div class="chat-tool-row-header" onClick={() => setExpanded(!expanded)}>
        <span class="chat-tool-caret">{expanded ? '▾' : '▸'}</span>
        <span class="chat-tool-badge chat-tool-badge-thinking">
          {lang === 'zh-CN' ? '思考' : 'Think'}
        </span>
        <span class="chat-tool-cmd-preview is-thinking-preview" title={preview}>
          {previewText}
        </span>
        <span class="chat-tool-status">
          {expanded ? '' : lang === 'zh-CN' ? '已折叠' : 'Folded'}
        </span>
      </div>
      {expanded && (
        <div class="chat-tool-row-body is-thinking">
          <div
            class="chat-thinking-body markdown-body"
            dangerouslySetInnerHTML={{ __html: renderMarkdown(content, { projectName, onOpenFile }) }}
          />
        </div>
      )}
    </div>
  );
}

function ToolGroupBubble({
  calls,
  thinkingBlocks = [],
  elements = [],
  pending,
  active,
  cwd,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
}: {
  calls: GroupedToolCall[];
  thinkingBlocks?: string[];
  elements?: ToolGroupElement[];
  pending?: boolean;
  active?: boolean;
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const title = agenticGroupTitle(calls, thinkingBlocks, !!pending, lang);

  return (
    <div class={`chat-bubble chat-bubble-tool-group ${expanded ? 'is-expanded' : 'is-collapsed'} ${pending ? 'is-pending' : ''}`}>
      <button
        type="button"
        class="chat-tool-group-header"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        <span class="chat-bubble-caret" aria-hidden="true">
          {expanded ? '▾' : '▸'}
        </span>
        <span class="chat-tool-group-title">{title}</span>
        {!pending && !active && (
          <span class="chat-tool-group-processed">{lang === 'zh-CN' ? '已处理' : 'Processed'}</span>
        )}
      </button>
      {expanded && (
        <div class="chat-tool-calls-list">
          {elements.length > 0 ? (
            elements.map((el, idx) => {
              if (el.kind === 'thinking') {
                return (
                  <GroupedThinkingItem
                    key={el.id || idx}
                    content={el.content}
                    projectName={projectName}
                    lang={lang}
                    onOpenFile={onOpenFile}
                  />
                );
              }
              return <ToolCallRow key={el.call.id || idx} call={el.call} active={active} onOpenFile={onOpenFile} cwd={cwd} />;
            })
          ) : (
            calls.map((call, idx) => (
              <ToolCallRow key={call.id || idx} call={call} active={active} onOpenFile={onOpenFile} cwd={cwd} />
            ))
          )}
        </div>
      )}
    </div>
  );
}

function ThinkingBubble({
  content,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
}: {
  content: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const preview = content.trim().replace(/\s+/g, ' ');
  const previewText = preview.length > 80 ? `${preview.slice(0, 80)}…` : preview;

  return (
    <div class={`chat-bubble chat-bubble-thinking ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <button
        type="button"
        class="chat-bubble-header-clickable chat-tool-group-header"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        <span class="chat-bubble-caret" aria-hidden="true">
          {expanded ? '▾' : '▸'}
        </span>
        <span class="chat-tool-group-title">{lang === 'zh-CN' ? '思考过程' : 'Thinking Process'}</span>
        {!expanded && previewText && <span class="chat-thinking-preview">{previewText}</span>}
        <span class="chat-tool-group-processed">{lang === 'zh-CN' ? '已处理' : 'Processed'}</span>
      </button>
      {expanded && (
        <div
          class="chat-bubble-body chat-thinking-body markdown-body"
          dangerouslySetInnerHTML={{ __html: renderMarkdown(content, { projectName, onOpenFile }) }}
        />
      )}
    </div>
  );
}

function UserBubble({
  content,
  queueStatus,
  queueRequestId,
  projectName,
  onOpenFile,
  onCancelQueued,
}: {
  content: string;
  queueStatus?: 'queued';
  queueRequestId?: string;
  projectName?: string;
  onOpenFile?: (path: string) => void;
  onCancelQueued?: (queueRequestId: string) => void;
}) {
  return (
    <div class="chat-message-row chat-message-row-user">
      <div class={`chat-bubble chat-bubble-user ${queueStatus === 'queued' ? 'chat-bubble-user-queued' : ''}`}>
        <div class="chat-bubble-body">
          <div
            class="markdown-body md-conv"
            dangerouslySetInnerHTML={{
              __html: renderMarkdown(content, { projectName, onOpenFile }),
            }}
          />
        </div>
        {queueStatus === 'queued' && queueRequestId && (
          <button
            type="button"
            class="chat-bubble-queue-cancel"
            onClick={() => onCancelQueued?.(queueRequestId)}
            title="取消等待中提示"
          >
            ✕
          </button>
        )}
      </div>
    </div>
  );
}

function AssistantBubble({
  content,
  isLatestAssistant,
  changeReport,
  cwd,
  projectName,
  onOpenFile,
  onToast,
}: {
  content: string;
  streaming?: boolean;
  isLatestAssistant?: boolean;
  changeReport?: TurnChangeReport;
  cwd?: string;
  projectName?: string;
  onOpenFile?: (path: string) => void;
  onToast?: (msg: string) => void;
}) {
  const copyContent = () => {
    if (typeof navigator !== 'undefined' && navigator.clipboard) {
      navigator.clipboard.writeText(content).then(
        () => onToast?.('已复制内容'),
        () => onToast?.('复制失败')
      );
    }
  };

  return (
    <div class="chat-message-row chat-message-row-assistant">
      <div class="chat-bubble chat-bubble-assistant">
        <div class="chat-bubble-body">
          <div class="chat-assistant-block">
            <div class={`chat-assistant-content ${isLatestAssistant ? 'is-unlimited' : 'is-expanded'}`}>
              <div
                class="markdown-body md-conv"
                dangerouslySetInnerHTML={{
                  __html: renderMarkdown(content, { projectName, onOpenFile }),
                }}
              />
            </div>
            <div class="chat-assistant-actions">
              <button
                type="button"
                class="chat-assistant-action"
                onClick={copyContent}
                title="复制内容"
                aria-label="复制内容"
              >
                <svg viewBox="0 0 16 16" width="14" height="14" fill="none" stroke="currentColor" stroke-width="1.3">
                  <rect x="5.5" y="5.5" width="8" height="8" rx="1.5" />
                  <path d="M10.5 5.5V4A1.5 1.5 0 0 0 9 2.5H4A1.5 1.5 0 0 0 2.5 4v5A1.5 1.5 0 0 0 4 10.5h1.5" />
                </svg>
              </button>
            </div>
          </div>
          {changeReport && <TurnChangeFileList report={changeReport} onOpenFile={onOpenFile} cwd={cwd} />}
        </div>
      </div>
    </div>
  );
}

function HistoricalTurnBubble({
  items,
  outcomeId,
  turnStatus,
  changeReport,
  cwd,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
  onToast,
}: {
  items: TurnContentItem[];
  outcomeId?: string;
  turnStatus?: 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';
  changeReport?: TurnChangeReport;
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  onOpenFile?: (path: string) => void;
  onToast?: (msg: string) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  let thinkingCount = 0;
  let toolCount = 0;
  for (const item of items) {
    if (item.kind === 'thinking') {
      thinkingCount++;
    } else if (item.kind === 'tool_group') {
      thinkingCount += item.thinkingBlocks?.length ?? 0;
      toolCount += item.calls.length;
    }
  }

  const turnProcessTitle = lang === 'zh-CN' ? '本轮过程' : 'Turn process';
  const turnSummary =
    lang === 'zh-CN'
      ? `${thinkingCount} 次思考 · ${toolCount} 次工具调用`
      : `${thinkingCount} thoughts · ${toolCount} tool calls`;

  const outcome = outcomeId ? items.find(it => it.id === outcomeId) : undefined;
  const visibleItems = expanded ? items : outcome ? [outcome] : [];
  const assistantOwnsReport = items.some(it => it.kind === 'assistant_text' && hasTurnChanges(it.changeReport));

  return (
    <div class={`chat-turn ${expanded ? 'is-expanded' : 'is-collapsed'}`}>
      <button
        type="button"
        class="chat-tool-group-header chat-turn-header"
        aria-expanded={expanded}
        onClick={() => setExpanded(!expanded)}
      >
        <span class="chat-bubble-caret" aria-hidden="true">
          {expanded ? '▾' : '▸'}
        </span>
        <span class="chat-tool-group-title">{turnProcessTitle}</span>
        <span class="chat-turn-summary">
          {turnSummary}
          <TurnChangeCounts report={changeReport} lang={lang} />
        </span>
        <span class="chat-tool-group-processed">
          {turnStatus === 'failed'
            ? '失败'
            : turnStatus === 'cancelled'
              ? '已取消'
              : lang === 'zh-CN'
                ? '已处理'
                : 'Processed'}
        </span>
      </button>
      {visibleItems.length > 0 && (
        <div class="chat-turn-items">
          {visibleItems.map(it => (
            <MessageBubble
              key={it.id}
              item={it}
              cwd={cwd}
              projectName={projectName}
              lang={lang}
              onOpenFile={onOpenFile}
              onToast={onToast}
            />
          ))}
          {!assistantOwnsReport && changeReport && (
            <TurnChangeFileList report={changeReport} onOpenFile={onOpenFile} cwd={cwd} />
          )}
        </div>
      )}
    </div>
  );
}

export function MessageBubble({
  item,
  isLast,
  active,
  isLatestAssistant,
  cwd,
  projectName,
  lang = 'zh-CN',
  onOpenFile,
  onToast,
  onCancelQueued,
}: MessageBubbleProps) {
  // 1. User message
  if (item.kind === 'user') {
    return (
      <UserBubble
        content={item.content}
        queueStatus={item.queueStatus}
        queueRequestId={item.queueRequestId}
        projectName={projectName}
        onOpenFile={onOpenFile}
        onCancelQueued={onCancelQueued}
      />
    );
  }

  // 2. Assistant text
  if (item.kind === 'assistant_text') {
    return (
      <AssistantBubble
        content={item.content}
        streaming={item.streaming}
        isLatestAssistant={isLatestAssistant}
        changeReport={item.changeReport}
        cwd={cwd}
        projectName={projectName}
        onOpenFile={onOpenFile}
        onToast={onToast}
      />
    );
  }

  // 3. Thinking block
  if (item.kind === 'thinking') {
    return (
      <ThinkingBubble
        content={item.content}
        projectName={projectName}
        lang={lang}
        onOpenFile={onOpenFile}
      />
    );
  }

  // 4. Tool Group
  if (item.kind === 'tool_group') {
    return (
      <ToolGroupBubble
        calls={item.calls}
        thinkingBlocks={item.thinkingBlocks}
        elements={item.elements}
        pending={item.pending}
        active={active}
        cwd={cwd}
        projectName={projectName}
        lang={lang}
        onOpenFile={onOpenFile}
      />
    );
  }

  // 5. Folded Turn
  if (item.kind === 'turn') {
    return (
      <HistoricalTurnBubble
        items={item.items}
        outcomeId={item.outcomeId}
        turnStatus={item.turnStatus}
        changeReport={item.changeReport}
        cwd={cwd}
        projectName={projectName}
        lang={lang}
        onOpenFile={onOpenFile}
        onToast={onToast}
      />
    );
  }

  // 6. Turn changes
  if (item.kind === 'turn_changes') {
    return <TurnChangeFileList report={item.changeReport} onOpenFile={onOpenFile} cwd={cwd} />;
  }

  // 7. Error card
  if (item.kind === 'error') {
    return (
      <div class="chat-message-row">
        <div class="chat-bubble chat-bubble-error">
          <div class="chat-bubble-body">{item.content}</div>
        </div>
      </div>
    );
  }

  // 8. Turn receipt
  if (item.kind === 'turn_receipt') {
    return (
      <div class="chat-message-row">
        <div class="chat-turn-receipt">
          <span>{item.content || `Turn ${item.status}`}</span>
        </div>
      </div>
    );
  }

  return null;
}
