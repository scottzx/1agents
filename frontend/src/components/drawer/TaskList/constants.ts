import { t, type Lang } from '../../../i18n';
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
    discussion: '讨论',
};

export const LINK_REL_LABELS: Record<string, string> = {
    closes: '修复 / 关闭',
    relates: '关联',
};

// Per-type acceptance-criteria templates (issue #135). A task without acceptance
// criteria is held as 未就绪 and never scheduled, so these starter checklists nudge
// the author to spell out "怎样算完成" up front, differentiated by work type.
export const TYPE_ACCEPTANCE_TEMPLATES: Record<string, string> = {
    task: '- [ ] 目标功能按描述实现\n- [ ] 相关测试通过\n- [ ] 无回归',
    requirement: '- [ ] 满足用户故事 / 需求描述\n- [ ] 关键场景可用\n- [ ] 验收人确认',
    bug: '- [ ] 复现步骤已不再触发该问题\n- [ ] 根因已修复（非掩盖）\n- [ ] 新增回归测试覆盖',
    discussion: '',
};

export const STATUS_LABELS: Record<string, string> = {
    pending: '等待中',
    queued: '排队中',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    blocked: '受阻',
    not_ready: '未就绪',
};

export const getPriorityLabels = (lang: Lang): Record<string, string> => ({
    urgent: t('task.priority.urgent', lang),
    high: t('task.priority.high', lang),
    medium: t('task.priority.medium', lang),
    low: t('task.priority.low', lang),
});

export const getStatusLabels = (lang: Lang): Record<string, string> => ({
    pending: t('task.status.pending', lang),
    queued: t('task.status.queued', lang),
    running: t('task.status.running', lang),
    completed: t('task.status.completed', lang),
    failed: t('task.status.failed', lang),
    cancelled: t('task.status.cancelled', lang),
    blocked: t('task.status.blocked', lang),
    not_ready: t('task.status.not_ready', lang),
});

export const getTypeLabels = (lang: Lang): Record<string, string> => ({
    task: t('task.type.task', lang),
    requirement: t('task.type.requirement', lang),
    bug: t('task.type.bug', lang),
    discussion: t('task.type.discussion', lang),
});

export const getLinkRelLabels = (lang: Lang): Record<string, string> => ({
    closes: t('task.link.closes', lang),
    relates: t('task.link.relates', lang),
});
