import { h } from 'preact';
import { useMemo, useState } from 'preact/hooks';

import type {
    CreateFeatureNodeInput,
    FeatureNode,
    FeatureNodeKind,
    UpdateFeatureNodeInput,
} from '@1agents/core/types/featureCatalog';
import type { Milestone } from './types';
import { buildFeatureTree, flattenFeatureTree, siblingNodes, validFeatureParents } from './featureCatalogModel';

interface FeatureNodeFormProps {
    kind: FeatureNodeKind;
    nodes: FeatureNode[];
    milestones: Milestone[];
    node?: FeatureNode;
    initialParentId?: string;
    busy: boolean;
    error: string;
    onCancel?: () => void;
    onCreate?: (input: CreateFeatureNodeInput) => Promise<void>;
    onUpdate?: (input: UpdateFeatureNodeInput) => Promise<void>;
}

export function FeatureNodeForm({
    kind,
    nodes,
    milestones,
    node,
    initialParentId = '',
    busy,
    error,
    onCancel,
    onCreate,
    onUpdate,
}: FeatureNodeFormProps) {
    const [title, setTitle] = useState(node?.title ?? '');
    const [description, setDescription] = useState(node?.description ?? '');
    const [parentId, setParentId] = useState(node?.parentId ?? initialParentId);
    const [targetMilestoneId, setTargetMilestoneId] = useState(node?.targetMilestoneId ?? '');
    const [validationError, setValidationError] = useState('');
    const parentCandidates = useMemo(() => validFeatureParents(kind, nodes, node), [kind, nodes, node]);
    const pathById = useMemo(
        () =>
            new Map(flattenFeatureTree(buildFeatureTree(nodes)).map(entry => [entry.node.id, entry.path.join(' / ')])),
        [nodes]
    );
    const isModule = kind === 'module';

    const submit = async (event: Event) => {
        event.preventDefault();
        const cleanTitle = title.trim();
        if (!cleanTitle) {
            setValidationError('请输入名称。');
            return;
        }
        if (!isModule && !parentId) {
            setValidationError('功能点必须选择所属模块。');
            return;
        }
        setValidationError('');

        if (node && onUpdate) {
            const input: UpdateFeatureNodeInput = {
                title: cleanTitle,
                description: description.trim(),
            };
            if ((node.parentId ?? '') !== parentId) {
                input.parentId = parentId;
                input.position = siblingNodes(nodes, parentId).filter(sibling => sibling.id !== node.id).length;
            }
            if (!isModule && (node.targetMilestoneId ?? '') !== targetMilestoneId) {
                input.targetMilestoneId = targetMilestoneId;
            }
            await onUpdate(input);
            return;
        }
        if (onCreate) {
            await onCreate({
                kind,
                parentId: parentId || undefined,
                title: cleanTitle,
                description: description.trim() || undefined,
                targetMilestoneId: !isModule && targetMilestoneId ? targetMilestoneId : undefined,
                position: siblingNodes(nodes, parentId).length,
            });
        }
    };

    return (
        <form class="feature-node-form" onSubmit={submit}>
            <label>
                <span>名称</span>
                <input
                    value={title}
                    onInput={(event: Event) => setTitle((event.currentTarget as HTMLInputElement).value)}
                    placeholder={isModule ? '例如：用户与权限' : '例如：密码登录'}
                    autoFocus
                    disabled={busy}
                />
            </label>
            <label>
                <span>说明</span>
                <textarea
                    value={description}
                    onInput={(event: Event) => setDescription((event.currentTarget as HTMLTextAreaElement).value)}
                    placeholder="补充该节点的范围与边界（可选）"
                    rows={4}
                    disabled={busy}
                />
            </label>
            <label>
                <span>所属模块</span>
                <select
                    value={parentId}
                    onChange={(event: Event) => setParentId((event.currentTarget as HTMLSelectElement).value)}
                    disabled={busy}
                >
                    {isModule && <option value="">作为一级模块</option>}
                    {parentCandidates.map(parent => (
                        <option key={parent.id} value={parent.id}>
                            {pathById.get(parent.id) ?? parent.title}
                        </option>
                    ))}
                </select>
            </label>
            {!isModule && (
                <label>
                    <span>目标版本</span>
                    <select
                        value={targetMilestoneId}
                        onChange={(event: Event) =>
                            setTargetMilestoneId((event.currentTarget as HTMLSelectElement).value)
                        }
                        disabled={busy}
                    >
                        <option value="">未指定</option>
                        {milestones
                            .filter(milestone => !!milestone.version && !milestone.isLegacy)
                            .map(milestone => (
                                <option key={milestone.id} value={milestone.id}>
                                    {milestone.version}
                                </option>
                            ))}
                    </select>
                    <small>仅可选择项目内的语义化版本；修改不会自动改写已有任务。</small>
                </label>
            )}
            {(validationError || error) && <div class="feature-form-error">{validationError || error}</div>}
            <div class="feature-form-actions">
                {onCancel && (
                    <button type="button" class="feature-btn secondary" onClick={onCancel} disabled={busy}>
                        取消
                    </button>
                )}
                <button type="submit" class="feature-btn primary" disabled={busy}>
                    {busy ? '保存中…' : node ? '保存修改' : `新增${isModule ? '模块' : '功能点'}`}
                </button>
            </div>
        </form>
    );
}
