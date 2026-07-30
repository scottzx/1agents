import { Fragment, h } from 'preact';
import { useCallback, useEffect, useMemo, useRef, useState } from 'preact/hooks';

import { featureCatalogService } from '@1agents/core/services/featureCatalogService';
import type {
    CreateFeatureNodeInput,
    FeatureCatalog,
    FeatureItemRelation,
    FeatureMilestoneSyncPreview,
    FeatureNode,
    FeatureNodeKind,
    UpdateFeatureNodeInput,
} from '@1agents/core/types/featureCatalog';
import type { Milestone, ProjectItem } from './types';
import * as sessionStore from '../../../stores/sessionStore';
import * as taskNav from '../../../stores/taskNavStore';
import * as tabsStore from '../../../stores/tabsStore';
import * as ui from '../../../stores/uiStore';
import * as viewPrefs from '../../../stores/projectViewPrefs';
import { FsRowActionsMenu, type FsRowAction } from '../FsRowActionsMenu';
import { buildFeatureBreakdownPrompt, buildFeatureCatalogGeneratePrompt } from './aiPMWorkflow';
import { FeatureNodeForm } from './FeatureNodeForm';
import { FeatureCatalogHistoryDialog } from './FeatureCatalogHistoryDialog';
import { TaskPreviewDrawer } from './TaskPreviewDrawer';
import {
    buildFeatureTree,
    collapsibleFeatureModuleIds,
    type FeatureDropPlacement,
    type FeatureDropTarget,
    filterFeatureTree,
    flattenFeatureTree,
    formatFeatureError,
    MAX_FEATURE_MODULE_DEPTH,
    normalizeCollapsedFeatureIds,
    resolveFeatureDrop,
    siblingNodes,
    type FeatureTreeNode,
} from './featureCatalogModel';

interface FeatureCatalogViewProps {
    workspaceId: string;
    items: ProjectItem[];
    milestones: Milestone[];
    versionFilterMilestoneId?: string;
    onClearVersionFilter?: () => void;
    onOpenMilestone?: (milestoneId: string) => void;
    onCatalogChange?: (catalog: FeatureCatalog) => void;
    onItemsChange?: () => Promise<void> | void;
}

interface CreateState {
    kind: FeatureNodeKind;
    parentId: string;
}

interface MoveLinkState {
    item: ProjectItem;
    relation: FeatureItemRelation;
    fromFeatureId: string;
}

const EMPTY_CATALOG: FeatureCatalog = { nodes: [], links: [] };
const FEATURE_ACTION_ICONS = {
    rename: <span aria-hidden="true">✎</span>,
    up: <span aria-hidden="true">↑</span>,
    down: <span aria-hidden="true">↓</span>,
    module: <span aria-hidden="true">▣</span>,
    feature: <span aria-hidden="true">•</span>,
    move: <span aria-hidden="true">↪</span>,
    remove: <span aria-hidden="true">×</span>,
} as const;
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

function dropPlacementForEvent(entry: FeatureTreeNode, event: DragEvent): FeatureDropPlacement {
    const bounds = (event.currentTarget as HTMLElement).getBoundingClientRect();
    const ratio = bounds.height > 0 ? (event.clientY - bounds.top) / bounds.height : 0.5;
    if (entry.node.kind === 'feature') return ratio < 0.5 ? 'before' : 'after';
    if (ratio < 0.25) return 'before';
    if (ratio > 0.75) return 'after';
    return 'inside';
}

function featureDocumentName(path: string): string {
    return path.split(/[\\/]/).filter(Boolean).at(-1) || path;
}

function closeParentDetails(event: MouseEvent): void {
    const details = (event.currentTarget as HTMLElement).closest('details') as HTMLDetailsElement | null;
    if (details) details.open = false;
}

