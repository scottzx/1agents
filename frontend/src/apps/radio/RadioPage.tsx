/**
 * RadioPage — the AI Radio on-demand L1 page (#347).
 *
 * Layout: a left list of episodes + a "new episode" form, and a right detail
 * pane showing the 3-stage pipeline state (via ListTasksForBusiness), the
 * transcript, and a point-and-play <audio> player streaming from /api/radio/audio.
 *
 * This view is purely additive: it only CALLS the radio HTTP surface and the
 * platform app-view registry — it touches no kernel page or task-flow code.
 */

import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { AppViewProps } from '../../modules/appViewRegistry';
import { setCopilotAppContext, clearCopilotAppContext } from '../../stores/appManifestStore';
import { workspaces } from '../../stores/workspaceStore';
import {
    listEpisodes,
    getEpisode,
    createEpisode,
    startPipeline,
    audioUrl,
    type RadioEpisode,
    type EpisodeDetail,
    type EpisodeStatus,
} from './radioService';

const STAGES: Array<{ key: string; label: string }> = [
    { key: 'summarize', label: '总结' },
    { key: 'transcript', label: '逐字稿' },
    { key: 'synthesize', label: 'TTS 合成' },
];

const STATUS_LABEL: Record<EpisodeStatus, string> = {
    draft: '草稿',
    summarizing: '总结中',
    transcribing: '生成逐字稿',
    synthesizing: '合成中',
    ready: '可播放',
};

export function RadioPage({ appId }: AppViewProps) {
    const [episodes, setEpisodes] = useState<RadioEpisode[]>([]);
    const [selectedId, setSelectedId] = useState<number | null>(null);
    const [detail, setDetail] = useState<EpisodeDetail | null>(null);
    const [title, setTitle] = useState('');
    const [sourceUrl, setSourceUrl] = useState('');
    const [workspace, setWorkspace] = useState('');
    const [busy, setBusy] = useState(false);

    const ws = workspaces.value;

    // Co-pilot context: announce the active app so chat can scope tasks to radio.
    useEffect(() => {
        setCopilotAppContext({ appId, namespace: 'AI 电台', connectors: [] });
        return () => clearCopilotAppContext();
    }, [appId]);

    const refreshList = useCallback(async () => {
        setEpisodes(await listEpisodes());
    }, []);

    useEffect(() => {
        void refreshList();
    }, [refreshList]);

    // Default the workspace picker to the first available workspace.
    useEffect(() => {
        if (!workspace && ws.length > 0) setWorkspace(ws[0].path);
    }, [ws, workspace]);

    const refreshDetail = useCallback(async (id: number) => {
        setDetail(await getEpisode(id));
    }, []);

    useEffect(() => {
        if (selectedId !== null) void refreshDetail(selectedId);
        else setDetail(null);
    }, [selectedId, refreshDetail]);

    const onCreate = async (e: Event) => {
        e.preventDefault();
        if (!title.trim() || !workspace) return;
        setBusy(true);
        const ep = await createEpisode(title.trim(), sourceUrl.trim(), workspace);
        setBusy(false);
        if (ep) {
            setTitle('');
            setSourceUrl('');
            await refreshList();
            setSelectedId(ep.id);
        }
    };

    const onStartPipeline = async (id: number) => {
        setBusy(true);
        await startPipeline(id);
        setBusy(false);
        await refreshList();
        await refreshDetail(id);
    };

    return (
        <div class="radio-page">
            <div class="radio-list-pane">
                <header class="radio-pane-header">
                    <h2>AI 电台</h2>
                    <p class="radio-pane-sub">每条来源 → 一期可点播节目</p>
                </header>

                <form class="radio-new-form" onSubmit={onCreate}>
                    <input
                        class="radio-input"
                        placeholder="节目标题"
                        value={title}
                        onInput={e => setTitle((e.target as HTMLInputElement).value)}
                    />
                    <input
                        class="radio-input"
                        placeholder="来源 URL(RSS 条目，可选)"
                        value={sourceUrl}
                        onInput={e => setSourceUrl((e.target as HTMLInputElement).value)}
                    />
                    <select
                        class="radio-input"
                        value={workspace}
                        onChange={e => setWorkspace((e.target as HTMLSelectElement).value)}
                    >
                        {ws.length === 0 && <option value="">无可用工作区</option>}
                        {ws.map(w => (
                            <option key={w.id} value={w.path}>
                                {w.name}
                            </option>
                        ))}
                    </select>
                    <button
                        class="radio-btn radio-btn-primary"
                        type="submit"
                        disabled={busy || !title.trim() || !workspace}
                    >
                        新建节目
                    </button>
                </form>

                <ul class="radio-episode-list">
                    {episodes.length === 0 && <li class="radio-empty">还没有节目</li>}
                    {episodes.map(ep => (
                        <li
                            key={ep.id}
                            class={`radio-episode-item${selectedId === ep.id ? ' is-active' : ''}`}
                            onClick={() => setSelectedId(ep.id)}
                        >
                            <span class="radio-episode-title">{ep.title || '未命名节目'}</span>
                            <span class={`radio-status-badge radio-status-${ep.status}`}>
                                {STATUS_LABEL[ep.status] ?? ep.status}
                            </span>
                        </li>
                    ))}
                </ul>
            </div>

            <div class="radio-detail-pane">
                {!detail && <div class="radio-detail-empty">选择一期节目查看详情</div>}
                {detail && <EpisodeDetailView detail={detail} busy={busy} onStartPipeline={onStartPipeline} />}
            </div>
        </div>
    );
}

