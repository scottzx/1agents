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
        <div class="providers-panel-wrapper">
            <div class="providers-panel-container">
                <div class="providers-header-bar">
                    <div>
                        <h2 class="providers-header-title">
                            <svg
                                width="20"
                                height="20"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="12" cy="12" r="10"></circle>
                                <polygon points="12 8 8 12 12 16 16 12 12 8"></polygon>
                            </svg>
                            Agent Profiles (预设调度管理)
                        </h2>
                        <p class="providers-header-desc">Runtime × Provider × Model 的组合执行者配置。</p>
                    </div>
                    <button class="btn-primary" onClick={() => (editing.value = emptyProfile())}>
                        + 新建 Profile
                    </button>
                </div>

                <div
                    style={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'space-between',
                        marginTop: '-8px',
                    }}
                >
                    <label
                        style={{
                            display: 'inline-flex',
                            alignItems: 'center',
                            gap: '6px',
                            fontSize: '13px',
                            color: 'var(--text-secondary)',
                            cursor: 'pointer',
                        }}
                    >
                        <input
                            type="checkbox"
                            checked={includeArchived.value}
                            onChange={() => {
                                includeArchived.value = !includeArchived.value;
                                void reload();
                            }}
                        />
                        显示已归档 Profile
                    </label>
                </div>

                {(error.value || profilesError.value) && (
                    <div class="providers-alert-banner alert-danger">
                        <span>⚠️</span> {error.value || profilesError.value}
                    </div>
                )}

                {profilesLoading.value ? (
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)' }}>加载中…</div>
                ) : agentProfiles.value.length === 0 ? (
                    <div
                        style={{
                            padding: '40px 24px',
                            textAlign: 'center',
                            border: '1px dashed var(--border-color)',
                            borderRadius: 'var(--bento-radius)',
                            color: 'var(--text-secondary)',
                            backgroundColor: 'var(--bg-card)',
                        }}
                    >
                        <div style={{ fontSize: '14px', fontWeight: 500, marginBottom: '4px' }}>暂无 Agent Profile</div>
                        <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                            点击右上角“+ 新建 Profile”创建可调度的 Agent 引擎实例。
                        </div>
                    </div>
                ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                        {agentProfiles.value.map(profile => {
                            const provider = providers.value.find(item => item.id === profile.provider_id);
                            const reason = disabledReason(profile);
                            return (
                                <div key={profile.id} class="bento-card" style={{ padding: '14px 18px' }}>
                                    <div
                                        style={{
                                            display: 'flex',
                                            justifyContent: 'space-between',
                                            alignItems: 'center',
                                            gap: 12,
                                        }}
                                    >
                                        <div>
                                            <div
                                                style={{
                                                    fontSize: '15px',
                                                    fontWeight: 600,
                                                    color: 'var(--text-main)',
                                                    marginBottom: '4px',
                                                }}
                                            >
                                                {profileLabel(profile, provider?.name)}
                                            </div>
                                            <div
                                                style={{
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    gap: '8px',
                                                    fontSize: '12px',
                                                    color: 'var(--text-secondary)',
                                                }}
                                            >
                                                <span>revision {profile.revision}</span>
                                                <span>·</span>
                                                <span
                                                    class={`status-tag ${reason ? 'is-unavailable' : 'is-available'}`}
                                                >
                                                    {reason ? reason : '可用'}
                                                </span>
                                            </div>
                                        </div>
                                        <div style={{ display: 'flex', gap: '8px' }}>
                                            <button
                                                class="btn-secondary btn-sm"
                                                onClick={() =>
                                                    (editing.value = { ...profile, options: { ...profile.options } })
                                                }
                                            >
                                                编辑
                                            </button>
                                            {profile.status === 'archived' ? (
                                                <button
                                                    class="btn-secondary btn-sm"
                                                    style={{ color: 'var(--success-fg)' }}
                                                    onClick={() => void setStatus(profile, 'restore')}
                                                >
                                                    恢复
                                                </button>
                                            ) : (
                                                <button
                                                    class="btn-danger btn-sm"
                                                    onClick={() => void setStatus(profile, 'archive')}
                                                >
                                                    归档
                                                </button>
                                            )}
                                        </div>
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}

                {editing.value && (
                    <div class="ws-modal-overlay" onClick={() => (editing.value = null)}>
                        <div
                            class="ws-modal"
                            style={{ width: '460px', maxWidth: 'calc(100vw - 32px)' }}
                            onClick={(event: MouseEvent) => event.stopPropagation()}
                        >
                            <div class="ws-modal-header">
                                <span>{editing.value.id ? '编辑 Agent Profile' : '新建 Agent Profile'}</span>
                                <button class="ws-modal-close" onClick={() => (editing.value = null)}>
                                    ✕
                                </button>
                            </div>
                            <div class="ws-modal-body">
                                <div>
                                    <label class="ws-modal-label">ID *</label>
                                    <input
                                        class="ws-modal-input"
                                        value={editing.value.id}
                                        disabled={agentProfiles.value.some(item => item.id === editing.value?.id)}
                                        placeholder="如：claude-3-7-sonnet-custom"
                                        onInput={(event: Event) =>
                                            (editing.value = {
                                                ...editing.value!,
                                                id: (event.target as HTMLInputElement).value,
                                            })
                                        }
                                    />
                                </div>
                                <div>
                                    <label class="ws-modal-label">名称 *</label>
                                    <input
                                        class="ws-modal-input"
                                        value={editing.value.name}
                                        placeholder="如：Claude 3.7 Sonnet 开发引擎"
                                        onInput={(event: Event) =>
                                            (editing.value = {
                                                ...editing.value!,
                                                name: (event.target as HTMLInputElement).value,
                                            })
                                        }
                                    />
                                </div>
                                <div>
                                    <label class="ws-modal-label">Runtime *</label>
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
                                </div>
                                <div>
                                    <label class="ws-modal-label">Provider *</label>
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
                                        <option value="">请选择服务商</option>
                                        {compatibleProviders.map(item => (
                                            <option key={item.id} value={item.id}>
                                                {item.name}
                                            </option>
                                        ))}
                                    </select>
                                </div>
                                <div>
                                    <label class="ws-modal-label">Model *</label>
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
                                        <option value="">请选择模型 ID</option>
                                        {compatibleModels.map(item => (
                                            <option key={item.model_id} value={item.model_id}>
                                                {item.model_id}
                                            </option>
                                        ))}
                                    </select>
                                </div>
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
                                <button class="btn-secondary" onClick={() => (editing.value = null)}>
                                    取消
                                </button>
                                <button class="btn-primary" onClick={() => void save()}>
                                    保存 Profile
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}

function legacyFamily(endpoint: ProviderEndpointItem): 'openai' | 'anthropic' {
    return endpoint.agent_id === 'claude' ? 'anthropic' : 'openai';
}
