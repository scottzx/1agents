import { h } from 'preact';
import { useRef } from 'preact/hooks';
import { WorkspaceFolder, Workspace, RightDrawerTab, getStatusLabel } from '../types';

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
}: LeftSidebarProps) {
    const longPressTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

    const handleMouseDown = (e: MouseEvent, id: string) => {
        if ((e.target as HTMLElement).closest('.ws-actions')) return;
        longPressTimer.current = setTimeout(() => {
            const ws = workspaces.find(w => w.id === id);
            if (ws && window.confirm(`是否移除工作空间 "${ws.name}"？`)) {
                onDeleteWorkspace(id);
            }
            longPressTimer.current = null;
        }, 600);
    };

    const handleMouseUp = () => {
        if (longPressTimer.current) {
            clearTimeout(longPressTimer.current);
            longPressTimer.current = null;
        }
    };

    return (
        <aside
            class={`left-sidebar ${leftSidebarOpen ? '' : 'collapsed'}`}
            style={leftSidebarOpen ? `width: ${leftSidebarWidth}px` : ''}
        >
            <div class="sidebar-header">
                <div class="coze-brand">
                    <div class="brand-left">
                        <img class="brand-logo-img" src="logo.png" />
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
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <path d="M3 16h10M3 12h18M3 8h18" />
                            </svg>
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
                            const isActive = ws?.id === activeWorkspaceId;

                            return (
                                <div
                                    key={folder.id}
                                    class={`project-node ${isActive ? 'ws-active' : ''}`}
                                    onMouseLeave={handleMouseUp}
                                    onMouseDown={(e: MouseEvent) => handleMouseDown(e, folder.id)}
                                    onMouseUp={handleMouseUp}
                                >
                                    <div
                                        class={`project-folder ${folder.expanded ? 'expanded' : ''} ${
                                            isActive ? 'active' : ''
                                        }`}
                                    >
                                        <div class="folder-click-area" onClick={() => toggleFolder(folder.id)}>
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
                                            {ws?.status && (
                                                <span class={`ws-status-badge ws-status-${ws.status}`}>
                                                    {getStatusLabel(ws.status)}
                                                </span>
                                            )}
                                        </div>

                                        {/* Action buttons: edit (hover), select (always, rightmost) */}
                                        {ws && (
                                            <div class="ws-actions" onClick={(e: MouseEvent) => e.stopPropagation()}>
                                                <button
                                                    class="ws-action-btn ws-action-edit"
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
                                                </button>
                                                {ws.path && (
                                                    <button
                                                        class={`ws-action-btn ws-action-select ${
                                                            isActive ? 'selected' : ''
                                                        }`}
                                                        title={isActive ? '当前工作空间' : '点击切换工作空间'}
                                                        onClick={(e: MouseEvent) => {
                                                            e.stopPropagation();
                                                            onSelectWorkspace(ws);
                                                        }}
                                                    >
                                                        {isActive ? '✓' : '→'}
                                                    </button>
                                                )}
                                            </div>
                                        )}
                                    </div>

                                    {folder.expanded && (
                                        <div class="project-children">
                                            {folder.children.length === 0 ? (
                                                <div class="ws-no-sessions">暂无会话</div>
                                            ) : (
                                                folder.children.map(child => (
                                                    <div
                                                        key={child.id}
                                                        class={`chat-item ${child.active ? 'active' : ''}`}
                                                    >
                                                        <span class="chat-title" title={child.title}>
                                                            {child.title}
                                                        </span>
                                                        <span class="chat-time">{child.time}</span>
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
