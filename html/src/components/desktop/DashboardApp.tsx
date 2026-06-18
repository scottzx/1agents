import { h, Component } from 'preact';
import { Workspace } from '../types';
import { Task } from '../drawer/TaskList/types';
import { workspaceService } from '../../services/workspaceService';
import * as wsStore from '../../stores/workspaceStore';
import * as stage from '../../stores/stageStore';
import * as ui from '../../stores/uiStore';
import { DashboardWorkshop, MockEmployee } from './DashboardWorkshop';

class PixelSoundManager {
    private ctx: AudioContext | null = null;
    public muted = localStorage.getItem('1agents-db-muted') === 'true';

    private init() {
        if (!this.ctx) {
            const win = window as unknown as {
                AudioContext?: typeof AudioContext;
                webkitAudioContext?: typeof AudioContext;
            };
            const AudioCtx = win.AudioContext || win.webkitAudioContext;
            if (AudioCtx) {
                this.ctx = new AudioCtx();
            }
        }
        if (this.ctx && this.ctx.state === 'suspended') {
            this.ctx.resume();
        }
    }

    setMuted(m: boolean) {
        this.muted = m;
        localStorage.setItem('1agents-db-muted', String(m));
    }

    playBlip() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();
        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.type = 'square';
        osc.frequency.setValueAtTime(600, this.ctx.currentTime);
        osc.frequency.exponentialRampToValueAtTime(1000, this.ctx.currentTime + 0.05);

        gain.gain.setValueAtTime(0.015, this.ctx.currentTime);
        gain.gain.exponentialRampToValueAtTime(0.001, this.ctx.currentTime + 0.05);

        osc.start();
        osc.stop(this.ctx.currentTime + 0.05);
    }

    playCoin() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();
        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.type = 'square';
        const now = this.ctx.currentTime;
        osc.frequency.setValueAtTime(987.77, now); // B5
        osc.frequency.setValueAtTime(1318.51, now + 0.08); // E6

        gain.gain.setValueAtTime(0.04, now);
        gain.gain.exponentialRampToValueAtTime(0.001, now + 0.3);

        osc.start();
        osc.stop(now + 0.3);
    }

    playLaser() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();
        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.type = 'sawtooth';
        const now = this.ctx.currentTime;
        osc.frequency.setValueAtTime(80, now);
        osc.frequency.exponentialRampToValueAtTime(1200, now + 1.5);

        gain.gain.setValueAtTime(0.05, now);
        gain.gain.exponentialRampToValueAtTime(0.001, now + 1.5);

        osc.start();
        osc.stop(now + 1.5);
    }

    playSelect() {
        if (this.muted) return;
        this.init();
        if (!this.ctx) return;
        const osc = this.ctx.createOscillator();
        const gain = this.ctx.createGain();
        osc.connect(gain);
        gain.connect(this.ctx.destination);

        osc.type = 'triangle';
        osc.frequency.setValueAtTime(300, this.ctx.currentTime);
        osc.frequency.setValueAtTime(400, this.ctx.currentTime + 0.05);

        gain.gain.setValueAtTime(0.05, this.ctx.currentTime);
        gain.gain.exponentialRampToValueAtTime(0.001, this.ctx.currentTime + 0.1);

        osc.start();
        osc.stop(this.ctx.currentTime + 0.1);
    }
}

export const sound = new PixelSoundManager();

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
    muted: boolean;

    // Tooltip HUD state
    tooltipVisible: boolean;
    tooltipX: number;
    tooltipY: number;
    tooltipData: unknown;

    // Rocket launch state
    launchingProjectId: string | null;

    // Gamification state
    employees: MockEmployee[];
    effortLevels: Record<string, 'low' | 'middle' | 'high'>;
    showReleaseModal: boolean;
    releasedProjectData: {
        id: string;
        name: string;
        views: number;
        stars: number;
        feedbacks: string[];
        phase: 'alpha' | 'beta' | 'stable';
    } | null;
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

