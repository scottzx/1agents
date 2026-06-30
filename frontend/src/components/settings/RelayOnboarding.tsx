/**
 * 客户端落地引导(三步门禁)。
 *
 * 真实客户端(happy-server 在 :3005 托管 H5,无同源后端 → relay 模式)进来时,
 * 把原「未连接设备」配对门禁改造成分步引导:
 *   ① 账号(创建/确认中转账户)→ ② 订阅(领体验/激活码,必须 active)→ ③ 配对设备
 *
 * 前两步过了才进配对。订阅是硬门槛:对应 server 的 subscription_required —— 若
 * 自动连接因订阅被拒落到门禁,进入即拉 getSubscription(),非 active 会停在 Step 2。
 *
 * 复用:Step 3 直接渲染 <RelayPairingPanel embedded>(账户级配对那套)。配对是
 * 账户级审批 —— Step 1 创建的是「本端账户 C」,审批机器(daemon)即让该机器加入
 * 账户 C;于是 C 的 user-scoped 中转连接天然带上 C 的订阅(Model A #1 账户级绑定,
 * 替代旧 RelayDevicePanel「借机器 token」的设备档案流)。进入 app 后订阅管理仍走
 * 设置里的 SubscriptionPanel(本组件只负责落地门禁)。
 */
import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { useEffect } from 'preact/hooks';
import {
    getSubscription,
    claimTrial,
    activateCode,
    hasRelayAccount,
    NoRelayAccountError,
    TrialAlreadyClaimedError,
    InvalidCodeError,
    CodeUsedError,
    type SubscriptionInfo,
} from '../../services/subscriptionService';
import { createAccount } from '../../services/relay/relayClient';
import { getPlatformBridge } from '@1agents/core/platform/bridge';
import { RelayPairingPanel } from './RelayPairingPanel';

const LS_URL = 'oneagents.relay.url';

type Step = 1 | 2 | 3;

const STEPS: { n: Step; label: string }[] = [
    { n: 1, label: '账号' },
    { n: 2, label: '订阅' },
    { n: 3, label: '配对设备' },
];

/** 中转地址默认值:happy-server 注入的 serverUrl → 当前 origin。 */
function defaultServerUrl(): string {
    const cfg = (window as unknown as { __HAPPY_CONFIG__?: { serverUrl?: string } }).__HAPPY_CONFIG__;
    return cfg?.serverUrl || window.location.origin;
}

/** 剩余天数(向上取整,至少 0)。 */
function daysLeft(expiresAt: string): number {
    const ms = new Date(expiresAt).getTime() - Date.now();
    return Math.max(0, Math.ceil(ms / 86_400_000));
}

/** ISO → 本地日期串(到日)。 */
function fmtDate(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    return d.toLocaleDateString();
}

