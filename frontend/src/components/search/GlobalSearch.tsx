import { h, Fragment } from 'preact';
import { useEffect, useRef } from 'preact/hooks';

import { t } from '../../i18n';
import { language } from '../../stores/uiStore';
import {
    searchOpen,
    searchQuery,
    searchResults,
    searchLoading,
    setQuery,
    closeSearch,
    gotoTask,
    gotoSession,
} from '../../stores/searchStore';

/**
 * 对话历史 command-palette overlay. Opened by the sidebar-header magnifier,
 * rendered once at the app-layout top level. Searches meta.db tasks + sessions
 * and jumps to the picked one (task card / chat session).
 */
export function GlobalSearch() {
    const inputRef = useRef<HTMLInputElement | null>(null);
    const open = searchOpen.value;

    // Focus the input whenever the palette opens.
    useEffect(() => {
        if (open) setTimeout(() => inputRef.current?.focus(), 30);
    }, [open]);

    // Global Escape closes even when focus has left the input.
    useEffect(() => {
        if (!open) return;
        const onKey = (e: KeyboardEvent) => {
            if (e.key === 'Escape') closeSearch();
        };
        document.addEventListener('keydown', onKey);
        return () => document.removeEventListener('keydown', onKey);
    }, [open]);

    if (!open) return null;

    const lang = language.value;
    const q = searchQuery.value;
    const { tasks, sessions } = searchResults.value;
    const loading = searchLoading.value;
    const hasQuery = q.trim().length >= 2;
    const empty = hasQuery && !loading && tasks.length === 0 && sessions.length === 0;

    return (
        <div class="gsearch-overlay" onClick={closeSearch}>
            <div class="gsearch-panel" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="gsearch-input-row">
                    <svg
                        class="gsearch-input-icon"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <circle cx="11" cy="11" r="8" />
                        <line x1="21" y1="21" x2="16.65" y2="16.65" />
                    </svg>
                    <input
                        ref={inputRef}
                        class="gsearch-input"
                        type="text"
                        placeholder={t('search.placeholder', lang)}
                        value={q}
                        onInput={(e: Event) => setQuery((e.target as HTMLInputElement).value)}
                    />
                    {loading && <span class="gsearch-spinner" aria-hidden="true" />}
                    <kbd class="gsearch-esc">Esc</kbd>
                </div>

                <div class="gsearch-results">
                    {!hasQuery && <div class="gsearch-hint">{t('search.hint', lang)}</div>}

                    {empty && <div class="gsearch-hint">{t('search.empty', lang)}</div>}

                    {tasks.length > 0 && (
                        <Fragment>
                            <div class="gsearch-group-label">{t('search.group.tasks', lang)}</div>
                            {tasks.map(h => (
                                <div key={`t-${h.id}`} class="gsearch-item" onClick={() => gotoTask(h.projectId, h.id)}>
                                    <span class="gsearch-item-icon" aria-hidden="true">
                                        {'\u{1F4CB}'}
                                    </span>
                                    <div class="gsearch-item-main">
                                        <span class="gsearch-item-title">
                                            {h.number > 0 && <span class="gsearch-item-num">#{h.number}</span>}
                                            {h.title || t('search.untitled', lang)}
                                        </span>
                                        <span class="gsearch-item-sub">
                                            {h.projectName && <span>{h.projectName}</span>}
                                            {h.status && <span class="gsearch-item-status">{h.status}</span>}
                                        </span>
                                    </div>
                                </div>
                            ))}
                        </Fragment>
                    )}

                    {sessions.length > 0 && (
                        <Fragment>
                            <div class="gsearch-group-label">{t('search.group.sessions', lang)}</div>
                            {sessions.map(h => (
                                <div key={`s-${h.id}`} class="gsearch-item" onClick={() => void gotoSession(h.id)}>
                                    <span class="gsearch-item-icon" aria-hidden="true">
                                        {'\u{1F4AC}'}
                                    </span>
                                    <div class="gsearch-item-main">
                                        <span class="gsearch-item-title">{h.name || t('search.untitled', lang)}</span>
                                        <span class="gsearch-item-sub">
                                            {h.projectName && <span>{h.projectName}</span>}
                                            {h.agentType && <span class="gsearch-item-status">{h.agentType}</span>}
                                        </span>
                                    </div>
                                </div>
                            ))}
                        </Fragment>
                    )}
                </div>
            </div>
        </div>
    );
}
