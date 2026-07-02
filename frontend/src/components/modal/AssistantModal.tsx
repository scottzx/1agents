import { h, Fragment } from 'preact';
import { useEffect, useMemo, useRef, useState } from 'preact/hooks';
import { t, type Lang } from '../i18n';
import { skillService, type SkillRow } from '@1agents/core/services/skillService';
import { soulService, type SoulPreset } from '@1agents/core/services/soulService';
import { workspaceService } from '@1agents/core/services/workspaceService';
import * as wsStore from '../../stores/workspaceStore';

interface AssistantModalProps {
    name: string;
    skills: string[];
    soul: string;
    onNameChange: (val: string) => void;
    onSkillsChange: (val: string[]) => void;
    onSoulChange: (val: string) => void;
    onClose: () => void;
    onSubmit: (avatar: string) => void;
    language: Lang;
}

type BottomTab = 'skills' | 'prompt' | 'mcp' | 'channel';

// Built-in avatar set: generated offline with agy, embedded in the backend
// binary and served under /avatars/presets/ (see workspace/avatar.go). Keep in
// sync with backend/internal/workspace/presets/.
const AVATAR_PRESETS = Array.from({ length: 8 }, (_, i) => `/avatars/presets/preset-${i + 1}.png`);

/**
 * AssistantModal — the create-assistant surface.
 *
 * Layout:
 *   ┌──────────────────────────────────────────┐
 *   │ 上区: 名字 + 头像(预置图片 / 本地上传)     │  ← basic identity
 *   ├──────────────────────────────────────────┤
 *   │ Tabs: 技能 | 系统提示词 | MCP | 通道       │  ← capabilities
 *   │  [tab body]                              │
 *   └──────────────────────────────────────────┘
 *
 * Only the 技能 tab does real work today (weak-copies into <ws>/.claude/skills);
 * the other three tabs render an "即将上线" placeholder so the shell is ready
 * to grow (系统提示词 / MCP / IM 通道) without another modal rewrite.
 *
 * Name uniqueness is checked against the local workspaces signal before submit
 * — the backend also returns 409 with a Chinese message, which the store surfaces
 * via toast; the modal stays open when submit is refused.
 */
