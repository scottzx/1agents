import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { App } from '../app';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sessStore from '../../stores/sessionStore';
import * as fs from '../../stores/fsStore';
import * as tabs from '../../stores/tabsStore';
import { ShellNav, type ShellTab } from '../platform/ShellNav';
import { TaskList } from '../drawer/TaskList';
import { SessionsView } from '../drawer/TaskList/SessionsView';
import { FilesPane, ChannelsPane } from '../shared/WorkspacePanes';
import { skillService, type WorkspaceSkillStatus } from '@1agents/core/services/skillService';
import { soulService } from '@1agents/core/services/soulService';

/**
 * AssistantDetail — breadcrumb level 2 (助理 › <name>). The trail + back-nav
 * live in the global WorkspaceHeader (published by AssistantsPage). This view is
 * the assistant's workbench hub: an identity hero + a secondary top-nav that
 * switches between the surfaces that used to be locked to the side pane.
 *
 * Tabs: 会话 (all sessions incl. archived, reusing the project-management
 * SessionsView) · 人设 (SOUL.md) · 任务 (TaskList) · 技能 (#360 reverse-sync) ·
 * 渠道 (cc-connect, moved out of the side pane) · 文件 (file browser + preview) ·
 * MCP (placeholder).
 *
 * 渠道 and 文件 read the *active* workspace's fs / cc-connect state, so on mount
 * we make this assistant the active workspace context (without the navigation
 * side-effects of selectWorkspace, which would drop the full-page detail).
 */
type DetailTab = 'sessions' | 'soul' | 'tasks' | 'skills' | 'channels' | 'files' | 'mcp';

interface AssistantDetailProps {
    workspaceId: string;
    app: App;
}

