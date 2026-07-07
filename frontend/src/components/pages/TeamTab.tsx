import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { App } from '../app';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as fs from '../../stores/fsStore';
import { FilePreviewPane } from '../shared/WorkspacePanes';
import { soulService, type WorkspaceTeam } from '@1agents/core/services/soulService';

/**
 * 团队 tab of the 助理/项目 详情 — the single home for personas (the 灵魂 tab was
 * retired into this). Two modes, keyed on whether a PRIMARY agent is set:
 *
 *   • primary set (assistant): show that agent's persona — the .claude/agents
 *     markdown — directly in the file preview/editor (修改 / 保存 built in).
 *   • no primary (project): a left roster of every .claude/agents member; click
 *     one to preview/edit it on the right. Each row can be promoted to primary.
 *
 * Editing reuses the shared fs preview/editor (same component the 灵魂 tab used),
 * so agent files get the full edit/save toolbar for free.
 */
const AGENTS_DIR = '.claude/agents';

export function TeamTab({ workspaceId, app, language }: { workspaceId: string; app: App; language: Lang }) {
    const [team, setTeam] = useState<WorkspaceTeam>({ primary: '', members: [] });
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [busy, setBusy] = useState('');
    const [flash, setFlash] = useState('');
    // The member (<name>.md) whose file is open in the right pane (list mode).
    const [selectedFile, setSelectedFile] = useState('');

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

    // Open an agent's markdown into the shared fs preview/editor.
    const openAgentFile = useCallback((file: string) => {
        if (!file) return;
        const path = `${AGENTS_DIR}/${file}`;
        void fs.openFileDetail({ name: file, path, isDir: false, size: 0, modTime: 0 });
    }, []);

    // Single-persona mode (primary set): auto-open the primary's file. List mode:
    // open the selected member, defaulting to the first once the roster loads.
    useEffect(() => {
        if (loading || error) return;
        if (team.primary) {
            openAgentFile(team.primary);
            return;
        }
        const target = selectedFile || team.members[0]?.file || '';
        if (target) {
            setSelectedFile(target);
            openAgentFile(target);
        }
    }, [loading, error, team.primary, team.members, selectedFile, openAgentFile]);

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

    if (loading) return <div class="assistant-empty-row">…</div>;
    if (error)
        return (
            <div class="assistant-empty-row">
                {t('assistant.detail.team.loadFailed', language)}: {error}
            </div>
        );

    // ── Single-persona mode — the assistant's primary agent ──────────────────
    if (team.primary) {
        const primaryName = team.members.find(m => m.file === team.primary)?.name || team.primary;
        return (
            <div class="team-tab team-tab-single">
                <div class="team-single-head">
                    <span class="team-single-name">{primaryName}</span>
                    <span class="assistant-skill-badge is-primary-badge">
                        {t('assistant.detail.team.primaryBadge', language)}
                    </span>
                    {flash && <span class="assistant-flash">{flash}</span>}
                    <button
                        class="assistant-btn assistant-btn-ghost team-single-unset"
                        disabled={busy !== ''}
                        onClick={() => void setPrimary('')}
                    >
                        {t('assistant.detail.team.unsetPrimary', language)}
                    </button>
                </div>
                <div class="team-single-preview">
                    <FilePreviewPane app={app} language={language} hideBack />
                </div>
            </div>
        );
    }

    // ── List mode — a project's agent roster (master-detail) ─────────────────
    if (team.members.length === 0) {
        return <div class="assistant-empty-row">{t('assistant.detail.team.empty', language)}</div>;
    }
    return (
        <div class="team-tab team-tab-list">
            <p class="assistant-hint">{t('assistant.detail.team.hint', language)}</p>
            {flash && <span class="assistant-flash">{flash}</span>}
            <div class="file-split">
                <div class="file-split-list team-member-list">
                    {team.members.map(m => (
                        <div key={m.agentRef} class={`team-member-row${selectedFile === m.file ? ' is-active' : ''}`}>
                            <button
                                class="team-member-main"
                                onClick={() => {
                                    setSelectedFile(m.file);
                                    openAgentFile(m.file);
                                }}
                            >
                                <span class="team-member-name">{m.name || m.file}</span>
                                {m.description && <span class="team-member-desc">{m.description}</span>}
                            </button>
                            <button
                                class="team-member-setprimary"
                                title={t('assistant.detail.team.setPrimary', language)}
                                disabled={busy !== ''}
                                onClick={() => void setPrimary(m.file)}
                            >
                                {t('assistant.detail.team.setPrimary', language)}
                            </button>
                        </div>
                    ))}
                </div>
                <div class="file-split-preview">
                    <FilePreviewPane app={app} language={language} hideBack />
                </div>
            </div>
        </div>
    );
}
