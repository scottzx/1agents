import { h, Component } from 'preact';
import { Workspace } from '../types';
import { ProjectItem } from '../drawer/TaskList/types';

export interface GamifiedTask extends ProjectItem {
    progress?: number;
}
import * as wsStore from '../../stores/workspaceStore';
import * as stage from '../../stores/stageStore';
import * as ui from '../../stores/uiStore';
import { DashboardWorkshop, MockEmployee } from './DashboardWorkshop';
import { DashboardCockpit } from './DashboardCockpit';
import { GlobalTaskBoard } from './GlobalTaskBoard';
import { dashboardService, DashboardData } from '@1agents/core/services/dashboardService';

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

// Main-app root URL for big-screen → workbench navigation. The big-screen is a
// separate document served at /dashboard, so "back to workbench" / drill-down
// must hit the app root, honoring a subpath mount (__BASE_PATH__, e.g. /tunnels).
const mainAppRoot = () => window.location.origin + (__BASE_PATH__ || '') + '/';

interface TooltipData {
    workspace: Workspace;
    tasks: ProjectItem[];
    status: string;
    progressPercent: number;
    dept: string;
    displayName: string;
    activeAgent: string;
}

interface DashboardAppState {
    workspaces: Workspace[];
    tasksMap: Record<string, ProjectItem[]>;
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

    // Roster & Skill system states
    showRosterModal: boolean;
    rosterTab: 'employees' | 'specialists' | 'skills' | 'practice';
    workspaceAssignments: Record<string, string[]>;
    skillCards: Array<{
        name: string;
        version: string;
        cost: number;
        level: number;
        good: number;
        normal: number;
        poor: number;
    }>;
    recentPractices: Array<{
        id: string;
        title: string;
        desc: string;
        model: string;
        skills: string[];
        status: 'pending' | 'encapsulated';
    }>;
    releasedProjectStage: 'rating' | 'settled';
    ratingMultiplier: number;

