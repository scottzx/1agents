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

/**
 * 账户级配对(Model A)。点「生成账户配对码」让本机后台向中转登记一个临时公钥
 * 并产出 happy://terminal?<key> 二维码;客户端(已登录账号)扫码批准后,本机
 * 即绑定到该账号,daemon 自动以新账号重连。取代旧「设备档案」(机器借自身账号)
 * 与「终端跑 happy auth login」两种方式。
 */
function AccountPairing({ onPaired }: { onPaired: () => void }) {
    const pairUrl = useSignal('');
    const pairStatus = useSignal<'idle' | 'pending' | 'authorized' | 'error'>('idle');
    const pairError = useSignal('');
    const busy = useSignal(false);
    const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

    const stopPoll = () => {
        if (pollRef.current) {
            clearInterval(pollRef.current);
            pollRef.current = null;
        }
    };
    useEffect(() => stopPoll, []); // 卸载时清理轮询

    const poll = async () => {
        try {
            const res = await fetch('/api/system/happy/pair/status');
            if (!res.ok) return;
            const j = await res.json();
            pairStatus.value = j.status;
            if (j.status === 'authorized') {
                stopPoll();
                onPaired();
            } else if (j.status === 'error') {
                stopPoll();
                pairError.value = j.error ?? '配对失败';
            }
        } catch {
            /* 轮询瞬时失败,下次再试 */
        }
    };

    const start = async () => {
        busy.value = true;
        pairError.value = '';
        try {
            const res = await fetch('/api/system/happy/pair/start', { method: 'POST' });
            const j = await res.json().catch(() => ({}));
            if (!res.ok) {
                pairError.value = j.error ?? `启动配对失败 HTTP ${res.status}`;
                pairStatus.value = 'error';
                return;
            }
            pairUrl.value = j.pairingUrl ?? '';
            pairStatus.value = 'pending';
            stopPoll();
            pollRef.current = setInterval(poll, 2000);
        } catch (e) {
            pairError.value = (e as Error).message;
            pairStatus.value = 'error';
        } finally {
            busy.value = false;
        }
    };

    return (
        <div style="margin-top:16px;border-top:1px solid var(--border-color);padding-top:14px">
            <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:4px">
                绑定到账号(账户级配对)
            </div>
            <div style="font-size:11.5px;color:var(--text-muted);margin-bottom:8px">
                生成配对码,用客户端(已登录账号)扫码批准,即可把本机绑定到你的账号 —— 订阅随账号生效。
            </div>
            <button
                class="sys-settings-btn primary"
                style="height:30px;padding:0 12px;font-size:12px"
                disabled={busy.value || pairStatus.value === 'pending'}
                onClick={start}
            >
                {pairStatus.value === 'pending' ? '等待客户端审批…' : '生成账户配对码'}
            </button>
            {pairStatus.value === 'pending' && pairUrl.value && (
                <Fragment>
                    <CredentialQr payload={pairUrl.value} />
                    <div style="text-align:center;font-size:11px;color:var(--text-muted);margin-top:4px">
                        用已登录账号的客户端扫码批准
                    </div>
                </Fragment>
            )}
            {pairStatus.value === 'authorized' && (
                <div style="margin-top:10px;font-size:12px;color:var(--success-fg)">
                    ✓ 已绑定到账号,daemon 正在以新账号重连…
                </div>
            )}
            {pairError.value && (
                <div style="margin-top:8px;font-size:12px;color:var(--danger-fg)">{pairError.value}</div>
            )}
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

            {/* 本机凭据(只读查看)。客户端接入统一走下方「账户级配对」(Model A);
                旧「设备档案」扫码流(Model B)已下线,这里仅供排查/查看本机身份。 */}
            {hasCredentials && (
                <div style="margin-top:16px;border-top:1px solid var(--border-color);padding-top:14px">
                    <div style="font-size:12px;font-weight:600;color:var(--text-secondary);margin-bottom:4px">
                        本机凭据(只读)
                    </div>
                    <div style="font-size:11.5px;color:var(--text-muted);margin-bottom:8px">
                        本机 relay 身份,供排查查看;客户端接入请用下方「生成账户配对码」。
                    </div>

                    {s?.hostname && <MonoRow label="设备名称" value={s.hostname} />}
                    {/* 中转地址不在面板明文展示 —— 避免暴露 relay 域名招致 DoS。 */}
                    {s?.machineId && <MonoRow label="Machine ID" value={s.machineId} />}
                    {s?.token && <MonoRow label="Token" value={s.token} redact />}
                    {s?.machineKey && <MonoRow label="Machine Key" value={s.machineKey} redact />}
                    {s?.publicKey && <MonoRow label="Public Key" value={s.publicKey} redact />}

                    {s?.running && s?.pid && (
                        <div style="margin-top:10px;font-size:11px;color:var(--text-muted)">
                            PID {s.pid}
                            {s.startedAt ? `，启动于 ${new Date(s.startedAt).toLocaleTimeString()}` : ''}
                        </div>
                    )}
                </div>
            )}

            {/* 账户级配对(Model A)—— 绑定本机到账号,取代终端 happy auth login */}
            <AccountPairing onPaired={fetchStatus} />

            {/* 未登录提示 */}
            {s !== null && !hasCredentials && (
                <div style="margin-top:12px;font-size:12px;color:var(--text-muted)">
                    未检测到 happy 凭据。点上方「生成账户配对码」用客户端扫码绑定本机到你的账号,即可启动 Daemon。
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