export function AssistantDetail({ workspaceId, app }: AssistantDetailProps) {
    const language = ui.language.value;
    const theme = ui.theme.value;
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);

    const [activeTab, setActiveTab] = useState<DetailTab>('sessions');

    // Make this assistant the active workspace *context* (fs + cc-connect) so
    // the 文件 / 渠道 tabs render its data — but skip selectWorkspace's tab
    // navigation, which would exit this full-page detail.
    useEffect(() => {
        if (!ws) return;
        if (wsStore.activeWorkspaceId.value !== workspaceId) {
            wsStore.activeWorkspaceId.value = workspaceId;
            wsStore.loadCcConnectUrl(workspaceId);
            wsStore.loadCcProvidersUrl(workspaceId);
            void fs.switchFsContext(ws);
        }
    }, [workspaceId]);

    // ── 人设 (SOUL.md) ──────────────────────────────────────────────────────
    const [soul, setSoul] = useState('');
    const [soulSaved, setSoulSaved] = useState('');
    const [soulSaving, setSoulSaving] = useState(false);
    const [soulFlash, setSoulFlash] = useState('');
    useEffect(() => {
        let cancelled = false;
        soulService
            .getWorkspaceSoul(workspaceId)
            .then(content => {
                if (!cancelled) {
                    setSoul(content);
                    setSoulSaved(content);
                }
            })
            .catch(() => {});
        return () => {
            cancelled = true;
        };
    }, [workspaceId]);

    const onSaveSoul = async () => {
        setSoulSaving(true);
        setSoulFlash('');
        try {
            await soulService.saveWorkspaceSoul(workspaceId, soul);
            setSoulSaved(soul);
            setSoulFlash(t('assistant.detail.soulSaved', language));
        } catch {
            setSoulFlash(t('assistant.detail.soulSaveFailed', language));
        } finally {
            setSoulSaving(false);
        }
    };

    // ── 技能 (#360 reverse-sync) ────────────────────────────────────────────
    const [skills, setSkills] = useState<WorkspaceSkillStatus[]>([]);
    const [skillsLoading, setSkillsLoading] = useState(true);
    const [skillsError, setSkillsError] = useState<string | null>(null);
    const [pushing, setPushing] = useState<string | null>(null);
    const [flash, setFlash] = useState<Record<string, string>>({});

    const loadSkills = useCallback(async () => {
        setSkillsLoading(true);
        setSkillsError(null);
        try {
            setSkills(await skillService.listWorkspaceSkills(workspaceId));
        } catch (e) {
            setSkillsError(String(e));
        } finally {
            setSkillsLoading(false);
        }
    }, [workspaceId]);
    useEffect(() => {
        void loadSkills();
    }, [loadSkills]);

    const onPush = async (ref: string) => {
        setPushing(ref);
        try {
            const { changed, created } = await skillService.pushSkill(workspaceId, ref);
            const msg = created
                ? t('assistant.detail.pushedCreated', language)
                : changed
                  ? t('assistant.detail.pushed', language)
                  : t('assistant.detail.pushNoChange', language);
            setFlash(f => ({ ...f, [ref]: msg }));
            await loadSkills();
        } catch {
            setFlash(f => ({ ...f, [ref]: t('assistant.detail.pushFailed', language) }));
        } finally {
            setPushing(null);
        }
    };

    const BADGE: Record<WorkspaceSkillStatus['state'], { cls: string; key: string }> = {
        synced: { cls: 'is-synced', key: 'assistant.detail.synced' },
        modified: { cls: 'is-modified', key: 'assistant.detail.modified' },
        local: { cls: 'is-local', key: 'assistant.detail.local' },
    };

    // Start a fresh conversation scoped to this assistant.
    const onNewChat = async () => {
        if (ws) await wsStore.selectWorkspace(ws);
        sessStore.onStartNewChat();
    };

    if (!ws) {
        // Stale id (assistant deleted) — drop back to the grid.
        tabs.assistantDetailId.value = null;
        return null;
    }

    const shellTabs: ShellTab[] = [
        { id: 'sessions', label: t('assistant.detail.tab.sessions', language) },
        { id: 'soul', label: t('assistant.detail.tab.soul', language) },
        { id: 'tasks', label: t('assistant.detail.tab.tasks', language) },
        { id: 'skills', label: t('assistant.detail.tab.skills', language) },
        { id: 'channels', label: t('assistant.detail.tab.channels', language) },
        { id: 'files', label: t('assistant.detail.tab.files', language) },
        { id: 'mcp', label: t('assistant.detail.tab.mcp', language) },
    ];

    return (
        <div class="assistant-detail">
            <div class="assistant-hero">
                {ws.avatar && ws.avatar.startsWith('/') ? (
                    <img class="assistant-hero-avatar" src={ws.avatar} alt="" />
                ) : (
                    <span class="assistant-hero-avatar is-emoji" aria-hidden="true">
                        {'\u{1F464}'}
                    </span>
                )}
                <div class="assistant-hero-ident">
                    <h1 class="assistant-hero-name">{ws.name}</h1>
                    {ws.id === 'default' && <span class="assistant-tag">{t('assistant.card.default', language)}</span>}
                </div>
                <button class="assistant-btn assistant-btn-primary" onClick={() => void onNewChat()}>
                    {t('assistant.detail.newChat', language)}
                </button>
            </div>

            <ShellNav tabs={shellTabs} activeTab={activeTab} onSelectTab={id => setActiveTab(id as DetailTab)} />

            <div class="assistant-tab-body">
                {activeTab === 'sessions' && (
                    <div class="assistant-pane-fill">
                        <SessionsView
                            workspaceId={workspaceId}
                            onSelectSession={s => void sessStore.selectSession(s)}
                        />
                    </div>
                )}

                {activeTab === 'soul' && (
                    <div class="assistant-pane-scroll">
                        <p class="assistant-hint">{t('assistant.detail.soulHint', language)}</p>
                        <textarea
                            class="assistant-soul-editor"
                            value={soul}
                            placeholder={t('assistant.detail.soulPlaceholder', language)}
                            onInput={(e: Event) => setSoul((e.target as HTMLTextAreaElement).value)}
                        />
                        <div class="assistant-section-actions">
                            {soulFlash && <span class="assistant-flash">{soulFlash}</span>}
                            <button
                                class="assistant-btn assistant-btn-ghost"
                                disabled={soul === soulSaved || soulSaving}
                                onClick={() => void onSaveSoul()}
                            >
                                {soulSaving
                                    ? t('assistant.detail.soulSaving', language)
                                    : t('assistant.detail.soulSave', language)}
                            </button>
                        </div>
                    </div>
                )}

                {activeTab === 'tasks' && (
                    <div class="project-shell-tasks-wrap">
                        <TaskList workspaceId={workspaceId} onSelectSession={s => void sessStore.selectSession(s)} />
                    </div>
                )}

                {activeTab === 'skills' && (
                    <div class="assistant-pane-scroll">
                        <p class="assistant-hint">{t('assistant.detail.skillsHint', language)}</p>
                        {skillsLoading && <div class="assistant-empty-row">…</div>}
                        {!skillsLoading && skillsError && (
                            <div class="assistant-empty-row">
                                {t('assistant.detail.skillsLoadFailed', language)}: {skillsError}
                            </div>
                        )}
                        {!skillsLoading && !skillsError && skills.length === 0 && (
                            <div class="assistant-empty-row">{t('assistant.detail.skillsEmpty', language)}</div>
                        )}
                        {!skillsLoading && !skillsError && skills.length > 0 && (
                            <ul class="assistant-skill-list">
                                {skills.map(s => {
                                    const canPush = s.state !== 'synced';
                                    const pushLabel =
                                        s.state === 'local'
                                            ? t('assistant.detail.pushCreate', language)
                                            : t('assistant.detail.push', language);
                                    return (
                                        <li key={s.skillRef} class="assistant-skill-row">
                                            <div class="assistant-skill-info">
                                                <div class="assistant-skill-main">
                                                    <span class="assistant-skill-name">{s.name || s.dir}</span>
                                                    <span class={`assistant-skill-badge ${BADGE[s.state].cls}`}>
                                                        {t(BADGE[s.state].key, language)}
                                                    </span>
                                                </div>
                                                {s.description && <p class="assistant-skill-desc">{s.description}</p>}
                                            </div>
                                            <div class="assistant-skill-actions">
                                                {flash[s.skillRef] && (
                                                    <span class="assistant-flash">{flash[s.skillRef]}</span>
                                                )}
                                                {canPush && (
                                                    <button
                                                        class="assistant-btn assistant-btn-ghost"
                                                        disabled={pushing === s.skillRef}
                                                        onClick={() => void onPush(s.skillRef)}
                                                    >
                                                        {pushing === s.skillRef
                                                            ? t('assistant.detail.pushing', language)
                                                            : pushLabel}
                                                    </button>
                                                )}
                                            </div>
                                        </li>
                                    );
                                })}
                            </ul>
                        )}
                    </div>
                )}

                {activeTab === 'channels' && (
                    <div class="assistant-pane-fill">
                        <ChannelsPane theme={theme} language={language} />
                    </div>
                )}

                {activeTab === 'files' && (
                    <div class="assistant-pane-fill">
                        <FilesPane app={app} language={language} />
                    </div>
                )}

                {activeTab === 'mcp' && (
                    <div class="assistant-pane-scroll">
                        <div class="assistant-empty-row">{t('assistant.detail.mcpPlaceholder', language)}</div>
                    </div>
                )}
            </div>
        </div>
    );
}
