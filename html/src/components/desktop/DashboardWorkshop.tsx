import { h, Component } from 'preact';
import { Workspace } from '../types';
import { Task } from '../drawer/TaskList/types';

export interface GamifiedTask extends Task {
    progress?: number;
}

export interface MockEmployee {
    id: string;
    name: string;
    kind: 'basic' | 'specialist';
    modelType: string;
    skills: string[];
    stamina: number;
    ratingGood: number;
    ratingNormal: number;
    ratingPoor: number;
    persona: string;
}

interface DashboardWorkshopProps {
    workspace: Workspace;
    tasks: Task[];
    onClick: () => void;
    onHover: (e: MouseEvent, visible: boolean, data: unknown) => void;
    onPlaySound?: (type: 'coin' | 'blip') => void;
    cohort: MockEmployee[];
    effortLevel: 'low' | 'middle' | 'high';
}

interface DashboardWorkshopState {
    logs: string[];
    coins: { id: number; text: string; color: 'cyan' | 'gold'; x: number; y: number }[];
}

export class DashboardWorkshop extends Component<DashboardWorkshopProps, DashboardWorkshopState> {
    private logInterval: ReturnType<typeof setInterval> | null = null;
    private logIdCounter = 0;
    private coinIdCounter = 0;

    constructor(props: DashboardWorkshopProps) {
        super(props);
        this.state = {
            logs: ['> Systems initialized.', '> Awaiting instructions.'],
            coins: [],
        };
    }

    componentDidMount() {
        // Generate simulated log telemetry lines to make the office look alive
        this.logInterval = setInterval(
            () => {
                const status = this.getProjectStatus();

                if (status === 'running') {
                    const codingLogs = [
                        `> Compile: package-${this.logIdCounter++} OK`,
                        '> Test suite: 98% passed',
                        '> Agent is executing command...',
                        '> Code patch: applied',
                        '> Checking linter constraints',
                        '> CPU usage: 22% | MEM: 450MB',
                        '> Syncing workspace modifications',
                    ];
                    const newLog = codingLogs[Math.floor(Math.random() * codingLogs.length)];
                    this.addLogLine(newLog);

                    // Spawn floating point/coin animation sometimes when coding
                    if (Math.random() < 0.3) {
                        const points = ['Code +1', 'FUN +1', 'Stability +1', 'Quality +2'];
                        this.spawnCoin(points[Math.floor(Math.random() * points.length)], 'cyan');
                    }
                } else if (status === 'blocked') {
                    const warningLogs = [
                        `> WARNING: blocked on task #${this.logIdCounter++}`,
                        '> Critical lock detected!',
                        '> Awaiting user approval...',
                        '> Agent paused execution',
                    ];
                    const newLog = warningLogs[Math.floor(Math.random() * warningLogs.length)];
                    this.addLogLine(newLog);
                } else if (status === 'failed') {
                    this.addLogLine('> CRITICAL ERROR: execution aborted');
                } else if (status === 'completed') {
                    if (Math.random() < 0.1) {
                        this.addLogLine('> Project finished. Deployed successfully.');
                        this.spawnCoin('GOLD +100', 'gold');
                    }
                }
            },
            3000 + Math.random() * 2000
        );
    }

    componentWillUnmount() {
        if (this.logInterval) clearInterval(this.logInterval);
    }

    addLogLine(line: string) {
        this.setState(prevState => {
            const nextLogs = [...prevState.logs, line];
            if (nextLogs.length > 3) nextLogs.shift();
            return { logs: nextLogs };
        });
    }

    spawnCoin(text: string, color: 'cyan' | 'gold') {
        const id = this.coinIdCounter++;
        const newCoin = {
            id,
            text,
            color,
            x: 40 + Math.random() * 40,
            y: 40 + Math.random() * 20,
        };
        this.setState(prevState => ({
            coins: [...prevState.coins, newCoin],
        }));
        if (this.props.onPlaySound) {
            this.props.onPlaySound(color === 'gold' ? 'coin' : 'blip');
        }
        // Remove coin after animation ends
        setTimeout(() => {
            this.setState(prevState => ({
                coins: prevState.coins.filter(c => c.id !== id),
            }));
        }, 1200);
    }

