import { h } from 'preact';
import { useState } from 'preact/hooks';
import { WorkspaceFolder, Workspace, RightDrawerTab, Session } from '../types';

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
    onCreateWorkspace: () => void;
    onRenameWorkspace: (ws: Workspace) => void;
    onDeleteWorkspace: (id: string) => void;
    onSelectWorkspace: (ws: Workspace) => void;
    onSelectSession: (session: Session) => void;
    onTerminalCreate: (workspaceId: string, cwd: string) => void;
    onTerminalKill: (windowIndex: number) => void;
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
    onCreateWorkspace,
    onRenameWorkspace,
    onDeleteWorkspace,
    onSelectWorkspace,
    onSelectSession,
    onTerminalCreate,
    onTerminalKill,
}: LeftSidebarProps) {
    const [hoveredId, setHoveredId] = useState<string | null>(null);
    const [confirmDeleteId, setConfirmDeleteId] = useState<string | null>(null);

    const handleDeleteClick = (e: MouseEvent, id: string) => {
        e.stopPropagation();
        setConfirmDeleteId(id);
    };

    const confirmDelete = (e: MouseEvent, id: string) => {
        e.stopPropagation();
        setConfirmDeleteId(null);
        onDeleteWorkspace(id);
    };

    const cancelDelete = (e: MouseEvent) => {
        e.stopPropagation();
        setConfirmDeleteId(null);
    };

    return (
        <aside
            class={`left-sidebar ${leftSidebarOpen ? '' : 'collapsed'}`}
            style={leftSidebarOpen ? `width: ${leftSidebarWidth}px` : ''}
            onClick={(e: MouseEvent) => {
                // If on mobile and clicking the backdrop (outside the sidebar container which is 280px wide on mobile)
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
                    <div class="sidebar-close-btn" onClick={toggleLeftSidebar} title="折叠侧边栏">
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

            <div class="sidebar-scroll">
                <div class="workspace-section">
                    <div class="section-header">
                        <span>工作空间 Workspaces</span>
                        <div class="header-actions">
                            {/* Add workspace button */}
                            <button
                                class="ws-add-btn"
                                onClick={(e: MouseEvent) => {
                                    e.stopPropagation();
                                    onCreateWorkspace();
                                }}
                                title="新建工作空间"
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
                        </div>
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
                            <span>暂无工作空间</span>
                            <button class="ws-empty-add" onClick={onCreateWorkspace}>
                                + 新建
                            </button>
                        </div>
                    )}

                    {!workspacesLoading &&
                        folders.map(folder => {
                            const ws = workspaces.find(w => w.id === folder.id);
                            const isHovered = hoveredId === folder.id;
                            const isConfirmingDelete = confirmDeleteId === folder.id;
                            const isActive = folder.id === activeWorkspaceId;

                            return (
                                <div
                                    key={folder.id}
                                    class={`project-node${isActive ? ' ws-active' : ''}`}
                                    onMouseEnter={() => setHoveredId(folder.id)}
                                    onMouseLeave={() => {
                                        setHoveredId(null);
                                        if (confirmDeleteId === folder.id) setConfirmDeleteId(null);
                                    }}
                                >
                                    {isConfirmingDelete ? (
                                        /* Delete confirm inline */
                                        <div class="ws-delete-confirm">
                                            <span>删除 "{folder.name}"？</span>
                                            <button
                                                class="ws-del-yes"
                                                onClick={(e: MouseEvent) => confirmDelete(e, folder.id)}
                                            >
                                                删除
                                            </button>
                                            <button class="ws-del-no" onClick={cancelDelete}>
                                                取消
                                            </button>
                                        </div>
                                    ) : (
                                        <div
                                            class={`project-folder ${folder.expanded ? 'expanded' : ''}`}
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
                                            <div class="ws-actions" onClick={(e: MouseEvent) => e.stopPropagation()}>
                                                {/* Add session button — always visible */}
                                                {ws && (
                                                    <button
                                                        class="ws-action-btn ws-action-add"
                                                        title="新建终端"
                                                        onClick={(e: MouseEvent) => {
                                                            e.stopPropagation();
                                                            onTerminalCreate(ws.id, ws.terminalDir || ws.path);
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
                                                )}
                                                {isHovered &&
                                                    ws && [
                                                        <button
                                                            class="ws-action-btn"
                                                            title="编辑"
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
                                                            title="删除"
                                                            onClick={(e: MouseEvent) => handleDeleteClick(e, folder.id)}
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

                                    {folder.expanded && (
                                        <div class="project-children">
                                            {folder.sessions.length === 0 ? (
                                                <div class="ws-no-sessions">暂无会话 — 点击工作空间旁的 + 创建</div>
                                            ) : (
                                                folder.sessions.map(session => (
                                                    <div
                                                        key={session.id}
                                                        class={`chat-item ${session.active ? 'active' : ''}`}
                                                        onClick={(e: MouseEvent) => {
                                                            e.stopPropagation();
                                                            onSelectSession(session);
                                                        }}
                                                    >
                                                        <span class="chat-title" title={session.name}>
                                                            {session.name}
                                                        </span>
                                                        <span class="chat-time">{session.workspaceId}</span>
                                                        <button
                                                            class="session-kill-btn"
                                                            title="关闭会话"
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                onTerminalKill(session.index);
                                                            }}
                                                        >
                                                            <svg
                                                                width="12"
                                                                height="12"
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                            >
                                                                <line x1="18" x2="6" y1="6" y2="18" />
                                                                <line x1="6" x2="18" y1="6" y2="18" />
                                                            </svg>
                                                        </button>
                                                    </div>
                                                ))
                                            )}
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                </div>
            </div>

            <div class="sidebar-footer">
                <div class="footer-item" onClick={() => toggleDrawerTab('settings')}>
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <circle cx="12" cy="12" r="3" />
                        <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z" />
                    </svg>
                    <span>Settings</span>
                </div>
                <div class="footer-item">
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                    </svg>
                    <span>Feedback</span>
                </div>
            </div>
        </aside>
    );
}
