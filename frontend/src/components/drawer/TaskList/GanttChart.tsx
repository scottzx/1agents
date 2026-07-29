import { h } from 'preact';
import { useCallback, useMemo, useState } from 'preact/hooks';
import type { GanttData } from '@1agents/core/types/featureCatalog';
import * as taskNav from '../../../stores/taskNavStore';
import {
    computeTimeAxis,
    dateToX,
    flattenGanttRows,
    parseDate,
    type GanttRow,
    type TimeGranularity,
} from './ganttModel';

interface GanttChartProps {
    workspaceId: string;
    data: GanttData;
}

const ROW_HEIGHT = 32;
const CHART_WIDTH = 1200; // default chart width for SVG

export function GanttChart({ workspaceId, data }: GanttChartProps) {
    const [collapsed, setCollapsed] = useState<Set<string>>(new Set());
    const [granularity, setGranularity] = useState<TimeGranularity>('week');
    const [hoveredTask, setHoveredTask] = useState<GanttRow | null>(null);

    const toggleCollapse = useCallback((moduleId: string) => {
        setCollapsed(prev => {
            const next = new Set(prev);
            if (next.has(moduleId)) {
                next.delete(moduleId);
            } else {
                next.add(moduleId);
            }
            return next;
        });
    }, []);

    const rows = useMemo(() => flattenGanttRows(data.modules, collapsed), [data.modules, collapsed]);

    const axis = useMemo(() => computeTimeAxis(rows, granularity), [rows, granularity]);

    const handleTaskClick = useCallback(
        (taskId: string) => {
            taskNav.openTaskById(workspaceId, taskId);
        },
        [workspaceId]
    );

    if (!axis) {
        return <div class="gantt-chart-empty">暂无日程安排数据</div>;
    }

    const rowPositions = new Map<string, number>();
    rows.forEach((row, i) => {
        rowPositions.set(row.id, i * ROW_HEIGHT);
    });

    return (
        <div class="gantt-chart">
            <div class="gantt-toolbar">
                <div class="gantt-granularity-toggle">
                    <button class={granularity === 'day' ? 'active' : ''} onClick={() => setGranularity('day')}>
                        日
                    </button>
                    <button class={granularity === 'week' ? 'active' : ''} onClick={() => setGranularity('week')}>
                        周
                    </button>
                    <button class={granularity === 'month' ? 'active' : ''} onClick={() => setGranularity('month')}>
                        月
                    </button>
                </div>
            </div>

            <div class="gantt-container">
                <div class="gantt-labels">
                    <div class="gantt-label-header">任务 / 模块</div>
                    {rows.map(row => (
                        <div
                            key={row.id}
                            class={`gantt-label-row ${row.type}`}
                            style={`padding-left: ${row.depth * 16 + 8}px; height: ${ROW_HEIGHT}px;`}
                            onClick={() => (row.type === 'module' ? toggleCollapse(row.id) : handleTaskClick(row.id))}
                            title={row.title}
                        >
                            {row.type === 'module' && (
                                <span class="gantt-collapse-icon">{row.collapsed ? '▶' : '▼'}</span>
                            )}
                            <span class="gantt-label-text">{row.title}</span>
                        </div>
                    ))}
                </div>

                <div class="gantt-timeline-wrapper">
                    <svg class="gantt-timeline" width={CHART_WIDTH} height={rows.length * ROW_HEIGHT + ROW_HEIGHT}>
                        <g class="gantt-header-axis">
                            {axis.labels.map((label, i) => (
                                <g key={i}>
                                    <line
                                        x1={dateToX(label.date, axis, CHART_WIDTH)}
                                        y1={0}
                                        x2={dateToX(label.date, axis, CHART_WIDTH)}
                                        y2={rows.length * ROW_HEIGHT + ROW_HEIGHT}
                                        class="gantt-grid-line"
                                    />
                                    <text
                                        x={dateToX(label.date, axis, CHART_WIDTH) + 4}
                                        y={ROW_HEIGHT - 8}
                                        class="gantt-header-text"
                                    >
                                        {label.label}
                                    </text>
                                </g>
                            ))}
                        </g>

                        <g class="gantt-rows-layer" transform={`translate(0, ${ROW_HEIGHT})`}>
                            {rows.map((row, i) => {
                                const y = i * ROW_HEIGHT;
                                if (!row.start || !row.end) return null;

                                const x1 = dateToX(row.start, axis, CHART_WIDTH);
                                const x2 = dateToX(row.end, axis, CHART_WIDTH);
                                const width = Math.max(x2 - x1, 4); // min width

                                return (
                                    <g
                                        key={row.id}
                                        class={`gantt-row-group ${row.type}`}
                                        onMouseEnter={() => row.type === 'task' && setHoveredTask(row)}
                                        onMouseLeave={() => row.type === 'task' && setHoveredTask(null)}
                                        onClick={() => row.type === 'task' && handleTaskClick(row.id)}
                                    >
                                        <rect
                                            x={x1}
                                            y={y + 6}
                                            width={width}
                                            height={ROW_HEIGHT - 12}
                                            rx={row.type === 'module' ? 2 : 4}
                                            class={`gantt-bar-bg ${row.status || ''}`}
                                        />
                                        <rect
                                            x={x1}
                                            y={y + 6}
                                            width={width * (row.progress / 100)}
                                            height={ROW_HEIGHT - 12}
                                            rx={row.type === 'module' ? 2 : 4}
                                            class={`gantt-bar-progress ${row.status || ''}`}
                                        />
                                        {row.type === 'task' && (
                                            <text x={x2 + 8} y={y + ROW_HEIGHT / 2 + 4} class="gantt-bar-label">
                                                {row.title}
                                            </text>
                                        )}
                                    </g>
                                );
                            })}

                            {/* Dependencies Layer */}
                            {rows.map((row, i) => {
                                if (row.type !== 'task' || !row.dependsOn || row.dependsOn.length === 0) return null;
                                return row.dependsOn.map(depId => {
                                    const depRow = rows.find(r => r.id === depId);
                                    if (!depRow || !depRow.end || !row.start) return null;

                                    const depIndex = rows.findIndex(r => r.id === depId);
                                    const startX = dateToX(depRow.end, axis, CHART_WIDTH);
                                    const startY = depIndex * ROW_HEIGHT + ROW_HEIGHT / 2;
                                    const endX = dateToX(row.start, axis, CHART_WIDTH);
                                    const endY = i * ROW_HEIGHT + ROW_HEIGHT / 2;

                                    // simple path logic
                                    const pathD = `M ${startX} ${startY} C ${startX + 20} ${startY}, ${endX - 20} ${endY}, ${endX} ${endY}`;

                                    return (
                                        <path
                                            key={`${depId}-${row.id}`}
                                            d={pathD}
                                            class="gantt-dependency-line"
                                            markerEnd="url(#arrowhead)"
                                        />
                                    );
                                });
                            })}
                        </g>

                        <g class="gantt-milestones-layer" transform={`translate(0, ${ROW_HEIGHT})`}>
                            {data.milestones &&
                                data.milestones.map(m => {
                                    if (!m.targetDate) return null;
                                    const date = parseDate(m.targetDate);
                                    if (!date) return null;
                                    const mx = dateToX(date, axis, CHART_WIDTH);
                                    return (
                                        <g
                                            key={m.id}
                                            class="gantt-milestone"
                                            transform={`translate(${mx}, 16)`}
                                            title={m.version}
                                        >
                                            <polygon points="0,-8 8,0 0,8 -8,0" fill="var(--warning-fg)" />
                                            <text
                                                x="12"
                                                y="4"
                                                font-size="11"
                                                fill="var(--warning-fg)"
                                                font-weight="bold"
                                            >
                                                {m.version}
                                            </text>
                                            <line
                                                x1="0"
                                                y1="8"
                                                x2="0"
                                                y2={rows.length * ROW_HEIGHT}
                                                stroke="var(--warning-fg)"
                                                stroke-width="1"
                                                stroke-dasharray="4,4"
                                                opacity="0.3"
                                            />
                                        </g>
                                    );
                                })}
                        </g>

                        <defs>
                            <marker id="arrowhead" markerWidth="10" markerHeight="7" refX="9" refY="3.5" orient="auto">
                                <polygon points="0 0, 10 3.5, 0 7" fill="var(--color-border-strong, #888)" />
                            </marker>
                        </defs>
                    </svg>
                </div>
            </div>

            {data.unscheduled && data.unscheduled.length > 0 && (
                <div class="gantt-unscheduled">
                    <h4>未排期任务 ({data.unscheduled.length})</h4>
                    <div class="gantt-unscheduled-list">
                        {data.unscheduled.map(task => (
                            <div key={task.id} class="gantt-unscheduled-item" onClick={() => handleTaskClick(task.id)}>
                                <span>{`#${task.number} ${task.title}`}</span>
                                <span class="gantt-badge">{task.status}</span>
                            </div>
                        ))}
                    </div>
                </div>
            )}

            {hoveredTask && (
                <div class="gantt-tooltip">
                    <strong>{hoveredTask.title}</strong>
                    <div>状态: {hoveredTask.status}</div>
                    <div>进度: {hoveredTask.progress}%</div>
                    {hoveredTask.start && <div>开始: {hoveredTask.start.toLocaleDateString()}</div>}
                    {hoveredTask.end && <div>结束: {hoveredTask.end.toLocaleDateString()}</div>}
                    {hoveredTask.milestone && <div>里程碑: {hoveredTask.milestone}</div>}
                </div>
            )}
        </div>
    );
}
