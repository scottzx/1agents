import { Fragment, h } from 'preact';
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { renderMarkdown } from '../../utils/markdown';
import { roleLabel } from './roleLabels';
import {
    analysisStatus,
    analysisStatusLabel,
    conclusionPreview,
    panelistSeats,
    progressForRound,
    responseTarget,
    speechForSeat,
    splitFinalSummary,
    stanceSignals,
    summaryForRound,
} from './workbench';

interface StageWorkbenchProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
    turns: RoundtableTurn[];
    onOpenSeat?: (seat: RoundtableSeat) => void | Promise<void>;
}

export function StageWorkbench(props: StageWorkbenchProps) {
    if (props.room.phase === 'done') return <DoneWorkbench {...props} />;
    if (props.room.phase === 'r3') return <RoundWorkbench {...props} round={3} />;
    return <RoundWorkbench {...props} round={2} />;
}

function RoundWorkbench({ room, seats, turns, round, onOpenSeat }: StageWorkbenchProps & { round: 2 | 3 }) {
    const summary = summaryForRound(turns, room, round);
    const priorSummary = round === 3 ? summaryForRound(turns, room, 2) : '';
    const panelists = panelistSeats(seats);
    const progress = progressForRound(room, round);

    return (
        <section class={`rt-stage-workbench is-r${round}`} aria-labelledby={`rt-r${round}-title`}>
            <header class="rt-workbench-intro">
                <div>
                    <p class="rt-workbench-kicker">{round === 2 ? 'R2 · 独立分析' : 'R3 · 交叉回应'}</p>
                    <h2 id={`rt-r${round}-title`} class="rt-workbench-title">
                        {round === 2 ? '比较五席的独立判断' : '查看观点如何被保留、修正或反驳'}
                    </h2>
                    <p class="rt-workbench-desc">
                        {round === 2
                            ? '每个职能只基于已确认议题独立分析，完成后由裁判形成首轮总结。'
                            : '每个职能回应首轮公开观点，并明确立场变化与新增证据。'}
                    </p>
                </div>
                <div class="rt-progress-card" aria-label={`${progress.completed} / ${progress.total} 个席位已完成`}>
                    <strong>
                        {progress.completed}/{progress.total}
                    </strong>
                    <span>席位已完成</span>
                    <span class="rt-progress-track" aria-hidden="true">
                        <span
                            style={{
                                width: `${progress.total ? (progress.completed / progress.total) * 100 : 0}%`,
                            }}
                        />
                    </span>
                </div>
            </header>

            {priorSummary && (
                <details class="rt-round-baseline">
                    <summary>查看 R2 首轮总结（本轮回应基线）</summary>
                    <SummaryArtifact round={2} content={priorSummary} label="首轮总结" />
                </details>
            )}

            {round === 3 && (
                <div class="rt-stance-legend" aria-label="立场变化图例">
                    <span>立场变化</span>
                    <span>保留</span>
                    <span>修正</span>
                    <span>反驳</span>
                    <span>新增证据</span>
                </div>
            )}

            <div class="rt-analysis-grid" role="list" aria-label={`${round === 2 ? '独立观点' : '交叉回应'}`}>
                {panelists.map(seat => (
                    <AnalysisCard
                        key={seat.id}
                        room={room}
                        seat={seat}
                        turn={speechForSeat(turns, seat, round)}
                        round={round}
                        onOpenSeat={onOpenSeat}
                    />
                ))}
            </div>

            {summary && <SummaryArtifact round={round} content={summary} label={round === 2 ? '首轮总结' : '终稿'} />}
        </section>
    );
}

