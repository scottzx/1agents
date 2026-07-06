import { h, Fragment } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { WorkspaceFolder, Workspace, RightDrawerTab, Session, isChat, isTerminal } from '../types';
import { t, type Lang } from '../i18n';
import { SessionRow } from './SessionRow';
import { ModuleNav } from './ModuleNav';
import type { ModuleManifest } from '../../modules/module-types';
import { getModuleIconPath } from '../../modules/icon-registry';
import { SETTINGS_MODULE_ID } from '../../modules/settings-manifest';
import { sidebarMode, isBeginnerMode } from '../../stores/uiStore';
import { openCreateAssistantModal } from '../../stores/modalStore';
import { assistantDetailId } from '../../stores/tabsStore';
import {
    remoteDevices,
    remoteExpanded,
    remoteLoading,
    remoteProjects,
    toggleRemoteDevice,
    activeWorkspaceId as activeWsIdSignal,
    activeWorkspaceDeviceId,
    collapseFolders,
} from '../../stores/workspaceStore';
import { activeL1PageId } from '../../stores/appManifestStore';
import { getL1NavEntries, L1NavItem } from '../platform/L1Shell';
import {
    enterL1App,
    exitL1App,
    projectOverview,
    projectStack,
    showProjectContext,
    showAssistantContext,
} from '../../stores/stageStore';
import { taskService } from '@1agents/core/services/taskService';
import { inboxService } from '@1agents/core/services/inboxService';
import { openSearch } from '../../stores/searchStore';

/** Mac / Linux / Windows OS 图标(currentColor,适配主题)。复用 DevicesPanel 的判定。 */
function DeviceOsIcon({ os }: { os?: string }) {
    const kind = (os ?? '').toLowerCase();
    const common = {
        viewBox: '0 0 24 24',
        fill: 'none',
        stroke: 'currentColor',
        'stroke-width': '2',
        'stroke-linecap': 'round' as const,
        'stroke-linejoin': 'round' as const,
        class: 'folder-icon',
    };
    if (kind.includes('darwin') || kind.includes('mac') || kind.includes('ios')) {
        return (
            <svg {...common}>
                <path d="M12 20.94c1.5 0 2.75 1.06 4 1.06 3 0 6-8 6-12.22A4.91 4.91 0 0 0 17 5c-2.22 0-4 1.44-5 2-1-.56-2.78-2-5-2a4.9 4.9 0 0 0-5 4.78C2 14 5 22 8 22c1.25 0 2.5-1.06 4-1.06Z" />
                <path d="M10 2c1 .5 2 2 2 5" />
            </svg>
        );
    }
    if (kind.includes('windows')) {
        return (
            <svg {...common}>
                <rect x="3" y="4" width="7" height="7" rx="0.5" />
                <rect x="14" y="4" width="7" height="7" rx="0.5" />
                <rect x="3" y="13" width="7" height="7" rx="0.5" />
                <rect x="14" y="13" width="7" height="7" rx="0.5" />
            </svg>
        );
    }
    // Linux / 未知 → 服务器箱体
    return (
        <svg {...common}>
            <rect x="2" y="2" width="20" height="8" rx="2" />
            <rect x="2" y="14" width="20" height="8" rx="2" />
            <line x1="6" y1="6" x2="6.01" y2="6" />
            <line x1="6" y1="18" x2="6.01" y2="18" />
        </svg>
    );
}

interface LeftSidebarProps {
    folders: WorkspaceFolder[];
    workspaces: Workspace[];
    workspacesLoading: boolean;
    leftSidebarOpen: boolean;
    leftSidebarWidth: number;
    activeWorkspaceId: string;
    toggleLeftSidebar: () => void;
    toggleFolder: (id: string) => void;
    toggleDrawerTab: (tab: RightDrawerTab) => void;
    activeDrawerTab: RightDrawerTab;
    onCreateWorkspace: () => void;
    onRenameWorkspace: (ws: Workspace) => void;
    onDeleteWorkspace: (id: string) => void;
    onSelectWorkspace: (ws: Workspace) => void;
    onSelectSession: (session: Session) => void;
    onTerminalCreate: (workspaceId: string, cwd: string) => void;
    onTerminalKill: (windowIndex: number) => void;
    onRenameSession: (session: Session) => void;
    onReorderFolders?: (draggedId: string, targetId: string, position: 'before' | 'after') => void;
    language: Lang;

    /**
     * Optional module nav surface. Set when the active drawer tab is backed
     * by a module (1skills today). The host renders this inside the same
     * sidebar column — never as a separate nested sidebar.
     */
    moduleNav?: {
        manifest: ModuleManifest;
        activePath: string;
        onNavigate: (to: string) => void;
    };
    onChatCreate: (workspaceId: string) => void;
    onChatKill: (sessionId: string) => void;
    onStartNewChat: () => void;
    activeTab: string;
    /** The user's currently-selected session (drives the sidebar row highlight). */
    activeSession: Session | null;
    /** Top-level tab id; 'tasks' means the 任务 landing is showing. */
    activeTabId: string;
}

