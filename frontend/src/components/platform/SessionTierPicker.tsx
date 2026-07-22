/**
 * SessionTierPicker (#328 / Epic #184 · #191) — creation-wizard entry selector.
 *
 * UX tiers (NOT persisted as projects.kind — see 名称定义表 §0.4):
 *   助理           — creates kind=workforce
 *   项目           — creates kind=project (no template scaffold)
 *   template_project — creates kind=project + template scaffold (app/experts/…)
 *
 * Historical ids generic-project / professional-project are not used as wire
 * values and must not be written to projects.kind.
 *
 * Usage: render at the top of the new-chat / project-creation entry flow.
 * When the user picks a tier, call the appropriate on* handler.
 */

import { h } from 'preact';
import { useState } from 'preact/hooks';
import type { Workspace } from '../types';
import type { AppManifest } from '../../services/appManifestService';
import * as appStore from '../../stores/appManifestStore';

// ── Types ─────────────────────────────────────────────────────────────────────

/** Creation-wizard tier ids. Never persisted as Workspace.kind. */
export type SessionTier = 'assistant' | 'project' | 'template_project';

/** @deprecated Use SessionTier. Kept so older imports type-check during rename. */
export type CreationWizardTier = SessionTier;

interface SessionTierPickerProps {
    workspaces: Workspace[];
    /** 助理 → kind=workforce */
    onSelectAssistant: () => void;
    /** 普通项目 → kind=project, no template */
    onSelectProject: (workspace: Workspace) => void;
    /** template_project → kind=project + scaffold(template) */
    onSelectTemplateProject: (workspace: Workspace, appTemplate: AppManifest) => void;
    /** Called to open the folder/workspace picker modal. */
    onOpenFolderPicker: () => void;
    /**
     * @deprecated Prefer onSelectProject. Same behavior as onSelectProject.
     */
    onSelectGenericProject?: (workspace: Workspace) => void;
    /**
     * @deprecated Prefer onSelectTemplateProject.
     */
    onSelectProfessionalProject?: (workspace: Workspace, appTemplate: AppManifest) => void;
}

// ── Tier definitions ──────────────────────────────────────────────────────────

interface TierDef {
    id: SessionTier;
    title: string;
    subtitle: string;
    icon: h.JSX.Element;
}

const TIER_DEFS: TierDef[] = [
    {
        id: 'assistant',
        title: '助理',
        subtitle: '直接对话 · 随时派任务 · 无需选项目',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            </svg>
        ),
    },
    {
        id: 'project',
        title: '项目',
        subtitle: '选择文件夹 · 直接对话 · 轻量协作',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
            </svg>
        ),
    },
    {
        id: 'template_project',
        title: '模板项目',
        subtitle: '应用模板 · 完整任务管理 · 专家注入',
        icon: (
            <svg
                viewBox="0 0 24 24"
                fill="none"
                stroke="currentColor"
                stroke-width="1.5"
                stroke-linecap="round"
                stroke-linejoin="round"
            >
                <rect x="2" y="3" width="20" height="14" rx="2" />
                <line x1="8" y1="21" x2="16" y2="21" />
                <line x1="12" y1="17" x2="12" y2="21" />
            </svg>
        ),
    },
];

// ── Component ─────────────────────────────────────────────────────────────────

