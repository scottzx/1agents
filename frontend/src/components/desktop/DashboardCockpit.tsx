import { h } from 'preact';
import { DashboardData, DashboardProject, ProjectHealth } from '@1agents/core/services/dashboardService';

// Company cockpit (公司驾驶舱) Phase 1 — real-data PMO overview.
//
// Renders the cross-project board on the backend's read-only aggregate. Its
// first job is 阻塞物理显著性: blocked / stalled projects float to the top
// (backend sorts them) and pulse / dim so they jump out without reading.

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

function ProjectCard({ p, onOpen }: { p: DashboardProject; onOpen: () => void }) {
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
                        <ProjectCard key={p.id} p={p} onOpen={() => onOpenProject(p.id)} />
                    ))}
                </main>
            )}
        </div>
    );
}
