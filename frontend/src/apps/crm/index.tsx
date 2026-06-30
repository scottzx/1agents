/**
 * CRM app bundle entry — registers the CRM views with the platform's
 * app view registry. The central src/apps/index.ts blank-imports this module
 * (orchestrator wiring) so the views are available before the L1 shell renders.
 *
 * View keys MUST match the backend manifest mountPoints[].view:
 *   - "CrmPage"      → l1-page  (lead funnel + contact library + decisions)
 *   - "CrmLeadsLens" → lens     (关联线索, project scope)
 */

import { registerAppView } from '../../modules/appViewRegistry';
import { CrmPage } from './CrmPage';
import { CrmLeadsLens } from './CrmLeadsLens';

registerAppView('CrmPage', CrmPage);
registerAppView('CrmLeadsLens', CrmLeadsLens);
