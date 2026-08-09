import { h } from 'preact';
import { useSignal } from '@preact/signals';
import * as ui from '../../stores/uiStore';
import { LlmProviderPanel } from '../settings/LlmProviderPanel';
import { AgentSwitchPanel } from '../settings/AgentSwitchPanel';

export interface CcProvidersPanelProps {
    ccProvidersUrl?: string;
    panelStyle?: string | Record<string, string | number>;
}

export function CcProvidersPanel(props: CcProvidersPanelProps) {
    const { ccProvidersUrl, panelStyle } = props;
    void ccProvidersUrl;
    void panelStyle;
    const activeTab = useSignal<'agents' | 'providers'>('agents');

    return (
        <div
            style={{
                width: '100%',
                height: '100%',
                display: 'flex',
                flexDirection: 'column',
                background: 'var(--bg-body, #fff)',
                overflow: 'hidden',
            }}
        >
            {/* Top Navigation Tabs - Clean, Minimalist, No Icons */}
            <div
                style={{
                    display: 'flex',
                    borderBottom: '1px solid var(--border, #e0e0e0)',
                    padding: '0 16px',
                    background: 'var(--bg-header, #fafafa)',
                }}
            >
                <button
                    onClick={() => (activeTab.value = 'agents')}
                    style={{
                        padding: '12px 18px',
                        border: 'none',
                        borderBottom:
                            activeTab.value === 'agents' ? '2px solid var(--accent, #0066cc)' : '2px solid transparent',
                        background: 'transparent',
                        color: activeTab.value === 'agents' ? 'var(--accent, #0066cc)' : 'var(--fg-muted, #666)',
                        fontWeight: activeTab.value === 'agents' ? 600 : 400,
                        cursor: 'pointer',
                        fontSize: '0.88rem',
                    }}
                >
                    智能体切换
                </button>
                <button
                    onClick={() => (activeTab.value = 'providers')}
                    style={{
                        padding: '12px 18px',
                        border: 'none',
                        borderBottom:
                            activeTab.value === 'providers'
                                ? '2px solid var(--accent, #0066cc)'
                                : '2px solid transparent',
                        background: 'transparent',
                        color: activeTab.value === 'providers' ? 'var(--accent, #0066cc)' : 'var(--fg-muted, #666)',
                        fontWeight: activeTab.value === 'providers' ? 600 : 400,
                        cursor: 'pointer',
                        fontSize: '0.88rem',
                    }}
                >
                    服务商配置
                </button>
            </div>

            {/* Tab Content */}
            <div style={{ flex: 1, minHeight: 0, overflowY: 'auto' }}>
                {activeTab.value === 'agents' ? (
                    <AgentSwitchPanel />
                ) : (
                    <LlmProviderPanel language={ui.language.value} />
                )}
            </div>
        </div>
    );
}
