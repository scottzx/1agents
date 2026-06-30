/**
 * Radio service — typed client for the radio app's HTTP surface, served by the
 * backend `radio.NewHandler()` at `/api/radio/*`. All data access lives here so
 * a Taro/RN client could reuse it by swapping `apiFetch`.
 *
 * Endpoints (see backend/internal/apps/radio/handler.go):
 *   GET  /api/radio/episodes
 *   POST /api/radio/episodes              {title, sourceUrl, workspace}
 *   GET  /api/radio/episodes/{id}         → {episode, tasks}
 *   POST /api/radio/episodes/{id}/pipeline
 *   GET  /api/radio/audio/{id}            (range-streamed audio)
 */

import { apiFetch } from '@1agents/core/services/apiClient';

export type EpisodeStatus = 'draft' | 'summarizing' | 'transcribing' | 'synthesizing' | 'ready';

export interface RadioEpisode {
    id: number;
    title: string;
    sourceUrl: string;
    status: EpisodeStatus;
    summary: string;
    transcript: string;
    audioPath: string;
    duration: number;
    createdAt: string;
    updatedAt: string;
}

/** A pipeline task as returned by the reverse binding seam (ListTasksForBusiness). */
export interface PipelineTask {
    id: string;
    title: string;
    status: string;
    executor: string;
    milestone?: string;
}

export interface EpisodeDetail {
    episode: RadioEpisode;
    tasks: PipelineTask[];
}

export async function listEpisodes(): Promise<RadioEpisode[]> {
    try {
        const res = await apiFetch('/api/radio/episodes');
        if (!res.ok) return [];
        const data = await res.json();
        return Array.isArray(data?.episodes) ? data.episodes : [];
    } catch {
        return [];
    }
}

export async function getEpisode(id: number): Promise<EpisodeDetail | null> {
    try {
        const res = await apiFetch(`/api/radio/episodes/${id}`);
        if (!res.ok) return null;
        return (await res.json()) as EpisodeDetail;
    } catch {
        return null;
    }
}

export async function createEpisode(title: string, sourceUrl: string, workspace: string): Promise<RadioEpisode | null> {
    try {
        const res = await apiFetch('/api/radio/episodes', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ title, sourceUrl, workspace }),
        });
        if (!res.ok) return null;
        return (await res.json()) as RadioEpisode;
    } catch {
        return null;
    }
}

export async function startPipeline(id: number): Promise<string[]> {
    try {
        const res = await apiFetch(`/api/radio/episodes/${id}/pipeline`, { method: 'POST' });
        if (!res.ok) return [];
        const data = await res.json();
        return Array.isArray(data?.taskIds) ? data.taskIds : [];
    } catch {
        return [];
    }
}

/** The streaming URL for an episode's audio, pointed at by <audio src>. */
export function audioUrl(id: number): string {
    return `/api/radio/audio/${id}`;
}
