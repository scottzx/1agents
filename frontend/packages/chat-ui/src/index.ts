import { h, render } from 'preact';
import { ConversationView, type ConversationViewProps } from './ConversationView';
import { MessageList, type MessageListProps, groupChatItems } from './MessageList';
import { MessageBubble } from './MessageBubble';
import { ToolDiffView } from './ToolDiffView';
import { ToolKindIcon, deriveToolKind } from './ToolKindIcon';
import { formatToolOutput } from './formatToolOutput';
import { terminalCommandLine } from './terminalCommand';
import { groupHistoricalTurns } from './turns';
import { renderMarkdown } from './markdown';
import { turnsToChatItems, type SessionReaderTurn } from './adapter';
import type {
  ChatItem,
  GroupedChatItem,
  GroupedToolCall,
  ToolCallInfo,
  TurnChangeReport,
  TurnChangeFile,
  MountConversationOptions,
} from './types';

declare const __CHAT_CSS__: string;

export * from './types';
export {
  ConversationView,
  MessageList,
  MessageBubble,
  ToolDiffView,
  ToolKindIcon,
  deriveToolKind,
  formatToolOutput,
  terminalCommandLine,
  groupChatItems,
  groupHistoricalTurns,
  renderMarkdown,
  turnsToChatItems,
};

export const CHAT_CSS = typeof __CHAT_CSS__ !== 'undefined' ? __CHAT_CSS__ : '';
export const CHAT_CSS_ID = '1agents-chat-ui-styles';

/**
 * Injects default or bundled chat styles into document.head.
 */
export function injectChatStyles(cssText?: string): HTMLStyleElement | null {
  if (typeof document === 'undefined') return null;
  let styleEl = document.getElementById(CHAT_CSS_ID) as HTMLStyleElement | null;
  const content = cssText || CHAT_CSS;
  if (!styleEl) {
    styleEl = document.createElement('style');
    styleEl.id = CHAT_CSS_ID;
    if (content) {
      styleEl.textContent = content;
    }
    document.head.appendChild(styleEl);
  } else if (content && !styleEl.textContent) {
    styleEl.textContent = content;
  }
  return styleEl;
}

/**
 * Mounts the 1Agents conversation view onto any standard DOM element.
 * Perfect for DSH web plugins, standalone modals, and webviews.
 */
export function mountConversation(
  container: HTMLElement,
  options: MountConversationOptions = {}
): { unmount: () => void; update: (newOptions: Partial<MountConversationOptions>) => void } {
  injectChatStyles();

  let currentOptions = { ...options };

  const handleCopyClick = (e: MouseEvent) => {
    const target = e.target as HTMLElement | null;
    if (!target) return;
    const copyBtn = target.closest('.chat-code-copy-btn') as HTMLElement | null;
    if (copyBtn) {
      const code = decodeURIComponent(copyBtn.getAttribute('data-copy') || '');
      if (code && typeof navigator !== 'undefined' && navigator.clipboard) {
        navigator.clipboard.writeText(code).then(() => {
          const orig = copyBtn.textContent;
          copyBtn.textContent = '已复制!';
          setTimeout(() => {
            copyBtn.textContent = orig || '复制';
          }, 1500);
          currentOptions.onToast?.('代码已复制到剪贴板');
        });
      }
    }
  };

  container.addEventListener('click', handleCopyClick);

  const renderComponent = () => {
    const vnode = h(ConversationView, {
      items: currentOptions.items,
      turns: currentOptions.turns,
      cwd: currentOptions.cwd,
      projectName: currentOptions.projectName,
      lang: currentOptions.lang || 'zh-CN',
      onOpenFile: currentOptions.onOpenFile,
      onToast: currentOptions.onToast,
      onCancelQueued: currentOptions.onCancelQueued,
    });
    render(vnode, container);
  };

  renderComponent();

  return {
    unmount: () => {
      container.removeEventListener('click', handleCopyClick);
      render(null, container);
    },
    update: (newOptions: Partial<MountConversationOptions>) => {
      currentOptions = { ...currentOptions, ...newOptions };
      renderComponent();
    },
  };
}
