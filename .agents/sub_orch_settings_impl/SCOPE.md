# Scope: Settings Implementation

## Architecture
- Navigation: LeftSidebar settings click changes `activeDrawerTab` to `'settings'`. Header and Mobile Drawer have their settings buttons/items removed.
- Tab logic: `isFullPageTab('settings')` returns true.
- Rendering: App component renders the settings panel full-screen when `activeDrawerTab === 'settings'`.
- Settings view: Split-column layout on desktop, responsive, with tabs/categories (General, Appearance & Terminal, Security, System Maintenance & Info).

## Milestones
| # | Name | Scope | Dependencies | Status |
|---|------|-------|-------------|--------|
| 1 | Exploration & Analysis | Find all components and locations where settings are referenced and verify behavior. | none | IN_PROGRESS |
| 2 | Implementation | Refactor header/mobile/sidebar/types/app and build premium split-column settings panel. | M1 | PLANNED |
| 3 | Verification & Auditing | Run unit tests, check verification via reviewers/challengers, verify via auditor. | M2 | PLANNED |
| 4 | E2E Testing Integration | Wait for E2E tests ready, run and pass 100% of E2E tests, handle failures. | M3 | PLANNED |

## Interface Contracts
- `isFullPageTab(tab: string): boolean`: must return `true` when `tab` is `'settings'`.
- `activeDrawerTab` in App state: must support `'settings'` which shows the settings panel in a full-page overlay container.
