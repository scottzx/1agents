# Task Board Integration as Main Panel Tab

This plan details the implementation of moving the Task Board (任务看板) from the right sliding drawer to a dedicated full-screen tab in the main workspace content area.

## User Review Required

> [!NOTE]
> The Task Board will be initialized as a permanent, non-closable tab (titled "任务看板" / "Tasks") next to the "工作台" tab.
> 
> When entering a session from the Task Board, the view switches back to the "工作台" tab and activates the selected AI chat session.

## Proposed Changes

### Frontend Configurations and Localization

#### [MODIFY] [types.ts](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/types.ts)
- Update `Tab` type definition to include `'tasks'` as a valid tab type.

#### [MODIFY] [dict.ts](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/i18n/dict.ts)
- Add translation keys `'app.tab.tasks'` mapping to `'任务看板'` in Chinese and `'Tasks'` in English.

---

### Core State & Route Toggling

#### [MODIFY] [app.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/app.tsx)
- Initialize the `tabs` array in the constructor with both "工作台" (Terminal/Workbench) and "任务看板" (Tasks) as permanent, non-closable tabs:
  ```typescript
  tabs: [
      { id: 'terminal', title: t('app.tab.workbench', 'zh-CN'), type: 'terminal', closable: false },
      { id: 'tasks', title: t('app.tab.tasks', 'zh-CN'), type: 'tasks', closable: false }
  ]
  ```
- Update the `toggleDrawerTab` method so that when a `'tasks'` tab activation is requested (e.g., via a header click), it intercepts and switches to the `'tasks'` Tab:
  ```typescript
  toggleDrawerTab = (tab: RightDrawerTab) => {
      if (tab === 'tasks') {
          this.selectTab('tasks');
          return;
      }
      // rest of toggleDrawerTab code...
  ```

---

### Layout Integration (Desktop & Mobile)

#### [MODIFY] [DesktopAppLayout.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/desktop/DesktopAppLayout.tsx)
- Import `TaskList` from `../drawer/TaskList`.
- Render the `TaskList` component inside the `.workspace-body-container.dynamic-tab-view` when `activeTabObj?.type === 'tasks'`:
  ```typescript
  {activeTabObj?.type === 'tasks' && (
      <div class="tasks-tab-container" style="flex: 1; height: 100%; display: flex; flex-direction: column; overflow: hidden; background-color: var(--bg-panel); padding: 12px 16px;">
          <TaskList
              workspaceId={activeWorkspaceId}
              onSelectSession={(session) => {
                  app.selectTab('terminal');
                  app.selectSession(session);
              }}
          />
      </div>
  )}
  ```

#### [MODIFY] [MobileAppLayout.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/mobile/MobileAppLayout.tsx)
- Import `TaskList` from `../drawer/TaskList`.
- Add tasks view rendering under the mobile subview layer:
  ```typescript
  {activeTabObj?.type === 'tasks' && (
      <div class="mobile-subview-layout">
          <div class="mobile-subview-header">
              <button class="mobile-subview-back-btn" onClick={() => app.selectTab('terminal')}>
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor">
                      <polyline points="15 18 9 12 15 6" />
                  </svg>
              </button>
              <div class="mobile-subview-title">{t('app.tab.tasks', language)}</div>
          </div>
          <div class="mobile-subview-content scrollable" style="background-color: var(--bg-panel); padding: 12px 16px;">
              <TaskList
                  workspaceId={selectedWorkspaceId || activeWorkspaceId}
                  onSelectSession={(session) => {
                      app.selectSession(session);
                      app.selectTab('terminal');
                      this.setState({ inSessionView: true });
                  }}
              />
          </div>
      </div>
  )}
  ```

---

### Task Component & Styling Improvements

#### [MODIFY] [RightPanel.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/drawer/RightPanel.tsx)
- Remove the rendering of `TaskList` from the sliding drawer cases since it has been moved to the main panel tab.

#### [MODIFY] [TaskList.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/drawer/TaskList.tsx)
- Adjust the layout container classes or styles of `TaskList` to use responsive classes.
- Wrap the outer container in class `.task-dashboard-container` instead of `.task-dashboard-drawer` to distinguish from drawer-only styles.
- Add CSS Grid wrapper to lay out tasks in a grid system for desktop screen sizes.

#### [MODIFY] [index.scss](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/style/index.scss)
- Add responsive grid styles for `.task-dashboard-container` using `@media` queries:
  ```scss
  .task-dashboard-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    padding: 24px;
    overflow-y: auto;
    
    .task-list-scroller {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
      gap: 16px;
      padding-top: 16px;
    }
    
    @media (max-width: 768px) {
      padding: 12px;
      .task-list-scroller {
        grid-template-columns: 1fr;
        gap: 12px;
      }
    }
  }
  ```

## Verification Plan

### Manual Verification
1. Run the local dev server using `yarn start` inside the `html/` directory.
2. Verify that two permanent tabs ("工作台" and "任务看板") are visible at the top left of the dashboard on desktop.
3. Click "任务看板" tab: Verify that the task dashboard replaces the workspace terminal/chat, hiding the sidebar and displaying a responsive grid of task cards.
4. Test task creation: Verify that the new task form renders and works.
5. In a task card, click "进入" for an existing session or create a new session: Verify that the app automatically switches back to the "工作台" tab, displays the LeftSidebar, and loads the active chat session.
6. Verify mobile view responsiveness: shrink browser or simulate mobile device. Tap "任务仪表盘" in the header slide-down menu. Verify that a full-screen sub-view with "任务看板" and a back button appears. Click a session to verify it goes back to the workbench and loads the chat session.
