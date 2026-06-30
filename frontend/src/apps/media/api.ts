/**
 * Media app API client. Thin wrappers over the backend handlers the orchestrator
 * wires under /api/apps/media/. All calls go through the shared apiFetch so they
 * inherit auth + base-url handling.
 */
import { apiFetch } from '@1agents/core/services/apiClient';

export interface ContentProject {
    id: string;
    projectId: string;
    workspace: string;
    title: string;
    status: string;
    createdAt: string;
    updatedAt: string;
}

export interface Material {
    id: string;
    projectId: string;
    kind: string;
    filePath: string;
    duration: number;
    stage: string;
    metadata: Record<string, string>;
    createdAt: string;
}

export interface Segment {
    id: string;
    materialId: string;
    start: number;
    end: number;
    label: string;
    decision: 'undecided' | 'keep' | 'drop';
    ordinal: number;
    createdAt: string;
}

export interface MediaTask {
    id: string;
    title: string;
    status: string;
    executor: string;
    milestone: string;
    businessRef: string;
}

async function jsonOrThrow<T>(res: Response): Promise<T> {
    if (!res.ok) {
        const text = await res.text().catch(() => res.statusText);
        throw new Error(text || `HTTP ${res.status}`);
    }
    return res.json() as Promise<T>;
}

export async function listProjects(workspace: string): Promise<ContentProject[]> {
    const res = await apiFetch(`/api/apps/media/projects?workspace=${encodeURIComponent(workspace)}`);
    const data = await jsonOrThrow<{ projects: ContentProject[] }>(res);
    return data.projects ?? [];
}

export async function createProject(workspace: string, title: string): Promise<ContentProject> {
    const res = await apiFetch('/api/apps/media/projects', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspace, title }),
    });
    return jsonOrThrow<ContentProject>(res);
}

/** Returns the existing content project for a workspace, creating one if absent. */
export async function ensureProject(workspace: string, title: string): Promise<ContentProject> {
    const existing = await listProjects(workspace);
    if (existing.length > 0) return existing[0];
    return createProject(workspace, title);
}

export async function listMaterials(projectId: string): Promise<Material[]> {
    const res = await apiFetch(`/api/apps/media/materials?projectId=${encodeURIComponent(projectId)}`);
    const data = await jsonOrThrow<{ materials: Material[] }>(res);
    return data.materials ?? [];
}

export async function uploadMaterial(
    projectId: string,
    file: File,
    duration: number,
    kind?: string
): Promise<Material> {
    const form = new FormData();
    form.append('projectId', projectId);
    form.append('file', file);
    form.append('duration', String(duration || 0));
    if (kind) form.append('kind', kind);
    const res = await apiFetch('/api/apps/media/materials/upload', { method: 'POST', body: form });
    return jsonOrThrow<Material>(res);
}

export async function launchPipeline(projectId: string, materialId: string): Promise<string[]> {
    const res = await apiFetch('/api/apps/media/pipeline', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ projectId, materialId }),
    });
    const data = await jsonOrThrow<{ taskIds: string[] }>(res);
    return data.taskIds ?? [];
}

export async function listSegments(materialId: string): Promise<Segment[]> {
    const res = await apiFetch(`/api/apps/media/segments?materialId=${encodeURIComponent(materialId)}`);
    const data = await jsonOrThrow<{ segments: Segment[] }>(res);
    return data.segments ?? [];
}

export async function setSegmentDecision(segmentId: string, decision: string): Promise<void> {
    const res = await apiFetch('/api/apps/media/segments', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ segmentId, decision }),
    });
    await jsonOrThrow<{ ok: boolean }>(res);
}

export async function listBusinessTasks(ref: string): Promise<MediaTask[]> {
    const res = await apiFetch(`/api/apps/media/tasks?ref=${encodeURIComponent(ref)}`);
    const data = await jsonOrThrow<{ tasks: MediaTask[] }>(res);
    return data.tasks ?? [];
}

export async function resolveHumanTask(
    taskId: string,
    verdict: string,
    payload?: Record<string, unknown>
): Promise<void> {
    const res = await apiFetch('/api/apps/media/human/resolve', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ taskId, verdict, payload: payload ?? {} }),
    });
    await jsonOrThrow<{ ok: boolean }>(res);
}

/** business_ref helper matching the backend format "media:<entity>:<id>". */
export function businessRef(entity: string, id: string): string {
    return `media:${entity}:${id}`;
}
