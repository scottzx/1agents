import { h, Component } from 'preact';
import { Workspace } from '../types';
import { Task } from '../drawer/TaskList/types';
import { workspaceService } from '../../services/workspaceService';
import * as wsStore from '../../stores/workspaceStore';
import * as stage from '../../stores/stageStore';
import * as ui from '../../stores/uiStore';
import { DashboardWorkshop } from './DashboardWorkshop';

interface TooltipData {
    workspace: Workspace;
    tasks: Task[];
    status: string;
    progressPercent: number;
    dept: string;
    displayName: string;
    activeAgent: string;
}

interface DashboardAppState {
    workspaces: Workspace[];
    tasksMap: Record<string, Task[]>;
    loading: boolean;
    useMock: boolean;
    companyName: string;
    isEditingCompany: boolean;
    dayCount: number;
    funds: number;
    reputation: number;

    // Tooltip HUD state
    tooltipVisible: boolean;
    tooltipX: number;
    tooltipY: number;
    tooltipData: unknown;

    // Rocket launch state
    launchingProjectId: string | null;
}

// ── MOCK DATA FOR SIMULATION DEMO ──
const MOCK_WORKSPACES: Workspace[] = [
    {
        id: 'mock-ws-01',
        name: '[核心软件] 玄武 AI 调度台',
        path: '/mock/sw-assist',
        status: 'active',
        defaultAgent: 'claudecode',
    },
    {
        id: 'mock-ws-02',
        name: '[硬件制造] 机械装配机器臂',
        path: '/mock/hw-arm',
        status: 'active',
        defaultAgent: 'gemini',
    },
    {
        id: 'mock-ws-03',
        name: '[自媒体部] 小红书爆款文案生成',
        path: '/mock/media-xhs',
        status: 'active',
        defaultAgent: 'kimi',
    },
    {
        id: 'mock-ws-04',
        name: '[核心软件] 深度神经网络加速引擎',
        path: '/mock/neural-net',
        status: 'active',
        defaultAgent: 'codex',
    },
    {
        id: 'mock-ws-05',
        name: '[控制算法] 无人机姿态解算程序',
        path: '/mock/uav-control',
        status: 'active',
        defaultAgent: 'cursor',
    },
    {
        id: 'mock-ws-06',
        name: '[前沿探索] 量子纠缠状态遥测',
        path: '/mock/quantum',
        status: 'active',
        defaultAgent: 'gemini',
    },
    {
        id: 'mock-ws-07',
        name: '[自媒体部] 自动剪辑短视频流',
        path: '/mock/media-video',
        status: 'active',
        defaultAgent: 'pi',
    },
    {
        id: 'mock-ws-08',
        name: '[硬件制造] 空气检测传感器 IoT',
        path: '/mock/iot-sensor',
        status: 'active',
        defaultAgent: 'claudecode',
    },
];

