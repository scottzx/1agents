//! 智能录音 —— 本地推理(sherpa-onnx SenseVoice + 说话人分离)+ 本地持久化。
//!
//! 设计约束(服务自己):并发 = 1,识别器在调用内创建、用完即 drop、不常驻、按需拉起。
//!
//! 分层:
//!   engine::transcribe_and_diarize  纯计算核心(未来抽成 `sherpa-engine` crate,relay-worker 复用)
//!   store::*                        持久化:sqlite(~/.1agents/meta.db) + jsonl(~/.1agents/recording/<YYYYMMDD>/<id>.jsonl)
//!   #[tauri::command]               IPC 层,前端 invoke
//!
//! 门阀在前端:isTauri() && macOS 才暴露入口;Windows 一期不开(见 recording.ts)。

use base64::Engine as _;
use serde::{Deserialize, Serialize};
use std::path::PathBuf;

// ---------------------------------------------------------------------------
// 数据模型(serde camelCase,直接对接前端 TS)
// ---------------------------------------------------------------------------

#[derive(Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Utterance {
    pub speaker: String, // "speaker_0" / "speaker_1"
    pub start: f32,      // 秒
    pub end: f32,
    pub text: String,
}

#[derive(Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Recording {
    pub id: String,
    pub created_at: i64, // unix 秒
    pub duration: f32,
    pub speaker_count: usize,
    pub title: String,
    pub full_text: String,
    pub summary: Option<String>, // 1acp 异步回填
    /// 仅 get_recording 时从 jsonl 加载;list 时为空。
    #[serde(default)]
    pub utterances: Vec<Utterance>,
}

// ---------------------------------------------------------------------------
// 引擎核心 —— 纯计算,无 IO 无状态。relay-worker 未来直接复用这一段。
// ---------------------------------------------------------------------------

mod engine {
    use super::Utterance;
    use sherpa_rs::{
        diarize::{Diarize, DiarizeConfig},
        sense_voice::{SenseVoiceConfig, SenseVoiceRecognizer},
    };

    /// macOS 走 CoreML(吃 ANE/GPU),其余回退 CPU。
    /// 注意:CoreML EP 部分算子会回退 CPU,上线前需 coreml vs cpu 实测对比。
    fn provider() -> String {
        if cfg!(target_os = "macos") {
            "coreml".into()
        } else {
            "cpu".into()
        }
    }

    pub struct Transcript {
        pub utterances: Vec<Utterance>,
        pub full_text: String,
        pub speaker_count: usize,
        pub duration: f32,
    }

    /// 16k 单声道 f32 → 带说话人 + 时间戳的转写。
    /// 识别器在本函数内创建、返回时 drop(并发=1、不常驻)。
    pub fn transcribe_and_diarize(samples: Vec<f32>, sr: u32, models: &str) -> Result<Transcript, String> {
        if sr != 16000 {
            return Err(format!("expected 16kHz mono, got {sr}"));
        }

        // 1) 说话人分离:谁在第几秒说话
        //    num_clusters<=0 → 按 threshold 自动估计人数(绑定里 None 会回落成固定 4 人,故传 -1)
        let mut dia = Diarize::new(
            format!("{models}/segmentation.onnx"),
            format!("{models}/embedding.onnx"),
            DiarizeConfig {
                num_clusters: Some(-1),
                threshold: Some(0.5),
                provider: Some(provider()),
                ..Default::default()
            },
        )
        .map_err(|e| e.to_string())?;
        let segments = dia.compute(samples.clone(), None).map_err(|e| e.to_string())?;

        // 2) 逐段 ASR(SenseVoice 自动识别 zh/en/ja/ko/yue,无需 language 字段)
        let mut asr = SenseVoiceRecognizer::new(SenseVoiceConfig {
            model: format!("{models}/model.int8.onnx"),
            tokens: format!("{models}/tokens.txt"),
            provider: Some(provider()),
            ..Default::default()
        })
        .map_err(|e: eyre::Error| e.to_string())?;

        let mut utterances = Vec::new();
        let mut full_text = String::new();
        let mut speakers = std::collections::HashSet::new();

        for seg in &segments {
            let a = (seg.start * sr as f32) as usize;
            let b = ((seg.end * sr as f32) as usize).min(samples.len());
            if a >= b {
                continue;
            }
            let text = asr.transcribe(sr, &samples[a..b]).text.trim().to_string();
            if text.is_empty() {
                continue;
            }
            speakers.insert(seg.speaker);
            full_text.push_str(&text);
            full_text.push('\n');
            utterances.push(Utterance {
                speaker: format!("speaker_{}", seg.speaker),
                start: seg.start,
                end: seg.end,
                text,
            });
        }

        Ok(Transcript {
            duration: samples.len() as f32 / sr as f32,
            speaker_count: speakers.len(),
            utterances,
            full_text,
        })
    }
}

// ---------------------------------------------------------------------------
// 持久化 —— sqlite 存元数据,jsonl 存原文。每次开新连接(并发=1,无需连接池)。
// ---------------------------------------------------------------------------