    getProjectStatus(): 'pending' | 'running' | 'completed' | 'blocked' | 'failed' {
        const { tasks } = this.props;
        if (!tasks || tasks.length === 0) return 'pending';

        const completedCount = tasks.filter(t => t.status === 'completed').length;
        const totalCount = tasks.length;
        if (completedCount === totalCount && totalCount > 0) return 'completed';

        if (tasks.some(t => t.status === 'failed')) return 'failed';
        if (tasks.some(t => t.status === 'blocked')) return 'blocked';
        if (tasks.some(t => t.status === 'running')) return 'running';
        return 'pending';
    }

    getDepartmentInfo() {
        const { workspace } = this.props;
        const name = workspace.name;

        // Parse department name prefix
        const match = name.match(/^\[(.*?)\]\s*(.*)$/);
        const dept = match ? match[1] : '常规业务部';
        const displayName = match ? match[2] : name;

        // Determine theme class & icon style
        let themeClass = 'theme-cyan';
        let type = 'software';
        let bgStyle = 'code';

        if (/硬件|制造|物联网|电子|机器|Iot/i.test(dept)) {
            themeClass = 'theme-orange';
            type = 'hardware';
            bgStyle = 'workbench';
        } else if (/媒体|推广|运营|自媒体|策划|视频/i.test(dept)) {
            themeClass = 'theme-purple';
            type = 'media';
            bgStyle = 'studio';
        } else if (/科研|理论|开发|探索|研究/i.test(dept)) {
            themeClass = 'theme-green';
            type = 'research';
            bgStyle = 'lab';
        }

        return { dept, displayName, themeClass, type, bgStyle };
    }

    renderWorkerSVG(status: string, type: string, stamina: number) {
        // Return cute custom inline SVGs representing pixel art assets
        let handsClass = '';
        let headClass = '';
        let shirtColor = '#3c4268';

        if (status === 'running' && stamina > 0) {
            handsClass = 'char-hands-typing';
            headClass = 'char-head-bob';
        } else if (status === 'pending' || stamina === 0) {
            headClass = 'char-head-bob';
        }

        if (type === 'hardware') {
            shirtColor = '#d97b4a';
        } else if (type === 'media') {
            shirtColor = '#9b7bd4';
        } else if (type === 'research') {
            shirtColor = '#6fa84a';
        }

        return (
            <svg class="pixel-char-canvas" viewBox="0 0 24 24" width="48" height="48" style="image-rendering:pixelated">
                {/* Desk/Table */}
                <rect x="2" y="18" width="20" height="2" fill="#c79a5e" />
                <rect x="4" y="20" width="2" height="4" fill="#a07840" />
                <rect x="18" y="20" width="2" height="4" fill="#a07840" />

                {/* Chair */}
                <rect x="7" y="14" width="10" height="2" fill="#d8b176" />
                <rect x="11" y="16" width="2" height="4" fill="#c79a5e" />

                {/* Body / Shirt */}
                <g class={headClass}>
                    <rect x="9" y="10" width="6" height="7" fill={shirtColor} />
                    <rect x="10" y="4" width="4" height="6" fill="#f4c890" /> {/* Head */}
                    <rect x="9" y="3" width="6" height="2" fill="#7a5a3c" /> {/* Hair */}
                    <rect x="8" y="5" width="2" height="3" fill="#7a5a3c" />
                </g>

                {/* Eyes */}
                {status === 'failed' ? (
                    // Closed eyes / sleeping
                    <g class={headClass}>
                        <rect x="11" y="6" width="2" height="1" fill="#7a5a3c" />
                    </g>
                ) : (
                    <g class={headClass}>
                        <rect x="10" y="6" width="1" height="1" fill="#5a3c20" />
                        <rect x="13" y="6" width="1" height="1" fill="#5a3c20" />
                    </g>
                )}

                {/* Hands typing */}
                {status === 'running' && stamina > 0 && (
                    <g class={handsClass}>
                        <rect x="8" y="14" width="2" height="2" fill="#f4c890" />
                        <rect x="14" y="14" width="2" height="2" fill="#f4c890" />
                    </g>
                )}

                {/* Computer Screen */}
                <rect x="14" y="9" width="8" height="7" fill="#d8c8a8" />
                <rect x="15" y="10" width="6" height="5" fill={status === 'running' ? '#6fa84a' : '#cdb892'} />
                <rect x="17" y="16" width="2" height="2" fill="#cdb892" />
                <rect x="15" y="18" width="6" height="1" fill="#cdb892" />
            </svg>
        );
    }

