import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import { t } from '../i18n';
import * as ui from '../../stores/uiStore';
import * as wsStore from '../../stores/workspaceStore';
import * as tabs from '../../stores/tabsStore';
import { skillService, type WorkspaceSkillStatus } from '@1agents/core/services/skillService';
import { soulService } from '@1agents/core/services/soulService';

/**
 * AssistantDetail — the L1 助理 detail view (reached by clicking a card on
 * AssistantsPage).
 *
 * Today it surfaces the assistant's synced skills and the push-back link (#360
 * reverse-sync): each skill copied into <ws>/.claude/skills shows whether it has
 * drifted from the shared store (母体), and a "推送到母体" button that overwrites
 * the store baseline with the local copy. Prompt / MCP / channel sections will
 * grow here later.
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
            const { changed, created, version } = await skillService.pushSkill(workspaceId, ref);
            const base = created
                ? t('assistant.detail.pushedCreated', language)
                : changed
                  ? t('assistant.detail.pushed', language)
                  : t('assistant.detail.pushNoChange', language);
            // Surface the new store version whenever the push moved it forward.
            const msg = changed && version ? `${base} · v${version}` : base;
            setFlash(f => ({ ...f, [ref]: msg }));
            await load();
        } catch {
            setFlash(f => ({ ...f, [ref]: t('assistant.detail.pushFailed', language) }));
        } finally {
            setPushing(null);
        }
    };

    // Badge + push-button config per store state.
    const BADGE: Record<WorkspaceSkillStatus['state'], { cls: string; key: string }> = {
        synced: { cls: 'is-synced', key: 'assistant.detail.synced' },
        modified: { cls: 'is-modified', key: 'assistant.detail.modified' },
        local: { cls: 'is-local', key: 'assistant.detail.local' },
    };

    const goBack = () => {
        tabs.assistantDetailId.value = null;
    };

    if (!ws) {
        // Stale id (assistant deleted) — drop back to the grid.
        tabs.assistantDetailId.value = null;
        return null;
    }

    return (
        <div class="assistant-detail">
            <header class="assistant-detail-header">
                <button class="assistant-detail-back" onClick={goBack}>
                    <span aria-hidden="true">←</span> {t('assistant.detail.back', language)}
                </button>
                <div class="assistant-detail-ident">
                    {ws.avatar && ws.avatar.startsWith('/') ? (
                        <img class="assistant-detail-avatar" src={ws.avatar} alt="" />
                    ) : (
                        <span class="assistant-detail-emoji" aria-hidden="true">
                            {'\u{1F464}'}
                        </span>
                    )}
                    <h1 class="assistant-detail-name">{ws.name}</h1>
                </div>
                <button class="assistant-detail-openchat" onClick={() => void wsStore.selectWorkspace(ws)}>
                    {t('assistant.detail.openChat', language)}
                </button>
            </header>

            <section class="assistant-detail-soul">
                <h2>{t('assistant.detail.soulTitle', language)}</h2>
                <p class="assistant-detail-hint">{t('assistant.detail.soulHint', language)}</p>
                <textarea
                    class="assistant-soul-editor"
                    value={soul}
                    placeholder={t('assistant.detail.soulPlaceholder', language)}
                    onInput={(e: Event) => setSoul((e.target as HTMLTextAreaElement).value)}
                />
                <div class="assistant-soul-actions">
                    {soulFlash && <span class="assistant-skill-flash">{soulFlash}</span>}
                    <button
                        class="assistant-skill-push"
                        disabled={soul === soulSaved || soulSaving}
                        onClick={() => void onSaveSoul()}
                    >
                        {soulSaving
                            ? t('assistant.detail.soulSaving', language)
                            : t('assistant.detail.soulSave', language)}
                    </button>
                </div>
            </section>

            <section class="assistant-detail-skills">
                <h2>{t('assistant.detail.skillsTitle', language)}</h2>
                <p class="assistant-detail-hint">{t('assistant.detail.skillsHint', language)}</p>

                {loading && <div class="assistant-detail-empty">…</div>}
                {!loading && error && (
                    <div class="assistant-detail-empty">
                        {t('assistant.detail.skillsLoadFailed', language)}: {error}
                    </div>
                )}
                {!loading && !error && skills.length === 0 && (
                    <div class="assistant-detail-empty">{t('assistant.detail.skillsEmpty', language)}</div>
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
                                            {s.version > 0 && <span class="assistant-skill-version">v{s.version}</span>}
                                        </div>
                                        {s.description && <p class="assistant-skill-desc">{s.description}</p>}
                                    </div>
                                    <div class="assistant-skill-actions">
                                        {flash[s.skillRef] && (
                                            <span class="assistant-skill-flash">{flash[s.skillRef]}</span>
                                        )}
                                        {canPush && (
                                            <button
                                                class="assistant-skill-push"
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
