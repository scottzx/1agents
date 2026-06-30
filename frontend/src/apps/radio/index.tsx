/**
 * Radio app frontend entry point (#347).
 *
 * Registers the RadioPage view under the `view` key declared in the backend
 * manifest (mountPoints[].view === "RadioPage"). The orchestrator blank-imports
 * this module from `src/apps/index.ts` so the registration runs at startup.
 *
 * Touches zero kernel UI — only the platform app-view registry seam.
 */

import { registerAppView } from '../../modules/appViewRegistry';
import { RadioPage } from './RadioPage';

registerAppView('RadioPage', RadioPage);
