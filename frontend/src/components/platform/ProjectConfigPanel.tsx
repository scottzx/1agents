/**
 * ProjectConfigPanel — per-project configuration drawer.
 *
 * Covers: 指令 (instructions) | 连接器 (MCP connectors) | 专家 (experts) |
 *         技能 (skills) | 自动化 (automation).
 *
 * Fetches config from GET /api/project/{id}/config and saves via
 * PUT /api/project/{id}/config. Degrades gracefully when the endpoint
 * is unavailable (shows the form with empty defaults).
 */

import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import { getProjectConfig, putProjectConfig, type ProjectConfig } from '../../services/appManifestService';
import * as ui from '../../stores/uiStore';

interface ProjectConfigPanelProps {
    workspaceId: string;
    onClose: () => void;
}

type ConfigTab = 'instructions' | 'connectors' | 'experts' | 'skills' | 'automation';

const CONFIG_TABS: Array<{ id: ConfigTab; label: string }> = [
    { id: 'instructions', label: '指令' },
    { id: 'connectors', label: '连接器' },
    { id: 'experts', label: '专家' },
    { id: 'skills', label: '技能' },
    { id: 'automation', label: '自动化' },
];

const DEFAULT_CONFIG: ProjectConfig = {
    workspaceId: '',
    instructions: '',
    connectors: [],
    experts: [],
    skills: [],
    automation: '',
};

export function ProjectConfigPanel({ workspaceId, onClose }: ProjectConfigPanelProps) {
    const [activeTab, setActiveTab] = useState<ConfigTab>('instructions');
    const [config, setConfig] = useState<ProjectConfig>({ ...DEFAULT_CONFIG, workspaceId });
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        setLoading(true);
        getProjectConfig(workspaceId)
            .then(c => {
                if (c) setConfig(c);
            })
            .finally(() => setLoading(false));
    }, [workspaceId]);

    const handleSave = async () => {
        setSaving(true);
        const ok = await putProjectConfig(workspaceId, config);
        setSaving(false);
        if (ok) {
            ui.showToast('项目配置已保存');
            onClose();
        } else {
            ui.showToast('保存失败，请重试');
        }
    };

    return (
        <div class="project-config-overlay" onClick={e => e.target === e.currentTarget && onClose()}>
            <div class="project-config-panel">
                <div class="project-config-header">
                    <span class="project-config-title">项目配置</span>
                    <button class="project-config-close" onClick={onClose} aria-label="关闭">
                        <svg
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2.5"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <line x1="18" y1="6" x2="6" y2="18" />
                            <line x1="6" y1="6" x2="18" y2="18" />
                        </svg>
                    </button>
                </div>

                <div class="project-config-tabs">
                    {CONFIG_TABS.map(tab => (
                        <button
                            key={tab.id}
                            class={`project-config-tab${activeTab === tab.id ? ' is-active' : ''}`}
                            onClick={() => setActiveTab(tab.id)}
                        >
                            {tab.label}
                        </button>
                    ))}
                </div>

                <div class="project-config-body">
                    {loading ? (
                        <div class="project-config-loading">
                            <div class="fb-loading-spinner" />
                        </div>
                    ) : (
                        <Fragment>
                            {activeTab === 'instructions' && (
                                <div class="project-config-section">
                                    <label class="project-config-label">
                                        系统指令
                                        <span class="project-config-label-hint">
                                            定义此项目内 AI 助理的角色、职责和行为规范
                                        </span>
                                    </label>
                                    <textarea
                                        class="project-config-textarea"
                                        value={config.instructions}
                                        onInput={e =>
                                            setConfig({
                                                ...config,
                                                instructions: (e.target as HTMLTextAreaElement).value,
                                            })
                                        }
                                        placeholder="例如：你是一个专业的产品经理助理，负责管理此项目的需求拆解和任务分配…"
                                        rows={10}
                                    />
                                </div>
                            )}

                            {activeTab === 'connectors' && (
                                <div class="project-config-section">
                                    <p class="project-config-empty-hint">
                                        连接器配置由安装的应用提供。在「探索」中安装应用后，其 MCP
                                        工具会自动出现在此项目的 AI 上下文中。
                                    </p>
                                    {config.connectors.length > 0 ? (
                                        <ul class="project-config-list">
                                            {config.connectors.map(c => (
                                                <li key={c} class="project-config-list-item">
                                                    <span class="project-config-connector-dot" />
                                                    {c}
                                                </li>
                                            ))}
                                        </ul>
                                    ) : (
                                        <div class="project-config-placeholder-row">暂无连接器</div>
                                    )}
                                </div>
                            )}

                            {activeTab === 'experts' && (
                                <div class="project-config-section">
                                    <p class="project-config-empty-hint">
                                        专家是拥有特定角色和指令的 AI 助理实例，可在任务中派遣。
                                    </p>
                                    {config.experts.length > 0 ? (
                                        <ul class="project-config-list">
                                            {config.experts.map(e => (
                                                <li key={e.id} class="project-config-list-item">
                                                    <strong>{e.name}</strong>
                                                    <span class="project-config-expert-role">{e.role}</span>
                                                </li>
                                            ))}
                                        </ul>
                                    ) : (
                                        <div class="project-config-placeholder-row">暂无专家配置</div>
                                    )}
                                </div>
                            )}

                            {activeTab === 'skills' && (
                                <div class="project-config-section">
                                    <p class="project-config-empty-hint">
                                        技能是可复用的工作流程模板，此项目内的任务可直接调用。
                                    </p>
                                    {config.skills.length > 0 ? (
                                        <ul class="project-config-list">
                                            {config.skills.map(s => (
                                                <li key={s} class="project-config-list-item">
                                                    {s}
                                                </li>
                                            ))}
                                        </ul>
                                    ) : (
                                        <div class="project-config-placeholder-row">暂无技能</div>
                                    )}
                                </div>
                            )}

                            {activeTab === 'automation' && (
                                <div class="project-config-section">
                                    <label class="project-config-label">
                                        自动化规则
                                        <span class="project-config-label-hint">
                                            定义触发条件和自动执行的动作（JSON 格式）
                                        </span>
                                    </label>
                                    <textarea
                                        class="project-config-textarea project-config-textarea-mono"
                                        value={config.automation}
                                        onInput={e =>
                                            setConfig({
                                                ...config,
                                                automation: (e.target as HTMLTextAreaElement).value,
                                            })
                                        }
                                        placeholder='{"trigger": "task.created", "action": "assign_to_agent"}'
                                        rows={10}
                                    />
                                </div>
                            )}
                        </Fragment>
                    )}
                </div>

                <div class="project-config-footer">
                    <button class="project-config-btn-cancel" onClick={onClose} disabled={saving}>
                        取消
                    </button>
                    <button class="project-config-btn-save" onClick={handleSave} disabled={saving || loading}>
                        {saving ? '保存中…' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}
