import { h } from 'preact';
import type { RoomState } from '@1agents/core/services/roundtableService';
import { STAGES, stageIndexFromState, stateLabel } from './stage';

interface StageBarProps {
    state: RoomState | string | undefined;
}

/**
 * Phase strip: R1 命题 · R2 首轮 · R3 次轮 · 终稿 (design §6.1).
 * Active step uses accent; completed steps use solid ink; future muted.
 */
export function StageBar({ state }: StageBarProps) {
    const active = stageIndexFromState(state);
    const failed = state === 'failed';

    return (
        <div class="rt-stage-bar" role="list" aria-label="圆桌阶段">
            <div class="rt-stage-bar-meta">
                <span class={`rt-stage-state${failed ? ' is-error' : ''}`}>{stateLabel(state)}</span>
            </div>
            <ol class="rt-stage-steps">
                {STAGES.map((s, i) => {
                    let cls = 'rt-stage-step';
                    if (failed) {
                        cls += ' is-failed';
                    } else if (active === i) {
                        cls += ' is-active';
                    } else if (active > i) {
                        cls += ' is-done';
                    } else {
                        cls += ' is-todo';
                    }
                    return (
                        <li key={s.id} class={cls} role="listitem">
                            <span class="rt-stage-dot" aria-hidden="true" />
                            <span class="rt-stage-label">{s.label}</span>
                            {i < STAGES.length - 1 && <span class="rt-stage-connector" aria-hidden="true" />}
                        </li>
                    );
                })}
            </ol>
        </div>
    );
}
