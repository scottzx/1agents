import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as wsStore from '../../stores/workspaceStore';
import * as ui from '../../stores/uiStore';
import * as appStore from '../../stores/appManifestStore';
import * as tabPrefs from '../../stores/projectTabPrefs';

/**
 * 设置 tab of the 助理/项目 详情. First feature: 归档/活跃 — archiving flips a
 * project from active to archived (dropping it from the sidebar); it stays
 * reachable from the overview's 已归档 board, where re-opening its detail shows
 * this button as 活跃 (reopen). Laid out as a settings-row so more toggles slot in.
 */
export function SettingsTab({ workspaceId, language }: { workspaceId: string; language: Lang }) {
    const [busy, setBusy] = useState(false);
    const [confirmArchive, setConfirmArchive] = useState(false);

    const wsPath = wsStore.findWorkspaceAnyStatus(workspaceId)?.path ?? '';

    // Ensure the archived board is loaded so we can tell an archived detail from
    // an active one (the active `workspaces` list excludes archived).
    useEffect(() => {
        void wsStore.loadArchivedWorkspaces();
    }, [workspaceId]);
    useEffect(() => {
        tabPrefs.ensureLoaded(workspaceId, wsPath);
    }, [workspaceId, wsPath]);
    const archived = wsStore.archivedWorkspaces.value.some(w => w.id === workspaceId);

    // 可显隐的 tab：内置可选项 + 各 app 贡献的 project-tab（如 口播剪辑）。核心
    // tab(会话/文件/设置等)不在此列，始终显示。
    const hidden = tabPrefs.getHiddenTabs(workspaceId);
    const appTabs = appStore.projectTabMounts.value.map(({ mount }) => ({ id: mount.id, label: mount.label }));
    const toggleableTabs = [
        { id: 'activity', label: '动态' },
        { id: 'plan', label: '计划' },
        { id: 'assets', label: '资产' },
        ...appTabs,
    ];

    const onArchive = async () => {
        setBusy(true);
        setConfirmArchive(false);
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
            <div class="settings-row settings-row-block">
                <div class="settings-row-info">
                    <span class="settings-row-title">显示的标签页</span>
                    <span class="settings-row-desc">勾选在本项目内显示哪些标签页。</span>
                </div>
                <div class="settings-tab-toggles">
                    {toggleableTabs.map(tb => (
                        <label key={tb.id} class="settings-tab-toggle">
                            <input
                                type="checkbox"
                                checked={!hidden.has(tb.id)}
                                onChange={e =>
                                    void tabPrefs.setTabHidden(
                                        workspaceId,
                                        wsPath,
                                        tb.id,
                                        !(e.target as HTMLInputElement).checked
                                    )
                                }
                            />
                            <span>{tb.label}</span>
                        </label>
                    ))}
                </div>
            </div>

            <div class="settings-zone-danger">
                <div class="settings-zone-danger-title">危险操作</div>
                <div class="settings-row">
                    <div class="settings-row-info">
                        <span class="settings-row-title">{t('assistant.detail.settings.archiveTitle', language)}</span>
                        <span class="settings-row-desc">
                            {archived
                                ? t('assistant.detail.settings.activateDesc', language)
                                : confirmArchive
                                  ? '确认后将从活跃列表移除，可在"已归档"中找回。'
                                  : t('assistant.detail.settings.archiveDesc', language)}
                        </span>
                    </div>
                    {archived ? (
                        <button
                            class="assistant-btn assistant-btn-primary"
                            disabled={busy}
                            onClick={() => void onReopen()}
                        >
                            {t('assistant.detail.settings.activate', language)}
                        </button>
                    ) : confirmArchive ? (
                        <div class="settings-confirm-row">
                            <button
                                class="assistant-btn assistant-btn-danger"
                                disabled={busy}
                                onClick={() => void onArchive()}
                            >
                                确认归档
                            </button>
                            <button
                                class="assistant-btn assistant-btn-ghost"
                                disabled={busy}
                                onClick={() => setConfirmArchive(false)}
                            >
                                取消
                            </button>
                        </div>
                    ) : (
                        <button
                            class="assistant-btn assistant-btn-ghost"
                            disabled={busy}
                            onClick={() => setConfirmArchive(true)}
                        >
                            {t('assistant.detail.settings.archive', language)}
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
