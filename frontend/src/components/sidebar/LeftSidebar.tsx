import { h, Fragment } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import { useSignal, useComputed } from '@preact/signals';
import { WorkspaceFolder, Workspace, RightDrawerTab, Session, isChat, isTerminal } from '../types';
import { t, type Lang } from '../i18n';
import { SessionRow } from './SessionRow';
import { ModuleNav } from './ModuleNav';
import { FsRowActionsMenu, type FsRowAction } from '../drawer/FsRowActionsMenu';
import { archiveWorkspace } from '../../stores/workspaceStore';
import type { ModuleManifest } from '../../modules/module-types';
import { getModuleIconPath } from '../../modules/icon-registry';
import { SETTINGS_MODULE_ID } from '../../modules/settings-manifest';
import { isBeginnerMode } from '../../stores/uiStore';
import { openCreateAssistantModal } from '../../stores/modalStore';
import { assistantDetailId, activeDrawerTab as activeDrawerTabSignal } from '../../stores/tabsStore';
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
import {
    chatSessions as chatSessionsSignal,
    activeSession as activeSessionSignal,
    chatsForWorkspace,
    chatsForAssistants,
    terminalsForFolderSessions,
} from '../../stores/sessionStore';
import { activeL1PageId, openL1Apps, activeRoundtableRoom, activeRoundtableSeats } from '../../stores/appManifestStore';
import { getL1NavEntries, L1NavItem } from '../platform/L1Shell';
import { roleLabel, seatStatusLabel, seatDisplayStatus } from '../roundtable/roleLabels';
import { requestRoundtableListView } from '../roundtable/navState';
import type { RoundtableSeat } from '@1agents/core/services/roundtableService';
import { enterL1App, exitL1App, archiveL1App, layoutMode, projectOverview } from '../../stores/stageStore';
import { projectItemService } from '@1agents/core/services/taskService';
import { openSearch } from '../../stores/searchStore';
import { ModeSwitcher } from '../header/ModeSwitcher';
import type { ChatSession } from '../types';

/** Short label for assistant filter chips (first grapheme of name). */
function assistantShortLabel(name: string): string {
    const chars = [...(name || '').trim()];
    return chars[0] || '?';
}

/** Session kind: ordinary chat / task-linked chat / terminal. */
type SessionKindFilter = 'all' | 'normal' | 'task' | 'terminal';
/** Last-reply window for chat sessions (terminals have no reply timestamp). */
type SessionTimeFilter = 'all' | '1h' | '1d' | '1w' | 'custom';

function sessionActivityMs(c: ChatSession): number {
    const raw = c.lastEventAt || c.createdAt || '';
    if (!raw) return 0;
    const ts = Date.parse(raw);
    return Number.isFinite(ts) ? ts : 0;
}

function timeBounds(time: SessionTimeFilter, customFrom: string, customTo: string): { minTs: number; maxTs: number } {
    let minTs = 0;
    let maxTs = Number.POSITIVE_INFINITY;
    if (time === '1h') minTs = Date.now() - 3_600_000;
    else if (time === '1d') minTs = Date.now() - 86_400_000;
    else if (time === '1w') minTs = Date.now() - 7 * 86_400_000;
    else if (time === 'custom') {
        if (customFrom) {
            const d = Date.parse(`${customFrom}T00:00:00`);
            if (Number.isFinite(d)) minTs = d;
        }
        if (customTo) {
            const d = Date.parse(`${customTo}T23:59:59.999`);
            if (Number.isFinite(d)) maxTs = d;
        }
    }
    return { minTs, maxTs };
}

/** Filter chat sessions by kind + last-reply window. Kind=terminal → no chats. */
function filterChatsByRules(
    sessions: ChatSession[],
    kind: SessionKindFilter,
    time: SessionTimeFilter,
    customFrom: string,
    customTo: string
): ChatSession[] {
    if (kind === 'terminal') return [];

    const { minTs, maxTs } = timeBounds(time, customFrom, customTo);
    return sessions.filter(c => {
        if (kind === 'normal' && c.taskId) return false;
        if (kind === 'task' && !c.taskId) return false;
        if (time !== 'all') {
            const ts = sessionActivityMs(c);
            if (ts < minTs || ts > maxTs) return false;
        }
        return true;
    });
}

/** Terminals only when kind is all or terminal; hidden for chat-only kinds. */
function filterTermsByKind(terms: Session[], kind: SessionKindFilter): Session[] {
    if (kind === 'normal' || kind === 'task') return [];
    return terms;
}

const SECTION_OPEN_KEY = {
    tasks: '1agents-sidebar-section-tasks',
    projects: '1agents-sidebar-section-projects',
    apps: '1agents-sidebar-section-apps',
} as const;

