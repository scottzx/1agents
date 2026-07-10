/** @jsx React.createElement */
/** @jsxFrag React.Fragment */
import React from 'react';
import ReactDOM from 'react-dom/client';
import { Player } from '@remotion/player';
import { AbsoluteFill, Sequence, useCurrentFrame, Video } from 'remotion';
import type { Timeline, TimelineClip } from './timeline';

// ── compositionId constant (used by Remotion render pipeline) ─────────────────
export const COMPOSITION_ID = 'ContentStudioTimeline';
const DEFAULT_FPS = 30;

// ── ms → frame helpers ────────────────────────────────────────────────────────

export interface ClipFrames {
    startFrame: number;
    endFrame: number;
    durationFrames: number;
}

export function clipToFrames(clip: TimelineClip, fps: number): ClipFrames {
    const startFrame = Math.round((clip.startMs / 1000) * fps);
    const endFrame = Math.round((clip.endMs / 1000) * fps);
    return { startFrame, endFrame, durationFrames: Math.max(1, endFrame - startFrame) };
}

export interface SequenceSpec extends ClipFrames {
    from: number;
    clip: TimelineClip;
}

export function timelineToSequences(timeline: Timeline, fps: number): SequenceSpec[] {
    let offset = 0;
    return timeline.clips.map(clip => {
        const f = clipToFrames(clip, fps);
        const spec: SequenceSpec = { ...f, from: offset, clip };
        offset += f.durationFrames;
        return spec;
    });
}

export function getTimelineDurationInFrames(timeline: Timeline, fps: number): number {
    const total = timeline.clips.reduce((s, c) => s + clipToFrames(c, fps).durationFrames, 0);
    return Math.max(1, total);
}

// ── ContentStudioTimelineComposition ─────────────────────────────────────────

export interface ContentStudioTimelineProps {
    timeline: Timeline;
    fps: number;
    assetUrl?: string | null;
    onVideoError?: () => void;
}

const ClipSlide: React.FC<{ text: string; durationFrames: number; index: number; total: number }> = ({
    text,
    durationFrames,
    index,
    total,
}) => {
    const frame = useCurrentFrame();
    const progress = durationFrames > 1 ? frame / (durationFrames - 1) : 1;
    return (
        <AbsoluteFill
            style={{
                background: '#111',
                color: '#f7f7f2',
                display: 'flex',
                flexDirection: 'column',
                justifyContent: 'center',
                alignItems: 'center',
                padding: 64,
                fontFamily: 'Inter, system-ui, sans-serif',
            }}
        >
            <div
                style={{
                    fontSize: 12,
                    letterSpacing: 1.5,
                    opacity: 0.45,
                    textTransform: 'uppercase',
                    marginBottom: 20,
                }}
            >
                {index + 1} / {total}
            </div>
            <div style={{ fontSize: 36, fontWeight: 600, textAlign: 'center', maxWidth: 860, lineHeight: 1.5 }}>
                {text}
            </div>
            <div
                style={{ marginTop: 40, height: 3, width: 400, background: 'rgba(255,255,255,0.12)', borderRadius: 2 }}
            >
                <div
                    style={{
                        height: '100%',
                        width: `${Math.round(progress * 100)}%`,
                        background: '#2563eb',
                        borderRadius: 2,
                    }}
                />
            </div>
        </AbsoluteFill>
    );
};

export const ContentStudioTimelineComposition: React.FC<ContentStudioTimelineProps> = ({
    timeline,
    fps,
    assetUrl,
    onVideoError,
}) => {
    const sequences = timelineToSequences(timeline, fps);
    return (
        <AbsoluteFill style={{ background: '#111' }}>
            {sequences.map((seq, i) => (
                <Sequence key={i} from={seq.from} durationInFrames={seq.durationFrames}>
                    {assetUrl ? (
                        <Video
                            src={assetUrl}
                            startFrom={seq.startFrame}
                            onError={() => onVideoError?.()}
                            style={{ objectFit: 'contain', width: '100%', height: '100%' }}
                        />
                    ) : null}
                    <ClipSlide
                        text={seq.clip.text}
                        durationFrames={seq.durationFrames}
                        index={i}
                        total={sequences.length}
                    />
                </Sequence>
            ))}
        </AbsoluteFill>
    );
};

