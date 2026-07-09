/**
 * speech_clip (口播剪辑) API client — thin fetch wrappers over the backend
 * /api/speech_clip/* routes. Domain data is filesystem jsonl on the backend;
 * these calls read/trigger it via the task kernel.
 */

const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req<T>(url: string, opts?: RequestInit): Promise<T> {
    const res = await fetch(url, { credentials: 'same-origin', ...opts });
    if (!res.ok) throw new Error((await res.text()) || `${res.status} ${url}`);
    return (await res.json()) as T;
}

const q = (params: Record<string, string>) =>
    Object.entries(params)
        .map(([k, v]) => `${k}=${encodeURIComponent(v)}`)
        .join('&');

export interface SCAsset {
    id: string;
    file: string;
    label: string;
    transcribed: boolean;
    transcriptSentences: number;
    highlighted: boolean;
    highlights: number;
}

export interface SCProject {
    stage: string;
    mainline: string[];
    assets: SCAsset[];
}

export interface SCSentence {
    i: number;
    asset: string;
    text: string;
    start: number;
    end: number;
    spk: number;
}

export interface SCHighlight {
    i: number;
    asset: string;
    picked: boolean;
    score: number;
    reason: string;
    corrected_text: string;
}

export const scApi = {
    project: (ws: string): Promise<SCProject> => req(`/api/speech_clip/project?${q({ workspacePath: ws })}`),

    transcript: (ws: string, asset: string): Promise<{ sentences: SCSentence[] }> =>
        req(`/api/speech_clip/transcript?${q({ workspacePath: ws, assetId: asset })}`),

    highlights: (ws: string, asset: string): Promise<{ highlights: SCHighlight[] }> =>
        req(`/api/speech_clip/highlights?${q({ workspacePath: ws, assetId: asset })}`),

    importAsset: (ws: string, sourcePath: string, label: string): Promise<{ assetId: string }> =>
        req('/api/speech_clip/assets', {
            method: 'POST',
            headers: JSON_HEADERS,
            body: JSON.stringify({ workspacePath: ws, sourcePath, label }),
        }),

    upload: (
        ws: string,
        label: string,
        ext: string,
        blobs: { videoBase64?: string; audioBase64?: string; dataBase64?: string }
    ): Promise<{ assetId: string }> =>
        req('/api/speech_clip/assets/upload', {
            method: 'POST',
            headers: JSON_HEADERS,
            body: JSON.stringify({ workspacePath: ws, label, ext, ...blobs }),
        }),

    transcribe: (ws: string, asset: string): Promise<{ taskId: string }> =>
        req('/api/speech_clip/transcribe', {
            method: 'POST',
            headers: JSON_HEADERS,
            body: JSON.stringify({ workspacePath: ws, assetId: asset }),
        }),

    extractHighlights: (ws: string, asset: string): Promise<{ taskId: string }> =>
        req('/api/speech_clip/highlights', {
            method: 'POST',
            headers: JSON_HEADERS,
            body: JSON.stringify({ workspacePath: ws, assetId: asset }),
        }),

    pick: (ws: string, asset: string, i: number, picked: boolean): Promise<{ ok: boolean }> =>
        req('/api/speech_clip/pick', {
            method: 'POST',
            headers: JSON_HEADERS,
            body: JSON.stringify({ workspacePath: ws, assetId: asset, i, picked }),
        }),
};
