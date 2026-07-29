import { Fragment, h } from 'preact';
import { useCallback, useEffect, useMemo, useState } from 'preact/hooks';

import { featureCatalogService } from '@1agents/core/services/featureCatalogService';
import type {
    CreateFeatureNodeInput,
    FeatureCatalog,
    FeatureItemRelation,
    FeatureMilestoneSyncPreview,
    FeatureNode,
    FeatureNodeKind,
    UpdateFeatureNodeInput,
    GanttData,
} from '@1agents/core/types/featureCatalog';
import type { Milestone, ProjectItem } from './types';
import * as sessionStore from '../../../stores/sessionStore';
import * as taskNav from '../../../stores/taskNavStore';
import * as ui from '../../../stores/uiStore';
import { buildFeatureBreakdownPrompt, buildFeatureCatalogGeneratePrompt } from './aiPMWorkflow';
import { FeatureNodeForm } from './FeatureNodeForm';
import {
    buildFeatureTree,
    flattenFeatureTree,
    formatFeatureError,
    siblingNodes,
    type FeatureTreeNode,
} from './featureCatalogModel';
import { GanttChart } from './GanttChart';

interface FeatureCatalogViewProps {
    workspaceId: string;
    items: ProjectItem[];
    milestones: Milestone[];
    onCatalogChange?: (catalog: FeatureCatalog) => void;
    onItemsChange?: () => Promise<void> | void;
}

interface CreateState {
    kind: FeatureNodeKind;
    parentId: string;
}

const EMPTY_CATALOG: FeatureCatalog = { nodes: [], links: [] };
const PROGRESS_LABELS = {
    unplanned: '未拆解',
    pending: '待开始',
    in_progress: '进行中',
    delivered: '已交付',
    replan: '需要重新规划',
} as const;

function progressSummary(node: FeatureNode): string {
    const progress = node.progress;
    if (!progress) return '';
    if (node.kind === 'module') {
        const parts = [
            `覆盖 ${progress.coveredFeatures}/${progress.totalFeatures}`,
            `交付 ${progress.progressPercent === null ? '—' : `${progress.progressPercent}%`}`,
            `未拆解 ${progress.unplannedFeatures}`,
        ];
        if (progress.replanFeatures > 0) parts.push(`需重规划 ${progress.replanFeatures}`);
        if (node.versionCoverage?.length) {
            parts.push(
                `版本 ${node.versionCoverage
                    .map(coverage => `${coverage.version}×${coverage.featureCount}`)
                    .join('、')}`
            );
        }
        return parts.join(' · ');
    }
    const label = PROGRESS_LABELS[progress.status];
    return progress.progressPercent === null
        ? label
        : `${label} · ${progress.completedTasks}/${progress.totalTasks} · ${progress.progressPercent}%`;
}

function TreeRow({
    entry,
    selectedId,
    onSelect,
}: {
    entry: FeatureTreeNode;
    selectedId: string | null;
    onSelect: (id: string) => void;
}) {
    const isFeature = entry.node.kind === 'feature';
    return (
        <li>
            <button
                type="button"
                class={`feature-tree-row${selectedId === entry.node.id ? ' selected' : ''}`}
                style={`--feature-depth:${Math.max(0, entry.path.length - 1)}`}
                onClick={() => onSelect(entry.node.id)}
            >
                <span class={`feature-kind-icon ${entry.node.kind}`} aria-hidden="true">
                    {isFeature ? '•' : '▾'}
                </span>
                <span class="feature-tree-copy">
                    <span class="feature-tree-title">{entry.node.title}</span>
                    <span class="feature-tree-path">{entry.path.join(' / ')}</span>
                </span>
                {entry.node.progress && <span class="feature-tree-progress">{progressSummary(entry.node)}</span>}
            </button>
            {entry.children.length > 0 && (
                <ul>
                    {entry.children.map(child => (
                        <TreeRow key={child.node.id} entry={child} selectedId={selectedId} onSelect={onSelect} />
                    ))}
                </ul>
            )}
        </li>
    );
}

