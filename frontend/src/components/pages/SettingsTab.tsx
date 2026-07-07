import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as wsStore from '../../stores/workspaceStore';
import * as ui from '../../stores/uiStore';

/**
 * 设置 tab of the 助理/项目 详情. First feature: 归档/活跃 — archiving flips a
 * project from active to archived (dropping it from the sidebar); it stays
 * reachable from the overview's 已归档 board, where re-opening its detail shows
 * this button as 活跃 (reopen). Laid out as a settings-row so more toggles slot in.
 */
export function SettingsTab({ workspaceId, language }: { workspaceId: string; language: Lang }) {
    const [busy, setBusy] = useState(false);

    // Ensure the archived board is loaded so we can tell an archived detail from
    // an active one (the active `workspaces` list excludes archived).
    useEffect(() => {
        void wsStore.loadArchivedWorkspaces();
    }, [workspaceId]);
    const archived = wsStore.archivedWorkspaces.value.some(w => w.id === workspaceId);

    const onArchive = async () => {
        setBusy(true);
        try {
            await wsStore.archiveWorkspace(workspaceId);
            ui.showToast(t('assistant.detail.settings.archived', language));
        } catch (e) {
            ui.showToast(String(e));
        } finally {
            setBusy(false);
        }
    };
    const onReopen = async () => {
        setBusy(true);
        try {
            await wsStore.reopenWorkspace(workspaceId);
            ui.showToast(t('assistant.detail.settings.activated', language));
        } catch (e) {
            ui.showToast(String(e));
        } finally {
            setBusy(false);
        }
    };

    return (
        <div class="settings-tab">
            <div class="settings-row">
                <div class="settings-row-info">
                    <span class="settings-row-title">{t('assistant.detail.settings.archiveTitle', language)}</span>
                    <span class="settings-row-desc">
                        {archived
                            ? t('assistant.detail.settings.activateDesc', language)
                            : t('assistant.detail.settings.archiveDesc', language)}
                    </span>
                </div>
                {archived ? (
                    <button class="assistant-btn assistant-btn-primary" disabled={busy} onClick={() => void onReopen()}>
                        {t('assistant.detail.settings.activate', language)}
                    </button>
                ) : (
                    <button class="assistant-btn assistant-btn-ghost" disabled={busy} onClick={() => void onArchive()}>
                        {t('assistant.detail.settings.archive', language)}
                    </button>
                )}
            </div>
        </div>
    );
}
