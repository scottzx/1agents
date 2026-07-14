/**
 * Project configuration — 指令 (instructions) | 连接器 (MCP connectors) |
 * 专家 (experts) | 技能 (skills) | 自动化 (automation).
 *
 * Two surfaces share the same section bodies + load/save hook:
 *   - ProjectConfigPanel — the drawer opened from the 项目管理 artifact column's
 *     配置 gear (panel variant).
 *   - ProjectConfigView  — a single section rendered inline as a top-level tab
 *     in the 项目详情 (detail variant); the 5 sub-tabs live in the main tab bar.
 *
 * Fetches config from GET /api/project/{id}/config, saves via PUT. Degrades
 * gracefully when the endpoint is unavailable (empty defaults).
 */

import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import type { App } from '../app';
import { t, type Lang } from '../i18n';
import { getProjectConfig, putProjectConfig, type ProjectConfig } from '../../services/appManifestService';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as stage from '../../stores/stageStore';
import { DetailSection } from '../shared/primitives';
import { SkillsTab } from '../pages/SkillsTab';

export type ConfigTab = 'instructions' | 'connectors' | 'experts' | 'skills' | 'automation';

export const PROJECT_CONFIG_TABS: Array<{ id: ConfigTab; label: string }> = [
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

/** Shared load/save state for a project's config. */
function useProjectConfig(workspaceId: string) {
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

    const save = async (onDone?: () => void) => {
        setSaving(true);
        const ok = await putProjectConfig(workspaceId, config);
        setSaving(false);
        if (ok) {
            ui.showToast('项目配置已保存');
            onDone?.();
        } else {
            ui.showToast('保存失败，请重试');
        }
    };

    return { config, setConfig, loading, saving, save };
}

/** Danger zone: archive or delete the project. Isolated at the bottom of config. */
function DangerZone({ workspaceId, onDone }: { workspaceId: string; onDone: () => void }) {
    const [confirming, setConfirming] = useState<'archive' | 'delete' | null>(null);

    const handleArchive = async () => {
        await wsStore.archiveWorkspace(workspaceId);
        onDone();
    };

    const handleDelete = async () => {
        await wsStore.deleteWorkspace(workspaceId);
        stage.projectOverview();
        onDone();
    };

    return (
        <DetailSection title="危险操作" danger={true}>
            <div class="project-config-danger-actions">
                {confirming === 'archive' ? (
                    <Fragment>
                        <span class="project-config-danger-confirm">确认归档项目？</span>
                        <button class="assistant-btn assistant-btn-danger" onClick={() => void handleArchive()}>
                            确认归档
                        </button>
                        <button class="assistant-btn assistant-btn-ghost" onClick={() => setConfirming(null)}>
                            取消
                        </button>
                    </Fragment>
                ) : confirming === 'delete' ? (
                    <Fragment>
                        <span class="project-config-danger-confirm">删除后无法恢复，请确认</span>
                        <button class="assistant-btn assistant-btn-danger" onClick={() => void handleDelete()}>
                            确认删除
                        </button>
                        <button class="assistant-btn assistant-btn-ghost" onClick={() => setConfirming(null)}>
                            取消
                        </button>
                    </Fragment>
                ) : (
                    <Fragment>
                        <button class="assistant-btn assistant-btn-ghost" onClick={() => setConfirming('archive')}>
                            归档项目
                        </button>
                        <button
                            class="assistant-btn assistant-btn-ghost assistant-btn-danger"
                            onClick={() => setConfirming('delete')}
                        >
                            删除项目
                        </button>
                    </Fragment>
                )}
            </div>
        </DetailSection>
    );
}

/** The form body for one config section (pure — driven by config + setConfig). */
function ConfigSectionBody({
    section,
    config,
    setConfig,
}: {
    section: ConfigTab;
    config: ProjectConfig;
    setConfig: (c: ProjectConfig) => void;
}) {
    if (section === 'instructions') {
        return (
            <div class="project-config-section">
                <label class="project-config-label">
                    系统指令
                    <span class="project-config-label-hint">定义此项目内 AI 助理的角色、职责和行为规范</span>
                </label>
                <textarea
                    class="project-config-textarea"
                    value={config.instructions}
                    onInput={e => setConfig({ ...config, instructions: (e.target as HTMLTextAreaElement).value })}
                    placeholder="例如：你是一个专业的产品经理助理，负责管理此项目的需求拆解和任务分配…"
                    rows={10}
                />
            </div>
        );
    }
    if (section === 'connectors') {
        return (
            <div class="project-config-section">
                <p class="project-config-empty-hint">
                    连接器配置由安装的应用提供。在「探索」中安装应用后，其 MCP 工具会自动出现在此项目的 AI 上下文中。
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
        );
    }
    if (section === 'experts') {
        return (
            <div class="project-config-section">
                <p class="project-config-empty-hint">专家是拥有特定角色和指令的 AI 助理实例，可在任务中派遣。</p>
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
        );
    }
    if (section === 'skills') {
        return (
            <div class="project-config-section">
                <p class="project-config-empty-hint">技能是可复用的工作流程模板，此项目内的任务可直接调用。</p>
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
        );
    }
    // automation
    return (
        <div class="project-config-section">
            <label class="project-config-label">
                自动化规则
                <span class="project-config-label-hint">定义触发条件和自动执行的动作（JSON 格式）</span>
            </label>
            <textarea
                class="project-config-textarea project-config-textarea-mono"
                value={config.automation}
                onInput={e => setConfig({ ...config, automation: (e.target as HTMLTextAreaElement).value })}
                placeholder='{"trigger": "task.created", "action": "assign_to_agent"}'
                rows={10}
            />
        </div>
    );
}

/** Inline single-section config, rendered as a top-level tab in 项目详情. */
export function ProjectConfigView({
    workspaceId,
    section,
    app,
    language,
}: {
    workspaceId: string;
    section: ConfigTab;
    /** Required when section === 'skills' — the skills tab reuses SkillsTab,
     *  which needs the App for FilePreviewPane. Other sections ignore it. */
    app?: App;
    language?: Lang;
}) {
    // Skills reuse the shared SkillsTab (same surface as the 助理 detail) —
    // it's fully service-driven (add/remove/push/pull) and bypasses the
    // project-config save flow. We pass a 项目总览 parent crumb so the
    // drill-in breadcrumb reads [项目总览, <project>, <skill>] — same shape
    // the 助理 detail uses ([助理, <name>, <skill>]).
    if (section === 'skills' && app && language) {
        return (
            <SkillsTab
                workspaceId={workspaceId}
                app={app}
                language={language}
                crumbsParent={{
                    label: t('projectHome.title', language),
                    onClick: stage.projectOverview,
                }}
            />
        );
    }
    const { config, setConfig, loading, saving, save } = useProjectConfig(workspaceId);
    return (
        <div class="project-config-inline">
            {loading ? (
                <div class="project-config-loading">
                    <div class="fb-loading-spinner" />
                </div>
            ) : (
                <Fragment>
                    <ConfigSectionBody section={section} config={config} setConfig={setConfig} />
                    <div class="project-config-inline-actions">
                        <button
                            class="assistant-btn assistant-btn-primary"
                            onClick={() => void save()}
                            disabled={saving}
                        >
                            {saving ? '保存中…' : '保存'}
                        </button>
                    </div>
                </Fragment>
            )}
        </div>
    );
}

interface ProjectConfigPanelProps {
    workspaceId: string;
    onClose: () => void;
}

/** The drawer (panel variant) — its own sub-tabs + save/cancel footer. */
export function ProjectConfigPanel({ workspaceId, onClose }: ProjectConfigPanelProps) {
    const [activeTab, setActiveTab] = useState<ConfigTab>('instructions');
    const { config, setConfig, loading, saving, save } = useProjectConfig(workspaceId);

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
                    {PROJECT_CONFIG_TABS.map(tab => (
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
                            <ConfigSectionBody section={activeTab} config={config} setConfig={setConfig} />
                            <div class="project-config-danger-zone">
                                <DangerZone workspaceId={workspaceId} onDone={onClose} />
                            </div>
                        </Fragment>
                    )}
                </div>

                <div class="project-config-footer">
                    <button class="project-config-btn-cancel" onClick={onClose} disabled={saving}>
                        取消
                    </button>
                    <button
                        class="project-config-btn-save"
                        onClick={() => void save(onClose)}
                        disabled={saving || loading}
                    >
                        {saving ? '保存中…' : '保存'}
                    </button>
                </div>
            </div>
        </div>
    );
}
