# Context: Settings Implementation

## Target Areas
- Desktop Workspace Header (remove `hdr-btn-settings` button).
- Mobile Drawer Menu (remove `mob-menu-settings` item).
- LeftSidebar footer settings item (make sure it changes `activeDrawerTab` to `'settings'`).
- `types.ts` (update `isFullPageTab` to return `true` for `'settings'`).
- `app.tsx` (ensure settings module is rendered in the full-page container).
- Settings Panel component (refactor to a premium split-column view with General, Appearance & Terminal, Security, and System Maintenance & Info settings).

## Expected Files
- `src/types.ts` or similar typescript type files.
- `src/app.tsx` or similar app entry/layout components.
- `src/components/LeftSidebar.tsx` or similar.
- `src/components/Header.tsx` or similar.
- `src/components/Settings.tsx` or similar.
