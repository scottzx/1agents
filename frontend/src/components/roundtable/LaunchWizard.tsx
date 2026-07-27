import { h } from 'preact';
import { useState } from 'preact/hooks';
import { ROLE_LABELS } from './roleLabels';
import type { SeatRole } from '@1agents/core/services/roundtableService';

/** Fixed MVP roster (design §3 / §6.3) — display only, not user-editable. */
export const FIXED_ROSTER: { role: SeatRole; harness: string }[] = [
    { role: 'referee', harness: 'Grok Build' },
    { role: 'market', harness: 'Grok Build' },
    { role: 'product', harness: 'Grok Build' },
    { role: 'eng', harness: 'Grok Build' },
    { role: 'ops', harness: 'Grok Build' },
    { role: 'finance', harness: 'Grok Build' },
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
                        真多 session 编排 · 裁判 + 五职能 · 固定 3 轮。开始后进入 R1 与裁判澄清议题。
                    </div>
                </div>

                <div class="bento-zone-body rt-wizard-body">
                    <label class="rt-field">
                        <span class="rt-field-label">议题草稿（可选）</span>
                        <input
                            class="rt-input"
                            type="text"
                            value={title}
                            placeholder="例如：是否自研 vs 外采"
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
                        <div class="rt-wizard-roster-label">固定编制（6 席）</div>
                        <ul class="rt-wizard-seats" aria-label="圆桌固定编制">
                            {FIXED_ROSTER.map(s => (
                                <li key={s.role} class="rt-wizard-seat">
                                    <span class="rt-wizard-seat-role">{ROLE_LABELS[s.role]}</span>
                                    <span class="rt-wizard-seat-agent">{s.harness}</span>
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
                        {busy ? '创建中…' : '开始'}
                    </button>
                    {previousRoomId && onContinue && (
                        <button
                            type="button"
                            class="rt-btn rt-btn-ghost"
                            disabled={busy}
                            onClick={() => onContinue(previousRoomId)}
                        >
                            继续上一局
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
