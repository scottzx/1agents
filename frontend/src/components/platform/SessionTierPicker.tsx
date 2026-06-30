/**
 * SessionTierPicker (#328) — three-tier session/project entry selector.
 *
 * Tiers:
 *   助理      — default workspace, direct chat, no project management
 *   通用项目  — pick an existing folder/workspace, chat directly, no app mounted
 *   专业项目  — workspace + app template + full ProjectShell (task management)
 *
 * This component renders the tier-selection UI as a step before creating a
 * new chat or project. It integrates into the new-chat home flow alongside
 * the existing NewChatHome component by wrapping it.
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

export type SessionTier = 'assistant' | 'generic-project' | 'professional-project';

interface SessionTierPickerProps {
    workspaces: Workspace[];
    /** Called when user confirms the assistant tier. */
    onSelectAssistant: () => void;
    /** Called when user confirms a generic project (folder already selected). */
    onSelectGenericProject: (workspace: Workspace) => void;
    /** Called when user confirms a professional project with an app template. */
    onSelectProfessionalProject: (workspace: Workspace, appTemplate: AppManifest) => void;
    /** Called to open the folder/workspace picker modal. */
    onOpenFolderPicker: () => void;
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
        id: 'generic-project',
        title: '通用项目',
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
        id: 'professional-project',
        title: '专业项目',
        subtitle: '应用模板 · 完整任务管理 · 团队协作',
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
    onSelectGenericProject,
    onSelectProfessionalProject,
    onOpenFolderPicker,
}: SessionTierPickerProps) {
    const [tier, setTier] = useState<SessionTier>('assistant');
    const [selectedWsId, setSelectedWsId] = useState<string>(workspaces[0]?.id ?? '');
    const [selectedAppId, setSelectedAppId] = useState<string>('');

    const apps = appStore.enabledApps.value;
    const selectedWs = workspaces.find(w => w.id === selectedWsId);
    const selectedApp = apps.find(a => a.id === selectedAppId);

    const handleConfirm = () => {
        if (tier === 'assistant') {
            onSelectAssistant();
        } else if (tier === 'generic-project') {
            if (selectedWs) onSelectGenericProject(selectedWs);
        } else {
            if (selectedWs && selectedApp) {
                onSelectProfessionalProject(selectedWs, selectedApp);
            }
        }
    };

    const canConfirm =
        tier === 'assistant' ||
        (tier === 'generic-project' && !!selectedWs) ||
        (tier === 'professional-project' && !!selectedWs && !!selectedApp);

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

            {/* Secondary config for project tiers */}
            {(tier === 'generic-project' || tier === 'professional-project') && (
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

                    {tier === 'professional-project' && (
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
                    {tier === 'assistant' ? '开始对话' : tier === 'generic-project' ? '进入项目' : '创建专业项目'}
                </button>
            </div>
        </div>
    );
}
