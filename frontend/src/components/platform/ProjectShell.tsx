/**
 * ProjectShell (#326) — universal project scaffold shared by all professional
 * project types (PM / content / radio / etc.).
 *
 * Four platform tabs: 动态 (activity) | 计划 (plan/milestones) | 任务 (tasks) | 资产 (assets)
 * Plus any project-tab mount points contributed by enabled apps.
 * Plus a 项目配置 (project config) panel accessible via a settings icon.
 *
 * Reuses:
 *   - TaskList  — 任务 tab wires directly into existing task UI
 *   - MountPointRenderer — renders app-contributed tabs via the view registry
 */

import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import { TaskList } from '../drawer/TaskList';
import { MountPointRenderer } from './MountPointRenderer';
import { ProjectConfigPanel } from './ProjectConfigPanel';

import * as appStore from '../../stores/appManifestStore';
import * as sess from '../../stores/sessionStore';

// ── Types ────────────────────────────────────────────────────────────────────

type BuiltinTab = 'activity' | 'plan' | 'tasks' | 'assets';
type TabId = BuiltinTab | string; // string for app-contributed tabs (mount point id)

interface ProjectShellProps {
    workspaceId: string;
    workspaceName?: string;
}

// ── Builtin tab configs ───────────────────────────────────────────────────────

const BUILTIN_TABS: Array<{ id: BuiltinTab; label: string }> = [
    { id: 'activity', label: '动态' },
    { id: 'plan', label: '计划' },
    { id: 'tasks', label: '任务' },
    { id: 'assets', label: '资产' },
];

// ── Component ─────────────────────────────────────────────────────────────────

export function ProjectShell({ workspaceId, workspaceName }: ProjectShellProps) {
    const [activeTab, setActiveTab] = useState<TabId>('tasks');
    const [configOpen, setConfigOpen] = useState(false);

    // Project-tab mount points from enabled apps
    const projectTabs = appStore.projectTabMounts.value;

    // Update co-pilot context when this shell is active
    useEffect(() => {
        appStore.setCopilotAppContext({
            appId: '__project__',
            namespace: workspaceName || workspaceId,
            connectors: [],
            projectWorkspaceId: workspaceId,
        });
        return () => {
            appStore.clearCopilotAppContext();
        };
    }, [workspaceId, workspaceName]);

    return (
        <div class="project-shell">
            {/* Tab bar */}
            <div class="project-shell-tabbar">
                <div class="project-shell-tabs">
                    {BUILTIN_TABS.map(tab => (
                        <button
                            key={tab.id}
                            class={`project-shell-tab${activeTab === tab.id ? ' is-active' : ''}`}
                            onClick={() => setActiveTab(tab.id)}
                        >
                            {tab.label}
                        </button>
                    ))}
                    {/* App-contributed tabs */}
                    {projectTabs.map(({ app, mount }) => (
                        <button
                            key={mount.id}
                            class={`project-shell-tab${activeTab === mount.id ? ' is-active' : ''}`}
                            onClick={() => setActiveTab(mount.id)}
                            title={`${app.name} · ${mount.label}`}
                        >
                            {mount.label}
                        </button>
                    ))}
                </div>

                {/* Project settings gear */}
                <button
                    class="project-shell-config-btn"
                    onClick={() => setConfigOpen(true)}
                    title="项目配置"
                    aria-label="项目配置"
                >
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <circle cx="12" cy="12" r="3" />
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83-2.83l.06-.06A1.65 1.65 0 0 0 4.68 15a1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83-2.83l.06.06A1.65 1.65 0 0 0 9 4.68a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 2.83l-.06.06A1.65 1.65 0 0 0 19.4 9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
                    </svg>
                </button>
            </div>

            {/* Tab content */}
            <div class="project-shell-body">
                {activeTab === 'activity' && <ProjectActivityTab workspaceId={workspaceId} />}
                {activeTab === 'plan' && <ProjectPlanTab workspaceId={workspaceId} />}
                {activeTab === 'tasks' && (
                    <div class="project-shell-tasks-wrap">
                        <TaskList workspaceId={workspaceId} onSelectSession={s => sess.selectSession(s)} />
                    </div>
                )}
                {activeTab === 'assets' && <ProjectAssetsTab workspaceId={workspaceId} />}
                {/* App-contributed tab views */}
                {projectTabs.map(({ app, mount }) =>
                    activeTab === mount.id ? (
                        <div key={mount.id} class="project-shell-app-tab">
                            <MountPointRenderer
                                view={mount.view}
                                appId={app.id}
                                mountId={mount.id}
                                workspaceId={workspaceId}
                            />
                        </div>
                    ) : null
                )}
            </div>

            {/* Project config drawer */}
            {configOpen && <ProjectConfigPanel workspaceId={workspaceId} onClose={() => setConfigOpen(false)} />}
        </div>
    );
}

// ── Builtin tab stub components ───────────────────────────────────────────────

function ProjectActivityTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scaffold">
            <div class="project-tab-scaffold-header">
                <span class="project-tab-scaffold-title">动态</span>
                <span class="project-tab-scaffold-hint">项目最近的事件、评论和状态变更</span>
            </div>
            <div class="project-tab-scaffold-empty">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <circle cx="12" cy="12" r="10" />
                    <polyline points="12 6 12 12 16 14" />
                </svg>
                <p>暂无动态记录</p>
                <span>任务状态变更、评论和 AI 操作日志将显示在这里</span>
            </div>
        </div>
    );
}

function ProjectPlanTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scaffold">
            <div class="project-tab-scaffold-header">
                <span class="project-tab-scaffold-title">计划</span>
                <span class="project-tab-scaffold-hint">里程碑、排期与阶段目标</span>
            </div>
            <div class="project-tab-scaffold-empty">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
                    <line x1="16" y1="2" x2="16" y2="6" />
                    <line x1="8" y1="2" x2="8" y2="6" />
                    <line x1="3" y1="10" x2="21" y2="10" />
                </svg>
                <p>尚未创建里程碑</p>
                <span>在任务视图中创建里程碑，或让 AI 助理帮你制定计划</span>
            </div>
        </div>
    );
}

function ProjectAssetsTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scaffold">
            <div class="project-tab-scaffold-header">
                <span class="project-tab-scaffold-title">资产</span>
                <span class="project-tab-scaffold-hint">项目产物、素材与文件</span>
            </div>
            <div class="project-tab-scaffold-empty">
                <svg
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="1.2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z" />
                </svg>
                <p>暂无资产</p>
                <span>AI 任务产出的文件、生成内容和素材将展示在这里</span>
            </div>
        </div>
    );
}
