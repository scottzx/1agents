/**
 * v1 timelines/main.json contract — TypeScript types and UI-side validation.
 *
 * Go backend (backend/internal/apps/speechclip/timeline.go) is the authoritative
 * validator on save. This module provides early feedback in the UI so users see
 * errors before the round-trip.
 *
 * Field alignment with JSONL sources:
 *   SourceSentenceIDs → transcript `i`
 *   startMs / endMs   → transcript `start` / `end` (already in ms)
 *   text              → highlight `corrected_text` (or raw `text`)
 */

export interface TimelineClip {
    /** ms, 0 <= startMs < endMs; maps to transcript `start` */
    startMs: number;
    /** ms; maps to transcript `end` */
    endMs: number;
    /** display text — typically highlight `corrected_text` or raw `text` */
    text: string;
    /** sentence `i` values from transcripts/<assetId>.jsonl that this clip covers */
    sourceSentenceIds: number[];
}

export interface Timeline {
    /** must be 1 */
    version: 1;
    /** must be "main" */
    id: 'main';
    /** root asset id, non-empty */
    assetId: string;
    /** optional total duration in ms; must be >= last clip endMs */
    durationMs?: number;
    /** ordered edit segments, non-empty */
    clips: TimelineClip[];
}

/**
 * validateTimeline returns an error message string, or null if valid.
 * Mirrors ValidateTimeline in timeline.go — keep the two in sync.
 */
export function validateTimeline(t: unknown): string | null {
    if (typeof t !== 'object' || t === null) return 'timeline must be an object';
    const tl = t as Record<string, unknown>;

    if (tl['version'] !== 1) return `version must be 1, got ${tl['version']}`;
    if (tl['id'] !== 'main') return `id must be "main", got ${JSON.stringify(tl['id'])}`;
    if (!tl['assetId'] || typeof tl['assetId'] !== 'string') return 'assetId is required';

    const clips = tl['clips'];
    if (!Array.isArray(clips) || clips.length === 0) return 'clips must be non-empty';

    for (let idx = 0; idx < clips.length; idx++) {
        const c = clips[idx] as Record<string, unknown>;
        const startMs = c['startMs'];
        const endMs = c['endMs'];
        if (typeof startMs !== 'number' || startMs < 0) {
            return `clip[${idx}]: startMs must be >= 0, got ${startMs}`;
        }
        if (typeof endMs !== 'number' || startMs >= endMs) {
            return `clip[${idx}]: startMs (${startMs}) must be < endMs (${endMs})`;
        }
    }

    if (tl['durationMs'] !== undefined) {
        const d = tl['durationMs'];
        if (typeof d !== 'number' || d <= 0) return `durationMs must be > 0, got ${d}`;
        const lastClip = clips[clips.length - 1] as Record<string, unknown>;
        const lastEnd = lastClip['endMs'] as number;
        if (d < lastEnd) return `durationMs (${d}) must be >= last clip endMs (${lastEnd})`;
    }

    return null;
}

// ── fixtures ────────────────────────────────────────────────────────────────

/**
 * Valid timeline fixture — derived from the smoke-test transcript sentences:
 *   i=0 start=0    end=3000  corrected_text="今天是一个硬件的采购记录。"
 *   i=2 start=6000 end=9000  corrected_text="芯片是ESP32，不支持经典蓝牙。"
 */
export const VALID_TIMELINE_FIXTURE: Timeline = {
    version: 1,
    id: 'main',
    assetId: 'a01',
    clips: [
        {
            startMs: 0,
            endMs: 3000,
            text: '今天是一个硬件的采购记录。',
            sourceSentenceIds: [0],
        },
        {
            startMs: 6000,
            endMs: 9000,
            text: '芯片是ESP32，不支持经典蓝牙。',
            sourceSentenceIds: [2],
        },
    ],
};

/**
 * Invalid-timestamp fixture — startMs > endMs, should fail validation.
 * Used in tests to verify the validator catches reversed timestamps.
 */
export const INVALID_TIMESTAMP_FIXTURE: Record<string, unknown> = {
    version: 1,
    id: 'main',
    assetId: 'a01',
    clips: [{ startMs: 5000, endMs: 3000, text: 'reversed', sourceSentenceIds: [0] }],
};
