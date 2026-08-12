import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';

export interface ProviderItem {
    id: string;
    name: string;
    protocol: string; // 'openai' | 'anthropic' | 'dual'
    base_url: string;
    anthropic_base_url?: string;
    openai_base_url?: string;
    api_key: string;
    has_api_key?: boolean;
    model: string;
    model_ids?: string[];
    haiku_model?: string;
    sonnet_model?: string;
    opus_model?: string;
    created_at?: number;
    updated_at?: number;
    endpoints?: ProviderEndpointItem[];
    status?: string;
}

export interface ProviderEndpointItem {
    family?: 'openai' | 'anthropic';
    /** schema v3 compatibility only. */
    agent_id?: string;
    protocol: string;
    base_url: string;
    api_key?: string;
    has_api_key?: boolean;
    models_endpoint?: string;
    headers?: Record<string, string>;
    has_headers?: boolean;
    header_names?: string[];
}

interface ProviderModelItem {
    provider_id: string;
    model_id: string;
    source: string;
    available: boolean;
    discovered_at?: number;
    last_seen_at?: number;
}

const ENDPOINT_TYPES = [
    { id: 'anthropic', label: 'Anthropic', protocol: 'anthropic_messages' },
    { id: 'openai', label: 'OpenAI', protocol: 'openai_responses' },
];

const endpointFamily = (endpoint: ProviderEndpointItem): 'openai' | 'anthropic' =>
    endpoint.family || (endpoint.agent_id === 'claude' ? 'anthropic' : 'openai');

const formatTimestamp = (timestamp?: number) => (timestamp ? new Date(timestamp * 1000).toLocaleString() : '尚未刷新');

interface LlmProviderPanelProps {
    language?: string;
    ccProvidersUrl?: string;
    panelStyle?: string | Record<string, string | number>;
}

const PRESETS = [
    {
        name: 'DeepSeek API',
        protocol: 'dual',
        base_url: 'https://api.deepseek.com/v1',
        openai_base_url: 'https://api.deepseek.com/v1',
        anthropic_base_url: 'https://api.deepseek.com/beta',
        model: 'deepseek-chat',
        sonnet_model: 'deepseek-chat',
        haiku_model: 'deepseek-chat',
        opus_model: 'deepseek-reasoner',
    },
    {
        name: 'SiliconFlow (硅基流动)',
        protocol: 'openai',
        base_url: 'https://api.siliconflow.cn/v1',
        openai_base_url: 'https://api.siliconflow.cn/v1',
        model: 'deepseek-ai/DeepSeek-V3',
    },
    {
        name: 'OpenRouter',
        protocol: 'openai',
        base_url: 'https://openrouter.ai/api/v1',
        openai_base_url: 'https://openrouter.ai/api/v1',
        model: 'anthropic/claude-3.7-sonnet',
    },
    {
        name: 'Anthropic Direct (Claude 官方端点)',
        protocol: 'anthropic',
        base_url: 'https://api.anthropic.com',
        anthropic_base_url: 'https://api.anthropic.com',
        model: 'claude-3-7-sonnet-20250219',
        sonnet_model: 'claude-3-7-sonnet-20250219',
        haiku_model: 'claude-3-5-haiku-20241022',
        opus_model: 'claude-3-opus-20240229',
    },
];

