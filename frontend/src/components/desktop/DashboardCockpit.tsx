import { h } from 'preact';
import { useState } from 'preact/hooks';
import { DashboardData, DashboardProject, ProjectHealth } from '@1agents/core/services/dashboardService';
import { projectItemService } from '@1agents/core/services/taskService';
import { AGENT_TYPES, AGENT_TYPE_LABELS, type AgentType } from '../types';
import * as ui from '../../stores/uiStore';

// Company cockpit (公司驾驶舱) Phase 2 — 看→控.
//
// Phase 1 rendered the read-only cross-project board. Phase 2 adds a control
// affordance to each card: dispatch an instruction / 派工 to a project's agent
// without leaving the big screen. It reuses the existing task-create path
// (POST /api/agent/project-items via projectItemService.create) — no new orchestration engine,
// no new backend semantics. A dispatched instruction is just an immediate task
// assigned to the chosen agent, which the scheduler picks up like any other.

interface CockpitProps {
    data: DashboardData;
    companyName: string;
    onOpenProject: (projectId: string) => void;
    onRefresh: () => void;
}

const HEALTH_LABEL: Record<ProjectHealth, string> = {
    blocked: '阻塞',
    stalled: '停滞',
    running: '进行中',
    done: '可发射',
    idle: '休息中',
};

