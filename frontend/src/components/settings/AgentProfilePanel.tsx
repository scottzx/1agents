import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import {
    type AgentProfile,
    agentProfiles,
    loadAgentProfiles,
    profileAvailability,
    profileLabel,
    profilesError,
    profilesLoading,
    runtimeDefinitions,
    type RuntimeDefinition,
} from '../../stores/agentProfileStore';
import { type ProviderEndpointItem, type ProviderItem } from './LlmProviderPanel';

interface ProviderModel {
    provider_id: string;
    model_id: string;
    available: boolean;
}

const emptyProfile = (): AgentProfile => ({
    id: '',
    name: '',
    runtime_id: 'grok-build',
    provider_id: '',
    model_id: '',
    options: {},
    revision: 0,
    status: 'active',
});

export function compatibleProvidersForRuntime(providers: ProviderItem[], runtime?: RuntimeDefinition): ProviderItem[] {
    return providers.filter(provider => {
        if (provider.status === 'archived') return false;
        return provider.endpoints?.some(endpoint =>
            runtime?.supported_endpoint_families.includes(endpoint.family || legacyFamily(endpoint))
        );
    });
}

export function RuntimeOptionFields(props: {
    runtime?: RuntimeDefinition;
    options: Record<string, unknown>;
    onChange: (key: string, value: unknown) => void;
}) {
    return (
        <div class="runtime-option-fields">
            {(props.runtime?.option_schema || []).map(option => (
                <label key={option.key} class="ws-modal-label">
                    {option.label}{' '}
                    {option.type === 'boolean' ? (
                        <input
                            type="checkbox"
                            checked={Boolean(props.options[option.key])}
                            onChange={(event: Event) =>
                                props.onChange(option.key, (event.target as HTMLInputElement).checked)
                            }
                        />
                    ) : option.type === 'select' ? (
                        <select
                            class="ws-modal-select"
                            value={String(props.options[option.key] ?? option.default ?? '')}
                            onChange={(event: Event) =>
                                props.onChange(option.key, (event.target as HTMLSelectElement).value)
                            }
                        >
                            {(option.choices || []).map(choice => (
                                <option key={choice} value={choice}>
                                    {choice}
                                </option>
                            ))}
                        </select>
                    ) : (
                        <input
                            class="ws-modal-input"
                            value={String(props.options[option.key] ?? option.default ?? '')}
                            onInput={(event: Event) =>
                                props.onChange(option.key, (event.target as HTMLInputElement).value)
                            }
                        />
                    )}
                </label>
            ))}
        </div>
    );
}

