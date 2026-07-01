// 对话历史全局搜索 store.
//
// Owns the command-palette overlay state (open/close), the debounced query, and
// the jump-to actions. Tasks route through taskNavStore.openTaskById; sessions
// are re-fetched by id and handed to sessionStore.selectSession so the chat
// opens exactly as it would from the sidebar.

import { signal } from '@preact/signals';

import { searchService, type SearchResults } from '@1agents/core/services/searchService';
import { agentService } from '@1agents/core/services/agentService';
import * as sessionStore from './sessionStore';
import * as taskNav from './taskNavStore';

export const searchOpen = signal(false);
export const searchQuery = signal('');
export const searchResults = signal<SearchResults>({ tasks: [], sessions: [] });
export const searchLoading = signal(false);

/** Minimum query length before a network round-trip fires. */
const MIN_QUERY = 2;
const DEBOUNCE_MS = 250;

let debounceTimer: ReturnType<typeof setTimeout> | null = null;
// Guards against out-of-order responses clobbering a newer query's results.
let querySeq = 0;

export const openSearch = (): void => {
    searchOpen.value = true;
};

export const closeSearch = (): void => {
    searchOpen.value = false;
    searchQuery.value = '';
    searchResults.value = { tasks: [], sessions: [] };
    searchLoading.value = false;
    if (debounceTimer) {
        clearTimeout(debounceTimer);
        debounceTimer = null;
    }
    querySeq++;
};

/** Called on every keystroke; debounces the actual fetch. */
export const setQuery = (q: string): void => {
    searchQuery.value = q;
    if (debounceTimer) clearTimeout(debounceTimer);

    const trimmed = q.trim();
    if (trimmed.length < MIN_QUERY) {
        searchResults.value = { tasks: [], sessions: [] };
        searchLoading.value = false;
        querySeq++; // cancel any in-flight response
        return;
    }

    searchLoading.value = true;
    const seq = ++querySeq;
    debounceTimer = setTimeout(async () => {
        try {
            const res = await searchService.search(trimmed);
            if (seq !== querySeq) return; // superseded by a newer query
            searchResults.value = res;
        } catch {
            if (seq !== querySeq) return;
            searchResults.value = { tasks: [], sessions: [] };
        } finally {
            if (seq === querySeq) searchLoading.value = false;
        }
    }, DEBOUNCE_MS);
};

/** Jump to a task card, then dismiss the palette. */
export const gotoTask = (workspaceId: string, taskId: string): void => {
    taskNav.openTaskById(workspaceId, taskId);
    closeSearch();
};

/** Open a chat session, then dismiss the palette. */
export const gotoSession = async (sessionId: string): Promise<void> => {
    try {
        const session = await agentService.get(sessionId);
        if (session) sessionStore.selectSession(session);
    } catch {
        /* opening is best-effort — a stale hit just no-ops */
    }
    closeSearch();
};
