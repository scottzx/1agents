import { h } from 'preact';
import { useMemo } from 'preact/hooks';
import type { ChatItem } from './types';
import { MessageList } from './MessageList';
import { turnsToChatItems, type SessionReaderTurn } from './adapter';

export interface ConversationViewProps {
  items?: ChatItem[];
  turns?: SessionReaderTurn[];
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  typing?: boolean;
  loading?: boolean;
  loadingHint?: string;
  emptyHint?: string;
  onOpenFile?: (path: string) => void;
  onToast?: (msg: string) => void;
  onCancelQueued?: (queueRequestId: string) => void;
}

export function ConversationView(props: ConversationViewProps) {
  const { items, turns, ...rest } = props;

  const resolvedItems = useMemo(() => {
    if (items && items.length > 0) return items;
    if (turns && turns.length > 0) return turnsToChatItems(turns);
    return [];
  }, [items, turns]);

  return <MessageList items={resolvedItems} {...rest} />;
}
