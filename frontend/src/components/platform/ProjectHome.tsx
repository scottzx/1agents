import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as modal from '../../stores/modalStore';
import * as stage from '../../stores/stageStore';
import * as tabsStore from '../../stores/tabsStore';
import * as appStore from '../../stores/appManifestStore';
import { getStatusLabel, type Workspace } from '../types';

/**
 * 项目总览 — L1 project registry (card wall).
 *
 * Layout (codex-minimal / Bento):
 *   toolbar  → count + 大屏 / 新建
 *   我的项目 → searchable bento grid of project cards
 *   已归档   → collapsible secondary grid
 *   模版     → quieter template grid
 *
 * Rendered by DesktopAppLayout when `layoutMode === 'project-overview'`.
 */
export function ProjectHome() {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const folders = wsStore.folders.value;
    const apps = appStore.appManifests.value;
    const [search, setSearch] = useState('');
    const [archivedOpen, setArchivedOpen] = useState(false);

    // Real projects only — the builtin 对话/助手 workspace is not a project.
    const allProjects = workspaces.filter(w => !w.builtin && w.id !== 'default' && (w.kind ?? 'project') === 'project');
    const projects = allProjects.filter(w => !search || w.name.toLowerCase().includes(search.toLowerCase()));

    // 已归档 board (loaded on demand; kept out of the sidebar).
    useEffect(() => {
        void wsStore.loadArchivedWorkspaces();
    }, []);
    const archivedProjects = wsStore.archivedWorkspaces.value.filter(
        w => !w.deviceId && (w.kind ?? 'project') === 'project'
    );

    const sessionCountFor = (wsId: string): number => {
        const folder = folders.find(f => f.id === wsId);
        return folder ? folder.sessions.length : 0;
    };

    // 大屏: desktop opens an in-app tab, the web opens a new browser tab.
    const openBigScreen = () => {
        const url = window.location.origin + (__BASE_PATH__ || '') + '/dashboard';
        tabsStore.openBrowserTab(url);
    };

    const FolderIcon = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
        >
            <path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7z" />
        </svg>
    );
    const TemplateIcon = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
        >
            <rect x="3" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="14" width="7" height="7" rx="1" />
            <rect x="3" y="14" width="7" height="7" rx="1" />
        </svg>
    );
    const ChevronIcon = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
        >
            <polyline points="9 6 15 12 9 18" />
        </svg>
    );

    const renderProjectCard = (ws: Workspace, opts?: { archived?: boolean }) => {
        const status = (ws.status || 'active') as string;
        const showStatus = opts?.archived || (status !== 'active' && status !== '');
        const sessions = sessionCountFor(ws.id);
        const pathLeaf = ws.path ? ws.path.replace(/\/+$/, '').split(/[/\\]/).pop() || ws.path : '';

        return (
            <button
                key={ws.id}
                type="button"
                class={`project-card${opts?.archived ? ' is-archived' : ''}`}
                onClick={() => stage.enterProjectDetail(ws.id, ws.name)}
            >
                <div class="bento-zone-header">
                    <div class="project-card-icon" aria-hidden="true">
                        {FolderIcon}
                    </div>
                    {showStatus && (
                        <span
                            class={`project-card-badge${
                                opts?.archived
                                    ? ' is-archived'
                                    : status === 'planning'
                                      ? ' is-planning'
                                      : status === 'inactive'
                                        ? ' is-inactive'
                                        : ''
                            }`}
                        >
                            {opts?.archived ? t('overview.archivedTag', language) : getStatusLabel(status, language, t)}
                        </span>
                    )}
                </div>
                <div class="bento-zone-body">
                    <h3 class="bento-card-title project-card-name">{ws.name}</h3>
                    {ws.path && (
                        <p class="project-card-path" title={ws.path}>
                            {pathLeaf}
                        </p>
                    )}
                </div>
                <div class="bento-zone-footer">
                    <span class="project-card-meta">
                        {t('projectHome.sessions', language)} · {sessions}
                    </span>
                    <span class="project-card-action">
                        <span>{t('projectHome.open', language)}</span>
                        {ChevronIcon}
                    </span>
                </div>
            </button>
        );
    };

    return (
        <div class="project-home">
            <div class="project-home-scroll">
                <div class="assistants-toolbar">
                    <p class="assistants-subtitle">
                        {allProjects.length > 0
                            ? t('projectHome.count', language, { count: String(allProjects.length) })
                            : t('projectHome.subtitle', language)}
                    </p>
                    <div class="project-home-toolbar-actions">
                        <button type="button" class="assistant-btn assistant-btn-ghost" onClick={openBigScreen}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <rect x="2" y="3" width="20" height="14" rx="2" />
                                <line x1="8" y1="21" x2="16" y2="21" />
                                <line x1="12" y1="17" x2="12" y2="21" />
                            </svg>
                            <span>{t('projectHome.bigScreen', language)}</span>
                        </button>
                        <button
                            type="button"
                            class="assistant-btn assistant-btn-primary"
                            onClick={modal.openCreateWorkspacePicker}
                        >
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <line x1="12" y1="5" x2="12" y2="19" />
                                <line x1="5" y1="12" x2="19" y2="12" />
                            </svg>
                            <span>{t('projectHome.newProject', language)}</span>
                        </button>
                    </div>
                </div>

                <section class="project-home-section">
                    <div class="project-home-section-head">
                        <h2 class="project-home-section-title">{t('projectHome.myProjects', language)}</h2>
                        {allProjects.length > 0 && (
                            <input
                                class="project-home-search"
                                type="search"
                                placeholder={t('projectHome.searchPlaceholder', language)}
                                value={search}
                                onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                                aria-label={t('projectHome.searchPlaceholder', language)}
                            />
                        )}
                    </div>
                    {allProjects.length === 0 ? (
                        <div class="assistants-empty">
                            <span>{t('projectHome.empty', language)}</span>
                            <button
                                type="button"
                                class="assistant-btn assistant-btn-ghost"
                                onClick={modal.openCreateWorkspacePicker}
                            >
                                {t('projectHome.newProject', language)}
                            </button>
                        </div>
                    ) : projects.length === 0 ? (
                        <div class="project-home-empty-filter">{t('projectHome.emptySearch', language)}</div>
                    ) : (
                        <div class="project-grid">{projects.map(ws => renderProjectCard(ws))}</div>
                    )}
                </section>

                {archivedProjects.length > 0 && (
                    <section class="project-home-section">
                        <button
                            type="button"
                            class={`archived-section-head is-toggle${archivedOpen ? ' is-open' : ''}`}
                            onClick={() => setArchivedOpen(v => !v)}
                            aria-expanded={archivedOpen}
                        >
                            <svg
                                class="archived-chevron"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <polyline points="9 6 15 12 9 18" />
                            </svg>
                            <span>
                                {t('overview.archived', language)} ({archivedProjects.length})
                            </span>
                        </button>
                        {archivedOpen && (
                            <div class="project-grid">
                                {archivedProjects.map(ws => renderProjectCard(ws, { archived: true }))}
                            </div>
                        )}
                    </section>
                )}

                {apps.length > 0 && (
                    <section class="project-home-section project-home-section-templates">
                        <div class="project-home-section-head">
                            <h2 class="project-home-section-title">{t('projectHome.templates', language)}</h2>
                        </div>
                        <div class="project-grid">
                            {apps.map(app => (
                                <button
                                    key={app.id}
                                    type="button"
                                    class="project-card is-template"
                                    onClick={modal.openCreateWorkspacePicker}
                                >
                                    <div class="bento-zone-header">
                                        <div class="project-card-icon is-template" aria-hidden="true">
                                            {TemplateIcon}
                                        </div>
                                        <span class="project-card-badge is-template">
                                            {t('projectHome.templateLabel', language)}
                                        </span>
                                    </div>
                                    <div class="bento-zone-body">
                                        <h3 class="bento-card-title project-card-name">{app.name}</h3>
                                        <p class="bento-card-desc project-card-desc">
                                            {t('projectHome.templateHint', language)}
                                        </p>
                                    </div>
                                    <div class="bento-zone-footer">
                                        <span class="project-card-meta">{t('projectHome.useTemplate', language)}</span>
                                        <span class="project-card-action">{ChevronIcon}</span>
                                    </div>
                                </button>
                            ))}
                        </div>
                    </section>
                )}
            </div>
        </div>
    );
}
