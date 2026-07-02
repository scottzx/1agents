import { h } from 'preact';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as modal from '../../stores/modalStore';
import * as tabs from '../../stores/tabsStore';
import { AssistantDetail } from './AssistantDetail';

/**
 * AssistantsPage — the L1 "助理" landing.
 *
 * Bento card grid of every assistant (kind='assistant', excluding remote-device
 * projects). The built-in default is pinned first. Clicking a card opens that
 * assistant's detail view (skills / push-back etc.); the detail's "打开对话"
 * button is what actually drops the sidebar into its conversations.
 *
 * Cards intentionally carry just avatar / name / a lightweight state hint for
 * now — richer status (skill count, channel indicators, pending tasks) will
 * light up as the per-assistant config surface grows.
 */
export function AssistantsPage() {
    const language = ui.language.value;
    const workspaces = wsStore.workspaces.value;
    const folders = wsStore.folders.value;

    // A selected assistant opens its detail view in place of the grid.
    const detailId = tabs.assistantDetailId.value;
    if (detailId && workspaces.some(w => w.id === detailId)) {
        return <AssistantDetail workspaceId={detailId} />;
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
        if (!folder) return 0;
        return folder.sessions.length;
    };

    const onCardClick = (wsId: string) => {
        tabs.assistantDetailId.value = wsId;
    };

    return (
        <div class="assistants-page">
            <header class="assistants-page-header">
                <h1>{t('sidebar.assistants', language)}</h1>
                <button class="assistants-page-new-btn" onClick={modal.openCreateAssistantModal}>
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
            </header>

            {assistants.length === 0 ? (
                <div class="assistants-page-empty">
                    <span>{t('sidebar.noAssistants', language)}</span>
                    <button class="ws-empty-add" onClick={modal.openCreateAssistantModal}>
                        {t('sidebar.addAssistant', language)}
                    </button>
                </div>
            ) : (
                <div class="bento-grid assistants-grid">
                    {assistants.map(ws => (
                        <button key={ws.id} class="bento-card assistant-card" onClick={() => onCardClick(ws.id)}>
                            <div class="bento-zone-header">
                                {ws.avatar && ws.avatar.startsWith('/') ? (
                                    <img class="assistant-card-avatar" src={ws.avatar} alt="" />
                                ) : (
                                    <span class="assistant-card-emoji" aria-hidden="true">
                                        {'\u{1F464}'}
                                    </span>
                                )}
                                <div class="assistant-card-title">
                                    {ws.name}
                                    {ws.id === 'default' && (
                                        <span class="assistant-card-badge">
                                            {t('assistant.card.default', language)}
                                        </span>
                                    )}
                                </div>
                            </div>
                            <div class="bento-zone-footer assistant-card-meta">
                                <span>
                                    {t('assistant.card.sessions', language)}: {chatCountFor(ws.id)}
                                </span>
                            </div>
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
