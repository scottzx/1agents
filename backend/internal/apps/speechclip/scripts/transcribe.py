#!/usr/bin/env python3
# -*- encoding: utf-8 -*-
"""FunClip ASR 转写：输出逐句 jsonl（每句带 asset 来源标记 + 说话人）。
用法: transcribe.py <audio> <out_jsonl> <asset_id> <funclip_dir>
最后一行 stdout 打印 JSON 摘要，供 Go function runner 读取写回 task.Result。
"""
import os
import sys
import json


def main():
    if len(sys.argv) < 5:
        print(json.dumps({"error": "usage: transcribe.py <audio> <out_jsonl> <asset_id> <funclip_dir>"}))
        sys.exit(2)
    audio, out_jsonl, asset_id, funclip_dir = sys.argv[1:5]
    sys.path.insert(0, os.path.join(funclip_dir, "funclip"))

    import librosa
    from funasr import AutoModel
    from videoclipper import VideoClipper

    def load_audio_16k(path):
        """Load mono 16k float audio. Falls back to an ffmpeg transcode for
        formats librosa can't decode directly (in-browser webm/opus recordings)."""
        try:
            wav, _ = librosa.load(path, sr=16000)
            if len(wav) > 0:
                return wav
        except Exception:
            pass
        import subprocess
        import tempfile
        import imageio_ffmpeg
        ff = imageio_ffmpeg.get_ffmpeg_exe()
        tmp = tempfile.NamedTemporaryFile(suffix=".wav", delete=False).name
        try:
            subprocess.run([ff, "-y", "-i", path, "-ac", "1", "-ar", "16000", tmp],
                           check=True, capture_output=True)
            wav, _ = librosa.load(tmp, sr=16000)
            return wav
        finally:
            if os.path.exists(tmp):
                os.remove(tmp)

    m = AutoModel(
        model="iic/speech_seaco_paraformer_large_asr_nat-zh-cn-16k-common-vocab8404-pytorch",
        vad_model="damo/speech_fsmn_vad_zh-cn-16k-common-pytorch",
        punc_model="damo/punc_ct-transformer_zh-cn-common-vocab272727-pytorch",
        spk_model="damo/speech_campplus_sv_zh-cn_16k-common",
        disable_update=True,
    )
    clipper = VideoClipper(m)
    clipper.lang = "zh"

    data = load_audio_16k(audio)
    # sd_switch=Yes 开说话人分离，拿到每句 spk
    _, _, state = clipper.recog((16000, data), sd_switch="Yes")
    sents = state.get("sd_sentences") or state.get("sentences") or []

    os.makedirs(os.path.dirname(out_jsonl), exist_ok=True)
    n = 0
    with open(out_jsonl, "w", encoding="utf-8") as f:
        for i, s in enumerate(sents):
            rec = {
                "i": i,
                "asset": asset_id,             # 来源素材标记(混剪定位)
                "text": (s.get("text") or "").strip(),
                "start": s.get("start"),       # 该素材内部时间轴(ms)
                "end": s.get("end"),
                "spk": s.get("spk"),
            }
            f.write(json.dumps(rec, ensure_ascii=False) + "\n")
            n += 1

    summary = {
        "asset": asset_id,
        "sentences": n,
        "duration_ms": int(len(data) / 16000 * 1000),
        "file": out_jsonl,
    }
    print(json.dumps(summary, ensure_ascii=False))


if __name__ == "__main__":
    main()
