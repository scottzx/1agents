import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as modal from '../../stores/modalStore';
import * as stage from '../../stores/stageStore';
import * as tabsStore from '../../stores/tabsStore';
import * as appStore from '../../stores/appManifestStore';
import { StatusRow } from '../shared/primitives';

/**
 * 项目总览 — the 项目 context landing (L1). Codex-minimal, matching the 助理
 * 概览: an in-page breadcrumb (项目), a lean toolbar (count + 新建项目 / 大屏
 * pills), then flat card grids for 我的项目 (drill into the detail page) and
 * 从模版创建. Rendered by DesktopAppLayout when `layoutMode === 'project-overview'`.
 */
export function ProjectHome() {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const apps = appStore.appManifests.value;
    const [search, setSearch] = useState('');
    const [archivedOpen, setArchivedOpen] = useState(false);

    // Real projects only — the builtin 对话/助手 workspace is not a project.
    const projects = workspaces
        .filter(w => !w.builtin && w.id !== 'default' && (w.kind ?? 'project') === 'project')
        .filter(w => !search || w.name.toLowerCase().includes(search.toLowerCase()));

    // 已归档 board (loaded on demand; kept out of the sidebar).
    useEffect(() => {
        void wsStore.loadArchivedWorkspaces();
    }, []);
    const archivedProjects = wsStore.archivedWorkspaces.value.filter(
        w => !w.deviceId && (w.kind ?? 'project') === 'project'
    );

    // 大屏: desktop opens an in-app tab, the web opens a new browser tab.
    const openBigScreen = () => {
        const url = window.location.origin + (__BASE_PATH__ || '') + '/dashboard';
        if (IS_DESKTOP) {
            tabsStore.openBrowserTab(url);
        } else {
            window.open(url, '_blank', 'noopener');
        }
    };

    const FolderIcon = (
        <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="1.8"
            stroke-linecap="round"
            stroke-linejoin="round"
        >
            <circle cx="18" cy="18" r="3" />
            <circle cx="6" cy="6" r="3" />
            <path d="M6 21V9a9 9 0 0 0 9 9" />
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
        >
            <rect x="3" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="3" width="7" height="7" rx="1" />
            <rect x="14" y="14" width="7" height="7" rx="1" />
            <rect x="3" y="14" width="7" height="7" rx="1" />
        </svg>
    );

    return (
        <div class="project-home">
            <div class="project-home-scroll">
                <div class="assistants-toolbar">
                    <p class="assistants-subtitle">
                        {projects.length > 0
                            ? t('projectHome.count', language, { count: String(projects.length) })
                            : t('projectHome.subtitle', language)}
                    </p>
                    <div class="project-home-toolbar-actions">
                        <button class="assistant-btn assistant-btn-ghost" onClick={openBigScreen}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <rect x="2" y="3" width="20" height="14" rx="2" />
                                <line x1="8" y1="21" x2="16" y2="21" />
                                <line x1="12" y1="17" x2="12" y2="21" />
                            </svg>
                            <span>{t('projectHome.bigScreen', language)}</span>
                        </button>
                        <button class="assistant-btn assistant-btn-primary" onClick={modal.openCreateWorkspacePicker}>
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
                            <span>{t('projectHome.newProject', language)}</span>
                        </button>
                    </div>
                </div>

                <section class="project-home-section">
                    <div class="project-home-section-head">
                        <h2 class="project-home-section-title">{t('projectHome.myProjects', language)}</h2>
                        <input
                            class="project-home-search"
                            type="text"
                            placeholder={t('projectHome.searchPlaceholder', language)}
                            value={search}
                            onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                        />
                    </div>
                    {projects.length === 0 ? (
                        <div class="assistants-empty">{t('projectHome.empty', language)}</div>
                    ) : (
                        <div class="project-home-list">
                            {projects.map(ws => (
                                <StatusRow
                                    key={ws.id}
                                    icon={FolderIcon}
                                    title={ws.name}
                                    summary={ws.path}
                                    onClick={() => stage.enterProjectDetail(ws.id, ws.name)}
                                />
                            ))}
                        </div>
                    )}
                </section>

                {archivedProjects.length > 0 && (
                    <section class="project-home-section">
                        <button
                            class={`archived-section-head is-toggle${archivedOpen ? ' is-open' : ''}`}
                            onClick={() => setArchivedOpen(v => !v)}
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
                            <div class="project-home-list">
                                {archivedProjects.map(ws => (
                                    <StatusRow
                                        key={ws.id}
                                        icon={FolderIcon}
                                        title={ws.name}
                                        summary={t('overview.archivedTag', language)}
                                        onClick={() => stage.enterProjectDetail(ws.id, ws.name)}
                                    />
                                ))}
                            </div>
                        )}
                    </section>
                )}

                {apps.length > 0 && (
                    <section class="project-home-section">
                        <div class="project-home-section-head">
                            <h2 class="project-home-section-title">{t('projectHome.templates', language)}</h2>
                        </div>
                        <div class="project-home-list">
                            {apps.map(app => (
                                <StatusRow
                                    key={app.id}
                                    icon={TemplateIcon}
                                    title={app.name}
                                    summary={t('projectHome.templateLabel', language)}
                                    onClick={modal.openCreateWorkspacePicker}
                                />
                            ))}
                        </div>
                    </section>
                )}
            </div>
        </div>
    );
}
