import { h } from 'preact';
import { useState } from 'preact/hooks';
import { DashboardData, DashboardProject, ProjectHealth } from '@1agents/core/services/dashboardService';
import { projectItemService } from '@1agents/core/services/taskService';
import { AGENT_TYPES, AGENT_TYPE_LABELS, type AgentType } from '../types';
import * as ui from '../../stores/uiStore';

// Company cockpit (公司驾驶舱) — Operational Dashboard.
//
// Layout priority: risk/blocked → recent activity → project grid.
// Status rows (op-status-row) surface actionable info without stat-card walls.

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
    idle: '待机',
};

function lastActiveText(iso?: string): string {
    if (!iso) return '—';
    const t = new Date(iso).getTime();
    if (!t) return '—';
    const mins = Math.floor((Date.now() - t) / 60000);
    if (mins < 1) return '刚刚';
    if (mins < 60) return `${mins} 分钟前`;
    const hrs = Math.floor(mins / 60);
    if (hrs < 24) return `${hrs} 小时前`;
    return `${Math.floor(hrs / 24)} 天前`;
}

// DispatchComposer: per-card 下指令/派工 form. Reuses projectItemService.create
// (immediate schedule) — no new backend semantics.
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

// StatusRow: compact process-block row used in "需关注" and "最近活动" sections.
function StatusRow({ p, onOpen, onRefresh }: { p: DashboardProject; onOpen: () => void; onRefresh: () => void }) {
    const [dispatching, setDispatching] = useState(false);
    return (
        <div class={`op-status-row${dispatching ? ' op-status-row--open' : ''}`}>
            <div class="op-status-row-main" onClick={!dispatching ? onOpen : undefined}>
                <span class={`op-status-dot op-status-dot--${p.health}`} />
                <span class="op-status-name">{p.name}</span>
                <span class="op-status-tags">
                    {p.blockedTasks > 0 && (
                        <span class="op-status-tag op-status-tag--danger">⛔ {p.blockedTasks} 阻塞</span>
                    )}
                    {p.runningTasks > 0 && p.health !== 'blocked' && (
                        <span class="op-status-tag">▶ {p.runningTasks} 进行中</span>
                    )}
                </span>
                <span class="op-status-time">{lastActiveText(p.lastEventAt)}</span>
                {!dispatching && (
                    <button
                        class="op-status-action"
                        title="派工"
                        onClick={e => {
                            e.stopPropagation();
                            setDispatching(true);
                        }}
                    >
                        派工
                    </button>
                )}
            </div>
            {dispatching && (
                <DispatchComposer project={p} onClose={() => setDispatching(false)} onDispatched={onRefresh} />
            )}
        </div>
    );
}

// ProjectCard: bento grid card for the "项目一览" section.
function ProjectCard({ p, onOpen, onRefresh }: { p: DashboardProject; onOpen: () => void; onRefresh: () => void }) {
    const [dispatching, setDispatching] = useState(false);
    // blocked / stalled cards get the salience treatment (pulse + halo);
    // stalled additionally dims to read as "stuck, no heartbeat".
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

export function DashboardCockpit({ data, companyName, onOpenProject, onRefresh }: CockpitProps) {
    const { summary, projects } = data;

    // Risk-first: blocked/stalled items need immediate attention.
    const attentionItems = projects.filter(p => p.health === 'blocked' || p.health === 'stalled');

    // Recent activity: top 6 projects sorted by lastEventAt desc.
    const recentItems = [...projects]
        .filter(p => p.lastEventAt)
        .sort((a, b) => new Date(b.lastEventAt!).getTime() - new Date(a.lastEventAt!).getTime())
        .slice(0, 6);

    return (
        <div class="cockpit-root">
            {/* Compact metrics bar — no stat-card wall */}
            <div class="op-metrics-bar">
                <span class="op-metrics-company">{companyName}</span>
                <div class="op-metrics-stats">
                    <span class="op-metric">
                        <span class={`op-metric-num${summary.runningProjects > 0 ? ' op-metric-num--success' : ''}`}>
                            {summary.runningProjects}
                        </span>{' '}
                        在跑
                    </span>
                    <span class="op-metric">
                        <span class={`op-metric-num${summary.blockedProjects > 0 ? ' op-metric-num--danger' : ''}`}>
                            {summary.blockedProjects}
                        </span>{' '}
                        阻塞
                    </span>
                    <span class="op-metric">
                        <span class="op-metric-num">{summary.activeAgents}</span> Agent
                    </span>
                    <span class="op-metric">
                        <span class="op-metric-num">{summary.deliveredTasks}</span> 今日交付
                    </span>
                </div>
                <button class="cockpit-refresh-btn" onClick={onRefresh} title="刷新大盘数据">
                    ↻ 刷新
                </button>
            </div>

            {/* Risk / blocked — priority section */}
            {attentionItems.length > 0 && (
                <section class="op-section">
                    <div class="op-section-header">
                        <span class="op-section-title op-section-title--danger">需关注</span>
                        <span class="op-section-badge op-section-badge--danger">{attentionItems.length}</span>
                    </div>
                    <div class="op-status-list">
                        {attentionItems.map(p => (
                            <StatusRow key={p.id} p={p} onOpen={() => onOpenProject(p.id)} onRefresh={onRefresh} />
                        ))}
                    </div>
                </section>
            )}

            {/* Recent activity — status-row process block */}
            {recentItems.length > 0 && (
                <section class="op-section">
                    <div class="op-section-header">
                        <span class="op-section-title">最近活动</span>
                    </div>
                    <div class="op-status-list">
                        {recentItems.map(p => (
                            <StatusRow key={p.id} p={p} onOpen={() => onOpenProject(p.id)} onRefresh={onRefresh} />
                        ))}
                    </div>
                </section>
            )}

            {/* Project grid overview */}
            <section class="op-section">
                <div class="op-section-header">
                    <span class="op-section-title">项目一览</span>
                    {projects.length > 0 && <span class="op-section-badge">{projects.length}</span>}
                </div>
                {projects.length === 0 ? (
                    <div class="cockpit-empty">还没有项目。去工作台创建一个项目，它就会出现在这里。</div>
                ) : (
                    <main class="cockpit-board bento-grid">
                        {projects.map(p => (
                            <ProjectCard key={p.id} p={p} onOpen={() => onOpenProject(p.id)} onRefresh={onRefresh} />
                        ))}
                    </main>
                )}
            </section>
        </div>
    );
}