function TreeRow({
    entry,
    selectedId,
    onSelect,
    collapsedIds,
    filtering,
    dragDisabled,
    draggedId,
    dropTarget,
    onToggleCollapsed,
    onDragStart,
    onDragEnd,
    onDragOver,
    onDragLeave,
    onDrop,
    nodes,
    onRename,
    onMove,
    onCreateChild,
    onDelete,
}: {
    entry: FeatureTreeNode;
    selectedId: string | null;
    onSelect: (id: string) => void;
    collapsedIds: Set<string>;
    filtering: boolean;
    dragDisabled: boolean;
    draggedId: string | null;
    dropTarget: FeatureDropTarget | null;
    onToggleCollapsed: (id: string) => void;
    onDragStart: (id: string, event: DragEvent) => void;
    onDragEnd: () => void;
    onDragOver: (entry: FeatureTreeNode, event: DragEvent) => void;
    onDragLeave: (entry: FeatureTreeNode, event: DragEvent) => void;
    onDrop: (entry: FeatureTreeNode, event: DragEvent) => void;
    nodes: FeatureNode[];
    onRename: (node: FeatureNode) => void;
    onMove: (node: FeatureNode, offset: -1 | 1) => void;
    onCreateChild: (kind: FeatureNodeKind, parent: FeatureNode) => void;
    onDelete: (node: FeatureNode) => void;
}) {
    const isFeature = entry.node.kind === 'feature';
    const isCollapsed = !filtering && collapsedIds.has(entry.node.id);
    const siblings = siblingNodes(nodes, entry.node.parentId);
    const siblingIndex = siblings.findIndex(node => node.id === entry.node.id);
    const activePlacement =
        dropTarget?.targetId === entry.node.id && dropTarget.placement !== 'root' ? dropTarget.placement : '';
    const actions: FsRowAction[] = [
        {
            id: 'rename',
            labelKey: 'common.rename',
            icon: FEATURE_ACTION_ICONS.rename,
            onSelect: () => onRename(entry.node),
        },
    ];
    if (siblingIndex > 0) {
        actions.push({
            id: 'move-up',
            labelKey: 'feature.actions.moveUp',
            icon: FEATURE_ACTION_ICONS.up,
            onSelect: () => onMove(entry.node, -1),
        });
    }
    if (siblingIndex >= 0 && siblingIndex < siblings.length - 1) {
        actions.push({
            id: 'move-down',
            labelKey: 'feature.actions.moveDown',
            icon: FEATURE_ACTION_ICONS.down,
            onSelect: () => onMove(entry.node, 1),
        });
    }
    if (entry.node.kind === 'module') {
        if (entry.moduleDepth < MAX_FEATURE_MODULE_DEPTH) {
            actions.push({
                id: 'add-module',
                labelKey: 'feature.actions.addModule',
                icon: FEATURE_ACTION_ICONS.module,
                onSelect: () => onCreateChild('module', entry.node),
            });
        }
        actions.push({
            id: 'add-feature',
            labelKey: 'feature.actions.addFeature',
            icon: FEATURE_ACTION_ICONS.feature,
            onSelect: () => onCreateChild('feature', entry.node),
        });
    }
    actions.push({
        id: 'delete',
        labelKey: 'common.delete',
        icon: FEATURE_ACTION_ICONS.remove,
        danger: true,
        onSelect: () => onDelete(entry.node),
    });
    const activateRow = (event: KeyboardEvent) => {
        if (event.key !== 'Enter' && event.key !== ' ') return;
        event.preventDefault();
        onSelect(entry.node.id);
    };
    return (
        <li class={`feature-tree-item${activePlacement ? ` drop-${activePlacement}` : ''}`}>
            <div
                role="button"
                tabIndex={0}
                class={`feature-tree-row${selectedId === entry.node.id ? ' selected' : ''}${
                    draggedId === entry.node.id ? ' dragging' : ''
                }`}
                style={`--feature-depth:${Math.max(0, entry.path.length - 1)}`}
                draggable={!dragDisabled}
                onClick={() => onSelect(entry.node.id)}
                onKeyDown={activateRow}
                onDragStart={(event: DragEvent) => {
                    const target = event.target;
                    if (target instanceof Element && target.closest('button')) {
                        event.preventDefault();
                        return;
                    }
                    onDragStart(entry.node.id, event);
                }}
                onDragEnd={onDragEnd}
                onDragOver={(event: DragEvent) => onDragOver(entry, event)}
                onDragLeave={(event: DragEvent) => onDragLeave(entry, event)}
                onDrop={(event: DragEvent) => onDrop(entry, event)}
            >
                {isFeature ? (
                    <span class="feature-tree-toggle placeholder" aria-hidden="true">
                        •
                    </span>
                ) : (
                    <button
                        type="button"
                        class="feature-tree-toggle"
                        aria-label={isCollapsed ? `展开${entry.node.title}` : `折叠${entry.node.title}`}
                        aria-expanded={!isCollapsed}
                        disabled={entry.children.length === 0}
                        onClick={(event: MouseEvent) => {
                            event.stopPropagation();
                            onToggleCollapsed(entry.node.id);
                        }}
                    >
                        {entry.children.length === 0 ? '·' : isCollapsed ? '▸' : '▾'}
                    </button>
                )}
                <span class="feature-tree-copy">
                    <span class="feature-tree-title">{entry.node.title}</span>
                </span>
                {entry.node.progress && <span class="feature-tree-progress">{progressSummary(entry.node)}</span>}
                <span class="feature-tree-actions" onClick={(event: MouseEvent) => event.stopPropagation()}>
                    <FsRowActionsMenu
                        entry={{ path: entry.node.id }}
                        items={actions}
                        language={ui.language.value}
                        triggerClassName="feature-tree-actions-trigger"
                    />
                </span>
            </div>
            {entry.children.length > 0 && !isCollapsed && (
                <ul>
                    {entry.children.map(child => (
                        <TreeRow
                            key={child.node.id}
                            entry={child}
                            selectedId={selectedId}
                            onSelect={onSelect}
                            collapsedIds={collapsedIds}
                            filtering={filtering}
                            dragDisabled={dragDisabled}
                            draggedId={draggedId}
                            dropTarget={dropTarget}
                            onToggleCollapsed={onToggleCollapsed}
                            onDragStart={onDragStart}
                            onDragEnd={onDragEnd}
                            onDragOver={onDragOver}
                            onDragLeave={onDragLeave}
                            onDrop={onDrop}
                            nodes={nodes}
                            onRename={onRename}
                            onMove={onMove}
                            onCreateChild={onCreateChild}
                            onDelete={onDelete}
                        />
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
    versionFilterMilestoneId,
    onClearVersionFilter,
    onOpenMilestone,
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
    const [editing, setEditing] = useState(false);
    const [query, setQuery] = useState('');
    const [statusFilter, setStatusFilter] = useState<'all' | 'unplanned' | 'risk'>('all');
    const [draggedId, setDraggedId] = useState<string | null>(null);
    const [dropTarget, setDropTarget] = useState<FeatureDropTarget | null>(null);
    const [historyOpen, setHistoryOpen] = useState(false);
    const [renameTarget, setRenameTarget] = useState<FeatureNode | null>(null);
    const [renameValue, setRenameValue] = useState('');
    const [documentPath, setDocumentPath] = useState('');
    const [previewItem, setPreviewItem] = useState<ProjectItem | null>(null);
    const [moveLink, setMoveLink] = useState<MoveLinkState | null>(null);
    const [moveTargetId, setMoveTargetId] = useState('');
    const [treePanePercent, setTreePanePercent] = useState(50);
    const catalogViewRef = useRef<HTMLDivElement | null>(null);
    const resizingTreePane = useRef(false);

    const tree = useMemo(() => buildFeatureTree(catalog.nodes), [catalog.nodes]);
    const flatTree = useMemo(() => flattenFeatureTree(tree), [tree]);
    const entryById = useMemo(() => new Map(flatTree.map(entry => [entry.node.id, entry])), [flatTree]);
    const selectedEntry = selectedId ? entryById.get(selectedId) : undefined;
    const selectedNode = selectedEntry?.node;
    const filtersActive = query.trim() !== '' || statusFilter !== 'all' || !!versionFilterMilestoneId;
    const storedCollapsedIds = viewPrefs.getPrefs(workspaceId).featureCatalogCollapsed;
    const persistedCollapsedIds = useMemo(() => normalizeCollapsedFeatureIds(storedCollapsedIds), [storedCollapsedIds]);
    const collapsedIds = new Set(persistedCollapsedIds);
    const dragDisabled = busy || filtersActive || ui.isMobile.value;
    const filteredTree = useMemo(() => {
        const cleanQuery = query.trim().toLocaleLowerCase();
        if (!cleanQuery && statusFilter === 'all' && !versionFilterMilestoneId) return tree;
        return filterFeatureTree(tree, entry => {
            const matchesQuery =
                !cleanQuery ||
                entry.node.title.toLocaleLowerCase().includes(cleanQuery) ||
                entry.path.join(' / ').toLocaleLowerCase().includes(cleanQuery);
            const matchesStatus =
                statusFilter === 'all' ||
                (statusFilter === 'unplanned' && entry.node.progress?.status === 'unplanned') ||
                (statusFilter === 'risk' && entry.node.progress?.status === 'replan');
            const matchesVersion =
                !versionFilterMilestoneId ||
                (entry.node.kind === 'feature' && entry.node.targetMilestoneId === versionFilterMilestoneId);
            return matchesQuery && matchesStatus && matchesVersion;
        });
    }, [query, statusFilter, tree, versionFilterMilestoneId]);
    const versionFilterMilestone = milestones.find(milestone => milestone.id === versionFilterMilestoneId);

    useEffect(() => {
        if (loading) return;
        const validIDs = new Set(catalog.nodes.filter(node => node.kind === 'module').map(node => node.id));
        const validCollapsedIds = persistedCollapsedIds.filter(id => validIDs.has(id));
        if (validCollapsedIds.length === persistedCollapsedIds.length) return;
        viewPrefs.updatePrefs(workspaceId, { featureCatalogCollapsed: validCollapsedIds });
    }, [catalog.nodes, loading, persistedCollapsedIds, workspaceId]);

    useEffect(() => {
        if (!filtersActive && !busy) return;
        setDraggedId(null);
        setDropTarget(null);
    }, [filtersActive, busy, workspaceId]);

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
        setEditing(false);
        setQuery('');
        setStatusFilter('all');
        setDraggedId(null);
        setDropTarget(null);
        setRenameTarget(null);
        setRenameValue('');
        setDocumentPath('');
        setPreviewItem(null);
        setMoveLink(null);
        setMoveTargetId('');
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
        setEditing(false);
        setDocumentPath('');
        setMoveLink(null);
        setMoveTargetId('');
    }, [selectedId]);

    useEffect(() => {
        if (!versionFilterMilestoneId) return;
        const firstMatch = catalog.nodes.find(
            node => node.kind === 'feature' && node.targetMilestoneId === versionFilterMilestoneId
        );
        if (firstMatch) setSelectedId(firstMatch.id);
    }, [versionFilterMilestoneId, catalog.nodes]);

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

    const handleHistoryRestored = async () => {
        await loadCatalog();
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

    const toggleCollapsed = useCallback(
        (id: string) => {
            const projectIDs = new Set(
                normalizeCollapsedFeatureIds(viewPrefs.getPrefs(workspaceId).featureCatalogCollapsed)
            );
            if (projectIDs.has(id)) projectIDs.delete(id);
            else projectIDs.add(id);
            viewPrefs.updatePrefs(workspaceId, { featureCatalogCollapsed: [...projectIDs] });
        },
        [workspaceId]
    );

    const setCollapsedIds = useCallback(
        (ids: Iterable<string>) => {
            viewPrefs.updatePrefs(workspaceId, { featureCatalogCollapsed: [...ids] });
        },
        [workspaceId]
    );

    const collapseAll = useCallback(() => {
        setCollapsedIds(collapsibleFeatureModuleIds(tree));
    }, [tree, setCollapsedIds]);

    const collapseToDepth = useCallback(
        (depth: number) => {
            setCollapsedIds(collapsibleFeatureModuleIds(tree, depth));
        },
        [tree, setCollapsedIds]
    );

    const expandAll = useCallback(() => setCollapsedIds([]), [setCollapsedIds]);

    const clearDrag = useCallback(() => {
        setDraggedId(null);
        setDropTarget(null);
    }, []);

    const handleDragStart = useCallback(
        (id: string, event: DragEvent) => {
            if (dragDisabled) {
                event.preventDefault();
                return;
            }
            event.stopPropagation();
            event.dataTransfer?.setData('text/plain', id);
            if (event.dataTransfer) event.dataTransfer.effectAllowed = 'move';
            setDraggedId(id);
            setDropTarget(null);
        },
        [dragDisabled]
    );

    const handleDragOver = useCallback(
        (entry: FeatureTreeNode, event: DragEvent) => {
            event.stopPropagation();
            if (!draggedId || dragDisabled) {
                setDropTarget(null);
                return;
            }
            const target: FeatureDropTarget = {
                targetId: entry.node.id,
                placement: dropPlacementForEvent(entry, event),
            };
            if (!resolveFeatureDrop(catalog.nodes, draggedId, target)) {
                setDropTarget(null);
                return;
            }
            event.preventDefault();
            if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
            setDropTarget(target);
        },
        [catalog.nodes, dragDisabled, draggedId]
    );

    const handleDragLeave = useCallback((entry: FeatureTreeNode, event: DragEvent) => {
        event.stopPropagation();
        const nextTarget = event.relatedTarget;
        if (nextTarget instanceof Node && (event.currentTarget as HTMLElement).contains(nextTarget)) return;
        setDropTarget(current => (current?.targetId === entry.node.id ? null : current));
    }, []);

    const moveByDrop = useCallback(
        async (target: FeatureDropTarget) => {
            if (!draggedId) return;
            const move = resolveFeatureDrop(catalog.nodes, draggedId, target);
            if (!move) {
                clearDrag();
                return;
            }
            setBusy(true);
            setError('');
            try {
                await featureCatalogService.update(workspaceId, draggedId, move);
            } catch (cause) {
                const message = `移动失败：${formatFeatureError(cause)}`;
                setError(message);
                ui.showToast(message);
                setBusy(false);
                clearDrag();
                return;
            }
            if (target.placement === 'inside' && target.targetId) {
                const projectIDs = new Set(
                    normalizeCollapsedFeatureIds(viewPrefs.getPrefs(workspaceId).featureCatalogCollapsed)
                );
                if (projectIDs.delete(target.targetId)) {
                    viewPrefs.updatePrefs(workspaceId, { featureCatalogCollapsed: [...projectIDs] });
                }
            }
            try {
                await loadCatalog();
                ui.showToast('节点已移动。');
            } catch {
                const message = '移动已保存，但目录刷新失败。请重新加载目录。';
                setError(message);
                ui.showToast(message);
            } finally {
                setBusy(false);
                clearDrag();
            }
        },
        [catalog.nodes, clearDrag, draggedId, loadCatalog, workspaceId]
    );

    const handleDrop = useCallback(
        (entry: FeatureTreeNode, event: DragEvent) => {
            event.stopPropagation();
            const target: FeatureDropTarget = {
                targetId: entry.node.id,
                placement: dropPlacementForEvent(entry, event),
            };
            if (!draggedId || dragDisabled || !resolveFeatureDrop(catalog.nodes, draggedId, target)) return;
            event.preventDefault();
            void moveByDrop(target);
        },
        [catalog.nodes, dragDisabled, draggedId, moveByDrop]
    );

    const handleRootDragOver = useCallback(
        (event: DragEvent) => {
            event.stopPropagation();
            const target: FeatureDropTarget = { placement: 'root' };
            if (!draggedId || dragDisabled || !resolveFeatureDrop(catalog.nodes, draggedId, target)) {
                setDropTarget(null);
                return;
            }
            event.preventDefault();
            if (event.dataTransfer) event.dataTransfer.dropEffect = 'move';
            setDropTarget(target);
        },
        [catalog.nodes, dragDisabled, draggedId]
    );

    const handleRootDrop = useCallback(
        (event: DragEvent) => {
            event.stopPropagation();
            const target: FeatureDropTarget = { placement: 'root' };
            if (!draggedId || dragDisabled || !resolveFeatureDrop(catalog.nodes, draggedId, target)) return;
            event.preventDefault();
            void moveByDrop(target);
        },
        [catalog.nodes, dragDisabled, draggedId, moveByDrop]
    );

    const openCreate = (kind: FeatureNodeKind, parentId = '') => {
        setCreateState({ kind, parentId });
        setError('');
        setMobileDetailOpen(true);
    };

    const openCreateForParent = (kind: FeatureNodeKind, parent: FeatureNode) => {
        setSelectedId(parent.id);
        openCreate(kind, parent.id);
    };

    const openRename = (node: FeatureNode) => {
        setRenameTarget(node);
        setRenameValue(node.title);
    };

    const selectNode = (id: string) => {
        setSelectedId(id);
        setCreateState(null);
        setEditing(false);
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
        const ok = await runMutation(async () => {
            await featureCatalogService.update(workspaceId, selectedNode.id, input);
        }, '节点已保存。');
        if (ok) setEditing(false);
    };

    const moveNode = async (node: FeatureNode, offset: -1 | 1) => {
        const siblings = siblingNodes(catalog.nodes, node.parentId);
        const index = siblings.findIndex(sibling => sibling.id === node.id);
        const target = index + offset;
        if (index < 0 || target < 0 || target >= siblings.length) return;
        await runMutation(
            async () => {
                await featureCatalogService.update(workspaceId, node.id, { position: target });
            },
            offset < 0 ? '已上移。' : '已下移。'
        );
    };

    const moveSelected = async (offset: -1 | 1) => {
        if (selectedNode) await moveNode(selectedNode, offset);
    };

    const renameNode = async () => {
        if (!renameTarget) return;
        const title = renameValue.trim();
        if (!title || title === renameTarget.title) {
            setRenameTarget(null);
            return;
        }
        const ok = await runMutation(async () => {
            await featureCatalogService.update(workspaceId, renameTarget.id, { title });
        }, '节点已重命名。');
        if (ok) setRenameTarget(null);
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

    const updateDocuments = async (documents: string[], successMessage: string) => {
        if (!selectedNode) return false;
        return runMutation(async () => {
            await featureCatalogService.update(workspaceId, selectedNode.id, { documents });
        }, successMessage);
    };

    const addDocument = async () => {
        if (!selectedNode) return;
        const path = documentPath.trim();
        if (!path || selectedNode.documents?.includes(path)) return;
        const ok = await updateDocuments([...(selectedNode.documents ?? []), path], '文档已关联。');
        if (ok) setDocumentPath('');
    };

    const removeDocument = async (path: string) => {
        if (!selectedNode) return;
        await updateDocuments(
            (selectedNode.documents ?? []).filter(document => document !== path),
            '文档关联已移除。'
        );
    };

    const openDocument = (path: string) => {
        void tabsStore.openPreviewTab(path, featureDocumentName(path));
    };

    const moveLinkedItem = async () => {
        if (!moveLink || !moveTargetId || moveTargetId === moveLink.fromFeatureId) return;
        const ok = await runMutation(async () => {
            await featureCatalogService.moveItem(
                workspaceId,
                moveLink.fromFeatureId,
                moveTargetId,
                moveLink.item.id,
                moveLink.relation
            );
        }, '卡片已移动到新的功能点。');
        if (ok) {
            setMoveLink(null);
            setMoveTargetId('');
        }
    };

    const updateTreePaneWidth = (clientX: number) => {
        const bounds = catalogViewRef.current?.getBoundingClientRect();
        if (!bounds || bounds.width <= 0) return;
        const next = Math.max(32, Math.min(68, ((clientX - bounds.left) / bounds.width) * 100));
        setTreePanePercent(next);
    };

    const startTreePaneResize = (event: PointerEvent) => {
        if (ui.isMobile.value) return;
        resizingTreePane.current = true;
        (event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
        updateTreePaneWidth(event.clientX);
        event.preventDefault();
    };

    const continueTreePaneResize = (event: PointerEvent) => {
        if (!resizingTreePane.current) return;
        updateTreePaneWidth(event.clientX);
    };

    const stopTreePaneResize = (event: PointerEvent) => {
        resizingTreePane.current = false;
        const handle = event.currentTarget as HTMLElement;
        if (handle.hasPointerCapture(event.pointerId)) handle.releasePointerCapture(event.pointerId);
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
            <div class="feature-catalog-shell">
                <div class="feature-catalog-empty">
                    <span class="feature-empty-icon">◇</span>
                    <h3>尚未建立功能蓝图</h3>
                    <p>从需求和架构设计出发，整理最多九级模块及功能点，再将功能点分解为可执行任务。</p>
                    {error && <div class="feature-form-error">{error}</div>}
                    <div class="feature-empty-actions">
                        <button type="button" class="feature-btn primary" onClick={generateWithPM}>
                            与 AI PM 一起生成
                        </button>
                        <button type="button" class="feature-btn secondary" onClick={() => openCreate('module')}>
                            手动新增一级模块
                        </button>
                        <button type="button" class="feature-btn secondary" onClick={() => setHistoryOpen(true)}>
                            历史版本
                        </button>
                    </div>
                    <div class="feature-root-drop disabled">空蓝图的一级目录落点</div>
                </div>
                <FeatureCatalogHistoryDialog
                    workspaceId={workspaceId}
                    open={historyOpen}
                    onClose={() => setHistoryOpen(false)}
                    onRestored={handleHistoryRestored}
                />
            </div>
        );
    }

    const siblings = selectedNode ? siblingNodes(catalog.nodes, selectedNode.parentId) : [];
    const selectedIndex = selectedNode ? siblings.findIndex(node => node.id === selectedNode.id) : -1;
    const linkedItems = selectedNode ? catalog.links.filter(link => link.featureId === selectedNode.id) : [];
    const deleteTargetChildren = deleteTarget ? catalog.nodes.filter(node => node.parentId === deleteTarget.id) : [];
    const deleteTargetLinks = deleteTarget ? catalog.links.filter(link => link.featureId === deleteTarget.id) : [];
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
    const featurePointEntries = flatTree.filter(entry => entry.node.kind === 'feature');

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
                        const moveActions: FsRowAction[] = [
                            {
                                id: 'move-card',
                                labelKey: 'feature.actions.moveCard',
                                icon: FEATURE_ACTION_ICONS.move,
                                onSelect: () => {
                                    setMoveLink({
                                        item,
                                        relation,
                                        fromFeatureId: selectedNode!.id,
                                    });
                                    setMoveTargetId('');
                                },
                            },
                        ];
                        return (
                            <div key={`${link.itemId}:${relation}`} class="feature-trace-row">
                                <button type="button" class="feature-trace-main" onClick={() => setPreviewItem(item)}>
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
                                </button>
                                <span
                                    class="feature-trace-actions"
                                    onClick={(event: MouseEvent) => event.stopPropagation()}
                                >
                                    <button
                                        type="button"
                                        class="feature-trace-remove"
                                        title="移除关联"
                                        aria-label={`移除“${item.title}”关联`}
                                        disabled={busy}
                                        onClick={() => void unlinkItem(relation, item.id)}
                                    >
                                        ×
                                    </button>
                                    <FsRowActionsMenu
                                        entry={{ path: item.id }}
                                        items={moveActions}
                                        language={ui.language.value}
                                        triggerClassName="feature-trace-more-trigger"
                                    />
                                </span>
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
        <div class="feature-catalog-shell">
            <header class="feature-catalog-page-header">
                <div>
                    <h2>功能蓝图</h2>
                    <p>管理产品由哪些模块与功能构成，以及它们从哪里来、由什么任务交付。</p>
                </div>
                <div class="feature-pane-actions">
                    <button type="button" class="feature-btn secondary" onClick={() => setHistoryOpen(true)}>
                        历史版本
                    </button>
                    <button type="button" class="feature-btn secondary" onClick={generateWithPM}>
                        与 AI PM 一起整理
                    </button>
                    <button type="button" class="feature-btn primary" onClick={() => openCreate('module')}>
                        + 一级模块
                    </button>
                    <details class="feature-export-menu">
                        <summary aria-label="更多功能蓝图操作">···</summary>
                        <div>
                            <button
                                type="button"
                                disabled={busy}
                                onClick={(event: MouseEvent) => {
                                    collapseAll();
                                    closeParentDetails(event);
                                }}
                            >
                                全部折叠
                            </button>
                            {[2, 3, 4, 5, 6, 7, 8].map(depth => (
                                <button
                                    type="button"
                                    key={depth}
                                    disabled={busy}
                                    onClick={(event: MouseEvent) => {
                                        collapseToDepth(depth);
                                        closeParentDetails(event);
                                    }}
                                >
                                    折叠到 {depth} 级
                                </button>
                            ))}
                            <button
                                type="button"
                                disabled={busy}
                                onClick={(event: MouseEvent) => {
                                    expandAll();
                                    closeParentDetails(event);
                                }}
                            >
                                全部展开
                            </button>
                            <span class="feature-menu-separator" />
                            <button
                                type="button"
                                disabled={busy}
                                onClick={(event: MouseEvent) => {
                                    closeParentDetails(event);
                                    void handleExport('markdown');
                                }}
                            >
                                导出 Markdown
                            </button>
                            <button
                                type="button"
                                disabled={busy}
                                onClick={(event: MouseEvent) => {
                                    closeParentDetails(event);
                                    void handleExport('json');
                                }}
                            >
                                导出 JSON
                            </button>
                        </div>
                    </details>
                </div>
            </header>

            {versionFilterMilestoneId && (
                <div class="feature-context-banner">
                    <span>
                        正在查看版本{' '}
                        <strong>{versionFilterMilestone?.version || versionFilterMilestone?.name || '未知版本'}</strong>{' '}
                        的功能范围
                    </span>
                    <button type="button" onClick={onClearVersionFilter}>
                        查看全部 ×
                    </button>
                </div>
            )}

            <div
                ref={catalogViewRef}
                class={`feature-catalog-view${detailOpen ? ' detail-open' : ''}`}
                style={`--feature-tree-width:${treePanePercent}%`}
            >
                <section class="feature-catalog-tree-pane" aria-label="功能蓝图树">
                    <div class="feature-tree-tools">
                        <input
                            type="search"
                            value={query}
                            placeholder="搜索模块或功能点"
                            aria-label="搜索模块或功能点"
                            onInput={(event: Event) => setQuery((event.currentTarget as HTMLInputElement).value)}
                        />
                        <div class="feature-filter-chips" aria-label="状态筛选">
                            <button
                                type="button"
                                class={statusFilter === 'all' ? 'active' : ''}
                                onClick={() => setStatusFilter('all')}
                            >
                                全部
                            </button>
                            <button
                                type="button"
                                class={statusFilter === 'unplanned' ? 'active' : ''}
                                onClick={() => setStatusFilter('unplanned')}
                            >
                                未拆解
                            </button>
                            <button
                                type="button"
                                class={statusFilter === 'risk' ? 'active' : ''}
                                onClick={() => setStatusFilter('risk')}
                            >
                                风险
                            </button>
                        </div>
                    </div>
                    <div class="feature-tree-count">
                        <span>{catalog.nodes.length} 个节点</span>
                        {filtersActive && <span>筛选中，拖拽已暂停</span>}
                    </div>
                    {error && (
                        <div class="feature-tree-error" role="alert">
                            <span>{error}</span>
                            {error.includes('刷新失败') && (
                                <button type="button" disabled={busy} onClick={() => void loadCatalog()}>
                                    重新加载
                                </button>
                            )}
                        </div>
                    )}
                    {filteredTree.length > 0 ? (
                        <ul class="feature-tree">
                            {filteredTree.map(entry => (
                                <TreeRow
                                    key={entry.node.id}
                                    entry={entry}
                                    selectedId={selectedId}
                                    onSelect={selectNode}
                                    collapsedIds={collapsedIds}
                                    filtering={filtersActive}
                                    dragDisabled={dragDisabled}
                                    draggedId={draggedId}
                                    dropTarget={dropTarget}
                                    onToggleCollapsed={toggleCollapsed}
                                    onDragStart={handleDragStart}
                                    onDragEnd={clearDrag}
                                    onDragOver={handleDragOver}
                                    onDragLeave={handleDragLeave}
                                    onDrop={handleDrop}
                                    nodes={catalog.nodes}
                                    onRename={openRename}
                                    onMove={(node, offset) => void moveNode(node, offset)}
                                    onCreateChild={openCreateForParent}
                                    onDelete={setDeleteTarget}
                                />
                            ))}
                        </ul>
                    ) : (
                        <div class="feature-tree-empty">没有符合当前条件的节点。</div>
                    )}
                    {catalog.nodes.length > 0 && (
                        <div
                            class={`feature-root-drop${dropTarget?.placement === 'root' ? ' active' : ''}${
                                dragDisabled ? ' disabled' : ''
                            }`}
                            onDragOver={handleRootDragOver}
                            onDragLeave={(event: DragEvent) => {
                                const nextTarget = event.relatedTarget;
                                if (
                                    nextTarget instanceof Node &&
                                    (event.currentTarget as HTMLElement).contains(nextTarget)
                                ) {
                                    return;
                                }
                                setDropTarget(current => (current?.placement === 'root' ? null : current));
                            }}
                            onDrop={handleRootDrop}
                        >
                            将模块拖到这里，移回一级目录
                        </div>
                    )}
                </section>

                <div
                    class="feature-catalog-resizer"
                    role="separator"
                    aria-label="调整功能列表和功能详情宽度"
                    aria-orientation="vertical"
                    aria-valuemin={32}
                    aria-valuemax={68}
                    aria-valuenow={Math.round(treePanePercent)}
                    tabIndex={0}
                    onPointerDown={startTreePaneResize}
                    onPointerMove={continueTreePaneResize}
                    onPointerUp={stopTreePaneResize}
                    onPointerCancel={stopTreePaneResize}
                    onKeyDown={(event: KeyboardEvent) => {
                        if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home') return;
                        event.preventDefault();
                        if (event.key === 'Home') setTreePanePercent(50);
                        else
                            setTreePanePercent(current =>
                                Math.max(32, Math.min(68, current + (event.key === 'ArrowLeft' ? -2 : 2)))
                            );
                    }}
                >
                    <span />
                </div>

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
                                    {editing ? (
                                        <button
                                            type="button"
                                            class="feature-btn secondary"
                                            onClick={() => setEditing(false)}
                                            disabled={busy}
                                        >
                                            取消编辑
                                        </button>
                                    ) : (
                                        <button
                                            type="button"
                                            class="feature-btn secondary"
                                            onClick={() => setEditing(true)}
                                        >
                                            编辑
                                        </button>
                                    )}
                                    <details class="feature-node-menu">
                                        <summary aria-label="更多节点操作">···</summary>
                                        <div>
                                            <button
                                                type="button"
                                                disabled={busy || selectedIndex <= 0}
                                                onClick={() => void moveSelected(-1)}
                                            >
                                                上移
                                            </button>
                                            <button
                                                type="button"
                                                disabled={busy || selectedIndex === siblings.length - 1}
                                                onClick={() => void moveSelected(1)}
                                            >
                                                下移
                                            </button>
                                            <button
                                                type="button"
                                                class="danger"
                                                disabled={busy}
                                                onClick={() => setDeleteTarget(selectedNode)}
                                            >
                                                删除
                                            </button>
                                        </div>
                                    </details>
                                </div>
                            </div>

                            {editing ? (
                                <FeatureNodeForm
                                    key={`edit:${selectedNode.id}`}
                                    kind={selectedNode.kind}
                                    nodes={catalog.nodes}
                                    milestones={milestones}
                                    node={selectedNode}
                                    busy={busy}
                                    error={error}
                                    onCancel={() => setEditing(false)}
                                    onUpdate={updateNode}
                                />
                            ) : (
                                <Fragment>
                                    <p class="feature-read-description">
                                        {selectedNode.description || '尚未补充该节点的范围与边界。'}
                                    </p>

                                    <section class="feature-documents" aria-label="关联文档">
                                        <div class="feature-documents-heading">
                                            <strong>关联文档</strong>
                                            <span>{selectedNode.documents?.length ?? 0}</span>
                                        </div>
                                        {selectedNode.documents?.length ? (
                                            <div class="feature-document-list">
                                                {selectedNode.documents.map(path => (
                                                    <div class="feature-document-row" key={path}>
                                                        <button
                                                            type="button"
                                                            class="feature-document-open"
                                                            title={path}
                                                            onClick={() => openDocument(path)}
                                                        >
                                                            <span aria-hidden="true">▤</span>
                                                            <span>
                                                                <strong>{featureDocumentName(path)}</strong>
                                                                <small>{path}</small>
                                                            </span>
                                                        </button>
                                                        <button
                                                            type="button"
                                                            class="feature-document-remove"
                                                            aria-label={`移除文档“${featureDocumentName(path)}”`}
                                                            title="移除关联"
                                                            disabled={busy}
                                                            onClick={() => void removeDocument(path)}
                                                        >
                                                            ×
                                                        </button>
                                                    </div>
                                                ))}
                                            </div>
                                        ) : (
                                            <p class="feature-documents-empty">
                                                可关联项目根目录相对路径或绝对路径，点击后在右侧文件抽屉中预览。
                                            </p>
                                        )}
                                        <div class="feature-document-add">
                                            <input
                                                type="text"
                                                value={documentPath}
                                                placeholder="例如 docs/spec.md 或 /绝对路径/spec.md"
                                                aria-label="文档路径"
                                                disabled={busy}
                                                onInput={(event: Event) =>
                                                    setDocumentPath((event.currentTarget as HTMLInputElement).value)
                                                }
                                                onKeyDown={(event: KeyboardEvent) => {
                                                    if (event.key !== 'Enter') return;
                                                    event.preventDefault();
                                                    void addDocument();
                                                }}
                                            />
                                            <button
                                                type="button"
                                                class="feature-btn secondary"
                                                disabled={busy || !documentPath.trim()}
                                                onClick={() => void addDocument()}
                                            >
                                                关联
                                            </button>
                                        </div>
                                    </section>

                                    {selectedNode.progress && (
                                        <div class="feature-read-summary">
                                            <div>
                                                <span>状态</span>
                                                <strong>{PROGRESS_LABELS[selectedNode.progress.status]}</strong>
                                            </div>
                                            <div>
                                                <span>交付</span>
                                                <strong>
                                                    {selectedNode.progress.progressPercent === null
                                                        ? '—'
                                                        : `${selectedNode.progress.progressPercent}%`}
                                                </strong>
                                                <small>
                                                    {selectedNode.progress.completedTasks}/
                                                    {selectedNode.progress.totalTasks} 个任务
                                                </small>
                                            </div>
                                            <div>
                                                <span>{selectedNode.kind === 'module' ? '功能覆盖' : '目标版本'}</span>
                                                {selectedNode.kind === 'module' ? (
                                                    <strong>
                                                        {selectedNode.progress.coveredFeatures}/
                                                        {selectedNode.progress.totalFeatures}
                                                    </strong>
                                                ) : targetMilestone && onOpenMilestone ? (
                                                    <button
                                                        type="button"
                                                        onClick={() => onOpenMilestone(targetMilestone.id)}
                                                    >
                                                        {targetMilestone.version || targetMilestone.name} →
                                                    </button>
                                                ) : (
                                                    <strong>
                                                        {targetMilestone?.version || targetMilestone?.name || '未指定'}
                                                    </strong>
                                                )}
                                            </div>
                                        </div>
                                    )}

                                    {selectedNode.kind === 'module' && (
                                        <Fragment>
                                            <div class="feature-add-row">
                                                {selectedEntry.moduleDepth < MAX_FEATURE_MODULE_DEPTH && (
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
                                            <details class="feature-version-coverage">
                                                <summary>后代功能点版本覆盖</summary>
                                                {selectedNode.versionCoverage?.length ? (
                                                    <div>
                                                        {selectedNode.versionCoverage.map(coverage => (
                                                            <button
                                                                type="button"
                                                                key={coverage.milestoneId}
                                                                onClick={() => onOpenMilestone?.(coverage.milestoneId)}
                                                            >
                                                                {coverage.version}
                                                                <small>{coverage.featureCount} 个功能点</small>
                                                            </button>
                                                        ))}
                                                    </div>
                                                ) : (
                                                    <p>后代功能点尚未指定目标版本。</p>
                                                )}
                                            </details>
                                        </Fragment>
                                    )}

                                    {selectedNode.kind === 'feature' && (
                                        <Fragment>
                                            {mismatchedTasks.length > 0 && (
                                                <div class="feature-version-diff" role="status">
                                                    <div>
                                                        <strong>{mismatchedTasks.length} 个关联任务版本不一致</strong>
                                                        <span>
                                                            目标为 {targetMilestone?.version ?? '未指定'}
                                                            ；修改功能点不会自动改写这些任务。
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
                                                <button
                                                    type="button"
                                                    class="feature-btn primary"
                                                    onClick={breakdownWithPM}
                                                >
                                                    让 AI PM 拆解为任务
                                                </button>
                                                <small>
                                                    新任务会关联此功能点，并继承
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
                                </Fragment>
                            )}
                        </Fragment>
                    ) : (
                        <div class="feature-detail-placeholder">从左侧选择模块或功能点查看详情。</div>
                    )}
                </section>
            </div>

            <TaskPreviewDrawer
                open={previewItem !== null}
                task={previewItem}
                onClose={() => setPreviewItem(null)}
                onOpenFull={taskId => taskNav.openTaskById(workspaceId, taskId)}
            />
            {renameTarget && (
                <div class="ws-modal-overlay" onClick={() => !busy && setRenameTarget(null)}>
                    <div class="ws-modal feature-rename-modal" onClick={(event: MouseEvent) => event.stopPropagation()}>
                        <div class="ws-modal-header">
                            <span>重命名{renameTarget.kind === 'module' ? '模块' : '功能点'}</span>
                            <button class="ws-modal-close" onClick={() => setRenameTarget(null)} disabled={busy}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            <label>
                                <span>名称</span>
                                <input
                                    type="text"
                                    value={renameValue}
                                    autoFocus
                                    disabled={busy}
                                    onInput={(event: Event) =>
                                        setRenameValue((event.currentTarget as HTMLInputElement).value)
                                    }
                                    onKeyDown={(event: KeyboardEvent) => {
                                        if (event.key === 'Escape') setRenameTarget(null);
                                        if (event.key === 'Enter') {
                                            event.preventDefault();
                                            void renameNode();
                                        }
                                    }}
                                />
                            </label>
                        </div>
                        <div class="ws-modal-footer">
                            <button class="ws-modal-cancel" onClick={() => setRenameTarget(null)} disabled={busy}>
                                取消
                            </button>
                            <button
                                class="ws-modal-confirm"
                                onClick={() => void renameNode()}
                                disabled={busy || !renameValue.trim()}
                            >
                                {busy ? '保存中…' : '保存'}
                            </button>
                        </div>
                    </div>
                </div>
            )}
            {moveLink && (
                <div class="ws-modal-overlay" onClick={() => !busy && setMoveLink(null)}>
                    <div
                        class="ws-modal feature-move-card-modal"
                        onClick={(event: MouseEvent) => event.stopPropagation()}
                    >
                        <div class="ws-modal-header">
                            <span>移动卡片</span>
                            <button class="ws-modal-close" onClick={() => setMoveLink(null)} disabled={busy}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            <p>
                                将“{moveLink.item.number ? `#${moveLink.item.number} ` : ''}
                                {moveLink.item.title}”移动到：
                            </p>
                            <select
                                value={moveTargetId}
                                disabled={busy}
                                onChange={(event: Event) =>
                                    setMoveTargetId((event.currentTarget as HTMLSelectElement).value)
                                }
                            >
                                <option value="">选择目标功能点…</option>
                                {featurePointEntries
                                    .filter(entry => entry.node.id !== moveLink.fromFeatureId)
                                    .map(entry => (
                                        <option key={entry.node.id} value={entry.node.id}>
                                            {entry.path.join(' / ')}
                                        </option>
                                    ))}
                            </select>
                        </div>
                        <div class="ws-modal-footer">
                            <button class="ws-modal-cancel" onClick={() => setMoveLink(null)} disabled={busy}>
                                取消
                            </button>
                            <button
                                class="ws-modal-confirm"
                                onClick={() => void moveLinkedItem()}
                                disabled={busy || !moveTargetId}
                            >
                                {busy ? '移动中…' : '确认移动'}
                            </button>
                        </div>
                    </div>
                </div>
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
                            {deleteTargetChildren.length > 0 ? (
                                <p>
                                    该模块仍有 {deleteTargetChildren.length}{' '}
                                    个直接子节点，系统不会递归删除。请先返回并移动或删除子节点。
                                </p>
                            ) : deleteTargetLinks.length > 0 ? (
                                <p>
                                    删除功能点会解除 {deleteTargetLinks.length}{' '}
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
                            {deleteTargetChildren.length === 0 && (
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
            <FeatureCatalogHistoryDialog
                workspaceId={workspaceId}
                open={historyOpen}
                onClose={() => setHistoryOpen(false)}
                onRestored={handleHistoryRestored}
            />
        </div>
    );
}
