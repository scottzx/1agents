/**
 * 订阅 / 领取体验面板(放设置「账号」分类)。
 *
 * 直连 happy-server 读取/领取订阅(见 subscriptionService),与「远程控制」
 * 分类里的中转账户共用同一 relay token:
 *  - 有账户 → 展示订阅三态(active/expired/none)+ 领取体验按钮。
 *  - 无账户 → 兜底提示并提供「创建账户」入口(复用 relayClient.createAccount),
 *    或引导用户去「远程控制」配置中转地址。
 *
 * 设计语言:Bento 卡片 + 语义色 token(active=success,expired=danger,
 * none=warning)。不做激活码输入框 / 设备列表(下一波)。
 */
import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { useEffect } from 'preact/hooks';
import {
    getSubscription,
    claimTrial,
    hasRelayAccount,
    NoRelayAccountError,
    TrialAlreadyClaimedError,
    type SubscriptionInfo,
} from '../../services/subscriptionService';
import { createAccount } from '../../services/relay/relayClient';

const LS_URL = 'oneagents.relay.url';

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

export function SubscriptionPanel() {
    // null = 尚未加载;hasAccount=false 时不会去拉。
    const sub = useSignal<SubscriptionInfo | null>(null);
    const hasAccount = useSignal<boolean>(hasRelayAccount());
    const loading = useSignal(false);
    const claiming = useSignal(false);
    const error = useSignal('');
    const notice = useSignal('');

    const refresh = async () => {
        if (!hasRelayAccount()) {
            hasAccount.value = false;
            return;
        }
        hasAccount.value = true;
        loading.value = true;
        error.value = '';
        try {
            sub.value = await getSubscription();
        } catch (e) {
            if (e instanceof NoRelayAccountError) {
                hasAccount.value = false;
            } else {
                error.value = (e as Error)?.message ?? String(e);
            }
        } finally {
            loading.value = false;
        }
    };

    useEffect(() => {
        void refresh();
    }, []);

    const doClaim = async () => {
        claiming.value = true;
        error.value = '';
        notice.value = '';
        try {
            const r = await claimTrial();
            notice.value = `体验已开通,有效期至 ${fmtDate(r.expiresAt)}`;
            await refresh();
        } catch (e) {
            if (e instanceof TrialAlreadyClaimedError) {
                error.value = '你已领取过体验,无法重复领取。';
            } else if (e instanceof NoRelayAccountError) {
                hasAccount.value = false;
            } else {
                error.value = (e as Error)?.message ?? String(e);
            }
        } finally {
            claiming.value = false;
        }
    };

    const doCreateAccount = async () => {
        loading.value = true;
        error.value = '';
        notice.value = '';
        try {
            const serverUrl = localStorage.getItem(LS_URL) || window.location.origin;
            await createAccount(serverUrl);
            notice.value = '账户已创建';
            await refresh();
        } catch (e) {
            error.value = '创建账户失败:' + ((e as Error)?.message ?? String(e));
        } finally {
            loading.value = false;
        }
    };

    // ── 无账户兜底 ───────────────────────────────────────────────────────────
    const renderNoAccount = () => (
        <div class="bento-card sub-card sub-card--warning">
            <div class="bento-zone-header">
                <div class="bento-card-icon sub-card-icon">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                        <circle cx="9" cy="7" r="4" />
                        <line x1="19" y1="8" x2="19" y2="14" />
                        <line x1="22" y1="11" x2="16" y2="11" />
                    </svg>
                </div>
                <div class="bento-card-title">尚未创建中转账户</div>
            </div>
            <div class="bento-zone-body">
                <div class="bento-card-desc">
                    订阅与领取体验需要先在中转(Relay)上创建账户。你可以直接在这里创建,或前往
                    「远程控制」分类配置中转地址后再创建。
                </div>
            </div>
            <div class="bento-zone-footer sub-card-actions">
                <button class="sys-settings-btn primary" disabled={loading.value} onClick={doCreateAccount}>
                    {loading.value ? '创建中…' : '创建账户'}
                </button>
            </div>
        </div>
    );

    // ── 订阅状态卡 ───────────────────────────────────────────────────────────
    const renderStatusCard = (info: SubscriptionInfo) => {
        const variant = info.status === 'active' ? 'success' : info.status === 'expired' ? 'danger' : 'warning';
        const title = info.status === 'active' ? '订阅生效中' : info.status === 'expired' ? '订阅已过期' : '尚未激活';
        return (
            <div class={`bento-card sub-card sub-card--${variant}`}>
                <div class="bento-zone-header">
                    <span class={`sub-status-badge sub-status-badge--${variant}`}>{title}</span>
                    {info.plan && <span class="sub-plan-chip">{info.plan}</span>}
                </div>
                <div class="bento-zone-body">
                    {info.status === 'active' && info.expiresAt && (
                        <div class="bento-card-desc">
                            有效期至 <strong>{fmtDate(info.expiresAt)}</strong>,剩余{' '}
                            <strong>{daysLeft(info.expiresAt)}</strong> 天。
                            {info.source === 'trial' && <span class="sub-source-note">(体验)</span>}
                        </div>
                    )}
                    {info.status === 'expired' && (
                        <div class="bento-card-desc">
                            你的订阅已于 {info.expiresAt ? fmtDate(info.expiresAt) : '此前'} 过期,
                            中继连接可能已被服务器拒绝。请续订或领取体验后重连。
                        </div>
                    )}
                    {info.status === 'none' && (
                        <div class="bento-card-desc">当前没有有效订阅。你可以先领取 3 天体验试用。</div>
                    )}
                    <div class="sub-meta-row">
                        <span class="sub-meta-item">最大设备数:{info.maxDevices}</span>
                    </div>
                </div>
            </div>
        );
    };

    return (
        <div class="sys-settings-section">
            <div class="sys-settings-section-title">账号与订阅</div>
            <div class="sys-settings-section-desc">查看当前订阅状态,或领取体验试用。订阅基于中转(Relay)账户。</div>

            {!hasAccount.value ? (
                renderNoAccount()
            ) : (
                <Fragment>
                    {loading.value && !sub.value ? (
                        <div class="bento-card sub-card">
                            <span style="color:var(--text-muted);font-size:13px">加载订阅状态…</span>
                        </div>
                    ) : (
                        sub.value && renderStatusCard(sub.value)
                    )}

                    {/* 领取体验 */}
                    <div class="bento-card sub-card">
                        <div class="bento-zone-header">
                            <div class="bento-card-icon sub-card-icon">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <polyline points="20 12 20 22 4 22 4 12" />
                                    <rect x="2" y="7" width="20" height="5" />
                                    <line x1="12" y1="22" x2="12" y2="7" />
                                    <path d="M12 7H7.5a2.5 2.5 0 0 1 0-5C11 2 12 7 12 7z" />
                                    <path d="M12 7h4.5a2.5 2.5 0 0 0 0-5C13 2 12 7 12 7z" />
                                </svg>
                            </div>
                            <div class="bento-card-title">领取体验(3 天)</div>
                        </div>
                        <div class="bento-zone-body">
                            <div class="bento-card-desc">
                                首次可免费领取 3 天体验,期间可正常使用中转连接。每个账户仅限一次。
                            </div>
                        </div>
                        <div class="bento-zone-footer sub-card-actions">
                            <button
                                class="sys-settings-btn primary"
                                disabled={claiming.value || loading.value}
                                onClick={doClaim}
                            >
                                {claiming.value ? '领取中…' : '领取体验'}
                            </button>
                            <button
                                class="sys-settings-btn ghost"
                                disabled={loading.value || claiming.value}
                                onClick={refresh}
                            >
                                刷新状态
                            </button>
                        </div>
                    </div>
                </Fragment>
            )}

            {notice.value && <div class="sub-toast sub-toast--success">{notice.value}</div>}
            {error.value && <div class="sub-toast sub-toast--danger">{error.value}</div>}
        </div>
    );
}
