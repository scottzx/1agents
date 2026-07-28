/**
 * App Manifest Store — signals-based state for the app registry and
 * co-pilot context injection (#331 / #332).
 *
 * This store owns:
 *   - The list of installed AppManifests (loaded from GET /api/apps).
 *   - The ACTIVE app/page context that the co-pilot (chat) reads to inject
 *     MCP connectors and context for the current L1 page or project app.
 *   - Helpers to filter mount points by type.
 *
 * Wave 3 apps: call `setCopilotAppContext({ appId, connectors, namespace })`
 * when your L1 page or project tab becomes active. The co-pilot reads
 * `copilotAppContext` to inject the right MCP tools into its system prompt.
 */

import { signal, computed } from '@preact/signals';
import { getApps, type AppManifest, type MountPoint } from '../services/appManifestService';
import type { RoundtableRoom, RoundtableSeat } from '@1agents/core/services/roundtableService';

// ── Manifest state ────────────────────────────────────────────────────────────

const ROUNDTABLE_APP_ID = 'agents-roundtable';
const ROUNDTABLE_APP_LABEL = '圆桌讨论';

const canonicalAppLabel = (appId: string, fallback: string): string =>
    appId === ROUNDTABLE_APP_ID ? ROUNDTABLE_APP_LABEL : fallback;

const canonicalizeManifest = (app: AppManifest): AppManifest => {
    if (app.id !== ROUNDTABLE_APP_ID) return app;
    return {
        ...app,
        name: ROUNDTABLE_APP_LABEL,
        mountPoints: app.mountPoints.map(mount => ({
            ...mount,
            label: ROUNDTABLE_APP_LABEL,
        })),
    };
};

/** All installed app manifests (enabled + disabled). */
export const appManifests = signal<AppManifest[]>([]);

/** True while the first load is in flight. */
export const appsLoading = signal(false);

/** Load (or reload) manifests from the backend. Graceful on failure. */
export const loadApps = async (): Promise<void> => {
    appsLoading.value = true;
    try {
        const list = await getApps();
        appManifests.value = list.map(canonicalizeManifest);
    } finally {
        appsLoading.value = false;
    }
};

/** Only the enabled apps. */
export const enabledApps = computed<AppManifest[]>(() => appManifests.value.filter(a => a.enabled));

/** All project-tab mount points from enabled apps. */
export const projectTabMounts = computed<Array<{ app: AppManifest; mount: MountPoint & { type: 'project-tab' } }>>(
    () => {
        const result: Array<{ app: AppManifest; mount: MountPoint & { type: 'project-tab' } }> = [];
        for (const app of enabledApps.value) {
            for (const mp of app.mountPoints) {
                if (mp.type === 'project-tab') result.push({ app, mount: mp as MountPoint & { type: 'project-tab' } });
            }
        }
        return result;
    }
);

/** All l1-page mount points from enabled apps. */
export const l1PageMounts = computed<Array<{ app: AppManifest; mount: MountPoint & { type: 'l1-page' } }>>(() => {
    const result: Array<{ app: AppManifest; mount: MountPoint & { type: 'l1-page' } }> = [];
    for (const app of enabledApps.value) {
        for (const mp of app.mountPoints) {
            if (mp.type === 'l1-page') result.push({ app, mount: mp as MountPoint & { type: 'l1-page' } });
        }
    }
    return result;
});

/** All lens mount points from enabled apps. */
export const lensMounts = computed<Array<{ app: AppManifest; mount: MountPoint & { type: 'lens' } }>>(() => {
    const result: Array<{ app: AppManifest; mount: MountPoint & { type: 'lens' } }> = [];
    for (const app of enabledApps.value) {
        for (const mp of app.mountPoints) {
            if (mp.type === 'lens') result.push({ app, mount: mp as MountPoint & { type: 'lens' } });
        }
    }
    return result;
});

// ── Co-pilot context injection (#332) ────────────────────────────────────────

/**
 * The context that the co-pilot (chat panel) injects when the user is inside
 * an app's L1 page or project tab.
 *
 * Wave 3 apps SET this when their view becomes active; the co-pilot READS it
 * to know which MCP connectors to load and which app namespace to scope tasks
 * under.
 *
 * The co-pilot implementation (ChatPanel / session creation) reads these
 * fields and passes them as context to the agent system prompt.
 *
 * ## What Wave 3 apps must provide
 * Call `setCopilotAppContext({ appId, connectors, namespace })` when:
 *   - Your L1 page is navigated to (e.g. in a useEffect / componentDidMount).
 *   - Your project tab gains focus.
 * Call `clearCopilotAppContext()` on unmount / when your view becomes inactive.
 *
 * ## Fields
 * - `appId`: the AppManifest.id — lets the co-pilot identify the active app.
 * - `namespace`: a human-readable label for the system prompt context block
 *   (e.g. "CRM" or "Content Studio").
 * - `connectors`: MCP connector IDs this app contributes; the co-pilot loads
 *   these for the session so tools like "search_crm" are available.
 * - `projectWorkspaceId`: when the app is active inside a specific project,
 *   the workspace id provides task-creation scope.
 */
