# Original User Request

## Initial Request — 2026-06-04T13:18:03Z

Move the application settings entry from the header to the sidebar footer only, refactor the settings view to be a full-screen/full-page overlay module (matching the style of the providers/discovery modules), and enhance the system settings features to be more functional, visually complete, and rich.

Working directory: `/Users/scott/Documents/01-开发项目/Web应用/1agents`
Integrity mode: development

## Requirements

### R1. Remove Settings from Header
- Remove the settings button (`hdr-btn-settings`) from the desktop workspace header.
- Remove the settings menu item (`mob-menu-settings`) from the mobile slide-down hamburger drawer menu.
- Ensure settings is exclusively opened from the LeftSidebar footer settings item.

### R2. Full-Page Overlay Mode for Settings
- Refactor the settings tab to be a full-page module. Update `isFullPageTab` in `types.ts` to return `true` for `'settings'`.
- Render the settings module in the full-page container inside `app.tsx`, occupying the main content viewport when active.

### R3. Comprehensive System Settings UI & Features
- Build a premium, split-column settings panel (Left column with settings categories, right column with specific configuration items) matching typical IDE settings.
- Implement the following settings categories and items:
  1. **General Settings (通用设置)**: Language selection (中文 / English), voice dictation language.
  2. **Appearance & Terminal (外观与终端)**: Theme toggle (Light/Dark), Tmux mouse behavior toggle ("滚轮滑动" / "选择复制"), and a terminal font-size customizer.
  3. **Security Settings (安全设置)**: Access token (Generate/Revoke) control.
  4. **System Maintenance & Info (关于与维护)**: Reset application cache button (clears localStorage after confirmation), and App info (Name: 1agents, Version, branding/logo).

### R4. Responsiveness and Visual Polish
- Follow modern design aesthetics (glassmorphism/premium layout, cards, smooth hover effects, consistent spacing, and subtle micro-animations).
- Ensure the settings screen is responsive and fits perfectly on both desktop and mobile viewports.

## Acceptance Criteria

### Visual & Navigation Verification
- [ ] Settings button is completely removed from the desktop header.
- [ ] Settings option is completely removed from the hamburger menu on mobile.
- [ ] LeftSidebar footer settings click changes `activeDrawerTab` to `'settings'` and displays the full-screen settings interface.
- [ ] Settings panel features a split-column layout on desktop, showing category options on the left and detail settings on the right.

### Functionality Verification
- [ ] Theme switching (Dark/Light) works immediately and updates UI styles.
- [ ] Voice dictation language can be toggled and persists.
- [ ] Access token generation/revocation works correctly and pops up the token display modal.
- [ ] Tmux mouse behavior toggle works correctly.
- [ ] "Reset Application Data" clears `localStorage` after a confirm dialog and prompts page reload.