function lastActiveText(iso?: string): string {
    if (!iso) return '暂无活动';
    const t = new Date(iso).getTime();
    if (!t) return '暂无活动';
    const mins = Math.floor((Date.now() - t) / 60000);
    if (mins < 1) return '刚刚活跃';
    if (mins < 60) return `${mins} 分钟前`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs} 小时前`;
    return `${Math.floor(hrs / 24)} 天前`;
}

// DispatchComposer is the per-card 下指令/派工 panel. It is a thin form over
// projectItemService.create: instruction → task title, agent → assignee, immediate
// schedule so the scheduler runs it right away.
function DispatchComposer({
    project,
    onClose,
    onDispatched,
}: {
    project: DashboardProject;
    onClose: () => void;
    onDispatched: () => void;
}) {
    const defaultAgent = (project.defaultAgent as AgentType) || 'claudecode';
    const [agent, setAgent] = useState<AgentType>(AGENT_TYPES.includes(defaultAgent) ? defaultAgent : 'claudecode');
    const [text, setText] = useState('');
    const [busy, setBusy] = useState(false);

    const submit = async () => {
        const title = text.trim();
        if (!title || busy) return;
        setBusy(true);
        try {
            await projectItemService.create({
                workspace_id: project.id,
                title,
                assignee: agent,
                scheduleType: 'immediate',
            });
            ui.showToast(`已向 [${project.name}] 派工：${AGENT_TYPE_LABELS[agent]} 即将执行`);
            onDispatched();
            onClose();
        } catch (err) {
            ui.showToast(`派工失败：${err instanceof Error ? err.message : String(err)}`);
            setBusy(false);
        }
    };

    return (
        <div class="cockpit-dispatch" onClick={e => e.stopPropagation()}>
            <textarea
                class="cockpit-dispatch-input"
                placeholder="给这个项目下一条指令 / 派工…"
                value={text}
                disabled={busy}
                autoFocus
                onInput={e => setText((e.target as HTMLTextAreaElement).value)}
                onKeyDown={e => {
                    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') submit();
                    if (e.key === 'Escape') onClose();
                }}
            />
            <div class="cockpit-dispatch-row">
                <select
                    class="cockpit-dispatch-agent"
                    value={agent}
                    disabled={busy}
                    onChange={e => setAgent((e.target as HTMLSelectElement).value as AgentType)}
                >
                    {AGENT_TYPES.map(t => (
                        <option key={t} value={t}>
                            {AGENT_TYPE_LABELS[t]}
                        </option>
                    ))}
                </select>
                <div class="cockpit-dispatch-actions">
                    <button class="cockpit-dispatch-cancel" disabled={busy} onClick={onClose}>
                        取消
                    </button>
                    <button class="cockpit-dispatch-send" disabled={busy || !text.trim()} onClick={submit}>
                        {busy ? '派工中…' : '派工 ⏎'}
                    </button>
                </div>
            </div>
        </div>
    );
}

function ProjectCard({ p, onOpen, onRefresh }: { p: DashboardProject; onOpen: () => void; onRefresh: () => void }) {
    const [dispatching, setDispatching] = useState(false);
    // blocked / stalled cards get the salience treatment (pulse + halo);
    // stalled additionally dims (降灰) to read as "stuck, no heartbeat".
    const salient = p.health === 'blocked' || p.health === 'stalled';
    const cls = [
        'cockpit-project',
        `is-${p.health}`,
        salient ? 'is-salient' : '',
        p.health === 'stalled' ? 'is-dimmed' : '',
    ]
        .filter(Boolean)
        .join(' ');

    return (
        <div class={cls} onClick={onOpen} title="点击下钻进入该项目工作台">
            <div class="bento-zone-header">
                <span class="cockpit-health-badge">{HEALTH_LABEL[p.health]}</span>
                <span class="cockpit-card-progress">{p.progressPercent}%</span>
            </div>

            <div class="bento-zone-body">
                <h3 class="bento-card-title cockpit-card-name">{p.name}</h3>
                <div class="cockpit-progress-track">
                    <div class="cockpit-progress-fill" style={`width:${p.progressPercent}%`} />
                </div>
                <div class="cockpit-card-meta">
                    <span title="已完成 / 任务总数">
                        ✅ {p.completedTasks}/{p.totalTasks} 任务
                    </span>
                    {p.blockedTasks > 0 && (
                        <span class="cockpit-meta-danger" title="阻塞 / 失败任务">
                            ⛔ {p.blockedTasks} 阻塞
                        </span>
                    )}
                </div>
            </div>

            <div class="bento-zone-footer cockpit-card-footer">
                <span title="在岗 / 在册会话">
                    👤 {p.activeSessions}/{p.agentSessions} 在岗
                </span>
                <span class="cockpit-card-last">{lastActiveText(p.lastEventAt)}</span>
            </div>

            {dispatching ? (
                <DispatchComposer project={p} onClose={() => setDispatching(false)} onDispatched={onRefresh} />
            ) : (
                <button
                    class="cockpit-dispatch-btn"
                    title="给该项目下指令 / 派工"
                    onClick={e => {
                        e.stopPropagation();
                        setDispatching(true);
                    }}
                >
                    ⚡ 派工
                </button>
            )}
        </div>
    );
}

function HudStat({
    icon,
    label,
    value,
    tone,
    pulse,
}: {
    icon: string;
    label: string;
    value: number;
    tone?: 'danger' | 'success' | 'accent';
    pulse?: boolean;
}) {
    const cls = ['cockpit-hud-stat', tone ? `tone-${tone}` : '', pulse ? 'is-pulse' : ''].filter(Boolean).join(' ');
    return (
        <div class={cls}>
            <span class="cockpit-hud-icon">{icon}</span>
            <span class="cockpit-hud-value">{value}</span>
            <span class="cockpit-hud-label">{label}</span>
        </div>
    );
}

export function DashboardCockpit({ data, companyName, onOpenProject, onRefresh }: CockpitProps) {
    const { summary, projects } = data;

    return (
        <div class="cockpit-root">
            <header class="cockpit-hud">
                <div class="cockpit-hud-title">
                    <span class="cockpit-company">🏢 {companyName}</span>
                    <span class="cockpit-subtitle">公司驾驶舱 · 项目大盘</span>
                </div>
                <div class="cockpit-hud-stats">
                    <HudStat icon="🟢" label="在跑" value={summary.runningProjects} tone="success" />
                    <HudStat
                        icon="⛔"
                        label="阻塞"
                        value={summary.blockedProjects}
                        tone="danger"
                        pulse={summary.blockedProjects > 0}
                    />
                    <HudStat icon="👤" label="在岗 Agent" value={summary.activeAgents} tone="accent" />
                    <HudStat icon="🚀" label="可发射" value={summary.doneProjects} />
                    <HudStat icon="✅" label="今日交付" value={summary.deliveredTasks} />
                </div>
                <button class="cockpit-refresh-btn" onClick={onRefresh} title="刷新大盘数据">
                    ↻ 刷新
                </button>
            </header>

            {projects.length === 0 ? (
                <div class="cockpit-empty">还没有项目。去工作台创建一个项目，它就会出现在这里。</div>
            ) : (
                <main class="cockpit-board bento-grid">
                    {projects.map(p => (
                        <ProjectCard key={p.id} p={p} onOpen={() => onOpenProject(p.id)} onRefresh={onRefresh} />
                    ))}
                </main>
            )}
        </div>
    );
}