function readSectionOpen(key: keyof typeof SECTION_OPEN_KEY, fallback: boolean): boolean {
    try {
        const raw = localStorage.getItem(SECTION_OPEN_KEY[key]);
        if (raw === null) return fallback;
        return raw === '1' || raw === 'true';
    } catch {
        return fallback;
    }
}

function writeSectionOpen(key: keyof typeof SECTION_OPEN_KEY, open: boolean): void {
    try {
        localStorage.setItem(SECTION_OPEN_KEY[key], open ? '1' : '0');
    } catch {
        /* ignore quota / private mode */
    }
}

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

/** Closed vs open folder glyph — doubles as expand/collapse affordance. */
function FolderToggleIcon({ open }: { open: boolean }) {
    return (
        <svg
            class="folder-icon"
            viewBox="0 0 24 24"
            width="16"
            height="16"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
            style={{ width: '16px', height: '16px', flexShrink: 0 }}
        >
            {open ? (
                <path d="M6 14l1.45-2.9A2 2 0 0 1 9.24 10H20a2 2 0 0 1 1.94 2.5l-1.55 6a2 2 0 0 1-1.94 1.5H4a2 2 0 0 1-2-2V5c0-1.1.9-2 2-2h3.93a2 2 0 0 1 1.66.9l.82 1.2a2 2 0 0 0 1.66.9H18a2 2 0 0 1 2 2v2" />
            ) : (
                <path d="M4 20h16a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7.93a2 2 0 0 1-1.66-.9l-.82-1.2A2 2 0 0 0 7.93 3H4a2 2 0 0 0-2 2v13a2 2 0 0 0 2 2z" />
            )}
        </svg>
    );
}

// 12px SVG icons used by the folder "..." menu (新建会话 / 新建终端 / 重命名 / 归档).
const WS_ACTION_ICONS = {
    newChat: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
            <line x1="12" y1="8" x2="12" y2="14" />
            <line x1="9" y1="11" x2="15" y2="11" />
        </svg>
    ),
    newTerminal: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="4 17 10 11 4 5" />
            <line x1="12" y1="19" x2="20" y2="19" />
        </svg>
    ),
    rename: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <path d="M11 4H4a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7" />
            <path d="M18.5 2.5a2.121 2.121 0 0 1 3 3L12 15l-4 1 1-4 9.5-9.5z" />
        </svg>
    ),
    archive: (
        <svg
            width="13"
            height="13"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <polyline points="21 8 21 21 3 21 3 8" />
            <rect x="1" y="3" width="22" height="5" />
            <line x1="10" y1="12" x2="14" y2="12" />
        </svg>
    ),
};

/** Build the "..." dropdown items for a workspace row. Built-in workspaces
 *  can't be archived, so the danger entry is hidden for them. */