function AnalysisCard({
    room,
    seat,
    turn,
    round,
    onOpenSeat,
}: {
    room: RoundtableRoom;
    seat: RoundtableSeat;
    turn?: RoundtableTurn;
    round: 2 | 3;
    onOpenSeat?: (seat: RoundtableSeat) => void | Promise<void>;
}) {
    const status = analysisStatus(room, seat, round, turn);
    const content = (turn?.content_text || '').trim();
    const preview = conclusionPreview(content);
    const signals = round === 3 ? stanceSignals(content) : [];

    return (
        <article class={`rt-analysis-card is-${status}`} role="listitem">
            <header class="rt-analysis-card-head">
                <h3>{roleLabel(seat.role)}</h3>
                <span class={`rt-analysis-status is-${status}`}>
                    <span class="rt-analysis-status-icon" aria-hidden="true">
                        {status === 'done' ? '✓' : status === 'failed' ? '!' : status === 'running' ? '↻' : '○'}
                    </span>
                    {analysisStatusLabel(status, round)}
                </span>
            </header>

            {round === 3 && (
                <Fragment>
                    <div class="rt-stance-signals" aria-label={`${roleLabel(seat.role)}立场变化`}>
                        {signals.map(signal => (
                            <span key={signal.label} class={signal.active ? 'is-active' : 'is-muted'}>
                                {signal.label}
                            </span>
                        ))}
                    </div>
                    <p class="rt-response-target">
                        <span>回应对象</span>
                        {responseTarget(content)}
                    </p>
                </Fragment>
            )}

            <div class="rt-analysis-preview">
                <span>结论预览</span>
                <p>{preview || analysisPlaceholder(status, round)}</p>
            </div>

            {content && status !== 'failed' && (
                <details class="rt-analysis-body">
                    <summary>查看完整正文</summary>
                    <MarkdownBody content={content} />
                </details>
            )}

            {turn?.process_ref && (
                <details class="rt-analysis-process">
                    <summary>查看分析过程</summary>
                    <p>分析过程按需查看，不会混入观点正文。</p>
                    {onOpenSeat && seat.session_id && (
                        <button type="button" class="rt-btn rt-btn-ghost" onClick={() => void onOpenSeat(seat)}>
                            打开该席过程
                        </button>
                    )}
                </details>
            )}
        </article>
    );
}

function SummaryArtifact({ round, content, label }: { round: 2 | 3; content: string; label: string }) {
    return (
        <article class="rt-summary-artifact" data-summary-round={round} aria-labelledby={`rt-summary-${round}-title`}>
            <header>
                <p>Summary{round === 2 ? '₂' : '₃'}</p>
                <h2 id={`rt-summary-${round}-title`}>{label}</h2>
            </header>
            <MarkdownBody content={content} />
        </article>
    );
}

function DoneWorkbench(props: StageWorkbenchProps) {
    const summary = summaryForRound(props.turns, props.room, 3);
    const sections = splitFinalSummary(summary);

    return (
        <section class="rt-stage-workbench is-done" aria-labelledby="rt-done-title">
            <header class="rt-workbench-intro">
                <div>
                    <p class="rt-workbench-kicker">Done · 已收敛</p>
                    <h2 id="rt-done-title" class="rt-workbench-title">
                        最终结论与执行重点
                    </h2>
                    <p class="rt-workbench-desc">先看建议、取舍、行动和风险；两轮讨论证据保留在下方。</p>
                </div>
            </header>

            <div id="rt-summary-3-title" class="rt-final-grid">
                {sections.map((section, index) => (
                    <article
                        key={section.id}
                        class={`rt-final-section is-${section.id}${index === 0 ? ' is-primary' : ''}`}
                    >
                        <h3>{section.label}</h3>
                        {section.content ? (
                            <MarkdownBody content={section.content} />
                        ) : (
                            <p class="rt-final-empty">终稿未单列此项，请结合最终建议确认。</p>
                        )}
                    </article>
                ))}
            </div>

            <details class="rt-history">
                <summary>查看 R2 / R3 历史轮次</summary>
                <div class="rt-history-rounds">
                    <HistoryRound {...props} round={2} />
                    <HistoryRound {...props} round={3} />
                </div>
            </details>
        </section>
    );
}

function HistoryRound({ room, seats, turns, round, onOpenSeat }: StageWorkbenchProps & { round: 2 | 3 }) {
    const panelists = panelistSeats(seats);
    const summary = round === 2 ? summaryForRound(turns, room, 2) : '';
    return (
        <section class="rt-history-round">
            <h3>{round === 2 ? 'R2 · 独立观点与首轮总结' : 'R3 · 立场变化与交叉回应'}</h3>
            <div class="rt-analysis-grid">
                {panelists.map(seat => (
                    <AnalysisCard
                        key={seat.id}
                        room={room}
                        seat={seat}
                        turn={speechForSeat(turns, seat, round)}
                        round={round}
                        onOpenSeat={onOpenSeat}
                    />
                ))}
            </div>
            {summary && <SummaryArtifact round={2} content={summary} label="首轮总结" />}
        </section>
    );
}

function MarkdownBody({ content }: { content: string }) {
    return (
        <div
            class="rt-artifact-body markdown-body md-conv"
            dangerouslySetInnerHTML={{ __html: renderMarkdown(content) }}
        />
    );
}

function analysisPlaceholder(status: ReturnType<typeof analysisStatus>, round: 2 | 3): string {
    if (status === 'failed') return '该席本轮未完成，请在参与者 Inspector 查看状态。';
    if (status === 'running') return round === 2 ? '正在形成独立判断…' : '正在回应首轮观点…';
    return round === 2 ? '尚未开始独立分析。' : '尚未开始交叉回应。';
}
