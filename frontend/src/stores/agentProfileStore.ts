import { signal } from '@preact/signals';

export interface RuntimeOptionDefinition {
    key: string;
    label: string;
    type: 'boolean' | 'select' | 'string';
    choices?: string[];
    default?: unknown;
}

export interface RuntimeDefinition {
    id: string;
    label: string;
    supported_endpoint_families: Array<'openai' | 'anthropic'>;
    option_schema?: RuntimeOptionDefinition[];
    installed: boolean;
    unavailable_reason?: string;
}

export interface AgentProfile {
    id: string;
    name: string;
    runtime_id: string;
    provider_id?: string;
    model_id?: string;
    options?: Record<string, unknown>;
    revision: number;
    status: 'active' | 'disabled' | 'archived';
    system?: boolean;
}

export const agentProfiles = signal<AgentProfile[]>([]);
export const runtimeDefinitions = signal<RuntimeDefinition[]>([]);
export const profileAvailability = signal<Record<string, string>>({});
export const profilesLoading = signal(false);
export const profilesError = signal('');

export async function loadAgentProfiles(includeArchived = false): Promise<void> {
    profilesLoading.value = true;
    profilesError.value = '';
    try {
        const res = await fetch(`/api/agent-profiles${includeArchived ? '?include_archived=1' : ''}`, {
            cache: 'no-store',
        });
        if (!res.ok) throw new Error(await res.text());
        const data = await res.json();
        agentProfiles.value = data.profiles || [];
        runtimeDefinitions.value = data.runtimes || [];
        profileAvailability.value = data.profile_availability || {};
    } catch (error) {
        profilesError.value = error instanceof Error ? error.message : String(error);
    } finally {
        profilesLoading.value = false;
    }
}

export function profileLabel(profile: AgentProfile, providerName?: string): string {
    const runtime = runtimeDefinitions.value.find(item => item.id === profile.runtime_id);
    return [profile.name, runtime?.label || profile.runtime_id, providerName, profile.model_id]
        .filter(Boolean)
        .join(' · ');
}
