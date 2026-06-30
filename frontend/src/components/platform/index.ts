/**
 * Platform layer exports — Epic #317 Wave 2b.
 *
 * ## For platform consumers (Wave 3 apps)
 *
 * ### View registration
 * ```ts
 * import { registerAppView } from '../../modules/appViewRegistry';
 * registerAppView('MyAppView', MyAppViewComponent);
 * ```
 *
 * ### Co-pilot context injection
 * ```ts
 * import { setCopilotAppContext, clearCopilotAppContext } from '../../stores/appManifestStore';
 * // In your component:
 * useEffect(() => {
 *   setCopilotAppContext({ appId: 'my-app', namespace: 'My App', connectors: ['my-mcp'] });
 *   return () => clearCopilotAppContext();
 * }, []);
 * ```
 *
 * ### Lens mount
 * ```ts
 * import { LensOverlay } from './platform';
 * // In your project/home view: <LensOverlay workspaceId={workspaceId} />
 * ```
 */

export { ProjectShell } from './ProjectShell';
export { SessionTierPicker } from './SessionTierPicker';
export type { SessionTier } from './SessionTierPicker';
export { MountPointRenderer } from './MountPointRenderer';
export { L1AppPage, L1NavItem, LensOverlay, getL1NavEntries } from './L1Shell';
export type { L1NavEntry } from './L1Shell';
export { ProjectConfigPanel } from './ProjectConfigPanel';