export function LeftSidebar({
    folders,
    workspaces,
    workspacesLoading,
    leftSidebarOpen,
    leftSidebarWidth,
    activeWorkspaceId,
    toggleLeftSidebar,
    toggleFolder,
    toggleDrawerTab,
    activeDrawerTab,
    onCreateWorkspace,
    onRenameWorkspace,
    onDeleteWorkspace,
    onSelectWorkspace,
    onSelectSession,
    onTerminalKill,
    onRenameSession,
    onReorderFolders,
    language,
    moduleNav,
    onChatKill,
    onStartNewChat,
    activeTab,
    activeSession,
    activeTabId,
}: LeftSidebarProps) {
    // On the 任务 landing, no session row is selected. Otherwise a row is
    // highlighted iff it matches the globally-active session (chat by id,
    // terminal by tmux index) — one highlight at a time, never alongside 任务.
    const isTaskView = activeTabId === 'tasks';
    const isSelectedSession = (s: Session): boolean => {
        if (isTaskView || !activeSession) return false;
        if (isChat(s) && isChat(activeSession)) return s.id === activeSession.id;
        if (isTerminal(s) && isTerminal(activeSession)) {
            return s.index === activeSession.index && s.workspaceId === activeSession.workspaceId;
        }
        return false;
    };
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [hoveredSessionId, setHoveredSessionId] = useState<string | null>(null);
    const [projectSearch, setProjectSearch] = useState('');
    const [projectSearchOpen, setProjectSearchOpen] = useState(false);
    // Footer utility entries (模型/技能/发现/数据源/系统设置) are collapsed
    // behind a single 「更多」 that pulls up this panel — frees sidebar space.
    const [moreOpen, setMoreOpen] = useState(false);
    const MORE_TABS: RightDrawerTab[] = ['providers', 'skills', 'discovery', 'datasources', 'settings'];
    const openMoreTab = (tab: RightDrawerTab) => {
        toggleDrawerTab(tab);
        setMoreOpen(false);
    };
    const projectSearchRef = useRef<HTMLInputElement | null>(null);
    // Task id → title map for the optional session task badge (issue
    // model: sessions linked to a task show 📋 <task title>).
    const [taskTitles, setTaskTitles] = useState<Record<string, string>>({});
    // Inbox unread badge (#60). Refetched on mount and whenever the Inbox tab is
    // opened/closed, so archiving/reading there keeps the count in sync.
    const [inboxUnread, setInboxUnread] = useState(0);
    useEffect(() => {
        let cancelled = false;
        inboxService
            .list(false)
            .then(res => {
                if (!cancelled) setInboxUnread(res.unread);
            })
            .catch(() => {
                /* badge is decorative — ignore fetch failures */
            });
        return () => {
            cancelled = true;
        };
    }, [activeDrawerTab]);

    const hasTaskLinkedSession = folders.some(f => f.sessions.some(s => isChat(s) && Boolean(s.taskId)));
    const folderIdsKey = folders.map(f => f.id).join(',');
    useEffect(() => {
        if (!hasTaskLinkedSession) return;
        let cancelled = false;
        (async () => {
            const titles: Record<string, string> = {};
            for (const folder of folders) {
                if (!folder.sessions.some(s => isChat(s) && Boolean(s.taskId))) continue;
                try {
                    const tasks = await taskService.list(folder.id);
                    for (const task of tasks) {
                        titles[task.id] = task.title;
                    }
                } catch {
                    // badge is decorative — ignore fetch failures
                }
            }
            if (!cancelled) setTaskTitles(titles);
        })();
        return () => {
            cancelled = true;
        };
        // folders identity churns every render; folderIdsKey is the stable signal.
    }, [folderIdsKey, hasTaskLinkedSession]);

    const confirmDeleteId = useSignal<string | null>(null);
    const deletingId = useSignal<string | null>(null);
    const killingSessionId = useSignal<string | null>(null);
    const [draggedId, setDraggedId] = useState<string | null>(null);
    const [dragOverId, setDragOverId] = useState<string | null>(null);
    const [dragOverPosition, setDragOverPosition] = useState<'before' | 'after' | null>(null);

    const handleDragStart = (e: DragEvent, id: string) => {
        if (confirmDeleteId.value === id) {
            e.preventDefault();
            return;
        }
        setDraggedId(id);
        if (e.dataTransfer) {
            e.dataTransfer.effectAllowed = 'move';
            e.dataTransfer.setData('text/plain', id);
        }
    };

    const handleDragOver = (e: DragEvent, targetId: string) => {
        e.preventDefault();
        if (draggedId === targetId) return;

        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const relativeY = e.clientY - rect.top;
        const isAfter = relativeY > rect.height / 2;

        setDragOverId(targetId);
        setDragOverPosition(isAfter ? 'after' : 'before');
    };

    const handleDragLeave = (e: DragEvent, targetId: string) => {
        const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
        const x = e.clientX;
        const y = e.clientY;
        if (x < rect.left || x >= rect.right || y < rect.top || y >= rect.bottom) {
            if (dragOverId === targetId) {
                setDragOverId(null);
                setDragOverPosition(null);
            }
        }
    };

    const handleDrop = (e: DragEvent, targetId: string) => {
        e.preventDefault();
        if (draggedId && draggedId !== targetId && dragOverPosition && onReorderFolders) {
            onReorderFolders(draggedId, targetId, dragOverPosition);
        }
        setDraggedId(null);
        setDragOverId(null);
        setDragOverPosition(null);
    };

    const handleDragEnd = () => {
        setDraggedId(null);
        setDragOverId(null);
        setDragOverPosition(null);
    };

    useEffect(() => {
        deletingId.value = null;
    }, [folders]);

    const handleDeleteClick = (e: MouseEvent, id: string) => {
        e.stopPropagation();
        confirmDeleteId.value = id;
    };

    const confirmDelete = (e: MouseEvent, id: string) => {
        e.stopPropagation();
        if (workspaces.length <= 1) {
            confirmDeleteId.value = null;
            onDeleteWorkspace(id);
            return;
        }
        confirmDeleteId.value = null;
        deletingId.value = id;
        setTimeout(() => {
            onDeleteWorkspace(id);
        }, 300);
    };

    const cancelDelete = (e: MouseEvent) => {
        e.stopPropagation();
        confirmDeleteId.value = null;
    };

    const handleSessionKill = (e: MouseEvent, session: Session) => {
        e.stopPropagation();
        killingSessionId.value = session.id;
        setTimeout(() => {
            if (isChat(session)) onChatKill(session.id);
            else onTerminalKill(session.index);
        }, 300);
    };

    const renderSession = (session: Session) => (
        <SessionRow
            key={session.id}
            session={session}
            selected={isSelectedSession(session)}
            killing={killingSessionId.value === session.id}
            isHovered={hoveredSessionId === session.id}
            taskTitles={taskTitles}
            language={language}
            onSelect={onSelectSession}
            onKill={handleSessionKill}
            onRename={onRenameSession}
            onHoverChange={setHoveredSessionId}
        />
    );

    return (
        <aside
            class={`left-sidebar ${leftSidebarOpen ? '' : 'collapsed'}`}
            style={leftSidebarOpen ? `width: ${leftSidebarWidth}px` : ''}
            onClick={(e: MouseEvent) => {
                if (window.innerWidth <= 768 && e.clientX > 280) {
                    toggleLeftSidebar();
                }
            }}
        >
            <div class="sidebar-header">
                <div class="coze-brand">
                    <div class="brand-left">
                        <img class="brand-logo-img" src="/logo.png" />
                        <span>1agents</span>
                    </div>
                    <div class="brand-right">
                        <div
                            class="sidebar-search-btn"
                            onClick={openSearch}
                            title={t('sidebar.navCtrl.history', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <circle cx="11" cy="11" r="8" />
                                <line x1="21" y1="21" x2="16.65" y2="16.65" />
                            </svg>
                        </div>
                        <div
                            class="sidebar-close-btn"
                            onClick={toggleLeftSidebar}
                            title={t('sidebar.collapse', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <polyline points="15 18 9 12 15 6" />
                            </svg>
                        </div>
                    </div>
                </div>

                {!isBeginnerMode.value && (
                    <div class="sidebar-mode-toggle">
                        <button
                            class={`mode-tab${sidebarMode.value === 'assistant' ? ' active' : ''}`}
                            onClick={() => showAssistantContext()}
                        >
                            {t('sidebar.mode.assistant', language)}
                        </button>
                        <button
                            class={`mode-tab${sidebarMode.value === 'project' ? ' active' : ''}`}
                            onClick={() => showProjectContext()}
                        >
                            {t('sidebar.mode.project', language)}
                        </button>
                    </div>
                )}

                <div class="sidebar-nav-controls">
                    {sidebarMode.value === 'project' && (
                        <div
                            class={`nav-control-item${projectStack.value.length === 0 ? ' active' : ''}`}
                            onClick={() => projectOverview()}
                        >
                            <svg
                                class="btn-icon"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect x="3" y="3" width="7" height="7" rx="1" />
                                <rect x="14" y="3" width="7" height="7" rx="1" />
                                <rect x="14" y="14" width="7" height="7" rx="1" />
                                <rect x="3" y="14" width="7" height="7" rx="1" />
                            </svg>
                            <span>{t('sidebar.navCtrl.projectOverview', language)}</span>
                        </div>
                    )}
                    {sidebarMode.value === 'assistant' && (
                        <Fragment>
                            <button
                                class={`nav-control-btn new-chat-btn ${activeTab === 'new_chat' ? 'active' : ''}`}
                                onClick={onStartNewChat}
                            >
                                <svg
                                    class="btn-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2.5"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <line x1="12" y1="5" x2="12" y2="19" />
                                    <line x1="5" y1="12" x2="19" y2="12" />
                                </svg>
                                <span>{t('sidebar.navCtrl.newConversation', language)}</span>
                            </button>
                            <div
                                class={`nav-control-item${activeDrawerTab === 'assistants' ? ' active' : ''}`}
                                onClick={() => {
                                    // 点这个总览项不算"选中了某个助理", 清掉残留的详情 id,
                                    // 落到助理网格, 不要复现上次点开的那个助理。
                                    assistantDetailId.value = null;
                                    toggleDrawerTab('assistants');
                                }}
                            >
                                <svg
                                    class="btn-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="8" r="4" />
                                    <path d="M4 20c0-4 4-6 8-6s8 2 8 6" />
                                </svg>
                                <span>{t('sidebar.assistants', language)}</span>
                            </div>
                            <div
                                class={`nav-control-item${activeDrawerTab === 'contacts' ? ' active' : ''}`}
                                onClick={() => toggleDrawerTab('contacts')}
                            >
                                <svg
                                    class="btn-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" />
                                    <circle cx="9" cy="7" r="4" />
                                    <path d="M22 21v-2a4 4 0 0 0-3-3.87" />
                                    <path d="M16 3.13a4 4 0 0 1 0 7.75" />
                                </svg>
                                <span>{t('sidebar.navCtrl.contacts', language)}</span>
                            </div>
                            <div
                                class={`nav-control-item${activeDrawerTab === 'inbox' ? ' active' : ''}`}
                                onClick={() => toggleDrawerTab('inbox')}
                            >
                                <svg
                                    class="btn-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M22 12h-6l-2 3h-4l-2-3H2" />
                                    <path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" />
                                </svg>
                                <span>{t('sidebar.navCtrl.inbox', language)}</span>
                                {inboxUnread > 0 && <span class="nav-control-badge">{inboxUnread}</span>}
                            </div>
                            <div
                                class={`nav-control-item${activeDrawerTab === 'reminders' ? ' active' : ''}`}
                                onClick={() => toggleDrawerTab('reminders')}
                            >
                                <svg
                                    class="btn-icon"
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="3" y="4" width="18" height="18" rx="2" ry="2" />
                                    <line x1="16" y1="2" x2="16" y2="6" />
                                    <line x1="8" y1="2" x2="8" y2="6" />
                                    <line x1="3" y1="10" x2="21" y2="10" />
                                </svg>
                                <span>{t('sidebar.navCtrl.scheduledTasks', language)}</span>
                            </div>
                        </Fragment>
                    )}
                </div>
            </div>

            <div class="sidebar-scroll">
                {!moduleNav ? (
                    <Fragment>
                        {/* ── 助理 section: 唯一的对话列表, default 置顶 ── */}
                        {sidebarMode.value === 'assistant' && (
                            <div class="workspace-section">
                                <div class="section-header">
                                    <span>{t('sidebar.assistants', language)}</span>
                                    <div class="section-header-actions">
                                        <button
                                            class="section-search-btn"
                                            title={t('sidebar.collapseAll', language) || '一键折叠'}
                                            onClick={() => {
                                                const ids = workspaces
                                                    .filter(w => (w.kind ?? 'project') === 'assistant' && !w.deviceId)
                                                    .map(w => w.id);
                                                collapseFolders(ids);
                                            }}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <polyline points="17 11 12 6 7 11" />
                                                <polyline points="17 18 12 13 7 18" />
                                            </svg>
                                        </button>
                                        <button
                                            class="section-search-btn"
                                            title={t('sidebar.addAssistant', language)}
                                            onClick={openCreateAssistantModal}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <line x1="12" y1="5" x2="12" y2="19" />
                                                <line x1="5" y1="12" x2="19" y2="12" />
                                            </svg>
                                        </button>
                                    </div>
                                </div>
                                {(() => {
                                    // 助理 = kind==='assistant' 的本地工作区。default 是内置助理,
                                    // 永远置顶;其余按 position 排序。远程设备项目归 project 模式管。
                                    // 兼容旧数据:kind 为空视作 project(不入助理列表)。
                                    const assistantWss = workspaces
                                        .filter(w => (w.kind ?? 'project') === 'assistant' && !w.deviceId)
                                        .sort((a, b) => {
                                            if (a.id === 'default') return -1;
                                            if (b.id === 'default') return 1;
                                            return 0;
                                        });
                                    if (assistantWss.length === 0) {
                                        return (
                                            <div class="ws-empty">
                                                <svg
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    stroke-width="1.5"
                                                    stroke-linecap="round"
                                                    stroke-linejoin="round"
                                                >
                                                    <circle cx="12" cy="8" r="4" />
                                                    <path d="M4 20c0-4 4-6 8-6s8 2 8 6" />
                                                </svg>
                                                <span>{t('sidebar.noAssistants', language)}</span>
                                                <button class="ws-empty-add" onClick={openCreateAssistantModal}>
                                                    {t('sidebar.addAssistant', language)}
                                                </button>
                                            </div>
                                        );
                                    }
                                    return assistantWss.map(ws => {
                                        const folder = folders.find(f => f.id === ws.id);
                                        if (!folder) return null;
                                        const isActive = activeWorkspaceId === ws.id;
                                        const chatSessions = folder.sessions.filter(isChat);
                                        const termSessions = folder.sessions.filter(s => !isChat(s));
                                        return (
                                            <div
                                                key={ws.id}
                                                class={`project-node${isActive ? ' ws-active' : ''}`}
                                                onMouseEnter={() => setHoveredId(ws.id)}
                                                onMouseLeave={() => setHoveredId(null)}
                                            >
                                                <div
                                                    class={`project-folder ${folder.expanded ? 'expanded' : ''}`}
                                                    onClick={() => {
                                                        toggleFolder(ws.id);
                                                        onSelectWorkspace(ws);
                                                    }}
                                                >
                                                    <svg
                                                        class="chevron"
                                                        viewBox="0 0 24 24"
                                                        fill="none"
                                                        stroke="currentColor"
                                                        stroke-width="2.5"
                                                        stroke-linecap="round"
                                                        stroke-linejoin="round"
                                                    >
                                                        <polyline points="9 18 15 12 9 6" />
                                                    </svg>
                                                    {ws.avatar && ws.avatar.startsWith('/') ? (
                                                        <img class="folder-avatar" src={ws.avatar} alt="" />
                                                    ) : (
                                                        <svg
                                                            class="folder-icon"
                                                            viewBox="0 0 24 24"
                                                            fill="none"
                                                            stroke="currentColor"
                                                            stroke-width="2"
                                                            stroke-linecap="round"
                                                            stroke-linejoin="round"
                                                        >
                                                            <circle cx="12" cy="8" r="4" />
                                                            <path d="M4 20c0-4 4-6 8-6s8 2 8 6" />
                                                        </svg>
                                                    )}
                                                    <span class="ws-name">{ws.name}</span>
                                                </div>
                                                {folder.expanded && (
                                                    <div class="project-children">
                                                        {chatSessions.map(renderSession)}
                                                        {termSessions.map(renderSession)}
                                                        {chatSessions.length === 0 && termSessions.length === 0 && (
                                                            <div
                                                                class="chat-item"
                                                                style="opacity:0.5;cursor:default;pointer-events:none;"
                                                            >
                                                                <div class="chat-item-left">
                                                                    <span class="chat-title">
                                                                        {t('sidebar.noChats', language)}
                                                                    </span>
                                                                </div>
                                                            </div>
                                                        )}
                                                    </div>
                                                )}
                                            </div>
                                        );
                                    });
                                })()}
                            </div>
                        )}

                        {!isBeginnerMode.value && sidebarMode.value === 'project' && (
                            <div class="workspace-section">
                                <div class="section-header">
                                    <span>Projects</span>
                                    <div class="section-header-actions">
                                        <button
                                            class="section-search-btn"
                                            title={t('sidebar.collapseAll', language) || '一键折叠'}
                                            onClick={() => {
                                                const ids = folders
                                                    .filter(
                                                        f =>
                                                            (workspaces.find(w => w.id === f.id)?.kind ?? 'project') !==
                                                            'assistant'
                                                    )
                                                    .map(f => f.id);
                                                collapseFolders(ids);
                                            }}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <polyline points="17 11 12 6 7 11" />
                                                <polyline points="17 18 12 13 7 18" />
                                            </svg>
                                        </button>
                                        <button
                                            class={`section-search-btn${projectSearchOpen ? ' active' : ''}`}
                                            title="搜索项目"
                                            onClick={() => {
                                                const next = !projectSearchOpen;
                                                setProjectSearchOpen(next);
                                                if (!next) setProjectSearch('');
                                                else setTimeout(() => projectSearchRef.current?.focus(), 50);
                                            }}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <circle cx="11" cy="11" r="8" />
                                                <line x1="21" y1="21" x2="16.65" y2="16.65" />
                                            </svg>
                                        </button>
                                    </div>
                                </div>
                                {projectSearchOpen && (
                                    <div class="section-search-wrap">
                                        <input
                                            ref={projectSearchRef}
                                            class="section-search-input"
                                            type="text"
                                            placeholder="搜索项目…"
                                            value={projectSearch}
                                            onInput={(e: Event) =>
                                                setProjectSearch((e.target as HTMLInputElement).value)
                                            }
                                        />
                                        {projectSearch && (
                                            <button
                                                class="section-search-clear"
                                                onClick={() => setProjectSearch('')}
                                                type="button"
                                            >
                                                ×
                                            </button>
                                        )}
                                    </div>
                                )}

                                {/* Loading skeleton */}
                                {workspacesLoading && (
                                    <div class="ws-skeleton">
                                        <div class="ws-skeleton-item" />
                                        <div class="ws-skeleton-item" style="width:75%" />
                                        <div class="ws-skeleton-item" style="width:60%" />
                                    </div>
                                )}

                                {/* Empty state — folders whose ws is a project (i.e. not an assistant). */}
                                {!workspacesLoading &&
                                    folders.filter(f => {
                                        const w = workspaces.find(x => x.id === f.id);
                                        return (w?.kind ?? 'project') !== 'assistant';
                                    }).length === 0 && (
                                        <div class="ws-empty">
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="1.5"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                            >
                                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                            </svg>
                                            <span>{t('sidebar.empty', language)}</span>
                                            <button class="ws-empty-add" onClick={onCreateWorkspace}>
                                                {t('common.new', language)}
                                            </button>
                                        </div>
                                    )}

                                {!workspacesLoading &&
                                    folders
                                        .filter(f => {
                                            const w = workspaces.find(x => x.id === f.id);
                                            return (w?.kind ?? 'project') !== 'assistant';
                                        })
                                        .filter(f => {
                                            if (!projectSearch) return true;
                                            const ws = workspaces.find(w => w.id === f.id);
                                            return (ws?.name ?? f.id)
                                                .toLowerCase()
                                                .includes(projectSearch.toLowerCase());
                                        })
                                        .map(folder => {
                                            const ws = workspaces.find(w => w.id === folder.id);
                                            const isHovered = hoveredId === folder.id;
                                            const isConfirmingDelete = confirmDeleteId.value === folder.id;
                                            const isActive = folder.id === activeWorkspaceId;
                                            const isDeleting = deletingId.value === folder.id;

                                            return (
                                                <div
                                                    key={folder.id}
                                                    class={`project-node${isActive ? ' ws-active' : ''}${
                                                        isDeleting ? ' ws-deleting' : ''
                                                    }`}
                                                    onMouseEnter={() => setHoveredId(folder.id)}
                                                    onMouseLeave={() => {
                                                        setHoveredId(null);
                                                        if (confirmDeleteId.value === folder.id)
                                                            confirmDeleteId.value = null;
                                                    }}
                                                >
                                                    {isConfirmingDelete ? (
                                                        /* Delete confirm inline */
                                                        <div class="ws-delete-confirm">
                                                            <span>
                                                                {t('sidebar.deleteConfirm', language, {
                                                                    name: folder.name,
                                                                })}
                                                            </span>
                                                            <button
                                                                class="ws-del-yes"
                                                                onClick={(e: MouseEvent) => confirmDelete(e, folder.id)}
                                                            >
                                                                {t('common.delete', language)}
                                                            </button>
                                                            <button class="ws-del-no" onClick={cancelDelete}>
                                                                {t('common.cancel', language)}
                                                            </button>
                                                        </div>
                                                    ) : (
                                                        <div
                                                            class={`project-folder ${folder.expanded ? 'expanded' : ''} ${
                                                                draggedId === folder.id ? 'dragging' : ''
                                                            } ${
                                                                dragOverId === folder.id &&
                                                                dragOverPosition === 'before'
                                                                    ? 'drag-over-before'
                                                                    : ''
                                                            } ${
                                                                dragOverId === folder.id && dragOverPosition === 'after'
                                                                    ? 'drag-over-after'
                                                                    : ''
                                                            }`}
                                                            draggable={true}
                                                            onDragStart={e => handleDragStart(e, folder.id)}
                                                            onDragOver={e => handleDragOver(e, folder.id)}
                                                            onDragLeave={e => handleDragLeave(e, folder.id)}
                                                            onDrop={e => handleDrop(e, folder.id)}
                                                            onDragEnd={handleDragEnd}
                                                            onClick={() => {
                                                                toggleFolder(folder.id);
                                                                if (ws) onSelectWorkspace(ws);
                                                            }}
                                                        >
                                                            <svg
                                                                class="chevron"
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2.5"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <polyline points="9 18 15 12 9 6" />
                                                            </svg>
                                                            <svg
                                                                class="folder-icon"
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                            >
                                                                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
                                                            </svg>
                                                            <span class="ws-name" title={ws?.path || folder.name}>
                                                                {folder.name}
                                                            </span>

                                                            {/* Action buttons */}
                                                            <div
                                                                class="ws-actions"
                                                                draggable={false}
                                                                onDragStart={e => e.preventDefault()}
                                                                onClick={(e: MouseEvent) => e.stopPropagation()}
                                                            >
                                                                {isHovered &&
                                                                    ws &&
                                                                    !ws.builtin && [
                                                                        <button
                                                                            class="ws-action-btn"
                                                                            title={t('common.edit', language)}
                                                                            onClick={(e: MouseEvent) => {
                                                                                e.stopPropagation();
                                                                                onRenameWorkspace(ws);
                                                                            }}
                                                                        >
                                                                            <svg
                                                                                viewBox="0 0 24 24"
                                                                                fill="none"
                                                                                stroke="currentColor"
                                                                                stroke-width="2"
                                                                                stroke-linecap="round"
                                                                                stroke-linejoin="round"
                                                                            >
                                                                                <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
                                                                                <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
                                                                            </svg>
                                                                        </button>,
                                                                        <button
                                                                            class="ws-action-btn ws-action-delete"
                                                                            title={t('common.delete', language)}
                                                                            onClick={(e: MouseEvent) =>
                                                                                handleDeleteClick(e, folder.id)
                                                                            }
                                                                        >
                                                                            <svg
                                                                                viewBox="0 0 24 24"
                                                                                fill="none"
                                                                                stroke="currentColor"
                                                                                stroke-width="2"
                                                                                stroke-linecap="round"
                                                                                stroke-linejoin="round"
                                                                            >
                                                                                <polyline points="3 6 5 6 21 6" />
                                                                                <path d="M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6" />
                                                                                <path d="M10 11v6M14 11v6" />
                                                                                <path d="M9 6V4a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
                                                                            </svg>
                                                                        </button>,
                                                                    ]}
                                                            </div>
                                                        </div>
                                                    )}

                                                    {folder.expanded &&
                                                        (() => {
                                                            const chatSessions = folder.sessions.filter(isChat);
                                                            const termSessions = folder.sessions.filter(
                                                                s => !isChat(s)
                                                            );

                                                            // One unified list under each workspace: every
                                                            // 会话 / 终端 session, using the same `.chat-item`
                                                            // row style (no group headers).
                                                            return (
                                                                <div class="project-children">
                                                                    {/* 会话 (chat) + 终端 (terminal) sessions */}
                                                                    {chatSessions.map(renderSession)}
                                                                    {termSessions.map(renderSession)}
                                                                </div>
                                                            );
                                                        })()}
                                                </div>
                                            );
                                        })}

                                {/* ── 远程设备分组(#114)──────────────────────────
                                    已注册的远程设备:可折叠组,展开时经代理路由拉取该
                                    设备的项目;离线组灰显并在展开时提示无法连接。 */}
                                {remoteDevices.value.map(device => {
                                    const expanded = Boolean(remoteExpanded.value[device.id]);
                                    const loading = Boolean(remoteLoading.value[device.id]);
                                    const projects = remoteProjects.value[device.id] ?? [];
                                    return (
                                        <div
                                            key={`dev-${device.id}`}
                                            class={`project-node device-node${device.active ? '' : ' device-offline'}`}
                                        >
                                            <div
                                                class={`project-folder ${expanded ? 'expanded' : ''}`}
                                                title={
                                                    device.active
                                                        ? device.name
                                                        : t('sidebar.device.offlineHint', language, {
                                                              name: device.name,
                                                          })
                                                }
                                                onClick={() => void toggleRemoteDevice(device)}
                                            >
                                                <svg
                                                    class="chevron"
                                                    viewBox="0 0 24 24"
                                                    fill="none"
                                                    stroke="currentColor"
                                                    stroke-width="2.5"
                                                    stroke-linecap="round"
                                                    stroke-linejoin="round"
                                                >
                                                    <polyline points="9 18 15 12 9 6" />
                                                </svg>
                                                <DeviceOsIcon os={device.os} />
                                                <span class="ws-name">{device.name}</span>
                                                <span
                                                    class={`device-status-dot${device.active ? ' online' : ''}`}
                                                    aria-hidden="true"
                                                />
                                            </div>
                                            {expanded && (
                                                <div class="project-children">
                                                    {loading && (
                                                        <div
                                                            class="chat-item"
                                                            style="opacity:0.6;cursor:default;pointer-events:none;"
                                                        >
                                                            <div class="chat-item-left">
                                                                <span class="chat-title">
                                                                    {t('common.loading', language)}
                                                                </span>
                                                            </div>
                                                        </div>
                                                    )}
                                                    {!loading && device.active && projects.length === 0 && (
                                                        <div
                                                            class="chat-item"
                                                            style="opacity:0.5;cursor:default;pointer-events:none;"
                                                        >
                                                            <div class="chat-item-left">
                                                                <span class="chat-title">
                                                                    {t('sidebar.empty', language)}
                                                                </span>
                                                            </div>
                                                        </div>
                                                    )}
                                                    {!loading &&
                                                        projects.map(rws => {
                                                            const isActiveRemote =
                                                                activeWsIdSignal.value === rws.id &&
                                                                activeWorkspaceDeviceId.value === device.id;
                                                            return (
                                                                <div
                                                                    key={`${device.id}-${rws.id}`}
                                                                    class={`chat-item chat-row-kind-task${
                                                                        isActiveRemote && isTaskView ? ' active' : ''
                                                                    }`}
                                                                    onClick={(e: MouseEvent) => {
                                                                        e.stopPropagation();
                                                                        onSelectWorkspace(rws);
                                                                    }}
                                                                >
                                                                    <div class="chat-item-left">
                                                                        <span
                                                                            class="chat-sidebar-avatar chat-task-icon"
                                                                            aria-hidden="true"
                                                                        >
                                                                            {'\u{1F4C1}'}
                                                                        </span>
                                                                        <span class="chat-title" title={rws.path}>
                                                                            {rws.name}
                                                                        </span>
                                                                    </div>
                                                                </div>
                                                            );
                                                        })}
                                                </div>
                                            )}
                                        </div>
                                    );
                                })}
                            </div>
                        )}
                        {/* ── 应用 / L1 apps section (#332) ──────────────────────────────────
                            Only shown when there are enabled L1-page mount points.
                            Each entry switches the main pane to the app's full-page view. */}
                        {(() => {
                            const appEntries = getL1NavEntries();
                            if (appEntries.length === 0) return null;
                            const currentL1Id = activeL1PageId.value;
                            return (
                                <div class="workspace-section">
                                    <div class="section-header">
                                        <span>{language === 'zh-CN' ? '应用' : 'Apps'}</span>
                                    </div>
                                    <div class="l1-apps-nav">
                                        {appEntries.map(entry => (
                                            <L1NavItem
                                                key={entry.id}
                                                entry={entry}
                                                isActive={currentL1Id === entry.id}
                                                onClick={() => {
                                                    if (currentL1Id === entry.id) {
                                                        // Toggle off — return to previous shell.
                                                        exitL1App();
                                                    } else {
                                                        enterL1App(entry.id);
                                                    }
                                                }}
                                            />
                                        ))}
                                    </div>
                                </div>
                            );
                        })()}
                    </Fragment>
                ) : (
                    <ModuleNav
                        manifest={moduleNav.manifest}
                        activePath={moduleNav.activePath}
                        language={language}
                        onNavigate={moduleNav.onNavigate}
                    />
                )}
            </div>

            <div class="sidebar-footer">
                {moreOpen && (
                    <Fragment>
                        <div class="sidebar-more-backdrop" onClick={() => setMoreOpen(false)} />
                        <div class="sidebar-more-panel" role="menu">
                            <div
                                class={`footer-item${activeDrawerTab === 'providers' ? ' active' : ''}`}
                                onClick={() => openMoreTab('providers')}
                                title={t('sidebar.providersTitle', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <rect x="2" y="2" width="20" height="8" rx="2" ry="2" />
                                    <rect x="2" y="14" width="20" height="8" rx="2" ry="2" />
                                    <line x1="6" y1="6" x2="6.01" y2="6" />
                                    <line x1="6" y1="18" x2="6.01" y2="18" />
                                </svg>
                                <span>{t('sidebar.providers', language)}</span>
                            </div>
                            <div
                                class={`footer-item${activeDrawerTab === 'skills' ? ' active' : ''}`}
                                onClick={() => openMoreTab('skills')}
                                title={t('sidebar.skillsTitle', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                                </svg>
                                <span>{t('sidebar.skills', language)}</span>
                            </div>
                            <div
                                class={`footer-item${activeDrawerTab === 'discovery' ? ' active' : ''}`}
                                onClick={() => openMoreTab('discovery')}
                                title={t('sidebar.discoveryTitle', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <circle cx="12" cy="12" r="10" />
                                    <polygon points="16.24 7.76 14.12 14.12 7.76 16.24 9.88 9.88 16.24 7.24" />
                                </svg>
                                <span>{t('sidebar.discovery', language)}</span>
                            </div>
                            <div
                                class={`footer-item${activeDrawerTab === 'datasources' ? ' active' : ''}`}
                                onClick={() => openMoreTab('datasources')}
                                title={t('sidebar.datasourcesTitle', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <ellipse cx="12" cy="5" rx="9" ry="3" />
                                    <path d="M3 5v14a9 3 0 0 0 18 0V5" />
                                    <path d="M3 12a9 3 0 0 0 18 0" />
                                </svg>
                                <span>{t('sidebar.datasources', language)}</span>
                            </div>
                            <div
                                class={`footer-item${activeDrawerTab === 'settings' ? ' active' : ''}`}
                                onClick={() => openMoreTab('settings')}
                                title={t('sidebar.settings', language)}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    // Icon comes from the host's icon registry, keyed by
                                    // module id — keeps the visual identity in sync with
                                    // the settings manifest, no inline SVG here.
                                    dangerouslySetInnerHTML={{ __html: getModuleIconPath(SETTINGS_MODULE_ID) || '' }}
                                />
                                <span>{t('sidebar.settings', language)}</span>
                            </div>
                        </div>
                    </Fragment>
                )}
                <div
                    class={`footer-item${moreOpen || MORE_TABS.includes(activeDrawerTab) ? ' active' : ''}`}
                    onClick={() => setMoreOpen(v => !v)}
                    title={t('sidebar.more', language)}
                >
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <rect x="3" y="3" width="7" height="7" rx="1.5" />
                        <rect x="14" y="3" width="7" height="7" rx="1.5" />
                        <rect x="3" y="14" width="7" height="7" rx="1.5" />
                        <rect x="14" y="14" width="7" height="7" rx="1.5" />
                    </svg>
                    <span>{t('sidebar.more', language)}</span>
                </div>
            </div>
        </aside>
    );
}
