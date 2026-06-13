import { signal, computed } from '@preact/signals';

import { agentService, type AgentStatus } from '../services/agentService';

/**
 * Global agent catalog state: which agent applications are installed on this
 * host, their upstream ACP/CLI capabilities, and how cc-connect currently
 * drives each one. Probed by the backend at startup and exposed via
 * /api/agent/catalog; consumers read these signals instead of the hardcoded
 * AGENT_TYPES list.
 */

export const agentCatalog = signal<AgentStatus[]>([]);
export const agentCatalogLoading = signal(false);

/** Installed agents only (detection list). */
export const installedAgents = computed(() => agentCatalog.value.filter(a => a.installed));

/**
 * Agents the chat picker may offer: installed AND drivable by this backend.
 * Detection-only frameworks (integrated === false) are excluded.
 */
export const pickableAgents = computed(() => agentCatalog.value.filter(a => a.installed && a.integrated));

/**
 * Fetch the catalog into the global signal. Pass refresh=true to force the
 * backend to re-probe the system PATH before returning.
 */
export const loadAgentCatalog = async (refresh = false): Promise<void> => {
    agentCatalogLoading.value = true;
    try {
        agentCatalog.value = await agentService.getCatalog(refresh);
    } catch (err) {
        // Non-fatal: consumers fall back to the static AGENT_TYPES list.
        console.error('[agentCatalog] load failed', err);
    } finally {
        agentCatalogLoading.value = false;
    }
};
