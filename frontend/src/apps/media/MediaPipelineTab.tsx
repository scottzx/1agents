import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { AppViewProps } from '../../modules/appViewRegistry';
import { setCopilotAppContext, clearCopilotAppContext } from '../../stores/appManifestStore';
import { showToast } from '../../stores/uiStore';
import {
    ensureProject,
    listMaterials,
    listSegments,
    setSegmentDecision,
    listBusinessTasks,
    resolveHumanTask,
    businessRef,
    type ContentProject,
    type Material,
    type Segment,
    type MediaTask,
} from './api';

const APP_ID = 'media';
const NAMESPACE = 'media';

// Pipeline stage order, keyed by task.milestone, with display + status glyph.
const STAGES = [
    { key: 'silence_detect', label: '静音检测', executor: 'function' },
    { key: 'edit', label: '智能剪辑', executor: 'agent' },
    { key: 'approve', label: '段落终审', executor: 'human' },
];

function statusGlyph(status: string): string {
    switch (status) {
        case 'completed':
            return '✓';
        case 'running':
        case 'in_progress':
        case 'pending_review':
            return '⟳';
        case 'failed':
            return '✗';
        default:
            return '◌';
    }
}

/**
 * MediaPipelineTab — 阶段追踪 + 段落取舍 (#337/#338). Per-material pipeline state
 * (function → agent → human) pulled from the reverse binding seam, plus a
 * keep/drop segment selection UI that resolves the human decision gate. The
 * completion writeback hook advances domain state; this view re-polls to reflect
 * the ⟳→✓ transition.
 */
export function MediaPipelineTab({ workspaceId }: AppViewProps) {
    const workspace = workspaceId ?? '';
    const [, setProject] = useState<ContentProject | null>(null);
    const [materials, setMaterials] = useState<Material[]>([]);
    const [selected, setSelected] = useState<Material | null>(null);
    const [segments, setSegments] = useState<Segment[]>([]);
    const [tasks, setTasks] = useState<MediaTask[]>([]);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);

    const loadMaterialDetail = useCallback(async (m: Material) => {
        try {
            const [segs, tks] = await Promise.all([
                listSegments(m.id),
                listBusinessTasks(businessRef('material', m.id)),
            ]);
            setSegments(segs);
            setTasks(tks);
        } catch (e) {
            showToast(`加载阶段信息失败: ${String(e)}`);
        }
    }, []);

    useEffect(() => {
        if (!workspace) {
            setLoading(false);
            return;
        }
        let active = true;
        (async () => {
            try {
                const cp = await ensureProject(workspace, '自媒体项目');
                if (!active) return;
                setProject(cp);
                const mats = await listMaterials(cp.id);
                if (!active) return;
                setMaterials(mats);
                if (mats.length > 0) {
                    setSelected(mats[0]);
                    await loadMaterialDetail(mats[0]);
                }
            } catch (e) {
                showToast(`初始化失败: ${String(e)}`);
            } finally {
                if (active) setLoading(false);
            }
        })();
        setCopilotAppContext({
            appId: APP_ID,
            namespace: NAMESPACE,
            connectors: [],
            projectWorkspaceId: workspace,
        });
        return () => {
            active = false;
            clearCopilotAppContext();
        };
    }, [workspace, loadMaterialDetail]);

    const onSelectMaterial = useCallback(
        async (m: Material) => {
            setSelected(m);
            await loadMaterialDetail(m);
        },
        [loadMaterialDetail]
    );

    const onDecide = useCallback(
        async (seg: Segment, decision: 'keep' | 'drop') => {
            const next = seg.decision === decision ? 'undecided' : decision;
            // optimistic
            setSegments(prev => prev.map(s => (s.id === seg.id ? { ...s, decision: next } : s)));
            try {
                await setSegmentDecision(seg.id, next);
            } catch (e) {
                showToast(`保存取舍失败: ${String(e)}`);
                if (selected) await loadMaterialDetail(selected);
            }
        },
        [selected, loadMaterialDetail]
    );

    const humanTask = tasks.find(t => t.milestone === 'approve');
    const humanPending = humanTask && humanTask.status !== 'completed';

    const onApprove = useCallback(async () => {
        if (!humanTask || !selected) return;
        const kept = segments.filter(s => s.decision === 'keep').length;
        setBusy(true);
        try {
            await resolveHumanTask(humanTask.id, 'approved', { kept });
            showToast(`已终审,保留 ${kept} 段`);
            await loadMaterialDetail(selected);
        } catch (e) {
            showToast(`终审失败: ${String(e)}`);
        } finally {
            setBusy(false);
        }
    }, [humanTask, selected, segments, loadMaterialDetail]);

    if (loading) return <div class="media-tab-empty">加载中…</div>;
    if (!workspace) return <div class="media-tab-empty">请在项目内打开此标签。</div>;
    if (materials.length === 0) return <div class="media-tab-empty">还没有素材。先在「素材」标签上传并启动管线。</div>;

    const stageByKey: Record<string, MediaTask | undefined> = {};
    for (const t of tasks) stageByKey[t.milestone] = t;

    return (
        <div class="media-tab media-pipeline-tab">
            <div class="media-pipeline-sidebar">
                {materials.map(m => (
                    <button
                        key={m.id}
                        class={`media-material-pill ${selected?.id === m.id ? 'is-active' : ''}`}
                        onClick={() => onSelectMaterial(m)}
                    >
                        {m.metadata?.originalName ?? m.id}
                    </button>
                ))}
            </div>

            <div class="media-pipeline-main">
                {/* Stage tracking strip */}
                <div class="media-stage-strip">
                    {STAGES.map(stage => {
                        const task = stageByKey[stage.key];
                        const status = task?.status ?? 'pending';
                        return (
                            <div key={stage.key} class={`media-stage-node media-stage-${status}`}>
                                <span class="media-stage-glyph">{statusGlyph(status)}</span>
                                <span class="media-stage-name">{stage.label}</span>
                                <span class={`media-exec-tag media-exec-${stage.executor}`}>{stage.executor}</span>
                            </div>
                        );
                    })}
                </div>

                {/* Segment keep/drop selection */}
                <div class="media-segment-head">
                    <span>段落取舍</span>
                    {humanPending && (
                        <button class="media-btn media-btn-primary" disabled={busy} onClick={onApprove}>
                            确认终审
                        </button>
                    )}
                    {humanTask && !humanPending && <span class="media-approved-tag">已终审 ✓</span>}
                </div>

                {segments.length === 0 ? (
                    <div class="media-tab-empty">暂无候选段落。静音检测阶段完成后会自动生成。</div>
                ) : (
                    <ul class="media-segment-list">
                        {segments.map(seg => (
                            <li key={seg.id} class={`media-segment-row media-decision-${seg.decision}`}>
                                <span class="media-segment-range">
                                    {seg.start.toFixed(1)}s – {seg.end.toFixed(1)}s
                                </span>
                                <span class="media-segment-label">{seg.label}</span>
                                <span class="media-segment-actions">
                                    <button
                                        class={`media-decide-btn ${seg.decision === 'keep' ? 'is-on' : ''}`}
                                        onClick={() => onDecide(seg, 'keep')}
                                    >
                                        保留
                                    </button>
                                    <button
                                        class={`media-decide-btn media-decide-drop ${
                                            seg.decision === 'drop' ? 'is-on' : ''
                                        }`}
                                        onClick={() => onDecide(seg, 'drop')}
                                    >
                                        弃用
                                    </button>
                                </span>
                            </li>
                        ))}
                    </ul>
                )}
            </div>
        </div>
    );
}
