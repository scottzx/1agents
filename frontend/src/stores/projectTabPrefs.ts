import { signal } from '@preact/signals';

/**
 * Per-project tab visibility, persisted **project-local** at
 * <workspace>/.1agents/project_config.json (via /api/project/local-config).
 * Config travels with the project directory (works cross-device), and avoids the
 * meta.db project_config plumbing. The file is a generic JSON blob; this store
 * owns the `hiddenTabs` field and preserves any other keys on write.
 */

interface LocalConfig {
    hiddenTabs?: string[];
    [k: string]: unknown;
}

const configs = signal<Record<string, LocalConfig>>({});
const loaded = new Set<string>();

const url = (ws: string) => `/api/project/local-config?workspacePath=${encodeURIComponent(ws)}`;

async function fetchConfig(workspacePath: string): Promise<LocalConfig> {
    try {
        const res = await fetch(url(workspacePath), { credentials: 'same-origin' });
        if (!res.ok) return {};
        return (await res.json()) as LocalConfig;
    } catch {
        return {};
    }
}

/** Fetch the project's local config once and cache it in the reactive signal. */
export function ensureLoaded(workspaceId: string, workspacePath: string): void {
    if (!workspacePath || loaded.has(workspaceId)) return;
    loaded.add(workspaceId);
    void fetchConfig(workspacePath).then(cfg => {
        configs.value = { ...configs.value, [workspaceId]: cfg };
    });
}

/** Hidden tab ids for a project. Reactive — reading it in a render subscribes to writes. */
export function getHiddenTabs(workspaceId: string): Set<string> {
    return new Set<string>(configs.value[workspaceId]?.hiddenTabs ?? []);
}

/** Show or hide one tab and persist to the project-local file (optimistic). */
export async function setTabHidden(
    workspaceId: string,
    workspacePath: string,
    tabId: string,
    hidden: boolean
): Promise<void> {
    const cur = configs.value[workspaceId] ?? {};
    const set = new Set<string>(cur.hiddenTabs ?? []);
    if (hidden) set.add(tabId);
    else set.delete(tabId);
    const hiddenTabs = [...set];
    configs.value = { ...configs.value, [workspaceId]: { ...cur, hiddenTabs } }; // optimistic
    // PUT only our own key — the backend shallow-merges, so sibling keys written
    // by other features (project config) are preserved.
    await fetch(url(workspacePath), {
        method: 'PUT',
        credentials: 'same-origin',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ hiddenTabs }),
    });
}
