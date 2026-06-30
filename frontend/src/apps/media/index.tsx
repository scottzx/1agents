/**
 * Media app frontend bundle entry. Registers the project-tab view components
 * whose names match the backend manifest's mountPoints[].view. The central
 * src/apps/index.ts blank-imports this module so the views are available before
 * ProjectShell renders any media tab.
 */
import { registerAppView } from '../../modules/appViewRegistry';
import { MediaMaterialTab } from './MediaMaterialTab';
import { MediaPipelineTab } from './MediaPipelineTab';

registerAppView('MediaMaterialTab', MediaMaterialTab);
registerAppView('MediaPipelineTab', MediaPipelineTab);