export interface CopilotAppContext {
    appId: string;
    namespace: string;
    connectors: string[];
    projectWorkspaceId?: string;
}

/** The currently injected co-pilot context, or null when not inside an app. */
export const copilotAppContext = signal<CopilotAppContext | null>(null);

/** Set the co-pilot context for the active app/page. Call from your L1 page or project tab. */
export const setCopilotAppContext = (ctx: CopilotAppContext): void => {
    copilotAppContext.value = ctx;
};

/** Clear the co-pilot context (call on unmount or when navigating away from an app view). */
export const clearCopilotAppContext = (): void => {
    copilotAppContext.value = null;
};

// ── Active L1 page tracking (#332) ───────────────────────────────────────────

/**
 * The mount-point id of the currently active L1 page (e.g. "crm-l1-page").
 * null = one of the built-in platform pages (助理 / project list / settings).
 * Drives left-nav active state and co-pilot context switching.
 *
 * L1 apps sit at the **same stage level** as a session / project overview:
 * only one of them owns the main pane. Opening a session or 项目总览 must
 * clear this id (see stageStore.exitL1App / selectSession).
 */
export const activeL1PageId = signal<string | null>(null);

export const setActiveL1Page = (mountPointId: string | null): void => {
    activeL1PageId.value = mountPointId;
};

// ── Open L1 apps (sidebar shortcuts, like open sessions) ─────────────────────

/**
 * An app the user has launched this session (or restored). Shown as a
 * sidebar shortcut card; archive removes the shortcut and exits if active.
 * Discovery-only apps (e.g. agents-roundtable) only appear here — they are
 * not permanent L1 nav entries.
 */
export interface OpenL1App {
    /** mountPoints[].id for type=l1-page */
    mountId: string;
    appId: string;
    label: string;
    icon?: string;
}

const OPEN_L1_APPS_KEY = '1agents-open-l1-apps';

const loadOpenL1Apps = (): OpenL1App[] => {
    try {
        const raw = localStorage.getItem(OPEN_L1_APPS_KEY);
        const parsed = raw ? JSON.parse(raw) : [];
        if (!Array.isArray(parsed)) return [];
        const valid = parsed.filter(
            (e: unknown): e is OpenL1App =>
                !!e &&
                typeof e === 'object' &&
                typeof (e as OpenL1App).mountId === 'string' &&
                typeof (e as OpenL1App).appId === 'string' &&
                typeof (e as OpenL1App).label === 'string'
        );
        const normalized = valid.map(entry => ({
            ...entry,
            label: canonicalAppLabel(entry.appId, entry.label),
        }));
        if (normalized.some((entry, index) => entry.label !== valid[index].label)) {
            localStorage.setItem(OPEN_L1_APPS_KEY, JSON.stringify(normalized));
        }
        return normalized;
    } catch {
        return [];
    }
};

/** Open/pinned L1 apps for the left-sidebar shortcut strip. */
export const openL1Apps = signal<OpenL1App[]>(loadOpenL1Apps());

const persistOpenL1Apps = (list: OpenL1App[]): void => {
    openL1Apps.value = list;
    try {
        localStorage.setItem(OPEN_L1_APPS_KEY, JSON.stringify(list));
    } catch {
        /* non-fatal */
    }
};

/** Pin an app into the open strip (idempotent; moves to front). */
export const pinOpenL1App = (entry: OpenL1App): void => {
    const rest = openL1Apps.value.filter(a => a.mountId !== entry.mountId);
    persistOpenL1Apps([
        {
            ...entry,
            label: canonicalAppLabel(entry.appId, entry.label),
        },
        ...rest,
    ]);
};

/**
 * Archive (close) an open-app shortcut. If it is the active L1 page, clear
 * the main pane so session/project can show again.
 */
export const archiveOpenL1App = (mountId: string): void => {
    persistOpenL1Apps(openL1Apps.value.filter(a => a.mountId !== mountId));
    if (activeL1PageId.value === mountId) {
        activeL1PageId.value = null;
        clearCopilotAppContext();
    }
};

// ── Roundtable active room / seats for sidebar cards (#new) ───────────────────

export const activeRoundtableRoom = signal<RoundtableRoom | null>(null);
export const activeRoundtableSeats = signal<RoundtableSeat[]>([]);

export const setActiveRoundtableRoom = (room: RoundtableRoom | null) => {
    activeRoundtableRoom.value = room;
};

export const setActiveRoundtableSeats = (seats: RoundtableSeat[]) => {
    activeRoundtableSeats.value = seats;
};

export const clearActiveRoundtable = () => {
    activeRoundtableRoom.value = null;
    activeRoundtableSeats.value = [];
};
