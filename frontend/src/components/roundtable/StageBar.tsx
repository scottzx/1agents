import { h } from 'preact';
import type { RoomState } from '@1agents/core/services/roundtableService';
import { STAGES, stageIdFromState, stageIndexFromState, type StageId } from './stage';

interface StageBarProps {
    state: RoomState | string | undefined;
    currentStage?: StageId;
    selectedStage?: StageId;
    onStepClick?: (stepId: StageId) => void;
}

/**
 * Global workflow progress. Completed/current stages can be viewed without
 * changing the room's real state; future stages stay disabled.
 */
export function StageBar({
    state,
    currentStage = stageIdFromState(state),
    selectedStage = currentStage,
    onStepClick,
}: StageBarProps) {
    const stateIndex = stageIndexFromState(state);
    const active = stateIndex >= 0 ? stateIndex : STAGES.findIndex(stage => stage.id === currentStage);
    const failed = state === 'failed';

    return (
        <nav class="rt-stage-bar" aria-label="圆桌进度">
            <ol class="rt-stage-steps" role="list">
                {STAGES.map((s, i) => {
                    let cls = 'rt-stage-step';
                    if (active === i) {
                        cls += ' is-active';
                    } else if (active > i) {
                        cls += ' is-done';
                    } else {
                        cls += ' is-todo';
                    }
                    if (failed && active === i) cls += ' is-failed';
                    if (selectedStage === s.id) cls += ' is-selected';

                    const isAvailable = i <= active;
                    const isCurrent = active === i;
                    const isDone = active > i;
                    const stateText =
                        selectedStage === s.id && !isCurrent
                            ? '正在查看'
                            : isCurrent
                              ? failed
                                  ? '阶段异常'
                                  : '当前阶段'
                              : isDone
                                ? '已完成'
                                : '未开始';

                    return (
                        <li key={s.id} class={cls}>
                            <button
                                type="button"
                                class="rt-stage-button"
                                disabled={!isAvailable}
                                aria-current={isCurrent ? 'step' : undefined}
                                aria-pressed={selectedStage === s.id}
                                aria-label={`第 ${i + 1} 步：${s.label}，${stateText}`}
                                onClick={() => onStepClick?.(s.id)}
                            >
                                <span class="rt-stage-dot" aria-hidden="true">
                                    {isDone ? '✓' : i + 1}
                                </span>
                                <span class="rt-stage-copy">
                                    <span class="rt-stage-label">{s.label}</span>
                                    <span class="rt-stage-state">{stateText}</span>
                                </span>
                            </button>
                            {i < STAGES.length - 1 && (
                                <span
                                    class={`rt-stage-connector${active > i ? ' is-complete' : ''}`}
                                    aria-hidden="true"
                                />
                            )}
                        </li>
                    );
                })}
            </ol>
        </nav>
    );
}
