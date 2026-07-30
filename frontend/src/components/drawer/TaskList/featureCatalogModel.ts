import type {
    FeatureCatalog,
    FeatureItemRelation,
    FeatureNode,
    FeatureNodeKind,
} from '@1agents/core/types/featureCatalog';

export interface FeatureTreeNode {
    node: FeatureNode;
    moduleDepth: number;
    path: string[];
    children: FeatureTreeNode[];
}

export const MAX_FEATURE_MODULE_DEPTH = 9;

export type FeatureDropPlacement = 'before' | 'inside' | 'after' | 'root';

export interface FeatureDropTarget {
    targetId?: string;
    placement: FeatureDropPlacement;
}

export interface FeatureDropMove {
    parentId?: string;
    position: number;
}

function compareNodes(a: FeatureNode, b: FeatureNode): number {
    return a.position - b.position || a.createdAt.localeCompare(b.createdAt) || a.id.localeCompare(b.id);
}

export function buildFeatureTree(nodes: FeatureNode[]): FeatureTreeNode[] {
    const byParent = new Map<string, FeatureNode[]>();
    const ids = new Set(nodes.map(node => node.id));
    for (const node of nodes) {
        const parentId = node.parentId && ids.has(node.parentId) ? node.parentId : '';
        const siblings = byParent.get(parentId) ?? [];
        siblings.push(node);
        byParent.set(parentId, siblings);
    }
    for (const siblings of byParent.values()) siblings.sort(compareNodes);

    const visit = (node: FeatureNode, moduleDepth: number, parentPath: string[]): FeatureTreeNode => {
        const path = [...parentPath, node.title];
        const depth = node.kind === 'module' ? moduleDepth + 1 : moduleDepth;
        return {
            node,
            moduleDepth: depth,
            path,
            children: (byParent.get(node.id) ?? []).map(child => visit(child, depth, path)),
        };
    };

    return (byParent.get('') ?? []).map(node => visit(node, 0, []));
}

export function flattenFeatureTree(tree: FeatureTreeNode[]): FeatureTreeNode[] {
    const result: FeatureTreeNode[] = [];
    const visit = (entry: FeatureTreeNode) => {
        result.push(entry);
        entry.children.forEach(visit);
    };
    tree.forEach(visit);
    return result;
}

/** Module ids that must be collapsed to keep the tree visible through the
 * requested level. Level-one modules remain open for "collapse to 2", while
 * level-two modules close over their deeper descendants. */
export function collapsibleFeatureModuleIds(tree: FeatureTreeNode[], minimumDepth = 1): string[] {
    return flattenFeatureTree(tree)
        .filter(entry => entry.node.kind === 'module' && entry.children.length > 0 && entry.moduleDepth >= minimumDepth)
        .map(entry => entry.node.id);
}

export function normalizeCollapsedFeatureIds(value: unknown): string[] {
    if (!Array.isArray(value)) return [];
    return [...new Set(value.filter((id): id is string => typeof id === 'string' && id !== ''))];
}

/** Keep matching nodes together with every ancestor required to preserve the
 * catalog hierarchy. This powers search, status lenses, and version-context
 * jumps without flattening results into a second navigation model. */
export function filterFeatureTree(
    tree: FeatureTreeNode[],
    matches: (entry: FeatureTreeNode) => boolean
): FeatureTreeNode[] {
    return tree.flatMap(entry => {
        const children = filterFeatureTree(entry.children, matches);
        if (!matches(entry) && children.length === 0) return [];
        return [{ ...entry, children }];
    });
}

export function featureEntriesById(nodes: FeatureNode[]): Map<string, FeatureTreeNode> {
    return new Map(flattenFeatureTree(buildFeatureTree(nodes)).map(entry => [entry.node.id, entry]));
}

/** Full module paths for an item's linked feature points. A cross-module task
 * intentionally returns several paths so the task grid can show it in each
 * relevant module group. */
export function featureModulePathsForItem(
    catalog: FeatureCatalog,
    itemId: string,
    relation: FeatureItemRelation
): string[] {
    const entries = featureEntriesById(catalog.nodes);
    const paths = new Set<string>();
    for (const link of catalog.links) {
        if (link.itemId !== itemId || link.relation !== relation) continue;
        const entry = entries.get(link.featureId);
        if (!entry || entry.node.kind !== 'feature') continue;
        const modulePath = entry.path.slice(0, -1).join(' / ');
        if (modulePath) paths.add(modulePath);
    }
    return [...paths].sort((a, b) => a.localeCompare(b));
}

export function deliveryModulePathsByItem(catalog: FeatureCatalog): Map<string, string[]> {
    const itemIDs = new Set(catalog.links.filter(link => link.relation === 'delivery').map(link => link.itemId));
    return new Map([...itemIDs].map(itemID => [itemID, featureModulePathsForItem(catalog, itemID, 'delivery')]));
}

export function linkedFeatureEntries(
    catalog: FeatureCatalog,
    itemId: string,
    relation: FeatureItemRelation
): FeatureTreeNode[] {
    const entries = featureEntriesById(catalog.nodes);
    return catalog.links
        .filter(link => link.itemId === itemId && link.relation === relation)
        .map(link => entries.get(link.featureId))
        .filter((entry): entry is FeatureTreeNode => !!entry && entry.node.kind === 'feature')
        .sort((a, b) => a.path.join('/').localeCompare(b.path.join('/')));
}

