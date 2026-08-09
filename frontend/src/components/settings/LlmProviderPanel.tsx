import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignal } from '@preact/signals';

export interface ProviderItem {
    id: string;
    name: string;
    protocol: string; // 'openai' | 'anthropic' | 'gemini' | 'dual'
    base_url: string;
    anthropic_base_url?: string;
    openai_base_url?: string;
    api_key: string;
    model: string;
    model_ids?: string[];
    haiku_model?: string;
    sonnet_model?: string;
    opus_model?: string;
    created_at?: number;
    updated_at?: number;
}

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
    const loading = useSignal<boolean>(true);
    const errorMsg = useSignal<string>('');
    const successMsg = useSignal<string>('');

    // Modal form state
    const isModalOpen = useSignal<boolean>(false);
    const editingId = useSignal<string>('');
    const formName = useSignal<string>('');
    const formProtocol = useSignal<string>('dual');
    const formOpenAIBaseUrl = useSignal<string>('');
    const formAnthropicBaseUrl = useSignal<string>('');
    const formApiKey = useSignal<string>('');
    const formModel = useSignal<string>('');
    const fetchedModels = useSignal<string[]>([]);
    const fetchingModels = useSignal<boolean>(false);
    const saving = useSignal<boolean>(false);

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

    const openCreateModal = (preset?: (typeof PRESETS)[0]) => {
        editingId.value = '';
        fetchedModels.value = [];
        if (preset) {
            formName.value = preset.name;
            formProtocol.value = preset.protocol;
            formOpenAIBaseUrl.value = preset.openai_base_url || preset.base_url || '';
            formAnthropicBaseUrl.value = preset.anthropic_base_url || preset.base_url || '';
            formModel.value = preset.model;
            formApiKey.value = '';
        } else {
            formName.value = '';
            formProtocol.value = 'dual';
            formOpenAIBaseUrl.value = '';
            formAnthropicBaseUrl.value = '';
            formApiKey.value = '';
            formModel.value = '';
        }
        isModalOpen.value = true;
    };

    const openEditModal = (p: ProviderItem) => {
        editingId.value = p.id;
        fetchedModels.value = p.model_ids || [];
        formName.value = p.name;
        formProtocol.value = p.protocol || 'openai';
        formOpenAIBaseUrl.value = p.openai_base_url || p.base_url || '';
        formAnthropicBaseUrl.value = p.anthropic_base_url || p.base_url || '';
        formApiKey.value = p.api_key || '';
        formModel.value = p.model || '';
        isModalOpen.value = true;
    };

    // Auto fetch models from provider's endpoint
    const handleFetchModels = async () => {
        const targetUrl = formOpenAIBaseUrl.value || formAnthropicBaseUrl.value;
        if (!targetUrl) {
            errorMsg.value = '请先输入 Base URL';
            return;
        }
        fetchingModels.value = true;
        errorMsg.value = '';
        try {
            const res = await fetch('/api/providers/fetch-models', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ base_url: targetUrl, api_key: formApiKey.value }),
            });
            if (!res.ok) {
                const errData = await res.json().catch(() => ({}));
                throw new Error(errData.error || `HTTP ${res.status}`);
            }
            const data = await res.json();
            fetchedModels.value = data.models || [];
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
                base_url: formOpenAIBaseUrl.value || formAnthropicBaseUrl.value,
                openai_base_url: formOpenAIBaseUrl.value || undefined,
                anthropic_base_url: formAnthropicBaseUrl.value || undefined,
                api_key: formApiKey.value,
                model: formModel.value,
                model_ids: fetchedModels.value.length > 0 ? fetchedModels.value : undefined,
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

    return (
        <div style={{ padding: '16px', maxWidth: '900px', margin: '0 auto', color: 'var(--fg)' }}>
            <div
                style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: '16px' }}
            >
                <div>
                    <h2 style={{ margin: 0, fontSize: '1.25rem', fontWeight: 600 }}>
                        服务商配置库 (Provider Profiles)
                    </h2>
                    <p style={{ margin: '4px 0 0', fontSize: '0.85rem', color: 'var(--fg-muted, #888)' }}>
                        统一配置服务商的双端点 URL 与 API 凭证，支持自动拉取可用模型列表。
                    </p>
                </div>
                <button
                    onClick={() => openCreateModal()}
                    style={{
                        padding: '8px 16px',
                        borderRadius: '6px',
                        background: 'var(--accent, #0066cc)',
                        color: '#fff',
                        border: 'none',
                        cursor: 'pointer',
                        fontWeight: 500,
                    }}
                >
                    + 添加服务商
                </button>
            </div>

            {errorMsg.value && (
                <div
                    style={{
                        padding: '10px 14px',
                        borderRadius: '6px',
                        background: 'rgba(255, 0, 0, 0.1)',
                        color: '#ff4d4f',
                        marginBottom: '12px',
                    }}
                >
                    {errorMsg.value}
                </div>
            )}

            {successMsg.value && (
                <div
                    style={{
                        padding: '10px 14px',
                        borderRadius: '6px',
                        background: 'rgba(0, 200, 80, 0.1)',
                        color: '#2e7d32',
                        marginBottom: '12px',
                    }}
                >
                    {successMsg.value}
                </div>
            )}

            {/* Presets Row */}
            <div
                style={{
                    marginBottom: '20px',
                    padding: '12px',
                    borderRadius: '8px',
                    background: 'var(--bg-subtle, #f5f5f5)',
                }}
            >
                <div
                    style={{
                        fontSize: '0.85rem',
                        fontWeight: 600,
                        marginBottom: '8px',
                        color: 'var(--fg-muted, #666)',
                    }}
                >
                    快捷服务商预设模板
                </div>
                <div style={{ display: 'flex', gap: '8px', flexWrap: 'wrap' }}>
                    {PRESETS.map(preset => (
                        <button
                            key={preset.name}
                            onClick={() => openCreateModal(preset)}
                            style={{
                                padding: '6px 12px',
                                borderRadius: '16px',
                                border: '1px solid var(--border, #ccc)',
                                background: 'var(--bg-card, #fff)',
                                cursor: 'pointer',
                                fontSize: '0.82rem',
                            }}
                        >
                            + {preset.name}
                        </button>
                    ))}
                </div>
            </div>

            {/* Provider List */}
            {loading.value ? (
                <div style={{ padding: '24px', textAlign: 'center', color: '#888' }}>加载中...</div>
            ) : providers.value.length === 0 ? (
                <div
                    style={{
                        padding: '32px',
                        textAlign: 'center',
                        border: '1px dashed var(--border, #ccc)',
                        borderRadius: '8px',
                    }}
                >
                    暂无已保存的服务商，点击“+ 添加服务商”或上方预设添加。
                </div>
            ) : (
                <div style={{ display: 'flex', flexDirection: 'column', gap: '12px' }}>
                    {providers.value.map(p => (
                        <div
                            key={p.id}
                            style={{
                                padding: '16px',
                                borderRadius: '8px',
                                border: '1px solid var(--border, #ddd)',
                                background: 'var(--bg-card, #fff)',
                                display: 'flex',
                                justifyContent: 'space-between',
                                alignItems: 'center',
                            }}
                        >
                            <div style={{ flex: 1, marginRight: '16px' }}>
                                <div
                                    style={{
                                        display: 'flex',
                                        alignItems: 'center',
                                        gap: '8px',
                                        marginBottom: '6px',
                                    }}
                                >
                                    <span style={{ fontWeight: 600, fontSize: '1rem' }}>{p.name}</span>
                                    <span
                                        style={{
                                            fontSize: '0.75rem',
                                            padding: '2px 6px',
                                            borderRadius: '4px',
                                            background: 'var(--badge-bg, #eee)',
                                            textTransform: 'uppercase',
                                            fontWeight: 500,
                                        }}
                                    >
                                        {p.protocol === 'dual' ? '双端点 (OpenAI + Anthropic)' : `${p.protocol} 协议`}
                                    </span>
                                </div>

                                <div style={{ fontSize: '0.82rem', color: '#666', wordBreak: 'break-all' }}>
                                    {p.openai_base_url && (
                                        <div>
                                            OpenAI Base URL: <code>{p.openai_base_url}</code>
                                        </div>
                                    )}
                                    {p.anthropic_base_url && (
                                        <div>
                                            Anthropic Base URL: <code>{p.anthropic_base_url}</code>
                                        </div>
                                    )}
                                    {p.model && (
                                        <div>
                                            默认 Model: <code>{p.model}</code>
                                        </div>
                                    )}
                                </div>
                            </div>
                            <div style={{ display: 'flex', gap: '8px' }}>
                                <button
                                    onClick={() => openEditModal(p)}
                                    style={{
                                        padding: '6px 12px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        background: 'transparent',
                                        cursor: 'pointer',
                                        fontSize: '0.85rem',
                                    }}
                                >
                                    编辑配置
                                </button>
                                <button
                                    onClick={() => handleDelete(p.id, p.name)}
                                    style={{
                                        padding: '6px 12px',
                                        borderRadius: '6px',
                                        border: '1px solid #ff4d4f',
                                        color: '#ff4d4f',
                                        background: 'transparent',
                                        cursor: 'pointer',
                                        fontSize: '0.85rem',
                                    }}
                                >
                                    删除
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            {/* Modal Form */}
            {isModalOpen.value && (
                <div
                    style={{
                        position: 'fixed',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        background: 'rgba(0,0,0,0.5)',
                        display: 'flex',
                        justifyContent: 'center',
                        alignItems: 'center',
                        zIndex: 1000,
                    }}
                >
                    <div
                        style={{
                            background: 'var(--bg-modal, #fff)',
                            width: '100%',
                            maxWidth: '560px',
                            padding: '24px',
                            borderRadius: '12px',
                            boxShadow: '0 8px 30px rgba(0,0,0,0.2)',
                            maxHeight: '90vh',
                            overflowY: 'auto',
                        }}
                    >
                        <h3 style={{ marginTop: 0, marginBottom: '16px' }}>
                            {editingId.value ? '编辑服务商基本配置' : '新增服务商配置'}
                        </h3>
                        <div>
                            <div style={{ marginBottom: '12px' }}>
                                <label
                                    style={{
                                        display: 'block',
                                        fontSize: '0.85rem',
                                        fontWeight: 500,
                                        marginBottom: '4px',
                                    }}
                                >
                                    服务商名称 *
                                </label>
                                <input
                                    type="text"
                                    value={formName.value}
                                    onInput={e => (formName.value = (e.target as HTMLInputElement).value)}
                                    placeholder="如：DeepSeek API 或 SiliconFlow"
                                    required
                                    style={{
                                        width: '100%',
                                        padding: '8px 10px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>

                            {/* 双端点填写区 */}
                            <div style={{ marginBottom: '12px' }}>
                                <label
                                    style={{
                                        display: 'block',
                                        fontSize: '0.85rem',
                                        fontWeight: 500,
                                        marginBottom: '4px',
                                    }}
                                >
                                    OpenAI 兼容端口 (Base URL)
                                </label>
                                <input
                                    type="text"
                                    value={formOpenAIBaseUrl.value}
                                    onInput={e => (formOpenAIBaseUrl.value = (e.target as HTMLInputElement).value)}
                                    placeholder="https://api.deepseek.com/v1"
                                    style={{
                                        width: '100%',
                                        padding: '8px 10px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>

                            <div style={{ marginBottom: '12px' }}>
                                <label
                                    style={{
                                        display: 'block',
                                        fontSize: '0.85rem',
                                        fontWeight: 500,
                                        marginBottom: '4px',
                                    }}
                                >
                                    Anthropic 兼容端口 (Base URL)
                                </label>
                                <input
                                    type="text"
                                    value={formAnthropicBaseUrl.value}
                                    onInput={e => (formAnthropicBaseUrl.value = (e.target as HTMLInputElement).value)}
                                    placeholder="https://api.deepseek.com/beta"
                                    style={{
                                        width: '100%',
                                        padding: '8px 10px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>

                            <div style={{ marginBottom: '12px' }}>
                                <label
                                    style={{
                                        display: 'block',
                                        fontSize: '0.85rem',
                                        fontWeight: 500,
                                        marginBottom: '4px',
                                    }}
                                >
                                    API Key (通用鉴权密钥)
                                </label>
                                <input
                                    type="password"
                                    value={formApiKey.value}
                                    onInput={e => (formApiKey.value = (e.target as HTMLInputElement).value)}
                                    placeholder="sk-..."
                                    style={{
                                        width: '100%',
                                        padding: '8px 10px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        boxSizing: 'border-box',
                                    }}
                                />
                            </div>

                            {/* 默认 Model & 自动拉取按钮 */}
                            <div style={{ marginBottom: '16px' }}>
                                <div
                                    style={{
                                        display: 'flex',
                                        justifyContent: 'space-between',
                                        alignItems: 'center',
                                        marginBottom: '4px',
                                    }}
                                >
                                    <label style={{ fontSize: '0.85rem', fontWeight: 500 }}>默认 Model ID</label>
                                    <button
                                        type="button"
                                        onClick={handleFetchModels}
                                        disabled={fetchingModels.value}
                                        style={{
                                            fontSize: '0.78rem',
                                            color: '#0066cc',
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
                                        value={formModel.value}
                                        onChange={e => (formModel.value = (e.target as HTMLSelectElement).value)}
                                        style={{
                                            width: '100%',
                                            padding: '8px 10px',
                                            borderRadius: '6px',
                                            border: '1px solid var(--border, #ccc)',
                                            boxSizing: 'border-box',
                                        }}
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
                                        value={formModel.value}
                                        onInput={e => (formModel.value = (e.target as HTMLInputElement).value)}
                                        placeholder="如：deepseek-chat 或点击“自动获取模型列表”"
                                        style={{
                                            width: '100%',
                                            padding: '8px 10px',
                                            borderRadius: '6px',
                                            border: '1px solid var(--border, #ccc)',
                                            boxSizing: 'border-box',
                                        }}
                                    />
                                )}
                            </div>

                            <div style={{ display: 'flex', justifyContent: 'flex-end', gap: '8px' }}>
                                <button
                                    type="button"
                                    onClick={() => (isModalOpen.value = false)}
                                    style={{
                                        padding: '8px 14px',
                                        borderRadius: '6px',
                                        border: '1px solid var(--border, #ccc)',
                                        background: 'transparent',
                                        cursor: 'pointer',
                                    }}
                                >
                                    取消
                                </button>
                                <button
                                    type="button"
                                    disabled={saving.value}
                                    onClick={handleSaveConfig}
                                    style={{
                                        padding: '8px 18px',
                                        borderRadius: '6px',
                                        background: 'var(--accent, #0066cc)',
                                        color: '#fff',
                                        border: 'none',
                                        cursor: 'pointer',
                                        fontWeight: 600,
                                    }}
                                >
                                    {saving.value ? '保存中...' : '保存服务商配置'}
                                </button>
                            </div>
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
