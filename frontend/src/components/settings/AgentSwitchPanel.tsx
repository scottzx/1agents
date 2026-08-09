import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { ProviderItem } from './LlmProviderPanel';

interface AgentCardProps {
    agentId: string;
    agentName: string;
    agentDesc: string;
    configPath: string;
    providers: ProviderItem[];
    onSync: () => void;
}

function AgentCard({ agentId, agentName, agentDesc, configPath, providers, onSync }: AgentCardProps) {
    const selectedProviderId = useSignal<string>('');
    const modelInput = useSignal<string>('');
    const sonnetInput = useSignal<string>('');
    const haikuInput = useSignal<string>('');
    const opusInput = useSignal<string>('');
    const fetchedModels = useSignal<string[]>([]);
    const loadingModels = useSignal<boolean>(false);
    const submitting = useSignal<boolean>(false);
    const statusMsg = useSignal<string>('');

    // When provider is selected, initialize model defaults and try fetching models from endpoint
    const handleSelectProvider = async (pId: string) => {
        selectedProviderId.value = pId;
        const p = providers.find(item => item.id === pId);
        if (!p) return;

        modelInput.value = p.model || '';
        sonnetInput.value = p.sonnet_model || p.model || '';
        haikuInput.value = p.haiku_model || p.model || '';
        opusInput.value = p.opus_model || p.model || '';

        // Use saved model_ids if present, otherwise fetch from endpoint
        if (p.model_ids && p.model_ids.length > 0) {
            fetchedModels.value = p.model_ids;
        } else {
            const targetUrl = p.openai_base_url || p.base_url || p.anthropic_base_url;
            if (targetUrl) {
                loadingModels.value = true;
                try {
                    const res = await fetch('/api/providers/fetch-models', {
                        method: 'POST',
                        headers: { 'Content-Type': 'application/json' },
                        body: JSON.stringify({ base_url: targetUrl, api_key: p.api_key }),
                    });
                    if (res.ok) {
                        const data = await res.json();
                        fetchedModels.value = data.models || [];
                    } else {
                        fetchedModels.value = [];
                    }
                } catch {
                    fetchedModels.value = [];
                } finally {
                    loadingModels.value = false;
                }
            } else {
                fetchedModels.value = [];
            }
        }
    };

    useEffect(() => {
        if (providers.length > 0 && !selectedProviderId.value) {
            handleSelectProvider(providers[0].id);
        }
    }, [providers]);

    const handleApply = async () => {
        if (!selectedProviderId.value) {
            statusMsg.value = '请选择服务商';
            return;
        }
        submitting.value = true;
        statusMsg.value = '';
        try {
            const res = await fetch('/api/agents/switch', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    agent: agentId,
                    provider_id: selectedProviderId.value,
                    model: modelInput.value,
                    sonnet_model: sonnetInput.value,
                    haiku_model: haikuInput.value,
                    opus_model: opusInput.value,
                }),
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
    const loading = useSignal<boolean>(true);
    const errorMsg = useSignal<string>('');

    const loadProviders = async () => {
        loading.value = true;
        errorMsg.value = '';
        try {
            const res = await fetch('/api/providers');
            if (!res.ok) throw new Error(`HTTP ${res.status}`);
            const data = await res.json();
            providers.value = data.providers || [];
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
                        configPath="~/.claude.json"
                        providers={providers.value}
                        onSync={loadProviders}
                    />
                    <AgentCard
                        agentId="codex"
                        agentName="Codex CLI"
                        agentDesc="OpenAI 协议轻量级 CLI 工具"
                        configPath="~/.codex/config.toml"
                        providers={providers.value}
                        onSync={loadProviders}
                    />
                    <AgentCard
                        agentId="gemini"
                        agentName="Gemini CLI"
                        agentDesc="Google Gemini 命令行 Agent"
                        configPath="~/.config/gemini/config.json"
                        providers={providers.value}
                        onSync={loadProviders}
                    />
                </div>
            )}
        </div>
    );
}
