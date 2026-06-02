import { h, Component } from 'preact';

interface WelcomeOnboardingProps {
    language: 'zh-CN' | 'en-US';
    onCreateWorkspace: () => void;
}

export class WelcomeOnboarding extends Component<WelcomeOnboardingProps> {
    render() {
        const { language, onCreateWorkspace } = this.props;

        return (
            <div
                class="welcome-container"
                style="display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 100vh; width: 100vw; background-color: var(--bg-page); color: var(--text-main); font-family: var(--font-sans); padding: 40px; box-sizing: border-box; text-align: center;"
            >
                <style>{`
                    @keyframes pulse {
                        0% { transform: scale(1); box-shadow: 0 4px 12px rgba(9, 105, 218, 0.3); }
                        50% { transform: scale(1.03); box-shadow: 0 4px 20px rgba(9, 105, 218, 0.5); }
                        100% { transform: scale(1); box-shadow: 0 4px 12px rgba(9, 105, 218, 0.3); }
                    }
                    .welcome-card-anim {
                        transition: transform 0.2s, box-shadow 0.2s;
                    }
                    .welcome-card-anim:hover {
                        transform: translateY(-2px);
                        box-shadow: var(--shadow-lg);
                    }
                `}</style>
                <div
                    class="welcome-card welcome-card-anim"
                    style="max-width: 600px; width: 100%; background-color: var(--bg-panel); border: 1px solid var(--border-color); border-radius: 16px; padding: 48px; box-sizing: border-box; box-shadow: var(--shadow-lg); display: flex; flex-direction: column; align-items: center;"
                >
                    {/* Logo / Header */}
                    <div
                        class="welcome-logo-wrap"
                        style="width: 80px; height: 80px; border-radius: 20px; background: linear-gradient(135deg, var(--accent-color), #4f46e5); display: flex; align-items: center; justify-content: center; margin-bottom: 24px; box-shadow: 0 8px 16px rgba(9, 105, 218, 0.25);"
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="#ffffff"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            style="width: 42px; height: 42px;"
                        >
                            <path d="M12 2L2 7l10 5 10-5-10-5z" />
                            <path d="M2 17l10 5 10-5" />
                            <path d="M2 12l10 5 10-5" />
                        </svg>
                    </div>

                    <h1 style="font-size: 32px; font-weight: 700; margin: 0 0 12px 0; background: linear-gradient(135deg, var(--text-main), var(--text-secondary)); -webkit-background-clip: text; -webkit-text-fill-color: transparent;">
                        {language === 'zh-CN' ? '欢迎使用 1Agents' : 'Welcome to 1Agents'}
                    </h1>

                    <p style="font-size: 15px; color: var(--text-secondary); line-height: 1.6; margin: 0 0 32px 0; max-width: 480px;">
                        {language === 'zh-CN'
                            ? '轻量级、免配置的 Web 协同智能体开发工作台。集成极致响应的 Web 终端、全功能文件浏览器及智能体路由调度。'
                            : 'A lightweight, zero-config remote web workbench integrating high-performance terminals, file manager, and AI agent channel routing.'}
                    </p>

                    <div
                        class="welcome-features"
                        style="width: 100%; display: grid; grid-template-columns: 1fr; gap: 16px; margin-bottom: 36px; text-align: left;"
                    >
                        <div style="display: flex; gap: 14px; align-items: flex-start; padding: 12px; border-radius: 8px; background: rgba(9, 105, 218, 0.04); border: 1px solid rgba(9, 105, 218, 0.08);">
                            <div style="color: var(--accent-color); margin-top: 2px; flex-shrink: 0;">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    style="width: 18px; height: 18px;"
                                >
                                    <polyline points="4 17 10 11 4 5" />
                                    <line x1="12" y1="19" x2="20" y2="19" />
                                </svg>
                            </div>
                            <div>
                                <h4 style="font-size: 14px; font-weight: 600; margin: 0 0 4px 0; color: var(--text-main);">
                                    {language === 'zh-CN' ? '持久化终端会话' : 'Persistent Terminal Sessions'}
                                </h4>
                                <p style="font-size: 13px; color: var(--text-secondary); margin: 0; line-height: 1.4;">
                                    {language === 'zh-CN'
                                        ? '内置 tmux，支持意外断开自动持久重连。'
                                        : 'Built-in tmux state keeps terminal sessions running indefinitely.'}
                                </p>
                            </div>
                        </div>

                        <div style="display: flex; gap: 14px; align-items: flex-start; padding: 12px; border-radius: 8px; background: rgba(9, 105, 218, 0.04); border: 1px solid rgba(9, 105, 218, 0.08);">
                            <div style="color: var(--accent-color); margin-top: 2px; flex-shrink: 0;">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    style="width: 18px; height: 18px;"
                                >
                                    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                                </svg>
                            </div>
                            <div>
                                <h4 style="font-size: 14px; font-weight: 600; margin: 0 0 4px 0; color: var(--text-main);">
                                    {language === 'zh-CN' ? '全功能文件管理' : 'Full File-System Explorer'}
                                </h4>
                                <p style="font-size: 13px; color: var(--text-secondary); margin: 0; line-height: 1.4;">
                                    {language === 'zh-CN'
                                        ? '直观管理项目代码，支持 HTML/PDF/图片高清预览及在线编辑。'
                                        : 'Manage code files with premium online editing and HTML/PDF previews.'}
                                </p>
                            </div>
                        </div>
                    </div>

                    {/* Pulsing Onboarding Action Button */}
                    <button
                        class="welcome-cta-btn"
                        onClick={onCreateWorkspace}
                        style="display: inline-flex; align-items: center; justify-content: center; gap: 8px; padding: 14px 28px; background: linear-gradient(135deg, var(--accent-color), #4f46e5); color: #ffffff; border: none; border-radius: 30px; font-size: 15px; font-weight: 600; cursor: pointer; box-shadow: 0 4px 12px rgba(9, 105, 218, 0.3); animation: pulse 2s infinite;"
                    >
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            style="width: 16px; height: 16px;"
                        >
                            <line x1="12" y1="5" x2="12" y2="19" />
                            <line x1="5" y1="12" x2="19" y2="12" />
                        </svg>
                        {language === 'zh-CN' ? '创建第一个工作空间' : 'Create First Workspace'}
                    </button>
                </div>
            </div>
        );
    }
}