const INITIAL_MOCK_EMPLOYEES: MockEmployee[] = [
    {
        id: 'emp-01',
        name: '克劳德 (Claude 3.5)',
        kind: 'basic',
        modelType: 'claude-3-5-sonnet',
        skills: ['代码重构 v1.2', 'TypeScript v2.0'],
        stamina: 85,
        ratingGood: 42,
        ratingNormal: 10,
        ratingPoor: 1,
        persona: '今天也是充满逻辑的一天！',
    },
    {
        id: 'emp-02',
        name: '视觉姬 (Imagine Pro)',
        kind: 'specialist',
        modelType: 'claude-3-5-sonnet',
        skills: ['SVG 矢量图 v3.5', 'UI 动效 v2.1', 'Tailwind 调色 v1.0'],
        stamina: 60,
        ratingGood: 128,
        ratingNormal: 8,
        ratingPoor: 0,
        persona: '平铺、扁平、不要渐变！美学就是正义。',
    },
    {
        id: 'emp-03',
        name: '双子座 (Gemini 1.5)',
        kind: 'basic',
        modelType: 'gemini-1-5-pro',
        skills: ['超长上下文 v1.5', '多模态视觉 v1.0'],
        stamina: 90,
        ratingGood: 35,
        ratingNormal: 15,
        ratingPoor: 2,
        persona: '只要上下文够长，没有什么我看不懂的！',
    },
    {
        id: 'emp-04',
        name: '运维狂人 (DevOps Guru)',
        kind: 'specialist',
        modelType: 'gpt-4o',
        skills: ['Docker 容器 v2.0', 'Nginx 配置 v1.5', 'Shell 脚本 v3.0'],
        stamina: 0,
        ratingGood: 95,
        ratingNormal: 12,
        ratingPoor: 1,
        persona: '精力槽已空... 正在补充咖啡因 Zzz...',
    },
    {
        id: 'emp-05',
        name: '小月 (Kimi Pro)',
        kind: 'basic',
        modelType: 'kimi-pro',
        skills: ['中文理解 v2.0', '文档提取 v1.0'],
        stamina: 75,
        ratingGood: 22,
        ratingNormal: 11,
        ratingPoor: 0,
        persona: '正在阅读 200 页的系统说明书。',
    },
    {
        id: 'emp-06',
        name: '量子学者 (Quantum Spec)',
        kind: 'specialist',
        modelType: 'gemini-1-5-pro',
        skills: ['量子数学 v4.1', 'NumPy 矩阵 v2.0'],
        stamina: 55,
        ratingGood: 110,
        ratingNormal: 5,
        ratingPoor: 0,
        persona: '观测即崩塌，我正在叠加状态中。',
    },
    {
        id: 'emp-07',
        name: '剪辑大师 (Video Creator)',
        kind: 'specialist',
        modelType: 'gpt-4o',
        skills: ['FFmpeg 裁剪 v3.0', '音轨合成 v1.0'],
        stamina: 70,
        ratingGood: 88,
        ratingNormal: 14,
        ratingPoor: 3,
        persona: '踩卡点，配 BGM，渲染走起！',
    },
    {
        id: 'emp-08',
        name: '极客之眼 (IoT Hacker)',
        kind: 'specialist',
        modelType: 'claude-3-5-sonnet',
        skills: ['固件烧录 v1.2', 'C++ 驱动 v2.5'],
        stamina: 100,
        ratingGood: 140,
        ratingNormal: 2,
        ratingPoor: 0,
        persona: '硬件就绪，开始写入寄存器。',
    },
];

export class DashboardApp extends Component<{}, DashboardAppState> {
    private dayTimer: ReturnType<typeof setInterval> | null = null;
    private staminaTimer: ReturnType<typeof setInterval> | null = null;

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
            muted: sound.muted,
            employees: INITIAL_MOCK_EMPLOYEES,
            showReleaseModal: false,
            releasedProjectData: null,
            effortLevels: {
                'mock-ws-01': 'middle',
                'mock-ws-02': 'high',
                'mock-ws-03': 'low',
                'mock-ws-04': 'middle',
                'mock-ws-05': 'high',
                'mock-ws-06': 'middle',
                'mock-ws-07': 'low',
                'mock-ws-08': 'middle',
            },
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

