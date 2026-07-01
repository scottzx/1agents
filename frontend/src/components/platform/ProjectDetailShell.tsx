import { h } from 'preact';

import { t } from '../../i18n';
import * as ui from '../../stores/uiStore';
import * as stage from '../../stores/stageStore';
import { ProjectShell } from './ProjectShell';

/**
 * 项目详情 — the drilled-in project page (#redesign). A thin adapter that feeds
 * the drill stack into ProjectShell as a breadcrumb; ProjectShell's ShellNav
 * renders one unified bar (项目总览 → project + 动态/计划/任务/资产 + gear).
 * Rendered by DesktopAppLayout when `layoutMode === 'project'`. Drilling further
 * (opening a conversation from a panel) flips to the split workbench.
 */
export function ProjectDetailShell() {
    const language = ui.language.value;
    const stack = stage.projectStack.value;
    const top = stack[stack.length - 1];
    if (!top) return null;

    return (
        <ProjectShell
            workspaceId={top.workspaceId}
            workspaceName={top.name}
            crumbs={[
                { label: t('projectHome.title', language), onClick: () => stage.projectOverview() },
                { label: top.name },
            ]}
        />
    );
}
