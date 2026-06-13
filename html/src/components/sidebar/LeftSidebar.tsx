import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal } from '@preact/signals';
import { WorkspaceFolder, Workspace, RightDrawerTab, Session, isChat } from '../types';
import { t, type Lang } from '../i18n';
import { SessionRow } from './SessionRow';
import { ModuleNav } from './ModuleNav';
import type { ModuleManifest } from '../../modules/module-types';
import { getModuleIconPath } from '../../modules/icon-registry';
import { SETTINGS_MODULE_ID } from '../../modules/settings-manifest';

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
    /**
     * Per-workspace collapse state for the 聊天 / 终端 sub-page groups.
     * Owned by the parent so it survives LeftSidebar remounts and
     * workspace switches.
     */
    collapsedGroups: Record<string, { chat?: boolean; term?: boolean }>;
    onToggleGroup: (folderId: string, key: 'chat' | 'term') => void;
    onStartNewChat: () => void;
    activeTab: string;
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
    onTerminalCreate,
    onTerminalKill,
    onRenameSession,
    onReorderFolders,
    language,
    moduleNav,
    onChatCreate,
    onChatKill,
    collapsedGroups,
    onToggleGroup,
    onStartNewChat,
    activeTab,
}: LeftSidebarProps) {
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [hoveredSessionId, setHoveredSessionId] = useState<string | null>(null);
    // Task id → title map for the optional session task badge (issue
    // model: sessions linked to a task show 📋 <task title>).
    const [taskTitles, setTaskTitles] = useState<Record<string, string>>({});

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
                    const res = await fetch(`/api/agent/tasks?workspace_id=${encodeURIComponent(folder.id)}`);
                    if (!res.ok) continue;
                    const tasks = (await res.json()) as Array<{ id: string; title: string }>;
                    for (const task of tasks || []) {
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
    // Per-workspace collapse state for the 聊天 / 终端 sub-page groups is
    // owned by the parent App (see `sidebarCollapsedGroups` / `onToggleGroup`)
    // so it survives LeftSidebar remounts and is preserved across workspace
    // switches. Default (absent) = expanded; the 任务 entry is a leaf, not a
    // group.

    const confirmDeleteId = useSignal<string | null>(null);
    const deletingId = useSignal<string | null>(null);
    const killingSessionId = useSignal<string | null>(null);
    const openDropdownWsId = useSignal<string | null>(null);
    const dropdownRef = useRef<HTMLDivElement | null>(null);

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
        killingSessionId.value = null;
    }, [folders]);

    // Close the add-session dropdown on outside click.
    useEffect(() => {
        if (!openDropdownWsId.value) return;
        const onDown = (e: MouseEvent) => {
            if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
                openDropdownWsId.value = null;
            }
        };
        document.addEventListener('mousedown', onDown);
        return () => document.removeEventListener('mousedown', onDown);
    }, [openDropdownWsId.value]);

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
                    <div class="sidebar-close-btn" onClick={toggleLeftSidebar} title={t('sidebar.collapse', language)}>
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

                <div class="sidebar-nav-controls">
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
                        <span>New Conversation</span>
                    </button>
                    <div class="nav-control-item" onClick={() => alert('Conversation History: Placeholder')}>
                        <svg
                            class="btn-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                        >
                            <circle cx="12" cy="12" r="10" />
                            <polyline points="12 6 12 12 16 14" />
                        </svg>
                        <span>Conversation History</span>
                    </div>
                    <div class="nav-control-item" onClick={() => alert('Scheduled Tasks: Placeholder')}>
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
                        <span>Scheduled Tasks</span>
                    </div>
                </div>
            </div>

            <div class="sidebar-scroll">
                {!moduleNav ? (
                    <div class="workspace-section">
                        <div class="section-header">
                            <span>Projects</span>
                        </div>

                        {/* Loading skeleton */}
                        {workspacesLoading && (
                            <div class="ws-skeleton">
                                <div class="ws-skeleton-item" />
                                <div class="ws-skeleton-item" style="width:75%" />
                                <div class="ws-skeleton-item" style="width:60%" />
                            </div>
                        )}

                        {/* Empty state */}
                        {!workspacesLoading && folders.length === 0 && (
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
                            folders.map(folder => {
                                const ws = workspaces.find(w => w.id === folder.id);
                                const isHovered = hoveredId === folder.id;
                                const isConfirmingDelete = confirmDeleteId.value === folder.id;
                                const isActive = folder.id === activeWorkspaceId;
                                const isDeleting = deletingId.value === folder.id;
                                const isDropdownOpen = openDropdownWsId.value === folder.id;

                                return (
                                    <div
                                        key={folder.id}
                                        class={`project-node${isActive ? ' ws-active' : ''}${
                                            isDeleting ? ' ws-deleting' : ''
                                        }`}
                                        style={isDropdownOpen ? 'z-index: 200' : ''}
                                        onMouseEnter={() => setHoveredId(folder.id)}
                                        onMouseLeave={() => {
                                            setHoveredId(null);
                                            if (confirmDeleteId.value === folder.id) confirmDeleteId.value = null;
                                        }}
                                    >
                                        {isConfirmingDelete ? (
                                            /* Delete confirm inline */
                                            <div class="ws-delete-confirm">
                                                <span>
                                                    {t('sidebar.deleteConfirm', language, { name: folder.name })}
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
                                                    dragOverId === folder.id && dragOverPosition === 'before'
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
                                                    {ws && (
                                                        <div
                                                            class="ws-add-dropdown"
                                                            ref={isDropdownOpen ? dropdownRef : null}
                                                        >
                                                            <button
                                                                class="ws-action-btn ws-action-add"
                                                                title={t('sidebar.newSession', language) || '新建会话'}
                                                                onClick={(e: MouseEvent) => {
                                                                    e.stopPropagation();
                                                                    openDropdownWsId.value = isDropdownOpen
                                                                        ? null
                                                                        : folder.id;
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
                                                                    <path d="M5 12h14M12 5v14" />
                                                                </svg>
                                                            </button>
                                                            {isDropdownOpen && (
                                                                <div class="ws-add-dropdown-menu">
                                                                    <button
                                                                        class="ws-add-dropdown-item"
                                                                        onClick={(e: MouseEvent) => {
                                                                            e.stopPropagation();
                                                                            openDropdownWsId.value = null;
                                                                            onTerminalCreate(
                                                                                ws.id,
                                                                                ws.terminalDir || ws.path
                                                                            );
                                                                        }}
                                                                    >
                                                                        {t('sidebar.newTerminal', language)}
                                                                    </button>
                                                                    <button
                                                                        class="ws-add-dropdown-item"
                                                                        onClick={(e: MouseEvent) => {
                                                                            e.stopPropagation();
                                                                            openDropdownWsId.value = null;
                                                                            onChatCreate(ws.id);
                                                                        }}
                                                                    >
                                                                        {t('sidebar.newChat', language) || '新建聊天'}
                                                                    </button>
                                                                </div>
                                                            )}
                                                        </div>
                                                    )}
                                                    {isHovered &&
                                                        ws && [
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
                                                // Three fixed sub-pages per workspace: 📋 任务
                                                // (default landing) + 💬 聊天 / 🖥️ 终端 groups,
                                                // each holding its own session list.
                                                const renderSession = (session: Session) => (
                                                    <SessionRow
                                                        key={session.id}
                                                        session={session}
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

                                                const chatSessions = folder.sessions.filter(isChat);
                                                const termSessions = folder.sessions.filter(s => !isChat(s));
                                                const collapsed = collapsedGroups[folder.id] || {};
                                                const wsObj = workspaces.find(w => w.id === folder.id);

                                                return (
                                                    <div class="project-children">
                                                        {/* 📋 任务 — default landing (task table) */}
                                                        <div
                                                            class={`subpage-item subpage-leaf${
                                                                activeWorkspaceId === folder.id ? ' active' : ''
                                                            }`}
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                if (wsObj) onSelectWorkspace(wsObj);
                                                            }}
                                                        >
                                                            <span class="subpage-icon">{'\u{1F4CB}'}</span>
                                                            <span class="subpage-label">
                                                                {t('sidebar.subpage.tasks', language) || '任务'}
                                                            </span>
                                                        </div>

                                                        {/* 💬 聊天 group */}
                                                        <div
                                                            class="subpage-item subpage-group-header"
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                onToggleGroup(folder.id, 'chat');
                                                            }}
                                                        >
                                                            <span
                                                                class={`subpage-chevron${collapsed.chat ? '' : ' open'}`}
                                                            >
                                                                ▸
                                                            </span>
                                                            <span class="subpage-icon">{'\u{1F4AC}'}</span>
                                                            <span class="subpage-label">
                                                                {t('sidebar.subpage.chats', language) || '聊天'}
                                                            </span>
                                                            <span class="subpage-count">{chatSessions.length}</span>
                                                            <button
                                                                class="subpage-add-btn"
                                                                title={t('sidebar.newChat', language) || '新建聊天'}
                                                                onClick={(e: MouseEvent) => {
                                                                    e.stopPropagation();
                                                                    onChatCreate(folder.id);
                                                                }}
                                                            >
                                                                +
                                                            </button>
                                                        </div>
                                                        {!collapsed.chat &&
                                                            (chatSessions.length > 0 ? (
                                                                <div class="subpage-children">
                                                                    {chatSessions.map(renderSession)}
                                                                </div>
                                                            ) : (
                                                                <div class="ws-no-sessions subpage-empty">
                                                                    {t('sidebar.noSessions', language)}
                                                                </div>
                                                            ))}

                                                        {/* 🖥️ 终端 group */}
                                                        <div
                                                            class="subpage-item subpage-group-header"
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                onToggleGroup(folder.id, 'term');
                                                            }}
                                                        >
                                                            <span
                                                                class={`subpage-chevron${collapsed.term ? '' : ' open'}`}
                                                            >
                                                                ▸
                                                            </span>
                                                            <span class="subpage-icon">{'\u{1F5A5}️'}</span>
                                                            <span class="subpage-label">
                                                                {t('sidebar.subpage.terminals', language) || '终端'}
                                                            </span>
                                                            <span class="subpage-count">{termSessions.length}</span>
                                                            <button
                                                                class="subpage-add-btn"
                                                                title={t('sidebar.newTerminal', language) || '新建终端'}
                                                                onClick={(e: MouseEvent) => {
                                                                    e.stopPropagation();
                                                                    if (wsObj) {
                                                                        onTerminalCreate(
                                                                            wsObj.id,
                                                                            wsObj.terminalDir || wsObj.path
                                                                        );
                                                                    }
                                                                }}
                                                            >
                                                                +
                                                            </button>
                                                        </div>
                                                        {!collapsed.term &&
                                                            (termSessions.length > 0 ? (
                                                                <div class="subpage-children">
                                                                    {termSessions.map(renderSession)}
                                                                </div>
                                                            ) : (
                                                                <div class="ws-no-sessions subpage-empty">
                                                                    {t('sidebar.noSessions', language)}
                                                                </div>
                                                            ))}
                                                    </div>
                                                );
                                            })()}
                                    </div>
                                );
                            })}
                    </div>
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
                <div
                    class={`footer-item${activeDrawerTab === 'providers' ? ' active' : ''}`}
                    onClick={() => toggleDrawerTab('providers')}
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
                    onClick={() => toggleDrawerTab('skills')}
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
                    onClick={() => toggleDrawerTab('discovery')}
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
                    class={`footer-item${activeDrawerTab === 'settings' ? ' active' : ''}`}
                    onClick={() => toggleDrawerTab('settings')}
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
        </aside>
    );
}
