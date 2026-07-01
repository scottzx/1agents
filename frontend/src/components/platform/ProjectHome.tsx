import { h } from 'preact';
import { useState } from 'preact/hooks';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as modal from '../../stores/modalStore';
import * as stage from '../../stores/stageStore';
import * as tabsStore from '../../stores/tabsStore';
import * as appStore from '../../stores/appManifestStore';

/**
 * 项目总览 — the 项目 context landing (#redesign). A WorkBuddy-style card wall:
 * hero + 新建项目 + 大屏跳转, then 我的项目 (existing projects → drill into the
 * detail page) and 从模版创建 (installed domain apps as project templates).
 * Rendered by DesktopAppLayout when `layoutMode === 'project-overview'`.
 */
export function ProjectHome() {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const apps = appStore.appManifests.value;
    const [search, setSearch] = useState('');

    // Real projects only — the builtin 对话/助手 workspace is not a project.
    const projects = workspaces
        .filter(w => !w.builtin && w.id !== 'default')
        .filter(w => !search || w.name.toLowerCase().includes(search.toLowerCase()));

    // 大屏: desktop opens an in-app tab, the web opens a new browser tab.
    const openBigScreen = () => {
        const url = window.location.origin + (__BASE_PATH__ || '') + '/dashboard';
        if (IS_DESKTOP) {
            tabsStore.openBrowserTab(url);
        } else {
            window.open(url, '_blank', 'noopener');
        }
    };

    return (
        <div class="project-home">
            <div class="project-home-hero">
                <div class="project-home-hero-text">
                    <h1 class="project-home-title">{t('projectHome.title', language)}</h1>
                    <p class="project-home-subtitle">{t('projectHome.subtitle', language)}</p>
                    <div class="project-home-hero-actions">
                        <button class="project-home-new-btn" onClick={modal.openCreateWorkspacePicker}>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                            >
                                <line x1="12" y1="5" x2="12" y2="19" />
                                <line x1="5" y1="12" x2="19" y2="12" />
                            </svg>
                            <span>{t('projectHome.newProject', language)}</span>
                        </button>
                        <button class="project-home-bigscreen-btn" onClick={openBigScreen}>
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
                    </div>
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
                    <div class="project-home-empty">{t('projectHome.empty', language)}</div>
                ) : (
                    <div class="bento-grid">
                        {projects.map(ws => (
                            <div
                                key={ws.id}
                                class="bento-card project-home-card"
                                onClick={() => stage.enterProjectDetail(ws.id, ws.name)}
                            >
                                <div class="project-home-card-icon">
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
                                </div>
                                <div class="project-home-card-body">
                                    <div class="project-home-card-name">{ws.name}</div>
                                    <div class="project-home-card-desc" title={ws.path}>
                                        {ws.path}
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </section>

            {apps.length > 0 && (
                <section class="project-home-section">
                    <div class="project-home-section-head">
                        <h2 class="project-home-section-title">{t('projectHome.templates', language)}</h2>
                    </div>
                    <div class="bento-grid">
                        {apps.map(app => (
                            <div
                                key={app.id}
                                class="bento-card project-home-card"
                                onClick={modal.openCreateWorkspacePicker}
                            >
                                <div class="project-home-card-icon">
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
                                </div>
                                <div class="project-home-card-body">
                                    <div class="project-home-card-name">{app.name}</div>
                                    <div class="project-home-card-desc">{t('projectHome.templateLabel', language)}</div>
                                </div>
                            </div>
                        ))}
                    </div>
                </section>
            )}
        </div>
    );
}