export function AgentProfilePanel() {
    const providers = useSignal<ProviderItem[]>([]);
    const models = useSignal<ProviderModel[]>([]);
    const includeArchived = useSignal(false);
    const editing = useSignal<AgentProfile | null>(null);
    const error = useSignal('');

    const reload = async () => {
        await loadAgentProfiles(includeArchived.value);
        const [providerRes, modelRes] = await Promise.all([fetch('/api/providers'), fetch('/api/provider-models')]);
        if (providerRes.ok) providers.value = (await providerRes.json()).providers || [];
        if (modelRes.ok) models.value = (await modelRes.json()).models || [];
    };

    useEffect(() => {
        void reload();
    }, []);

    const runtime = editing.value
        ? runtimeDefinitions.value.find(item => item.id === editing.value?.runtime_id)
        : undefined;
    const compatibleProviders = compatibleProvidersForRuntime(providers.value, runtime);
    const compatibleModels = models.value.filter(
        model => model.provider_id === editing.value?.provider_id && model.available
    );

    const save = async () => {
        const profile = editing.value;
        if (!profile?.id || !profile.name || !profile.provider_id || !profile.model_id) {
            error.value = '请完整选择 Runtime、Provider 和 Model';
            return;
        }
        const exists = agentProfiles.value.some(item => item.id === profile.id);
        const res = await fetch(
            exists ? `/api/agent-profiles/${encodeURIComponent(profile.id)}` : '/api/agent-profiles',
            {
                method: exists ? 'PUT' : 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(profile),
            }
        );
        if (!res.ok) {
            error.value = await res.text();
            return;
        }
        editing.value = null;
        await reload();
    };

    const setStatus = async (profile: AgentProfile, action: 'archive' | 'restore') => {
        const res = await fetch(`/api/agent-profiles/${encodeURIComponent(profile.id)}/${action}`, { method: 'POST' });
        if (!res.ok) {
            error.value = await res.text();
            return;
        }
        await reload();
    };

    const disabledReason = (profile: AgentProfile): string => {
        if (profile.status !== 'active') return profile.status === 'archived' ? '已归档' : '待补全';
        if (!runtimeDefinitions.value.find(item => item.id === profile.runtime_id)?.installed) return 'Runtime 未安装';
        const provider = providers.value.find(item => item.id === profile.provider_id);
        if (!provider || provider.status === 'archived') return 'Provider 不可用';
        const knownModel = models.value.find(
            item => item.provider_id === profile.provider_id && item.model_id === profile.model_id
        );
        if (knownModel && !knownModel.available) return '模型 unavailable';
        return profileAvailability.value[profile.id] || '';
    };

    return (
        <div style={{ padding: 16, maxWidth: 900, margin: '0 auto', color: 'var(--fg)' }}>
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <div>
                    <h2 style={{ margin: 0, fontSize: '1.25rem' }}>Agent Profiles</h2>
                    <p style={{ color: 'var(--fg-muted)', marginTop: 4 }}>
                        Runtime × Provider × Model 的可调度执行者。
                    </p>
                </div>
                <button onClick={() => (editing.value = emptyProfile())}>+ 新建 Profile</button>
            </div>
            <label style={{ display: 'block', margin: '12px 0' }}>
                <input
                    type="checkbox"
                    checked={includeArchived.value}
                    onChange={() => {
                        includeArchived.value = !includeArchived.value;
                        void reload();
                    }}
                />{' '}
                显示已归档
            </label>
            {(error.value || profilesError.value) && (
                <div style={{ color: '#d33', marginBottom: 12 }}>{error.value || profilesError.value}</div>
            )}
            {profilesLoading.value ? (
                <p>加载中…</p>
            ) : (
                agentProfiles.value.map(profile => {
                    const provider = providers.value.find(item => item.id === profile.provider_id);
                    const reason = disabledReason(profile);
                    return (
                        <div
                            key={profile.id}
                            style={{
                                border: '1px solid var(--border)',
                                borderRadius: 8,
                                padding: 12,
                                marginBottom: 10,
                            }}
                        >
                            <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12 }}>
                                <div>
                                    <strong>{profileLabel(profile, provider?.name)}</strong>
                                    <div style={{ color: reason ? '#c67a00' : 'var(--fg-muted)', fontSize: 12 }}>
                                        revision {profile.revision} {reason ? `· ${reason}` : '· 可用'}
                                    </div>
                                </div>
                                <div>
                                    <button
                                        onClick={() =>
                                            (editing.value = { ...profile, options: { ...profile.options } })
                                        }
                                    >
                                        编辑
                                    </button>{' '}
                                    {profile.status === 'archived' ? (
                                        <button onClick={() => void setStatus(profile, 'restore')}>恢复</button>
                                    ) : (
                                        <button onClick={() => void setStatus(profile, 'archive')}>归档</button>
                                    )}
                                </div>
                            </div>
                        </div>
                    );
                })
            )}
            {editing.value && (
                <div class="ws-modal-overlay" onClick={() => (editing.value = null)}>
                    <div class="ws-modal" onClick={(event: MouseEvent) => event.stopPropagation()}>
                        <div class="ws-modal-header">Profile 配置</div>
                        <div class="ws-modal-body">
                            <label class="ws-modal-label">ID</label>
                            <input
                                class="ws-modal-input"
                                value={editing.value.id}
                                disabled={agentProfiles.value.some(item => item.id === editing.value?.id)}
                                onInput={(event: Event) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        id: (event.target as HTMLInputElement).value,
                                    })
                                }
                            />
                            <label class="ws-modal-label">名称</label>
                            <input
                                class="ws-modal-input"
                                value={editing.value.name}
                                onInput={(event: Event) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        name: (event.target as HTMLInputElement).value,
                                    })
                                }
                            />
                            <label class="ws-modal-label">Runtime</label>
                            <select
                                class="ws-modal-select"
                                value={editing.value.runtime_id}
                                onChange={(event: Event) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        runtime_id: (event.target as HTMLSelectElement).value,
                                        provider_id: '',
                                        model_id: '',
                                    })
                                }
                            >
                                {runtimeDefinitions.value.map(item => (
                                    <option value={item.id} key={item.id}>
                                        {item.label}
                                    </option>
                                ))}
                            </select>
                            <label class="ws-modal-label">Provider</label>
                            <select
                                class="ws-modal-select"
                                value={editing.value.provider_id}
                                onChange={(event: Event) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        provider_id: (event.target as HTMLSelectElement).value,
                                        model_id: '',
                                    })
                                }
                            >
                                <option value="">请选择</option>
                                {compatibleProviders.map(item => (
                                    <option key={item.id} value={item.id}>
                                        {item.name}
                                    </option>
                                ))}
                            </select>
                            <label class="ws-modal-label">Model</label>
                            <select
                                class="ws-modal-select"
                                value={editing.value.model_id}
                                onChange={(event: Event) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        model_id: (event.target as HTMLSelectElement).value,
                                    })
                                }
                            >
                                <option value="">请选择</option>
                                {compatibleModels.map(item => (
                                    <option key={item.model_id} value={item.model_id}>
                                        {item.model_id}
                                    </option>
                                ))}
                            </select>
                            <RuntimeOptionFields
                                runtime={runtime}
                                options={editing.value.options || {}}
                                onChange={(key, value) =>
                                    (editing.value = {
                                        ...editing.value!,
                                        options: { ...editing.value!.options, [key]: value },
                                    })
                                }
                            />
                        </div>
                        <div class="ws-modal-footer">
                            <button onClick={() => (editing.value = null)}>取消</button>
                            <button onClick={() => void save()}>保存</button>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}

function legacyFamily(endpoint: ProviderEndpointItem): 'openai' | 'anthropic' {
    return endpoint.agent_id === 'claude' ? 'anthropic' : 'openai';
}
