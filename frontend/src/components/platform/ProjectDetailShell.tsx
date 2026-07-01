import { h } from 'preact';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as stage from '../../stores/stageStore';
import { ProjectShell } from './ProjectShell';

/**
 * 项目详情 — the drilled-in project page (#redesign). A breadcrumb (项目总览 →
 * project) over the shared ProjectShell (动态/计划/任务/资产 + project-tab apps).
 * Rendered by DesktopAppLayout when `layoutMode === 'project'`. Drilling further
 * (opening a conversation from a panel) flips to the split workbench.
 */
export function ProjectDetailShell() {
    const language = ui.language.value;
    const stack = stage.projectStack.value;
    const top = stack[stack.length - 1];
    if (!top) return null;

    return (
        <div class="project-detail-shell">
            <div class="project-detail-breadcrumb">
                <button class="project-crumb-link" onClick={() => stage.projectOverview()}>
                    {t('projectHome.title', language)}
                </button>
                <svg
                    class="project-crumb-sep"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <polyline points="9 6 15 12 9 18" />
                </svg>
                <span class="project-crumb-current">{top.name}</span>
            </div>
            <div class="project-detail-body">
                <ProjectShell workspaceId={top.workspaceId} workspaceName={top.name} />
            </div>
        </div>
    );
}