export function SessionTierPicker({
    workspaces,
    onSelectAssistant,
    onSelectProject,
    onSelectTemplateProject,
    onOpenFolderPicker,
    onSelectGenericProject,
    onSelectProfessionalProject,
}: SessionTierPickerProps) {
    const selectProject = onSelectProject ?? onSelectGenericProject;
    const selectTemplate = onSelectTemplateProject ?? onSelectProfessionalProject;

    const [tier, setTier] = useState<SessionTier>('assistant');
    const [selectedWsId, setSelectedWsId] = useState<string>(workspaces[0]?.id ?? '');
    const [selectedAppId, setSelectedAppId] = useState<string>('');

    const apps = appStore.enabledApps.value;
    const selectedWs = workspaces.find(w => w.id === selectedWsId);
    const selectedApp = apps.find(a => a.id === selectedAppId);

    const handleConfirm = () => {
        if (tier === 'assistant') {
            onSelectAssistant();
        } else if (tier === 'project') {
            if (selectedWs && selectProject) selectProject(selectedWs);
        } else if (tier === 'template_project') {
            if (selectedWs && selectedApp && selectTemplate) {
                selectTemplate(selectedWs, selectedApp);
            }
        }
    };

    const canConfirm =
        tier === 'assistant' ||
        (tier === 'project' && !!selectedWs && !!selectProject) ||
        (tier === 'template_project' && !!selectedWs && !!selectedApp && !!selectTemplate);

    return (
        <div class="session-tier-picker">
            <h2 class="session-tier-picker-title">选择工作模式</h2>
            <p class="session-tier-picker-subtitle">根据工作性质选择合适的入口</p>

            <div class="session-tier-grid">
                {TIER_DEFS.map(def => (
                    <button
                        key={def.id}
                        class={`session-tier-card bento-card${tier === def.id ? ' is-active' : ''}`}
                        onClick={() => setTier(def.id)}
                    >
                        <div class="session-tier-card-icon">{def.icon}</div>
                        <div class="session-tier-card-body">
                            <span class="session-tier-card-title">{def.title}</span>
                            <span class="session-tier-card-subtitle">{def.subtitle}</span>
                        </div>
                        {tier === def.id && (
                            <div class="session-tier-card-check">
                                <svg viewBox="0 0 16 16" fill="currentColor">
                                    <path d="M13.78 4.22a.75.75 0 0 1 0 1.06l-7.25 7.25a.75.75 0 0 1-1.06 0L2.22 9.28a.75.75 0 0 1 1.06-1.06L6 10.94l6.72-6.72a.75.75 0 0 1 1.06 0z" />
                                </svg>
                            </div>
                        )}
                    </button>
                ))}
            </div>

            {(tier === 'project' || tier === 'template_project') && (
                <div class="session-tier-config">
                    <div class="session-tier-config-row">
                        <label class="session-tier-config-label">工作区</label>
                        <div class="session-tier-config-controls">
                            <select
                                class="session-tier-select"
                                value={selectedWsId}
                                onChange={e => setSelectedWsId((e.target as HTMLSelectElement).value)}
                            >
                                {workspaces.map(ws => (
                                    <option key={ws.id} value={ws.id}>
                                        {ws.name}
                                    </option>
                                ))}
                            </select>
                            <button class="session-tier-btn-folder" onClick={onOpenFolderPicker} title="打开文件夹">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                                </svg>
                                打开文件夹
                            </button>
                        </div>
                    </div>

                    {tier === 'template_project' && (
                        <div class="session-tier-config-row">
                            <label class="session-tier-config-label">应用模板</label>
                            {apps.length === 0 ? (
                                <p class="session-tier-no-apps">
                                    暂无已安装的应用。前往「探索」安装专业应用后，可在此选择模板。
                                </p>
                            ) : (
                                <div class="session-tier-app-grid">
                                    {apps.map(app => (
                                        <button
                                            key={app.id}
                                            class={`session-tier-app-card${selectedAppId === app.id ? ' is-selected' : ''}`}
                                            onClick={() => setSelectedAppId(app.id)}
                                        >
                                            <span class="session-tier-app-name">{app.name}</span>
                                            <span class="session-tier-app-version">v{app.version}</span>
                                        </button>
                                    ))}
                                </div>
                            )}
                        </div>
                    )}
                </div>
            )}

            <div class="session-tier-actions">
                <button class="session-tier-btn-confirm" onClick={handleConfirm} disabled={!canConfirm}>
                    {tier === 'assistant' ? '开始对话' : tier === 'project' ? '进入项目' : '用模板创建项目'}
                </button>
            </div>
        </div>
    );
}