export function LlmProviderPanel(props: LlmProviderPanelProps) {
    const { language } = props;
    void language;
    const providers = useSignal<ProviderItem[]>([]);
    const models = useSignal<ProviderModelItem[]>([]);
    const loading = useSignal<boolean>(true);
    const errorMsg = useSignal<string>('');
    const successMsg = useSignal<string>('');

    // Modal form state
    const isModalOpen = useSignal<boolean>(false);
    const editingId = useSignal<string>('');
    const formName = useSignal<string>('');
    const formProtocol = useSignal<string>('dual');
    const formApiKey = useSignal<string>('');
    const formEndpoints = useSignal<ProviderEndpointItem[]>([]);
    const formModel = useSignal<string>('');
    const fetchedModels = useSignal<string[]>([]);
    const fetchingModels = useSignal<boolean>(false);
    const saving = useSignal<boolean>(false);

    const loadProviders = async () => {
        loading.value = true;
        errorMsg.value = '';
        try {
            const [res, modelsRes] = await Promise.all([fetch('/api/providers'), fetch('/api/provider-models')]);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            providers.value = data.providers || [];
            if (modelsRes.ok) {
                const modelData = await modelsRes.json();
                models.value = modelData.models || [];
            }
        } catch (err) {
            errorMsg.value = err instanceof Error ? err.message : String(err);
        } finally {
            loading.value = false;
        }
    };

    useEffect(() => {
        loadProviders();
    }, []);

    const openCreateModal = (preset?: (typeof PRESETS)[0]) => {
        editingId.value = '';
        fetchedModels.value = [];
        if (preset) {
            formName.value = preset.name;
            formProtocol.value = preset.protocol;
            formModel.value = preset.model;
            formApiKey.value = '';
            const endpoints: ProviderEndpointItem[] = [];
            if (preset.anthropic_base_url) {
                endpoints.push({
                    family: 'anthropic',
                    protocol: 'anthropic_messages',
                    base_url: preset.anthropic_base_url,
                });
            }
            if (preset.openai_base_url) {
                endpoints.push({
                    family: 'openai',
                    protocol: 'openai_responses',
                    base_url: preset.openai_base_url,
                });
            }
            formEndpoints.value = endpoints;
        } else {
            formName.value = '';
            formProtocol.value = 'dual';
            formApiKey.value = '';
            formEndpoints.value = [];
            formModel.value = '';
        }
        isModalOpen.value = true;
    };

    const openEditModal = (p: ProviderItem) => {
        editingId.value = p.id;
        fetchedModels.value = p.model_ids || [];
        formName.value = p.name;
        formProtocol.value = p.protocol || 'openai';
        formApiKey.value = p.api_key || '';
        const endpoints: ProviderEndpointItem[] = p.endpoints?.length ? [...p.endpoints] : [];
        if (!p.endpoints?.length && p.anthropic_base_url) {
            endpoints.push({
                family: 'anthropic',
                protocol: 'anthropic_messages',
                base_url: p.anthropic_base_url,
            });
        }
        if (!p.endpoints?.length && p.openai_base_url) {
            endpoints.push({ family: 'openai', protocol: 'openai_responses', base_url: p.openai_base_url });
        }
        formEndpoints.value = endpoints
            .filter(endpoint => ENDPOINT_TYPES.some(type => type.id === endpointFamily(endpoint)))
            .map(endpoint => ({
                ...endpoint,
                api_key: '',
                headers: Object.fromEntries((endpoint.header_names || []).map(name => [name, ''])),
            }));
        formModel.value = p.model || '';
        isModalOpen.value = true;
    };

    // Auto fetch models from provider's endpoint
    const handleFetchModels = async (target?: ProviderEndpointItem) => {
        if (!target?.base_url) {
            errorMsg.value = '请先输入 Base URL';
            return;
        }
        fetchingModels.value = true;
        errorMsg.value = '';
        try {
            const res = await fetch(
                editingId.value ? '/api/providers/discover-models' : '/api/providers/fetch-models',
                {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(
                        editingId.value
                            ? {
                                  provider_id: editingId.value,
                                  family: endpointFamily(target),
                                  base_url: target.base_url,
                              }
                            : {
                                  base_url: target.base_url,
                                  api_key: target.api_key || formApiKey.value,
                              }
                    ),
                }
            );
            if (!res.ok) {
                const errData = await res.json().catch(() => ({}));
                throw new Error(errData.error || `HTTP ${res.status}`);
            }
            const data = await res.json();
            fetchedModels.value = (data.models || []).map((model: string | ProviderModelItem) =>
                typeof model === 'string' ? model : model.model_id
            );
            if (fetchedModels.value.length > 0 && !formModel.value) {
                formModel.value = fetchedModels.value[0];
            }
            successMsg.value = `获取成功！共拉取到 ${fetchedModels.value.length} 个模型。`;
            setTimeout(() => (successMsg.value = ''), 2500);
        } catch (err) {
            errorMsg.value = err instanceof Error ? err.message : String(err);
        } finally {
            fetchingModels.value = false;
        }
    };

    const handleSaveConfig = async () => {
        if (!formName.value) {
            errorMsg.value = '请填写提供商名称';
            return;
        }
        saving.value = true;
        errorMsg.value = '';
        try {
            const payload: Partial<ProviderItem> = {
                id: editingId.value || undefined,
                name: formName.value,
                protocol: formProtocol.value,
                base_url: formEndpoints.value.find(endpoint => endpoint.base_url)?.base_url || '',
                openai_base_url: formEndpoints.value.find(endpoint => endpointFamily(endpoint) === 'openai')?.base_url,
                anthropic_base_url: formEndpoints.value.find(endpoint => endpointFamily(endpoint) === 'anthropic')
                    ?.base_url,
                api_key: formApiKey.value,
                model: formModel.value,
                model_ids: fetchedModels.value.length > 0 ? fetchedModels.value : undefined,
                endpoints: formEndpoints.value.map(endpoint => ({
                    family: endpointFamily(endpoint),
                    protocol: endpoint.protocol,
                    base_url: endpoint.base_url,
                    api_key: endpoint.api_key || undefined,
                    models_endpoint: endpoint.models_endpoint || undefined,
                    headers: endpoint.headers,
                })),
            };
            const res = await fetch('/api/providers', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload),
            });
            if (!res.ok) {
                const errData = await res.json().catch(() => ({}));
                throw new Error(errData.error || `HTTP ${res.status}`);
            }
            isModalOpen.value = false;
            successMsg.value = '服务商配置已保存至 ~/.1agents/providers.json';
            setTimeout(() => (successMsg.value = ''), 2500);
            await loadProviders();
        } catch (err) {
            errorMsg.value = err instanceof Error ? err.message : String(err);
        } finally {
            saving.value = false;
        }
    };

    const handleDelete = async (id: string, name: string) => {
        if (!confirm(`确定要删除提供商 "${name}" 吗？`)) return;
        try {
            const res = await fetch(`/api/providers?id=${encodeURIComponent(id)}`, {
                method: 'DELETE',
            });
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            await loadProviders();
        } catch (err) {
            errorMsg.value = err instanceof Error ? err.message : String(err);
        }
    };

    const updateEndpoint = (index: number, patch: Partial<ProviderEndpointItem>) => {
        formEndpoints.value = formEndpoints.value.map((endpoint, currentIndex) =>
            currentIndex === index ? { ...endpoint, ...patch } : endpoint
        );
    };

    const addEndpoint = () => {
        const nextType = ENDPOINT_TYPES.find(
            type => !formEndpoints.value.some(endpoint => endpointFamily(endpoint) === type.id)
        );
        if (!nextType) return;
        formEndpoints.value = [
            ...formEndpoints.value,
            { family: nextType.id as 'openai' | 'anthropic', protocol: nextType.protocol, base_url: '', headers: {} },
        ];
    };

    const changeEndpointType = (index: number, family: string) => {
        const type = ENDPOINT_TYPES.find(item => item.id === family);
        updateEndpoint(index, {
            family: family as 'openai' | 'anthropic',
            agent_id: undefined,
            protocol: type?.protocol || 'openai_chat',
        });
    };

    const updateEndpointHeader = (endpointIndex: number, previousName: string, name: string, value: string) => {
        const headers = { ...(formEndpoints.value[endpointIndex].headers || {}) };
        if (previousName !== name) delete headers[previousName];
        if (name) headers[name] = value;
        updateEndpoint(endpointIndex, { headers });
    };

    const editingProvider = providers.value.find(provider => provider.id === editingId.value);

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
                                <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
                                <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
                                <line x1="6" y1="6" x2="6.01" y2="6"></line>
                                <line x1="6" y1="18" x2="6.01" y2="18"></line>
                            </svg>
                            服务商配置库 (Provider Profiles)
                        </h2>
                        <p class="providers-header-desc">
                            统一配置服务商的双端点 URL 与 API 凭证，支持自动拉取可用模型列表。
                        </p>
                    </div>
                    <button class="btn-primary" onClick={() => openCreateModal()}>
                        + 添加服务商
                    </button>
                </div>

                {errorMsg.value && (
                    <div class="providers-alert-banner alert-danger">
                        <span>⚠️</span> {errorMsg.value}
                    </div>
                )}

                {successMsg.value && (
                    <div class="providers-alert-banner alert-success">
                        <span>✅</span> {successMsg.value}
                    </div>
                )}

                {/* Presets Row */}
                <div class="providers-presets-card">
                    <div class="providers-presets-title">快捷服务商预设模板</div>
                    <div class="providers-presets-list">
                        {PRESETS.map(preset => (
                            <button key={preset.name} class="preset-chip-btn" onClick={() => openCreateModal(preset)}>
                                ⚡ + {preset.name}
                            </button>
                        ))}
                    </div>
                </div>

                {/* Provider List */}
                {loading.value ? (
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)' }}>加载中...</div>
                ) : providers.value.length === 0 ? (
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
                        <div style={{ fontSize: '14px', fontWeight: 500, marginBottom: '4px' }}>暂无已保存的服务商</div>
                        <div style={{ fontSize: '12px', color: 'var(--text-muted)' }}>
                            点击“+ 添加服务商”或选择上方快捷预设创建服务商配置。
                        </div>
                    </div>
                ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: '14px' }}>
                        {providers.value.map(p => (
                            <div key={p.id} class="provider-bento-card">
                                <div class="provider-card-header">
                                    <div class="provider-card-identity">
                                        <div class="provider-icon-box">
                                            <svg
                                                width="20"
                                                height="20"
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                            >
                                                <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                                            </svg>
                                        </div>
                                        <div>
                                            <div class="provider-title">{p.name}</div>
                                        </div>
                                    </div>
                                    <div style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                                        <span class="provider-protocol-badge">
                                            {p.protocol === 'dual'
                                                ? '双端点 (OpenAI + Anthropic)'
                                                : `${p.protocol} 协议`}
                                        </span>
                                        <button class="btn-secondary btn-sm" onClick={() => openEditModal(p)}>
                                            编辑配置
                                        </button>
                                        <button class="btn-danger btn-sm" onClick={() => handleDelete(p.id, p.name)}>
                                            删除
                                        </button>
                                    </div>
                                </div>

                                <div class="provider-endpoints-box">
                                    {(p.endpoints || []).map(endpoint => (
                                        <div key={endpointFamily(endpoint)}>
                                            <strong style={{ color: 'var(--text-main)', textTransform: 'capitalize' }}>
                                                {endpointFamily(endpoint)}:
                                            </strong>{' '}
                                            <code>{endpoint.base_url}</code> · {endpoint.protocol}
                                            {endpoint.has_api_key ? ' · 已保存独立 Key' : ''}
                                        </div>
                                    ))}
                                    {p.model && (
                                        <div>
                                            <strong style={{ color: 'var(--text-main)' }}>默认 Model:</strong>{' '}
                                            <code>{p.model}</code>
                                        </div>
                                    )}
                                </div>

                                <div class="provider-models-summary">
                                    <span>
                                        模型目录：
                                        <strong style={{ color: 'var(--success-fg)' }}>
                                            {
                                                models.value.filter(
                                                    model => model.provider_id === p.id && model.available
                                                ).length
                                            }
                                        </strong>{' '}
                                        可用 /{' '}
                                        <strong style={{ color: 'var(--text-muted)' }}>
                                            {
                                                models.value.filter(
                                                    model => model.provider_id === p.id && !model.available
                                                ).length
                                            }
                                        </strong>{' '}
                                        不可用
                                    </span>
                                </div>

                                {models.value.some(model => model.provider_id === p.id) && (
                                    <details class="provider-models-details">
                                        <summary>
                                            查看模型目录明细 (
                                            {models.value.filter(model => model.provider_id === p.id).length})
                                        </summary>
                                        <div class="provider-models-grid">
                                            {models.value
                                                .filter(model => model.provider_id === p.id)
                                                .map(model => (
                                                    <div key={model.model_id} class="provider-model-item">
                                                        <code>{model.model_id}</code>
                                                        <span style={{ color: 'var(--text-muted)' }}>
                                                            {model.source || 'unknown'}
                                                        </span>
                                                        <span
                                                            class={`status-tag ${model.available ? 'is-available' : 'is-unavailable'}`}
                                                        >
                                                            {model.available ? '可用' : '不可用'} ·{' '}
                                                            {formatTimestamp(model.last_seen_at || model.discovered_at)}
                                                        </span>
                                                    </div>
                                                ))}
                                        </div>
                                    </details>
                                )}
                            </div>
                        ))}
                    </div>
                )}

                {/* Modal Form */}
                {isModalOpen.value && (
                    <div class="ws-modal-overlay" onClick={() => (isModalOpen.value = false)}>
                        <div
                            class="ws-modal"
                            style={{ width: '560px', maxWidth: 'calc(100vw - 32px)', maxHeight: '90vh' }}
                            onClick={e => e.stopPropagation()}
                        >
                            <div class="ws-modal-header">
                                <span>{editingId.value ? '编辑服务商配置' : '新增服务商配置'}</span>
                                <button class="ws-modal-close" onClick={() => (isModalOpen.value = false)}>
                                    ✕
                                </button>
                            </div>
                            <div class="ws-modal-body" style={{ overflowY: 'auto', maxHeight: 'calc(90vh - 120px)' }}>
                                <div>
                                    <label class="ws-modal-label">服务商名称 *</label>
                                    <input
                                        type="text"
                                        class="ws-modal-input"
                                        value={formName.value}
                                        onInput={e => (formName.value = (e.target as HTMLInputElement).value)}
                                        placeholder="如：DeepSeek API 或 SiliconFlow"
                                        required
                                    />
                                </div>

                                <div>
                                    <label class="ws-modal-label">API Key (通用鉴权密钥)</label>
                                    <input
                                        type="password"
                                        class="ws-modal-input"
                                        value={formApiKey.value}
                                        onInput={e => (formApiKey.value = (e.target as HTMLInputElement).value)}
                                        placeholder={
                                            editingProvider?.has_api_key ? '已保存，留空表示保留原密钥' : 'sk-...'
                                        }
                                    />
                                </div>

                                <div style={{ marginTop: '4px' }}>
                                    <div
                                        style={{
                                            display: 'flex',
                                            justifyContent: 'space-between',
                                            alignItems: 'center',
                                            marginBottom: '8px',
                                        }}
                                    >
                                        <span class="ws-modal-label">协议 Endpoints</span>
                                        <button
                                            type="button"
                                            class="btn-secondary btn-sm"
                                            onClick={addEndpoint}
                                            disabled={formEndpoints.value.length >= ENDPOINT_TYPES.length}
                                        >
                                            + 添加 Endpoint
                                        </button>
                                    </div>
                                    {formEndpoints.value.length === 0 && (
                                        <div
                                            style={{
                                                padding: '12px',
                                                border: '1px dashed var(--border-color)',
                                                borderRadius: '6px',
                                                fontSize: '12px',
                                                color: 'var(--text-muted)',
                                            }}
                                        >
                                            请至少添加一个 OpenAI 或 Anthropic Endpoint。
                                        </div>
                                    )}
                                    {formEndpoints.value.map((endpoint, endpointIndex) => (
                                        <div
                                            key={`${endpointFamily(endpoint)}-${endpointIndex}`}
                                            style={{
                                                border: '1px solid var(--border-color)',
                                                borderRadius: '8px',
                                                padding: '12px',
                                                marginBottom: '10px',
                                                backgroundColor: 'var(--bg-page)',
                                            }}
                                        >
                                            <div
                                                style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: '8px' }}
                                            >
                                                <div>
                                                    <label class="ws-modal-label">协议类型</label>
                                                    <select
                                                        class="ws-modal-select"
                                                        value={endpointFamily(endpoint)}
                                                        onChange={e =>
                                                            changeEndpointType(
                                                                endpointIndex,
                                                                (e.target as HTMLSelectElement).value
                                                            )
                                                        }
                                                    >
                                                        {ENDPOINT_TYPES.map(type => (
                                                            <option
                                                                key={type.id}
                                                                value={type.id}
                                                                disabled={formEndpoints.value.some(
                                                                    (item, index) =>
                                                                        index !== endpointIndex &&
                                                                        endpointFamily(item) === type.id
                                                                )}
                                                            >
                                                                {type.label}
                                                            </option>
                                                        ))}
                                                    </select>
                                                </div>
                                                <div>
                                                    <label class="ws-modal-label">Wire Protocol</label>
                                                    <input
                                                        class="ws-modal-input"
                                                        value={endpoint.protocol}
                                                        onInput={e =>
                                                            updateEndpoint(endpointIndex, {
                                                                protocol: (e.target as HTMLInputElement).value,
                                                            })
                                                        }
                                                    />
                                                </div>
                                            </div>

                                            <div style={{ marginTop: '8px' }}>
                                                <label class="ws-modal-label">Base URL</label>
                                                <input
                                                    class="ws-modal-input"
                                                    value={endpoint.base_url}
                                                    onInput={e =>
                                                        updateEndpoint(endpointIndex, {
                                                            base_url: (e.target as HTMLInputElement).value,
                                                        })
                                                    }
                                                />
                                            </div>

                                            <div style={{ marginTop: '8px' }}>
                                                <label class="ws-modal-label">Models Endpoint（可选）</label>
                                                <input
                                                    class="ws-modal-input"
                                                    value={endpoint.models_endpoint || ''}
                                                    onInput={e =>
                                                        updateEndpoint(endpointIndex, {
                                                            models_endpoint: (e.target as HTMLInputElement).value,
                                                        })
                                                    }
                                                    placeholder="https://api.example.com/v1/models"
                                                />
                                            </div>

                                            <div style={{ marginTop: '8px' }}>
                                                <label class="ws-modal-label">独立 API Key（可选）</label>
                                                <input
                                                    type="password"
                                                    class="ws-modal-input"
                                                    value={endpoint.api_key || ''}
                                                    onInput={e =>
                                                        updateEndpoint(endpointIndex, {
                                                            api_key: (e.target as HTMLInputElement).value,
                                                        })
                                                    }
                                                    placeholder={
                                                        endpoint.has_api_key ? '已保存，留空保留' : '留空使用通用密钥'
                                                    }
                                                />
                                            </div>

                                            <details style={{ marginTop: '8px' }}>
                                                <summary
                                                    style={{
                                                        cursor: 'pointer',
                                                        fontSize: '12px',
                                                        color: 'var(--accent-color)',
                                                    }}
                                                >
                                                    Custom Headers ({Object.keys(endpoint.headers || {}).length})
                                                </summary>
                                                {Object.entries(endpoint.headers || {}).map(([name, value]) => (
                                                    <div
                                                        key={name}
                                                        style={{
                                                            display: 'grid',
                                                            gridTemplateColumns: '1fr 1fr auto',
                                                            gap: '6px',
                                                            marginTop: '6px',
                                                        }}
                                                    >
                                                        <input
                                                            class="ws-modal-input"
                                                            style={{ height: '32px' }}
                                                            value={name}
                                                            onInput={e =>
                                                                updateEndpointHeader(
                                                                    endpointIndex,
                                                                    name,
                                                                    (e.target as HTMLInputElement).value,
                                                                    value
                                                                )
                                                            }
                                                            aria-label="Header Name"
                                                        />
                                                        <input
                                                            type="password"
                                                            class="ws-modal-input"
                                                            style={{ height: '32px' }}
                                                            value={value}
                                                            onInput={e =>
                                                                updateEndpointHeader(
                                                                    endpointIndex,
                                                                    name,
                                                                    name,
                                                                    (e.target as HTMLInputElement).value
                                                                )
                                                            }
                                                            placeholder={
                                                                endpoint.header_names?.includes(name)
                                                                    ? '已保存，留空保留'
                                                                    : 'Header Value'
                                                            }
                                                            aria-label={`${name} Header Value`}
                                                        />
                                                        <button
                                                            type="button"
                                                            class="btn-danger btn-sm"
                                                            onClick={() => {
                                                                const headers = { ...(endpoint.headers || {}) };
                                                                delete headers[name];
                                                                updateEndpoint(endpointIndex, { headers });
                                                            }}
                                                        >
                                                            删除
                                                        </button>
                                                    </div>
                                                ))}
                                                <button
                                                    type="button"
                                                    class="btn-secondary btn-sm"
                                                    style={{ marginTop: '6px' }}
                                                    onClick={() => {
                                                        const headers = { ...(endpoint.headers || {}) };
                                                        let name = 'X-Custom-Header';
                                                        let suffix = 2;
                                                        while (name in headers) name = `X-Custom-Header-${suffix++}`;
                                                        headers[name] = '';
                                                        updateEndpoint(endpointIndex, { headers });
                                                    }}
                                                >
                                                    + Header
                                                </button>
                                            </details>

                                            <div
                                                style={{
                                                    display: 'flex',
                                                    justifyContent: 'space-between',
                                                    marginTop: '10px',
                                                }}
                                            >
                                                <button
                                                    type="button"
                                                    class="btn-secondary btn-sm"
                                                    onClick={() => handleFetchModels(endpoint)}
                                                    disabled={fetchingModels.value || !endpoint.base_url}
                                                >
                                                    刷新该 Endpoint 模型
                                                </button>
                                                <button
                                                    type="button"
                                                    class="btn-danger btn-sm"
                                                    onClick={() =>
                                                        (formEndpoints.value = formEndpoints.value.filter(
                                                            (_, index) => index !== endpointIndex
                                                        ))
                                                    }
                                                >
                                                    移除 Endpoint
                                                </button>
                                            </div>
                                        </div>
                                    ))}
                                </div>

                                {/* 默认 Model & 自动拉取按钮 */}
                                <div>
                                    <div
                                        style={{
                                            display: 'flex',
                                            justifyContent: 'space-between',
                                            alignItems: 'center',
                                            marginBottom: '4px',
                                        }}
                                    >
                                        <label class="ws-modal-label">默认 Model ID</label>
                                        <button
                                            type="button"
                                            onClick={() =>
                                                handleFetchModels(
                                                    formEndpoints.value.find(endpoint => endpoint.base_url)
                                                )
                                            }
                                            disabled={fetchingModels.value}
                                            style={{
                                                fontSize: '12px',
                                                color: 'var(--accent-color)',
                                                background: 'transparent',
                                                border: 'none',
                                                cursor: 'pointer',
                                                textDecoration: 'underline',
                                            }}
                                        >
                                            {fetchingModels.value ? '拉取中...' : '自动获取模型列表'}
                                        </button>
                                    </div>

                                    {fetchedModels.value.length > 0 ? (
                                        <select
                                            class="ws-modal-select"
                                            value={formModel.value}
                                            onChange={e => (formModel.value = (e.target as HTMLSelectElement).value)}
                                        >
                                            {fetchedModels.value.map(m => (
                                                <option key={m} value={m}>
                                                    {m}
                                                </option>
                                            ))}
                                        </select>
                                    ) : (
                                        <input
                                            type="text"
                                            class="ws-modal-input"
                                            value={formModel.value}
                                            onInput={e => (formModel.value = (e.target as HTMLInputElement).value)}
                                            placeholder="如：deepseek-chat 或点击“自动获取模型列表”"
                                        />
                                    )}
                                </div>
                            </div>
                            <div class="ws-modal-footer">
                                <button type="button" class="btn-secondary" onClick={() => (isModalOpen.value = false)}>
                                    取消
                                </button>
                                <button
                                    type="button"
                                    class="btn-primary"
                                    disabled={saving.value}
                                    onClick={handleSaveConfig}
                                >
                                    {saving.value ? '保存中...' : '保存服务商配置'}
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
