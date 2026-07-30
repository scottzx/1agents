import { h } from 'preact';
import { useCallback, useEffect, useState } from 'preact/hooks';

import { featureCatalogService } from '@1agents/core/services/featureCatalogService';
import type { FeatureCatalogRestoreResult, FeatureCatalogVersion } from '@1agents/core/types/featureCatalog';

interface FeatureCatalogHistoryDialogProps {
    workspaceId: string;
    open: boolean;
    onClose: () => void;
    onRestored: (result: FeatureCatalogRestoreResult) => Promise<void>;
}

interface Confirmation {
    kind: 'restore' | 'delete';
    version: FeatureCatalogVersion;
    requestId?: string;
}

function versionTitle(version: FeatureCatalogVersion): string {
    return version.alias.trim() || new Date(version.createdAt).toLocaleString();
}

function newRequestID(): string {
    if (typeof crypto !== 'undefined' && crypto.randomUUID) return crypto.randomUUID();
    return `restore-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

export function FeatureCatalogHistoryDialog({
    workspaceId,
    open,
    onClose,
    onRestored,
}: FeatureCatalogHistoryDialogProps) {
    const [versions, setVersions] = useState<FeatureCatalogVersion[]>([]);
    const [nextCursor, setNextCursor] = useState('');
    const [hasMore, setHasMore] = useState(false);
    const [loading, setLoading] = useState(false);
    const [pending, setPending] = useState('');
    const [error, setError] = useState('');
    const [alias, setAlias] = useState('');
    const [editingID, setEditingID] = useState('');
    const [editingAlias, setEditingAlias] = useState('');
    const [confirmation, setConfirmation] = useState<Confirmation | null>(null);
    const [restoreResult, setRestoreResult] = useState<FeatureCatalogRestoreResult | null>(null);
    const [refreshFailed, setRefreshFailed] = useState(false);

    const loadVersions = useCallback(
        async (append = false, cursor = '') => {
            setLoading(true);
            setError('');
            try {
                const page = await featureCatalogService.listVersions(workspaceId, append ? cursor : undefined);
                setVersions(current => (append ? [...current, ...page.items] : page.items));
                setNextCursor(page.nextCursor ?? '');
                setHasMore(page.hasMore);
            } catch (cause) {
                setError(cause instanceof Error ? cause.message : String(cause));
            } finally {
                setLoading(false);
            }
        },
        [workspaceId]
    );

    useEffect(() => {
        if (!open) return;
        setVersions([]);
        setNextCursor('');
        setHasMore(false);
        setAlias('');
        setEditingID('');
        setConfirmation(null);
        setRestoreResult(null);
        setRefreshFailed(false);
        void loadVersions(false);
    }, [open, workspaceId]);

    if (!open) return null;

    const saveVersion = async () => {
        setPending('create');
        setError('');
        try {
            const created = await featureCatalogService.createVersion(workspaceId, alias);
            setVersions(current => [created, ...current]);
            setAlias('');
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            setPending('');
        }
    };

    const renameVersion = async (version: FeatureCatalogVersion) => {
        setPending(`rename:${version.id}`);
        setError('');
        try {
            const renamed = await featureCatalogService.renameVersion(workspaceId, version.id, editingAlias);
            setVersions(current => current.map(item => (item.id === renamed.id ? renamed : item)));
            setEditingID('');
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            setPending('');
        }
    };

    const confirmAction = async () => {
        if (!confirmation) return;
        const version = confirmation.version;
        setPending(`${confirmation.kind}:${version.id}`);
        setError('');
        if (confirmation.kind === 'delete') {
            try {
                await featureCatalogService.deleteVersion(workspaceId, version.id);
                setVersions(current => current.filter(item => item.id !== version.id));
                setConfirmation(null);
            } catch (cause) {
                setError(cause instanceof Error ? cause.message : String(cause));
            } finally {
                setPending('');
            }
            return;
        }
        try {
            const result = await featureCatalogService.restoreVersion(workspaceId, version.id, confirmation.requestId!);
            setRestoreResult(result);
            setConfirmation(null);
            setRefreshFailed(false);
            try {
                await onRestored(result);
            } catch {
                setRefreshFailed(true);
            }
            await loadVersions(false);
        } catch (cause) {
            setError(cause instanceof Error ? cause.message : String(cause));
        } finally {
            setPending('');
        }
    };

    const beginRestore = (version: FeatureCatalogVersion) => {
        setRestoreResult(null);
        setConfirmation({ kind: 'restore', version, requestId: newRequestID() });
    };

    const retryRefresh = async () => {
        if (!restoreResult) return;
        setPending('refresh');
        try {
            await onRestored(restoreResult);
            setRefreshFailed(false);
        } catch {
            setRefreshFailed(true);
        } finally {
            setPending('');
        }
    };

    return (
        <div class="ws-modal-overlay feature-history-overlay" onClick={() => !pending && onClose()}>
            <div class="ws-modal feature-history-modal" onClick={(event: MouseEvent) => event.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>功能蓝图历史版本</span>
                    <button class="ws-modal-close" onClick={onClose} disabled={!!pending}>
                        ✕
                    </button>
                </div>
                <div class="ws-modal-body feature-history-body">
                    <section class="feature-history-save">
                        <div>
                            <strong>保存当前版本</strong>
                            <small>保存节点、顺序、描述、目标版本和蓝图关联。</small>
                        </div>
                        <div>
                            <input
                                value={alias}
                                placeholder="别名（可选）"
                                aria-label="历史版本别名"
                                disabled={!!pending}
                                onInput={(event: Event) => setAlias((event.currentTarget as HTMLInputElement).value)}
                            />
                            <button
                                type="button"
                                class="feature-btn primary"
                                disabled={!!pending}
                                onClick={() => void saveVersion()}
                            >
                                {pending === 'create' ? '保存中…' : '保存当前版本'}
                            </button>
                        </div>
                    </section>
                    {error && (
                        <div class="feature-form-error" role="alert">
                            {error}
                        </div>
                    )}
                    {refreshFailed && (
                        <div class="feature-history-refresh-warning" role="alert">
                            <span>恢复已完成，但界面刷新失败。请只重试刷新，不要再次提交恢复。</span>
                            <button type="button" disabled={!!pending} onClick={() => void retryRefresh()}>
                                重试刷新
                            </button>
                        </div>
                    )}
                    {restoreResult && (
                        <section class="feature-history-result" role="status">
                            <strong>
                                已恢复 {restoreResult.restoredNodeCount} 个节点、{restoreResult.restoredLinkCount}{' '}
                                条关联
                            </strong>
                            <span>
                                已自动保存恢复前版本；跳过 {restoreResult.skippedLinkCount} 条失效关联，清空{' '}
                                {restoreResult.clearedTargetMilestoneCount} 个失效目标版本。
                            </span>
                            {restoreResult.warnings.length > 0 && (
                                <ul>
                                    {restoreResult.warnings.map((warning, index) => (
                                        <li
                                            key={`${warning.kind}:${warning.featureId}:${warning.referenceId}:${index}`}
                                        >
                                            {warning.kind === 'target_milestone'
                                                ? '已清空目标版本'
                                                : warning.kind === 'source_link'
                                                  ? '已跳过来源关联'
                                                  : '已跳过交付关联'}
                                            ：{warning.featureId} → {warning.referenceId}
                                        </li>
                                    ))}
                                </ul>
                            )}
                            {restoreResult.warningsTruncated && <small>仍有更多警告未展示。</small>}
                        </section>
                    )}
                    <section class="feature-history-list" aria-label="历史版本列表">
                        {versions.map(version => (
                            <article key={version.id}>
                                <div class="feature-history-version-copy">
                                    {editingID === version.id ? (
                                        <div class="feature-history-rename">
                                            <input
                                                value={editingAlias}
                                                aria-label={`重命名${versionTitle(version)}`}
                                                disabled={!!pending}
                                                onInput={(event: Event) =>
                                                    setEditingAlias((event.currentTarget as HTMLInputElement).value)
                                                }
                                            />
                                            <button
                                                type="button"
                                                disabled={!!pending}
                                                onClick={() => void renameVersion(version)}
                                            >
                                                保存
                                            </button>
                                            <button type="button" disabled={!!pending} onClick={() => setEditingID('')}>
                                                取消
                                            </button>
                                        </div>
                                    ) : (
                                        <strong>{versionTitle(version)}</strong>
                                    )}
                                    <span>
                                        <em>{version.kind === 'manual' ? '手动存档' : '恢复前'}</em>
                                        {version.nodeCount} 个节点 · {version.linkCount} 条关联 ·{' '}
                                        {new Date(version.createdAt).toLocaleString()}
                                    </span>
                                </div>
                                <div class="feature-history-actions">
                                    <button
                                        type="button"
                                        disabled={!!pending}
                                        onClick={() => {
                                            setEditingID(version.id);
                                            setEditingAlias(version.alias);
                                        }}
                                    >
                                        重命名
                                    </button>
                                    <button type="button" disabled={!!pending} onClick={() => beginRestore(version)}>
                                        恢复
                                    </button>
                                    <button
                                        type="button"
                                        class="danger"
                                        disabled={!!pending}
                                        onClick={() => setConfirmation({ kind: 'delete', version })}
                                    >
                                        删除
                                    </button>
                                </div>
                            </article>
                        ))}
                        {!loading && versions.length === 0 && <div class="feature-tree-empty">尚无历史版本。</div>}
                        {loading && <div class="feature-tree-empty">正在加载历史版本…</div>}
                        {hasMore && !loading && (
                            <button
                                type="button"
                                class="feature-btn secondary feature-history-more"
                                disabled={!!pending}
                                onClick={() => void loadVersions(true, nextCursor)}
                            >
                                加载更多
                            </button>
                        )}
                    </section>
                </div>
                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose} disabled={!!pending}>
                        关闭
                    </button>
                </div>
                {confirmation && (
                    <div class="feature-history-confirm">
                        <div>
                            <strong>
                                {confirmation.kind === 'restore'
                                    ? `恢复“${versionTitle(confirmation.version)}”？`
                                    : `删除“${versionTitle(confirmation.version)}”？`}
                            </strong>
                            {confirmation.kind === 'restore' ? (
                                <p>
                                    当前蓝图的节点、顺序、描述、目标版本和蓝图关联会被替换。项目中的需求、任务、缺陷和里程碑本身不会被删除或回滚；系统会先自动保存一份“恢复前”版本。
                                </p>
                            ) : (
                                <p>删除后无法找回该存档；不会删除当前功能蓝图或项目事项。</p>
                            )}
                            <div>
                                <button
                                    type="button"
                                    class="feature-btn secondary"
                                    disabled={!!pending}
                                    onClick={() => setConfirmation(null)}
                                >
                                    取消
                                </button>
                                <button
                                    type="button"
                                    class={`feature-btn ${confirmation.kind === 'delete' ? 'danger' : 'primary'}`}
                                    disabled={!!pending}
                                    onClick={() => void confirmAction()}
                                >
                                    {pending ? '处理中…' : confirmation.kind === 'restore' ? '确认恢复' : '确认删除'}
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
