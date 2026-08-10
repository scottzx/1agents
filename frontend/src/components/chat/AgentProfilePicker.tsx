import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import {
    agentProfiles,
    loadAgentProfiles,
    profileAvailability,
    profileLabel,
    profilesLoading,
} from '../../stores/agentProfileStore';

export function AgentProfilePicker(props: {
    value?: string;
    onChange: (profileId: string) => void;
    allowLegacy?: boolean;
    disabled?: boolean;
}) {
    useEffect(() => {
        void loadAgentProfiles(false);
    }, []);
    const profiles = agentProfiles.value.filter(profile => profile.status === 'active');
    return (
        <select
            class="agent-type-picker"
            value={props.value || ''}
            disabled={props.disabled || profilesLoading.value}
            onChange={(event: Event) => props.onChange((event.target as HTMLSelectElement).value)}
        >
            {props.allowLegacy && <option value="">使用传统桌面智能体</option>}
            {!props.allowLegacy && <option value="">请选择 Profile</option>}
            {profiles.map(profile => (
                <option key={profile.id} value={profile.id} disabled={Boolean(profileAvailability.value[profile.id])}>
                    {profileLabel(profile)}
                    {profileAvailability.value[profile.id] ? ` · 不可用：${profileAvailability.value[profile.id]}` : ''}
                </option>
            ))}
        </select>
    );
}