function EpisodeDetailView({
    detail,
    busy,
    onStartPipeline,
}: {
    detail: EpisodeDetail;
    busy: boolean;
    onStartPipeline: (id: number) => void;
}) {
    const { episode, tasks } = detail;

    // Map each stage to its task status for the inline pipeline strip.
    const stageStatus = (stageKey: string): string => {
        const tk = tasks.find(t => t.milestone === stageKey);
        return tk ? tk.status : 'pending';
    };

    const canStart = episode.status === 'draft';

    return (
        <Fragment>
            <header class="radio-detail-header">
                <h3>{episode.title || '未命名节目'}</h3>
                {episode.sourceUrl && (
                    <a class="radio-source-link" href={episode.sourceUrl} target="_blank" rel="noreferrer">
                        来源
                    </a>
                )}
            </header>

            <div class="radio-pipeline-strip">
                {STAGES.map((s, i) => {
                    const st = stageStatus(s.key);
                    return (
                        <Fragment key={s.key}>
                            {i > 0 && (
                                <span class="radio-pipeline-arrow" aria-hidden="true">
                                    →
                                </span>
                            )}
                            <span class={`radio-stage radio-stage-${st}`}>
                                <span class="radio-stage-label">{s.label}</span>
                                <span class="radio-stage-status">{st}</span>
                            </span>
                        </Fragment>
                    );
                })}
            </div>

            {canStart && (
                <button
                    class="radio-btn radio-btn-primary radio-start-btn"
                    disabled={busy}
                    onClick={() => onStartPipeline(episode.id)}
                >
                    启动三段管线
                </button>
            )}

            {episode.status === 'ready' && episode.audioPath && (
                <div class="radio-player-box">
                    <audio class="radio-audio" controls src={audioUrl(episode.id)}>
                        您的浏览器不支持音频播放。
                    </audio>
                </div>
            )}

            {episode.summary && (
                <section class="radio-text-section">
                    <h4>总结</h4>
                    <p>{episode.summary}</p>
                </section>
            )}

            {episode.transcript && (
                <section class="radio-text-section">
                    <h4>逐字稿</h4>
                    <p class="radio-transcript">{episode.transcript}</p>
                </section>
            )}
        </Fragment>
    );
}
