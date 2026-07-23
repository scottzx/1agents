import { h } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';

import { t, type Lang } from '../../i18n';
import type { Workspace } from '../types';
import { ONESHOT_WORKSPACE_ID } from '../../utils/oneshot';

export { ONESHOT_WORKSPACE_ID };

const isAssistantWs = (w: Workspace) => (w.kind ?? 'project') === 'workforce';

interface WorkspaceScopePickerProps {
    workspaces: Workspace[];
    value: string;
    language: Lang;
    disabled?: boolean;
    onChange: (workspaceId: string) => void;
}

/**
 * Segmented workspace picker for new-session setup:
 *   1. 单次对话 (oneshot)
 *   2. 助理 (workforce)
 *   3. 项目 (project)
 * Each list section has a max height; search filters assistant + project only.
 */
export function WorkspaceScopePicker({
    workspaces,
    value,
    language,
    disabled = false,
    onChange,
}: WorkspaceScopePickerProps) {
    const [open, setOpen] = useState(false);
    const [search, setSearch] = useState('');
    const rootRef = useRef<HTMLDivElement | null>(null);

    const localWorkspaces = useMemo(() => workspaces.filter(w => !w.deviceId), [workspaces]);

    const q = search.trim().toLowerCase();
    const assistants = useMemo(
        () =>
            localWorkspaces
                .filter(w => isAssistantWs(w))
                .filter(w => !q || w.name.toLowerCase().includes(q))
                .sort((a, b) => {
                    if (a.id === 'default') return -1;
                    if (b.id === 'default') return 1;
                    return a.name.localeCompare(b.name);
                }),
        [localWorkspaces, q]
    );
    const projects = useMemo(
        () =>
            localWorkspaces
                .filter(w => !isAssistantWs(w))
                .filter(w => !q || w.name.toLowerCase().includes(q))
                .sort((a, b) => a.name.localeCompare(b.name)),
        [localWorkspaces, q]
    );

    const selectedLabel = useMemo(() => {
        if (value === ONESHOT_WORKSPACE_ID) {
            return t('newchat.kind.oneshot', language);
        }
        return localWorkspaces.find(w => w.id === value)?.name || t('modal.sessionSetup.workspace', language);
    }, [value, localWorkspaces, language]);

    const selectedKindTag = useMemo(() => {
        if (value === ONESHOT_WORKSPACE_ID) {
            return { className: 'is-oneshot', label: t('newchat.kind.oneshot', language) };
        }
        const ws = localWorkspaces.find(w => w.id === value);
        if (ws && isAssistantWs(ws)) {
            return { className: 'is-assistant', label: t('newchat.kind.assistant', language) };
        }
        return { className: 'is-project', label: t('newchat.kind.project', language) };
    }, [value, localWorkspaces, language]);

    useEffect(() => {
        if (!open) return;
        const onDown = (e: MouseEvent) => {
            if (rootRef.current && !rootRef.current.contains(e.target as Node)) {
                setOpen(false);
                setSearch('');
            }
        };
        document.addEventListener('mousedown', onDown);
        return () => document.removeEventListener('mousedown', onDown);
    }, [open]);

    const pick = (id: string) => {
        onChange(id);
        setOpen(false);
        setSearch('');
    };

    const noMatches = assistants.length === 0 && projects.length === 0;

    return (
        <div class="ws-scope-picker" ref={rootRef}>
            <button
                type="button"
                class={`ws-scope-picker-trigger${open ? ' is-open' : ''}`}
                disabled={disabled}
                onClick={() => setOpen(v => !v)}
                aria-expanded={open}
                aria-haspopup="listbox"
            >
                <span class="ws-scope-picker-label">{selectedLabel}</span>
                <span class={`dropdown-kind-tag ${selectedKindTag.className}`}>{selectedKindTag.label}</span>
                <svg
                    class={`chevron ${open ? 'open' : ''}`}
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                    aria-hidden="true"
                >
                    <polyline points="6 9 12 15 18 9" />
                </svg>
            </button>

            {open && (
                <div class="ws-scope-dropdown" role="listbox">
                    <div class="dropdown-search-wrap">
                        <svg
                            class="dropdown-search-icon"
                            viewBox="0 0 24 24"
                            fill="none"
                            stroke="currentColor"
                            stroke-width="2"
                            stroke-linecap="round"
                            stroke-linejoin="round"
                            aria-hidden="true"
                        >
                            <circle cx="11" cy="11" r="7" />
                            <line x1="21" y1="21" x2="16.65" y2="16.65" />
                        </svg>
                        <input
                            class="dropdown-search-input"
                            type="text"
                            placeholder={t('newchat.searchWs', language)}
                            value={search}
                            onInput={(e: Event) => setSearch((e.target as HTMLInputElement).value)}
                            autoFocus
                        />
                    </div>

                    <div class="ws-scope-body">
                        {/* Section 1: oneshot */}
                        <div class="ws-scope-section">
                            <div class="dropdown-header">{t('newchat.section.oneshot', language)}</div>
                            <div class="ws-scope-section-list">
                                <button
                                    type="button"
                                    role="option"
                                    aria-selected={value === ONESHOT_WORKSPACE_ID}
                                    class={`dropdown-item ${value === ONESHOT_WORKSPACE_ID ? 'active' : ''}`}
                                    onClick={() => pick(ONESHOT_WORKSPACE_ID)}
                                >
                                    <span class="item-name">{t('newchat.kind.oneshot', language)}</span>
                                    <span class="dropdown-kind-tag is-oneshot">
                                        {t('newchat.kind.oneshotTag', language)}
                                    </span>
                                </button>
                            </div>
                        </div>

                        {/* Section 2: assistants (workforce) */}
                        <div class="ws-scope-section">
                            <div class="dropdown-header">{t('newchat.section.assistant', language)}</div>
                            <div class="ws-scope-section-list">
                                {assistants.map(ws => (
                                    <button
                                        type="button"
                                        role="option"
                                        aria-selected={ws.id === value}
                                        key={ws.id}
                                        class={`dropdown-item ${ws.id === value ? 'active' : ''}`}
                                        onClick={() => pick(ws.id)}
                                    >
                                        <span class="item-name">{ws.name}</span>
                                        <span class="dropdown-kind-tag is-assistant">
                                            {t('newchat.kind.assistant', language)}
                                        </span>
                                    </button>
                                ))}
                                {assistants.length === 0 && (
                                    <div class="dropdown-empty">{t('newchat.noWsMatch', language)}</div>
                                )}
                            </div>
                        </div>

                        {/* Section 3: projects */}
                        <div class="ws-scope-section">
                            <div class="dropdown-header">{t('newchat.section.project', language)}</div>
                            <div class="ws-scope-section-list">
                                {projects.map(ws => (
                                    <button
                                        type="button"
                                        role="option"
                                        aria-selected={ws.id === value}
                                        key={ws.id}
                                        class={`dropdown-item ${ws.id === value ? 'active' : ''}`}
                                        onClick={() => pick(ws.id)}
                                    >
                                        <span class="item-name">{ws.name}</span>
                                        <span class="dropdown-kind-tag is-project">
                                            {t('newchat.kind.project', language)}
                                        </span>
                                    </button>
                                ))}
                                {projects.length === 0 && (
                                    <div class="dropdown-empty">{t('newchat.noWsMatch', language)}</div>
                                )}
                            </div>
                        </div>
                    </div>

                    {noMatches && q && (
                        <div class="dropdown-empty ws-scope-footer-empty">{t('newchat.noWsMatch', language)}</div>
                    )}
                </div>
            )}
        </div>
    );
}
