import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import type { App } from '../app';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sessStore from '../../stores/sessionStore';
import * as fs from '../../stores/fsStore';
import * as tabs from '../../stores/tabsStore';
import { ShellNav, type ShellTab } from '../platform/ShellNav';
import { TaskList } from '../drawer/TaskList';
import { SessionsView } from '../drawer/TaskList/SessionsView';
import { WorkspaceFilesSplit, ChannelsPane } from '../shared/WorkspacePanes';
import { SkillsTab } from './SkillsTab';
import { TeamTab } from './TeamTab';
import { SettingsTab } from './SettingsTab';

/**
 * AssistantDetail — breadcrumb level 2 (助理 › <name>). The trail + back-nav
 * live in the global WorkspaceHeader (published by AssistantsPage). This view is
 * the assistant's workbench hub: an identity hero + a secondary top-nav that
 * switches between the surfaces that used to be locked to the side pane.
 *
 * Tabs: 会话 (all sessions incl. archived) · 灵魂 (SOUL.md, via the shared file
 * preview/editor) · 任务 (TaskList) · 技能 (folder-per-skill browser) · 渠道
 * (cc-connect) · 文件 (two-pane browser + preview) · MCP (placeholder).
 *
 * 渠道 / 文件 / 灵魂 / 技能 read the *active* workspace's fs / cc-connect state,
 * so on mount we make this assistant the active workspace context (without the
 * navigation side-effects of selectWorkspace, which would drop the full-page
 * detail).
 */
type DetailTab = 'sessions' | 'team' | 'tasks' | 'skills' | 'channels' | 'files' | 'mcp' | 'settings';

interface AssistantDetailProps {
    workspaceId: string;
    app: App;
}

export function AssistantDetail({ workspaceId, app }: AssistantDetailProps) {
    const language = ui.language.value;
    const theme = ui.theme.value;
    // Resolve from active OR archived so an archived assistant's detail (opened
    // from the overview's 已归档 board) still renders.
    const ws = wsStore.findWorkspaceAnyStatus(workspaceId);

    const [activeTab, setActiveTab] = useState<DetailTab>('sessions');

    // Make this assistant the active workspace *context* (fs + cc-connect) so
    // the 文件 / 渠道 / 灵魂 / 技能 tabs render its data — but skip
    // selectWorkspace's tab navigation, which would exit this full-page detail.
    useEffect(() => {
        if (!ws) return;
        if (wsStore.activeWorkspaceId.value !== workspaceId) {
            wsStore.activeWorkspaceId.value = workspaceId;
            wsStore.loadCcConnectUrl(workspaceId);
            wsStore.loadCcProvidersUrl(workspaceId);
            void fs.switchFsContext(ws);
        }
    }, [workspaceId, ws]);

    // Start a fresh conversation scoped to this assistant via unified SessionSetup.
    const onNewChat = async () => {
        if (ws) await wsStore.selectWorkspace(ws);
        void sessStore.openSessionSetup({
            workspaceId,
            locked: true,
            defaultAgent: ws?.defaultAgent,
        });
    };

    if (!ws) {
        // Stale id (assistant deleted) — drop back to the grid.
        tabs.assistantDetailId.value = null;
        return null;
    }

    const shellTabs: ShellTab[] = [
        { id: 'sessions', label: t('assistant.detail.tab.sessions', language) },
        { id: 'team', label: t('assistant.detail.tab.team', language) },
        { id: 'tasks', label: t('assistant.detail.tab.tasks', language) },
        { id: 'skills', label: t('assistant.detail.tab.skills', language) },
        { id: 'channels', label: t('assistant.detail.tab.channels', language) },
        { id: 'files', label: t('assistant.detail.tab.files', language) },
        { id: 'mcp', label: t('assistant.detail.tab.mcp', language) },
        { id: 'settings', label: t('assistant.detail.tab.settings', language) },
    ];

    return (
        <div class="assistant-detail">
            <ShellNav
                tabs={shellTabs}
                activeTab={activeTab}
                onSelectTab={id => setActiveTab(id as DetailTab)}
                actions={
                    <button class="assistant-btn assistant-btn-primary" onClick={() => void onNewChat()}>
                        {t('assistant.detail.newChat', language)}
                    </button>
                }
            />

            <div class="assistant-tab-body">
                {activeTab === 'sessions' && (
                    <div class="assistant-pane-fill assistant-pane-inset">
                        <SessionsView
                            workspaceId={workspaceId}
                            onSelectSession={s => sessStore.selectSession(s)}
                        />
                    </div>
                )}

                {activeTab === 'team' && (
                    <div class="assistant-pane-fill assistant-pane-inset">
                        <TeamTab workspaceId={workspaceId} app={app} language={language} />
                    </div>
                )}

                {activeTab === 'tasks' && (
                    <div class="project-shell-tasks-wrap">
                        <TaskList workspaceId={workspaceId} onSelectSession={s => void sessStore.selectSession(s)} />
                    </div>
                )}

                {activeTab === 'skills' && (
                    <div class="assistant-pane-fill assistant-pane-inset">
                        <SkillsTab workspaceId={workspaceId} app={app} language={language} />
                    </div>
                )}

                {activeTab === 'channels' && (
                    <div class="assistant-pane-fill">
                        <ChannelsPane theme={theme} language={language} />
                    </div>
                )}

                {activeTab === 'files' && (
                    <div class="assistant-pane-fill assistant-pane-inset">
                        <WorkspaceFilesSplit app={app} language={language} />
                    </div>
                )}

                {activeTab === 'mcp' && (
                    <div class="assistant-pane-scroll">
                        <div class="assistant-empty-row">{t('assistant.detail.mcpPlaceholder', language)}</div>
                    </div>
                )}

                {activeTab === 'settings' && (
                    <div class="assistant-pane-fill assistant-pane-inset assistant-pane-scroll">
                        <SettingsTab workspaceId={workspaceId} language={language} />
                    </div>
                )}
            </div>
        </div>
    );
}
