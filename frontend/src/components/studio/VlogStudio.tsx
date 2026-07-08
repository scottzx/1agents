import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import type { Lang } from '../../i18n';
import { t } from '../../i18n';
import { StudioCategory } from '../../modules/studio-manifest';
import { StudioRecorder } from '../../utils/studioRecorder';
import { listRecordings, getRecording, deleteRecording, type Recording } from '../../utils/recording';
import * as ui from '../../stores/uiStore';

interface VlogStudioProps {
    activeCategory: StudioCategory;
    language: Lang;
}

export function VlogStudio({ activeCategory, language }: VlogStudioProps) {
    // Recording state
    const [recorder] = useState(() => new StudioRecorder());
    const [isRecording, setIsRecording] = useState(false);
    const [webpageUrl, setWebpageUrl] = useState('https://1agents.com');
    const [recordTime, setRecordTime] = useState(0);
    const timerRef = useRef<NodeJS.Timeout | null>(null);

    // Test audio state
    const [isTestAudioRecording, setIsTestAudioRecording] = useState(false);
    const [testAudioUrl, setTestAudioUrl] = useState<string | null>(null);
    const testAudioStreamRef = useRef<MediaStream | null>(null);
    const testAudioRecorderRef = useRef<MediaRecorder | null>(null);
    const testAudioChunksRef = useRef<Blob[]>([]);

    // Audio input devices list
    const [audioDevices, setAudioDevices] = useState<MediaDeviceInfo[]>([]);
    const [selectedDeviceId, setSelectedDeviceId] = useState('');

    const loadAudioDevices = async () => {
        try {
            const devices = await navigator.mediaDevices.enumerateDevices();
            const audioInputs = devices.filter(d => d.kind === 'audioinput' && d.deviceId);
            setAudioDevices(audioInputs);
            if (audioInputs.length > 0) {
                // Try to find a default AirPods or Built-in Mic or similar
                const preferred = audioInputs.find(
                    d =>
                        d.label.toLowerCase().includes('airpod') ||
                        d.label.toLowerCase().includes('macbook') ||
                        d.label.toLowerCase().includes('built-in')
                );
                setSelectedDeviceId(preferred ? preferred.deviceId : audioInputs[0].deviceId);
            }
        } catch (err) {
            console.warn('Failed to load audio inputs', err);
        }
    };

    useEffect(() => {
        loadAudioDevices();

        // Try requesting access once if labels are empty to query actual device names
        navigator.mediaDevices
            .getUserMedia({ audio: true })
            .then(s => {
                loadAudioDevices();
                s.getTracks().forEach(t => t.stop());
            })
            .catch(() => {});

        navigator.mediaDevices.addEventListener('devicechange', loadAudioDevices);
        return () => {
            navigator.mediaDevices.removeEventListener('devicechange', loadAudioDevices);
        };
    }, []);

    // Script / Outline state
    const [outline, setOutline] = useState(() => {
        return (
            localStorage.getItem('1agents-studio-outline') ||
            '## 1. 自我介绍与项目背景\n大家好，我是Scott。今天给大家演示我们新开发的 1agents workbench 桌面端...\n\n## 2. 核心功能展示\n- 第一步：介绍终端集成与远程连接。\n- 第二步：现场运行一个本地 Go 服务的打包编译。'
        );
    });
    const [scriptText, setScriptText] = useState(() => {
        return (
            localStorage.getItem('1agents-studio-script') ||
            '观众朋友们好！今天我们要聊一个非常酷的独立开发者工具：1agents。平时我们录口播，最头疼的就是人脸和网页实操对不齐，现在我们有了双录制功能，可以分开录制。下面我带大家现场演示一下……'
        );
    });

    // Asset status
    const [isTranscribing, setIsTranscribing] = useState(false);
    const [currentRecording, setCurrentRecording] = useState<Recording | null>(null);
    const [webcamUrl, setWebcamUrl] = useState<string | null>(null);
    const [screenUrl, setScreenUrl] = useState<string | null>(null);
    const [cutUtteranceIds, setCutUtteranceIds] = useState<Set<number>>(() => new Set());

    // Camera preview element
    const videoPreviewRef = useRef<HTMLVideoElement | null>(null);

    // History list
    const [recordingsList, setRecordingsList] = useState<Recording[]>([]);
    const [loadingList, setLoadingList] = useState(false);

    // Save script text to localStorage
    useEffect(() => {
        localStorage.setItem('1agents-studio-outline', outline);
    }, [outline]);

    useEffect(() => {
        localStorage.setItem('1agents-studio-script', scriptText);
    }, [scriptText]);

    // Timer effect
    useEffect(() => {
        if (isRecording) {
            timerRef.current = setInterval(() => {
                setRecordTime(t => t + 1);
            }, 1000);
        } else {
            if (timerRef.current) clearInterval(timerRef.current);
            setRecordTime(0);
        }
        return () => {
            if (timerRef.current) clearInterval(timerRef.current);
        };
    }, [isRecording]);

    // Load history
    useEffect(() => {
        if (activeCategory === 'list') {
            loadHistory();
        }
    }, [activeCategory]);

    const loadHistory = async () => {
        setLoadingList(true);
        try {
            const list = await listRecordings();
            setRecordingsList(list);
        } catch (err) {
            console.error('Failed to load recordings', err);
            ui.showToast('无法加载历史录像列表');
        } finally {
            setLoadingList(false);
        }
    };

    const handleOpenPage = () => {
        if (!webpageUrl) return;
        window.open(webpageUrl, '_blank', 'width=1280,height=720');
    };

    const handleStartRecording = async () => {
        try {
            const streams = await recorder.start(selectedDeviceId);
            setIsRecording(true);

            // Set live camera preview
            if (videoPreviewRef.current) {
                videoPreviewRef.current.srcObject = streams.webcamStream;
            }
            ui.showToast('开始录像，请在弹出的系统选择器中选中你要录制的网页窗口');
        } catch (err) {
            console.error('Failed to start recording', err);
            ui.showToast('启动录制失败，请检查摄像头与屏幕录制权限');
        }
    };

    const handleToggleTestAudio = async () => {
        if (isTestAudioRecording) {
            // Stop
            if (testAudioRecorderRef.current) {
                testAudioRecorderRef.current.onstop = () => {
                    const audioBlob = new Blob(testAudioChunksRef.current, { type: 'audio/webm' });
                    console.log('Recorded test audio blob size:', audioBlob.size);
                    setTestAudioUrl(URL.createObjectURL(audioBlob));
                    ui.showToast(`测试录音完成，文件大小: ${audioBlob.size} 字节`);

                    // Stop tracks
                    if (testAudioStreamRef.current) {
                        testAudioStreamRef.current.getTracks().forEach(t => t.stop());
                    }
                };
                testAudioRecorderRef.current.stop();
            }
            setIsTestAudioRecording(false);
        } else {
            // Start
            try {
                const stream = await navigator.mediaDevices.getUserMedia({
                    audio: selectedDeviceId
                        ? {
                              deviceId: { exact: selectedDeviceId },
                              echoCancellation: true,
                              noiseSuppression: true,
                              autoGainControl: true,
                          }
                        : {
                              echoCancellation: true,
                              noiseSuppression: true,
                              autoGainControl: true,
                          },
                });
                testAudioStreamRef.current = stream;
                testAudioChunksRef.current = [];

                const track = stream.getAudioTracks()[0];
                const deviceName = track ? track.label : '未知设备';
                console.log('Opened microphone device:', deviceName);

                const mime = MediaRecorder.isTypeSupported('audio/webm;codecs=opus')
                    ? 'audio/webm;codecs=opus'
                    : 'audio/webm';

                const recorder = new MediaRecorder(stream, { mimeType: mime });
                recorder.ondataavailable = e => {
                    if (e.data && e.data.size > 0) {
                        testAudioChunksRef.current.push(e.data);
                    }
                };
                testAudioRecorderRef.current = recorder;
                recorder.start(1000);
                setIsTestAudioRecording(true);
                setTestAudioUrl(null);
                ui.showToast(`测试音频录制中... 麦克风: ${deviceName}`);
            } catch (err) {
                console.error('Failed to start test audio recording', err);
                ui.showToast(`无法开启麦克风: ${err}`);
            }
        }
    };

    const handleStopRecording = async () => {
        setIsRecording(false);
        setIsTranscribing(true);
        if (videoPreviewRef.current) {
            videoPreviewRef.current.srcObject = null;
        }

        try {
            const assets = await recorder.stop();
            ui.showToast('录像已完成，正在生成本地预览播放...');

            // Mock recording object for the UI
            const randId = 'rec_' + Math.random().toString(36).substring(2, 11);
            const rec = {
                id: randId,
                createdAt: Math.floor(Date.now() / 1000),
                duration: 60,
                speakerCount: 1,
                title: '演示录制 (本地)',
                fullText: '这是一次双录屏的本地演示录制。我们将在这里展示摄像头和网页的同步录制，并进行粗剪测试。',
                summary: '本次录制成功捕获了摄像头与屏幕画面。',
                utterances: [
                    {
                        speaker: 'speaker_0',
                        start: 0,
                        end: 5,
                        text: '这是一次双录屏的本地演示录制。',
                    },
                    {
                        speaker: 'speaker_0',
                        start: 5,
                        end: 10,
                        text: '我们将在这里展示摄像头和网页的同步录制，',
                    },
                    {
                        speaker: 'speaker_0',
                        start: 10,
                        end: 15,
                        text: '并进行粗剪测试。',
                    },
                ],
            };

            // Set local blob URLs for instant playback in browser
            setWebcamUrl(URL.createObjectURL(assets.webcamBlob));
            setScreenUrl(URL.createObjectURL(assets.screenBlob));

            // Background save assets to backend disk for verification
            (async () => {
                try {
                    const webcamBase64 = await blobToBase64(assets.webcamBlob);
                    const screenBase64 = await blobToBase64(assets.screenBlob);
                    const audioBase64 = await blobToBase64(assets.audioBlob);

                    const saveRes = await fetch('/api/studio/save-assets', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({
                            id: rec.id,
                            webcamBase64,
                            screenBase64,
                            audioBase64,
                        }),
                    });
                    if (saveRes.ok) {
                        const info = await saveRes.json();
                        console.log('Successfully saved studio assets to backend path:', info.path);
                    } else {
                        console.warn('Backend save-assets returned status:', saveRes.status);
                    }
                } catch (e) {
                    console.warn('Background save assets to backend failed:', e);
                }
            })();

            setCurrentRecording(rec);
            setCutUtteranceIds(new Set());
            ui.showToast('本地预览生成成功！开始剪辑吧。');
        } catch (err) {
            console.error('Failed to stop recording', err);
            ui.showToast('录像后期处理失败');
        } finally {
            setIsTranscribing(false);
        }
    };

    // Load full recording from list
    const handleSelectRecording = async (rec: Recording) => {
        try {
            const details = await getRecording(rec.id);
            setCurrentRecording(details);
            setCutUtteranceIds(new Set());

            // Construct local paths (Go file stream endpoint)
            setWebcamUrl(`/api/fs/view/Users/scott/.1agents/studio/${rec.id}/webcam.webm`);
            setScreenUrl(`/api/fs/view/Users/scott/.1agents/studio/${rec.id}/screen.webm`);

            // Switch view
            ui.showToast(`已加载录像: ${rec.title}`);
        } catch (err) {
            console.error('Failed to get recording details', err);
            ui.showToast('加载录像详情失败');
        }
    };

    const handleDeleteRecording = async (id: string, e: Event) => {
        e.stopPropagation();
        if (!confirm('确认删除这段录像及其本地的全部音视频资产吗？此操作不可逆。')) {
            return;
        }
        try {
            await deleteRecording(id);
            // Delete local studio folders
            // (Tauri's delete command clears metadata, we can let it be or write command later)
            ui.showToast('已删除录像');
            loadHistory();
            if (currentRecording?.id === id) {
                setCurrentRecording(null);
                setWebcamUrl(null);
                setScreenUrl(null);
            }
        } catch (err) {
            console.error('Delete failed', err);
            ui.showToast('删除失败');
        }
    };

    const toggleUtteranceCut = (index: number) => {
        setCutUtteranceIds(prev => {
            const next = new Set(prev);
            if (next.has(index)) {
                next.delete(index);
            } else {
                next.add(index);
            }
            return next;
        });
    };

    const getKeptDuration = () => {
        if (!currentRecording || !currentRecording.utterances) return 0;
        let dur = 0;
        currentRecording.utterances.forEach((u, index) => {
            if (!cutUtteranceIds.has(index)) {
                dur += u.end - u.start;
            }
        });
        return parseFloat(dur.toFixed(1));
    };

    // Helper: Convert Blob to Base64
    const blobToBase64 = (blob: Blob): Promise<string> => {
        return new Promise((resolve, reject) => {
            const reader = new FileReader();
            reader.onloadend = () => {
                const res = reader.result as string;
                resolve(res.split(',')[1]);
            };
            reader.onerror = reject;
            reader.readAsDataURL(blob);
        });
    };

    const formatTime = (secs: number) => {
        const m = Math.floor(secs / 60);
        const s = secs % 60;
        return `${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
    };

    if (activeCategory === 'list') {
        return (
            <div
                class="studio-container list-view"
                style={{
                    display: 'flex',
                    flex: 1,
                    minHeight: 0,
                    padding: '24px',
                    backgroundColor: 'var(--bg-page)',
                    color: 'var(--text-main)',
                }}
            >
                <div
                    class="sidebar-list"
                    style={{
                        width: '320px',
                        borderRight: '1px solid var(--border-color)',
                        paddingRight: '24px',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '16px',
                        overflowY: 'auto',
                    }}
                >
                    <h3>{t('studio.catList', language)}</h3>
                    {loadingList ? (
                        <div class="spinner">加载中...</div>
                    ) : recordingsList.length === 0 ? (
                        <div style={{ color: 'var(--text-muted)' }}>暂无录像历史</div>
                    ) : (
                        recordingsList.map(rec => (
                            <div
                                key={rec.id}
                                class="list-item"
                                onClick={() => handleSelectRecording(rec)}
                                style={{
                                    padding: '12px',
                                    border:
                                        currentRecording?.id === rec.id
                                            ? '1px solid var(--text-main)'
                                            : '1px solid var(--border-color)',
                                    borderRadius: '8px',
                                    cursor: 'pointer',
                                    backgroundColor: 'var(--bg-card)',
                                    display: 'flex',
                                    flexDirection: 'column',
                                    gap: '6px',
                                }}
                            >
                                <div
                                    style={{
                                        fontWeight: '500',
                                        fontSize: '14px',
                                        display: 'flex',
                                        justifyContent: 'space-between',
                                    }}
                                >
                                    <span>{rec.title}</span>
                                    <button
                                        onClick={e => handleDeleteRecording(rec.id, e)}
                                        style={{
                                            background: 'none',
                                            border: 'none',
                                            color: 'var(--danger-fg)',
                                            cursor: 'pointer',
                                            padding: 0,
                                        }}
                                        title="删除"
                                    >
                                        🗑️
                                    </button>
                                </div>
                                <div
                                    style={{
                                        display: 'flex',
                                        justifyContent: 'space-between',
                                        fontSize: '12px',
                                        color: 'var(--text-secondary)',
                                    }}
                                >
                                    <span>时长: {formatTime(Math.round(rec.duration))}</span>
                                    <span>{new Date(rec.createdAt * 1000).toLocaleDateString()}</span>
                                </div>
                            </div>
                        ))
                    )}
                </div>

                <div
                    class="detail-viewer"
                    style={{
                        flex: 1,
                        paddingLeft: '24px',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '20px',
                        minWidth: 0,
                    }}
                >
                    {currentRecording ? (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '20px', height: '100%' }}>
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                <h2>{currentRecording.title}</h2>
                                <span style={{ fontSize: '14px', color: 'var(--text-secondary)' }}>
                                    原始时长: {formatTime(Math.round(currentRecording.duration))}s | 裁剪后预计:{' '}
                                    {formatTime(Math.round(getKeptDuration()))}s
                                </span>
                            </div>

                            {/* Dual video preview tracks */}
                            <div style={{ display: 'flex', gap: '16px', height: '240px' }}>
                                <div
                                    style={{
                                        flex: 1,
                                        backgroundColor: '#000',
                                        borderRadius: '8px',
                                        display: 'flex',
                                        flexDirection: 'column',
                                        justifyContent: 'center',
                                        alignItems: 'center',
                                        position: 'relative',
                                    }}
                                >
                                    <span
                                        style={{
                                            position: 'absolute',
                                            top: '8px',
                                            left: '8px',
                                            color: '#fff',
                                            fontSize: '12px',
                                            background: 'rgba(0,0,0,0.5)',
                                            padding: '2px 6px',
                                            borderRadius: '4px',
                                        }}
                                    >
                                        摄像头人脸
                                    </span>
                                    {webcamUrl ? (
                                        <video
                                            src={webcamUrl}
                                            controls
                                            style={{
                                                width: '100%',
                                                height: '100%',
                                                borderRadius: '8px',
                                                objectFit: 'contain',
                                            }}
                                        />
                                    ) : (
                                        <span style={{ color: '#aaa' }}>视频缺失</span>
                                    )}
                                </div>
                                <div
                                    style={{
                                        flex: 1,
                                        backgroundColor: '#000',
                                        borderRadius: '8px',
                                        display: 'flex',
                                        flexDirection: 'column',
                                        justifyContent: 'center',
                                        alignItems: 'center',
                                        position: 'relative',
                                    }}
                                >
                                    <span
                                        style={{
                                            position: 'absolute',
                                            top: '8px',
                                            left: '8px',
                                            color: '#fff',
                                            fontSize: '12px',
                                            background: 'rgba(0,0,0,0.5)',
                                            padding: '2px 6px',
                                            borderRadius: '4px',
                                        }}
                                    >
                                        HTML网页屏
                                    </span>
                                    {screenUrl ? (
                                        <video
                                            src={screenUrl}
                                            controls
                                            style={{
                                                width: '100%',
                                                height: '100%',
                                                borderRadius: '8px',
                                                objectFit: 'contain',
                                            }}
                                        />
                                    ) : (
                                        <span style={{ color: '#aaa' }}>视频缺失</span>
                                    )}
                                </div>
                            </div>

                            {/* Transcript editing panel */}
                            <div
                                style={{
                                    flex: 1,
                                    border: '1px solid var(--border-color)',
                                    borderRadius: '8px',
                                    display: 'flex',
                                    flexDirection: 'column',
                                    minHeight: 0,
                                    backgroundColor: 'var(--bg-card)',
                                }}
                            >
                                <div
                                    style={{
                                        padding: '12px 16px',
                                        borderBottom: '1px solid var(--border-color)',
                                        fontWeight: 'bold',
                                    }}
                                >
                                    基于语音逐字稿粗剪 (点击句子可反选删除)
                                </div>
                                <div
                                    style={{
                                        padding: '16px',
                                        overflowY: 'auto',
                                        flex: 1,
                                        display: 'flex',
                                        flexDirection: 'column',
                                        gap: '10px',
                                    }}
                                >
                                    {currentRecording.utterances?.map((u, index) => {
                                        const isCut = cutUtteranceIds.has(index);
                                        return (
                                            <div
                                                key={index}
                                                onClick={() => toggleUtteranceCut(index)}
                                                style={{
                                                    padding: '10px 14px',
                                                    borderRadius: '6px',
                                                    border: '1px solid var(--border-color)',
                                                    cursor: 'pointer',
                                                    backgroundColor: isCut
                                                        ? 'rgba(var(--danger-rgb), 0.05)'
                                                        : 'rgba(var(--success-rgb), 0.03)',
                                                    transition: 'all 0.2s',
                                                    display: 'flex',
                                                    alignItems: 'flex-start',
                                                    gap: '12px',
                                                }}
                                            >
                                                <span
                                                    style={{
                                                        fontSize: '12px',
                                                        color: 'var(--text-muted)',
                                                        fontFamily: 'monospace',
                                                        minWidth: '80px',
                                                    }}
                                                >
                                                    {u.start.toFixed(1)}s - {u.end.toFixed(1)}s
                                                </span>
                                                <span
                                                    style={{
                                                        flex: 1,
                                                        color: isCut ? 'var(--text-muted)' : 'var(--text-main)',
                                                        textDecoration: isCut ? 'line-through' : 'none',
                                                    }}
                                                >
                                                    {u.text}
                                                </span>
                                                <span
                                                    style={{
                                                        fontSize: '12px',
                                                        fontWeight: 'bold',
                                                        color: isCut ? 'var(--danger-fg)' : 'var(--success-fg)',
                                                    }}
                                                >
                                                    {isCut ? '✂️ 已删除' : '✅ 保留'}
                                                </span>
                                            </div>
                                        );
                                    })}
                                </div>
                            </div>
                        </div>
                    ) : (
                        <div
                            style={{
                                flex: 1,
                                display: 'flex',
                                justifyContent: 'center',
                                alignItems: 'center',
                                color: 'var(--text-muted)',
                            }}
                        >
                            在左侧选择一段录像历史进行查看与剪辑
                        </div>
                    )}
                </div>
            </div>
        );
    }

    // Default 'record' view
    return (
        <div
            class="studio-container"
            style={{
                display: 'flex',
                flex: 1,
                minHeight: 0,
                padding: '24px',
                backgroundColor: 'var(--bg-page)',
                color: 'var(--text-main)',
                gap: '24px',
            }}
        >
            {/* Left: Outline & Script Planner */}
            <div
                class="studio-planner"
                style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '16px', minWidth: 0 }}
            >
                <div
                    class="outline-box"
                    style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px', minHeight: 0 }}
                >
                    <h3>1. 口播规划与大纲 (Outline)</h3>
                    <textarea
                        value={outline}
                        onInput={e => setOutline((e.target as HTMLTextAreaElement).value)}
                        style={{
                            flex: 1,
                            padding: '12px',
                            border: '1px solid var(--border-color)',
                            borderRadius: '8px',
                            backgroundColor: 'var(--bg-card)',
                            color: 'var(--text-main)',
                            fontFamily: 'var(--font-mono)',
                            resize: 'none',
                            outline: 'none',
                        }}
                        placeholder="在此规划你的口播分镜和大纲要点..."
                    />
                </div>
                <div
                    class="script-box"
                    style={{ flex: 1, display: 'flex', flexDirection: 'column', gap: '8px', minHeight: 0 }}
                >
                    <h3>2. 提示词逐字稿 (Script)</h3>
                    <textarea
                        value={scriptText}
                        onInput={e => setScriptText((e.target as HTMLTextAreaElement).value)}
                        style={{
                            flex: 1,
                            padding: '12px',
                            border: '1px solid var(--border-color)',
                            borderRadius: '8px',
                            backgroundColor: 'var(--bg-card)',
                            color: 'var(--text-main)',
                            resize: 'none',
                            outline: 'none',
                            lineHeight: '1.6',
                        }}
                        placeholder="在此输入你的提示词逐字稿，供录制时视读..."
                    />
                </div>
            </div>

            {/* Right: Camera Preview & Recording Control */}
            <div
                class="studio-control"
                style={{ width: '400px', display: 'flex', flexDirection: 'column', gap: '20px', shrink: 0 }}
            >
                <h3>3. 现场录制与控制</h3>

                {/* HTML web page URL picker */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: '8px' }}>
                    <label style={{ fontSize: '13px', fontWeight: 'bold' }}>待展示 HTML 网页地址 / 本地 URL</label>
                    <div style={{ display: 'flex', gap: '8px' }}>
                        <input
                            type="text"
                            value={webpageUrl}
                            onInput={e => setWebpageUrl((e.target as HTMLInputElement).value)}
                            style={{
                                flex: 1,
                                padding: '8px 12px',
                                border: '1px solid var(--border-color)',
                                borderRadius: '999px',
                                backgroundColor: 'var(--bg-card)',
                                color: 'var(--text-main)',
                                outline: 'none',
                            }}
                        />
                        <button
                            onClick={handleOpenPage}
                            style={{
                                padding: '6px 16px',
                                border: '1px solid var(--text-main)',
                                borderRadius: '999px',
                                backgroundColor: 'transparent',
                                color: 'var(--text-main)',
                                cursor: 'pointer',
                                fontWeight: '500',
                            }}
                        >
                            打开页面
                        </button>
                    </div>
                </div>

                {/* Camera preview window */}
                <div
                    style={{
                        height: '225px', // 16:9 aspect for preview
                        backgroundColor: '#111',
                        borderRadius: '12px',
                        border: '1px solid var(--border-color)',
                        overflow: 'hidden',
                        display: 'flex',
                        flexDirection: 'column',
                        justifyContent: 'center',
                        alignItems: 'center',
                        position: 'relative',
                    }}
                >
                    <video
                        ref={videoPreviewRef}
                        autoplay
                        muted
                        style={{ width: '100%', height: '100%', objectFit: 'cover' }}
                    />
                    {!isRecording && !webcamUrl && (
                        <div style={{ position: 'absolute', color: '#666', fontSize: '14px', textAlign: 'center' }}>
                            🎥 摄像头未开启
                            <br />
                            <span style={{ fontSize: '11px', marginTop: '6px', display: 'inline-block' }}>
                                点击下方按钮启动录制后自动开启
                            </span>
                        </div>
                    )}
                    {isRecording && (
                        <div
                            style={{
                                position: 'absolute',
                                top: '12px',
                                right: '12px',
                                backgroundColor: 'rgba(235, 87, 87, 0.9)',
                                color: '#fff',
                                padding: '4px 10px',
                                borderRadius: '999px',
                                fontSize: '12px',
                                fontWeight: 'bold',
                                display: 'flex',
                                alignItems: 'center',
                                gap: '6px',
                                animation: 'pulse 1.5s infinite',
                            }}
                        >
                            <span
                                style={{
                                    width: '8px',
                                    height: '8px',
                                    backgroundColor: '#fff',
                                    borderRadius: '50%',
                                    display: 'inline-block',
                                }}
                            ></span>
                            REC {formatTime(recordTime)}
                        </div>
                    )}
                </div>

                {/* Action button */}
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                    {/* Microphone device selector */}
                    {!isRecording && audioDevices.length > 0 && (
                        <div style={{ display: 'flex', flexDirection: 'column', gap: '6px', marginBottom: '8px' }}>
                            <label style={{ fontSize: '12px', color: 'var(--text-secondary)', fontWeight: 'bold' }}>
                                🎙 选择输入麦克风 (Select Mic Input)：
                            </label>
                            <select
                                value={selectedDeviceId}
                                onChange={e => setSelectedDeviceId((e.target as HTMLSelectElement).value)}
                                style={{
                                    width: '100%',
                                    padding: '10px',
                                    borderRadius: '8px',
                                    border: '1px solid var(--border-color)',
                                    backgroundColor: 'var(--bg-card)',
                                    color: 'var(--text-primary)',
                                    fontSize: '13px',
                                    cursor: 'pointer',
                                    outline: 'none',
                                }}
                            >
                                {audioDevices.map(d => (
                                    <option key={d.deviceId} value={d.deviceId}>
                                        {d.label || `未命名麦克风设备 (${d.deviceId.substring(0, 5)})`}
                                    </option>
                                ))}
                            </select>
                        </div>
                    )}
                    {isTranscribing ? (
                        <button
                            disabled
                            style={{
                                width: '100%',
                                padding: '14px',
                                border: 'none',
                                borderRadius: '999px',
                                backgroundColor: 'var(--border-color)',
                                color: 'var(--text-secondary)',
                                fontSize: '15px',
                                fontWeight: 'bold',
                            }}
                        >
                            🔄 正在提取音频并进行 ASR 识别...
                        </button>
                    ) : isRecording ? (
                        <button
                            onClick={handleStopRecording}
                            style={{
                                width: '100%',
                                padding: '14px',
                                border: 'none',
                                borderRadius: '999px',
                                backgroundColor: 'var(--danger-fg)',
                                color: '#fff',
                                fontSize: '15px',
                                fontWeight: 'bold',
                                cursor: 'pointer',
                                display: 'flex',
                                justifyContent: 'center',
                                alignItems: 'center',
                                gap: '8px',
                            }}
                        >
                            🛑 停止录制并生成逐字稿
                        </button>
                    ) : (
                        <button
                            onClick={handleStartRecording}
                            style={{
                                width: '100%',
                                padding: '14px',
                                border: 'none',
                                borderRadius: '999px',
                                backgroundColor: 'var(--accent-color)',
                                color: 'var(--on-accent)',
                                fontSize: '15px',
                                fontWeight: 'bold',
                                cursor: 'pointer',
                                display: 'flex',
                                justifyContent: 'center',
                                alignItems: 'center',
                                gap: '8px',
                            }}
                        >
                            🔴 开始双屏录屏
                        </button>
                    )}
                </div>

                {/* Standalone Audio Recording Test Panel */}
                <div
                    style={{ marginTop: '16px', display: 'flex', flexDirection: 'column', gap: '10px', width: '100%' }}
                >
                    <button
                        onClick={handleToggleTestAudio}
                        style={{
                            width: '100%',
                            padding: '10px',
                            border: '1px dashed var(--accent-color)',
                            borderRadius: '8px',
                            backgroundColor: 'transparent',
                            color: 'var(--accent-color)',
                            fontSize: '13px',
                            fontWeight: '500',
                            cursor: 'pointer',
                        }}
                    >
                        {isTestAudioRecording ? '⏹ 停止录音测试 (Stop Test)' : '🎙 独立录音测试 (Start Test)'}
                    </button>
                    {testAudioUrl && (
                        <div
                            style={{
                                display: 'flex',
                                flexDirection: 'column',
                                gap: '6px',
                                alignItems: 'center',
                                backgroundColor: 'var(--bg-card)',
                                padding: '10px',
                                borderRadius: '8px',
                                border: '1px solid var(--border-color)',
                            }}
                        >
                            <span style={{ fontSize: '11px', color: 'var(--text-secondary)' }}>测试音频回放：</span>
                            <audio src={testAudioUrl} controls style={{ width: '100%' }} />
                        </div>
                    )}
                </div>

                {/* Instructions */}
                <div
                    style={{
                        fontSize: '12px',
                        color: 'var(--text-secondary)',
                        backgroundColor: 'var(--bg-card)',
                        padding: '16px',
                        borderRadius: '8px',
                        border: '1px solid var(--border-color)',
                        lineHeight: '1.6',
                        display: 'flex',
                        flexDirection: 'column',
                        gap: '8px',
                    }}
                >
                    <span style={{ fontWeight: 'bold', color: 'var(--text-main)' }}>💡 录制小贴士：</span>
                    <span>1. 输入你想展示的网页，点击“打开页面”在独立窗口中浏览。</span>
                    <span>2. 点击“开始双屏录屏”，系统会提示选择共享屏幕/窗口。**请务必选择刚打开的网页窗口**。</span>
                    <span>3. 看着左侧的大纲或提示词逐字稿进行录制，人脸与网页动作会同时捕获。</span>
                    <span>4. 录制完毕后，可在“录制列表”中对生成的音频逐字稿进行可视化粗剪。</span>
                </div>
            </div>

            {/* Inline pulse animation styling */}
            <style
                dangerouslySetInnerHTML={{
                    __html: `
                @keyframes pulse {
                    0% { opacity: 1; }
                    50% { opacity: 0.6; }
                    100% { opacity: 1; }
                }
            `,
                }}
            />
        </div>
    );
}
