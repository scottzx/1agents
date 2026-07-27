import { h } from 'preact';
import { useState } from 'preact/hooks';
import { ROLE_LABELS } from './roleLabels';
import type { SeatRole } from '@1agents/core/services/roundtableService';

/** Fixed roles shown as user-facing responsibilities, not implementation details. */
export const FIXED_ROSTER: { role: SeatRole; responsibility: string }[] = [
    { role: 'referee', responsibility: '澄清问题并综合判断' },
    { role: 'market', responsibility: '用户需求与市场机会' },
    { role: 'product', responsibility: '产品价值与体验取舍' },
    { role: 'eng', responsibility: '技术路径与交付风险' },
    { role: 'ops', responsibility: '落地运营与增长验证' },
    { role: 'finance', responsibility: '成本、收益与资金约束' },
];

export interface LaunchWizardProps {
    busy?: boolean;
    error?: string | null;
    /** Previous room id from localStorage — show “继续” when set. */
    previousRoomId?: string | null;
    defaultTitle?: string;
    onStart: (title: string) => void | Promise<void>;
    onContinue?: (roomId: string) => void;
}

/**
 * Startup wizard (design §6.3): optional topic draft → fixed 6-seat roster → Start.
 * Start creates a room and enters R1 (drafting_brief).
 */
export function LaunchWizard({
    busy,
    error,
    previousRoomId,
    defaultTitle = '',
    onStart,
    onContinue,
}: LaunchWizardProps) {
    const [title, setTitle] = useState(defaultTitle);

    return (
        <div class="rt-room rt-room-empty">
            <div class="rt-empty-card bento-card rt-wizard-card">
                <div class="bento-zone-header">
                    <div class="bento-card-title">新建圆桌</div>
                    <div class="bento-card-desc">
                        写下你真正要解决的问题。主持人会先帮你澄清，再邀请五个职能独立判断、交叉回应。
                    </div>
                </div>

                <div class="bento-zone-body rt-wizard-body">
                    <label class="rt-field">
                        <span class="rt-field-label">你希望圆桌解决什么问题？</span>
                        <input
                            class="rt-input"
                            type="text"
                            value={title}
                            placeholder="例如：新客服系统应该自研还是采购？"
                            disabled={busy}
                            onInput={e => setTitle((e.target as HTMLInputElement).value)}
                            onKeyDown={e => {
                                if (e.key === 'Enter' && !busy) {
                                    e.preventDefault();
                                    void onStart(title.trim());
                                }
                            }}
                        />
                    </label>

                    <div class="rt-wizard-roster">
                        <div class="rt-wizard-roster-label">这场讨论会从六个角度审视问题</div>
                        <ul class="rt-wizard-seats" aria-label="圆桌参与职能">
                            {FIXED_ROSTER.map(s => (
                                <li key={s.role} class="rt-wizard-seat">
                                    <span class="rt-wizard-seat-role">{ROLE_LABELS[s.role]}</span>
                                    <span class="rt-wizard-seat-agent">{s.responsibility}</span>
                                </li>
                            ))}
                        </ul>
                    </div>

                    {error && <div class="rt-error">{error}</div>}
                </div>

                <div class="bento-zone-footer rt-wizard-footer">
                    <button
                        type="button"
                        class="rt-btn rt-btn-primary"
                        disabled={busy}
                        onClick={() => void onStart(title.trim())}
                    >
                        {busy ? '正在召集…' : '带着问题开始'}
                    </button>
                    {previousRoomId && onContinue && (
                        <button
                            type="button"
                            class="rt-btn rt-btn-ghost"
                            disabled={busy}
                            onClick={() => onContinue(previousRoomId)}
                        >
                            回到上次讨论
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
