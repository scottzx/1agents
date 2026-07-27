import type {
    RoundtableRoom,
    RoundtableSeat,
    RoundtableTurn,
    SeatRole,
} from '@1agents/core/services/roundtableService';
import { roleLabel } from './roleLabels';

export const PANELIST_ROLES: SeatRole[] = ['market', 'product', 'eng', 'ops', 'finance'];

export type AnalysisStatus = 'ready' | 'running' | 'done' | 'failed';

export interface FinalSection {
    id: 'recommendation' | 'tradeoffs' | 'actions' | 'risks';
    label: string;
    content: string;
}

export function progressForRound(
    room: RoundtableRoom,
    round: 2 | 3
): { completed: number; total: number; activeRoles: SeatRole[]; failedRoles: SeatRole[] } {
    const total = room.progress.total || PANELIST_ROLES.length;
    if (room.active_run && room.active_run.round !== round) {
        return { completed: 0, total, activeRoles: [], failedRoles: [] };
    }
    return {
        completed: Math.min(room.progress.completed || 0, total),
        total,
        activeRoles: room.progress.active_roles,
        failedRoles: room.progress.failed_roles,
    };
}

export function panelistSeats(seats: RoundtableSeat[]): RoundtableSeat[] {
    return PANELIST_ROLES.map(role => seats.find(seat => seat.role === role)).filter((seat): seat is RoundtableSeat =>
        Boolean(seat)
    );
}

export function speechForSeat(turns: RoundtableTurn[], seat: RoundtableSeat, round: 2 | 3): RoundtableTurn | undefined {
    return [...turns]
        .reverse()
        .find(turn => turn.round === round && turn.kind === 'speech' && turn.seat_id === seat.id);
}

export function summaryForRound(turns: RoundtableTurn[], room: RoundtableRoom, round: 2 | 3): string {
    const turn = turns.find(item => item.round === round && item.kind === 'summary');
    return (turn?.content_text || (round === 2 ? room.summary_r2 : room.summary_r3) || '').trim();
}

export function analysisStatus(
    room: RoundtableRoom,
    seat: RoundtableSeat,
    round: 2 | 3,
    turn?: RoundtableTurn
): AnalysisStatus {
    const progress = progressForRound(room, round);
    if (
        progress.failedRoles.includes(seat.role) ||
        seat.status === 'failed' ||
        seat.status === 'skipped' ||
        turn?.content_text.trim().startsWith('[failed]')
    ) {
        return 'failed';
    }
    if (progress.activeRoles.includes(seat.role) || seat.status === 'speaking') return 'running';
    if (turn?.content_text.trim()) return 'done';
    return 'ready';
}

export function analysisStatusLabel(status: AnalysisStatus, round: 2 | 3): string {
    switch (status) {
        case 'running':
            return round === 2 ? '分析中' : '回应中';
        case 'done':
            return '已完成';
        case 'failed':
            return '需要处理';
        default:
            return round === 2 ? '等待分析' : '等待回应';
    }
}

export function conclusionPreview(content: string): string {
    const line = content
        .split('\n')
        .map(item =>
            item
                .replace(/^\s{0,3}(?:#{1,6}\s+|[-*+]\s+|\d+[.)]\s+)/u, '')
                .replace(/\*\*|__|`/gu, '')
                .trim()
        )
        .find(Boolean);
    if (!line) return '';
    return line.length > 108 ? `${line.slice(0, 108).trim()}…` : line;
}

const STANCE_PATTERNS = [
    { label: '保留', pattern: /保留|仍然认为|继续支持|维持/iu },
    { label: '修正', pattern: /修正|调整|改为|重新判断|不再/iu },
    { label: '反驳', pattern: /反驳|不同意|不能接受|不成立|相反/iu },
    { label: '新增证据', pattern: /新增证据|新证据|数据显示|研究显示|事实表明|验证结果/iu },
] as const;

export function stanceSignals(content: string): { label: string; active: boolean }[] {
    return STANCE_PATTERNS.map(item => ({ label: item.label, active: item.pattern.test(content) }));
}

export function responseTarget(content: string): string {
    const roleNames = ['市场', '产品', '研发', '运营', '财务', '裁判'];
    const target = roleNames.find(name =>
        new RegExp(`(?:回应|针对|同意|反驳)[^\\n。]{0,28}${name}`, 'u').test(content)
    );
    if (target) return `${target}席上一轮观点`;
    if (/Summary[₂2]|首轮总结/iu.test(content)) return '首轮总结';
    return '首轮公开观点';
}

const FINAL_SECTION_DEFS: Omit<FinalSection, 'content'>[] = [
    { id: 'recommendation', label: '最终建议' },
    { id: 'tradeoffs', label: '关键取舍' },
    { id: 'actions', label: '行动项与负责职能' },
    { id: 'risks', label: '未决风险' },
];

function finalSectionId(title: string): FinalSection['id'] | null {
    if (/最终建议|最终判断|最终结论|建议|recommendation|decision/iu.test(title)) return 'recommendation';
    if (/关键取舍|取舍与条件|取舍|trade-?offs?/iu.test(title)) return 'tradeoffs';
    if (/行动项|下一步|执行计划|action items?/iu.test(title)) return 'actions';
    if (/未决风险|风险|待确认|open risks?/iu.test(title)) return 'risks';
    return null;
}

/** Split Summary₃ without rendering a second copy of its complete body. */
export function splitFinalSummary(summary: string): FinalSection[] {
    const buckets: Record<FinalSection['id'], string[]> = {
        recommendation: [],
        tradeoffs: [],
        actions: [],
        risks: [],
    };
    let current: FinalSection['id'] = 'recommendation';

    for (const line of summary.trim().split('\n')) {
        const heading = line.match(/^\s{0,3}#{1,6}\s+(.+?)\s*$/u) || line.match(/^\s*\*\*(.+?)\*\*\s*[:：]?\s*$/u);
        const next = heading ? finalSectionId(heading[1]) : null;
        if (next) {
            current = next;
            continue;
        }
        buckets[current].push(line);
    }

    return FINAL_SECTION_DEFS.map(section => ({
        ...section,
        content: buckets[section.id].join('\n').trim(),
    }));
}

export function progressText(room: RoundtableRoom): string {
    if (room.phase === 'r1') return '等待确认议题';
    if (room.phase === 'done') return '讨论已完成';
    const progress = progressForRound(room, room.phase === 'r3' ? 3 : 2);
    const active = progress.activeRoles.map(roleLabel);
    if (active.length > 0) {
        return `${progress.completed}/${progress.total} 已完成 · ${active.join('、')}进行中`;
    }
    if (room.phase_status === 'summarizing') {
        return `${progress.completed}/${progress.total} 已完成 · 正在形成总结`;
    }
    return `${progress.completed}/${progress.total} 已完成`;
}
