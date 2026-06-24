// 智能录音 —— 前端 IPC 客户端。仅在 Tauri 桌面壳内可用,调 Rust 命令
// (见 src-tauri/src/recording.rs)。门阀:isTauri() && macOS。

import { isTauri } from '../ota/desktopCheck';

interface TauriCore {
    invoke: <T = unknown>(cmd: string, args?: Record<string, unknown>) => Promise<T>;
}
const core = (): TauriCore | undefined => (window as unknown as { __TAURI__?: { core: TauriCore } }).__TAURI__?.core;

/** 一期门阀:桌面壳 + macOS 才开放本地推理录音(Windows 一期不开)。 */
export function canUseSmartRecording(): boolean {
    return isTauri() && navigator.userAgent.toLowerCase().includes('mac');
}

// 与 Rust serde(camelCase)一一对应
export interface Utterance {
    speaker: string; // "speaker_0"
    start: number; // 秒
    end: number;
    text: string;
}

export interface Recording {
    id: string;
    createdAt: number; // unix 秒
    duration: number;
    speakerCount: number;
    title: string;
    fullText: string;
    summary: string | null;
    utterances: Utterance[]; // 列表接口为空,详情接口才有
}

/** 录音结束:16k 单声道 PCM(i16 小端)base64 → 转写+分离+落库,返回 Recording。 */
export function transcribeAndSave(pcmBase64: string, sampleRate = 16000): Promise<Recording> {
    const c = core();
    if (!c) return Promise.reject(new Error('not in desktop shell'));
    return c.invoke<Recording>('transcribe_and_save', { pcmBase64, sampleRate });
}

export function listRecordings(): Promise<Recording[]> {
    const c = core();
    if (!c) return Promise.resolve([]);
    return c.invoke<Recording[]>('list_recordings');
}

export function getRecording(id: string): Promise<Recording> {
    const c = core();
    if (!c) return Promise.reject(new Error('not in desktop shell'));
    return c.invoke<Recording>('get_recording', { id });
}

/** 1acp 总结生成后回填。 */
export function updateRecordingSummary(id: string, summary: string): Promise<void> {
    const c = core();
    if (!c) return Promise.resolve();
    return c.invoke<void>('update_recording_summary', { id, summary });
}

export function deleteRecording(id: string): Promise<void> {
    const c = core();
    if (!c) return Promise.resolve();
    return c.invoke<void>('delete_recording', { id });
}