    // ── Real-data company cockpit (Phase 1) ──
    cockpit: DashboardData | null;
    // Real-mode cockpit sub-view: project rollup (大盘) vs cross-project task
    // board/calendar (全局看板, issue #91).
    cockpitView: 'projects' | 'board';
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

const MOCK_TASKS: Record<string, ProjectItem[]> = {
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
            companyName: localStorage.getItem('1agents-company-name') || '一万数字军团',
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
            showRosterModal: false,
            rosterTab: 'employees',
            workspaceAssignments: {
                'mock-ws-01': ['emp-01', 'emp-02'],
                'mock-ws-02': ['emp-03', 'emp-04'],
                'mock-ws-03': ['emp-05', 'emp-02'],
                'mock-ws-04': ['emp-08', 'emp-01'],
                'mock-ws-05': ['emp-07', 'emp-04'],
                'mock-ws-06': ['emp-06', 'emp-03'],
                'mock-ws-07': ['emp-07', 'emp-02'],
                'mock-ws-08': ['emp-08', 'emp-01'],
            },
            skillCards: [
                { name: '代码重构', version: 'v1.2', cost: 45000, level: 1, good: 48, normal: 12, poor: 1 },
                { name: 'SVG 矢量图', version: 'v3.5', cost: 60000, level: 3, good: 110, normal: 8, poor: 0 },
                { name: '超长上下文', version: 'v1.5', cost: 80000, level: 1, good: 35, normal: 15, poor: 2 },
                { name: 'Docker 容器', version: 'v2.0', cost: 50000, level: 2, good: 95, normal: 12, poor: 1 },
                { name: '中文理解', version: 'v2.0', cost: 30000, level: 2, good: 22, normal: 11, poor: 0 },
                { name: '量子数学', version: 'v4.1', cost: 120000, level: 4, good: 110, normal: 5, poor: 0 },
                { name: 'FFmpeg 裁剪', version: 'v3.0', cost: 40000, level: 3, good: 88, normal: 14, poor: 3 },
                { name: 'C++ 驱动开发', version: 'v2.5', cost: 55000, level: 2, good: 140, normal: 2, poor: 0 },
            ],
            recentPractices: [
                {
                    id: 'prac-01',
                    title: '玄武 AI 调度台主界面重构最佳实践',
                    desc: '在对玄武 AI 调度台进行重构时，通过多轮细化，利用克劳德将代码精简了 60% 且运行完美。',
                    model: 'claude-3-5-sonnet',
                    skills: ['代码重构 v1.2', 'TypeScript v2.0'],
                    status: 'pending',
                },
                {
                    id: 'prac-02',
                    title: 'FFmpeg 自动卡点剪辑会话最佳实践',
                    desc: '在一键剪影任务中，通过多次修复，成功解决了音频解码器冲突并生成了超带感音乐视频。',
                    model: 'gpt-4o',
                    skills: ['FFmpeg 裁剪 v3.0', '音轨合成 v1.0'],
                    status: 'pending',
                },
                {
                    id: 'prac-03',
                    title: '量子状态遥测与过滤器设计最佳实践',
                    desc: '在对量子感应设备做滤波时，智能体自主使用 NumPy 和卡尔曼滤波完成了数学计算和波形优化。',
                    model: 'gemini-1-5-pro',
                    skills: ['量子数学 v4.1', 'NumPy 矩阵 v2.0'],
                    status: 'pending',
                },
            ],
            releasedProjectStage: 'rating',
            ratingMultiplier: 1.0,
            cockpit: null,
            cockpitView: 'projects',
        };
    }

    async componentDidMount() {
        await this.loadData();

        // Days counter increments every 10 seconds to simulate time progression.
        // Real mode shows real data only — no simulated time progression.
        this.dayTimer = setInterval(() => {
            if (!this.state.useMock) return;
            this.setState(prevState => {
                const nextDay = prevState.dayCount + 1;
                localStorage.setItem('1agents-company-day', String(nextDay));
                return { dayCount: nextDay };
            });
        }, 10000);

        // Stamina timer to simulate stamina depletion, recovery and task progress
        // (3-second cycle). Mock-only: real mode must never fabricate stamina /
        // task progress / specialists / funds — it shows backend data verbatim.
        this.staminaTimer = setInterval(() => {
            if (!this.state.useMock) return;
            this.setState(prevState => {
                const {
                    workspaces,
                    workspaceAssignments,
                    effortLevels,
                    skillCards,
                    employees,
                    tasksMap,
                    recentPractices,
                    funds,
                } = prevState;

                // 1. Update employees stamina
                let nextEmployeesList = employees.map(emp => {
                    let activeClones = 0;
                    let totalCost = 0;

                    workspaces.forEach(ws => {
                        const cohortIds = workspaceAssignments[ws.id] || [];
                        const isAssigned = cohortIds.includes(emp.id);
                        const isRunning = this.getProjectStatus(ws.id, tasksMap) === 'running';
                        if (isAssigned && isRunning) {
                            activeClones++;
                            const effort = effortLevels[ws.id] || 'middle';
                            const costMap = { low: 2, middle: 4, high: 8 };
                            totalCost += costMap[effort];
                        }
                    });

                    if (activeClones > 0 && emp.stamina > 0) {
                        return { ...emp, stamina: Math.max(0, emp.stamina - totalCost) };
                    } else {
                        // Recover stamina
                        const recoverAmount = emp.stamina === 0 ? 5 : 8;
                        return { ...emp, stamina: Math.min(100, emp.stamina + recoverAmount) };
                    }
                });

                // 2. Update task progress
                const nextTasksMap = { ...tasksMap };
                let tasksUpdated = false;

                workspaces.forEach(ws => {
                    const cohortIds = workspaceAssignments[ws.id] || [];
                    const cohort = cohortIds
                        .map(id => nextEmployeesList.find(e => e.id === id))
                        .filter(Boolean) as MockEmployee[];

                    const averageStamina =
                        cohort.length === 0
                            ? 0
                            : Math.round(cohort.reduce((acc, curr) => acc + curr.stamina, 0) / cohort.length);
                    const status = this.getProjectStatus(ws.id, nextTasksMap);

                    if (status === 'running' && averageStamina > 0) {
                        const tasks = nextTasksMap[ws.id] || [];
                        const activeTaskIdx = tasks.findIndex(t => t.status === 'running');
                        if (activeTaskIdx !== -1) {
                            const activeTask = { ...tasks[activeTaskIdx] } as GamifiedTask;
                            const effort = effortLevels[ws.id] || 'middle';

                            // Speed of progress: low = 7%, middle = 13%, high = 24% (plus skill modifiers)
                            let speedBoost = 1.0;
                            cohort.forEach(emp => {
                                emp.skills.forEach(skillName => {
                                    const cleanSkillName = skillName.split(' ')[0];
                                    const card = skillCards.find(c => c.name.startsWith(cleanSkillName));
                                    if (card) {
                                        speedBoost += (card.level - 1) * 0.1;
                                    }
                                });
                            });
                            const stepMap = { low: 7, middle: 13, high: 24 };
                            const baseStep = stepMap[effort];
                            const actualStep = Math.round(baseStep * speedBoost);

                            activeTask.progress = Math.min(100, (activeTask.progress || 0) + actualStep);

                            const nextTasks = [...tasks];

                            if (activeTask.progress >= 100) {
                                activeTask.status = 'completed';
                                sound.playBlip();

                                // Find next pending task
                                const nextPendingIdx = nextTasks.findIndex(
                                    (t, idx) => idx > activeTaskIdx && t.status === 'pending'
                                );
                                if (nextPendingIdx !== -1) {
                                    nextTasks[nextPendingIdx] = {
                                        ...nextTasks[nextPendingIdx],
                                        status: 'running',
                                        progress: 0,
                                    } as GamifiedTask;
                                } else {
                                    // All tasks completed!
                                    sound.playCoin();
                                    const match = ws.name.match(/^\[(.*?)\]\s*(.*)$/);
                                    const dName = match ? match[2] : ws.name;
                                    setTimeout(() => {
                                        ui.showToast(`✨ 项目 [${dName}] 所有研发工作已就绪，可以发射！`);
                                    }, 100);
                                }
                            }

                            nextTasks[activeTaskIdx] = activeTask;
                            nextTasksMap[ws.id] = nextTasks;
                            tasksUpdated = true;
                        }
                    }
                });

                // 3. Auto-encapsulate pending practices randomly (e.g. 5% chance per 3s tick)
                let nextPractices = recentPractices;
                if (Math.random() < 0.05) {
                    const pendingPracIdx = nextPractices.findIndex(p => p.status === 'pending');
                    if (pendingPracIdx !== -1) {
                        const prac = nextPractices[pendingPracIdx];
                        const firstWord = prac.title.replace(/^玄武\s*AI\s*/, '').substring(0, 4);
                        const namePool = [
                            `${firstWord}姬`,
                            `专才-${firstWord}`,
                            `${firstWord}极客`,
                            `${firstWord}大拿`,
                        ];
                        const specName = namePool[Math.floor(Math.random() * namePool.length)];

                        const newEmp: MockEmployee = {
                            id: `emp-spec-${Date.now()}`,
                            name: specName,
                            kind: 'specialist',
                            modelType: prac.model,
                            skills: prac.skills,
                            stamina: 100,
                            ratingGood: 5,
                            ratingNormal: 0,
                            ratingPoor: 0,
                            persona: `从《${prac.title.substring(0, 10)}...》最佳实践会话中自动封装沉淀出来的专属专家！`,
                        };

                        nextPractices = nextPractices.map((p, idx) =>
                            idx === pendingPracIdx ? { ...p, status: 'encapsulated' as const } : p
                        );
                        nextEmployeesList = [...nextEmployeesList, newEmp];

                        setTimeout(() => {
                            sound.playLaser();
                            ui.showToast(`🎉 PMO 自动封装成功！固化专才员工 [${specName}]！`);
                        }, 50);
                    }
                }

                // 4. Auto-upgrade skills randomly (e.g. 3% chance per tick)
                let nextSkillCards = skillCards;
                let nextFunds = funds;
                if (Math.random() < 0.03 && funds > 80000) {
                    const randomSkillIdx = Math.floor(Math.random() * nextSkillCards.length);
                    const card = nextSkillCards[randomSkillIdx];
                    const nextLevel = card.level + 1;
                    const upgradeCost = card.cost;

                    if (funds >= upgradeCost) {
                        nextSkillCards = [...nextSkillCards];
                        nextSkillCards[randomSkillIdx] = {
                            ...card,
                            level: nextLevel,
                            version: `v${nextLevel}.0`,
                            cost: Math.round(card.cost * 1.8),
                        };

                        // Also update employee skills
                        const cleanName = card.name;
                        nextEmployeesList = nextEmployeesList.map(emp => {
                            const updatedSkills = emp.skills.map(s => {
                                if (s.startsWith(cleanName)) {
                                    return `${cleanName} v${nextLevel}.0`;
                                }
                                return s;
                            });
                            return { ...emp, skills: updatedSkills };
                        });

                        nextFunds = funds - upgradeCost;
                        localStorage.setItem('1agents-company-funds', String(nextFunds));

                        setTimeout(() => {
                            sound.playLaser();
                            ui.showToast(`✨ PMO 自动将技能卡 [${card.name}] 升级至 v${nextLevel}.0！`);
                        }, 100);
                    }
                }

                return {
                    employees: nextEmployeesList,
                    tasksMap: tasksUpdated ? nextTasksMap : tasksMap,
                    recentPractices: nextPractices,
                    skillCards: nextSkillCards,
                    funds: nextFunds,
                };
            });
        }, 3000);
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
                cockpit: null,
                loading: false,
            });
            return;
        }

        // Real mode: one read-only cross-project aggregate drives the cockpit.
        // This sidesteps the old per-workspace fan-out (and the mock-ws-XX key
        // mismatch) — the backend摊开 every project and ranks blockers on top.
        this.setState({ loading: true });
        try {
            const cockpit = await dashboardService.get();
            this.setState({ cockpit, loading: false });
        } catch (err) {
            console.error('Failed to load company dashboard:', err);
            this.setState({ cockpit: null, loading: false });
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
            stage.enterProjectDetail(targetWs.id, targetWs.name);

            // selectWorkspace persisted the active workspace id to localStorage,
            // so navigating the browser to the main app root makes it restore
            // that project. The big-screen page lives at /dashboard, so we point
            // at the app root (honoring a subpath mount) rather than the current
            // pathname.
            window.location.href = mainAppRoot();
        }
    };

    // Cockpit drill-down: open the real project's workbench by id.
    handleOpenProject = async (projectId: string) => {
        sound.playSelect();
        const targetWs = wsStore.workspaces.value.find(w => w.id === projectId);
        if (targetWs) {
            await wsStore.selectWorkspace(targetWs);
            stage.enterProjectDetail(targetWs.id, targetWs.name);
            window.location.href = mainAppRoot();
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
        window.location.href = mainAppRoot();
    };

    getProjectStatus(wsId: string, customTasksMap?: Record<string, ProjectItem[]>): string {
        const tasks = customTasksMap ? customTasksMap[wsId] : this.state.tasksMap[wsId];
        if (!tasks || tasks.length === 0) return 'pending';
        const completedCount = tasks.filter(t => t.status === 'completed').length;
        if (completedCount === tasks.length && tasks.length > 0) return 'completed';
        if (tasks.some(t => t.status === 'failed')) return 'failed';
        if (tasks.some(t => t.status === 'blocked')) return 'blocked';
        if (tasks.some(t => t.status === 'running')) return 'running';
        return 'pending';
    }

    toggleRosterModal = () => {
        sound.playSelect();
        this.setState(prevState => ({ showRosterModal: !prevState.showRosterModal }));
    };

    generateRandomTasksForWorkspace(wsId: string): ProjectItem[] {
        const softwareTasks = [
            ['主模块编写', 'API 接口对接', '单元测试覆盖', 'Bug 修复与审查'],
            ['前端组件开发', '响应式布局优化', '主题样式集成', '性能专项测试'],
            ['数据库设计', '索引与查询优化', 'Redis 缓存集成', '读写分离测试'],
        ];
        const hardwareTasks = [
            ['电路原理图设计', 'PCB 绘制与打样', '元器件焊接', '电气性能测试'],
            ['固件代码编写', '寄存器配置调试', '串口协议对接', '抗干扰压力测试'],
            ['3D 模型外壳设计', '支撑打印与打磨', '内部元器件装配', '防跌落与防尘测试'],
        ];
        const mediaTasks = [
            ['爆款文案策划', '排版与图文配图', '发布与社群推广', '数据复盘汇报'],
            ['视频分镜脚本编写', '绿幕素材拍摄', 'FFmpeg 精准裁剪', '音轨卡点与渲染'],
            ['活动海报设计', '配色方案确定', 'SVG 矢量导出', '适配各种尺寸分辨率'],
        ];
        const researchTasks = [
            ['查阅最新文献', '数学模型公式推导', 'MATLAB/Python 仿真', '实验报告总结'],
            ['量子模型设计', '矩阵状态方程求解', '误差修正与补偿', '阶段性成果汇报'],
        ];

        let pool = softwareTasks;
        if (wsId === 'mock-ws-02' || wsId === 'mock-ws-08') pool = hardwareTasks;
        else if (wsId === 'mock-ws-03' || wsId === 'mock-ws-07') pool = mediaTasks;
        else if (wsId === 'mock-ws-06') pool = researchTasks;

        const selectedGroup = pool[Math.floor(Math.random() * pool.length)];
        return selectedGroup.map((title, index) => ({
            id: `t-${Date.now()}-${index}`,
            title,
            status: index === 0 ? 'running' : 'pending',
            progress: 0,
            type: 'task',
            scheduleType: 'immediate',
            createdAt: '',
            updatedAt: '',
            replies: [],
            sessions: [],
        }));
    }

    handleLaunchProject = (wsId: string, e: MouseEvent) => {
        e.stopPropagation();
        sound.playLaser();

        const ws = this.state.workspaces.find(w => w.id === wsId);
        if (!ws) return;

        const match = ws.name.match(/^\[(.*?)\]\s*(.*)$/);
        const displayName = match ? match[2] : ws.name;

        this.setState({ launchingProjectId: wsId });

        // Calculate auto rating based on cohort stats
        const cohortIds = this.state.workspaceAssignments[wsId] || [];
        const cohort = cohortIds
            .map(id => this.state.employees.find(e => e.id === id))
            .filter(Boolean) as MockEmployee[];
        const averageStamina =
            cohort.length === 0 ? 0 : Math.round(cohort.reduce((acc, curr) => acc + curr.stamina, 0) / cohort.length);
        const totalSkillsCount = cohort.flatMap(c => c.skills).length;

        let autoRating: 'exceeds' | 'normal' | 'improvement' | 'poor' = 'normal';
        if (averageStamina > 70 && totalSkillsCount >= 3) {
            autoRating = 'exceeds';
        } else if (averageStamina < 30) {
            autoRating = 'poor';
        } else if (averageStamina < 55) {
            autoRating = 'improvement';
        }

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
                releasedProjectStage: 'rating',
                ratingMultiplier: 1.0,
                releasedProjectData: {
                    id: wsId,
                    name: displayName,
                    views,
                    stars,
                    feedbacks: selectedReviews,
                    phase: 'beta',
                },
            });

            // Automatically transition from 'rating' to 'settled' after 2.5s (simulated audit scan)
            setTimeout(() => {
                if (this.state.showReleaseModal && this.state.releasedProjectStage === 'rating') {
                    this.rateProject(autoRating);
                }
            }, 2500);
        }, 3000);
    };

    rateProject = (rating: 'exceeds' | 'normal' | 'improvement' | 'poor') => {
        sound.playCoin();

        let multiplier = 1.0;
        if (rating === 'exceeds') multiplier = 1.35;
        if (rating === 'improvement') multiplier = 0.7;
        if (rating === 'poor') multiplier = 0.35;

        this.setState(prevState => {
            if (!prevState.releasedProjectData) return {};
            const data = prevState.releasedProjectData;

            const views = Math.round(data.views * multiplier);
            const stars = Math.round(data.stars * multiplier);

            return {
                releasedProjectStage: 'settled',
                ratingMultiplier: multiplier,
                releasedProjectData: {
                    ...data,
                    views,
                    stars,
                },
            };
        });
    };

    confirmReleaseAndSettle = () => {
        sound.playCoin();
        const data = this.state.releasedProjectData;
        const multiplier = this.state.ratingMultiplier;
        if (data) {
            const rewardFunds = Math.round(data.stars * 200 + (data.phase === 'stable' ? 100000 : 30000));
            const rewardRep = Math.round(data.stars * 1.5 + (data.phase === 'stable' ? 300 : 100));

            let ratingCategory: 'good' | 'normal' | 'poor' = 'normal';
            if (multiplier > 1.0) ratingCategory = 'good';
            if (multiplier < 1.0) ratingCategory = 'poor';

            this.setState(prevState => {
                const nextFunds = prevState.funds + rewardFunds;
                const nextRep = prevState.reputation + rewardRep;

                localStorage.setItem('1agents-company-funds', String(nextFunds));
                localStorage.setItem('1agents-company-rep', String(nextRep));

                // 1. Update employee stats for all members in the cohort
                const cohortIds = prevState.workspaceAssignments[data.id] || [];
                const nextEmployees = prevState.employees.map(emp => {
                    if (cohortIds.includes(emp.id)) {
                        return {
                            ...emp,
                            ratingGood: ratingCategory === 'good' ? emp.ratingGood + 1 : emp.ratingGood,
                            ratingNormal: ratingCategory === 'normal' ? emp.ratingNormal + 1 : emp.ratingNormal,
                            ratingPoor: ratingCategory === 'poor' ? emp.ratingPoor + 1 : emp.ratingPoor,
                        };
                    }
                    return emp;
                });

                // 2. Update skill card stats
                const nextSkillCards = prevState.skillCards.map(card => {
                    const cleanSkillName = card.name.split(' ')[0];
                    const hasSkill = prevState.employees
                        .filter(e => cohortIds.includes(e.id))
                        .some(e => e.skills.some(s => s.startsWith(cleanSkillName)));
                    if (hasSkill) {
                        return {
                            ...card,
                            good: ratingCategory === 'good' ? card.good + 1 : card.good,
                            normal: ratingCategory === 'normal' ? card.normal + 1 : card.normal,
                            poor: ratingCategory === 'poor' ? card.poor + 1 : card.poor,
                        };
                    }
                    return card;
                });

                // 3. Generate new task list for this workspace so it runs again!
                const nextTasksMap = { ...prevState.tasksMap };
                nextTasksMap[data.id] = this.generateRandomTasksForWorkspace(data.id);

                return {
                    funds: nextFunds,
                    reputation: nextRep,
                    employees: nextEmployees,
                    skillCards: nextSkillCards,
                    tasksMap: nextTasksMap,
                    showReleaseModal: false,
                    releasedProjectData: null,
                    launchingProjectId: null,
                };
            });

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
        // ── Real-data company cockpit (Phase 1) ──
        // Mock mode keeps the精美 pixel demo below as the fallback shell; real
        // mode renders the Bento cockpit on the backend aggregate.
        if (!this.state.useMock) {
            return (
                <div class="cockpit-page">
                    <div class="cockpit-topbar">
                        <div class="cockpit-view-switch">
                            <button
                                class={`cockpit-topbar-btn ${this.state.cockpitView === 'projects' ? 'active' : ''}`}
                                onClick={() => this.setState({ cockpitView: 'projects' })}
                            >
                                🗂 项目大盘
                            </button>
                            <button
                                class={`cockpit-topbar-btn ${this.state.cockpitView === 'board' ? 'active' : ''}`}
                                onClick={() => this.setState({ cockpitView: 'board' })}
                            >
                                📋 全局看板
                            </button>
                        </div>
                        <button class="cockpit-topbar-btn" onClick={this.toggleMock}>
                            🎮 加载模拟演示
                        </button>
                        <button class="cockpit-topbar-btn" onClick={this.handleExit}>
                            ↩️ 返回工作台
                        </button>
                    </div>
                    {this.state.cockpitView === 'board' ? (
                        <GlobalTaskBoard />
                    ) : this.state.loading ? (
                        <div class="cockpit-empty">正在装载公司大盘...</div>
                    ) : this.state.cockpit ? (
                        <DashboardCockpit
                            data={this.state.cockpit}
                            companyName={this.state.companyName}
                            onOpenProject={this.handleOpenProject}
                            onRefresh={this.loadData}
                        />
                    ) : (
                        <div class="cockpit-empty">
                            无法加载大盘数据。
                            <button class="cockpit-topbar-btn" onClick={this.loadData}>
                                重试
                            </button>
                        </div>
                    )}
                </div>
            );
        }

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
                            <rect x="4" y="6" width="2" height="2" fill="#5a3c20" />
                            <rect x="10" y="6" width="2" height="2" fill="#5a3c20" />
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
                        <button
                            class="pixel-header-btn"
                            onClick={this.toggleRosterModal}
                            style="border-color: var(--pixel-cyan); color: var(--pixel-cyan);"
                            title="打开人才管理与技能升级中心"
                        >
                            👥 人才与技能
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
                                            const cohortIds = this.state.workspaceAssignments[ws.id] || [];
                                            const cohort = cohortIds
                                                .map(id => this.state.employees.find(e => e.id === id))
                                                .filter(Boolean) as MockEmployee[];
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
                                                    cohort={cohort}
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
                            <rect x="3" y="3" width="10" height="10" fill="#9b7bd4" />
                            <rect x="5" y="5" width="6" height="6" fill="#fffaf0" opacity="0.35" />
                            <rect x="7" y="1" width="2" height="2" fill="#d97b4a" />
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
                                    <rect x="7" y="4" width="2" height="4" fill="#7a5a3c" />
                                    <rect x="4" y="11" width="8" height="2" fill="var(--pixel-red)" />
                                    <path d="M3,10 L5,10 L5,13 L3,13 Z" fill="var(--pixel-orange)" />
                                    <path d="M11,10 L13,10 L13,13 L11,13 Z" fill="var(--pixel-orange)" />
                                </svg>
                                <div class="pixel-rocket-flame" />
                            </div>
                        ) : (
                            <svg viewBox="0 0 16 16" width="42" height="42">
                                <rect x="3" y="13" width="10" height="2" fill="#e6d8b8" />
                                <rect x="7" y="5" width="2" height="8" fill="#e6d8b8" />
                                <rect x="5" y="9" width="6" height="2" fill="#e6d8b8" />
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
                        <div class="pixel-dialog release-dialog" style="width: 580px;">
                            <div class="pixel-dialog-header">🚀 BUILDING IN PUBLIC 发布大厅</div>
                            <div class="pixel-dialog-body">
                                <p style="font-size:16px;color:var(--pixel-gold);text-align:center;margin-bottom:12px;">
                                    已成功交付项目：《{this.state.releasedProjectData.name}》
                                </p>

                                {this.state.releasedProjectStage === 'rating' ? (
                                    <div style="text-align:center; margin: 20px 0;">
                                        <p style="font-size:14px; color:#fff; margin-bottom:16px;" class="pixel-blink">
                                            🔍 PMO 质量检验与自动审计扫描中...
                                        </p>
                                        <div style="font-size:12px; color:var(--pixel-border-light); margin-top:20px;">
                                            <div>🤖 正在评估协同智能体梯队贡献...</div>
                                            <div>📊 正在校验执行日志与 Benchmark 分数...</div>
                                            <div style="margin-top:15px; display:inline-block; width:200px; height:8px; border:2px solid var(--pixel-border); position:relative; overflow:hidden;">
                                                <div
                                                    class="pixel-loading-bar"
                                                    style="height:100%; background:var(--pixel-green); width:60%; animation: loadingMove 2.5s infinite linear;"
                                                />
                                            </div>
                                        </div>
                                    </div>
                                ) : (
                                    <div>
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
                                                    Beta (公测)
                                                </span>
                                            </div>
                                        </div>

                                        <div class="release-reviews-box">
                                            <div class="reviews-header">
                                                💬 社区反馈墙 (PMO 审计评价:{' '}
                                                {this.state.ratingMultiplier === 1.35
                                                    ? '🌟 超乎期望 (Exceeds)'
                                                    : this.state.ratingMultiplier === 1.0
                                                      ? '👍 正常 (Normal)'
                                                      : this.state.ratingMultiplier === 0.7
                                                        ? '⚠️ 有待提升 (Improve)'
                                                        : '❌ 糟糕 (Poor)'}
                                                )
                                            </div>
                                            <div class="reviews-list">
                                                {this.state.releasedProjectData.feedbacks.map((f, i) => (
                                                    <div key={i} class="review-item">
                                                        <span style="color:var(--pixel-cyan)">User_{100 + i}:</span> {f}
                                                    </div>
                                                ))}
                                            </div>
                                        </div>

                                        <div
                                            class="release-phase-selector"
                                            style="text-align: center; justify-content: center;"
                                        >
                                            <span style="font-size:12px;color:var(--pixel-border-light)">
                                                💡 本次交付评价由 PMO 自动化审计生成，结果已存档。
                                            </span>
                                        </div>

                                        <div class="pixel-dialog-buttons">
                                            <button class="pixel-dialog-btn" onClick={this.confirmReleaseAndSettle}>
                                                ✨ 确定结算收工
                                            </button>
                                        </div>
                                    </div>
                                )}
                            </div>
                        </div>
                    </div>
                )}

                {/* ── ROSTER & SKILLS MODAL ── */}
                {this.state.showRosterModal && (
                    <div class="pixel-dialog-overlay" onClick={this.toggleRosterModal}>
                        <div
                            class="pixel-dialog roster-dialog"
                            onClick={e => e.stopPropagation()}
                            style="width: 900px; max-width: 95%;"
                        >
                            <div class="pixel-dialog-header">👥 一芥像素工坊 - 人才与技能管理中心</div>

                            {/* Tabs */}
                            <div
                                class="pixel-tabs"
                                style="display:flex; justify-content:center; gap:8px; margin-bottom:16px; border-bottom:2px solid var(--pixel-border); padding-bottom:8px;"
                            >
                                <button
                                    class={`pixel-tab-btn ${this.state.rosterTab === 'employees' ? 'active' : ''}`}
                                    onClick={() => this.setState({ rosterTab: 'employees' })}
                                >
                                    🧑‍💻 基础雇员库
                                </button>
                                <button
                                    class={`pixel-tab-btn ${this.state.rosterTab === 'specialists' ? 'active' : ''}`}
                                    onClick={() => this.setState({ rosterTab: 'specialists' })}
                                >
                                    ⭐ 专才专家库
                                </button>
                                <button
                                    class={`pixel-tab-btn ${this.state.rosterTab === 'skills' ? 'active' : ''}`}
                                    onClick={() => this.setState({ rosterTab: 'skills' })}
                                >
                                    ⚡ 技能卡管理
                                </button>
                                <button
                                    class={`pixel-tab-btn ${this.state.rosterTab === 'practice' ? 'active' : ''}`}
                                    onClick={() => this.setState({ rosterTab: 'practice' })}
                                >
                                    📦 最佳实践封装
                                </button>
                            </div>

                            <div
                                class="pixel-dialog-body roster-modal-body"
                                style="height: 400px; overflow-y: auto; padding-right:8px;"
                            >
                                {this.state.rosterTab === 'employees' && this.renderEmployeesTab()}
                                {this.state.rosterTab === 'specialists' && this.renderSpecialistsTab()}
                                {this.state.rosterTab === 'skills' && this.renderSkillsTab()}
                                {this.state.rosterTab === 'practice' && this.renderPracticeTab()}
                            </div>

                            <div class="pixel-dialog-buttons" style="margin-top:16px;">
                                <button class="pixel-dialog-btn" onClick={this.toggleRosterModal}>
                                    确定关闭
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    renderEmployeesTab() {
        const basicEmployees = this.state.employees.filter(e => e.kind === 'basic');
        return (
            <div class="roster-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:16px;">
                {basicEmployees.map(emp => {
                    const runningWS = this.state.workspaces.filter(
                        w =>
                            (this.state.workspaceAssignments[w.id] || []).includes(emp.id) &&
                            this.getProjectStatus(w.id) === 'running'
                    );
                    const clones = runningWS.length;
                    return (
                        <div
                            key={emp.id}
                            class="roster-card basic-card"
                            style="background: rgba(0,0,0,0.4); border:2px solid var(--pixel-border); padding:12px; border-radius:4px; display:flex; flex-direction:column; justify-content:space-between;"
                        >
                            <div>
                                <div
                                    class="card-title-bar"
                                    style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px; border-bottom:1px solid rgba(255,255,255,0.1); padding-bottom:4px;"
                                >
                                    <span
                                        class="emp-name"
                                        style="font-weight:bold; color:var(--pixel-cyan); font-size:14px;"
                                    >
                                        {emp.name}
                                    </span>
                                </div>
                                <div class="card-details" style="font-size:12px; line-height:1.6;">
                                    <div class="detail-row">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            基座模型:
                                        </span>{' '}
                                        <span class="val cyan" style="color:var(--pixel-cyan); margin-left:4px;">
                                            {emp.modelType}
                                        </span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            评测得分:
                                        </span>{' '}
                                        <span class="val gold" style="color:var(--pixel-gold); margin-left:4px;">
                                            {emp.modelType.includes('claude')
                                                ? '88.7% (MMLU)'
                                                : emp.modelType.includes('gemini')
                                                  ? '86.2% (MMLU)'
                                                  : '85.4% (MMLU)'}
                                        </span>
                                    </div>
                                    <div class="detail-row" style="display:flex; align-items:center;">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            精力电池:
                                        </span>
                                        <div
                                            class="pixel-stamina-battery"
                                            style="width: 80px; height: 12px; margin-left: 8px;"
                                            title={`精力值: ${emp.stamina}/100`}
                                        >
                                            <div
                                                class={`pixel-stamina-fill ${emp.stamina < 30 ? 'low' : emp.stamina < 60 ? 'medium' : 'high'}`}
                                                style={`width: ${emp.stamina}%`}
                                            />
                                        </div>
                                        <span class="stamina-text" style="font-size:10px; margin-left:6px;">
                                            {emp.stamina}%
                                        </span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            当前分身:
                                        </span>{' '}
                                        <span class="val green" style="color:var(--pixel-green); margin-left:4px;">
                                            {clones} Clone(s)
                                        </span>
                                    </div>
                                    <div class="detail-row">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            评价历史:
                                        </span>
                                        <span class="val text-small" style="margin-left:4px;">
                                            🌟{emp.ratingGood} 👍{emp.ratingNormal} ❌{emp.ratingPoor}
                                        </span>
                                    </div>
                                    <div class="detail-row skills-row" style="margin-top:4px;">
                                        <span class="label" style="color:var(--pixel-border-light)">
                                            搭载技能:
                                        </span>
                                        <div
                                            class="tags-container"
                                            style="display:flex; flex-wrap:wrap; gap:4px; margin-top:4px;"
                                        >
                                            {emp.skills.map((s, i) => (
                                                <span key={i} class="pixel-skill-tag">
                                                    {s}
                                                </span>
                                            ))}
                                        </div>
                                    </div>
                                    <div
                                        class="persona-bubble"
                                        style="background:#000; border:1px dashed var(--pixel-border); padding:6px; font-style:italic; font-size:11px; margin-top:8px; border-radius:4px; color:#aaa;"
                                    >
                                        "{emp.persona}"
                                    </div>
                                </div>
                            </div>
                            <div class="card-actions" style="margin-top:8px;">
                                <div style="font-size:10px; color:var(--pixel-border-light); text-align:center; padding: 4px; border: 1px dashed rgba(255,255,255,0.1);">
                                    🔋 算力电量自动充能中
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>
        );
    }

    renderSpecialistsTab() {
        const specEmployees = this.state.employees.filter(e => e.kind === 'specialist');
        return (
            <div class="roster-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:16px;">
                {specEmployees.length === 0 ? (
                    <div style="grid-column: 1/-1; text-align: center; padding: 40px; color: var(--pixel-border-light)">
                        💡 暂无特聘专家。可以在“最佳实践封装”中将日常成功聊天轨迹固化为专家！
                    </div>
                ) : (
                    specEmployees.map(emp => {
                        const runningWS = this.state.workspaces.filter(
                            w =>
                                (this.state.workspaceAssignments[w.id] || []).includes(emp.id) &&
                                this.getProjectStatus(w.id) === 'running'
                        );
                        const clones = runningWS.length;
                        return (
                            <div
                                key={emp.id}
                                class="roster-card spec-card"
                                style="background: rgba(0,0,0,0.4); border:2px solid var(--pixel-purple); padding:12px; border-radius:4px; display:flex; flex-direction:column; justify-content:space-between;"
                            >
                                <div>
                                    <div
                                        class="card-title-bar"
                                        style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px; border-bottom:1px solid rgba(168,92,249,0.3); padding-bottom:4px; background:rgba(168,92,249,0.1); margin:-12px -12px 8px -12px; padding:8px 12px;"
                                    >
                                        <span
                                            class="emp-name"
                                            style="font-weight:bold; color:var(--pixel-purple); font-size:14px;"
                                        >
                                            ★ {emp.name}
                                        </span>
                                    </div>
                                    <div class="card-details" style="font-size:12px; line-height:1.6;">
                                        <div class="detail-row">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                基座模型:
                                            </span>{' '}
                                            <span class="val cyan" style="color:var(--pixel-cyan); margin-left:4px;">
                                                {emp.modelType}
                                            </span>
                                        </div>
                                        <div class="detail-row">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                评测得分:
                                            </span>{' '}
                                            <span class="val gold" style="color:var(--pixel-gold); margin-left:4px;">
                                                98.4% (专才能力)
                                            </span>
                                        </div>
                                        <div class="detail-row" style="display:flex; align-items:center;">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                精力电池:
                                            </span>
                                            <div
                                                class="pixel-stamina-battery"
                                                style="width: 80px; height: 12px; margin-left: 8px;"
                                                title={`精力值: ${emp.stamina}/100`}
                                            >
                                                <div
                                                    class={`pixel-stamina-fill ${emp.stamina < 30 ? 'low' : emp.stamina < 60 ? 'medium' : 'high'}`}
                                                    style={`width: ${emp.stamina}%`}
                                                />
                                            </div>
                                            <span class="stamina-text" style="font-size:10px; margin-left:6px;">
                                                {emp.stamina}%
                                            </span>
                                        </div>
                                        <div class="detail-row">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                当前分身:
                                            </span>{' '}
                                            <span class="val green" style="color:var(--pixel-green); margin-left:4px;">
                                                {clones} Clone(s)
                                            </span>
                                        </div>
                                        <div class="detail-row">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                评价历史:
                                            </span>
                                            <span class="val text-small" style="margin-left:4px;">
                                                🌟{emp.ratingGood} 👍{emp.ratingNormal} ❌{emp.ratingPoor}
                                            </span>
                                        </div>
                                        <div class="detail-row skills-row" style="margin-top:4px;">
                                            <span class="label" style="color:var(--pixel-border-light)">
                                                搭载特长:
                                            </span>
                                            <div
                                                class="tags-container"
                                                style="display:flex; flex-wrap:wrap; gap:4px; margin-top:4px;"
                                            >
                                                {emp.skills.map((s, i) => (
                                                    <span
                                                        key={i}
                                                        class="pixel-skill-tag"
                                                        style="border-color:var(--pixel-purple); color:var(--pixel-purple);"
                                                    >
                                                        {s}
                                                    </span>
                                                ))}
                                            </div>
                                        </div>
                                        <div
                                            class="persona-bubble"
                                            style="background:#000; border:1px dashed var(--pixel-purple); padding:6px; font-style:italic; font-size:11px; margin-top:8px; border-radius:4px; color:#aaa;"
                                        >
                                            "{emp.persona}"
                                        </div>
                                    </div>
                                </div>
                                <div class="card-actions" style="margin-top:8px;">
                                    <div style="font-size:10px; color:var(--pixel-border-light); text-align:center; padding: 4px; border: 1px dashed rgba(168,92,249,0.2);">
                                        🔋 算力电量自动充能中
                                    </div>
                                </div>
                            </div>
                        );
                    })
                )}
            </div>
        );
    }

    renderSkillsTab() {
        return (
            <div class="skills-grid" style="display:grid; grid-template-columns:1fr 1fr; gap:16px;">
                {this.state.skillCards.map((card, idx) => (
                    <div
                        key={idx}
                        class="skill-card-item"
                        style="background: rgba(0,0,0,0.4); border: 2px solid var(--pixel-border); padding: 12px; border-radius: 4px; display: flex; flex-direction: column; justify-content: space-between;"
                    >
                        <div>
                            <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:8px;">
                                <span style="font-size:14px; font-weight:bold; color:var(--pixel-cyan);">
                                    {card.name}
                                </span>
                                <span
                                    class="pixel-wb-status-badge"
                                    style="background: var(--pixel-gold); font-size:8px;"
                                >
                                    Ver {card.version}
                                </span>
                            </div>
                            <div style="font-size:11px; color:var(--pixel-border-light); margin-bottom:10px; line-height:1.5;">
                                <div>
                                    当前级别: Level {card.level} (研发效率 +{(card.level - 1) * 15}%)
                                </div>
                                <div>
                                    历史成效: 🌟{card.good} | 👍{card.normal} | ❌{card.poor}
                                </div>
                            </div>
                        </div>
                        <div style="width: 100%; font-size: 11px; padding: 6px 4px; margin-top: 8px; text-align: center; border: 1px dashed var(--pixel-border); color: var(--pixel-border-light); background: rgba(0,0,0,0.2);">
                            ⚙️ 技能由 PMO 自动研发升级中
                        </div>
                    </div>
                ))}
            </div>
        );
    }

    renderPracticeTab() {
        return (
            <div class="practice-list" style="display:flex; flex-direction:column; gap:16px;">
                {this.state.recentPractices.map(prac => (
                    <div
                        key={prac.id}
                        class="practice-item"
                        style="background: rgba(0,0,0,0.4); border: 2px solid var(--pixel-border); padding:16px; border-radius: 4px; display:flex; justify-content:space-between; align-items:center;"
                    >
                        <div style="max-width: 75%;">
                            <div style="font-size:14px; font-weight:bold; color:var(--pixel-gold); margin-bottom:6px;">
                                📜 {prac.title}
                            </div>
                            <div style="font-size:12px; color:var(--pixel-text); margin-bottom:8px; line-height:1.4;">
                                {prac.desc}
                            </div>
                            <div style="font-size:11px; color:var(--pixel-border-light); display:flex; gap:12px;">
                                <span>
                                    模型底座: <strong style="color:var(--pixel-cyan)">{prac.model}</strong>
                                </span>
                                <span>
                                    沉淀技能卡:{' '}
                                    <strong style="color:var(--pixel-cyan)">{prac.skills.join(', ')}</strong>
                                </span>
                            </div>
                        </div>
                        <div>
                            {prac.status === 'pending' ? (
                                <span style="color:var(--pixel-gold); font-size:12px; display:flex; flex-direction:column; align-items:flex-end; gap:4px;">
                                    <span class="pixel-loading-dots">⏳ 自动归档中</span>
                                    <span style="font-size:9px;color:var(--pixel-border-light)">PMO Syncing...</span>
                                </span>
                            ) : (
                                <span style="color:var(--pixel-green); font-size:12px; display:flex; flex-direction:column; align-items:flex-end; gap:4px;">
                                    <span>✅ 已固化</span>
                                    <span style="font-size:9px;color:var(--pixel-border-light)">
                                        Specialist Spawned
                                    </span>
                                </span>
                            )}
                        </div>
                    </div>
                ))}
            </div>
        );
    }
}
