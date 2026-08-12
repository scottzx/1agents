import { h } from 'preact';
import { useSignal } from '@preact/signals';
import * as ui from '../../stores/uiStore';
import { ShellNav, type ShellTab } from '../platform/ShellNav';
import { LlmProviderPanel } from '../settings/LlmProviderPanel';
import { AgentSwitchPanel } from '../settings/AgentSwitchPanel';
import { AgentProfilePanel } from '../settings/AgentProfilePanel';

export interface CcProvidersPanelProps {
    ccProvidersUrl?: string;
    panelStyle?: string | Record<string, string | number>;
}

export function CcProvidersPanel(props: CcProvidersPanelProps) {
    const { ccProvidersUrl, panelStyle } = props;
    void ccProvidersUrl;
    void panelStyle;
    const activeTab = useSignal<'profiles' | 'agents' | 'providers'>('profiles');

    const providerTabs: ShellTab[] = [
        { id: 'profiles', label: 'Agent 预设 (Profiles)' },
        { id: 'agents', label: '智能体绑定 (Agents)' },
        { id: 'providers', label: '服务商配置 (Providers)' },
    ];

    return (
        <div class="project-shell">
            <ShellNav
                tabs={providerTabs}
                activeTab={activeTab.value}
                onSelectTab={id => (activeTab.value = id as 'profiles' | 'agents' | 'providers')}
            />
            <div style={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
                {activeTab.value === 'profiles' ? (
                    <AgentProfilePanel />
                ) : activeTab.value === 'agents' ? (
                    <AgentSwitchPanel />
                ) : (
                    <LlmProviderPanel language={ui.language.value} />
                )}
            </div>
        </div>
    );
}
