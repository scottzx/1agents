import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { ProviderItem } from './LlmProviderPanel';

interface AgentRuntimeStatus {
    agent_id: string;
    installed: boolean;
    config_path: string;
    base_url?: string;
    model_id?: string;
    warnings?: string[];
}

interface AgentBinding {
    agent_id: string;
    provider_id: string;
    model_id?: string;
    model_mapping?: Record<string, string>;
    options?: Record<string, unknown>;
}

interface ProviderModel {
    model_id: string;
    available: boolean;
}

type AgentOptionValue = boolean | number | string;

interface AgentOptionDefinition {
    key: string;
    type: 'boolean' | 'integer' | 'select' | string;
    label: string;
    default?: AgentOptionValue;
    choices?: string[];
    minimum?: number;
    depends_on?: string;
}

interface AgentOptionSchema {
    agent_id: string;
    options: AgentOptionDefinition[];
}

interface AgentCardProps {
    agentId: string;
    agentName: string;
    agentDesc: string;
    configPath: string;
    providers: ProviderItem[];
    runtime?: AgentRuntimeStatus;
    binding?: AgentBinding;
    optionSchema?: AgentOptionSchema;
    onSync: () => void;
}

function AgentCard({
    agentId,
    agentName,
    agentDesc,
    configPath,
    providers,
    runtime,
    binding,
    optionSchema,
    onSync,
}: AgentCardProps) {
    const selectedProviderId = useSignal<string>('');
    const modelInput = useSignal<string>('');
    const sonnetInput = useSignal<string>('');
    const haikuInput = useSignal<string>('');
    const opusInput = useSignal<string>('');
    const optionValues = useSignal<Record<string, AgentOptionValue>>({});
    const fetchedModels = useSignal<string[]>([]);
    const loadingModels = useSignal<boolean>(false);
    const submitting = useSignal<boolean>(false);
    const statusMsg = useSignal<string>('');

    // When provider is selected, initialize model defaults and try fetching models from endpoint
    const handleSelectProvider = async (pId: string, savedBinding?: AgentBinding) => {
        selectedProviderId.value = pId;
        const p = providers.find(item => item.id === pId);
        if (!p) return;

        modelInput.value = savedBinding?.model_id || p.model || '';
        sonnetInput.value = savedBinding?.model_mapping?.sonnet || p.sonnet_model || p.model || '';
        haikuInput.value = savedBinding?.model_mapping?.haiku || p.haiku_model || p.model || '';
        opusInput.value = savedBinding?.model_mapping?.opus || p.opus_model || p.model || '';
        optionValues.value = Object.fromEntries(
            (optionSchema?.options || []).map(option => [
                option.key,
                (savedBinding?.options?.[option.key] as AgentOptionValue | undefined) ?? option.default ?? '',
            ])
        );

        loadingModels.value = true;
        try {
            const catalogRes = await fetch(`/api/provider-models?provider_id=${encodeURIComponent(p.id)}`);
            if (catalogRes.ok) {
                const catalogData = await catalogRes.json();
                const catalogModels = ((catalogData.models || []) as ProviderModel[])
                    .filter(model => model.available)
                    .map(model => model.model_id);
                if (catalogModels.length > 0) {
                    fetchedModels.value = catalogModels;
                    return;
                }
            }

            const endpoint = p.endpoints?.find(item => item.agent_id === agentId);
            const targetUrl = endpoint?.base_url || p.base_url;
            if (!targetUrl) {
                fetchedModels.value = p.model_ids || [];
                return;
            }
            const res = await fetch('/api/providers/discover-models', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ provider_id: p.id, agent_id: agentId, base_url: targetUrl }),
            });
            if (!res.ok) {
                fetchedModels.value = p.model_ids || [];
                return;
            }
            const data = await res.json();
            fetchedModels.value = ((data.models || []) as ProviderModel[])
                .filter(model => model.available)
                .map(model => model.model_id);
        } catch {
            fetchedModels.value = p.model_ids || [];
        } finally {
            loadingModels.value = false;
        }
    };

    useEffect(() => {
        if (providers.length > 0 && !selectedProviderId.value) {
            const initialProviderID = binding?.provider_id || providers[0].id;
            void handleSelectProvider(initialProviderID, binding);
        }
    }, [providers, binding, optionSchema]);

    const handleApply = async () => {
        if (!selectedProviderId.value) {
            statusMsg.value = '请选择服务商';
            return;
        }
        submitting.value = true;
        statusMsg.value = '';
        try {
            const binding = {
                agent_id: agentId === 'claudecode' ? 'claude' : agentId,
                provider_id: selectedProviderId.value,
                model_id: modelInput.value,
                model_mapping:
                    agentId === 'claude'
                        ? {
                              sonnet: sonnetInput.value,
                              haiku: haikuInput.value,
                              opus: opusInput.value,
                          }
                        : undefined,
                options: optionSchema?.options.length ? optionValues.value : undefined,
            };
            const previewRes = await fetch('/api/agents/binding', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ binding, apply: false }),
            });
            if (!previewRes.ok) {
                const errData = await previewRes.json().catch(() => ({}));
                throw new Error(errData.error || `HTTP ${previewRes.status}`);
            }
            const preview = await previewRes.json();
            const paths = (preview.plan?.changes || []).map((change: { path: string }) => change.path);
            if (
                paths.length > 0 &&
                !confirm(`将修改以下配置文件，并自动创建备份：\n\n${paths.join('\n')}\n\n是否继续？`)
            )
                return;
            const res = await fetch('/api/agents/binding', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ binding, apply: true }),
            });
            if (!res.ok) {
                const errData = await res.json().catch(() => ({}));
                throw new Error(errData.error || `HTTP ${res.status}`);
            }
            statusMsg.value = `成功应用配置至 ${agentName}！`;
            setTimeout(() => (statusMsg.value = ''), 3000);
            onSync();
        } catch (err) {
            statusMsg.value = err instanceof Error ? err.message : String(err);
        } finally {
            submitting.value = false;
        }
    };

    const activeProvider = providers.find(p => p.id === selectedProviderId.value);

    return (
        <div
            style={{
                border: '1px solid var(--border, #e0e0e0)',
                borderRadius: '10px',
                padding: '18px',
                background: 'var(--bg-card, #fff)',
                boxShadow: '0 2px 8px rgba(0,0,0,0.04)',
                display: 'flex',
                flexDirection: 'column',
                gap: '14px',
            }}
        >
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <div>
                    <h3 style={{ margin: '0 0 4px', fontSize: '1.1rem', fontWeight: 600 }}>{agentName}</h3>
                    <p style={{ margin: 0, fontSize: '0.82rem', color: 'var(--fg-muted, #666)' }}>{agentDesc}</p>
                </div>
                <code
                    style={{
                        fontSize: '0.75rem',
                        padding: '2px 8px',
                        borderRadius: '4px',
                        background: 'var(--bg-subtle, #f5f5f5)',
                    }}
                >
                    {configPath}
                </code>
            </div>

            <div
                style={{
                    padding: '8px 10px',
                    borderRadius: '6px',
                    background: 'var(--bg-subtle, #f5f5f5)',
                    fontSize: '0.78rem',
                }}
            >
                <strong>当前本地配置：</strong>{' '}
                {runtime?.installed ? (
                    <span>
                        {runtime.model_id || '未检测到模型'}
                        {runtime.base_url ? ` · ${runtime.base_url}` : ''}
                    </span>
                ) : (
                    <span>未发现配置文件</span>
                )}
                {runtime?.warnings?.map(warning => (
                    <div key={warning} style={{ color: '#b26a00', marginTop: '4px' }}>
                        {warning}
                    </div>
                ))}
            </div>

            {/* Provider Selection */}
            <div>
                <label style={{ display: 'block', fontSize: '0.83rem', fontWeight: 600, marginBottom: '6px' }}>
                    选择调用的服务商 *
                </label>
                <select
                    value={selectedProviderId.value}
                    onChange={e => handleSelectProvider((e.target as HTMLSelectElement).value)}
                    style={{
                        width: '100%',
                        padding: '8px 10px',
                        borderRadius: '6px',
                        border: '1px solid var(--border, #ccc)',
                        fontWeight: 500,
                    }}
                >
                    {providers.map(p => (
                        <option key={p.id} value={p.id}>
                            {p.name} ({p.protocol === 'anthropic' ? 'Anthropic 协议' : 'OpenAI 协议'})
                        </option>
                    ))}
                </select>
            </div>

            {/* Model Fine-Tuning */}
            {activeProvider && (
                <div
                    style={{
                        padding: '12px',
                        borderRadius: '8px',
                        background: 'var(--bg-subtle, #f8f9fa)',
                        border: '1px solid var(--border, #e5e5e5)',
                    }}
                >
                    <div
                        style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            marginBottom: '8px',
                        }}
                    >
                        <span style={{ fontSize: '0.83rem', fontWeight: 600 }}>模型选择与微调 (Model Fine-Tuning)</span>
                        {loadingModels.value && (
                            <span style={{ fontSize: '0.75rem', color: '#0066cc' }}>正在拉取可用模型...</span>
                        )}
                    </div>

                    {/* If we fetched models from provider's endpoint, show select dropdown */}
                    {fetchedModels.value.length > 0 ? (
                        <div style={{ marginBottom: '10px' }}>
                            <label
                                style={{ display: 'block', fontSize: '0.78rem', marginBottom: '4px', color: '#555' }}
                            >
                                从服务商端点自动拉取到的模型列表 ({fetchedModels.value.length} 个):
                            </label>
                            <select
                                value={modelInput.value}
                                onChange={e => {
                                    const selectedModel = (e.target as HTMLSelectElement).value;
                                    modelInput.value = selectedModel;
                                    sonnetInput.value = selectedModel;
                                    haikuInput.value = selectedModel;
                                }}
                                style={{
                                    width: '100%',
                                    padding: '6px 8px',
                                    borderRadius: '4px',
                                    border: '1px solid #ccc',
                                    fontSize: '0.82rem',
                                }}
                            >
                                {fetchedModels.value.map(m => (
                                    <option key={m} value={m}>
                                        {m}
                                    </option>
                                ))}
                            </select>
                        </div>
                    ) : (
                        <div style={{ marginBottom: '10px' }}>
                            <label
                                style={{ display: 'block', fontSize: '0.78rem', marginBottom: '4px', color: '#555' }}
                            >
                                默认 Model ID:
                            </label>
                            <input
                                type="text"
                                value={modelInput.value}
                                onInput={e => (modelInput.value = (e.target as HTMLInputElement).value)}
                                placeholder="如：deepseek-chat 或 gpt-4o"
                                style={{
                                    width: '100%',
                                    padding: '6px 8px',
                                    borderRadius: '4px',
                                    border: '1px solid #ccc',
                                    fontSize: '0.82rem',
                                    boxSizing: 'border-box',
                                }}
                            />
                        </div>
                    )}

                    {/* If Agent is Claude Code, allow fine-tuning Haiku/Sonnet/Opus mappings */}
                    {agentId === 'claude' && (
                        <div
                            style={{
                                display: 'grid',
                                gridTemplateColumns: '1fr 1fr',
                                gap: '8px',
                                marginTop: '8px',
                                paddingTop: '8px',
                                borderTop: '1px dashed #ddd',
                            }}
                        >
                            <div>
                                <label style={{ display: 'block', fontSize: '0.75rem', color: '#555' }}>
                                    Sonnet 映射
                                </label>
                                <input
                                    type="text"
                                    value={sonnetInput.value}
                                    onInput={e => (sonnetInput.value = (e.target as HTMLInputElement).value)}
                                    style={{
                                        width: '100%',
                                        padding: '4px 6px',
                                        borderRadius: '4px',
                                        border: '1px solid #ccc',
                                        fontSize: '0.78rem',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>
                            <div>
                                <label style={{ display: 'block', fontSize: '0.75rem', color: '#555' }}>
                                    Haiku 映射
                                </label>
                                <input
                                    type="text"
                                    value={haikuInput.value}
                                    onInput={e => (haikuInput.value = (e.target as HTMLInputElement).value)}
                                    style={{
                                        width: '100%',
                                        padding: '4px 6px',
                                        borderRadius: '4px',
                                        border: '1px solid #ccc',
                                        fontSize: '0.78rem',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>
                        </div>
                    )}
                    {(optionSchema?.options || []).length > 0 && (
                        <div style={{ marginTop: '10px', paddingTop: '8px', borderTop: '1px dashed #ddd' }}>
                            <div style={{ fontSize: '0.78rem', fontWeight: 600, marginBottom: '6px' }}>
                                智能体可选配置
                            </div>
                            {optionSchema?.options.map(option => {
                                if (option.depends_on && !optionValues.value[option.depends_on]) return null;
                                const value = optionValues.value[option.key] ?? option.default ?? '';
                                const updateValue = (nextValue: AgentOptionValue) => {
                                    optionValues.value = { ...optionValues.value, [option.key]: nextValue };
                                };
                                return (
                                    <label
                                        key={option.key}
                                        style={{ display: 'block', fontSize: '0.75rem', marginTop: '6px' }}
                                    >
                                        {option.type === 'boolean' ? (
                                            <span style={{ display: 'flex', alignItems: 'center', gap: '8px' }}>
                                                <input
                                                    type="checkbox"
                                                    checked={value === true}
                                                    onChange={e => updateValue((e.target as HTMLInputElement).checked)}
                                                />
                                                {option.label}
                                            </span>
                                        ) : option.type === 'select' ? (
                                            <span>
                                                {option.label}
                                                <select
                                                    value={String(value)}
                                                    onChange={e => updateValue((e.target as HTMLSelectElement).value)}
                                                    style={{ width: '100%', marginTop: '4px', padding: '6px 8px' }}
                                                >
                                                    {(option.choices || []).map(choice => (
                                                        <option key={choice} value={choice}>
                                                            {choice}
                                                        </option>
                                                    ))}
                                                </select>
                                            </span>
                                        ) : (
                                            <span>
                                                {option.label}
                                                <input
                                                    type={option.type === 'integer' ? 'number' : 'text'}
                                                    min={option.minimum}
                                                    value={String(value)}
                                                    onInput={e => {
                                                        const input = e.target as HTMLInputElement;
                                                        updateValue(
                                                            option.type === 'integer'
                                                                ? Number(input.value)
                                                                : input.value
                                                        );
                                                    }}
                                                    style={{ width: '100%', marginTop: '4px', padding: '6px 8px' }}
                                                />
                                            </span>
                                        )}
                                    </label>
                                );
                            })}
                        </div>
                    )}
                </div>
            )}

            {/* Action button & Status */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <span style={{ fontSize: '0.82rem', color: statusMsg.value.includes('成功') ? '#2e7d32' : '#ff4d4f' }}>
                    {statusMsg.value}
                </span>
                <button
                    onClick={handleApply}
                    disabled={submitting.value}
                    style={{
                        padding: '8px 18px',
                        borderRadius: '6px',
                        background: 'var(--accent, #0066cc)',
                        color: '#fff',
                        border: 'none',
                        cursor: 'pointer',
                        fontWeight: 600,
                        fontSize: '0.85rem',
                    }}
                >
                    {submitting.value ? '写入中...' : `应用配置至 ${agentName}`}
                </button>
            </div>
        </div>
    );
}

export function AgentSwitchPanel() {
    const providers = useSignal<ProviderItem[]>([]);
    const runtimes = useSignal<AgentRuntimeStatus[]>([]);
    const bindings = useSignal<AgentBinding[]>([]);
    const optionSchemas = useSignal<AgentOptionSchema[]>([]);
    const loading = useSignal<boolean>(true);
    const errorMsg = useSignal<string>('');

    const loadProviders = async () => {
        loading.value = true;
        errorMsg.value = '';
        try {
            const [res, runtimeRes, optionsRes] = await Promise.all([
                fetch('/api/providers'),
                fetch('/api/agents/runtime'),
                fetch('/api/agents/options'),
            ]);
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            providers.value = data.providers || [];
            bindings.value = data.bindings || [];
            if (runtimeRes.ok) {
                const runtimeData = await runtimeRes.json();
                runtimes.value = runtimeData.agents || [];
            }
            if (optionsRes.ok) {
                const optionsData = await optionsRes.json();
                optionSchemas.value = optionsData.agents || [];
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

    return (
        <div style={{ padding: '16px', maxWidth: '900px', margin: '0 auto', color: 'var(--fg)' }}>
            <div style={{ marginBottom: '20px' }}>
                <h2 style={{ margin: '0 0 4px', fontSize: '1.25rem', fontWeight: 600 }}>智能体绑定与切换看板</h2>
                <p style={{ margin: 0, fontSize: '0.85rem', color: 'var(--fg-muted, #888)' }}>
                    每一个 Agent 独立选择调用的服务商，并针对具体 Agent 微调 Model ID。
                </p>
            </div>

            {errorMsg.value && (
                <div
                    style={{
                        padding: '10px 14px',
                        borderRadius: '6px',
                        background: 'rgba(255, 0, 0, 0.1)',
                        color: '#ff4d4f',
                        marginBottom: '16px',
                    }}
                >
                    {errorMsg.value}
                </div>
            )}

            {loading.value ? (
                <div style={{ padding: '32px', textAlign: 'center', color: '#888' }}>加载智能体与服务商列表中...</div>
            ) : providers.value.length === 0 ? (
                <div
                    style={{
                        padding: '32px',
                        textAlign: 'center',
                        border: '1px dashed var(--border, #ccc)',
                        borderRadius: '8px',
                    }}
                >
                    暂无已保存的服务商，请先前往“服务商配置库”添加并保存服务商。
                </div>
            ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '16px' }}>
                    <AgentCard
                        agentId="claude"
                        agentName="Claude Code"
                        agentDesc="Anthropic CLI AI 编程助手"
                        configPath="~/.claude/settings.json"
                        providers={providers.value}
                        runtime={runtimes.value.find(item => item.agent_id === 'claude')}
                        binding={bindings.value.find(item => item.agent_id === 'claude')}
                        optionSchema={optionSchemas.value.find(item => item.agent_id === 'claude')}
                        onSync={loadProviders}
                    />
                    <AgentCard
                        agentId="codex"
                        agentName="Codex CLI"
                        agentDesc="OpenAI 协议轻量级 CLI 工具"
                        configPath="~/.codex/config.toml"
                        providers={providers.value}
                        runtime={runtimes.value.find(item => item.agent_id === 'codex')}
                        binding={bindings.value.find(item => item.agent_id === 'codex')}
                        optionSchema={optionSchemas.value.find(item => item.agent_id === 'codex')}
                        onSync={loadProviders}
                    />
                </div>
            )}
        </div>
    );
}