mod store {
    use super::{engine::Transcript, Recording, Utterance};
    use rusqlite::{params, Connection};
    use std::io::{BufRead, Write};
    use std::path::PathBuf;

    fn base_dir() -> PathBuf {
        dirs::home_dir().expect("no home dir").join(".1agents")
    }

    fn db_path() -> PathBuf {
        base_dir().join("meta.db")
    }

    /// ~/.1agents/recording/<YYYYMMDD>/<id>.jsonl(按本地日期分目录)
    fn jsonl_path(id: &str, created_at: i64) -> PathBuf {
        let day = chrono::DateTime::from_timestamp(created_at, 0)
            .unwrap_or_else(chrono::Utc::now)
            .with_timezone(&chrono::Local)
            .format("%Y%m%d")
            .to_string();
        base_dir().join("recording").join(day).join(format!("{id}.jsonl"))
    }

    fn open_db() -> Result<Connection, String> {
        std::fs::create_dir_all(base_dir()).map_err(|e| e.to_string())?;
        let conn = Connection::open(db_path()).map_err(|e| e.to_string())?;
        conn.execute(
            "CREATE TABLE IF NOT EXISTS recordings (
                id            TEXT PRIMARY KEY,
                created_at    INTEGER NOT NULL,
                duration      REAL    NOT NULL,
                speaker_count INTEGER NOT NULL,
                title         TEXT    NOT NULL,
                full_text     TEXT    NOT NULL,
                summary       TEXT,
                jsonl_path    TEXT    NOT NULL
            )",
            [],
        )
        .map_err(|e| e.to_string())?;
        Ok(conn)
    }

    fn derive_title(full_text: &str, created_at: i64) -> String {
        let first = full_text.lines().next().unwrap_or("").trim();
        if first.is_empty() {
            chrono::DateTime::from_timestamp(created_at, 0)
                .unwrap_or_else(chrono::Utc::now)
                .with_timezone(&chrono::Local)
                .format("录音 %Y-%m-%d %H:%M")
                .to_string()
        } else {
            first.chars().take(20).collect()
        }
    }

    pub fn save(t: Transcript) -> Result<Recording, String> {
        let id = uuid::Uuid::new_v4().to_string();
        let created_at = chrono::Local::now().timestamp();
        let title = derive_title(&t.full_text, created_at);
        let path = jsonl_path(&id, created_at);

        // 原文 → jsonl(每行一条 Utterance)
        std::fs::create_dir_all(path.parent().unwrap()).map_err(|e| e.to_string())?;
        let mut f = std::fs::File::create(&path).map_err(|e| e.to_string())?;
        for u in &t.utterances {
            let line = serde_json::to_string(u).map_err(|e| e.to_string())?;
            writeln!(f, "{line}").map_err(|e| e.to_string())?;
        }

        // 元数据 → sqlite
        open_db()?
            .execute(
                "INSERT INTO recordings
                    (id, created_at, duration, speaker_count, title, full_text, summary, jsonl_path)
                 VALUES (?1, ?2, ?3, ?4, ?5, ?6, NULL, ?7)",
                params![
                    id,
                    created_at,
                    t.duration,
                    t.speaker_count as i64,
                    title,
                    t.full_text,
                    path.to_string_lossy(),
                ],
            )
            .map_err(|e| e.to_string())?;

        Ok(Recording {
            id,
            created_at,
            duration: t.duration,
            speaker_count: t.speaker_count,
            title,
            full_text: t.full_text,
            summary: None,
            utterances: t.utterances,
        })
    }

    /// 列表:不加载 utterances(详情才读 jsonl)。
    pub fn list() -> Result<Vec<Recording>, String> {
        let conn = open_db()?;
        let mut stmt = conn
            .prepare(
                "SELECT id, created_at, duration, speaker_count, title, full_text, summary
                 FROM recordings ORDER BY created_at DESC",
            )
            .map_err(|e| e.to_string())?;
        let rows = stmt
            .query_map([], |r| {
                Ok(Recording {
                    id: r.get(0)?,
                    created_at: r.get(1)?,
                    duration: r.get(2)?,
                    speaker_count: r.get::<_, i64>(3)? as usize,
                    title: r.get(4)?,
                    full_text: r.get(5)?,
                    summary: r.get(6)?,
                    utterances: Vec::new(),
                })
            })
            .map_err(|e| e.to_string())?;
        rows.collect::<Result<Vec<_>, _>>().map_err(|e| e.to_string())
    }

    pub fn get(id: &str) -> Result<Recording, String> {
        let conn = open_db()?;
        let (mut rec, jsonl): (Recording, String) = conn
            .query_row(
                "SELECT id, created_at, duration, speaker_count, title, full_text, summary, jsonl_path
                 FROM recordings WHERE id = ?1",
                params![id],
                |r| {
                    Ok((
                        Recording {
                            id: r.get(0)?,
                            created_at: r.get(1)?,
                            duration: r.get(2)?,
                            speaker_count: r.get::<_, i64>(3)? as usize,
                            title: r.get(4)?,
                            full_text: r.get(5)?,
                            summary: r.get(6)?,
                            utterances: Vec::new(),
                        },
                        r.get(7)?,
                    ))
                },
            )
            .map_err(|e| e.to_string())?;

        // 原文从 jsonl 读回
        if let Ok(file) = std::fs::File::open(&jsonl) {
            for line in std::io::BufReader::new(file).lines().map_while(Result::ok) {
                if line.trim().is_empty() {
                    continue;
                }
                if let Ok(u) = serde_json::from_str::<Utterance>(&line) {
                    rec.utterances.push(u);
                }
            }
        }
        Ok(rec)
    }

    pub fn update_summary(id: &str, summary: &str) -> Result<(), String> {
        open_db()?
            .execute("UPDATE recordings SET summary = ?1 WHERE id = ?2", params![summary, id])
            .map_err(|e| e.to_string())?;
        Ok(())
    }

    pub fn delete(id: &str) -> Result<(), String> {
        let conn = open_db()?;
        // 先取 jsonl 路径再删行
        if let Ok(path) = conn.query_row(
            "SELECT jsonl_path FROM recordings WHERE id = ?1",
            params![id],
            |r| r.get::<_, String>(0),
        ) {
            let _ = std::fs::remove_file(path);
        }
        conn.execute("DELETE FROM recordings WHERE id = ?1", params![id])
            .map_err(|e| e.to_string())?;
        Ok(())
    }
}

