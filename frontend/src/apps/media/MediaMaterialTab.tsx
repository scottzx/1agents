import { h } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';
import type { AppViewProps } from '../../modules/appViewRegistry';
import { setCopilotAppContext, clearCopilotAppContext } from '../../stores/appManifestStore';
import { showToast } from '../../stores/uiStore';
import {
    ensureProject,
    listMaterials,
    uploadMaterial,
    launchPipeline,
    type ContentProject,
    type Material,
} from './api';

const APP_ID = 'media';
const NAMESPACE = 'media';

const STAGE_LABELS: Record<string, string> = {
    raw: '原始',
    processing: '处理中',
    silence_detected: '已检测',
    trimmed: '已裁剪',
    edited: '已剪辑',
    approved: '已终审',
};

/**
 * MediaMaterialTab — 素材库 (#336): the 自媒体 specialization of the generic 资产 tab.
 * Lists materials for the workspace's content project, supports upload/record
 * (bytes land on the file face server-side), and launches the mixed-executor
 * processing pipeline per material.
 */
export function MediaMaterialTab({ workspaceId }: AppViewProps) {
    const workspace = workspaceId ?? '';
    const [project, setProject] = useState<ContentProject | null>(null);
    const [materials, setMaterials] = useState<Material[]>([]);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);
    const fileRef = useRef<HTMLInputElement>(null);

    const refresh = useCallback(async (projectId: string) => {
        try {
            setMaterials(await listMaterials(projectId));
        } catch (e) {
            showToast(`加载素材失败: ${String(e)}`);
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
                await refresh(cp.id);
            } catch (e) {
                showToast(`初始化自媒体项目失败: ${String(e)}`);
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
    }, [workspace, refresh]);

    const onPickFile = useCallback(
        async (e: Event) => {
            const input = e.target as HTMLInputElement;
            const file = input.files?.[0];
            if (!file || !project) return;
            setBusy(true);
            try {
                await uploadMaterial(project.id, file, 0);
                showToast(`已上传 ${file.name}`);
                await refresh(project.id);
            } catch (err) {
                showToast(`上传失败: ${String(err)}`);
            } finally {
                setBusy(false);
                if (fileRef.current) fileRef.current.value = '';
            }
        },
        [project, refresh]
    );

    const onLaunch = useCallback(
        async (m: Material) => {
            if (!project) return;
            setBusy(true);
            try {
                const ids = await launchPipeline(project.id, m.id);
                showToast(`已启动处理管线 (${ids.length} 个阶段任务)`);
                await refresh(project.id);
            } catch (err) {
                showToast(`启动管线失败: ${String(err)}`);
            } finally {
                setBusy(false);
            }
        },
        [project, refresh]
    );

    if (loading) return <div class="media-tab-empty">加载中…</div>;
    if (!workspace) return <div class="media-tab-empty">请在项目内打开此标签。</div>;

    return (
        <div class="media-tab">
            <div class="media-tab-toolbar">
                <div class="media-tab-title">素材库</div>
                <div class="media-tab-actions">
                    <button
                        class="media-btn media-btn-primary"
                        disabled={busy}
                        onClick={() => fileRef.current?.click()}
                    >
                        上传 / 录制素材
                    </button>
                    <input
                        ref={fileRef}
                        type="file"
                        accept="video/*,audio/*,image/*"
                        class="media-hidden-input"
                        onChange={onPickFile}
                    />
                </div>
            </div>

            {materials.length === 0 ? (
                <div class="media-tab-empty">还没有素材。上传或录制以开始。</div>
            ) : (
                <div class="bento-grid media-material-grid">
                    {materials.map(m => (
                        <div key={m.id} class="bento-card media-material-card">
                            <div class="bento-zone-header">
                                <span class={`media-kind-badge media-kind-${m.kind}`}>{m.kind}</span>
                                <span class={`media-stage-badge media-stage-${m.stage}`}>
                                    {STAGE_LABELS[m.stage] ?? m.stage}
                                </span>
                            </div>
                            <div class="bento-zone-body">
                                <div class="media-material-name">{m.metadata?.originalName ?? m.filePath ?? m.id}</div>
                                <div class="media-material-meta">
                                    {m.duration > 0 ? `${m.duration.toFixed(1)}s · ` : ''}
                                    {m.filePath}
                                </div>
                            </div>
                            <div class="bento-zone-footer">
                                <button class="media-btn media-btn-sm" disabled={busy} onClick={() => onLaunch(m)}>
                                    启动处理管线
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
}