export function FeatureCatalogView({
    workspaceId,
    items,
    milestones,
    onCatalogChange,
    onItemsChange,
}: FeatureCatalogViewProps) {
    const [catalog, setCatalog] = useState<FeatureCatalog>(EMPTY_CATALOG);
    const [selectedId, setSelectedId] = useState<string | null>(null);
    const [createState, setCreateState] = useState<CreateState | null>(null);
    const [deleteTarget, setDeleteTarget] = useState<FeatureNode | null>(null);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [mobileDetailOpen, setMobileDetailOpen] = useState(false);
    const [sourceSelection, setSourceSelection] = useState('');
    const [deliverySelection, setDeliverySelection] = useState('');
    const [syncPreview, setSyncPreview] = useState<FeatureMilestoneSyncPreview | null>(null);

    const [viewMode, setViewMode] = useState<'tree' | 'gantt'>('tree');
    const [ganttData, setGanttData] = useState<GanttData | null>(null);

    const tree = useMemo(() => buildFeatureTree(catalog.nodes), [catalog.nodes]);
    const flatTree = useMemo(() => flattenFeatureTree(tree), [tree]);
    const entryById = useMemo(() => new Map(flatTree.map(entry => [entry.node.id, entry])), [flatTree]);
    const selectedEntry = selectedId ? entryById.get(selectedId) : undefined;
    const selectedNode = selectedEntry?.node;

    const loadCatalog = useCallback(async () => {
        const next = await featureCatalogService.get(workspaceId);
        setCatalog(next);
        onCatalogChange?.(next);
        setSelectedId(current => {
            if (current && next.nodes.some(node => node.id === current)) return current;
            return next.nodes[0]?.id ?? null;
        });
    }, [workspaceId, onCatalogChange]);

    useEffect(() => {
        let active = true;
        setCatalog(EMPTY_CATALOG);
        setSelectedId(null);
        setCreateState(null);
        setDeleteTarget(null);
        setMobileDetailOpen(false);
        setLoading(true);
        setError('');
        featureCatalogService
            .get(workspaceId)
            .then(next => {
                if (!active) return;
                setCatalog(next);
                onCatalogChange?.(next);
                setSelectedId(next.nodes[0]?.id ?? null);
            })
            .catch(cause => {
                if (!active) return;
                const message = formatFeatureError(cause);
                setError(message);
                ui.showToast(message);
            })
            .finally(() => {
                if (active) setLoading(false);
            });
        return () => {
            active = false;
        };
    }, [workspaceId, onCatalogChange]);

    useEffect(() => {
        setSourceSelection('');
        setDeliverySelection('');
        setSyncPreview(null);
    }, [selectedId]);

    const closeMobileDetail = useCallback(() => setMobileDetailOpen(false), []);
    useEffect(() => {
        if (!mobileDetailOpen || !ui.isMobile.value) return;
        return taskNav.registerHeaderBackAction(
            `feature-catalog:${workspaceId}`,
            closeMobileDetail,
            taskNav.HEADER_BACK_PRIORITY.detail
        );
    }, [mobileDetailOpen, workspaceId, closeMobileDetail, ui.isMobile.value]);

    const reportError = useCallback((cause: unknown) => {
        const message = formatFeatureError(cause);
        setError(message);
        ui.showToast(message);
    }, []);

    const loadGantt = useCallback(async () => {
        setBusy(true);
        try {
            const data = await featureCatalogService.gantt(workspaceId);
            setGanttData(data);
        } catch (cause) {
            reportError(cause);
        } finally {
            setBusy(false);
        }
    }, [workspaceId, reportError]);

    const handleViewModeChange = (mode: 'tree' | 'gantt') => {
        setViewMode(mode);
        if (mode === 'gantt' && !ganttData) {
            void loadGantt();
        }
    };

    const downloadBlob = (blob: Blob, filename: string) => {
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        a.click();
        URL.revokeObjectURL(url);
    };

    const handleExport = async (format: 'markdown' | 'json') => {
        setBusy(true);
        try {
            const blob = await featureCatalogService.exportCatalog(workspaceId, format);
            downloadBlob(blob, `catalog.${format === 'markdown' ? 'md' : 'json'}`);
        } catch (cause) {
            reportError(cause);
        } finally {
            setBusy(false);
        }
    };

    const runMutation = useCallback(
        async (mutation: () => Promise<void>, successMessage: string) => {
            setBusy(true);
            setError('');
            try {
                await mutation();
                await loadCatalog();
                ui.showToast(successMessage);
                return true;
            } catch (cause) {
                reportError(cause);
                return false;
            } finally {
                setBusy(false);
            }
        },
        [loadCatalog, reportError]
    );

    const openCreate = (kind: FeatureNodeKind, parentId = '') => {
        setCreateState({ kind, parentId });
        setError('');
        setMobileDetailOpen(true);
    };

    const selectNode = (id: string) => {
        setSelectedId(id);
        setCreateState(null);
        setError('');
        setMobileDetailOpen(true);
    };

    const createNode = async (input: CreateFeatureNodeInput) => {
        let created: FeatureNode | undefined;
        const ok = await runMutation(
            async () => {
                created = await featureCatalogService.create(workspaceId, input);
            },
            input.kind === 'module' ? '模块已新增。' : '功能点已新增。'
        );
        if (ok && created) {
            setSelectedId(created.id);
            setCreateState(null);
        }
    };

    const updateNode = async (input: UpdateFeatureNodeInput) => {
        if (!selectedNode) return;
        await runMutation(async () => {
            await featureCatalogService.update(workspaceId, selectedNode.id, input);
        }, '节点已保存。');
    };

    const moveSelected = async (offset: -1 | 1) => {
        if (!selectedNode) return;
        const siblings = siblingNodes(catalog.nodes, selectedNode.parentId);
        const index = siblings.findIndex(node => node.id === selectedNode.id);
        const target = index + offset;
        if (index < 0 || target < 0 || target >= siblings.length) return;
        await runMutation(
            async () => {
                await featureCatalogService.update(workspaceId, selectedNode.id, { position: target });
            },
            offset < 0 ? '已上移。' : '已下移。'
        );
    };

    const removeNode = async () => {
        if (!deleteTarget) return;
        const id = deleteTarget.id;
        const ok = await runMutation(
            async () => {
                await featureCatalogService.remove(workspaceId, id);
            },
            deleteTarget.kind === 'module' ? '模块已删除。' : '功能点已删除，关联的项目项保持不变。'
        );
        if (ok) {
            setDeleteTarget(null);
            setCreateState(null);
            setMobileDetailOpen(false);
        }
    };

    const linkItem = async (relation: FeatureItemRelation, itemId: string) => {
        if (!selectedNode || !itemId) return;
        const ok = await runMutation(
            async () => {
                await featureCatalogService.linkItem(workspaceId, selectedNode.id, itemId, relation);
            },
            relation === 'source' ? '来源已关联。' : '交付任务已关联。'
        );
        if (ok) {
            if (relation === 'source') setSourceSelection('');
            else setDeliverySelection('');
        }
    };

    const unlinkItem = async (relation: FeatureItemRelation, itemId: string) => {
        if (!selectedNode) return;
        await runMutation(
            async () => {
                await featureCatalogService.unlinkItem(workspaceId, selectedNode.id, itemId, relation);
            },
            relation === 'source' ? '来源关联已移除。' : '交付任务关联已移除。'
        );
    };

    const generateWithPM = () => {
        void sessionStore.createPMSession(
            workspaceId,
            catalog.nodes.length > 0 ? 'AI PM：维护功能蓝图' : 'AI PM：生成功能蓝图',
            buildFeatureCatalogGeneratePrompt(catalog.nodes.length)
        );
    };

    if (loading) {
        return <div class="feature-catalog-loading">正在加载功能蓝图…</div>;
    }

    if (catalog.nodes.length === 0 && !createState) {
        return (
            <div class="feature-catalog-empty">
                <span class="feature-empty-icon">◇</span>
                <h3>尚未建立功能蓝图</h3>
                <p>从需求和架构设计出发，整理一级、二级、三级模块及功能点，再将功能点分解为可执行任务。</p>
                {error && <div class="feature-form-error">{error}</div>}
                <div class="feature-empty-actions">
                    <button type="button" class="feature-btn primary" onClick={generateWithPM}>
                        与 AI PM 一起生成
                    </button>
                    <button type="button" class="feature-btn secondary" onClick={() => openCreate('module')}>
                        手动新增一级模块
                    </button>
                </div>
            </div>
        );
    }

    const siblings = selectedNode ? siblingNodes(catalog.nodes, selectedNode.parentId) : [];
    const selectedIndex = selectedNode ? siblings.findIndex(node => node.id === selectedNode.id) : -1;
    const directChildren = selectedNode ? catalog.nodes.filter(node => node.parentId === selectedNode.id) : [];
    const linkedItems = selectedNode ? catalog.links.filter(link => link.featureId === selectedNode.id) : [];
    const itemByID = new Map(items.map(item => [item.id, item]));
    const visibleLinkedItems = linkedItems.filter(link => itemByID.has(link.itemId));
    const sourceLinks = visibleLinkedItems.filter(link => link.relation === 'source');
    const deliveryLinks = visibleLinkedItems.filter(link => link.relation === 'delivery');
    const sourceLinkedIDs = new Set(sourceLinks.map(link => link.itemId));
    const deliveryLinkedIDs = new Set(deliveryLinks.map(link => link.itemId));
    const sourceCandidates = items.filter(
        item => (item.type === 'requirement' || item.type === 'bug') && !sourceLinkedIDs.has(item.id)
    );
    const deliveryCandidates = items.filter(
        item => (!item.type || item.type === 'task') && !deliveryLinkedIDs.has(item.id)
    );
    const targetMilestone = milestones.find(milestone => milestone.id === selectedNode?.targetMilestoneId);
    const targetMilestoneName = targetMilestone?.name ?? '';
    const targetMilestoneResolved = !selectedNode?.targetMilestoneId || !!targetMilestone;
    const mismatchedTasks =
        selectedNode?.kind === 'feature' && targetMilestoneResolved
            ? deliveryLinks
                  .map(link => itemByID.get(link.itemId))
                  .filter((item): item is ProjectItem => !!item && (item.milestone ?? '') !== targetMilestoneName)
            : [];
    const detailOpen = mobileDetailOpen || createState !== null;

    const breakdownWithPM = () => {
        if (!selectedNode || selectedNode.kind !== 'feature' || !selectedEntry) return;
        const sources = sourceLinks.map(link => {
            const item = itemByID.get(link.itemId)!;
            const type = item.type === 'bug' ? 'bug' : 'requirement';
            return `${item.number ? `#${item.number} ` : ''}${item.title} [${type}:${item.id}]`;
        });
        void sessionStore.createPMSession(
            workspaceId,
            `AI PM：拆解 ${selectedNode.title}`,
            buildFeatureBreakdownPrompt({
                featureId: selectedNode.id,
                title: selectedNode.title,
                path: selectedEntry.path.join(' / '),
                sources,
                targetVersion: targetMilestone?.version || targetMilestone?.name || '未指定',
            })
        );
    };

    const syncMilestone = async () => {
        if (!selectedNode || selectedNode.kind !== 'feature' || !syncPreview) return;
        const affectedCount = syncPreview.tasks.length;
        const ok = await runMutation(async () => {
            await featureCatalogService.syncMilestone(workspaceId, selectedNode.id);
            await onItemsChange?.();
        }, `已同步 ${affectedCount} 个关联任务。`);
        if (ok) setSyncPreview(null);
    };

    const openSyncPreview = async () => {
        if (!selectedNode || selectedNode.kind !== 'feature') return;
        setBusy(true);
        setError('');
        try {
            const preview = await featureCatalogService.milestoneDiff(workspaceId, selectedNode.id);
            if (preview.tasks.length === 0) {
                await onItemsChange?.();
                ui.showToast('关联任务版本已一致。');
                return;
            }
            setSyncPreview(preview);
        } catch (cause) {
            reportError(cause);
        } finally {
            setBusy(false);
        }
    };

    const renderLinks = (
        title: string,
        relation: FeatureItemRelation,
        links: typeof visibleLinkedItems,
        candidates: ProjectItem[],
        selection: string,
        setSelection: (value: string) => void
    ) => (
        <div class="feature-trace-card">
            <div class="feature-trace-heading">
                <strong>{title}</strong>
                <span>{links.length}</span>
            </div>
            {links.length === 0 ? (
                <div class="feature-trace-empty">{relation === 'source' ? '尚未关联来源需求或缺陷。' : '未拆解'}</div>
            ) : (
                <div class="feature-trace-list">
                    {links.map(link => {
                        const item = itemByID.get(link.itemId)!;
                        return (
                            <div key={`${link.itemId}:${relation}`} class="feature-trace-row">
                                <span>
                                    <strong>
                                        {item.number ? `#${item.number} ` : ''}
                                        {item.title}
                                    </strong>
                                    <small>
                                        {item.type === 'requirement'
                                            ? '需求'
                                            : item.type === 'bug'
                                              ? '缺陷'
                                              : item.status}
                                    </small>
                                </span>
                                <button
                                    type="button"
                                    title="移除关联"
                                    disabled={busy}
                                    onClick={() => void unlinkItem(relation, item.id)}
                                >
                                    ×
                                </button>
                            </div>
                        );
                    })}
                </div>
            )}
            <div class="feature-trace-add">
                <select
                    value={selection}
                    disabled={busy || candidates.length === 0}
                    onChange={(event: Event) => setSelection((event.target as HTMLSelectElement).value)}
                >
                    <option value="">
                        {candidates.length === 0
                            ? relation === 'source'
                                ? '没有可关联的需求/缺陷'
                                : '没有可关联的任务'
                            : relation === 'source'
                              ? '选择需求或缺陷…'
                              : '选择任务…'}
                    </option>
                    {candidates.map(item => (
                        <option key={item.id} value={item.id}>
                            {item.number ? `#${item.number} ` : ''}
                            {item.title}
                        </option>
                    ))}
                </select>
                <button
                    type="button"
                    class="feature-btn secondary"
                    disabled={busy || !selection}
                    onClick={() => void linkItem(relation, selection)}
                >
                    关联
                </button>
            </div>
        </div>
    );

    return (
        <div class={`feature-catalog-view${detailOpen ? ' detail-open' : ''}`}>
            <div class="feature-catalog-toolbar">
                <div class="feature-view-toggle">
                    <button class={viewMode === 'tree' ? 'active' : ''} onClick={() => handleViewModeChange('tree')}>
                        树形视图
                    </button>
                    <button class={viewMode === 'gantt' ? 'active' : ''} onClick={() => handleViewModeChange('gantt')}>
                        甘特图
                    </button>
                </div>
                <div class="feature-export-actions">
                    <button
                        type="button"
                        class="feature-btn secondary compact"
                        disabled={busy}
                        onClick={() => void handleExport('markdown')}
                    >
                        导出 Markdown
                    </button>
                    <button
                        type="button"
                        class="feature-btn secondary compact"
                        disabled={busy}
                        onClick={() => void handleExport('json')}
                    >
                        导出 JSON
                    </button>
                </div>
            </div>

            {viewMode === 'gantt' ? (
                ganttData ? (
                    <GanttChart workspaceId={workspaceId} data={ganttData} />
                ) : (
                    <div class="feature-catalog-loading">加载甘特图…</div>
                )
            ) : (
                <Fragment>
                    <section class="feature-catalog-tree-pane" aria-label="功能蓝图树">
                        <div class="feature-pane-header">
                            <div>
                                <h3>功能蓝图</h3>
                                <span>{catalog.nodes.length} 个节点</span>
                            </div>
                            <div class="feature-pane-actions">
                                <button type="button" class="feature-btn secondary compact" onClick={generateWithPM}>
                                    与 AI PM 一起生成
                                </button>
                                <button
                                    type="button"
                                    class="feature-btn primary compact"
                                    onClick={() => openCreate('module')}
                                >
                                    + 一级模块
                                </button>
                            </div>
                        </div>
                        {error && !selectedNode && <div class="feature-form-error">{error}</div>}
                        <ul class="feature-tree">
                            {tree.map(entry => (
                                <TreeRow
                                    key={entry.node.id}
                                    entry={entry}
                                    selectedId={selectedId}
                                    onSelect={selectNode}
                                />
                            ))}
                        </ul>
                    </section>

                    <section class="feature-catalog-detail-pane" aria-label="功能节点详情">
                        <button type="button" class="feature-mobile-back" onClick={closeMobileDetail}>
                            ← 返回功能树
                        </button>
                        {createState ? (
                            <Fragment>
                                <div class="feature-detail-heading">
                                    <div>
                                        <span class="feature-detail-kicker">新增</span>
                                        <h3>{createState.kind === 'module' ? '模块' : '功能点'}</h3>
                                    </div>
                                </div>
                                <FeatureNodeForm
                                    key={`create:${createState.kind}:${createState.parentId}`}
                                    kind={createState.kind}
                                    nodes={catalog.nodes}
                                    milestones={milestones}
                                    initialParentId={createState.parentId}
                                    busy={busy}
                                    error={error}
                                    onCancel={() => setCreateState(null)}
                                    onCreate={createNode}
                                />
                            </Fragment>
                        ) : selectedNode && selectedEntry ? (
                            <Fragment>
                                <div class="feature-detail-heading">
                                    <div>
                                        <span class="feature-detail-kicker">
                                            {selectedNode.kind === 'module'
                                                ? `${selectedEntry.moduleDepth} 级模块`
                                                : '功能点'}
                                        </span>
                                        <h3>{selectedNode.title}</h3>
                                        <p>{selectedEntry.path.join(' / ')}</p>
                                    </div>
                                    <div class="feature-detail-actions">
                                        <button
                                            type="button"
                                            class="feature-icon-btn"
                                            title="上移"
                                            disabled={busy || selectedIndex <= 0}
                                            onClick={() => void moveSelected(-1)}
                                        >
                                            ↑
                                        </button>
                                        <button
                                            type="button"
                                            class="feature-icon-btn"
                                            title="下移"
                                            disabled={busy || selectedIndex === siblings.length - 1}
                                            onClick={() => void moveSelected(1)}
                                        >
                                            ↓
                                        </button>
                                    </div>
                                </div>
                                {selectedNode.kind === 'module' && (
                                    <div class="feature-add-row">
                                        {selectedEntry.moduleDepth < 3 && (
                                            <button
                                                type="button"
                                                class="feature-btn secondary"
                                                onClick={() => openCreate('module', selectedNode.id)}
                                            >
                                                + 子模块
                                            </button>
                                        )}
                                        <button
                                            type="button"
                                            class="feature-btn secondary"
                                            onClick={() => openCreate('feature', selectedNode.id)}
                                        >
                                            + 功能点
                                        </button>
                                    </div>
                                )}
                                <FeatureNodeForm
                                    key={selectedNode.id}
                                    kind={selectedNode.kind}
                                    nodes={catalog.nodes}
                                    milestones={milestones}
                                    node={selectedNode}
                                    busy={busy}
                                    error={error}
                                    onUpdate={updateNode}
                                />
                                {selectedNode.progress && (
                                    <div class="feature-progress-card">
                                        <div>
                                            <span>派生状态</span>
                                            <strong>{PROGRESS_LABELS[selectedNode.progress.status]}</strong>
                                        </div>
                                        <div>
                                            <span>交付进度</span>
                                            <strong>
                                                {selectedNode.progress.progressPercent === null
                                                    ? '—'
                                                    : `${selectedNode.progress.progressPercent}%`}
                                            </strong>
                                            <small>
                                                {selectedNode.progress.completedTasks}/
                                                {selectedNode.progress.totalTasks} 个有效任务
                                            </small>
                                        </div>
                                        <div>
                                            <span>功能覆盖</span>
                                            <strong>
                                                {selectedNode.progress.coveredFeatures}/
                                                {selectedNode.progress.totalFeatures}
                                            </strong>
                                            <small>
                                                未拆解 {selectedNode.progress.unplannedFeatures} · 需重规划{' '}
                                                {selectedNode.progress.replanFeatures}
                                            </small>
                                        </div>
                                    </div>
                                )}
                                {selectedNode.kind === 'module' && (
                                    <div class="feature-version-coverage">
                                        <strong>后代功能点版本覆盖</strong>
                                        {selectedNode.versionCoverage?.length ? (
                                            <div>
                                                {selectedNode.versionCoverage.map(coverage => (
                                                    <span key={coverage.milestoneId}>
                                                        {coverage.version}
                                                        <small>{coverage.featureCount} 个功能点</small>
                                                    </span>
                                                ))}
                                            </div>
                                        ) : (
                                            <p>后代功能点尚未指定目标版本。</p>
                                        )}
                                    </div>
                                )}
                                {selectedNode.kind === 'feature' && (
                                    <Fragment>
                                        {mismatchedTasks.length > 0 && (
                                            <div class="feature-version-diff" role="status">
                                                <div>
                                                    <strong>{mismatchedTasks.length} 个关联任务版本不一致</strong>
                                                    <span>
                                                        目标为 {targetMilestone?.version ?? '未指定'}
                                                        ；保存功能点不会自动修改这些任务。
                                                    </span>
                                                </div>
                                                <button
                                                    type="button"
                                                    class="feature-btn secondary"
                                                    disabled={busy}
                                                    onClick={() => void openSyncPreview()}
                                                >
                                                    同步到关联任务
                                                </button>
                                            </div>
                                        )}
                                        <div class="feature-create-task">
                                            <button type="button" class="feature-btn primary" onClick={breakdownWithPM}>
                                                让 AI PM 拆解为任务
                                            </button>
                                            <small>
                                                AI PM 会让新任务引用顶层需求、关联此功能点并继承
                                                {targetMilestone?.version
                                                    ? ` ${targetMilestone.version}`
                                                    : '未指定版本'}
                                                。
                                            </small>
                                        </div>
                                        <div class="feature-trace-grid">
                                            {renderLinks(
                                                '来源需求 / 缺陷',
                                                'source',
                                                sourceLinks,
                                                sourceCandidates,
                                                sourceSelection,
                                                setSourceSelection
                                            )}
                                            {renderLinks(
                                                '交付任务',
                                                'delivery',
                                                deliveryLinks,
                                                deliveryCandidates,
                                                deliverySelection,
                                                setDeliverySelection
                                            )}
                                        </div>
                                    </Fragment>
                                )}
                                <div class="feature-danger-zone">
                                    <div>
                                        <strong>删除{selectedNode.kind === 'module' ? '模块' : '功能点'}</strong>
                                        <span>
                                            {directChildren.length > 0
                                                ? `包含 ${directChildren.length} 个直接子节点，需先移动或删除它们。`
                                                : linkedItems.length > 0
                                                  ? `将解除 ${linkedItems.length} 项关联，但不会删除关联的需求或任务。`
                                                  : '删除后不可恢复。'}
                                        </span>
                                    </div>
                                    <button
                                        type="button"
                                        class="feature-btn danger"
                                        onClick={() => setDeleteTarget(selectedNode)}
                                        disabled={busy}
                                    >
                                        删除
                                    </button>
                                </div>
                            </Fragment>
                        ) : (
                            <div class="feature-detail-placeholder">从左侧选择模块或功能点查看详情。</div>
                        )}
                    </section>
                </Fragment>
            )}

            {deleteTarget && (
                <div class="ws-modal-overlay" onClick={() => !busy && setDeleteTarget(null)}>
                    <div class="ws-modal feature-delete-modal" onClick={(event: MouseEvent) => event.stopPropagation()}>
                        <div class="ws-modal-header">
                            <span>确认删除“{deleteTarget.title}”？</span>
                            <button class="ws-modal-close" onClick={() => setDeleteTarget(null)} disabled={busy}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            {directChildren.length > 0 ? (
                                <p>
                                    该模块仍有 {directChildren.length}{' '}
                                    个直接子节点，系统不会递归删除。请先返回并移动或删除子节点。
                                </p>
                            ) : linkedItems.length > 0 ? (
                                <p>
                                    删除功能点会解除 {linkedItems.length}{' '}
                                    项来源/交付关联；关联的需求、缺陷和任务不会被删除。
                                </p>
                            ) : (
                                <p>此操作不可恢复，但不会删除任何项目任务。</p>
                            )}
                        </div>
                        <div class="ws-modal-footer">
                            <button class="ws-modal-cancel" onClick={() => setDeleteTarget(null)} disabled={busy}>
                                取消
                            </button>
                            {directChildren.length === 0 && (
                                <button
                                    class="ws-modal-confirm ws-modal-confirm-danger"
                                    onClick={() => void removeNode()}
                                    disabled={busy}
                                >
                                    {busy ? '删除中…' : '确认删除'}
                                </button>
                            )}
                        </div>
                    </div>
                </div>
            )}
            {syncPreview && selectedNode?.kind === 'feature' && (
                <div class="ws-modal-overlay" onClick={() => !busy && setSyncPreview(null)}>
                    <div class="ws-modal feature-sync-modal" onClick={(event: MouseEvent) => event.stopPropagation()}>
                        <div class="ws-modal-header">
                            <span>确认同步到关联任务？</span>
                            <button class="ws-modal-close" onClick={() => setSyncPreview(null)} disabled={busy}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            <p>
                                以下 {syncPreview.tasks.length} 个任务将改为目标版本
                                <strong> {syncPreview.targetVersion || '未指定'}</strong>：
                            </p>
                            <ul class="feature-sync-impact-list">
                                {syncPreview.tasks.map(task => (
                                    <li key={task.id}>
                                        <strong>
                                            {task.number ? `#${task.number} ` : ''}
                                            {task.title}
                                        </strong>
                                        <span>
                                            {task.currentMilestone || '未指定'} →{' '}
                                            {syncPreview.targetMilestone || '未指定'}
                                        </span>
                                    </li>
                                ))}
                            </ul>
                        </div>
                        <div class="ws-modal-footer">
                            <button class="ws-modal-cancel" onClick={() => setSyncPreview(null)} disabled={busy}>
                                取消
                            </button>
                            <button
                                class="ws-modal-confirm"
                                onClick={() => void syncMilestone()}
                                disabled={busy || syncPreview.tasks.length === 0}
                            >
                                {busy ? '同步中…' : `确认同步 ${syncPreview.tasks.length} 个任务`}
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
