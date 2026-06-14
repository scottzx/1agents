import type { TaskPriority } from './types';

export const PRIORITY_LABELS: Record<string, string> = {
    urgent: '紧急',
    high: '高',
    medium: '中',
    low: '低',
};

export const PRIORITY_RANK: Record<TaskPriority, number> = {
    urgent: 0,
    high: 1,
    medium: 2,
    low: 3,
};

export const AGENT_OPTIONS = ['claudecode', 'codex', 'gemini', 'cursor', 'opencode', 'kimi', 'iflow', 'qoder'];

export const TYPE_LABELS: Record<string, string> = {
    task: '任务',
    requirement: '需求',
    bug: '缺陷',
};

export const STATUS_LABELS: Record<string, string> = {
    pending: '等待中',
    queued: '排队中',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    blocked: '受阻',
};
