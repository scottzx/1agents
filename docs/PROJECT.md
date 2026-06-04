# Project: 1agents-settings

## Architecture
- **Frontend Framework**: Preact / React.
- **State Management**: Root state in `app.tsx`, including `theme`, `language`, `tmuxMouseOn`, `terminalFontSize`, and `activeDrawerTab`.
- **Drawer Panels**: Controlled by `activeDrawerTab`. Right drawer panels are either regular width (e.g., files, git) or full-page overlays (e.g., providers, discovery, settings).
- **Settings Panel**: Built as a split-column panel with a Left Sidebar for Categories (General, Appearance/Terminal, Security, maintenance/Info) and a Right content area for the corresponding items.

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | E2E Test Suite Creation | Design E2E testing infra and write Tier 1-4 test cases | none | IN_PROGRESS (ID: bdbbcb8f-fd19-45ce-92e9-3ef1d081682d) |
| 2 | Refactor Settings Entry | Remove settings button from header and mobile menu, keep LeftSidebar click | M1 | IN_PROGRESS (ID: 12b0c7c7-ec3c-4941-b7e0-d0fc73c21ddb) |
| 3 | Full-Page Tab Refactor | Update `isFullPageTab` for settings, render settings in full-page container in `app.tsx` | M2 | IN_PROGRESS (ID: 12b0c7c7-ec3c-4941-b7e0-d0fc73c21ddb) |
| 4 | Premium Settings UI | Implement split-column settings panel (General, Appearance, Security, About/Maintenance) | M3 | IN_PROGRESS (ID: 12b0c7c7-ec3c-4941-b7e0-d0fc73c21ddb) |
| 5 | Verification & Hardening | Run E2E tests, perform adversarial testing, audit checks | M4 | PLANNED |

## Interface Contracts
### App State & Props for Settings Panel
- `isFullPageTab(tab: RightDrawerTab): boolean` in `types.ts` must return `true` for `'settings'`.
- `SettingsPanelProps`:
  - `theme`: `'light' | 'dark'`
  - `toggleTheme`: `(mode?: 'light' | 'dark') => void`
  - `language`: `'zh-CN' | 'en-US'` (for Voice Dictation Language)
  - `toggleLanguage`: `(lang: 'zh-CN' | 'en-US') => void`
  - `systemLanguage`: `'zh-CN' | 'en-US'` (for UI system language)
  - `toggleSystemLanguage`: `(lang: 'zh-CN' | 'en-US') => void`
  - `tmuxMouseOn`: `boolean`
  - `toggleTmuxMouse`: `() => void`
  - `terminalFontSize`: `number`
  - `changeTerminalFontSize`: `(size: number) => void`
  - `accessTokenExists`: `boolean`
  - `onGenerateAccessToken`: `() => void`
  - `onRevokeAccessToken`: `() => void`
  - `onClose`: `() => void`

## Code Layout
- Main view wrapper: `html/src/components/app.tsx`
- Tab types: `html/src/components/types.ts`
- Workspace Header: `html/src/components/header/WorkspaceHeader.tsx`
- Left Sidebar: `html/src/components/sidebar/LeftSidebar.tsx`
- Settings UI: `html/src/components/drawer/ThemeSettings.tsx` (to be rewritten or replaced)
