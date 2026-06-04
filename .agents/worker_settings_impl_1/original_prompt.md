## 2026-06-04T13:26:52Z

You are the Settings Implementation Worker.
Your working directory is `/Users/scott/Documents/01-开发项目/Web应用/1agents/.agents/worker_settings_impl_1`. Maintain your plan.md, progress.md, and context.md files there.

Your task is to implement the settings refactoring according to the following requirements:

R1. Remove Settings from Header and Mobile hamburger:
- Remove settings button (`hdr-btn-settings`) from desktop header in `html/src/components/header/WorkspaceHeader.tsx`.
- Remove settings menu item (`mob-menu-settings`) from mobile menu drawer in `html/src/components/header/WorkspaceHeader.tsx`.
- Delete the unused `IconSettings` variable in `WorkspaceHeader.tsx`.

R2. Full-Page Overlay Mode for Settings:
- Update `isFullPageTab` in `html/src/components/types.ts` to return `true` for `'settings'`.
- In `html/src/components/app.tsx`, import `ThemeSettings` from `./drawer/ThemeSettings` and render it inside the full-page container (inside `isFullPageTab(activeDrawerTab)` block) when `activeDrawerTab === 'settings'`.
- Update `getHeaderTitle` in `WorkspaceHeader.tsx` to return `'系统设置'` when `tab === 'settings'`.
- Remove `<ThemeSettings ... />` from `html/src/components/drawer/RightPanel.tsx`.

R3. Build a Premium Split-Column Settings Panel in `html/src/components/drawer/ThemeSettings.tsx`:
- Entirely replace the file contents of `ThemeSettings.tsx` to build a premium split-column settings panel (left column for categories, right column for content detail).
- The settings categories are:
  1. General Settings (通用设置): Language selection (中文 / English), voice dictation language. Also display current workspace name, path, and ID.
  2. Appearance & Terminal (外观与终端): Theme toggle (Light/Dark), Tmux mouse behavior toggle ("滚轮滑动" / "选择复制" which calls `toggleTmuxMouse` prop), and a terminal font-size customizer.
  3. Security Settings (安全设置): Access token (Generate/Revoke) control.
  4. System Maintenance & Info (关于与维护): App info (Name: 1agents, Version: v1.0.0, branding logo: `/logo.png`) and Reset Application Cache button.
- Manage Terminal Font Size:
  - Add `terminalFontSize` to `AppState` and `this.state` in `app.tsx` (persisted to `localStorage` key `'1agents-terminal-font-size'`, defaulting to 12 on mobile and 13 on desktop).
  - Update `termOptions` in `app.tsx` to use `this.state.terminalFontSize` instead of the hardcoded size.
  - Pass `terminalFontSize` and a callback `onUpdateTerminalFontSize` to `ThemeSettings` so the font size can be adjusted.
- Manage Reset Application Cache:
  - When the Reset button is clicked, confirm with a dialog. If confirmed, clear `localStorage` keys (`1agents-theme`, `1agents-language`, `1agents-active-workspace`, `fav-files`, `1agents-onboarded`, `1agents-terminal-font-size`), empty Service Worker caches if `caches` exists, empty parent's `_workspaceTreeCache`, and call `window.location.reload()`.
- System Diagnostics:
  - Under System Maintenance, perform a connection test to the backend by fetching `/api/access/status` and display connection status (Connected / Failed / Testing) along with screen resolution and user agent.

R4. Responsiveness and Visual Polish:
- Un-nest `.settings-container` from `.right-panel` inside `html/src/style/index.scss`, and rewrite/expand settings styles to support the split-column layout on desktop and responsive styling on mobile (e.g. stacking columns, horizontal scroll, or tab buttons).
- Use premium design aesthetics: glassmorphism, cards, smooth hover effects, consistent spacing, and micro-animations.

Build Verification:
- Run compilation using `make frontend` inside `/Users/scott/Documents/01-开发项目/Web应用/1agents` to verify there are no TypeScript or SCSS compilation errors.

MANDATORY INTEGRITY WARNING:
DO NOT CHEAT. All implementations must be genuine. DO NOT hardcode test results, create dummy/facade implementations, or circumvent the intended task. A Forensic Auditor will independently verify your work. Integrity violations WILL be detected and your work WILL be rejected.