export function AssistantModal(props: AssistantModalProps) {
    const { name, skills, soul, onNameChange, onSkillsChange, onSoulChange, onClose, onSubmit, language } = props;
    const [avatar, setAvatar] = useState<string>(AVATAR_PRESETS[0]); // always a served URL
    const [tab, setTab] = useState<BottomTab>('skills');

    // Persona presets (人设) — loaded once on mount.
    const [souls, setSouls] = useState<SoulPreset[]>([]);
    const [soulsLoading, setSoulsLoading] = useState(true);
    const [soulSearch, setSoulSearch] = useState('');
    useEffect(() => {
        let cancelled = false;
        soulService
            .listSouls(language)
            .then(list => {
                if (!cancelled) {
                    setSouls(list);
                    setSoulsLoading(false);
                }
            })
            .catch(() => {
                if (!cancelled) setSoulsLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [language]);

    const filteredSouls = useMemo(() => {
        const q = soulSearch.trim().toLowerCase();
        if (!q) return souls;
        return souls.filter(s => s.title.toLowerCase().includes(q) || s.summary.toLowerCase().includes(q));
    }, [souls, soulSearch]);

    // Skills list — loaded once on mount.
    const [rows, setRows] = useState<SkillRow[]>([]);
    const [skillsLoading, setSkillsLoading] = useState(true);
    const [skillsError, setSkillsError] = useState<string | null>(null);
    useEffect(() => {
        let cancelled = false;
        skillService
            .listSkills()
            .then(list => {
                if (cancelled) return;
                setRows(list);
                setSkillsLoading(false);
            })
            .catch((err: unknown) => {
                if (cancelled) return;
                setSkillsError(String(err));
                setSkillsLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, []);

    // Avatar upload (custom image beyond the preset set).
    const [avatarError, setAvatarError] = useState<string | null>(null);
    const fileInputRef = useRef<HTMLInputElement | null>(null);
    const uploadAvatar = async (file: File) => {
        setAvatarError(null);
        try {
            const url = await workspaceService.uploadAvatar(file);
            setAvatar(url);
        } catch (err) {
            console.error(err);
            setAvatarError(t('assistant.avatar.uploadFailed', language));
        }
    };
    const onFileChange = (e: Event) => {
        const input = e.target as HTMLInputElement;
        const f = input.files?.[0];
        if (f) void uploadAvatar(f);
        input.value = '';
    };

    // Name uniqueness — surface right below the name input.
    const nameConflict = useMemo(() => {
        const target = name.trim().toLowerCase();
        if (!target) return null;
        const match = wsStore.workspaces.value.find(w => w.name.trim().toLowerCase() === target);
        if (!match) return null;
        const kindLabel =
            (match.kind ?? 'project') === 'assistant'
                ? t('assistant.form.nameTakenAssistant', language)
                : t('assistant.form.nameTakenProject', language);
        return t('assistant.form.nameTaken', language, { kind: kindLabel });
    }, [name, language]);

    const toggleSkill = (ref: string) => {
        onSkillsChange(skills.includes(ref) ? skills.filter(r => r !== ref) : [...skills, ref]);
    };

    const canSubmit = name.trim().length > 0 && !nameConflict;

    return (
        <div class="ws-modal-overlay" onClick={onClose}>
            <div class="ws-modal assistant-modal" onClick={(e: MouseEvent) => e.stopPropagation()}>
                <div class="ws-modal-header">
                    <span>{t('modal.assistant.createTitle', language)}</span>
                    <button class="ws-modal-close" onClick={onClose}>
                        ✕
                    </button>
                </div>

                {/* ── 上区: 基本资料 ──────────────────────────────────── */}
                <div class="assistant-modal-basics">
                    <div class="assistant-modal-avatar-col">
                        <div class="assistant-avatar-preview">
                            <img src={avatar} alt="" />
                        </div>
                        <label class="ws-modal-label">{t('assistant.avatar.presets', language)}</label>
                        <div class="assistant-avatar-preset-row">
                            {AVATAR_PRESETS.map(url => (
                                <button
                                    key={url}
                                    type="button"
                                    class={`assistant-avatar-preset-btn${avatar === url ? ' selected' : ''}`}
                                    onClick={() => setAvatar(url)}
                                >
                                    <img src={url} alt="" loading="lazy" />
                                </button>
                            ))}
                        </div>
                        <div class="assistant-avatar-actions">
                            <button type="button" class="ws-modal-cancel" onClick={() => fileInputRef.current?.click()}>
                                {t('assistant.avatar.upload', language)}
                            </button>
                            <input
                                ref={fileInputRef}
                                type="file"
                                accept="image/*"
                                style="display:none"
                                onChange={onFileChange}
                            />
                        </div>
                        {avatarError && <div class="assistant-avatar-error">{avatarError}</div>}
                    </div>
                    <div class="assistant-modal-name-col">
                        <label class="ws-modal-label">{t('modal.assistant.name', language)}</label>
                        <input
                            class="ws-modal-input"
                            placeholder={t('modal.assistant.namePlaceholder', language)}
                            value={name}
                            onInput={(e: Event) => onNameChange((e.target as HTMLInputElement).value)}
                            onKeyDown={(e: KeyboardEvent) => {
                                if (e.key === 'Enter' && canSubmit) onSubmit(avatar);
                            }}
                            autoFocus
                        />
                        {nameConflict && <div class="assistant-name-conflict">{nameConflict}</div>}
                    </div>
                </div>

                {/* ── 下区: 多 Tab 能力配置 ────────────────────────────── */}
                <div class="assistant-modal-tabs">
                    {(
                        [
                            ['skills', 'assistant.tab.skills'],
                            ['prompt', 'assistant.tab.prompt'],
                            ['mcp', 'assistant.tab.mcp'],
                            ['channel', 'assistant.tab.channel'],
                        ] as const
                    ).map(([id, key]) => (
                        <button
                            key={id}
                            type="button"
                            class={`assistant-modal-tab${tab === id ? ' active' : ''}`}
                            onClick={() => setTab(id)}
                        >
                            {t(key, language)}
                        </button>
                    ))}
                </div>
                <div class="assistant-modal-tab-body">
                    {tab === 'skills' && (
                        <Fragment>
                            <p class="ws-modal-hint">{t('modal.assistant.skillsHint', language)}</p>
                            <div class="assistant-modal-skill-grid">
                                {skillsLoading && <div class="assistant-modal-skills-empty">…</div>}
                                {!skillsLoading && skillsError && (
                                    <div class="assistant-modal-skills-empty">{skillsError}</div>
                                )}
                                {!skillsLoading && !skillsError && rows.length === 0 && (
                                    <div class="assistant-modal-skills-empty">
                                        {t('modal.assistant.skillsEmpty', language)}
                                    </div>
                                )}
                                {!skillsLoading &&
                                    !skillsError &&
                                    rows.map(row => {
                                        const checked = skills.includes(row.skillRef);
                                        return (
                                            <button
                                                key={row.skillRef}
                                                type="button"
                                                class={`assistant-skill-card${checked ? ' checked' : ''}`}
                                                onClick={() => toggleSkill(row.skillRef)}
                                            >
                                                <div class="assistant-skill-card-title">{row.name}</div>
                                                {row.description && (
                                                    <div class="assistant-skill-card-desc">{row.description}</div>
                                                )}
                                            </button>
                                        );
                                    })}
                            </div>
                        </Fragment>
                    )}
                    {tab === 'prompt' && (
                        <Fragment>
                            <p class="ws-modal-hint">{t('modal.assistant.soulHint', language)}</p>
                            <input
                                class="ws-modal-input assistant-soul-search"
                                placeholder={t('modal.assistant.soulSearch', language)}
                                value={soulSearch}
                                onInput={(e: Event) => setSoulSearch((e.target as HTMLInputElement).value)}
                            />
                            <div class="assistant-modal-skill-grid">
                                {/* 空人设 — always first, selected when no ref chosen. */}
                                <button
                                    type="button"
                                    class={`assistant-skill-card${soul === '' ? ' checked' : ''}`}
                                    onClick={() => onSoulChange('')}
                                >
                                    <div class="assistant-skill-card-title">
                                        {t('modal.assistant.soulBlank', language)}
                                    </div>
                                    <div class="assistant-skill-card-desc">
                                        {t('modal.assistant.soulBlankDesc', language)}
                                    </div>
                                </button>
                                {soulsLoading && <div class="assistant-modal-skills-empty">…</div>}
                                {!soulsLoading &&
                                    filteredSouls.map(s => (
                                        <button
                                            key={s.ref}
                                            type="button"
                                            class={`assistant-skill-card${soul === s.ref ? ' checked' : ''}`}
                                            onClick={() => onSoulChange(s.ref)}
                                            title={s.content.slice(0, 400)}
                                        >
                                            <div class="assistant-skill-card-title">{s.title}</div>
                                            {s.summary && <div class="assistant-skill-card-desc">{s.summary}</div>}
                                        </button>
                                    ))}
                            </div>
                        </Fragment>
                    )}
                    {(tab === 'mcp' || tab === 'channel') && (
                        <div class="assistant-modal-tab-placeholder">{t('assistant.tab.comingSoon', language)}</div>
                    )}
                </div>

                <div class="ws-modal-footer">
                    <button class="ws-modal-cancel" onClick={onClose}>
                        {t('common.cancel', language)}
                    </button>
                    <button class="ws-modal-confirm" disabled={!canSubmit} onClick={() => onSubmit(avatar)}>
                        {t('modal.assistant.create', language)}
                    </button>
                </div>
            </div>
        </div>
    );
}
