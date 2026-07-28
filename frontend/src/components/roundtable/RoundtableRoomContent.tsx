import { h, Fragment, type ComponentChildren, type RefObject } from 'preact';
import type { RoundtableRoom, RoundtableSeat, RoundtableTurn } from '@1agents/core/services/roundtableService';
import { TurnCard } from './TurnCard';
import { timelineTurnsWithoutR1Chat } from './r1Timeline';
import { StageBar } from './StageBar';
import type { StageId } from './stage';

interface RoundtableRoomContentProps {
    room: RoundtableRoom;
    seats: RoundtableSeat[];
    turns: RoundtableTurn[];
    header: ComponentChildren;
    sidebar: ComponentChildren;
    notice?: ComponentChildren;
    timelineRef?: RefObject<HTMLDivElement>;
    emptyMessage?: string;
    primaryContent?: ComponentChildren;
    mobilePane?: RoundtableMobilePane;
    onMobilePaneChange?: (pane: RoundtableMobilePane) => void;
    selectedStage: StageId;
    onStageChange: (stageId: StageId) => void;
}

export type RoundtableMobilePane = 'discussion' | 'participants';

const MOBILE_PANES: RoundtableMobilePane[] = ['discussion', 'participants'];

export function mobilePaneForKey(current: RoundtableMobilePane, key: string): RoundtableMobilePane | null {
    const index = MOBILE_PANES.indexOf(current);
    if (key === 'Home') return MOBILE_PANES[0];
    if (key === 'End') return MOBILE_PANES[MOBILE_PANES.length - 1];
    if (key === 'ArrowLeft') return MOBILE_PANES[(index - 1 + MOBILE_PANES.length) % MOBILE_PANES.length];
    if (key === 'ArrowRight') return MOBILE_PANES[(index + 1) % MOBILE_PANES.length];
    return null;
}

/** Loaded room composition. Kept presentational so duplicate content is component-testable. */
export function RoundtableRoomContent({
    room,
    seats,
    turns,
    header,
    sidebar,
    notice,
    timelineRef,
    emptyMessage = '尚无发言。',
    primaryContent,
    mobilePane = 'discussion',
    onMobilePaneChange,
    selectedStage,
    onStageChange,
}: RoundtableRoomContentProps) {
    const timelineTurns = timelineTurnsWithoutR1Chat(turns);
    const selectMobilePane = (pane: RoundtableMobilePane) => {
        onMobilePaneChange?.(pane);
    };

    const handleStepClick = (stepId: StageId) => {
        onStageChange(stepId);
        onMobilePaneChange?.('discussion');
    };
    const onMobileTabKeyDown = (event: KeyboardEvent, pane: RoundtableMobilePane) => {
        const next = mobilePaneForKey(pane, event.key);
        if (!next) return;
        event.preventDefault();
        selectMobilePane(next);
        queueMicrotask(() => document.getElementById(`rt-mobile-tab-${next}`)?.focus());
    };

    return (
        <div class="rt-room" data-room-id={room.id} data-mobile-pane={mobilePane}>
            <div class="rt-room-main">
                {header}
                <StageBar state={room.state} selectedStage={selectedStage} onStepClick={handleStepClick} />
                <div class="rt-mobile-tabs" role="tablist" aria-label="圆桌视图">
                    {MOBILE_PANES.map(pane => (
                        <button
                            key={pane}
                            type="button"
                            role="tab"
                            id={`rt-mobile-tab-${pane}`}
                            aria-selected={mobilePane === pane}
                            aria-controls={
                                pane === 'discussion' ? 'rt-mobile-panel-discussion' : 'rt-mobile-panel-inspector'
                            }
                            tabIndex={mobilePane === pane ? 0 : -1}
                            class={mobilePane === pane ? 'is-active' : ''}
                            onClick={() => selectMobilePane(pane)}
                            onKeyDown={event => onMobileTabKeyDown(event, pane)}
                        >
                            {pane === 'discussion' ? '阶段内容' : '参与者'}
                        </button>
                    ))}
                </div>
                {notice && <div class="rt-room-notice">{notice}</div>}
                {primaryContent ? (
                    <div
                        id="rt-mobile-panel-discussion"
                        class="rt-primary-content"
                        role="tabpanel"
                        aria-labelledby="rt-mobile-tab-discussion"
                    >
                        {primaryContent}
                    </div>
                ) : (
                    <div
                        id="rt-mobile-panel-discussion"
                        class="rt-timeline"
                        ref={timelineRef}
                        role="log"
                        aria-live="off"
                        aria-label="主时间线"
                        aria-labelledby="rt-mobile-tab-discussion"
                    >
                        {timelineTurns.length === 0 ? (
                            <div class="rt-timeline-empty">{emptyMessage}</div>
                        ) : (
                            <Fragment>
                                {timelineTurns.map(turn => (
                                    <TurnCard key={turn.id} turn={turn} seats={seats} />
                                ))}
                            </Fragment>
                        )}
                    </div>
                )}
            </div>

            <div
                id="rt-mobile-panel-inspector"
                class="rt-mobile-inspector-panel"
                role="tabpanel"
                aria-labelledby="rt-mobile-tab-participants"
            >
                {sidebar}
            </div>
        </div>
    );
}
