/**
 * 中转旁路(Relay) 配置 + 扫码配对面板。
 *
 * 模型:本地直连 Agent 一直在;中转是"旁路"——让本端作为持密钥的 user-scoped 客户端,
 * 经中转去寻址/转发到某台机器节点(其 daemon 暴露 1agents-proxy → 本地 Go 后端)。
 *
 * 能力:配置中转地址 / 创建账户 / 扫码或粘贴链接审批节点配对 / 列出节点 / 经中转测试调用。
 */
import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { useRef, useEffect } from 'preact/hooks';
import type { Socket } from 'socket.io-client';
import jsQR from 'jsqr';
import {
    createAccount,
    approveTerminal,
    listMachines,
    connect,
    proxyApi,
    loadCredentialsRemote,
    clearCredentials,
    type RelayCredentials,
    type RelayMachine,
} from '../../services/relay/relayClient';
import { setRelayNode, normalizeOrigin } from '../../services/apiClient';

const LS_URL = 'oneagents.relay.url';

// 从扫到/粘贴的内容里提取配对 key(支持 happy://terminal?<key> 与 .../terminal/connect#key=<key>)
function extractKey(raw: string): string | null {
    const s = raw.trim();
    let m = s.match(/[#?]key=([A-Za-z0-9_-]+)/);
    if (m) return m[1];
    m = s.match(/happy:\/\/terminal\?([A-Za-z0-9_-]+)/);
    if (m) return m[1];
    if (/^[A-Za-z0-9_-]{20,}$/.test(s)) return s; // 直接就是 key
    return null;
}

export function RelayPairingPanel({
    onNodeSelected,
    embedded = false,
}: { onNodeSelected?: () => void; embedded?: boolean } = {}) {
    // 默认用当前页面同源(部署中前端与中转在同一 HTTPS 源 → /v1 经代理路由到中转);
    // 可手动改成独立的中转地址。
    const relayUrl = useSignal(localStorage.getItem(LS_URL) || window.location.origin);
    // 凭据不再从 localStorage 同步初始化(issue #112):mount 时从后端拉取(见下方 effect),
    // 这样清空 localStorage / 换设备后仍能恢复,localStorage 仅作后端命中后的读缓存。
    const creds = useSignal<RelayCredentials | null>(null);
    const pairInput = useSignal('');
    const machines = useSignal<RelayMachine[]>([]);
    const msg = useSignal('');
    const busy = useSignal(false);
    const scanning = useSignal(false);
    const connected = useSignal(false);
    const videoRef = useRef<HTMLVideoElement | null>(null);
    const rafRef = useRef<number | null>(null);
    const socketRef = useRef<Socket | null>(null);

    // 确保有一个长连接(自动连接的核心):有凭据就连上,复用同一 socket。
    const ensureSocket = async (): Promise<Socket> => {
        if (socketRef.current?.connected) return socketRef.current;
        if (!creds.value) throw new Error('请先创建账户');
        socketRef.current = await connect(relayUrl.value, creds.value);
        connected.value = true;
        return socketRef.current;
    };

    // 进入页面即自动连接 + 拉取节点(“有地址+凭据就自动连上,无需手动”)。
    // 凭据优先从后端读取(issue #109):清掉 localStorage / 换设备后仍能恢复。
    useEffect(() => {
        (async () => {
            try {
                const persisted = await loadCredentialsRemote();
                if (persisted) creds.value = persisted;
                if (!creds.value) return;
                await ensureSocket();
                machines.value = await listMachines(relayUrl.value, creds.value);
                msg.value = `✅ 已自动连接中转,节点 ${machines.value.length} 台`;
            } catch (e) {
                msg.value = '⚠️ 自动连接失败: ' + ((e as Error)?.message ?? String(e));
            }
        })();
    }, []);

    const setUrl = (v: string) => {
        relayUrl.value = normalizeOrigin(v);
        localStorage.setItem(LS_URL, relayUrl.value);
    };
    const run = async (label: string, fn: () => Promise<void>) => {
        busy.value = true;
        msg.value = label + '…';
        try {
            await fn();
        } catch (e) {
            msg.value = '❌ ' + ((e as Error)?.message ?? String(e));
        } finally {
            busy.value = false;
        }
    };

    const doCreateAccount = () =>
        run('创建账户', async () => {
            creds.value = await createAccount(relayUrl.value);
            msg.value = '✅ 账户已创建';
        });

    // 断开账户:清后端 + localStorage 凭据(issue #112),并关闭长连接、清空节点列表。
    const doDisconnect = () =>
        run('断开账户', async () => {
            await clearCredentials();
            socketRef.current?.close();
            socketRef.current = null;
            connected.value = false;
            creds.value = null;
            machines.value = [];
            msg.value = '✅ 已断开账户并清除凭据';
        });

    const doPair = (key: string) =>
        run('审批配对', async () => {
            if (!creds.value) throw new Error('请先创建账户');
            const k = extractKey(key);
            if (!k) throw new Error('未识别到配对 key');
            await approveTerminal(relayUrl.value, creds.value, k);
            msg.value = '✅ 已审批配对,请稍后刷新节点列表';
            stopScan();
        });

    const doRefresh = () =>
        run('刷新节点', async () => {
            if (!creds.value) throw new Error('请先创建账户');
            machines.value = await listMachines(relayUrl.value, creds.value);
            msg.value = `✅ 节点 ${machines.value.length} 台`;
        });

    const doTest = (m: RelayMachine) =>
        run('经中转测试', async () => {
            const socket = await ensureSocket();
            const resp = await proxyApi(socket, m, '/api/agent/agent-types');
            msg.value = `✅ [经中转] ${m.id.slice(0, 8)} → Go后端 ${resp.status}: ${(resp.body ?? resp.error ?? '').slice(0, 160)}`;
        });

    // 把某节点设为当前后端并进入主界面(供门禁页使用)。
    const doUseNode = (m: RelayMachine) =>
        run('设为后端', async () => {
            const socket = await ensureSocket();
            setRelayNode(socket, m);
            msg.value = `✅ 已选用节点 ${m.id.slice(0, 8)} 作为后端`;
            onNodeSelected?.();
        });

    // 直连对比:直接打当前源的 /api(不经中转)。
    // 若本页由"中转/CDN 源"托管(无后端直连路径),这里会失败 —— 正好证明中转的作用。
    const doDirectTest = () =>
        run('直连测试(无中转)', async () => {
            const url = `${window.location.origin}/api/agent/agent-types`;
            const resp = await fetch(url);
            const text = (await resp.text()).slice(0, 160);
            msg.value = `${resp.ok ? '✅' : '❌'} [直连] ${url} → ${resp.status}: ${text}`;
        });

    const stopScan = () => {
        scanning.value = false;
        if (rafRef.current) cancelAnimationFrame(rafRef.current);
        const v = videoRef.current;
        const stream = v?.srcObject as MediaStream | null;
        stream?.getTracks().forEach(t => t.stop());
        if (v) v.srcObject = null;
    };

    const startScan = async () => {
        if (!navigator.mediaDevices?.getUserMedia) {
            msg.value = '❌ 当前环境无法访问摄像头(需 HTTPS/安全上下文)';
            return;
        }
        try {
            scanning.value = true;
            const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'environment' } });
            const v = videoRef.current!;
            v.srcObject = stream;
            await v.play();
            const canvas = document.createElement('canvas');
            const ctx = canvas.getContext('2d')!;
            const tick = () => {
                if (!scanning.value) return;
                if (v.readyState === v.HAVE_ENOUGH_DATA) {
                    canvas.width = v.videoWidth;
                    canvas.height = v.videoHeight;
                    ctx.drawImage(v, 0, 0, canvas.width, canvas.height);
                    const img = ctx.getImageData(0, 0, canvas.width, canvas.height);
                    const code = jsQR(img.data, img.width, img.height);
                    if (code?.data) {
                        pairInput.value = code.data;
                        doPair(code.data);
                        return;
                    }
                }
                rafRef.current = requestAnimationFrame(tick);
            };
            rafRef.current = requestAnimationFrame(tick);
        } catch (e) {
            scanning.value = false;
            msg.value = '❌ 摄像头打开失败: ' + ((e as Error)?.message ?? String(e));
        }
    };

    const body = (
        <Fragment>
            {/* 配置 */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-title">中转地址</div>
                    <div class="sys-settings-card-subtitle">Happy 中转服务器 URL</div>
                </div>
                <input
                    class="sys-settings-input"
                    style="width:100%;padding:8px;margin-top:8px"
                    value={relayUrl.value}
                    onInput={e => setUrl((e.target as HTMLInputElement).value)}
                    placeholder="http://10.100.158.93:3005"
                />
                <div style="margin-top:10px;display:flex;gap:8px;align-items:center">
                    <button class="sys-settings-option-btn" disabled={busy.value} onClick={doCreateAccount}>
                        {creds.value ? '重新创建账户' : '创建账户'}
                    </button>
                    {creds.value && (
                        <button class="sys-settings-option-btn" disabled={busy.value} onClick={doDisconnect}>
                            断开账户
                        </button>
                    )}
                    <span style="font-size:12px;color:var(--text-muted)">
                        {creds.value ? '账户: ' + creds.value.token.slice(0, 18) + '…' : '未创建账户'}
                    </span>
                </div>
            </div>

            {/* 扫码配对 */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-title">扫码配对节点</div>
                    <div class="sys-settings-card-subtitle">
                        在节点上运行 <code>happy auth login</code> 生成配对二维码/链接,这里扫码或粘贴以绑定该节点。
                    </div>
                </div>
                <div style="display:flex;gap:8px;margin-top:8px">
                    <input
                        class="sys-settings-input"
                        style="flex:1;padding:8px"
                        value={pairInput.value}
                        onInput={e => (pairInput.value = (e.target as HTMLInputElement).value)}
                        placeholder="粘贴 happy://terminal?... 或 .../connect#key=... 或 key"
                    />
                    <button
                        class="sys-settings-option-btn"
                        disabled={busy.value}
                        onClick={() => doPair(pairInput.value)}
                    >
                        配对
                    </button>
                </div>
                <div style="margin-top:8px">
                    {!scanning.value ? (
                        <button class="sys-settings-option-btn" onClick={startScan}>
                            📷 扫码
                        </button>
                    ) : (
                        <button class="sys-settings-option-btn active" onClick={stopScan}>
                            停止扫码
                        </button>
                    )}
                </div>
                <video
                    ref={videoRef}
                    playsInline
                    muted
                    style={`width:100%;max-width:320px;margin-top:8px;border-radius:8px;${scanning.value ? '' : 'display:none'}`}
                />
            </div>

            {/* 节点列表 */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-title">已配对节点</div>
                    <div class="sys-settings-card-subtitle">经中转可达的机器</div>
                </div>
                <div style="margin-top:8px;display:flex;gap:8px;flex-wrap:wrap">
                    <button class="sys-settings-option-btn" disabled={busy.value} onClick={doRefresh}>
                        刷新
                    </button>
                    <button class="sys-settings-option-btn" disabled={busy.value} onClick={doDirectTest}>
                        直连测试(无中转)
                    </button>
                </div>
                <div style="margin-top:8px;display:flex;flex-direction:column;gap:6px">
                    {machines.value.map(m => (
                        <div
                            key={m.id}
                            style="display:flex;gap:8px;align-items:center;justify-content:space-between;font-size:13px"
                        >
                            <span>
                                <span
                                    style={`display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:6px;background:${m.active ? 'var(--success-emphasis)' : 'var(--text-muted)'}`}
                                />
                                {m.id.slice(0, 12)}… <span style="color:var(--text-muted)">{m.variant}</span>
                            </span>
                            <span style="display:flex;gap:6px">
                                <button class="sys-settings-option-btn" disabled={busy.value} onClick={() => doTest(m)}>
                                    经中转测试
                                </button>
                                <button
                                    class="sys-settings-option-btn"
                                    disabled={busy.value}
                                    onClick={() => doUseNode(m)}
                                >
                                    设为后端并进入
                                </button>
                            </span>
                        </div>
                    ))}
                    {machines.value.length === 0 && (
                        <span style="font-size:12px;color:var(--text-muted)">暂无,先配对并刷新</span>
                    )}
                </div>
            </div>

            {msg.value && (
                <div class="sys-settings-card" style="font-size:13px;word-break:break-all">
                    {msg.value}
                </div>
            )}
        </Fragment>
    );

    // Embedded inside the 远程控制 settings section (parent provides the section
    // chrome) — just return the cards. Standalone (backend gate) wraps them in
    // its own titled section.
    if (embedded) return body;
    return (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">远程控制</div>
            <div class="sys-settings-section-desc">本地直连始终保持;远程控制让你经中转安全地访问并操作远端节点。</div>
            {body}
        </div>
    );
}