        // Stamina timer to simulate stamina depletion and recovery
        this.staminaTimer = setInterval(() => {
            this.setState(prevState => {
                const nextEmployees = prevState.employees.map((emp, idx) => {
                    const ws = prevState.workspaces[idx];
                    if (ws && this.getProjectStatus(ws.id) === 'running') {
                        const cost = emp.stamina > 0 ? 5 : 0;
                        return { ...emp, stamina: Math.max(0, emp.stamina - cost) };
                    } else if (emp.stamina < 100) {
                        return { ...emp, stamina: Math.min(100, emp.stamina + 2) };
                    }
                    return emp;
                });
                return { employees: nextEmployees };
            });
        }, 8000);
    }

    componentWillUnmount() {
        if (this.dayTimer) clearInterval(this.dayTimer);
        if (this.staminaTimer) clearInterval(this.staminaTimer);
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
        sound.playSelect();
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
        sound.playSelect();
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
        sound.playSelect();
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
            this.setState({ tooltipVisible: false, tooltipData: null });
            return;
        }

        const prevData = this.state.tooltipData as TooltipData | null;
        const currentData = data as TooltipData | null;
        if (
            !this.state.tooltipVisible ||
            (currentData && prevData && currentData.workspace.id !== prevData.workspace.id)
        ) {
            sound.playBlip();
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
        sound.playSelect();
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
        sound.playLaser();

        const ws = this.state.workspaces.find(w => w.id === wsId);
        if (!ws) return;

        const match = ws.name.match(/^\[(.*?)\]\s*(.*)$/);
        const displayName = match ? match[2] : ws.name;

        this.setState({ launchingProjectId: wsId });

        // Simulate rocket launch shaking & flying
        setTimeout(() => {
            const reviewsPool = [
                '哇！运行速度真快，体验极佳！',
                'SVG 渲染得太精致了，设计感拉满！',
                '有些微小的 Bug，但基本不影响使用。',
                '全自动流水线部署就是省心，太酷了。',
                '这个开源版本我会一直收藏关注！',
                '太棒了！作者是一人成军吗？简直是独立开发者的光芒！',
                '推理深度拉满的成果就是不一样，代码质量极高。',
            ];

            const selectedReviews: string[] = [];
            while (selectedReviews.length < 3) {
                const r = reviewsPool[Math.floor(Math.random() * reviewsPool.length)];
                if (!selectedReviews.includes(r)) selectedReviews.push(r);
            }

            const views = 1000 + Math.floor(Math.random() * 8000);
            const stars = Math.floor(views * (0.05 + Math.random() * 0.1));

            this.setState({
                showReleaseModal: true,
                releasedProjectData: {
                    id: wsId,
                    name: displayName,
                    views,
                    stars,
                    feedbacks: selectedReviews,
                    phase: 'beta',
                },
            });
        }, 3000);
    };

    changeReleasePhase = (phase: 'alpha' | 'beta' | 'stable') => {
        sound.playSelect();
        if (this.state.releasedProjectData) {
            const multiplier = phase === 'alpha' ? 0.6 : phase === 'beta' ? 1.0 : 1.8;
            const baseViews = this.state.releasedProjectData.views;
            this.setState(prevState => {
                if (!prevState.releasedProjectData) return {};
                return {
                    releasedProjectData: {
                        ...prevState.releasedProjectData,
                        phase,
                        views: Math.round(baseViews * multiplier),
                        stars: Math.round(baseViews * multiplier * (0.05 + Math.random() * 0.1)),
                    },
                };
            });
        }
    };

    confirmReleaseAndSettle = () => {
        sound.playCoin();
        const data = this.state.releasedProjectData;
        if (data) {
            const rewardFunds = data.stars * 200 + (data.phase === 'stable' ? 100000 : 30000);
            const rewardRep = Math.round(data.stars * 1.5 + (data.phase === 'stable' ? 300 : 100));

            this.setState(prevState => {
                const nextFunds = prevState.funds + rewardFunds;
                const nextRep = prevState.reputation + rewardRep;

                localStorage.setItem('1agents-company-funds', String(nextFunds));
                localStorage.setItem('1agents-company-rep', String(nextRep));

                return {
                    funds: nextFunds,
                    reputation: nextRep,
                    showReleaseModal: false,
                    releasedProjectData: null,
                    launchingProjectId: null,
                };
            });

            const wsIdx = this.state.workspaces.findIndex(w => w.id === data.id);
            if (wsIdx !== -1) {
                this.setState(prevState => {
                    const nextEmployees = [...prevState.employees];
                    if (nextEmployees[wsIdx]) {
                        nextEmployees[wsIdx] = {
                            ...nextEmployees[wsIdx],
                            ratingGood: nextEmployees[wsIdx].ratingGood + 5,
                        };
                    }
                    return { employees: nextEmployees };
                });
            }

            ui.showToast(`✨ 交付结算：获得研发资金 +$${rewardFunds.toLocaleString()}，声望 +${rewardRep}！`);
        }
    };

    toggleMute = () => {
        const nextMute = !this.state.muted;
        sound.setMuted(nextMute);
        this.setState({ muted: nextMute });
        sound.playSelect();
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
            muted,
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
                        <div class="pixel-hud-item" title="当前并发的智能体分身数量">
                            <span class="pixel-hud-icon">👥</span>
                            <span class="pixel-hud-label">分身:</span>
                            <span class="pixel-hud-value cyan">{runningCount} Clone(s)</span>
                        </div>
                        <div class="pixel-hud-item" title="自动运转完成的任务总数">
                            <span class="pixel-hud-icon">⚙️</span>
                            <span class="pixel-hud-label">自动化率:</span>
                            <span class="pixel-hud-value green">98.4%</span>
                        </div>
                    </div>

                    <div class="pixel-header-right">
                        <button class="pixel-header-btn" onClick={this.toggleMute} title="静音开关">
                            {muted ? '🔇' : '🔊'}
                        </button>
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
                        <div style="display:flex;flex:1;align-items:center;justify-content:center;font-size:36px;">
                            🚀 装载星际工坊数据中...
                        </div>
                    ) : workspaces.length === 0 ? (
                        <div style="display:flex;flex-direction:column;flex:1;align-items:center;justify-content:center;gap:16px;">
                            <span style="font-size:24px;">工坊里空荡荡的，还没有创建任何项目呢！</span>
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
                                        {deptWorkspaces.map(ws => {
                                            const wsIdx = workspaces.findIndex(w => w.id === ws.id);
                                            const emp =
                                                this.state.employees[
                                                    wsIdx >= 0 ? wsIdx % this.state.employees.length : 0
                                                ];
                                            const effort = this.state.effortLevels[ws.id] || 'middle';
                                            return (
                                                <DashboardWorkshop
                                                    key={ws.id}
                                                    workspace={ws}
                                                    tasks={tasksMap[ws.id] || []}
                                                    onClick={() => this.handleWorkbenchClick(ws)}
                                                    onHover={this.handleHover}
                                                    onPlaySound={type =>
                                                        type === 'coin' ? sound.playCoin() : sound.playBlip()
                                                    }
                                                    employee={emp}
                                                    effortLevel={effort}
                                                />
                                            );
                                        })}
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
                        <div style="font-size:12px;color:var(--pixel-border-light);margin-top:8px;border-top:1px dashed var(--pixel-border);padding-top:8px;">
                            💡 双击转场直连“玄武看板”
                        </div>
                    </div>
                )}

                {/* ── BUILDING IN PUBLIC RELEASE MODAL ── */}
                {this.state.showReleaseModal && this.state.releasedProjectData && (
                    <div class="pixel-dialog-overlay">
                        <div class="pixel-dialog release-dialog">
                            <div class="pixel-dialog-header">🚀 BUILDING IN PUBLIC 发布大厅</div>
                            <div class="pixel-dialog-body">
                                <p style="font-size:16px;color:var(--pixel-gold);text-align:center;margin-bottom:12px;">
                                    已成功交付项目：《{this.state.releasedProjectData.name}》
                                </p>

                                <div class="release-metrics-grid">
                                    <div class="release-metric-card">
                                        <span class="metric-icon">👁️</span>
                                        <span class="metric-label">围观量:</span>
                                        <span class="metric-val">
                                            {this.state.releasedProjectData.views.toLocaleString()}
                                        </span>
                                    </div>
                                    <div class="release-metric-card">
                                        <span class="metric-icon">⭐</span>
                                        <span class="metric-label">点赞/Stars:</span>
                                        <span class="metric-val">
                                            {this.state.releasedProjectData.stars.toLocaleString()}
                                        </span>
                                    </div>
                                    <div class="release-metric-card">
                                        <span class="metric-icon">📦</span>
                                        <span class="metric-label">交付阶段:</span>
                                        <span class="metric-val" style="color:var(--pixel-cyan)">
                                            {this.state.releasedProjectData.phase === 'alpha' && 'Alpha (内测)'}
                                            {this.state.releasedProjectData.phase === 'beta' && 'Beta (公测)'}
                                            {this.state.releasedProjectData.phase === 'stable' && '1.0 Stable (正式)'}
                                        </span>
                                    </div>
                                </div>

                                <div class="release-reviews-box">
                                    <div class="reviews-header">💬 社区反馈墙</div>
                                    <div class="reviews-list">
                                        {this.state.releasedProjectData.feedbacks.map((f, i) => (
                                            <div key={i} class="review-item">
                                                <span style="color:var(--pixel-cyan)">User_{100 + i}:</span> {f}
                                            </div>
                                        ))}
                                    </div>
                                </div>

                                <div class="release-phase-selector">
                                    <span style="font-size:12px;color:var(--pixel-border-light)">迭代发布版本：</span>
                                    <button
                                        class={`phase-btn ${this.state.releasedProjectData.phase === 'alpha' ? 'active' : ''}`}
                                        onClick={() => this.changeReleasePhase('alpha')}
                                    >
                                        Alpha
                                    </button>
                                    <button
                                        class={`phase-btn ${this.state.releasedProjectData.phase === 'beta' ? 'active' : ''}`}
                                        onClick={() => this.changeReleasePhase('beta')}
                                    >
                                        Beta
                                    </button>
                                    <button
                                        class={`phase-btn ${this.state.releasedProjectData.phase === 'stable' ? 'active' : ''}`}
                                        onClick={() => this.changeReleasePhase('stable')}
                                    >
                                        1.0 Stable
                                    </button>
                                </div>
                            </div>

                            <div class="pixel-dialog-buttons">
                                <button class="pixel-dialog-btn" onClick={this.confirmReleaseAndSettle}>
                                    ✨ 确 认 发 布
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        );
    }
}