export function RelayOnboarding({ onReady }: { onReady?: () => void }) {
    const step = useSignal<Step>(1);

    // ── Step 1:账号 ──
    const serverUrl = useSignal<string>(getPlatformBridge().storage.get(LS_URL) || defaultServerUrl());
    const hasAccount = useSignal<boolean>(hasRelayAccount());
    const creatingAccount = useSignal(false);
    const accountError = useSignal('');

    // ── Step 2:订阅 ──
    const sub = useSignal<SubscriptionInfo | null>(null);
    const subLoading = useSignal(false);
    const subError = useSignal('');
    const subNotice = useSignal('');
    const claiming = useSignal(false);
    const activating = useSignal(false);
    const codeInput = useSignal('');

    const persistUrl = () => {
        const cleaned = serverUrl.value.trim().replace(/\/+$/, '');
        getPlatformBridge().storage.set(LS_URL, cleaned);
        return cleaned;
    };

    const doCreateAccount = async () => {
        creatingAccount.value = true;
        accountError.value = '';
        try {
            await createAccount(persistUrl());
            hasAccount.value = true;
        } catch (e) {
            accountError.value = '创建账户失败:' + ((e as Error)?.message ?? String(e));
        } finally {
            creatingAccount.value = false;
        }
    };

    const refreshSub = async () => {
        if (!hasRelayAccount()) {
            hasAccount.value = false;
            step.value = 1;
            return;
        }
        subLoading.value = true;
        subError.value = '';
        try {
            sub.value = await getSubscription();
        } catch (e) {
            if (e instanceof NoRelayAccountError) {
                hasAccount.value = false;
                step.value = 1;
            } else {
                subError.value = (e as Error)?.message ?? String(e);
            }
        } finally {
            subLoading.value = false;
        }
    };

    const doClaim = async () => {
        claiming.value = true;
        subError.value = '';
        subNotice.value = '';
        try {
            const r = await claimTrial();
            subNotice.value = `体验已开通,有效期至 ${fmtDate(r.expiresAt)}`;
            await refreshSub();
        } catch (e) {
            if (e instanceof TrialAlreadyClaimedError) subError.value = '已领取过体验';
            else if (e instanceof NoRelayAccountError) {
                hasAccount.value = false;
                step.value = 1;
            } else subError.value = (e as Error)?.message ?? String(e);
        } finally {
            claiming.value = false;
        }
    };

    const doActivate = async () => {
        const code = codeInput.value.trim();
        if (!code) return;
        activating.value = true;
        subError.value = '';
        subNotice.value = '';
        try {
            const r = await activateCode(code);
            subNotice.value = `激活成功,有效期至 ${fmtDate(r.expiresAt)}`;
            codeInput.value = '';
            await refreshSub();
        } catch (e) {
            if (e instanceof InvalidCodeError) subError.value = '激活码无效';
            else if (e instanceof CodeUsedError) subError.value = '激活码已被使用';
            else if (e instanceof NoRelayAccountError) {
                hasAccount.value = false;
                step.value = 1;
            } else subError.value = (e as Error)?.message ?? String(e);
        } finally {
            activating.value = false;
        }
    };

    // 进入 Step 2 时(或账户就绪后)自动拉订阅。
    useEffect(() => {
        if (step.value === 2) void refreshSub();
    }, [step.value]);

    const isActive = sub.value?.status === 'active';

    // ── 步骤指示器 ──
    const stepper = (
        <div class="onb-stepper">
            {STEPS.map((s, i) => (
                <Fragment key={s.n}>
                    {i > 0 && <span class={`onb-stepper-line ${step.value > s.n - 1 ? 'is-done' : ''}`} />}
                    <div
                        class={`onb-stepper-item ${
                            step.value === s.n ? 'is-active' : step.value > s.n ? 'is-done' : ''
                        }`}
                    >
                        <span class="onb-stepper-dot">{step.value > s.n ? '✓' : s.n}</span>
                        <span class="onb-stepper-label">{s.label}</span>
                    </div>
                </Fragment>
            ))}
        </div>
    );

    // ── Step 1 内容 ──
    const renderStep1 = () => (
        <div class="bento-card sub-card">
            <div class="bento-zone-header">
                <div class="bento-card-title">中转账户</div>
            </div>
            <div class="bento-zone-body">
                <div class="bento-card-desc">
                    当前页面由中转服务器提供。先确认中转地址并创建一个账户,后续的订阅与设备都基于它。
                </div>
                <div class="onb-field">
                    <label class="onb-field-label">中转地址</label>
                    <input
                        class="sys-settings-input onb-input"
                        value={serverUrl.value}
                        disabled={hasAccount.value}
                        onInput={e => (serverUrl.value = (e.target as HTMLInputElement).value)}
                        placeholder="https://relay.example.com"
                    />
                </div>
                {hasAccount.value && <div class="onb-ok-line">账户已就绪</div>}
                {accountError.value && <div class="sub-toast sub-toast--danger">{accountError.value}</div>}
            </div>
            <div class="bento-zone-footer sub-card-actions">
                {hasAccount.value ? (
                    <button class="sys-settings-btn primary" onClick={() => (step.value = 2)}>
                        下一步:订阅
                    </button>
                ) : (
                    <button
                        class="sys-settings-btn primary"
                        disabled={creatingAccount.value || !serverUrl.value.trim()}
                        onClick={doCreateAccount}
                    >
                        {creatingAccount.value ? '创建中…' : '创建账户'}
                    </button>
                )}
            </div>
        </div>
    );

    // ── Step 2 内容 ──
    const renderStep2 = () => {
        const info = sub.value;
        return (
            <Fragment>
                {subLoading.value && !info ? (
                    <div class="bento-card sub-card">
                        <span style="color:var(--text-muted);font-size:13px">加载订阅状态…</span>
                    </div>
                ) : isActive && info ? (
                    <div class="bento-card sub-card sub-card--success">
                        <div class="bento-zone-header">
                            <span class="sub-status-badge sub-status-badge--success">订阅生效中</span>
                            {info.plan && <span class="sub-plan-chip">{info.plan}</span>}
                        </div>
                        <div class="bento-zone-body">
                            {info.expiresAt && (
                                <div class="bento-card-desc">
                                    有效期至 <strong>{fmtDate(info.expiresAt)}</strong>,剩余{' '}
                                    <strong>{daysLeft(info.expiresAt)}</strong> 天。
                                    {info.source === 'trial' && <span class="sub-source-note">(体验)</span>}
                                </div>
                            )}
                        </div>
                        <div class="bento-zone-footer sub-card-actions">
                            <button class="sys-settings-btn primary" onClick={() => (step.value = 3)}>
                                下一步:配对设备
                            </button>
                        </div>
                    </div>
                ) : (
                    <Fragment>
                        <div class="bento-card sub-card sub-card--warning">
                            <div class="bento-zone-header">
                                <span class="sub-status-badge sub-status-badge--warning">
                                    {info?.status === 'expired' ? '订阅已过期' : '尚未激活'}
                                </span>
                            </div>
                            <div class="bento-zone-body">
                                <div class="bento-card-desc">
                                    需要有效订阅才能配对设备。可领取 3 天体验,或用激活码激活。
                                </div>
                            </div>
                        </div>

                        {/* 领取体验 */}
                        <div class="bento-card sub-card">
                            <div class="bento-zone-header">
                                <div class="bento-card-title">领取体验(3 天)</div>
                            </div>
                            <div class="bento-zone-body">
                                <div class="bento-card-desc">首次可免费领取 3 天体验,每个账户仅限一次。</div>
                            </div>
                            <div class="bento-zone-footer sub-card-actions">
                                <button
                                    class="sys-settings-btn primary"
                                    disabled={claiming.value || activating.value}
                                    onClick={doClaim}
                                >
                                    {claiming.value ? '领取中…' : '领取体验'}
                                </button>
                            </div>
                        </div>

                        {/* 激活码 */}
                        <div class="bento-card sub-card">
                            <div class="bento-zone-header">
                                <div class="bento-card-title">激活码</div>
                            </div>
                            <div class="bento-zone-body">
                                <div class="bento-card-desc">有激活码可在此激活订阅。</div>
                                <div class="onb-field">
                                    <input
                                        class="sys-settings-input onb-input"
                                        value={codeInput.value}
                                        onInput={e => (codeInput.value = (e.target as HTMLInputElement).value)}
                                        onKeyDown={e => e.key === 'Enter' && void doActivate()}
                                        placeholder="输入激活码"
                                    />
                                </div>
                            </div>
                            <div class="bento-zone-footer sub-card-actions">
                                <button
                                    class="sys-settings-btn primary"
                                    disabled={activating.value || claiming.value || !codeInput.value.trim()}
                                    onClick={doActivate}
                                >
                                    {activating.value ? '激活中…' : '激活'}
                                </button>
                                <button
                                    class="sys-settings-btn ghost"
                                    disabled={subLoading.value || claiming.value || activating.value}
                                    onClick={refreshSub}
                                >
                                    刷新状态
                                </button>
                            </div>
                        </div>
                    </Fragment>
                )}

                {subNotice.value && <div class="sub-toast sub-toast--success">{subNotice.value}</div>}
                {subError.value && <div class="sub-toast sub-toast--danger">{subError.value}</div>}

                <div class="onb-nav">
                    <button class="sys-settings-btn ghost" onClick={() => (step.value = 1)}>
                        上一步
                    </button>
                </div>
            </Fragment>
        );
    };

    // ── Step 3 内容 ──
    // 账户级配对:在机器上跑 `happy auth login` 生成配对码,这里扫码/粘贴审批,
    // 即把该机器并入 Step 1 创建的本端账户(于是本账户的订阅天然随中转连接生效);
    // 再「设为后端并进入」即可开始使用。
    const renderStep3 = () => (
        <Fragment>
            <div class="bento-card sub-card">
                <div class="bento-zone-body">
                    <div class="bento-card-desc">
                        订阅已就绪。在机器端运行 <code>happy auth login</code> 生成配对二维码/链接,这里扫码或
                        粘贴审批,即把该机器并入你的账户;随后「设为后端并进入」即可开始使用。
                    </div>
                </div>
            </div>
            <RelayPairingPanel embedded onNodeSelected={() => onReady?.()} />
            <div class="onb-nav">
                <button class="sys-settings-btn ghost" onClick={() => (step.value = 2)}>
                    上一步
                </button>
            </div>
        </Fragment>
    );

    return (
        <div class="app-container" style="min-height:100vh;background:var(--bg-page);overflow:auto">
            <div class="sys-settings-page sys-settings-page--bare">
                <div class="sys-settings-content">
                    <div class="sys-settings-section" style="margin-bottom:8px">
                        <div class="sys-settings-section-title">连接到你的设备</div>
                        <div class="sys-settings-section-desc">
                            按三步完成落地:创建账户 → 开通订阅 → 配对一台远程机器即可开始使用。
                        </div>
                    </div>
                    {stepper}
                    {step.value === 1 && renderStep1()}
                    {step.value === 2 && renderStep2()}
                    {step.value === 3 && renderStep3()}
                </div>
            </div>
        </div>
    );
}
