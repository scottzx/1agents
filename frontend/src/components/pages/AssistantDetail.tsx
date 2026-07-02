import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as sessStore from '../../stores/sessionStore';
import * as tabs from '../../stores/tabsStore';
import type { Session } from '../types';
import { skillService, type WorkspaceSkillStatus } from '@1agents/core/services/skillService';
import { soulService } from '@1agents/core/services/soulService';

/**
 * AssistantDetail — breadcrumb level 2 (助理 › <name>), reached by picking a
 * card on AssistantsPage. The trail + back-navigation live in the global
 * WorkspaceHeader (published by AssistantsPage), so this view has no bar of its
 * own — it opens with an identity hero, then stacks the assistant's 会话 /
 * 人设 / 技能 as hairline-separated sections.
 *
 * 会话 (L3): the assistant's conversations. Clicking one drops into the session
 * view (where the header shows the full 助理 › <name> › <session> trail); 新建
 * 会话 opens the new-chat landing scoped to this assistant.
 *
 * 技能 (#360 reverse-sync): each skill copied into <ws>/.claude/skills shows
 * whether it has drifted from the shared store (母体), with a 推送到母体 action.
 */
interface AssistantDetailProps {
    workspaceId: string;
}

export function AssistantDetail({ workspaceId }: AssistantDetailProps) {
    const language = ui.language.value;
    const ws = wsStore.workspaces.value.find(w => w.id === workspaceId);

    const [skills, setSkills] = useState<WorkspaceSkillStatus[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [pushing, setPushing] = useState<string | null>(null);
    const [flash, setFlash] = useState<Record<string, string>>({});

    // Persona (人设 / SOUL.md) — loaded independently of skills.
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

    // 会话 (L3) — this assistant's chat sessions. Load them even when the
    // assistant isn't the active workspace, so the list is live in the detail.
    useEffect(() => {
        void sessStore.loadChatSessions(workspaceId);
    }, [workspaceId]);
    const sessions = sessStore.chatSessions.value
        .filter(s => s.workspaceId === workspaceId && !s.archived)
        .sort((a, b) => (b.lastEventAt || b.createdAt || '').localeCompare(a.lastEventAt || a.createdAt || ''));

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

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            setSkills(await skillService.listWorkspaceSkills(workspaceId));
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [workspaceId]);

    useEffect(() => {
        void load();
    }, [load]);

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
            await load();
        } catch {
            setFlash(f => ({ ...f, [ref]: t('assistant.detail.pushFailed', language) }));
        } finally {
            setPushing(null);
        }
    };

    // Open one of this assistant's sessions in the workbench (drops the L1
    // full-page tab, lands in the session view + its L3 breadcrumb).
    const onOpenSession = (s: Session) => void sessStore.selectSession(s);

    // Start a fresh conversation scoped to this assistant.
    const onNewSession = async () => {
        if (ws) await wsStore.selectWorkspace(ws);
        sessStore.onStartNewChat();
    };

    // Badge config per store state.
    const BADGE: Record<WorkspaceSkillStatus['state'], { cls: string; key: string }> = {
        synced: { cls: 'is-synced', key: 'assistant.detail.synced' },
        modified: { cls: 'is-modified', key: 'assistant.detail.modified' },
        local: { cls: 'is-local', key: 'assistant.detail.local' },
    };

    if (!ws) {
        // Stale id (assistant deleted) — drop back to the grid.
        tabs.assistantDetailId.value = null;
        return null;
    }

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
                <button class="assistant-btn assistant-btn-primary" onClick={() => void wsStore.selectWorkspace(ws)}>
                    {t('assistant.detail.openChat', language)}
                </button>
            </div>

            <section class="assistant-section">
                <div class="assistant-section-head">
                    <h2>{t('assistant.detail.sessionsTitle', language)}</h2>
                    <button class="assistant-btn assistant-btn-ghost" onClick={() => void onNewSession()}>
                        {t('assistant.detail.newSession', language)}
                    </button>
                </div>
                <p class="assistant-hint">{t('assistant.detail.sessionsHint', language)}</p>
                {sessions.length === 0 ? (
                    <div class="assistant-empty-row">{t('assistant.detail.sessionsEmpty', language)}</div>
                ) : (
                    <ul class="assistant-session-list">
                        {sessions.map(s => (
                            <li key={s.id}>
                                <button class="assistant-session-row" onClick={() => onOpenSession(s)}>
                                    <span class="assistant-session-name">
                                        {s.name || t('assistant.detail.sessionsTitle', language)}
                                    </span>
                                    <span class="assistant-session-agent">{s.agentType}</span>
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
                            </li>
                        ))}
                    </ul>
                )}
            </section>

            <section class="assistant-section">
                <div class="assistant-section-head">
                    <h2>{t('assistant.detail.soulTitle', language)}</h2>
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
                <p class="assistant-hint">{t('assistant.detail.soulHint', language)}</p>
                <textarea
                    class="assistant-soul-editor"
                    value={soul}
                    placeholder={t('assistant.detail.soulPlaceholder', language)}
                    onInput={(e: Event) => setSoul((e.target as HTMLTextAreaElement).value)}
                />
            </section>

            <section class="assistant-section">
                <h2>{t('assistant.detail.skillsTitle', language)}</h2>
                <p class="assistant-hint">{t('assistant.detail.skillsHint', language)}</p>

                {loading && <div class="assistant-empty-row">…</div>}
                {!loading && error && (
                    <div class="assistant-empty-row">
                        {t('assistant.detail.skillsLoadFailed', language)}: {error}
                    </div>
                )}
                {!loading && !error && skills.length === 0 && (
                    <div class="assistant-empty-row">{t('assistant.detail.skillsEmpty', language)}</div>
                )}
                {!loading && !error && skills.length > 0 && (
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
                                        {flash[s.skillRef] && <span class="assistant-flash">{flash[s.skillRef]}</span>}
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
            </section>
        </div>
    );
}
