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
        <div class="bento-card">
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: '12px' }}>
                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                    <div
                        style={{
                            width: '36px',
                            height: '36px',
                            borderRadius: 'var(--bento-radius-sm)',
                            backgroundColor: 'var(--accent-light)',
                            color: 'var(--accent-color)',
                            display: 'flex',
                            alignItems: 'center',
                            justifyContent: 'center',
                            flexShrink: 0,
                        }}
                    >
                        <svg
                            width="18"
                            height="18"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                        >
                            <path d="M12 2a10 10 0 1 0 10 10A10 10 0 0 0 12 2zm0 18a8 8 0 1 1 8-8 8 8 0 0 1-8 8z" />
                            <path d="M12 6v6l4 2" />
                        </svg>
                    </div>
                    <div>
                        <div style={{ fontSize: '16px', fontWeight: 600, color: 'var(--text-main)' }}>{agentName}</div>
                        <div style={{ fontSize: '12px', color: 'var(--text-secondary)' }}>{agentDesc}</div>
                    </div>
                </div>
                <code
                    style={{
                        fontSize: '11.5px',
                        fontFamily: 'var(--font-mono)',
                        padding: '2px 8px',
                        borderRadius: '4px',
                        background: 'var(--bg-page)',
                        border: '1px solid var(--border-color)',
                        color: 'var(--text-secondary)',
                    }}
                >
                    {configPath}
                </code>
            </div>

            <div
                style={{
                    padding: '10px 12px',
                    borderRadius: 'var(--bento-radius-sm)',
                    backgroundColor: 'var(--bg-page)',
                    border: '1px solid var(--border-color)',
                    fontSize: '12.5px',
                    color: 'var(--text-secondary)',
                }}
            >
                <strong style={{ color: 'var(--text-main)' }}>当前本地配置：</strong>{' '}
                {runtime?.installed ? (
                    <span>
                        <code>{runtime.model_id || '未检测到模型'}</code>
                        {runtime.base_url ? ` · ${runtime.base_url}` : ''}
                    </span>
                ) : (
                    <span style={{ color: 'var(--text-muted)' }}>未发现配置文件</span>
                )}
                {runtime?.warnings?.map(warning => (
                    <div key={warning} style={{ color: 'var(--warning-fg)', marginTop: '4px' }}>
                        ⚠️ {warning}
                    </div>
                ))}
            </div>

            {/* Provider Selection */}
            <div>
                <label class="ws-modal-label">选择调用的服务商 *</label>
                <select
                    class="ws-modal-select"
                    value={selectedProviderId.value}
                    onChange={e => handleSelectProvider((e.target as HTMLSelectElement).value)}
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
                        padding: '14px',
                        borderRadius: 'var(--bento-radius-sm)',
                        backgroundColor: 'var(--bg-page)',
                        border: '1px solid var(--border-color)',
                    }}
                >
                    <div
                        style={{
                            display: 'flex',
                            justifyContent: 'space-between',
                            alignItems: 'center',
                            marginBottom: '10px',
                        }}
                    >
                        <span style={{ fontSize: '13px', fontWeight: 600, color: 'var(--text-main)' }}>
                            模型选择与微调 (Model Fine-Tuning)
                        </span>
                        {loadingModels.value && (
                            <span style={{ fontSize: '12px', color: 'var(--accent-color)' }}>正在拉取可用模型...</span>
                        )}
                    </div>

                    {/* If we fetched models from provider's endpoint, show select dropdown */}
                    {fetchedModels.value.length > 0 ? (
                        <div style={{ marginBottom: '10px' }}>
                            <label class="ws-modal-label">
                                从服务商端点自动拉取到的模型列表 ({fetchedModels.value.length} 个):
                            </label>
                            <select
                                class="ws-modal-select"
                                value={modelInput.value}
                                onChange={e => {
                                    const selectedModel = (e.target as HTMLSelectElement).value;
                                    modelInput.value = selectedModel;
                                    sonnetInput.value = selectedModel;
                                    haikuInput.value = selectedModel;
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
                            <label class="ws-modal-label">默认 Model ID:</label>
                            <input
                                type="text"
                                class="ws-modal-input"
                                value={modelInput.value}
                                onInput={e => (modelInput.value = (e.target as HTMLInputElement).value)}
                                placeholder="如：deepseek-chat 或 gpt-4o"
                            />
                        </div>
                    )}

                    {/* If Agent is Claude Code, allow fine-tuning Haiku/Sonnet/Opus mappings */}
                    {agentId === 'claude' && (
                        <div
                            style={{
                                display: 'grid',
                                gridTemplateColumns: '1fr 1fr',
                                gap: '10px',
                                marginTop: '10px',
                                paddingTop: '10px',
                                borderTop: '1px dashed var(--border-color)',
                            }}
                        >
                            <div>
                                <label class="ws-modal-label">Sonnet 映射</label>
                                <input
                                    type="text"
                                    class="ws-modal-input"
                                    value={sonnetInput.value}
                                    onInput={e => (sonnetInput.value = (e.target as HTMLInputElement).value)}
                                />
                            </div>
                            <div>
                                <label class="ws-modal-label">Haiku 映射</label>
                                <input
                                    type="text"
                                    class="ws-modal-input"
                                    value={haikuInput.value}
                                    onInput={e => (haikuInput.value = (e.target as HTMLInputElement).value)}
                                />
                            </div>
                        </div>
                    )}
                    {(optionSchema?.options || []).length > 0 && (
                        <div
                            style={{
                                marginTop: '10px',
                                paddingTop: '10px',
                                borderTop: '1px dashed var(--border-color)',
                            }}
                        >
                            <div class="ws-modal-label" style={{ marginBottom: '6px' }}>
                                智能体可选配置
                            </div>
                            {optionSchema?.options.map(option => {
                                if (option.depends_on && !optionValues.value[option.depends_on]) return null;
                                const value = optionValues.value[option.key] ?? option.default ?? '';
                                const updateValue = (nextValue: AgentOptionValue) => {
                                    optionValues.value = { ...optionValues.value, [option.key]: nextValue };
                                };
                                return (
                                    <div key={option.key} style={{ marginTop: '8px' }}>
                                        {option.type === 'boolean' ? (
                                            <label
                                                style={{
                                                    display: 'flex',
                                                    alignItems: 'center',
                                                    gap: '8px',
                                                    fontSize: '13px',
                                                    color: 'var(--text-main)',
                                                    cursor: 'pointer',
                                                }}
                                            >
                                                <input
                                                    type="checkbox"
                                                    checked={value === true}
                                                    onChange={e => updateValue((e.target as HTMLInputElement).checked)}
                                                />
                                                {option.label}
                                            </label>
                                        ) : option.type === 'select' ? (
                                            <div>
                                                <label class="ws-modal-label">{option.label}</label>
                                                <select
                                                    class="ws-modal-select"
                                                    value={String(value)}
                                                    onChange={e => updateValue((e.target as HTMLSelectElement).value)}
                                                >
                                                    {(option.choices || []).map(choice => (
                                                        <option key={choice} value={choice}>
                                                            {choice}
                                                        </option>
                                                    ))}
                                                </select>
                                            </div>
                                        ) : (
                                            <div>
                                                <label class="ws-modal-label">{option.label}</label>
                                                <input
                                                    class="ws-modal-input"
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
                                                />
                                            </div>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            )}

            {/* Action button & Status */}
            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginTop: '4px' }}>
                <span
                    style={{
                        fontSize: '13px',
                        color: statusMsg.value.includes('成功') ? 'var(--success-fg)' : 'var(--danger-fg)',
                    }}
                >
                    {statusMsg.value}
                </span>
                <button class="btn-primary" onClick={handleApply} disabled={submitting.value}>
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
                                <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2"></path>
                                <circle cx="9" cy="7" r="4"></circle>
                                <polyline points="16 11 18 13 22 9"></polyline>
                            </svg>
                            智能体绑定与切换看板 (Agent Switching)
                        </h2>
                        <p class="providers-header-desc">
                            每一个 Agent 独立选择调用的服务商，并针对具体 Agent 微调 Model ID 与配置。
                        </p>
                    </div>
                </div>

                {errorMsg.value && (
                    <div class="providers-alert-banner alert-danger">
                        <span>⚠️</span> {errorMsg.value}
                    </div>
                )}

                {loading.value ? (
                    <div style={{ padding: '32px', textAlign: 'center', color: 'var(--text-muted)' }}>
                        加载智能体与服务商列表中...
                    </div>
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
                            请先前往“服务商配置” Tab 添加并保存服务商。
                        </div>
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
        </div>
    );
}
