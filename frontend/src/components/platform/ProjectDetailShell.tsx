import { h } from 'preact';
import { useEffect } from 'preact/hooks';

import type { App } from '../app';
import * as stage from '../../stores/stageStore';
import * as taskNav from '../../stores/taskNavStore';
import { ProjectShell } from './ProjectShell';

/**
 * 项目详情 — the drilled-in project page (#redesign). A thin adapter that
 * feeds the drill stack into ProjectShell. Breadcrumbs live in WorkspaceHeader
 * (DesktopAppLayout passes customCrumbs), so ShellNav gets no crumbs here.
 * Rendered by DesktopAppLayout when `layoutMode === 'project'`. Drilling
 * further (opening a conversation) flips to the split workbench.
 */
export function ProjectDetailShell({ app }: { app?: App }) {
    const stack = stage.projectStack.value;
    const top = stack[stack.length - 1];

    useEffect(() => {
        if (!top) return;
        taskNav.clearHeaderBackActions();
        return taskNav.registerHeaderBackAction(
            'project-detail',
            stage.projectOverview,
            taskNav.HEADER_BACK_PRIORITY.surface
        );
    }, [top?.workspaceId]);

    if (!top) return null;

    return <ProjectShell workspaceId={top.workspaceId} workspaceName={top.name} variant="detail" app={app} />;
}