    render() {
        const { workspace, tasks, onClick, onHover, cohort, effortLevel } = this.props;
        const { logs, coins } = this.state;

        const status = this.getProjectStatus();
        const { dept, displayName, themeClass, type } = this.getDepartmentInfo();

        const completedTasks = tasks.filter(t => t.status === 'completed').length;
        const totalTasks = tasks.length;

        let progressPercent = 0;
        if (totalTasks > 0) {
            const basePercent = (completedTasks / totalTasks) * 100;
            const activeTask = tasks.find(t => t.status === 'running');
            const activeContribution =
                activeTask && (activeTask as GamifiedTask).progress
                    ? (activeTask as GamifiedTask).progress! / totalTasks
                    : 0;
            progressPercent = Math.min(100, Math.round(basePercent + activeContribution));
        }

        // Segmented bar has 10 chunks
        const chunks = Array.from({ length: 10 });
        const filledChunks = Math.round(progressPercent / 10);

        // Compute average cohort stamina
        const averageStamina =
            cohort.length === 0 ? 0 : Math.round(cohort.reduce((acc, curr) => acc + curr.stamina, 0) / cohort.length);

        // Cohort Names list
        const cohortNames = cohort.map(c => c.name.split(' ')[0]).join(' + ') || '无智能体';
        const activeAgent = cohort.map(c => c.name).join(', ') || '无智能体';

        return (
            <div
                class={`pixel-workbench status-${status} ${themeClass}`}
                onClick={onClick}
                onMouseEnter={e =>
                    onHover(e, true, { workspace, tasks, status, progressPercent, dept, displayName, activeAgent })
                }
                onMouseMove={e =>
                    onHover(e, true, { workspace, tasks, status, progressPercent, dept, displayName, activeAgent })
                }
                onMouseLeave={e => onHover(e, false, null)}
            >
                {/* Floating Stats Point Animations */}
                {coins.map(c => (
                    <span
                        key={c.id}
                        class="pixel-floating-point"
                        style={`left: ${c.x}px; bottom: ${c.y}px; color: ${c.color === 'gold' ? 'var(--pixel-gold)' : 'var(--pixel-cyan)'}`}
                    >
                        {c.text}
                    </span>
                ))}

                {/* Top Title Bar */}
                <div class="pixel-wb-top">
                    <div class="pixel-wb-info">
                        <span class="pixel-wb-name">{displayName}</span>
                        <span class="pixel-wb-desc">{dept}</span>
                    </div>
                    <div class="pixel-wb-status-wrapper">
                        {/* Effort Level LED */}
                        <div
                            class={`pixel-effort-led effort-${effortLevel}`}
                            title={`当前投入度: ${effortLevel === 'low' ? '低 (LOW)' : effortLevel === 'middle' ? '中 (MID)' : '高 (HIGH)'}`}
                        >
                            <span class="pixel-led-dot" />
                        </div>
                        <span class={`pixel-wb-status-badge pixel-badge-${status}`}>
                            {status === 'pending' && (averageStamina === 0 ? '休眠中' : '策划中')}
                            {status === 'running' && (averageStamina === 0 ? '休眠中' : '进行中')}
                            {status === 'completed' && '已完成'}
                            {status === 'blocked' && '警告'}
                            {status === 'failed' && (averageStamina === 0 ? '休眠中' : '故障')}
                        </span>
                    </div>
                </div>

                {/* Middle Worker/R&D Deck */}
                <div class="pixel-wb-middle">
                    <div class="pixel-worker-sprite">
                        <div class="pixel-status-ring" />
                        {this.renderWorkerSVG(status, type, averageStamina)}
                        {averageStamina === 0 && <div class="pixel-zzz-bubble">Zzz...</div>}
                    </div>
                    <div class="pixel-wb-details">
                        <div
                            class="pixel-employee-hud"
                            title={`协同智能体梯队: ${cohort.map(c => `${c.name} (${c.modelType})`).join(', ')}`}
                        >
                            <span
                                class="pixel-agent-label"
                                style="max-width: 130px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;"
                            >
                                👥 {cohortNames}
                            </span>
                            {/* Stamina battery */}
                            <div class="pixel-stamina-battery" title={`梯队平均算力余额: ${averageStamina}%`}>
                                <div
                                    class={`pixel-stamina-fill ${averageStamina < 30 ? 'low' : averageStamina < 60 ? 'medium' : 'high'}`}
                                    style={`width: ${averageStamina}%`}
                                />
                            </div>
                        </div>

                        {/* Active skills snapshot */}
                        <div
                            class="pixel-active-skills"
                            title={`搭载技能: ${cohort.flatMap(c => c.skills).join(', ')}`}
                        >
                            {cohort.slice(0, 2).map((c, idx) => (
                                <span key={idx} class="pixel-skill-tag">
                                    {c.skills[0] ? c.skills[0].split(' ')[0] : '常规技能'}
                                </span>
                            ))}
                            {cohort.flatMap(c => c.skills).length > 2 && (
                                <span class="pixel-skill-tag">+ {cohort.flatMap(c => c.skills).length - 2}</span>
                            )}
                        </div>

                        {/* Progress telemetry */}
                        <div class="pixel-progress-container">
                            <div class="pixel-progress-text">
                                <span>进度</span>
                                <span>{progressPercent}%</span>
                            </div>
                            <div class="pixel-progress-bar">
                                {chunks.map((_, idx) => (
                                    <div
                                        key={idx}
                                        class={`pixel-progress-chunk ${idx < filledChunks ? 'filled' : ''}`}
                                    />
                                ))}
                            </div>
                        </div>
                    </div>

                    {/* Active Blocked Bug Graphics */}
                    {status === 'blocked' && (
                        <svg class="pixel-bug-sprite" viewBox="0 0 16 16" width="24" height="24">
                            <rect x="5" y="4" width="6" height="8" fill="#ef7d1a" />
                            <rect x="4" y="6" width="8" height="4" fill="#ef7d1a" />
                            <rect x="3" y="5" width="10" height="1" fill="#ff3e3e" /> {/* antenna */}
                            <rect x="5" y="5" width="2" height="2" fill="#000" /> {/* eyes */}
                            <rect x="9" y="5" width="2" height="2" fill="#000" />
                            {/* legs */}
                            <rect x="2" y="7" width="2" height="1" fill="#ef7d1a" />
                            <rect x="12" y="7" width="2" height="1" fill="#ef7d1a" />
                            <rect x="2" y="9" width="2" height="1" fill="#ef7d1a" />
                            <rect x="12" y="9" width="2" height="1" fill="#ef7d1a" />
                        </svg>
                    )}

                    {/* Active Fail Fire Graphics */}
                    {status === 'failed' && (
                        <svg class="pixel-fire-sprite" viewBox="0 0 16 16" width="32" height="32">
                            <path
                                d="M8,1 C6,3 4,6 4,8 C4,10 6,12 8,12 C10,12 12,10 12,8 C12,6 10,3 8,1 Z"
                                fill="#ff3e3e"
                            />
                            <path d="M8,4 C7,5 6,7 6,8 C6,9 7,10 8,10 C9,10 10,9 10,8 C10,7 9,5 8,4 Z" fill="#ef7d1a" />
                            <path
                                d="M8,6 C7.5,6.5 7,7.5 7,8 C7,8.5 7.5,9 8,9 C8.5,9 9,8.5 9,8 C9,7.5 8.5,6.5 8,6 Z"
                                fill="#f4b41b"
                            />
                        </svg>
                    )}
                </div>

                {/* Console Log Panel */}
                <div class="pixel-wb-console">
                    {logs.map((line, idx) => (
                        <div key={idx} class="pixel-console-line">
                            {line}
                        </div>
                    ))}
                </div>
            </div>
        );
    }
}
