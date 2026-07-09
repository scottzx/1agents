import { h } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';

import * as wsStore from '../../stores/workspaceStore';
import type { AppViewProps } from '../../modules/appViewRegistry';
import { StudioRecorder } from '../../utils/studioRecorder';
import { scApi } from './api';
import type { SCProject, SCSentence, SCHighlight } from './api';

/** A transcript sentence merged with its 1acp highlight grading (by index). */
interface Row extends SCSentence {
    picked: boolean;
    score: number;
    reason: string;
    corrected: string;
}

function fmtTime(ms: number): string {
    if (!ms && ms !== 0) return '';
    const s = Math.floor(ms / 1000);
    return `${String(Math.floor(s / 60)).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
}

const errMsg = (e: unknown): string => (e instanceof Error ? e.message : String(e));

function blobToBase64(blob: Blob): Promise<string> {
    return new Promise((resolve, reject) => {
        const reader = new FileReader();
        reader.onloadend = () => resolve((reader.result as string).split(',')[1] ?? '');
        reader.onerror = reject;
        reader.readAsDataURL(blob);
    });
}

/**
 * SpeechClipTab — 口播剪辑 project-tab. Stepper workspace over the file-based
 * pipeline: 素材 → 转录(FunClip) → 金句/纠错(1acp). Both heavy steps run as tasks;
 * this view triggers them and polls the resulting jsonl.
 */
export function SpeechClipTab({ workspaceId }: AppViewProps) {
    const workspaces = wsStore.workspaces.value;
    const ws = useMemo(() => workspaces.find(w => w.id === workspaceId)?.path ?? '', [workspaces, workspaceId]);

    const [project, setProject] = useState<SCProject | null>(null);
    const [sel, setSel] = useState<string>('');
    const [rows, setRows] = useState<Row[]>([]);
    const [importPath, setImportPath] = useState('');
    const [importLabel, setImportLabel] = useState('');
    const [err, setErr] = useState('');
    const [recorder] = useState(() => new StudioRecorder());
    const [isRecording, setIsRecording] = useState(false);
    const [busy, setBusy] = useState('');

    const loadProject = async () => {
        if (!ws) return;
        try {
            const p = await scApi.project(ws);
            setProject(p);
            if (!sel && p.assets.length) setSel(p.assets[0].id);
        } catch (e) {
            setErr(errMsg(e));
        }
    };

    const loadRows = async (asset: string) => {
        if (!ws || !asset) return;
        const [t, hl] = await Promise.all([scApi.transcript(ws, asset), scApi.highlights(ws, asset)]);
        const byI = new Map<number, SCHighlight>();
        (hl.highlights || []).forEach(h => byI.set(h.i, h));
        setRows(
            (t.sentences || []).map(s => {
                const h = byI.get(s.i);
                return {
                    ...s,
                    picked: h?.picked ?? false,
                    score: h?.score ?? 0,
                    reason: h?.reason ?? '',
                    corrected: h?.corrected_text ?? '',
                };
            })
        );
    };

    // Steady poll while the tab is open — transcription/highlighting run async.
    useEffect(() => {
        loadProject();
        const id = setInterval(loadProject, 4000);
        return () => clearInterval(id);
    }, [ws]);

    useEffect(() => {
        loadRows(sel);
        const id = setInterval(() => loadRows(sel), 4000);
        return () => clearInterval(id);
    }, [ws, sel]);

    const doImport = async () => {
        if (!ws || !importPath) return;
        setErr('');
        try {
            await scApi.importAsset(ws, importPath.trim(), importLabel.trim());
            setImportPath('');
            setImportLabel('');
            await loadProject();
        } catch (e) {
            setErr(errMsg(e));
        }
    };

    // In-tab recording: reuse the studio recorder (screen + camera + mic), then
    // upload the audio track as a transcribable asset.
    const startRec = async () => {
        setErr('');
        try {
            await recorder.start();
            setIsRecording(true);
        } catch (e) {
            setErr(errMsg(e));
        }
    };

    const stopRec = async () => {
        setBusy('保存录音…');
        try {
            const assets = await recorder.stop();
            setIsRecording(false);
            // Upload screen video + audio; the backend muxes them into one webm
            // (video kept for 混剪, audio used for transcription).
            const [videoB64, audioB64] = await Promise.all([
                blobToBase64(assets.screenBlob),
                blobToBase64(assets.audioBlob),
            ]);
            const label = importLabel.trim() || `录制 ${new Date().toLocaleString()}`;
            await scApi.upload(ws, label, '.webm', { videoBase64: videoB64, audioBase64: audioB64 });
            setImportLabel('');
            await loadProject();
        } catch (e) {
            setErr(errMsg(e));
            setIsRecording(false);
        } finally {
            setBusy('');
        }
    };

    const doTranscribe = async (asset: string) => {
        setErr('');
        try {
            await scApi.transcribe(ws, asset);
            await loadProject();
        } catch (e) {
            setErr(errMsg(e));
        }
    };

    const doExtract = async (asset: string) => {
        setErr('');
        try {
            await scApi.extractHighlights(ws, asset);
            await loadProject();
        } catch (e) {
            setErr(errMsg(e));
        }
    };

    const togglePick = async (row: Row) => {
        const next = !row.picked;
        setRows(rs => rs.map(r => (r.i === row.i ? { ...r, picked: next } : r)));
        try {
            await scApi.pick(ws, sel, row.i, next);
        } catch (e) {
            setErr(errMsg(e));
            setRows(rs => rs.map(r => (r.i === row.i ? { ...r, picked: !next } : r)));
        }
    };

    if (!ws) {
        return <div class="speech-clip-empty">未找到当前项目路径。</div>;
    }

    const assets = project?.assets ?? [];
    const step = assets.length === 0 ? 1 : assets.some(a => !a.transcribed) ? 2 : 3;
    const selAsset = assets.find(a => a.id === sel);
    const pickedCount = rows.filter(r => r.picked).length;

    return (
        <div class="speech-clip">
            <div class="speech-clip-steps">
                {['① 素材', '② 转录', '③ 金句 / 混剪'].map((label, i) => (
                    <div
                        class={`speech-clip-step ${i + 1 === step ? 'is-active' : ''} ${i + 1 < step ? 'is-done' : ''}`}
                    >
                        {label}
                    </div>
                ))}
            </div>

            {err && <div class="speech-clip-error">{err}</div>}

            <div class="speech-clip-import">
                <input
                    class="speech-clip-input"
                    placeholder="素材文件绝对路径 (mp3/mp4/wav…)"
                    value={importPath}
                    onInput={e => setImportPath((e.target as HTMLInputElement).value)}
                />
                <input
                    class="speech-clip-input speech-clip-input-label"
                    placeholder="标签 (如: 个人介绍)"
                    value={importLabel}
                    onInput={e => setImportLabel((e.target as HTMLInputElement).value)}
                />
                <button class="speech-clip-btn" onClick={doImport} disabled={!importPath}>
                    导入素材
                </button>
                {!isRecording ? (
                    <button class="speech-clip-btn speech-clip-btn-rec" onClick={startRec} disabled={!!busy}>
                        ● 录制
                    </button>
                ) : (
                    <button class="speech-clip-btn speech-clip-btn-recording" onClick={stopRec}>
                        ■ 停止并保存
                    </button>
                )}
                {busy && <span class="speech-clip-busy">{busy}</span>}
            </div>

            <div class="speech-clip-body">
                <div class="speech-clip-assets">
                    {assets.length === 0 && <div class="speech-clip-hint">还没有素材，先导入。</div>}
                    {assets.map(a => (
                        <div class={`speech-clip-asset ${a.id === sel ? 'is-sel' : ''}`} onClick={() => setSel(a.id)}>
                            <div class="speech-clip-asset-head">
                                <span class="speech-clip-asset-label">{a.label || a.id}</span>
                                <span class="speech-clip-asset-id">{a.id}</span>
                            </div>
                            <div class="speech-clip-asset-status">
                                {a.transcribed ? `转录 ${a.transcriptSentences} 句` : '未转录'}
                                {a.highlighted ? ` · 金句 ${a.highlights}` : ''}
                            </div>
                            <div class="speech-clip-asset-actions">
                                <button
                                    class="speech-clip-btn-sm"
                                    onClick={e => {
                                        e.stopPropagation();
                                        doTranscribe(a.id);
                                    }}
                                >
                                    {a.transcribed ? '重转录' : '转录'}
                                </button>
                                <button
                                    class="speech-clip-btn-sm"
                                    disabled={!a.transcribed}
                                    onClick={e => {
                                        e.stopPropagation();
                                        doExtract(a.id);
                                    }}
                                >
                                    {a.highlighted ? '重提金句' : '提金句'}
                                </button>
                            </div>
                        </div>
                    ))}
                </div>

                <div class="speech-clip-table-wrap">
                    {selAsset && (
                        <div class="speech-clip-table-head">
                            {selAsset.label || selAsset.id} · {rows.length} 句 · 已选金句 {pickedCount}
                        </div>
                    )}
                    <div class="speech-clip-table-scroll">
                        <table class="speech-clip-table">
                            <thead>
                                <tr>
                                    <th>金句</th>
                                    <th>时间</th>
                                    <th>来源</th>
                                    <th>原文 → 纠错</th>
                                    <th>理由</th>
                                </tr>
                            </thead>
                            <tbody>
                                {rows.map(r => (
                                    <tr class={r.picked ? 'is-picked' : ''}>
                                        <td class="speech-clip-td-pick">
                                            <input type="checkbox" checked={r.picked} onChange={() => togglePick(r)} />
                                        </td>
                                        <td class="speech-clip-td-time">{fmtTime(r.start)}</td>
                                        <td class="speech-clip-td-src">{r.asset}</td>
                                        <td class="speech-clip-td-text">
                                            <div class="speech-clip-orig">{r.text}</div>
                                            {r.corrected && r.corrected !== r.text && (
                                                <div class="speech-clip-corr">{r.corrected}</div>
                                            )}
                                        </td>
                                        <td class="speech-clip-td-reason">{r.reason}</td>
                                    </tr>
                                ))}
                                {rows.length === 0 && (
                                    <tr>
                                        <td colSpan={5} class="speech-clip-hint">
                                            {selAsset ? '尚无转录，点「转录」。' : '选择一个素材。'}
                                        </td>
                                    </tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                </div>
            </div>
        </div>
    );
}
