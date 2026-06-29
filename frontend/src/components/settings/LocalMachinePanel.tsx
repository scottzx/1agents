/**
 * LocalMachinePanel — 本机 relay daemon 管理面板。
 *
 * 只在本机模式下渲染（hostname === localhost / 127.0.0.1）。
 * 展示 happy daemon 状态、启停按钮，以及 machine key / token，
 * 供客户端（H5、小程序、App）配置时复制使用。
 */
import { h, Fragment } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import QRCode from 'qrcode';

interface HappyStatus {
    running: boolean;
    pid?: number;
    startedAt?: string;
    serverUrl?: string;
    token?: string;
    machineKey?: string;
    publicKey?: string;
    machineId?: string;
    hostname?: string;
}

/**
 * 把本机凭据编码成二维码,供客户端(H5 / 小程序 / App)扫码导入,免去手动复制多个字段。
 * 二维码内容 = 面板上展示的同一份凭据(JSON),不含任何额外信息。
 * 注意:这是机密 —— 黑白固定渲染保证可扫,默认隐藏、点按才显示。
 */
function buildCredentialPayload(s: HappyStatus): string {
    const payload: Record<string, string> = { v: '1', type: '1agents-relay' };
    if (s.hostname) payload.hostname = s.hostname;
    if (s.serverUrl) payload.serverUrl = s.serverUrl;
    if (s.token) payload.token = s.token;
    if (s.machineId) payload.machineId = s.machineId;
    if (s.machineKey) payload.machineKey = s.machineKey;
    // 客户端配对不需要 publicKey(parseDeviceBundle 不读),省略以减小二维码体积、提升可扫性。
    return JSON.stringify(payload);
}

function CredentialQr({ payload }: { payload: string }) {
    const canvasRef = useRef<HTMLCanvasElement | null>(null);
    const err = useSignal('');
    useEffect(() => {
        const canvas = canvasRef.current;
        if (!canvas) return;
        // 黑白固定(不随主题切换),否则深色模式下对比度不足、扫不出来。
        QRCode.toCanvas(canvas, payload, {
            // 较大尺寸 + 低纠错级(L):bundle 数据较多,模块多,放大并减少冗余
            // 让单个模块更大,手机隔屏扫码更容易成功。
            width: 300,
            margin: 2,
            errorCorrectionLevel: 'L',
            color: { dark: '#000000', light: '#ffffff' },
        }).catch((e: unknown) => (err.value = (e as Error)?.message ?? String(e)));
    }, [payload]);
    return (
        <div style="display:flex;flex-direction:column;align-items:center;gap:6px;margin-top:10px">
            <div style="padding:10px;background:#ffffff;border-radius:8px;border:1px solid var(--border-color)">
                <canvas ref={canvasRef} style="display:block" />
            </div>
            {err.value && <span style="font-size:11px;color:var(--danger-fg)">二维码生成失败: {err.value}</span>}
        </div>
    );
}

function copyToClipboard(text: string, onDone: () => void) {
    if (navigator.clipboard?.writeText) {
        navigator.clipboard.writeText(text).then(onDone).catch(onDone);
    } else {
        onDone();
    }
}

function CopyButton({ value, label }: { value: string; label?: string }) {
    const copied = useSignal(false);
    const handle = () => {
        copyToClipboard(value, () => {
            copied.value = true;
            setTimeout(() => (copied.value = false), 1500);
        });
    };
    return (
        <button class="sys-settings-btn ghost" style="height:28px;padding:0 10px;font-size:11.5px;" onClick={handle}>
            {copied.value ? (
                <Fragment>
                    <svg
                        width="12"
                        height="12"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        style="margin-right:4px"
                    >
                        <polyline points="20 6 9 17 4 12" />
                    </svg>
                    {label ? `已复制 ${label}` : '已复制'}
                </Fragment>
            ) : (
                <Fragment>
                    <svg
                        width="12"
                        height="12"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        style="margin-right:4px"
                    >
                        <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                    </svg>
                    {label ? `复制 ${label}` : '复制'}
                </Fragment>
            )}
        </button>
    );
}

