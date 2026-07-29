import { signal } from '@preact/signals';

/**
 * Per-project tab visibility, persisted **project-local** at
 * <workspace>/.1agents/project_config.json (via /api/project/local-config).
 * Config travels with the project directory (works cross-device), and avoids the
 * meta.db project_config plumbing. The file is a generic JSON blob; this store
 * owns `hiddenTabs` and `featureCatalogEnabled`, preserving other keys on write.
 */

export interface LocalConfig {
    hiddenTabs?: string[];
    featureCatalogEnabled?: boolean;
    [k: string]: unknown;
}

export interface ProjectConfigStatus {
    loading: boolean;
    loaded: boolean;
    loadError: string;
    saving: boolean;
    saveError: string;
}

const DEFAULT_STATUS: ProjectConfigStatus = {
    loading: false,
    loaded: false,
    loadError: '',
    saving: false,
    saveError: '',
};

const configs = signal<Record<string, LocalConfig>>({});
const statuses = signal<Record<string, ProjectConfigStatus>>({});

const url = (ws: string) => `/api/project/local-config?workspacePath=${encodeURIComponent(ws)}`;

async function fetchConfig(workspacePath: string): Promise<LocalConfig> {
    const res = await fetch(url(workspacePath), { credentials: 'same-origin' });
    if (!res.ok) {
        throw new Error(`加载项目配置失败 (HTTP ${res.status})`);
    }
    const raw = (await res.json()) as LocalConfig;
    return {
        ...raw,
        // Missing and malformed values both preserve the upgrade-safe default.
        featureCatalogEnabled: raw.featureCatalogEnabled === true,
    };
}

function updateStatus(workspaceId: string, patch: Partial<ProjectConfigStatus>): void {
    const current = statuses.value[workspaceId] ?? DEFAULT_STATUS;
    statuses.value = {
        ...statuses.value,
        [workspaceId]: { ...current, ...patch },
    };
}

/** Fetch the project's local config once and cache it in the reactive signal. */
export async function ensureLoaded(workspaceId: string, workspacePath: string): Promise<void> {
    const status = statuses.value[workspaceId] ?? DEFAULT_STATUS;
    if (!workspacePath || status.loading || status.loaded) return;
    updateStatus(workspaceId, { loading: true, loadError: '' });
    try {
        const cfg = await fetchConfig(workspacePath);
        configs.value = { ...configs.value, [workspaceId]: cfg };
        updateStatus(workspaceId, { loading: false, loaded: true, loadError: '' });
    } catch (error) {
        // A completed failed load is still "ready": consumers use the safe
        // defaults and may offer an explicit retry without waiting forever.
        updateStatus(workspaceId, {
            loading: false,
            loaded: true,
            loadError: error instanceof Error ? error.message : String(error),
        });
    }
}

/** Retry a failed project-config load. */
export async function reload(workspaceId: string, workspacePath: string): Promise<void> {
    updateStatus(workspaceId, { loaded: false, loadError: '' });
    await ensureLoaded(workspaceId, workspacePath);
}

/** Reactive load/save state for the project's local configuration. */
export function getProjectConfigStatus(workspaceId: string): ProjectConfigStatus {
    return statuses.value[workspaceId] ?? DEFAULT_STATUS;
}

/** Hidden tab ids for a project. Reactive — reading it in a render subscribes to writes. */
export function getHiddenTabs(workspaceId: string): Set<string> {
    return new Set<string>(configs.value[workspaceId]?.hiddenTabs ?? []);
}

/** Feature-catalog capability for a project. Missing fields default to off. */
export function getFeatureCatalogEnabled(workspaceId: string): boolean {
    return configs.value[workspaceId]?.featureCatalogEnabled === true;
}

async function persistOptimistic(
    workspaceId: string,
    workspacePath: string,
    previous: LocalConfig,
    next: LocalConfig,
    patch: Partial<LocalConfig>
): Promise<boolean> {
    configs.value = { ...configs.value, [workspaceId]: next };
    updateStatus(workspaceId, { saving: true, saveError: '' });
    try {
        if (!workspacePath) throw new Error('项目路径不可用');
        const res = await fetch(url(workspacePath), {
            method: 'PUT',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(patch),
        });
        if (!res.ok) throw new Error(`保存项目配置失败 (HTTP ${res.status})`);
        updateStatus(workspaceId, { saving: false, saveError: '' });
        return true;
    } catch (error) {
        configs.value = { ...configs.value, [workspaceId]: previous };
        updateStatus(workspaceId, {
            saving: false,
            saveError: error instanceof Error ? error.message : String(error),
        });
        return false;
    }
}

/** Show or hide one tab and persist to the project-local file (optimistic). */
export async function setTabHidden(
    workspaceId: string,
    workspacePath: string,
    tabId: string,
    hidden: boolean
): Promise<boolean> {
    const cur = configs.value[workspaceId] ?? {};
    const set = new Set<string>(cur.hiddenTabs ?? []);
    if (hidden) set.add(tabId);
    else set.delete(tabId);
    const hiddenTabs = [...set];
    // PUT only our own key — the backend shallow-merges, so sibling keys written
    // by other features (project config) are preserved.
    return persistOptimistic(workspaceId, workspacePath, cur, { ...cur, hiddenTabs }, { hiddenTabs });
}

/** Enable or disable the feature catalog without touching its backend data. */
export async function setFeatureCatalogEnabled(
    workspaceId: string,
    workspacePath: string,
    enabled: boolean
): Promise<boolean> {
    const cur = configs.value[workspaceId] ?? {};
    return persistOptimistic(
        workspaceId,
        workspacePath,
        cur,
        { ...cur, featureCatalogEnabled: enabled },
        { featureCatalogEnabled: enabled }
    );
}
