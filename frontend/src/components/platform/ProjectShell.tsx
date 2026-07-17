/**
 * ProjectShell (#326) — universal project scaffold shared by all professional
 * project types (PM / content / radio / etc.).
 *
 * Two variants:
 *   - 'panel'  (default) — the lean 项目管理 artifact column in the split
 *     workbench: crumbs + 动态/计划/任务/资产 (+ app tabs) + 配置 gear.
 *   - 'detail' — the full-page 项目详情 (L2), redesigned to match the 助理 详情:
 *     breadcrumb + identity hero + a secondary top-nav that adds the reusable
 *     会话/文件/渠道 panes alongside the project-specific tabs. Reuses the same
 *     codex-minimal primitives (.assistant-hero / .assistant-pane-* etc.).
 *
 * Reuses:
 *   - TaskList / SessionsView — 任务 / 会话 tabs
 *   - WorkspaceFilesSplit / ChannelsPane — 文件 / 渠道 (need active fs context)
 *   - MountPointRenderer — app-contributed tabs via the view registry
 */

import { h, Fragment } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import type { App } from '../app';
import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sess from '../../stores/sessionStore';
import * as fs from '../../stores/fsStore';
import { TaskList } from '../drawer/TaskList';
import { SessionsView } from '../drawer/TaskList/SessionsView';
import { WorkspaceFilesSplit, ChannelsPane } from '../shared/WorkspacePanes';
import { TeamTab } from '../pages/TeamTab';
import { SettingsTab } from '../pages/SettingsTab';
import { MountPointRenderer } from './MountPointRenderer';
import { ProjectConfigPanel, ProjectConfigView, PROJECT_CONFIG_TABS, type ConfigTab } from './ProjectConfigPanel';
import { ShellNav, CrumbTrail, type ShellTab, type Crumb } from './ShellNav';
import { EmptyState, DetailSection } from '../shared/primitives';

import * as appStore from '../../stores/appManifestStore';
import * as tabPrefs from '../../stores/projectTabPrefs';

// ── Types ────────────────────────────────────────────────────────────────────

type BuiltinTab = 'sessions' | 'team' | 'activity' | 'plan' | 'tasks' | 'files' | 'channels' | 'assets' | 'settings';
type TabId = BuiltinTab | string; // string for app-contributed tabs (mount point id)

interface ProjectShellProps {
    workspaceId: string;
    workspaceName?: string;
    /** Optional breadcrumb trail rendered above the tab bar (e.g. 项目总览 → this). */
    crumbs?: Crumb[];
    /** 'detail' = full-page L2 hub with hero + reusable panes; 'panel' = lean column. */
    variant?: 'detail' | 'panel';
    /** Needed by the 文件/渠道 panes (detail variant). */
    app?: App;
}

// ── Component ─────────────────────────────────────────────────────────────────