function MonoRow({ label, value, redact = false }: { label: string; value: string; redact?: boolean }) {
    const revealed = useSignal(!redact);
    const display = revealed.value ? value : value.slice(0, 8) + '…' + value.slice(-4);
    return (
        <div style="display:flex;align-items:center;gap:8px;margin-top:8px;flex-wrap:wrap">
            <span style="font-size:12px;color:var(--text-muted);min-width:80px;flex-shrink:0">{label}</span>
            <code style="font-family:var(--font-mono);font-size:11.5px;color:var(--text-secondary);flex:1;word-break:break-all;background:var(--bg-panel);border-radius:4px;padding:3px 6px;border:1px solid var(--border-color);">
                {display}
            </code>
            {redact && (
                <button
                    class="sys-settings-btn ghost"
                    style="height:28px;padding:0 10px;font-size:11.5px;"
                    onClick={() => (revealed.value = !revealed.value)}
                >
                    {revealed.value ? '隐藏' : '显示'}
                </button>
            )}
            <CopyButton value={value} />
        </div>
    );
}

export function LocalMachinePanel() {
    const status = useSignal<HappyStatus | null>(null);
    const error = useSignal('');
    const busy = useSignal(false);
    const timerRef = useRef<ReturnType<typeof setInterval> | null>(null);

    const fetchStatus = async () => {
        try {
            const res = await fetch('/api/system/happy/status');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            status.value = await res.json();
            error.value = '';
        } catch (e) {
            error.value = (e as Error).message;
        }
    };

    useEffect(() => {
        fetchStatus();
        timerRef.current = setInterval(fetchStatus, 4000);
        return () => {
            if (timerRef.current) clearInterval(timerRef.current);
        };
    }, []);

    const doStart = async () => {
        busy.value = true;
        try {
            const res = await fetch('/api/system/happy/daemon/start', { method: 'POST' });
            if (!res.ok) {
                const j = await res.json().catch(() => ({}));
                error.value = j.error ?? `启动失败 HTTP ${res.status}`;
            } else {
                // give daemon a moment to write its state file
                setTimeout(fetchStatus, 1200);
            }
        } catch (e) {
            error.value = (e as Error).message;
        } finally {
            busy.value = false;
        }
    };

    const doStop = async () => {
        busy.value = true;
        try {
            const res = await fetch('/api/system/happy/daemon/stop', { method: 'POST' });
            if (!res.ok) {
                const j = await res.json().catch(() => ({}));
                error.value = j.error ?? `停止失败 HTTP ${res.status}`;
            } else {
                setTimeout(fetchStatus, 800);
            }
        } catch (e) {
            error.value = (e as Error).message;
        } finally {
            busy.value = false;
        }
    };

    const s = status.value;
    const hasCredentials = !!(s?.token || s?.machineKey);
    const showQr = useSignal(false);

    return (
        <div class="sys-settings-card">
            <div class="sys-settings-card-header">
                <div class="sys-settings-card-icon">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
                        <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
                        <line x1="6" y1="6" x2="6.01" y2="6" />
                        <line x1="6" y1="18" x2="6.01" y2="18" />
                    </svg>
                </div>
                <div style="flex:1">
                    <div class="sys-settings-card-title">本机 Relay (Machine)</div>
                    <div class="sys-settings-card-subtitle">
                        开启后，H5 / 小程序 / App 可通过 happy 中转连接到这台机器
                    </div>
                </div>
                {/* 状态指示灯 */}
                <div style="display:flex;align-items:center;gap:6px;flex-shrink:0">
                    <span
                        style={`display:inline-block;width:8px;height:8px;border-radius:50%;background:${s?.running ? 'var(--success-emphasis)' : 'var(--text-muted)'};${s?.running ? 'box-shadow:0 0 0 2px rgba(var(--success-rgb),0.25)' : ''}`}
                    />
                    <span style="font-size:12px;color:var(--text-muted)">
                        {s === null ? '检测中…' : s.running ? '运行中' : '未运行'}
                    </span>
                </div>
            </div>

            {/* 操作按钮 */}
            <div class="sys-settings-action-row" style="margin-top:12px">
                {s?.running ? (
                    <button class="sys-settings-btn danger" disabled={busy.value} onClick={doStop}>
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <rect x="3" y="3" width="18" height="18" rx="2" ry="2" />
                        </svg>
                        停止 Daemon
                    </button>
                ) : (
                    <button class="sys-settings-btn primary" disabled={busy.value || s === null} onClick={doStart}>
                        <svg
                            width="14"
                            height="14"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <polygon points="5 3 19 12 5 21 5 3" />
                        </svg>
                        启动 Daemon
                    </button>
                )}
                <button
                    class="sys-settings-btn ghost"
                    disabled={busy.value}
                    onClick={fetchStatus}
                    style="margin-left:6px"
                >
                    <svg
                        width="14"
                        height="14"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <polyline points="23 4 23 10 17 10" />
                        <polyline points="1 20 1 14 7 14" />
                        <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15" />
                    </svg>
                    刷新
                </button>
            </div>

            {error.value && <div style="margin-top:8px;font-size:12px;color:var(--danger-fg)">{error.value}</div>}

            {/* 凭据展示 — 只在有数据时显示 */}
            {hasCredentials && (
                <div style="margin-top:16px;border-top:1px solid var(--border-color);padding-top:14px">
                    <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:4px">
                        客户端配置凭据
                    </div>
                    <div style="font-size:11.5px;color:var(--text-muted);margin-bottom:8px">
                        将以下信息填入 H5 / 小程序 / App 的中转配置，使其能连接到本机
                    </div>

                    {s?.hostname && <MonoRow label="设备名称" value={s.hostname} />}
                    {/* 中转地址不在面板明文展示 —— 避免暴露 relay 域名招致 DoS。
                        仍随二维码下发(buildCredentialPayload),客户端扫码即可连接。 */}
                    {s?.machineId && <MonoRow label="Machine ID" value={s.machineId} />}
                    {s?.token && <MonoRow label="Token" value={s.token} redact />}
                    {s?.machineKey && <MonoRow label="Machine Key" value={s.machineKey} redact />}
                    {s?.publicKey && <MonoRow label="Public Key" value={s.publicKey} redact />}

                    {/* 扫码导入 — 把以上凭据打包成二维码,客户端扫码即可,免去逐个复制 */}
                    <div style="margin-top:12px">
                        <button
                            class="sys-settings-btn ghost"
                            style="height:28px;padding:0 12px;font-size:11.5px"
                            onClick={() => (showQr.value = !showQr.value)}
                        >
                            <svg
                                width="13"
                                height="13"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                style="margin-right:5px"
                            >
                                <rect x="3" y="3" width="7" height="7" rx="1" />
                                <rect x="14" y="3" width="7" height="7" rx="1" />
                                <rect x="3" y="14" width="7" height="7" rx="1" />
                                <line x1="14" y1="14" x2="14" y2="17" />
                                <line x1="17" y1="14" x2="21" y2="14" />
                                <line x1="21" y1="17" x2="21" y2="21" />
                                <line x1="14" y1="21" x2="17" y2="21" />
                            </svg>
                            {showQr.value ? '隐藏二维码' : '显示配置二维码'}
                        </button>
                        {showQr.value && s && (
                            <Fragment>
                                <CredentialQr payload={buildCredentialPayload(s)} />
                                <div style="text-align:center;font-size:11px;color:var(--text-muted);margin-top:4px">
                                    含敏感凭据,请勿截图外传
                                </div>
                            </Fragment>
                        )}
                    </div>

                    {s?.running && s?.pid && (
                        <div style="margin-top:10px;font-size:11px;color:var(--text-muted)">
                            PID {s.pid}
                            {s.startedAt ? `，启动于 ${new Date(s.startedAt).toLocaleTimeString()}` : ''}
                        </div>
                    )}
                </div>
            )}

            {/* 未登录提示 */}
            {s !== null && !hasCredentials && (
                <div style="margin-top:12px;font-size:12px;color:var(--text-muted)">
                    未检测到 happy 凭据。请先在终端运行{' '}
                    <code style="font-family:var(--font-mono);background:var(--bg-panel);padding:1px 5px;border-radius:3px">
                        happy auth login
                    </code>{' '}
                    完成登录后再启动 Daemon。
                </div>
            )}
        </div>
    );
}

/** 当前访问来源是否为本机（localhost / 127.0.0.1）。 */
export function isLocalMachineMode(): boolean {
    const h = window.location.hostname;
    return h === 'localhost' || h === '127.0.0.1';
}
