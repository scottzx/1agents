import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import { soulService, type WorkspaceTeam } from '@1agents/core/services/soulService';

/**
 * 团队 tab of the 助理/项目 详情. Lists the agent team (<ws>/.claude/agents/*.md)
 * and lets the user pick which member is the PRIMARY — the persona that drives
 * the default conversation. Empty primary (or a bare project) injects no persona;
 * the input-box expert picker (NewChatHome) can still choose a member per chat.
 *
 * Adding/removing agents goes through the shared 1skills agent flow; this tab is
 * the roster + primary control.
 */
const BADGE: Record<string, string> = {
    synced: 'is-synced',
    modified: 'is-modified',
    local: 'is-local',
};

export function TeamTab({ workspaceId, language }: { workspaceId: string; language: Lang }) {
    const [team, setTeam] = useState<WorkspaceTeam>({ primary: '', members: [] });
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState('');
    const [flash, setFlash] = useState('');

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            setTeam(await soulService.getWorkspaceTeam(workspaceId));
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [workspaceId]);
    useEffect(() => {
        void load();
    }, [load]);

    const setPrimary = async (file: string) => {
        setBusy(file || '__clear__');
        try {
            await soulService.setWorkspacePrimary(workspaceId, file);
            setFlash(t('assistant.detail.team.primarySet', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.team.primaryFailed', language));
        } finally {
            setBusy('');
        }
    };

    return (
        <div class="skills-tab">
            <p class="assistant-hint">{t('assistant.detail.team.hint', language)}</p>
            {flash && <span class="assistant-flash">{flash}</span>}
            {loading && <div class="assistant-empty-row">…</div>}
            {!loading && error && (
                <div class="assistant-empty-row">
                    {t('assistant.detail.team.loadFailed', language)}: {error}
                </div>
            )}
            {!loading && !error && team.members.length === 0 && (
                <div class="assistant-empty-row">{t('assistant.detail.team.empty', language)}</div>
            )}
            {!loading && !error && team.members.length > 0 && (
                <div class="skills-grid">
                    {team.members.map(m => {
                        const isPrimary = m.file === team.primary && team.primary !== '';
                        return (
                            <div key={m.agentRef} class={`skill-card team-card${isPrimary ? ' is-primary' : ''}`}>
                                <div class="skill-card-head">
                                    <span class="skill-card-name">{m.name || m.file}</span>
                                    {isPrimary && (
                                        <span class="assistant-skill-badge is-primary-badge">
                                            {t('assistant.detail.team.primaryBadge', language)}
                                        </span>
                                    )}
                                    {!isPrimary && m.state && BADGE[m.state] && (
                                        <span class={`assistant-skill-badge ${BADGE[m.state]}`}>
                                            {t(`assistant.detail.${m.state}`, language)}
                                        </span>
                                    )}
                                </div>
                                {m.description && <p class="skill-card-desc">{m.description}</p>}
                                <div class="team-card-actions">
                                    {isPrimary ? (
                                        <button
                                            class="assistant-btn assistant-btn-ghost"
                                            disabled={busy !== ''}
                                            onClick={() => void setPrimary('')}
                                        >
                                            {t('assistant.detail.team.unsetPrimary', language)}
                                        </button>
                                    ) : (
                                        <button
                                            class="assistant-btn assistant-btn-ghost"
                                            disabled={busy !== ''}
                                            onClick={() => void setPrimary(m.file)}
                                        >
                                            {t('assistant.detail.team.setPrimary', language)}
                                        </button>
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
}
