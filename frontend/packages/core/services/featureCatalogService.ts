import { apiFetch } from './apiClient';
import type {
    CreateFeatureNodeInput,
    FeatureCatalog,
    FeatureCatalogRestoreResult,
    FeatureCatalogVersion,
    FeatureCatalogVersionPage,
    FeatureItemLink,
    FeatureItemRelation,
    FeatureMilestoneSyncPreview,
    FeatureNode,
    UpdateFeatureNodeInput,
    GanttData,
} from '../types/featureCatalog';

const q = encodeURIComponent;

async function ok(res: Response): Promise<Response> {
    if (!res.ok) throw new Error(await res.text());
    return res;
}

export const featureCatalogService = {
    async get(workspaceId: string): Promise<FeatureCatalog> {
        const res = await ok(await apiFetch(`/agent/feature-catalog?workspace_id=${q(workspaceId)}`));
        return res.json();
    },

    async create(workspaceId: string, input: CreateFeatureNodeInput): Promise<FeatureNode> {
        const res = await ok(
            await apiFetch('/agent/feature-catalog', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...input }),
            })
        );
        return res.json();
    },

    async update(workspaceId: string, id: string, input: UpdateFeatureNodeInput): Promise<FeatureNode> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/${q(id)}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, ...input }),
            })
        );
        return res.json();
    },

    async remove(workspaceId: string, id: string): Promise<void> {
        await ok(
            await apiFetch(`/agent/feature-catalog/${q(id)}?workspace_id=${q(workspaceId)}`, {
                method: 'DELETE',
            })
        );
    },

    async linkItem(
        workspaceId: string,
        featureId: string,
        itemId: string,
        relation: FeatureItemRelation
    ): Promise<FeatureItemLink> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/${q(featureId)}/items`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, itemId, relation }),
            })
        );
        return res.json();
    },

    async unlinkItem(
        workspaceId: string,
        featureId: string,
        itemId: string,
        relation: FeatureItemRelation
    ): Promise<void> {
        await ok(
            await apiFetch(
                `/agent/feature-catalog/${q(featureId)}/items/${q(itemId)}?workspace_id=${q(
                    workspaceId
                )}&relation=${q(relation)}`,
                { method: 'DELETE' }
            )
        );
    },

    async moveItem(
        workspaceId: string,
        fromFeatureId: string,
        toFeatureId: string,
        itemId: string,
        relation: FeatureItemRelation
    ): Promise<void> {
        await ok(
            await apiFetch('/agent/feature-catalog/batch', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    workspace_id: workspaceId,
                    operations: [
                        { op: 'link', featureId: toFeatureId, itemId, relation },
                        { op: 'unlink', featureId: fromFeatureId, itemId, relation },
                    ],
                }),
            })
        );
    },

    async milestoneDiff(workspaceId: string, featureId: string): Promise<FeatureMilestoneSyncPreview> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/${q(featureId)}/milestone-diff?workspace_id=${q(workspaceId)}`)
        );
        return res.json();
    },

    async syncMilestone(workspaceId: string, featureId: string): Promise<FeatureMilestoneSyncPreview> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/${q(featureId)}/sync-milestone`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId }),
            })
        );
        return res.json();
    },

    async gantt(workspaceId: string): Promise<GanttData> {
        const res = await ok(await apiFetch(`/agent/feature-catalog/gantt?workspace_id=${q(workspaceId)}`));
        return res.json();
    },

    async exportCatalog(workspaceId: string, format: 'markdown' | 'json'): Promise<Blob> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/export?workspace_id=${q(workspaceId)}&format=${q(format)}`)
        );
        return res.blob();
    },

    async listVersions(workspaceId: string, cursor?: string): Promise<FeatureCatalogVersionPage> {
        const suffix = cursor ? `&cursor=${q(cursor)}` : '';
        const res = await ok(await apiFetch(`/agent/feature-catalog/versions?workspace_id=${q(workspaceId)}${suffix}`));
        return res.json();
    },

    async createVersion(workspaceId: string, alias = ''): Promise<FeatureCatalogVersion> {
        const res = await ok(
            await apiFetch('/agent/feature-catalog/versions', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, alias }),
            })
        );
        return res.json();
    },

    async renameVersion(workspaceId: string, versionId: string, alias: string): Promise<FeatureCatalogVersion> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/versions/${q(versionId)}`, {
                method: 'PATCH',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, alias }),
            })
        );
        return res.json();
    },

    async deleteVersion(workspaceId: string, versionId: string): Promise<void> {
        await ok(
            await apiFetch(`/agent/feature-catalog/versions/${q(versionId)}?workspace_id=${q(workspaceId)}`, {
                method: 'DELETE',
            })
        );
    },

    async restoreVersion(
        workspaceId: string,
        versionId: string,
        requestId: string
    ): Promise<FeatureCatalogRestoreResult> {
        const res = await ok(
            await apiFetch(`/agent/feature-catalog/versions/${q(versionId)}/restore`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ workspace_id: workspaceId, requestId }),
            })
        );
        return res.json();
    },
};
