/**
 * 客户端「设备」面板 —— 扫码添加 / 切换 / 重命名本端绑定的远程机器。
 *
 * 设备 = 机器端「配置二维码」(LocalMachinePanel)里的凭据 bundle。扫一台存一条,
 * 可同时存多台、随时切换当前后端(switchToDevice)。重命名只改本地展示名。
 *
 * 两处复用:
 *  - 门禁页(还没有任何设备 → 直接进扫码页,即「最早的匹配页面」)
 *  - 设置页(已有设备 → 增/切/改名/删)
 */
import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { useRef } from 'preact/hooks';
import jsQR from 'jsqr';
import {
    loadDevices,
    parseDeviceBundle,
    upsertDevice,
    renameDevice,
    removeDevice,
    getActiveDeviceId,
    type DeviceProfile,
} from '@1agents/core/services/relay/devices';
import { switchToDevice } from '../../services/apiClient';

export function RelayDevicePanel({
    onConnected,
    embedded = false,
}: { onConnected?: () => void; embedded?: boolean } = {}) {
    const devices = useSignal<DeviceProfile[]>(loadDevices());
    const activeId = useSignal<string | null>(getActiveDeviceId());
    const pasteInput = useSignal('');
    const msg = useSignal('');
    const busy = useSignal(false);
    const scanning = useSignal(false);
    const editingId = useSignal<string | null>(null);
    const editDraft = useSignal('');
    const videoRef = useRef<HTMLVideoElement | null>(null);
    const rafRef = useRef<number | null>(null);

    const refresh = () => {
        devices.value = loadDevices();
        activeId.value = getActiveDeviceId();
    };

    // 添加一条 bundle:解析 → 落库 → 立即设为当前并连接 → 通知外层(门禁页据此进入)。
    const addBundle = async (raw: string) => {
        const profile = parseDeviceBundle(raw);
        if (!profile) {
            msg.value = '❌ 无法识别的二维码/内容,请扫机器端「配置二维码」';
            return;
        }
        busy.value = true;
        msg.value = '连接设备…';
        try {
            const saved = upsertDevice(profile);
            await switchToDevice(saved.machineId);
            refresh();
            msg.value = `✅ 已连接 ${saved.name}`;
            stopScan();
            onConnected?.();
        } catch (e) {
            msg.value = '❌ 连接失败: ' + ((e as Error)?.message ?? String(e));
        } finally {
            busy.value = false;
        }
    };

    const doSwitch = async (d: DeviceProfile) => {
        busy.value = true;
        msg.value = `切换到 ${d.name}…`;
        try {
            await switchToDevice(d.machineId);
            refresh();
            msg.value = `✅ 当前后端: ${d.name}`;
            onConnected?.();
        } catch (e) {
            msg.value = '❌ 切换失败: ' + ((e as Error)?.message ?? String(e));
        } finally {
            busy.value = false;
        }
    };

    const doDelete = (d: DeviceProfile) => {
        removeDevice(d.machineId);
        refresh();
        msg.value = `已删除 ${d.name}`;
    };

    const startEdit = (d: DeviceProfile) => {
        editingId.value = d.machineId;
        editDraft.value = d.name;
    };
    const saveEdit = (d: DeviceProfile) => {
        renameDevice(d.machineId, editDraft.value);
        editingId.value = null;
        refresh();
    };

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
            msg.value = '❌ 当前环境无法访问摄像头(需 HTTPS / 安全上下文)';
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
                        void addBundle(code.data);
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
            {/* 扫码 / 粘贴添加 */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-title">添加设备</div>
                    <div class="sys-settings-card-subtitle">
                        扫机器端「本机 Relay」里的<strong>配置二维码</strong>,或粘贴其内容
                    </div>
                </div>
                <div style="margin-top:10px;display:flex;gap:8px">
                    {!scanning.value ? (
                        <button class="sys-settings-option-btn" disabled={busy.value} onClick={startScan}>
                            📷 扫码添加
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
                <div style="display:flex;gap:8px;margin-top:8px">
                    <input
                        class="sys-settings-input"
                        style="flex:1;padding:8px"
                        value={pasteInput.value}
                        onInput={e => (pasteInput.value = (e.target as HTMLInputElement).value)}
                        placeholder='粘贴二维码内容 {"type":"1agents-relay",...}'
                    />
                    <button
                        class="sys-settings-option-btn"
                        disabled={busy.value || !pasteInput.value.trim()}
                        onClick={() => addBundle(pasteInput.value)}
                    >
                        添加
                    </button>
                </div>
            </div>

            {/* 设备列表 / 切换 / 改名 / 删除 */}
            <div class="sys-settings-card">
                <div class="sys-settings-card-header">
                    <div class="sys-settings-card-title">我的设备</div>
                    <div class="sys-settings-card-subtitle">点「设为当前」切换后端;名称仅本地保存</div>
                </div>
                <div style="margin-top:10px;display:flex;flex-direction:column;gap:8px">
                    {devices.value.map(d => {
                        const isActive = d.machineId === activeId.value;
                        const isEditing = d.machineId === editingId.value;
                        return (
                            <div
                                key={d.machineId}
                                style={`display:flex;align-items:center;gap:8px;padding:8px 10px;border-radius:8px;border:1px solid ${isActive ? 'var(--accent-emphasis)' : 'var(--border-color)'};background:var(--bg-panel)`}
                            >
                                <span
                                    style={`display:inline-block;width:8px;height:8px;border-radius:50%;flex-shrink:0;background:${isActive ? 'var(--success-emphasis)' : 'var(--text-muted)'}`}
                                />
                                {isEditing ? (
                                    <input
                                        class="sys-settings-input"
                                        style="flex:1;padding:4px 8px;font-size:13px"
                                        value={editDraft.value}
                                        autoFocus
                                        onInput={e => (editDraft.value = (e.target as HTMLInputElement).value)}
                                        onKeyDown={e => e.key === 'Enter' && saveEdit(d)}
                                    />
                                ) : (
                                    <span style="flex:1;min-width:0">
                                        <span style="font-size:13px;color:var(--text-main)">{d.name}</span>
                                        <span style="font-size:11px;color:var(--text-muted);margin-left:6px">
                                            {d.machineId.slice(0, 8)}…
                                        </span>
                                        {isActive && (
                                            <span style="font-size:11px;color:var(--accent-fg);margin-left:6px">
                                                当前
                                            </span>
                                        )}
                                    </span>
                                )}
                                <span style="display:flex;gap:6px;flex-shrink:0">
                                    {isEditing ? (
                                        <button class="sys-settings-option-btn" onClick={() => saveEdit(d)}>
                                            保存
                                        </button>
                                    ) : (
                                        <Fragment>
                                            {!isActive && (
                                                <button
                                                    class="sys-settings-option-btn"
                                                    disabled={busy.value}
                                                    onClick={() => doSwitch(d)}
                                                >
                                                    设为当前
                                                </button>
                                            )}
                                            <button class="sys-settings-option-btn" onClick={() => startEdit(d)}>
                                                重命名
                                            </button>
                                            <button class="sys-settings-option-btn" onClick={() => doDelete(d)}>
                                                删除
                                            </button>
                                        </Fragment>
                                    )}
                                </span>
                            </div>
                        );
                    })}
                    {devices.value.length === 0 && (
                        <span style="font-size:12px;color:var(--text-muted)">还没有设备,先扫码添加一台</span>
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

    if (embedded) return body;
    return (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">我的设备</div>
            <div class="sys-settings-section-desc">扫码绑定远程机器,可保存多台并随时切换当前后端。</div>
            {body}
        </div>
    );
}
