import { h } from 'preact';
import { useState, useEffect, useCallback } from 'preact/hooks';
import type { App } from '../app';
import type { Lang } from '../i18n';
import { t } from '../i18n';
import * as fs from '../../stores/fsStore';
import * as taskNav from '../../stores/taskNavStore';
import { fsService } from '../../services/fsService';
import { FilePreviewPane } from '../shared/WorkspacePanes';
import { soulService, type TeamMember, type AvailableAgent } from '@1agents/core/services/soulService';
import { looksLikeFrontmatterYaml } from '../../utils/frontmatter';

/**
 * 团队 tab — the persona home (the 灵魂 tab was retired into this). Mirrors the
 * 技能 tab shape: a card roster you add to / open, then a member detail with a
 * metadata header (name + 核心/删除 actions, then 概述/工具) over the agent's single
 * markdown file. Because an agent is one .md, the detail hides the file list and
 * shows the file preview/editor directly.
 *
 * 核心 (primary) = the agent whose persona drives the default conversation.
 */
const AGENTS_DIR = '.claude/agents';

/** Pull `tools:` (and a description fallback) out of an agent .md frontmatter. */
function parseFrontmatter(md: string): { description: string; tools: string } {
    const s = md.replace(/^\uFEFF/, '');
    // Require `---` to be on its own line so it isn't confused with thematic
    // breaks (`--- / # Title / ---` is a common README header shape, not
    // frontmatter). Mirrors utils/frontmatter.ts to stay consistent with the
    // task-card parser.
    if (!s.startsWith('---\n') && !s.startsWith('---\r\n')) {
        return { description: '', tools: '' };
    }
    const rest = s.slice(s.indexOf('\n') + 1);
    const lines = rest.split('\n');
    let endIdx = -1;
    for (let i = 0; i < lines.length; i++) {
        if (lines[i].replace(/\r$/, '') === '---') {
            endIdx = i;
            break;
        }
    }
    if (endIdx < 0) return { description: '', tools: '' };
    const candidate = lines.slice(0, endIdx);
    if (!looksLikeFrontmatterYaml(candidate)) {
        return { description: '', tools: '' };
    }
    const block = candidate.join('\n');
    let description = '';
    let tools = '';
    for (const line of block.split('\n')) {
        const i = line.indexOf(':');
        if (i < 0) continue;
        const key = line.slice(0, i).trim();
        const val = line
            .slice(i + 1)
            .trim()
            .replace(/^["']|["']$/g, '');
        if (key === 'description') description = val;
        else if (key === 'tools') tools = val;
    }
    return { description, tools };
}

export function TeamTab({ workspaceId, app, language }: { workspaceId: string; app: App; language: Lang }) {
    const [primary, setPrimary] = useState('');
    const [members, setMembers] = useState<TeamMember[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [selected, setSelected] = useState<TeamMember | null>(null);
    const [busy, setBusy] = useState(false);
    const [flash, setFlash] = useState('');
    const [meta, setMeta] = useState<{ description: string; tools: string }>({ description: '', tools: '' });
    // Add-from-母体 picker.
    const [pickerOpen, setPickerOpen] = useState(false);
    const [available, setAvailable] = useState<AvailableAgent[]>([]);
    const [pickerBusy, setPickerBusy] = useState(false);
    const [pickerQuery, setPickerQuery] = useState('');
    const [confirmRemove, setConfirmRemove] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            const team = await soulService.getWorkspaceTeam(workspaceId);
            setPrimary(team.primary);
            setMembers(team.members);
            // Keep the open member in sync (e.g. after set-primary reload).
            setSelected(prev => (prev ? team.members.find(m => m.file === prev.file) ?? null : null));
        } catch (e) {
            setError(String(e));
        } finally {
            setLoading(false);
        }
    }, [workspaceId]);
    useEffect(() => {
        void load();
    }, [load]);

    // On opening a member: preview its .md and parse 概述/工具 from frontmatter.
    useEffect(() => {
        if (!selected) return;
        const path = `${AGENTS_DIR}/${selected.file}`;
        void fs.openFileDetail({ name: selected.file, path, isDir: false, size: 0, modTime: 0 });
        let cancelled = false;
        fsService
            .read(path)
            .then(content => {
                if (!cancelled) setMeta(parseFrontmatter(typeof content === 'string' ? content : ''));
            })
            .catch(() => !cancelled && setMeta({ description: '', tools: '' }));
        return () => {
            cancelled = true;
        };
    }, [selected]);

    const makePrimary = async (file: string) => {
        setBusy(true);
        try {
            await soulService.setWorkspacePrimary(workspaceId, file);
            setFlash(t('assistant.detail.team.primarySet', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.team.primaryFailed', language));
        } finally {
            setBusy(false);
        }
    };

    const openPicker = async () => {
        setPickerOpen(true);
        setPickerQuery('');
        setPickerBusy(true);
        try {
            setAvailable(await soulService.listAvailableAgents(workspaceId));
        } catch {
            setAvailable([]);
        } finally {
            setPickerBusy(false);
        }
    };

    const onAdd = async (agentRef: string) => {
        setPickerBusy(true);
        try {
            await soulService.addAgent(workspaceId, agentRef);
            setPickerOpen(false);
            setFlash(t('assistant.detail.added', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.team.addFailed', language));
        } finally {
            setPickerBusy(false);
        }
    };

    const onRemove = async () => {
        if (!selected) return;
        setBusy(true);
        try {
            await soulService.removeAgent(workspaceId, selected.file);
            setConfirmRemove(false);
            setSelected(null);
            setFlash(t('assistant.detail.removed', language));
            await load();
        } catch {
            setFlash(t('assistant.detail.team.removeFailed', language));
        } finally {
            setBusy(false);
        }
    };

    useEffect(() => {
        if (!selected) return;
        return taskNav.registerHeaderBackAction(
            `team-member-detail:${workspaceId}`,
            () => setSelected(null),
            taskNav.HEADER_BACK_PRIORITY.detail
        );
    }, [selected, workspaceId]);

    if (loading) return <div class="assistant-empty-row">…</div>;
    if (error)
        return (
            <div class="assistant-empty-row">
                {t('assistant.detail.team.loadFailed', language)}: {error}
            </div>
        );

    // ── Member detail ────────────────────────────────────────────────────────
    if (selected) {
        const isPrimary = selected.file === primary && primary !== '';
        const desc = meta.description || selected.description;
        return (
            <div class="skill-detail team-detail">
                <div class="skill-detail-head">
                    {/* Row 1: title + 核心 tag on the left, quick actions on the right. */}
                    <div class="skill-detail-row1">
                        <div class="skill-detail-titlewrap">
                            <div class="skill-detail-title">
                                <span class="skill-detail-name">{selected.name || selected.file}</span>
                                {isPrimary && (
                                    <span class="assistant-skill-badge is-primary-badge">
                                        {t('assistant.detail.team.primaryBadge', language)}
                                    </span>
                                )}
                            </div>
                        </div>
                        <div class="skill-detail-actions">
                            {flash && <span class="assistant-flash">{flash}</span>}
                            {!isPrimary && (
                                <button
                                    class="assistant-btn assistant-btn-ghost"
                                    disabled={busy}
                                    onClick={() => void makePrimary(selected.file)}
                                >
                                    {t('assistant.detail.team.setPrimary', language)}
                                </button>
                            )}
                            {isPrimary && (
                                <button
                                    class="assistant-btn assistant-btn-ghost"
                                    disabled={busy}
                                    onClick={() => void makePrimary('')}
                                >
                                    {t('assistant.detail.team.unsetPrimary', language)}
                                </button>
                            )}
                            <button
                                class="assistant-btn assistant-btn-danger"
                                disabled={busy}
                                onClick={() => setConfirmRemove(true)}
                            >
                                {t('assistant.detail.remove', language)}
                            </button>
                        </div>
                    </div>
                    {/* Row 2+: metadata — 概述 / 工具. */}
                    {desc && <p class="skill-detail-desc">{desc}</p>}
                    {meta.tools && (
                        <div class="team-meta-tools">
                            <span class="team-meta-label">{t('assistant.detail.team.tools', language)}</span>
                            <div class="team-tool-chips">
                                {meta.tools
                                    .split(',')
                                    .map(s => s.trim())
                                    .filter(Boolean)
                                    .map(tool => (
                                        <span key={tool} class="team-tool-chip">
                                            {tool}
                                        </span>
                                    ))}
                            </div>
                        </div>
                    )}
                </div>
                {/* Single-file agent → hide the file list, show the editor directly. */}
                <div class="team-detail-preview">
                    <FilePreviewPane app={app} language={language} hideBack />
                </div>
                {confirmRemove && (
                    <div class="ws-modal-overlay" onClick={() => !busy && setConfirmRemove(false)}>
                        <div class="ws-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                            <div class="ws-modal-header">
                                <span>{t('assistant.detail.team.removeConfirmTitle', language)}</span>
                                <button class="ws-modal-close" onClick={() => setConfirmRemove(false)}>
                                    ✕
                                </button>
                            </div>
                            <div class="ws-modal-body">
                                <p>
                                    {t('assistant.detail.team.removeConfirmBody', language, {
                                        name: selected.name || selected.file,
                                    })}
                                </p>
                            </div>
                            <div class="ws-modal-footer">
                                <button class="ws-modal-cancel" onClick={() => setConfirmRemove(false)} disabled={busy}>
                                    {t('assistant.detail.cancel', language)}
                                </button>
                                <button
                                    class="ws-modal-confirm ws-modal-danger"
                                    onClick={() => void onRemove()}
                                    disabled={busy}
                                >
                                    {t('assistant.detail.confirmRemove', language)}
                                </button>
                            </div>
                        </div>
                    </div>
                )}
            </div>
        );
    }

    // ── Roster (card grid) ───────────────────────────────────────────────────
    return (
        <div class="skills-tab team-tab">
            <p class="assistant-hint">{t('assistant.detail.team.hint', language)}</p>
            {flash && <span class="assistant-flash">{flash}</span>}
            <div class="skills-grid">
                <button class="skill-card skill-add-card" onClick={() => void openPicker()}>
                    <span class="skill-add-plus">＋</span>
                    <span class="skill-add-label">{t('assistant.detail.team.addMember', language)}</span>
                </button>
                {members.map(m => {
                    const isPrimary = m.file === primary && primary !== '';
                    return (
                        <button
                            key={m.agentRef}
                            class={`skill-card team-card${isPrimary ? ' is-primary' : ''}`}
                            onClick={() => setSelected(m)}
                        >
                            <div class="skill-card-head">
                                <span class="skill-card-name">{m.name || m.file}</span>
                                {isPrimary && (
                                    <span class="assistant-skill-badge is-primary-badge">
                                        {t('assistant.detail.team.primaryBadge', language)}
                                    </span>
                                )}
                            </div>
                            {m.description && <p class="skill-card-desc">{m.description}</p>}
                        </button>
                    );
                })}
            </div>
            {pickerOpen && (
                <div class="ws-modal-overlay" onClick={() => !pickerBusy && setPickerOpen(false)}>
                    <div class="ws-modal skill-picker-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                        <div class="ws-modal-header">
                            <span>{t('assistant.detail.team.addMemberTitle', language)}</span>
                            <button class="ws-modal-close" onClick={() => setPickerOpen(false)}>
                                ✕
                            </button>
                        </div>
                        <div class="ws-modal-body">
                            <input
                                class="ws-modal-input skill-picker-search"
                                type="search"
                                placeholder={t('assistant.detail.team.addMemberSearch', language)}
                                value={pickerQuery}
                                onInput={(e: Event) => setPickerQuery((e.target as HTMLInputElement).value)}
                            />
                            {(() => {
                                const q = pickerQuery.trim().toLowerCase();
                                const pool = available.filter(a => !a.installed);
                                const filtered = q
                                    ? pool.filter(a =>
                                          [a.name, a.file, a.description]
                                              .filter(Boolean)
                                              .some(s => (s as string).toLowerCase().includes(q))
                                      )
                                    : pool;
                                if (pickerBusy && available.length === 0)
                                    return <div class="assistant-empty-row">…</div>;
                                if (pool.length === 0)
                                    return (
                                        <div class="assistant-empty-row">
                                            {t('assistant.detail.team.addMemberEmpty', language)}
                                        </div>
                                    );
                                if (filtered.length === 0)
                                    return (
                                        <div class="assistant-empty-row">
                                            {t('assistant.detail.team.addMemberNoMatch', language)}
                                        </div>
                                    );
                                return (
                                    <div class="skill-picker-grid">
                                        {filtered.map(a => (
                                            <button
                                                key={a.agentRef}
                                                class="skill-card skill-picker-card"
                                                disabled={pickerBusy}
                                                onClick={() => void onAdd(a.agentRef)}
                                            >
                                                <div class="skill-card-head">
                                                    <span class="skill-card-name">{a.name || a.file}</span>
                                                </div>
                                                {a.description && <p class="skill-card-desc">{a.description}</p>}
                                            </button>
                                        ))}
                                    </div>
                                );
                            })()}
                        </div>
                    </div>
                </div>
            )}
        </div>
    );
}