// ---------------------------------------------------------------------------
// 模型目录:一期约定放 ~/.1agents/models/(用户自备模型,后续可改打进 Tauri resources)
// 需要:model.int8.onnx + tokens.txt(SenseVoice)、segmentation.onnx、embedding.onnx
// ---------------------------------------------------------------------------

fn models_dir() -> String {
    dirs::home_dir()
        .map(|h| h.join(".1agents").join("models"))
        .unwrap_or_else(|| PathBuf::from("."))
        .to_string_lossy()
        .into_owned()
}

/// 前端传 16k 单声道 PCM(i16 小端)的 base64 → f32 样本。
fn decode_pcm16(b64: &str) -> Result<Vec<f32>, String> {
    let bytes = base64::engine::general_purpose::STANDARD
        .decode(b64)
        .map_err(|e| e.to_string())?;
    Ok(bytes
        .chunks_exact(2)
        .map(|c| i16::from_le_bytes([c[0], c[1]]) as f32 / 32768.0)
        .collect())
}

// ---------------------------------------------------------------------------
// Tauri 命令(IPC 契约)
// ---------------------------------------------------------------------------

/// 录音结束 → 转写 + 分离 + 落库,返回 Recording(summary 仍为 None,由前端调 1acp 后回填)。
/// 重活在 spawn_blocking,避免阻塞 WebView 主线程。
#[tauri::command]
pub async fn transcribe_and_save(pcm_base64: String, sample_rate: u32) -> Result<Recording, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let samples = decode_pcm16(&pcm_base64)?;
        let t = engine::transcribe_and_diarize(samples, sample_rate, &models_dir())?;
        store::save(t)
    })
    .await
    .map_err(|e| e.to_string())?
}

#[tauri::command]
pub fn list_recordings() -> Result<Vec<Recording>, String> {
    store::list()
}

#[tauri::command]
pub fn get_recording(id: String) -> Result<Recording, String> {
    store::get(&id)
}

#[tauri::command]
pub fn update_recording_summary(id: String, summary: String) -> Result<(), String> {
    store::update_summary(&id, &summary)
}

#[tauri::command]
pub fn delete_recording(id: String) -> Result<(), String> {
    store::delete(&id)
}

fn save_base64_file(path: PathBuf, b64: &str) -> Result<(), String> {
    let bytes = base64::engine::general_purpose::STANDARD
        .decode(b64)
        .map_err(|e| e.to_string())?;
    if let Some(parent) = path.parent() {
        std::fs::create_dir_all(parent).map_err(|e| e.to_string())?;
    }
    std::fs::write(path, bytes).map_err(|e| e.to_string())?;
    Ok(())
}

/// 保存录像资产：视频轨道和音频轨道
#[tauri::command]
pub async fn save_studio_assets(
    id: String,
    webcam_base64: String,
    screen_base64: String,
    audio_base64: String,
) -> Result<String, String> {
    tauri::async_runtime::spawn_blocking(move || {
        let dir = dirs::home_dir()
            .expect("no home dir")
            .join(".1agents")
            .join("studio")
            .join(&id);

        let webcam_path = dir.join("webcam.webm");
        let screen_path = dir.join("screen.webm");
        let audio_path = dir.join("audio.webm");

        save_base64_file(webcam_path, &webcam_base64)?;
        save_base64_file(screen_path, &screen_base64)?;
        save_base64_file(audio_path, &audio_base64)?;

        Ok(dir.to_string_lossy().into_owned())
    })
    .await
    .map_err(|e| e.to_string())?
}