export function ProjectShell({ workspaceId, workspaceName, crumbs, variant = 'panel', app }: ProjectShellProps) {
    const isDetail = variant === 'detail';
    const language = ui.language.value;
    const theme = ui.theme.value;
    const ws = wsStore.findWorkspaceAnyStatus(workspaceId);

    const [activeTab, setActiveTab] = useState<TabId>('tasks');
    const [configOpen, setConfigOpen] = useState(false);

    // Project-tab mount points from enabled apps
    const projectTabs = appStore.projectTabMounts.value;
    // Per-project hidden tabs (toggled in 设置), persisted project-local. Load once;
    // getHiddenTabs is a reactive read that re-renders when it lands / changes.
    useEffect(() => {
        tabPrefs.ensureLoaded(workspaceId, ws?.path ?? '');
    }, [workspaceId, ws?.path]);
    const hiddenTabs = tabPrefs.getHiddenTabs(workspaceId);

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

    // Detail hub → make this project the active workspace context (fs +
    // cc-connect) so 文件/渠道 render its data, without selectWorkspace's tab nav.
    useEffect(() => {
        if (!isDetail || !ws) return;
        if (wsStore.activeWorkspaceId.value !== workspaceId) {
            wsStore.activeWorkspaceId.value = workspaceId;
            wsStore.loadCcConnectUrl(workspaceId);
            wsStore.loadCcProvidersUrl(workspaceId);
            void fs.switchFsContext(ws);
        }
    }, [isDetail, workspaceId, ws]);

    // Detail-only: 会话/文件/渠道 tabs prepend the project-specific ones.
    const detailLead: Array<{ id: BuiltinTab; label: string }> = [
        { id: 'sessions', label: t('assistant.detail.tab.sessions', language) },
        { id: 'team', label: t('assistant.detail.tab.team', language) },
        { id: 'tasks', label: t('assistant.detail.tab.tasks', language) },
        { id: 'files', label: t('assistant.detail.tab.files', language) },
        { id: 'channels', label: t('assistant.detail.tab.channels', language) },
    ];
    const projectSpecific: Array<{ id: BuiltinTab; label: string }> = [
        { id: 'activity', label: '动态' },
        { id: 'plan', label: '计划' },
        ...(isDetail ? [] : [{ id: 'tasks' as BuiltinTab, label: '任务' }]),
        { id: 'assets', label: '资产' },
    ];

    const shellTabs: ShellTab[] = [
        ...(isDetail ? detailLead : []),
        ...projectSpecific.map(tb => ({ id: tb.id, label: tb.label })),
        ...projectTabs.map(({ app: a, mount }) => ({
            id: mount.id,
            label: mount.label,
            title: `${a.name} · ${mount.label}`,
        })),
        // Detail folds the 项目配置 sub-tabs (指令/连接器/…) into the main bar.
        ...(isDetail ? PROJECT_CONFIG_TABS.map(c => ({ id: c.id, label: c.label })) : []),
        ...(isDetail ? [{ id: 'settings' as BuiltinTab, label: t('assistant.detail.tab.settings', language) }] : []),
    ].filter(tb => tb.id === 'settings' || !hiddenTabs.has(tb.id)); // 项目设置里勾掉的 tab 隐藏（settings 恒显以便恢复）
    const isConfigTab = PROJECT_CONFIG_TABS.some(c => c.id === activeTab);

    // Start a fresh conversation scoped to this project via unified SessionSetup.
    const onNewChat = async () => {
        if (ws) await wsStore.selectWorkspace(ws);
        void sess.openSessionSetup({
            workspaceId,
            locked: true,
            defaultAgent: ws?.defaultAgent,
        });
    };

    const configGear = (
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
    );

    // Shared tab body — one active pane. Detail wraps app-like panes with the
    // 8px inset used by the 助理 详情.
    const tabContent = (
        <Fragment>
            {activeTab === 'sessions' && (
                <div class="assistant-pane-fill assistant-pane-inset">
                    <SessionsView workspaceId={workspaceId} onSelectSession={s => sess.selectSession(s)} />
                </div>
            )}
            {activeTab === 'tasks' && (
                <div class="project-shell-tasks-wrap">
                    <TaskList workspaceId={workspaceId} onSelectSession={s => void sess.selectSession(s)} />
                </div>
            )}
            {activeTab === 'files' && app && (
                <div class="assistant-pane-fill assistant-pane-inset">
                    <WorkspaceFilesSplit app={app} language={language} />
                </div>
            )}
            {activeTab === 'channels' && (
                <div class="assistant-pane-fill">
                    <ChannelsPane theme={theme} language={language} />
                </div>
            )}
            {activeTab === 'team' && app && (
                <div class="assistant-pane-fill assistant-pane-inset">
                    <TeamTab workspaceId={workspaceId} app={app} language={language} />
                </div>
            )}
            {activeTab === 'settings' && (
                <div class="assistant-pane-fill assistant-pane-inset">
                    <SettingsTab workspaceId={workspaceId} language={language} />
                </div>
            )}
            {activeTab === 'activity' && <ProjectActivityTab workspaceId={workspaceId} />}
            {activeTab === 'plan' && <ProjectPlanTab workspaceId={workspaceId} />}
            {activeTab === 'assets' && <ProjectAssetsTab workspaceId={workspaceId} />}
            {isDetail && isConfigTab && (
                <div class="assistant-pane-fill assistant-pane-inset">
                    <ProjectConfigView
                        workspaceId={workspaceId}
                        section={activeTab as ConfigTab}
                        app={app}
                        language={language}
                    />
                </div>
            )}
            {projectTabs.map(({ app: a, mount }) =>
                activeTab === mount.id ? (
                    <div key={mount.id} class="project-shell-app-tab">
                        <MountPointRenderer
                            view={mount.view}
                            appId={a.id}
                            mountId={mount.id}
                            workspaceId={workspaceId}
                        />
                    </div>
                ) : null
            )}
        </Fragment>
    );

    if (isDetail) {
        return (
            <div class="project-shell project-shell-detail">
                {crumbs && crumbs.length > 0 && (
                    <div class="project-detail-breadcrumb">
                        <CrumbTrail crumbs={crumbs} />
                    </div>
                )}
                <ShellNav
                    tabs={shellTabs}
                    activeTab={activeTab}
                    onSelectTab={id => setActiveTab(id)}
                    actions={
                        <button class="assistant-btn assistant-btn-primary" onClick={() => void onNewChat()}>
                            {t('assistant.detail.newChat', language)}
                        </button>
                    }
                />
                <div class="assistant-tab-body">{tabContent}</div>
            </div>
        );
    }

    return (
        <div class="project-shell">
            <ShellNav
                crumbs={crumbs}
                tabs={shellTabs}
                activeTab={activeTab}
                onSelectTab={id => setActiveTab(id)}
                actions={configGear}
            />
            <div class="project-shell-body">{tabContent}</div>
            {configOpen && <ProjectConfigPanel workspaceId={workspaceId} onClose={() => setConfigOpen(false)} />}
        </div>
    );
}

// ── Builtin tab stub components ───────────────────────────────────────────────

function ProjectActivityTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scroll">
            <EmptyState title="暂无动态记录" description="任务状态变更、评论和 AI 操作日志将以时间线形式显示在这里" />
        </div>
    );
}

function ProjectPlanTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scroll">
            <DetailSection title="里程碑" description="排期与阶段目标">
                <EmptyState title="尚未创建里程碑" description="在任务视图中创建里程碑，或让 AI 助理帮你制定计划" />
            </DetailSection>
        </div>
    );
}

function ProjectAssetsTab(props: { workspaceId: string }) {
    void props;
    return (
        <div class="project-tab-scroll">
            <DetailSection title="项目产物" description="AI 任务产出的文件、生成内容和素材">
                <EmptyState title="暂无资产" description="完成的任务产物和素材将展示在这里" />
            </DetailSection>
        </div>
    );
}