// ── Player wiring ─────────────────────────────────────────────────────────────

type BarePlayerProps = {
    component: React.ComponentType<Record<string, unknown>>;
    durationInFrames: number;
    compositionWidth: number;
    compositionHeight: number;
    fps: number;
    style?: React.CSSProperties;
    controls?: boolean;
    defaultProps?: Record<string, unknown>;
};
const BarePlayer = Player as unknown as React.ComponentType<BarePlayerProps>;

class IslandErrorBoundary extends React.Component<
    { onError: (e: string) => void; children: React.ReactNode },
    { hasError: boolean }
> {
    constructor(props: { onError: (e: string) => void; children: React.ReactNode }) {
        super(props);
        this.state = { hasError: false };
    }
    static getDerivedStateFromError(): { hasError: boolean } {
        return { hasError: true };
    }
    componentDidCatch(error: Error): void {
        this.props.onError(error.message);
    }
    render(): React.ReactNode {
        if (this.state.hasError) return null;
        return this.props.children;
    }
}

export interface RemotionPreviewIslandProps {
    onError: (e: string) => void;
    timeline?: Timeline;
    assetUrl?: string | null;
    fps?: number;
}

const EmptyComp: React.ComponentType<Record<string, unknown>> = () => {
    const frame = useCurrentFrame();
    return (
        <AbsoluteFill
            style={{
                background: '#111',
                color: 'rgba(247,247,242,0.35)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                fontFamily: 'Inter, system-ui, sans-serif',
                fontSize: 18,
            }}
        >
            选择金句并生成 Timeline 后预览 · frame {frame}
        </AbsoluteFill>
    );
};

export function RemotionPreviewIsland({
    onError,
    timeline,
    assetUrl,
    fps = DEFAULT_FPS,
}: RemotionPreviewIslandProps): React.ReactElement {
    const hasTl = timeline && timeline.clips.length > 0;

    if (!hasTl) {
        return (
            <IslandErrorBoundary onError={onError}>
                <BarePlayer
                    component={EmptyComp}
                    durationInFrames={150}
                    compositionWidth={1920}
                    compositionHeight={1080}
                    fps={fps}
                    style={{ width: '100%', aspectRatio: '16/9' }}
                    controls
                    defaultProps={{}}
                />
            </IslandErrorBoundary>
        );
    }

    const duration = getTimelineDurationInFrames(timeline, fps);
    const comp = ContentStudioTimelineComposition as unknown as React.ComponentType<Record<string, unknown>>;
    return (
        <IslandErrorBoundary onError={onError}>
            <BarePlayer
                component={comp}
                durationInFrames={duration}
                compositionWidth={1920}
                compositionHeight={1080}
                fps={fps}
                style={{ width: '100%', aspectRatio: '16/9' }}
                controls
                defaultProps={
                    {
                        timeline,
                        fps,
                        assetUrl: assetUrl ?? null,
                        onVideoError: () => onError('视频加载失败'),
                    } as Record<string, unknown>
                }
            />
        </IslandErrorBoundary>
    );
}

export interface RemotionIslandHandle {
    update(timeline: Timeline | null, assetUrl: string | null): void;
    unmount(): void;
}

export function mountRemotionIsland(
    container: HTMLElement,
    onError: (e: string) => void,
    opts?: { timeline?: Timeline; assetUrl?: string | null; fps?: number }
): RemotionIslandHandle {
    const root = ReactDOM.createRoot(container);
    const fps = opts?.fps ?? DEFAULT_FPS;

    const render = (timeline: Timeline | null | undefined, assetUrl: string | null | undefined) => {
        root.render(
            <RemotionPreviewIsland onError={onError} timeline={timeline ?? undefined} assetUrl={assetUrl} fps={fps} />
        );
    };

    render(opts?.timeline, opts?.assetUrl);

    return {
        update: render,
        unmount: () => root.unmount(),
    };
}
