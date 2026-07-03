import { h } from 'preact';
import { useEffect } from 'preact/hooks';
import { useSignalEffect } from '@preact/signals';
import type { App } from '../app';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as modal from '../../stores/modalStore';
import * as tabs from '../../stores/tabsStore';
import * as taskNav from '../../stores/taskNavStore';
import { AssistantDetail } from './AssistantDetail';

/**
 * AssistantsPage — the 助理 landing (breadcrumb level 1: 助理 概览).
 *
 * Renders the flat grid of every assistant (kind='assistant', excluding
 * remote-device projects), or — when a card is picked — drills into that
 * assistant's detail view (L2). The L1/L2 breadcrumb is published to the one
 * global WorkspaceHeader (助理 › <name>) rather than a second in-page bar; L3
 * (会话) shows once you're inside a session (see WorkspaceHeader).
 *
 * Visual language: codex-minimal — flat surfaces, hairline dividers, typography
 * carries the hierarchy, near-zero color. Interactive elements are pill-shaped.
 */
export function AssistantsPage({ app }: { app: App }) {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const folders = wsStore.folders.value;

    const detailId = tabs.assistantDetailId.value;
    const showDetail = !!detailId && workspaces.some(w => w.id === detailId);

    // Own the global header breadcrumb for both levels: 助理 (grid) and
    // 助理 › <name> (detail). Clicking 助理 drops back to the grid. Cleared on
    // unmount (leaving the assistants tab).
    useSignalEffect(() => {
        const lang = ui.language.value;
        const id = tabs.assistantDetailId.value;
        const ws = id ? wsStore.workspaces.value.find(w => w.id === id) : null;
        taskNav.headerCrumbs.value = ws
            ? [
                  { label: t('sidebar.assistants', lang), onClick: () => (tabs.assistantDetailId.value = null) },
                  { label: ws.name },
              ]
            : [{ label: t('sidebar.assistants', lang) }];
    });
    useEffect(() => () => void (taskNav.headerCrumbs.value = null), []);

    if (showDetail) {
        return <AssistantDetail workspaceId={detailId!} app={app} />;
    }

    const assistants = workspaces
        .filter(w => (w.kind ?? 'project') === 'assistant' && !w.deviceId)
        .sort((a, b) => {
            if (a.id === 'default') return -1;
            if (b.id === 'default') return 1;
            return 0;
        });

    const chatCountFor = (wsId: string): number => {
        const folder = folders.find(f => f.id === wsId);
        return folder ? folder.sessions.length : 0;
    };

    const onCardClick = (wsId: string) => {
        tabs.assistantDetailId.value = wsId;
    };

    return (
        <div class="assistants-page">
            <div class="assistants-toolbar">
                <p class="assistants-subtitle">
                    {assistants.length > 0
                        ? t('assistant.overview.count', language, { count: String(assistants.length) })
                        : t('assistant.overview.subtitle', language)}
                </p>
                <button class="assistant-btn assistant-btn-primary" onClick={modal.openCreateAssistantModal}>
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <line x1="12" y1="5" x2="12" y2="19" />
                        <line x1="5" y1="12" x2="19" y2="12" />
                    </svg>
                    <span>{t('sidebar.addAssistant', language)}</span>
                </button>
            </div>

            {assistants.length === 0 ? (
                <div class="assistants-empty">
                    <span>{t('sidebar.noAssistants', language)}</span>
                    <button class="assistant-btn assistant-btn-ghost" onClick={modal.openCreateAssistantModal}>
                        {t('sidebar.addAssistant', language)}
                    </button>
                </div>
            ) : (
                <div class="assistants-grid">
                    {assistants.map(ws => (
                        <button key={ws.id} class="assistant-card" onClick={() => onCardClick(ws.id)}>
                            {ws.avatar && ws.avatar.startsWith('/') ? (
                                <img class="assistant-card-avatar" src={ws.avatar} alt="" />
                            ) : (
                                <span class="assistant-card-avatar is-emoji" aria-hidden="true">
                                    {'\u{1F464}'}
                                </span>
                            )}
                            <div class="assistant-card-body">
                                <div class="assistant-card-title">
                                    <span class="assistant-card-name">{ws.name}</span>
                                    {ws.id === 'default' && (
                                        <span class="assistant-tag">{t('assistant.card.default', language)}</span>
                                    )}
                                </div>
                                <span class="assistant-card-meta">
                                    {t('assistant.card.sessions', language)} · {chatCountFor(ws.id)}
                                </span>
                            </div>
                            <svg
                                class="assistant-card-chevron"
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                aria-hidden="true"
                            >
                                <polyline points="9 6 15 12 9 18" />
                            </svg>
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