export function featureModuleDepth(node: FeatureNode, byId: Map<string, FeatureNode>): number {
    let depth = node.kind === 'module' ? 1 : 0;
    let parentId = node.parentId ?? '';
    const seen = new Set<string>([node.id]);
    while (parentId && !seen.has(parentId)) {
        seen.add(parentId);
        const parent = byId.get(parentId);
        if (!parent) break;
        if (parent.kind === 'module') depth++;
        parentId = parent.parentId ?? '';
    }
    return depth;
}

export function featureDescendantIds(id: string, nodes: FeatureNode[]): Set<string> {
    const descendants = new Set<string>();
    let changed = true;
    while (changed) {
        changed = false;
        for (const node of nodes) {
            if (node.parentId === id || (node.parentId && descendants.has(node.parentId))) {
                if (!descendants.has(node.id)) {
                    descendants.add(node.id);
                    changed = true;
                }
            }
        }
    }
    return descendants;
}

export function featureModuleSubtreeHeight(id: string, nodes: FeatureNode[]): number {
    const children = nodes.filter(node => node.parentId === id && node.kind === 'module');
    if (children.length === 0) return 1;
    return 1 + Math.max(...children.map(child => featureModuleSubtreeHeight(child.id, nodes)));
}

export function validFeatureParents(
    kind: FeatureNodeKind,
    nodes: FeatureNode[],
    editingNode?: FeatureNode
): FeatureNode[] {
    const byId = new Map(nodes.map(node => [node.id, node]));
    const orderedNodes = flattenFeatureTree(buildFeatureTree(nodes)).map(entry => entry.node);
    const excluded = editingNode ? featureDescendantIds(editingNode.id, nodes) : new Set<string>();
    if (editingNode) excluded.add(editingNode.id);
    const subtreeHeight = editingNode?.kind === 'module' ? featureModuleSubtreeHeight(editingNode.id, nodes) : 1;

    return orderedNodes.filter(node => {
        if (node.kind !== 'module' || excluded.has(node.id)) return false;
        if (kind === 'feature') return true;
        return featureModuleDepth(node, byId) + subtreeHeight <= MAX_FEATURE_MODULE_DEPTH;
    });
}

export function siblingNodes(nodes: FeatureNode[], parentId?: string): FeatureNode[] {
    return nodes.filter(node => (node.parentId ?? '') === (parentId ?? '')).sort(compareNodes);
}

export function resolveFeatureDrop(
    nodes: FeatureNode[],
    draggedId: string,
    target: FeatureDropTarget
): FeatureDropMove | null {
    const byId = new Map(nodes.map(node => [node.id, node]));
    const dragged = byId.get(draggedId);
    if (!dragged) return null;

    let parentId = '';
    let position = 0;
    if (target.placement === 'root') {
        if (target.targetId || dragged.kind !== 'module') return null;
        const roots = siblingNodes(nodes, '').filter(node => node.id !== dragged.id);
        position = roots.length;
    } else {
        const targetNode = target.targetId ? byId.get(target.targetId) : undefined;
        if (!targetNode || targetNode.id === dragged.id) return null;
        if (target.placement === 'inside') {
            if (targetNode.kind !== 'module') return null;
            parentId = targetNode.id;
            position = siblingNodes(nodes, parentId).filter(node => node.id !== dragged.id).length;
        } else {
            parentId = targetNode.parentId ?? '';
            if (!parentId && dragged.kind === 'feature') return null;
            const siblings = siblingNodes(nodes, parentId).filter(node => node.id !== dragged.id);
            const targetIndex = siblings.findIndex(node => node.id === targetNode.id);
            if (targetIndex < 0) return null;
            position = targetIndex + (target.placement === 'after' ? 1 : 0);
        }
    }

    if (dragged.kind === 'feature' && !parentId) return null;
    if (dragged.kind === 'module') {
        const descendants = featureDescendantIds(dragged.id, nodes);
        if (parentId === dragged.id || descendants.has(parentId)) return null;
        const parentDepth = parentId ? featureModuleDepth(byId.get(parentId)!, byId) : 0;
        if (parentId && byId.get(parentId)?.kind !== 'module') return null;
        if (parentDepth + featureModuleSubtreeHeight(dragged.id, nodes) > MAX_FEATURE_MODULE_DEPTH) {
            return null;
        }
    } else if (byId.get(parentId)?.kind !== 'module') {
        return null;
    }

    const currentParent = dragged.parentId ?? '';
    const currentSiblings = siblingNodes(nodes, currentParent);
    const currentPosition = currentSiblings.findIndex(node => node.id === dragged.id);
    if (currentParent === parentId && currentPosition === position) return null;
    return { parentId: parentId || undefined, position };
}

export function formatFeatureError(error: unknown): string {
    const raw = error instanceof Error ? error.message : String(error);
    const message = raw.trim();
    if (message.includes('has_children')) return '该模块仍有子节点，请先移动或删除子节点。';
    if (message.includes('depth exceeds nine')) return '模块最多支持九级，请选择更高层级的父模块。';
    if (message.includes('tree cycle')) return '不能把模块移动到自身或其后代之下。';
    if (message.includes('invalid feature parent')) return '父节点无效；功能点只能挂在模块下。';
    if (message.includes('invalid feature node kind')) return '节点类型无效。';
    if (message.includes('invalid project item type')) return '关联类型不匹配：来源仅支持需求/缺陷，交付仅支持任务。';
    if (message.includes('invalid feature item relation')) return '关联类型无效。';
    if (message.includes('feature target must be a semantic version milestone')) {
        return '目标版本必须是当前项目的语义化版本，不能选择 legacy 里程碑。';
    }
    if (message.includes('project mismatch')) return '所选节点不属于当前项目。';
    return message || '操作失败，请稍后重试。';
}
