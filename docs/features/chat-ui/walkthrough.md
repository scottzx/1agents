# Walkthrough: Task Board Tab Migration

We have successfully migrated the Task Board (任务看板) from a right-sliding drawer panel to a dedicated, responsive, full-screen tab in the main content area.

## Key Changes Made

### 1. Tab Configurations and Routing
- **[types.ts](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/types.ts)**: Added `'tasks'` to the `Tab` type union.
- **[app.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/app.tsx)**: 
  - Initialized both the Workbench (`terminal`) and the Task Board (`tasks`) as default, permanent, non-closable tabs in the constructor.
  - Handled automated language lookup for translating tab titles.
  - Intercepted the header drawer trigger for tasks (`toggleDrawerTab('tasks')`) and mapped it to select the Tasks tab (`selectTab('tasks')`).
  - Configured `selectSession` to automatically switch the layout active tab back to `'terminal'` (Workbench), so entering any session from the Task Board instantly reveals the terminal/chat and sidebar context.

### 2. Layout Integration (Desktop & Mobile)
- **[DesktopAppLayout.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/desktop/DesktopAppLayout.tsx)**: Rendered the `TaskList` component in the main content area inside the full-screen dynamic tab view when `activeTabObj?.type === 'tasks'`, hiding the LeftSidebar and RightPanel.
- **[MobileAppLayout.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/mobile/MobileAppLayout.tsx)**: Rendered the `TaskList` in a full-screen sub-view layer on mobile when the tasks tab is active, including a header back button to select `'terminal'`. Selecting a session redirects back to the Workbench.
- **[RightPanel.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/drawer/RightPanel.tsx)**: Removed the drawer rendering cases and unused variables related to `TaskList` to clean up dead code.

### 3. Styling & Responsive Layout
- **[TaskList.tsx](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/components/drawer/TaskList.tsx)**: Renamed the outer container class to `.task-dashboard-container`.
- **[index.scss](file:///Users/scott/Documents/01-开发项目/Web应用/1agents/html/src/style/index.scss)**: Added a responsive CSS Grid system to lay out task cards horizontally across the screen on desktop viewports, while falling back to a single-column card list on mobile screens.

## Verification Checklist

1. **Build Verification**:
   - Compiles and builds frontend assets (`make frontend`) and Go backend (`make backend`) successfully.

2. **Desktop Layout**:
   - Open the web interface. A "任务看板" tab will be pinned next to "工作台" at the top left.
   - Click "任务看板": The LeftSidebar hides, and the main area displays the responsive grid layout of task cards.
   - Click "进入" on a session inside a task card: The view switches back to "工作台" tab, the sidebar slides open, and the chat session loads inside the main workspace canvas.

3. **Mobile Layout**:
   - Switch to mobile mode or narrow the window.
   - Click the "任务仪表盘" item in the top menu list.
   - A full-screen sub-view appears showing the list of tasks.
   - Tap "进入" on any session: the app returns to the Workbench view and opens that session.