const MOCK_TASKS: Record<string, Task[]> = {
    'mock-ws-01': [
        // Completed project
        {
            id: 't1',
            title: '主界面重构',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '性能优化',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-02': [
        // Running
        {
            id: 't1',
            title: '关节电机校准',
            status: 'running',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '防碰撞测试',
            status: 'pending',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-03': [
        // Blocked / Warning
        {
            id: 't1',
            title: '小红书API授权过期',
            status: 'blocked',
            type: 'bug',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '配图生成模块',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-04': [
        // Running
        {
            id: 't1',
            title: '计算核心重写',
            status: 'running',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '并发锁测试',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-05': [
        // Failed / Error
        {
            id: 't1',
            title: '卡尔曼滤波数学公式错误',
            status: 'failed',
            type: 'bug',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '陀螺仪驱动加载',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-06': [
        // Pending / Idle
        {
            id: 't1',
            title: '论文大纲草稿',
            status: 'pending',
            type: 'requirement',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-07': [
        // Running
        {
            id: 't1',
            title: '视频画面剪切算法',
            status: 'running',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
        {
            id: 't2',
            title: '背景音乐匹配',
            status: 'pending',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
    'mock-ws-08': [
        // Completed
        {
            id: 't1',
            title: '传感器固件烧录',
            status: 'completed',
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        },
    ],
};

export class DashboardApp extends Component<{}, DashboardAppState> {
    private dayTimer: ReturnType<typeof setInterval> | null = null;

    constructor() {
        super();
        this.state = {
            workspaces: [],
            tasksMap: {},
            loading: true,
            useMock: localStorage.getItem('1agents-db-use-mock') === 'true',
            companyName: localStorage.getItem('1agents-company-name') || '玄武智能科技工坊',
            isEditingCompany: false,
            dayCount: Number(localStorage.getItem('1agents-company-day') || '148'),
            funds: Number(localStorage.getItem('1agents-company-funds') || '2456800'),
            reputation: Number(localStorage.getItem('1agents-company-rep') || '88340'),
            tooltipVisible: false,
            tooltipX: 0,
            tooltipY: 0,
            tooltipData: null,
            launchingProjectId: null,
        };
    }

    async componentDidMount() {
        await this.loadData();

        // Days counter increments every 10 seconds to simulate time progression
        this.dayTimer = setInterval(() => {
            this.setState(prevState => {
                const nextDay = prevState.dayCount + 1;
                localStorage.setItem('1agents-company-day', String(nextDay));
                return { dayCount: nextDay };
            });
        }, 10000);
    }

    componentWillUnmount() {
        if (this.dayTimer) clearInterval(this.dayTimer);
    }

    loadData = async () => {
        if (this.state.useMock) {
            this.setState({
                workspaces: MOCK_WORKSPACES,
                tasksMap: MOCK_TASKS,
                loading: false,
            });
            return;
        }

        this.setState({ loading: true });
        try {
            const list = await workspaceService.list();
            const tasksMap: Record<string, Task[]> = {};

            // Parallel load task logs from the backend
            await Promise.all(
                list.map(async ws => {
                    try {
                        const res = await fetch(`/api/agent/tasks?workspace_id=${encodeURIComponent(ws.id)}`);
                        if (res.ok) {
                            const data = await res.json();
                            tasksMap[ws.id] = data || [];
                        }
                    } catch (e) {
                        console.error(`Failed to load tasks for workspace ${ws.id}`, e);
                    }
                })
            );

            this.setState({
                workspaces: list,
                tasksMap,
                loading: false,
            });
        } catch (err) {
            console.error('Failed to load workspaces list:', err);
            this.setState({
                workspaces: [],
                tasksMap: {},
                loading: false,
            });
        }
    };

    toggleMock = () => {
        this.setState(
            prevState => {
                const nextMock = !prevState.useMock;
                localStorage.setItem('1agents-db-use-mock', String(nextMock));
                return { useMock: nextMock };
            },
            () => {
                this.loadData();
            }
        );
    };

    handleCompanyDoubleClick = () => {
        this.setState({ isEditingCompany: true });
    };

    handleCompanyBlur = (e: Event) => {
        const value = (e.target as HTMLInputElement).value.trim();
        if (value) {
            this.setState({ companyName: value, isEditingCompany: false });
            localStorage.setItem('1agents-company-name', value);
        } else {
            this.setState({ isEditingCompany: false });
        }
    };

    handleCompanyKeyDown = (e: KeyboardEvent) => {
        if (e.key === 'Enter') {
            const value = (e.currentTarget as HTMLInputElement).value.trim();
            if (value) {
                this.setState({ companyName: value, isEditingCompany: false });
                localStorage.setItem('1agents-company-name', value);
            } else {
                this.setState({ isEditingCompany: false });
            }
        }
    };

    handleWorkbenchClick = async (ws: Workspace) => {
        if (ws.id.startsWith('mock-')) {
            ui.showToast('这是演示项目，真实项目请在大屏中双击直连。');
            return;
        }

        // Navigate back to workbench & load selected workspace
        const targetWs = wsStore.workspaces.value.find(w => w.id === ws.id);
        if (targetWs) {
            await wsStore.selectWorkspace(targetWs);
            stage.enterProject();

            // Reset query param & reload normal SPA
            window.location.href = window.location.origin + window.location.pathname;
        }
    };

    handleHover = (e: MouseEvent, visible: boolean, data: unknown) => {
        if (!visible) {
            this.setState({ tooltipVisible: false });
            return;
        }

        // Calculate offset position for retro tooltip
        this.setState({
            tooltipVisible: true,
            tooltipX: e.clientX + 15,
            tooltipY: e.clientY + 15,
            tooltipData: data,
        });
    };

    handleExit = () => {
        window.location.href = window.location.origin + window.location.pathname;
    };

    getProjectStatus(wsId: string): string {
        const tasks = this.state.tasksMap[wsId];
        if (!tasks || tasks.length === 0) return 'pending';
        const completedCount = tasks.filter(t => t.status === 'completed').length;
        if (completedCount === tasks.length && tasks.length > 0) return 'completed';
        if (tasks.some(t => t.status === 'failed')) return 'failed';
        if (tasks.some(t => t.status === 'blocked')) return 'blocked';
        if (tasks.some(t => t.status === 'running')) return 'running';
        return 'pending';
    }

    handleLaunchProject = (wsId: string, e: MouseEvent) => {
        e.stopPropagation();
        this.setState({ launchingProjectId: wsId });

        // Add credits/funds animation
        setTimeout(() => {
            this.setState(prevState => {
                const addedFunds = 150000;
                const addedRep = 450;
                const nextFunds = prevState.funds + addedFunds;
                const nextRep = prevState.reputation + addedRep;

                localStorage.setItem('1agents-company-funds', String(nextFunds));
                localStorage.setItem('1agents-company-rep', String(nextRep));

                return {
                    funds: nextFunds,
                    reputation: nextRep,
                    launchingProjectId: null,
                };
            });
            ui.showToast('🚀 发射成功！获得研发奖励资金 +$150,000，声望 +450！');
        }, 3000);
    };

    render() {
        const {
            workspaces,
            tasksMap,
            loading,
            useMock,
            companyName,
            isEditingCompany,
            dayCount,
            funds,
            reputation,
            tooltipVisible,
            tooltipX,
            tooltipY,
            tooltipData,
            launchingProjectId,
        } = this.state;

        const data = tooltipData as TooltipData | null;

        // Group workspaces by department parsed from names
        const groupedWorkspaces: Record<string, Workspace[]> = {};
        workspaces.forEach(ws => {
            const match = ws.name.match(/^\[(.*?)\]\s*(.*)$/);
            const dept = match ? match[1] : '常规业务部';
            if (!groupedWorkspaces[dept]) groupedWorkspaces[dept] = [];
            groupedWorkspaces[dept].push(ws);
        });

        // Compute active running operations count
        let runningCount = 0;
        Object.keys(tasksMap).forEach(id => {
            if (this.getProjectStatus(id) === 'running') {
                runningCount++;
            }
        });

        // Pick one completed project to display on the launchpad
        const completedProject = workspaces.find(w => this.getProjectStatus(w.id) === 'completed');

        return (
            <div class="pixel-dashboard-root">
                {/* ── HEADER TELEMETRY PANEL ── */}
                <header class="pixel-header">
                    <div class="pixel-logo-section">
                        {/* Inline retro pixel icon */}
                        <svg class="pixel-logo-icon" viewBox="0 0 16 16" width="48" height="48">
                            <rect x="5" y="2" width="6" height="2" fill="var(--pixel-cyan)" />
                            <rect x="3" y="4" width="10" height="2" fill="var(--pixel-cyan)" />
                            <rect x="2" y="6" width="12" height="4" fill="var(--pixel-cyan)" />
                            <rect x="3" y="10" width="10" height="2" fill="var(--pixel-cyan)" />
                            <rect x="5" y="12" width="6" height="2" fill="var(--pixel-cyan)" />
                            {/* Face */}
                            <rect x="4" y="6" width="2" height="2" fill="#000" />
                            <rect x="10" y="6" width="2" height="2" fill="#000" />
                        </svg>
                        <div class="pixel-company-name-box">
                            {isEditingCompany ? (
                                <input
                                    class="pixel-company-name-input"
                                    value={companyName}
                                    onBlur={this.handleCompanyBlur}
                                    onKeyDown={this.handleCompanyKeyDown}
                                    maxLength={20}
                                    autoFocus
                                />
                            ) : (
                                <span class="pixel-company-name" onDblClick={this.handleCompanyDoubleClick}>
                                    🏢 {companyName}
                                </span>
                            )}
                            <span class="pixel-day-counter">第 {dayCount} 天 (Level 12)</span>
                        </div>
                    </div>

                    <div class="pixel-hud-stats">
                        <div class="pixel-hud-item">
                            <span class="pixel-hud-icon">💰</span>
                            <span class="pixel-hud-label">资金:</span>
                            <span class="pixel-hud-value gold">${funds.toLocaleString()}</span>
                        </div>
                        <div class="pixel-hud-item">
                            <span class="pixel-hud-icon">⭐</span>
                            <span class="pixel-hud-label">声望:</span>
                            <span class="pixel-hud-value purple">{reputation.toLocaleString()}</span>
                        </div>
                        <div class="pixel-hud-item">
                            <span class="pixel-hud-icon">🔋</span>
                            <span class="pixel-hud-label">AI 负荷:</span>
                            <span class="pixel-hud-value cyan">
                                {runningCount}/{workspaces.length * 2} Ops
                            </span>
                        </div>
                    </div>

                    <div class="pixel-header-right">
                        <button class="pixel-header-btn" onClick={this.toggleMock}>
                            🎮 {useMock ? '使用真实数据' : '加载模拟演示'}
                        </button>
                        <button class="pixel-header-btn" onClick={this.handleExit}>
                            ↩️ 返回工作台
                        </button>
                    </div>
                </header>

                {/* ── OFFICE FLOORS SCROLL CONTAINER ── */}
                <main class="pixel-floors-container">
                    {loading ? (
                        <div style="display:flex;flex:1;align-items:center;justify-content:center;font-size:32px;">
                            🚀 装载星际工坊数据中...
                        </div>
                    ) : workspaces.length === 0 ? (
                        <div style="display:flex;flex-direction:column;flex:1;align-items:center;justify-content:center;gap:16px;">
                            <span style="font-size:28px;">工坊里空荡荡的，还没有创建任何项目呢！</span>
                            <button class="pixel-header-btn" onClick={this.toggleMock}>
                                🎮 加载模拟演示项目
                            </button>
                        </div>
                    ) : (
                        Object.keys(groupedWorkspaces).map(dept => {
                            const deptWorkspaces = groupedWorkspaces[dept];

                            // Determine department theme color
                            let theme = 'theme-cyan';
                            if (/硬件|制造|物联网|电子|机器|Iot/i.test(dept)) {
                                theme = 'theme-orange';
                            } else if (/媒体|推广|运营|自媒体|策划|视频/i.test(dept)) {
                                theme = 'theme-purple';
                            } else if (/科研|理论|开发|探索|研究/i.test(dept)) {
                                theme = 'theme-green';
                            }

                            return (
                                <section key={dept} class={`pixel-floor ${theme}`}>
                                    <div class="pixel-floor-header">
                                        <span class="pixel-floor-title">⚙️ {dept}</span>
                                        <span class="pixel-floor-count">工位数: {deptWorkspaces.length}</span>
                                    </div>
                                    <div class="pixel-floor-body">
                                        {deptWorkspaces.map(ws => (
                                            <DashboardWorkshop
                                                key={ws.id}
                                                workspace={ws}
                                                tasks={tasksMap[ws.id] || []}
                                                onClick={() => this.handleWorkbenchClick(ws)}
                                                onHover={this.handleHover}
                                            />
                                        ))}
                                    </div>
                                </section>
                            );
                        })
                    )}
                </main>

                {/* ── BOTTOM PIPELINE CONVEYOR BELT ── */}
                <footer class="pixel-pipeline">
                    {/* Intake Chamber */}
                    <div class="pixel-pipeline-chamber">
                        <svg class="pixel-chamber-icon" viewBox="0 0 16 16" width="42" height="42">
                            <rect x="3" y="3" width="10" height="10" fill="#a85cf9" />
                            <rect x="5" y="5" width="6" height="6" fill="#fff" opacity="0.3" />
                            <rect x="7" y="1" width="2" height="2" fill="#ef7d1a" />
                        </svg>
                        <span class="pixel-chamber-title">需求接收港</span>
                    </div>

                    {/* Conveyor Belt track */}
                    <div class="pixel-belt-track">
                        <div class="pixel-belt-scroller" />
                        {/* Package */}
                        <svg class="pixel-belt-package" viewBox="0 0 16 16" width="24" height="24">
                            <rect x="4" y="4" width="8" height="8" fill="var(--pixel-cyan)" />
                            <rect x="3" y="6" width="10" height="4" fill="var(--pixel-cyan)" />
                            <rect x="7" y="3" width="2" height="10" fill="#fff" opacity="0.4" />
                            <rect x="3" y="7" width="10" height="2" fill="#fff" opacity="0.4" />
                        </svg>
                    </div>

                    {/* Rocket Launchpad */}
                    <div class="pixel-pipeline-chamber">
                        {completedProject ? (
                            <div
                                class={`pixel-rocket-pad ${launchingProjectId === completedProject.id ? 'launching' : ''}`}
                                onClick={e => this.handleLaunchProject(completedProject.id, e)}
                                style="cursor:pointer;"
                                title="点击发射卡带，交付归档项目获得金币！"
                            >
                                <svg viewBox="0 0 16 16" width="42" height="42">
                                    <path
                                        d="M8,1 C8,1 11,4 11,8 L11,13 L5,13 L5,8 C5,4 8,1 8,1 Z"
                                        fill="var(--pixel-gold)"
                                    />
                                    <rect x="7" y="4" width="2" height="4" fill="#000" />
                                    <rect x="4" y="11" width="8" height="2" fill="var(--pixel-red)" />
                                    <path d="M3,10 L5,10 L5,13 L3,13 Z" fill="var(--pixel-orange)" />
                                    <path d="M11,10 L13,10 L13,13 L11,13 Z" fill="var(--pixel-orange)" />
                                </svg>
                                <div class="pixel-rocket-flame" />
                            </div>
                        ) : (
                            <svg viewBox="0 0 16 16" width="42" height="42">
                                <rect x="3" y="13" width="10" height="2" fill="#3c4268" />
                                <rect x="7" y="5" width="2" height="8" fill="#3c4268" />
                                <rect x="5" y="9" width="6" height="2" fill="#3c4268" />
                            </svg>
                        )}
                        <span class="pixel-chamber-title">
                            {completedProject ? '🚀 发射塔 (可发射)' : '发射塔 (待命中)'}
                        </span>
                    </div>
                </footer>

                {/* ── TOOLTIP HUD OVERLAY ── */}
                {tooltipVisible && data && (
                    <div class="pixel-tooltip-panel" style={`left: ${tooltipX}px; top: ${tooltipY}px;`}>
                        <div class="pixel-tooltip-title">{data.displayName}</div>
                        <div class="pixel-tooltip-row">
                            <span class="pixel-tooltip-label">所属科室:</span>
                            <span class="pixel-tooltip-value">{data.dept}</span>
                        </div>
                        <div class="pixel-tooltip-row">
                            <span class="pixel-tooltip-label">运行状态:</span>
                            <span
                                class="pixel-tooltip-value"
                                style={`color: ${data.status === 'running' ? 'var(--pixel-green)' : data.status === 'blocked' ? 'var(--pixel-orange)' : data.status === 'completed' ? 'var(--pixel-gold)' : 'var(--pixel-cyan)'}`}
                            >
                                {data.status === 'pending' && '策划提案阶段'}
                                {data.status === 'running' && '软件/硬件研发中'}
                                {data.status === 'completed' && '已完成，待发射'}
                                {data.status === 'blocked' && '警告：有阻碍项'}
                                {data.status === 'failed' && '故障：构建失败'}
                            </span>
                        </div>
                        <div class="pixel-tooltip-row">
                            <span class="pixel-tooltip-label">任务进度:</span>
                            <span class="pixel-tooltip-value">{data.progressPercent}%</span>
                        </div>
                        <div class="pixel-tooltip-row">
                            <span class="pixel-tooltip-label">特聘智能体:</span>
                            <span class="pixel-tooltip-value">👤 {data.activeAgent}</span>
                        </div>
                        <div style="font-size:16px;color:var(--pixel-border-light);margin-top:8px;border-top:1px dashed var(--pixel-border);padding-top:8px;">
                            💡 双击转场直连“玄武看板”
                        </div>
                    </div>
                )}
            </div>
        );
    }
}