function buildFolderActions(
    ws: Workspace,
    onChatCreate: (id: string) => void,
    onTerminalCreate: (workspaceId: string, cwd: string) => void,
    onRename: (ws: Workspace) => void
): FsRowAction[] {
    const items: FsRowAction[] = [
        {
            id: 'newChat',
            labelKey: 'sidebar.newSession',
            icon: WS_ACTION_ICONS.newChat,
            onSelect: () => onChatCreate(ws.id),
        },
        {
            id: 'newTerminal',
            labelKey: 'sidebar.newTerminal',
            icon: WS_ACTION_ICONS.newTerminal,
            onSelect: () => onTerminalCreate(ws.id, ws.terminalDir || ws.path || '.'),
        },
        {
            id: 'rename',
            labelKey: 'common.edit',
            icon: WS_ACTION_ICONS.rename,
            onSelect: () => onRename(ws),
        },
    ];
    if (!ws.builtin) {
        items.push({
            id: 'archive',
            labelKey: 'assistant.detail.settings.archive',
            icon: WS_ACTION_ICONS.archive,
            danger: true,
            onSelect: () => {
                void archiveWorkspace(ws.id);
            },
        });
    }
    return items;
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
    onSelectWorkspace: (ws: Workspace) => void;
    onSelectSession: (session: Session) => void;
    onTerminalCreate: (workspaceId: string, cwd: string) => void;
    onTerminalKill: (windowIndex: number) => void;
    onRenameSession: (session: Session) => void;
    onReorderFolders?: (draggedId: string, targetId: string, position: 'before' | 'after') => void;
    language: Lang;

    /**
     * Optional module nav surface. Set when the active drawer tab is backed
     * by a module (HarnessKit today). The host renders this inside the same
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
    const [projectSearch, setProjectSearch] = useState('');
    const [projectSearchOpen, setProjectSearchOpen] = useState(false);
    /** Flat task list filter: null = all assistants; else workspace id. */
    const [taskFilterWsId, setTaskFilterWsId] = useState<string | null>(null);
    /** Global session filters (kind + last-reply window for chats). */
    const [sessionKindFilter, setSessionKindFilter] = useState<SessionKindFilter>('all');
    const [sessionTimeFilter, setSessionTimeFilter] = useState<SessionTimeFilter>('all');
    const [sessionTimeFrom, setSessionTimeFrom] = useState('');
    const [sessionTimeTo, setSessionTimeTo] = useState('');
    const [sessionFilterOpen, setSessionFilterOpen] = useState(false);
    const sessionFilterRef = useRef<HTMLDivElement | null>(null);
    /** Section-level fold: 任务 / 项目 region expand (persisted). */
    const [tasksSectionOpen, setTasksSectionOpen] = useState(() => readSectionOpen('tasks', true));
    const [projectsSectionOpen, setProjectsSectionOpen] = useState(() => readSectionOpen('projects', true));
    const [appsSectionOpen, setAppsSectionOpen] = useState(() => readSectionOpen('apps', true));
    const toggleTasksSection = () => {
        setTasksSectionOpen(prev => {
            const next = !prev;
            writeSectionOpen('tasks', next);
            return next;
        });
    };
    const toggleProjectsSection = () => {
        setProjectsSectionOpen(prev => {
            const next = !prev;
            writeSectionOpen('projects', next);
            return next;
        });
    };
    const toggleAppsSection = () => {
        setAppsSectionOpen(prev => {
            const next = !prev;
            writeSectionOpen('apps', next);
            return next;
        });
    };

    const [appExpanded, setAppExpanded] = useState<Record<string, boolean>>({});
    const toggleAppExpanded = (mountId: string) => {
        setAppExpanded(prev => ({ ...prev, [mountId]: !prev[mountId] }));
    };
    const isAppExpanded = (mountId: string) => Boolean(appExpanded[mountId]);

    const onSeatClick = async (seat: RoundtableSeat) => {
        if (!seat.session_id?.trim() || !activeRoundtableRoom.value) return;
        const { openSeatSession: open } = await import('../roundtable/openSeatSession');
        await open(seat, {
            roomId: activeRoundtableRoom.value.id,
            roomTitle: activeRoundtableRoom.value.title || '',
        });
    };

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

    const sessionFilterActive = sessionKindFilter !== 'all' || sessionTimeFilter !== 'all';

    useEffect(() => {
        if (!sessionFilterOpen) return;
        const onDown = (e: MouseEvent) => {
            const el = sessionFilterRef.current;
            if (el && !el.contains(e.target as Node)) setSessionFilterOpen(false);
        };
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setSessionFilterOpen(false);
        };
        document.addEventListener('mousedown', onDown);
        document.addEventListener('keydown', onKey);
        return () => {
            document.removeEventListener('mousedown', onDown);
            document.removeEventListener('keydown', onKey);
        };
    }, [sessionFilterOpen]);

    const hasTaskLinkedSession = chatSessionsSignal.value.some(c => !c.archived && Boolean(c.taskId));
    const folderIdsKey = folders.map(f => f.id).join(',');
    useEffect(() => {
        if (!hasTaskLinkedSession) return;
        let cancelled = false;
        (async () => {
            const titles: Record<string, string> = {};
            for (const folder of folders) {
                if (!chatsForWorkspace(folder.id).some(s => Boolean(s.taskId))) continue;
                try {
                    const tasks = await projectItemService.list(folder.id);
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

    const killingSessionId = useSignal<string | null>(null);
    const [draggedId, setDraggedId] = useState<string | null>(null);
    const [dragOverId, setDragOverId] = useState<string | null>(null);
    const [dragOverPosition, setDragOverPosition] = useState<'before' | 'after' | null>(null);

    const handleDragStart = (e: DragEvent, id: string) => {
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

    const handleSessionKill = (e: MouseEvent, session: Session) => {
        e.stopPropagation();
        killingSessionId.value = session.id;
        setTimeout(() => {
            if (isChat(session)) onChatKill(session.id);
            else onTerminalKill(session.index);
        }, 300);
    };

    // useComputed so @preact/signals reliably re-renders this function component
    // when the chat index or active session changes (void reads were not enough
    // inside the class-shell tree — restored rows only appeared after the next
    // archive forced a folders write / parent paint).
    const chatIndexRev = useComputed(
        () =>
            `${chatSessionsSignal.value.length}:${chatSessionsSignal.value
                .map(c => `${c.id}:${c.archived ? 1 : 0}:${c.lastEventAt || c.createdAt || ''}`)
                .join(
                    ','
                )}:${activeSessionSignal.value && isChat(activeSessionSignal.value) ? activeSessionSignal.value.id : ''}`
    );
    void chatIndexRev.value;

    /** Chat rows from chatSessions (SoT) + terminal rows from folder.sessions.
     *  Global session filters apply in both 任务 and 项目 sections. */
    const sessionsForFolder = (folderId: string, folderSessions: Session[]) => ({
        chats: filterChatsByRules(
            chatsForWorkspace(folderId),
            sessionKindFilter,
            sessionTimeFilter,
            sessionTimeFrom,
            sessionTimeTo
        ),
        terms: filterTermsByKind(terminalsForFolderSessions(folderSessions), sessionKindFilter),
    });

    const openAssistantDetail = (workspaceId: string) => {
        assistantDetailId.value = workspaceId;
        activeDrawerTabSignal.value = 'assistants';
    };

    const renderSession = (
        session: Session,
        opts?: {
            assistantAvatar?: string;
            assistantName?: string;
            withAssistantDetail?: boolean;
        }
    ) => (
        <SessionRow
            key={session.id}
            session={session}
            selected={isSelectedSession(session)}
            killing={killingSessionId.value === session.id}
            taskTitles={taskTitles}
            language={language}
            onSelect={onSelectSession}
            onKill={handleSessionKill}
            onRename={onRenameSession}
            assistantAvatar={opts?.assistantAvatar}
            assistantName={opts?.assistantName}
            onOpenAssistantDetail={
                opts?.withAssistantDetail
                    ? s => {
                          if (isChat(s)) openAssistantDetail(s.workspaceId);
                      }
                    : undefined
            }
        />
    );

    const assistantWorkspaces = workspaces
        .filter(w => (w.kind ?? 'project') === 'workforce' && !w.deviceId)
        .sort((a, b) => {
            if (a.id === 'default') return -1;
            if (b.id === 'default') return 1;
            return 0;
        });
    // 任务区 = workforce ∪ tmp（临时/单次对话）。kind=app（圆桌等应用席位）故意排除。
    const tmpWorkspaces = workspaces.filter(w => w.kind === 'tmp' && !w.deviceId);
    const taskWorkspaceIds = [
        ...assistantWorkspaces.map(w => w.id),
        ...tmpWorkspaces.map(w => w.id),
        'oneshot', // legacy sentinel sessions
    ];
    const assistantById = new Map([...assistantWorkspaces, ...tmpWorkspaces].map(w => [w.id, w] as const));
    const taskChats: ChatSession[] = filterChatsByRules(
        chatsForAssistants(taskWorkspaceIds, taskFilterWsId),
        sessionKindFilter,
        sessionTimeFilter,
        sessionTimeFrom,
        sessionTimeTo
    );
    const showProjects = !isBeginnerMode.value;

    const clearSessionFilters = () => {
        setSessionKindFilter('all');
        setSessionTimeFilter('all');
        setSessionTimeFrom('');
        setSessionTimeTo('');
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
                        <ModeSwitcher language={language} />
                    </div>
                    <div class="brand-right">
                        <button
                            type="button"
                            class={`sidebar-new-session-btn${activeTab === 'new_chat' ? ' active' : ''}`}
                            onClick={onStartNewChat}
                            title={t('sidebar.newSession', language)}
                            aria-label={t('sidebar.newSession', language)}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z" />
                            </svg>
                        </button>
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

                <div class="sidebar-nav-controls">
                    <div
                        class={`nav-control-item${activeDrawerTab === 'assistants' ? ' active' : ''}`}
                        onClick={() => {
                            // 总览：清掉详情 id，落到助理网格，不复现上次点开的助理。
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
                        <span>{t('sidebar.navCtrl.assistantOverview', language)}</span>
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
                        class={`nav-control-item${
                            activeDrawerTab === 'automation' ||
                            activeDrawerTab === 'reminders' ||
                            activeDrawerTab === 'aggregate'
                                ? ' active'
                                : ''
                        }`}
                        onClick={() => toggleDrawerTab('automation')}
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
                            <circle cx="12" cy="12" r="3" />
                            <path d="M12 2v3" />
                            <path d="M12 19v3" />
                            <path d="M4.2 4.2l2.1 2.1" />
                            <path d="M17.7 17.7l2.1 2.1" />
                            <path d="M2 12h3" />
                            <path d="M19 12h3" />
                            <path d="M4.2 19.8l2.1-2.1" />
                            <path d="M17.7 6.3l2.1-2.1" />
                        </svg>
                        <span>{t('sidebar.navCtrl.automation', language)}</span>
                    </div>
                </div>
            </div>

            <div class="sidebar-scroll">
                {!moduleNav ? (
                    <Fragment>
                        {/* ── 任务: 跨助理扁平 chat 列表（recency + 助理头像）── */}
                        <div class={`workspace-section task-section${tasksSectionOpen ? '' : ' is-collapsed'}`}>
                            <div class="section-header">
                                <button
                                    type="button"
                                    class="section-fold-btn"
                                    aria-expanded={tasksSectionOpen}
                                    title={t(
                                        tasksSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                        language
                                    )}
                                    aria-label={t(
                                        tasksSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                        language
                                    )}
                                    onClick={toggleTasksSection}
                                >
                                    <FolderToggleIcon open={tasksSectionOpen} />
                                </button>
                                <button type="button" class="section-header-label" onClick={toggleTasksSection}>
                                    {t('sidebar.section.tasks', language)}
                                </button>
                                <div class="section-header-actions">
                                    <div class="session-filter-host" ref={sessionFilterRef}>
                                        <button
                                            type="button"
                                            class={`section-search-btn${
                                                sessionFilterOpen || sessionFilterActive ? ' active' : ''
                                            }`}
                                            title={t('sidebar.sessionFilter', language)}
                                            aria-label={t('sidebar.sessionFilter', language)}
                                            aria-expanded={sessionFilterOpen}
                                            aria-haspopup="dialog"
                                            onClick={() => setSessionFilterOpen(v => !v)}
                                        >
                                            <svg
                                                viewBox="0 0 24 24"
                                                fill="none"
                                                stroke="currentColor"
                                                stroke-width="2"
                                                stroke-linecap="round"
                                                stroke-linejoin="round"
                                                aria-hidden="true"
                                            >
                                                <polygon points="22 3 2 3 10 12.46 10 19 14 21 14 12.46 22 3" />
                                            </svg>
                                        </button>
                                        {sessionFilterOpen && (
                                            <div
                                                class="session-filter-popover"
                                                role="dialog"
                                                aria-label={t('sidebar.sessionFilter', language)}
                                            >
                                                <div class="session-filter-section">
                                                    <div class="session-filter-label">
                                                        {t('sidebar.sessionFilter.link', language)}
                                                    </div>
                                                    <div class="session-filter-chips" role="group">
                                                        {(
                                                            [
                                                                ['all', 'sidebar.sessionFilter.link.all'],
                                                                ['normal', 'sidebar.sessionFilter.link.normal'],
                                                                ['task', 'sidebar.sessionFilter.link.task'],
                                                                ['terminal', 'sidebar.sessionFilter.link.terminal'],
                                                            ] as const
                                                        ).map(([value, key]) => (
                                                            <button
                                                                key={value}
                                                                type="button"
                                                                class={`session-filter-chip${
                                                                    sessionKindFilter === value ? ' active' : ''
                                                                }`}
                                                                onClick={() => setSessionKindFilter(value)}
                                                            >
                                                                {t(key, language)}
                                                            </button>
                                                        ))}
                                                    </div>
                                                </div>
                                                <div class="session-filter-section">
                                                    <div class="session-filter-label">
                                                        {t('sidebar.sessionFilter.time', language)}
                                                    </div>
                                                    <div class="session-filter-chips" role="group">
                                                        {(
                                                            [
                                                                ['all', 'sidebar.sessionFilter.time.all'],
                                                                ['1h', 'sidebar.sessionFilter.time.1h'],
                                                                ['1d', 'sidebar.sessionFilter.time.1d'],
                                                                ['1w', 'sidebar.sessionFilter.time.1w'],
                                                                ['custom', 'sidebar.sessionFilter.time.custom'],
                                                            ] as const
                                                        ).map(([value, key]) => (
                                                            <button
                                                                key={value}
                                                                type="button"
                                                                class={`session-filter-chip${
                                                                    sessionTimeFilter === value ? ' active' : ''
                                                                }`}
                                                                onClick={() => setSessionTimeFilter(value)}
                                                            >
                                                                {t(key, language)}
                                                            </button>
                                                        ))}
                                                    </div>
                                                    {sessionTimeFilter === 'custom' && (
                                                        <div class="session-filter-range">
                                                            <label class="session-filter-date">
                                                                <span>
                                                                    {t('sidebar.sessionFilter.time.from', language)}
                                                                </span>
                                                                <input
                                                                    type="date"
                                                                    value={sessionTimeFrom}
                                                                    max={sessionTimeTo || undefined}
                                                                    onInput={(e: Event) =>
                                                                        setSessionTimeFrom(
                                                                            (e.target as HTMLInputElement).value
                                                                        )
                                                                    }
                                                                />
                                                            </label>
                                                            <label class="session-filter-date">
                                                                <span>
                                                                    {t('sidebar.sessionFilter.time.to', language)}
                                                                </span>
                                                                <input
                                                                    type="date"
                                                                    value={sessionTimeTo}
                                                                    min={sessionTimeFrom || undefined}
                                                                    onInput={(e: Event) =>
                                                                        setSessionTimeTo(
                                                                            (e.target as HTMLInputElement).value
                                                                        )
                                                                    }
                                                                />
                                                            </label>
                                                        </div>
                                                    )}
                                                </div>
                                                {sessionFilterActive && (
                                                    <button
                                                        type="button"
                                                        class="session-filter-clear"
                                                        onClick={clearSessionFilters}
                                                    >
                                                        {t('sidebar.sessionFilter.clear', language)}
                                                    </button>
                                                )}
                                            </div>
                                        )}
                                    </div>
                                    {tasksSectionOpen && (
                                        <button
                                            type="button"
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
                                    )}
                                </div>
                            </div>
                            {tasksSectionOpen && (
                                <Fragment>
                                    {assistantWorkspaces.length > 0 && (
                                        <div
                                            class="task-filter-chips"
                                            role="tablist"
                                            aria-label={t('sidebar.taskFilter', language)}
                                        >
                                            <button
                                                type="button"
                                                class={`task-filter-chip${taskFilterWsId === null ? ' active' : ''}`}
                                                onClick={() => setTaskFilterWsId(null)}
                                            >
                                                {t('sidebar.taskFilterAll', language)}
                                            </button>
                                            {assistantWorkspaces.map(ws => (
                                                <button
                                                    key={ws.id}
                                                    type="button"
                                                    class={`task-filter-chip${
                                                        taskFilterWsId === ws.id ? ' active' : ''
                                                    }`}
                                                    title={ws.name}
                                                    onClick={() =>
                                                        setTaskFilterWsId(prev => (prev === ws.id ? null : ws.id))
                                                    }
                                                >
                                                    {assistantShortLabel(ws.name)}
                                                    <span class="task-filter-chip-name">{ws.name}</span>
                                                </button>
                                            ))}
                                        </div>
                                    )}
                                    {assistantWorkspaces.length === 0 && taskChats.length === 0 ? (
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
                                    ) : taskChats.length === 0 ? (
                                        <div class="chat-item" style="opacity:0.5;cursor:default;pointer-events:none;">
                                            <div class="chat-item-left">
                                                <span class="chat-title">
                                                    {t(
                                                        sessionFilterActive
                                                            ? 'sidebar.sessionFilter.empty'
                                                            : 'sidebar.noChats',
                                                        language
                                                    )}
                                                </span>
                                            </div>
                                        </div>
                                    ) : (
                                        <div class="task-session-list">
                                            {taskChats.map(session => {
                                                const aw = assistantById.get(session.workspaceId);
                                                const isTmp =
                                                    session.workspaceId === 'oneshot' ||
                                                    session.workspaceId.startsWith('tmp-') ||
                                                    aw?.kind === 'tmp';
                                                return renderSession(session, {
                                                    assistantAvatar: aw?.avatar,
                                                    assistantName: isTmp
                                                        ? aw?.name || t('newchat.kind.oneshot', language)
                                                        : aw?.name || '?',
                                                    withAssistantDetail: !isTmp && aw?.kind === 'workforce',
                                                });
                                            })}
                                        </div>
                                    )}
                                </Fragment>
                            )}
                        </div>

                        {showProjects && (
                            <div class={`workspace-section${projectsSectionOpen ? '' : ' is-collapsed'}`}>
                                <div class="section-header">
                                    <button
                                        type="button"
                                        class="section-fold-btn"
                                        aria-expanded={projectsSectionOpen}
                                        title={t(
                                            projectsSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                            language
                                        )}
                                        aria-label={t(
                                            projectsSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                            language
                                        )}
                                        onClick={toggleProjectsSection}
                                    >
                                        <FolderToggleIcon open={projectsSectionOpen} />
                                    </button>
                                    <button
                                        type="button"
                                        class={`section-header-label${
                                            layoutMode.value === 'project-overview' ? ' active' : ''
                                        }`}
                                        title={t('sidebar.navCtrl.projectOverview', language)}
                                        onClick={() => {
                                            projectOverview();
                                            if (!projectsSectionOpen) {
                                                setProjectsSectionOpen(true);
                                                writeSectionOpen('projects', true);
                                            }
                                        }}
                                    >
                                        {t('sidebar.section.projects', language)}
                                    </button>
                                    {projectsSectionOpen && (
                                        <div class="section-header-actions">
                                            <button
                                                type="button"
                                                class="section-search-btn"
                                                title={t('sidebar.collapseAll', language)}
                                                aria-label={t('sidebar.collapseAll', language)}
                                                onClick={() => {
                                                    const ids = folders
                                                        .filter(
                                                            f =>
                                                                (workspaces.find(w => w.id === f.id)?.kind ??
                                                                    'project') !== 'workforce'
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
                                                type="button"
                                                class={`section-search-btn${projectSearchOpen ? ' active' : ''}`}
                                                title={t('sidebar.searchProjects', language)}
                                                aria-label={t('sidebar.searchProjects', language)}
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
                                    )}
                                </div>
                                {projectsSectionOpen && (
                                    <Fragment>
                                        {projectSearchOpen && (
                                            <div class="section-search-wrap">
                                                <input
                                                    ref={projectSearchRef}
                                                    class="section-search-input"
                                                    type="text"
                                                    placeholder={t('sidebar.searchProjectsPlaceholder', language)}
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

                                        {/* Empty state — project folders only (not assistant / tmp). */}
                                        {!workspacesLoading &&
                                            folders.filter(f => {
                                                const w = workspaces.find(x => x.id === f.id);
                                                return (w?.kind ?? 'project') === 'project';
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
                                                    return (w?.kind ?? 'project') === 'project';
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
                                                    const isActive = folder.id === activeWorkspaceId;

                                                    return (
                                                        <div
                                                            key={folder.id}
                                                            class={`project-node${isActive ? ' ws-active' : ''}`}
                                                        >
                                                            <div
                                                                class={`project-folder ${folder.expanded ? 'expanded' : ''} ${
                                                                    draggedId === folder.id ? 'dragging' : ''
                                                                } ${
                                                                    dragOverId === folder.id &&
                                                                    dragOverPosition === 'before'
                                                                        ? 'drag-over-before'
                                                                        : ''
                                                                } ${
                                                                    dragOverId === folder.id &&
                                                                    dragOverPosition === 'after'
                                                                        ? 'drag-over-after'
                                                                        : ''
                                                                }`}
                                                                onDragOver={e => handleDragOver(e, folder.id)}
                                                                onDragLeave={e => handleDragLeave(e, folder.id)}
                                                                onDrop={e => handleDrop(e, folder.id)}
                                                            >
                                                                <button
                                                                    type="button"
                                                                    class="folder-toggle-btn"
                                                                    aria-expanded={folder.expanded}
                                                                    title={t(
                                                                        folder.expanded
                                                                            ? 'sidebar.collapseProject'
                                                                            : 'sidebar.expandProject',
                                                                        language
                                                                    )}
                                                                    aria-label={t(
                                                                        folder.expanded
                                                                            ? 'sidebar.collapseProject'
                                                                            : 'sidebar.expandProject',
                                                                        language
                                                                    )}
                                                                    onClick={(e: MouseEvent) => {
                                                                        e.stopPropagation();
                                                                        toggleFolder(folder.id);
                                                                    }}
                                                                >
                                                                    <FolderToggleIcon open={folder.expanded} />
                                                                </button>
                                                                <div
                                                                    class="folder-click-area"
                                                                    draggable={true}
                                                                    onDragStart={e => handleDragStart(e, folder.id)}
                                                                    onDragEnd={handleDragEnd}
                                                                    onClick={() => {
                                                                        if (ws) onSelectWorkspace(ws);
                                                                    }}
                                                                >
                                                                    <span
                                                                        class="ws-name"
                                                                        title={ws?.path || folder.name}
                                                                    >
                                                                        {folder.name}
                                                                    </span>
                                                                </div>
                                                                {ws && (
                                                                    <div
                                                                        class="ws-actions"
                                                                        onClick={(e: MouseEvent) => e.stopPropagation()}
                                                                    >
                                                                        <FsRowActionsMenu
                                                                            entry={ws}
                                                                            items={buildFolderActions(
                                                                                ws,
                                                                                onChatCreate,
                                                                                onTerminalCreate,
                                                                                onRenameWorkspace
                                                                            )}
                                                                            language={language}
                                                                            triggerClassName="ws-actions-trigger"
                                                                        />
                                                                    </div>
                                                                )}
                                                            </div>

                                                            {folder.expanded &&
                                                                (() => {
                                                                    const { chats: chatSessions, terms: termSessions } =
                                                                        sessionsForFolder(folder.id, folder.sessions);

                                                                    // One unified list under each workspace: every
                                                                    // 会话 / 终端 session, using the same `.chat-item`
                                                                    // row style (no group headers).
                                                                    return (
                                                                        <div class="project-children">
                                                                            {/* 会话 (chat) + 终端 (terminal) sessions */}
                                                                            {chatSessions.map(s => renderSession(s))}
                                                                            {termSessions.map(s => renderSession(s))}
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
                                                                                isActiveRemote && isTaskView
                                                                                    ? ' active'
                                                                                    : ''
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
                                                                                <span
                                                                                    class="chat-title"
                                                                                    title={rws.path}
                                                                                >
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
                                    </Fragment>
                                )}
                            </div>
                        )}
                        {/* ── 应用 / L1 apps (#332 + open shortcuts) ─────────────────────────
                            Same stage level as sessions: open apps appear as shortcut cards
                            (incl. discovery-only like 圆桌 — now shows seat cards under roundtable
                            folder when expanded; click referee card opens dialogue page).
                            Archive removes the shortcut and exits if active.
                            Permanent L1 mounts still listed when not already open. */}
                        {(() => {
                            const permanent = getL1NavEntries();
                            const opened = openL1Apps.value;
                            const openIds = new Set(opened.map(a => a.mountId));
                            const permanentOnly = permanent.filter(e => !openIds.has(e.id));
                            if (opened.length === 0 && permanentOnly.length === 0) return null;
                            const currentL1Id = activeL1PageId.value;
                            return (
                                <div class={`workspace-section apps-section${appsSectionOpen ? '' : ' is-collapsed'}`}>
                                    <div class="section-header">
                                        <button
                                            type="button"
                                            class="section-fold-btn"
                                            aria-expanded={appsSectionOpen}
                                            title={t(
                                                appsSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                                language
                                            )}
                                            aria-label={t(
                                                appsSectionOpen ? 'sidebar.section.collapse' : 'sidebar.section.expand',
                                                language
                                            )}
                                            onClick={toggleAppsSection}
                                        >
                                            <FolderToggleIcon open={appsSectionOpen} />
                                        </button>
                                        <button type="button" class="section-header-label" onClick={toggleAppsSection}>
                                            {t('sidebar.openApps', language)}
                                        </button>
                                    </div>
                                    {appsSectionOpen && (
                                        <div class="l1-apps-nav">
                                            {opened.map(entry => {
                                                const isRoundtable = entry.appId === 'agents-roundtable';
                                                const isActive = currentL1Id === entry.mountId;
                                                const expanded = isAppExpanded(entry.mountId);
                                                return (
                                                    <div
                                                        key={entry.mountId}
                                                        class={`l1-open-app-row${isActive ? ' is-active' : ''} ${expanded ? 'expanded' : ''}`}
                                                    >
                                                        <button
                                                            type="button"
                                                            class="l1-open-app-main"
                                                            title={entry.label}
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                if (isRoundtable) {
                                                                    requestRoundtableListView();
                                                                    if (!isActive) {
                                                                        enterL1App(entry.mountId);
                                                                        toggleAppExpanded(entry.mountId);
                                                                    }
                                                                } else if (isActive) {
                                                                    exitL1App();
                                                                } else {
                                                                    enterL1App(entry.mountId);
                                                                    toggleAppExpanded(entry.mountId);
                                                                }
                                                            }}
                                                        >
                                                            <FolderToggleIcon open={expanded} />
                                                            {!isRoundtable ? (
                                                                <span class="l1-nav-item-icon" aria-hidden="true">
                                                                    {entry.icon || '◇'}
                                                                </span>
                                                            ) : null}
                                                            <span class="l1-nav-item-label">{entry.label}</span>
                                                        </button>
                                                        <button
                                                            type="button"
                                                            class="l1-open-app-archive"
                                                            title={t('sidebar.archiveApp', language)}
                                                            aria-label={t('sidebar.archiveApp', language)}
                                                            onClick={(e: MouseEvent) => {
                                                                e.stopPropagation();
                                                                archiveL1App(entry.mountId);
                                                            }}
                                                        >
                                                            <svg
                                                                viewBox="0 0 24 24"
                                                                fill="none"
                                                                stroke="currentColor"
                                                                stroke-width="2"
                                                                stroke-linecap="round"
                                                                stroke-linejoin="round"
                                                                width="14"
                                                                height="14"
                                                            >
                                                                <polyline points="3 6 5 6 21 6" />
                                                                <path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2" />
                                                            </svg>
                                                        </button>
                                                        {isRoundtable && expanded && (
                                                            <div class="rt-side-seats">
                                                                {activeRoundtableSeats.value.map(seat => (
                                                                    <button
                                                                        key={seat.id}
                                                                        type="button"
                                                                        class="rt-side-seat is-clickable bento-card"
                                                                        onClick={() => onSeatClick(seat)}
                                                                        title={`打开「${roleLabel(seat.role)}」完整讨论 · ${seatStatusLabel(seatDisplayStatus(seat))}`}
                                                                    >
                                                                        <span
                                                                            class="rt-seat-dot is-idle"
                                                                            aria-hidden="true"
                                                                        />
                                                                        <span class="rt-side-seat-name">
                                                                            {roleLabel(seat.role)}
                                                                        </span>
                                                                    </button>
                                                                ))}
                                                            </div>
                                                        )}
                                                    </div>
                                                );
                                            })}
                                            {permanentOnly.map(entry => (
                                                <L1NavItem
                                                    key={entry.id}
                                                    entry={entry}
                                                    isActive={currentL1Id === entry.id}
                                                    onClick={() => {
                                                        if (entry.appId === 'agents-roundtable') {
                                                            requestRoundtableListView();
                                                        }
                                                        if (currentL1Id === entry.id) {
                                                            exitL1App();
                                                        } else {
                                                            enterL1App(entry.id);
                                                        }
                                                    }}
                                                />
                                            ))}
                                        </div>
                                    )}
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
