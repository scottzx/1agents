/**
 * Core protocol & chat view types for @1agents/chat-ui.
 */

export interface ToolCallLocation {
  path: string;
  line?: number;
}

export interface ToolCallDiff {
  path: string;
  oldText?: string;
  newText: string;
}

export type ToolCallStatus = 'pending' | 'in_progress' | 'completed' | 'failed';

export interface AskUserOption {
  label: string;
  description: string;
  preview?: string | null;
}

export interface AskUserQuestionItem {
  question: string;
  options: AskUserOption[];
  multiSelect?: boolean | null;
}

export interface AskUserQuestionState {
  requestId: string;
  toolCallId?: string;
  mode?: string;
  questions: AskUserQuestionItem[];
  resolved?: string;
  answers?: Record<string, any>;
}

export type ExitPlanOutcome = 'approved' | 'rejected' | 'abandoned';

export interface ExitPlanModeState {
  requestId: string;
  toolCallId?: string;
  planContent: string;
  resolved?: ExitPlanOutcome;
  comments?: string;
}

export interface ToolCallInfo {
  id?: string;
  toolName: string;
  input: string;
  toolCallId?: string;
  output?: string;
  isError?: boolean;
  kind?: string;
  status?: ToolCallStatus;
  locations?: ToolCallLocation[];
  diffs?: ToolCallDiff[];
  permission?: {
    requestId: string;
    toolName: string;
    input: string;
    options: Array<{ text: string; data: string }>;
    resolved?: 'allow' | 'deny';
  };
  askUser?: AskUserQuestionState;
  exitPlan?: ExitPlanModeState;
}

export type TurnChangeOp = 'added' | 'modified' | 'deleted';

export interface TurnChangeFile {
  path: string;
  op: TurnChangeOp;
}

export interface TurnChangeReport {
  turnId?: string;
  recipeVersion?: number;
  addedCount: number;
  deletedCount: number;
  modifiedCount: number;
  files: TurnChangeFile[];
  source?: string;
  computedAt?: string;
}

export type ChatTurnStatus = 'queued' | 'running' | 'completed' | 'failed' | 'cancelled';

export type ChatItem =
  | {
      id: string;
      kind: 'user';
      content: string;
      createdAt: number;
      clientRequestId?: string;
      queueStatus?: 'queued';
      queueRequestId?: string;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
      changeReport?: TurnChangeReport;
    }
  | {
      id: string;
      kind: 'assistant_text';
      content: string;
      createdAt: number;
      streaming?: boolean;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
      changeReport?: TurnChangeReport;
    }
  | {
      id: string;
      kind: 'thinking';
      content: string;
      createdAt: number;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'tool_use';
      toolName: string;
      input: string;
      calls: ToolCallInfo[];
      createdAt: number;
      toolCallId?: string;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'tool_result';
      toolCallId?: string;
      toolName?: string;
      content: string;
      createdAt: number;
      isError: boolean;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'permission_request';
      toolCallId?: string;
      requestId: string;
      toolName: string;
      input: string;
      options: Array<{ text: string; data: string }>;
      createdAt: number;
      resolved?: 'allow' | 'deny';
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'ask_user_question';
      requestId: string;
      toolCallId?: string;
      mode?: string;
      questions: AskUserQuestionItem[];
      createdAt: number;
      resolved?: string;
      answers?: Record<string, any>;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'exit_plan_mode';
      requestId: string;
      toolCallId?: string;
      planContent: string;
      createdAt: number;
      resolved?: ExitPlanOutcome;
      comments?: string;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'error';
      content: string;
      createdAt: number;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | {
      id: string;
      kind: 'turn_receipt';
      content: string;
      count: number;
      status: 'succeeded' | 'rejected' | 'failed' | 'cancelled';
      createdAt: number;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    };

export interface GroupedToolCall {
  id: string;
  toolCallId?: string;
  toolName: string;
  input: string;
  output?: string;
  isError?: boolean;
  kind?: string;
  status?: 'pending' | 'in_progress' | 'completed' | 'failed';
  locations?: Array<{ path: string; line?: number }>;
  diffs?: Array<{ path: string; oldText?: string; newText: string }>;
  askUser?: AskUserQuestionState;
  exitPlan?: ExitPlanModeState;
  permission?: {
    requestId: string;
    toolName: string;
    input: string;
    options: Array<{ text: string; data: string }>;
    resolved?: 'allow' | 'deny';
  };
}

export type ToolGroupElement =
  | { kind: 'thinking'; id: string; content: string }
  | { kind: 'call'; call: GroupedToolCall };

export type TurnContentItem =
  | Extract<ChatItem, { kind: 'user' }>
  | Extract<ChatItem, { kind: 'assistant_text' }>
  | Extract<ChatItem, { kind: 'thinking' }>
  | {
      id: string;
      kind: 'tool_group';
      calls: GroupedToolCall[];
      thinkingBlocks?: string[];
      elements?: ToolGroupElement[];
      createdAt: number;
      pending?: boolean;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
    }
  | Extract<ChatItem, { kind: 'error' }>
  | Extract<ChatItem, { kind: 'turn_receipt' }>;

export type GroupedChatItem =
  | TurnContentItem
  | {
      id: string;
      kind: 'turn';
      items: TurnContentItem[];
      outcomeId?: string;
      createdAt: number;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
      changeReport?: TurnChangeReport;
    }
  | {
      id: string;
      kind: 'turn_changes';
      createdAt: number;
      turnId?: string;
      turnStatus?: ChatTurnStatus;
      changeReport?: TurnChangeReport;
    };

export interface MountConversationOptions {
  items?: ChatItem[];
  turns?: any[];
  cwd?: string;
  projectName?: string;
  lang?: 'zh-CN' | 'en';
  theme?: 'dark' | 'light' | 'auto';
  onOpenFile?: (path: string) => void;
  onToast?: (message: string) => void;
  onCancelQueued?: (queueRequestId: string) => void;
}
