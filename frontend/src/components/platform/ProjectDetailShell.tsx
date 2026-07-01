import { h } from 'preact';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as stage from '../../stores/stageStore';
import { ProjectShell } from './ProjectShell';
import { ShellNav } from './ShellNav';

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
            <ShellNav
                crumbs={[
                    { label: t('projectHome.title', language), onClick: () => stage.projectOverview() },
                    { label: top.name },
                ]}
            />
            <div class="project-detail-body">
                <ProjectShell workspaceId={top.workspaceId} workspaceName={top.name} />
            </div>
        </div>
    );
}
