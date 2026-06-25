// Big-screen build target (issue #120). A standalone HTML/JS bundle that boots
// straight into the company dashboard — no main-app shell, no terminal, no
// ttyd embedding. It reuses the existing DashboardApp (cockpit / global kanban /
// pixel demo) and shares core/services + the global design tokens.
//
// Served by the Go backend at /dashboard (→ dist/dashboard.html). Drill-down
// into a real project navigates the browser to the main app via
// DashboardApp's existing window.location.href handlers.
if (process.env.NODE_ENV === 'development') {
    require('preact/debug');
}
import 'whatwg-fetch';
import { h, render } from 'preact';
import { DashboardApp } from './components/desktop/DashboardApp';
import { initPlatformBridge } from '@1agents/core/platform/bridge';
import { installRelayFetch } from '@1agents/core/services/relay/installRelayFetch';
import './style/index.scss';

// Same relay-mode fetch wrapper as the main app so dashboardService data
// fetches work behind a relay/tunnel.
installRelayFetch();

// Swap in the host-specific platform bridge (no-op on the web).
void initPlatformBridge();

render(<DashboardApp />, document.body);
