import { h, Fragment } from 'preact';
import { useState, useEffect, useCallback, useRef } from 'preact/hooks';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';
import { renderMarkdown } from '../../../utils/markdown';
import { renderMermaidBlocks } from '../../../utils/mermaid';
import { retroService, type RetroItem } from '@1agents/core/services/retroService';

// 复盘归档展示 (#271): read-only view of the retrospectives the project archive
// hook (#144) ingests into the kwiki knowledge base. The list is a bento-grid
// of one card per archived project; opening a card renders the retrospective
// Markdown body (项目元信息 / 任务完成 / 决策记录 / 经验沉淀), reusing the shared
// markdown + mermaid (#203) renderers.
export function RetroPane() {
    const language = ui.language.value;
    const theme = ui.theme.value;
    const [items, setItems] = useState<RetroItem[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState('');
    const [selected, setSelected] = useState<RetroItem | null>(null);
    const bodyRef = useRef<HTMLDivElement>(null);

    const refresh = useCallback(async () => {
        setLoading(true);
        setError('');
        try {
            setItems(await retroService.list());
        } catch (err) {
            setError((err as Error).message);
        } finally {
            setLoading(false);
        }
    }, []);

    useEffect(() => {
        refresh();
    }, [refresh]);

    // Paint ```mermaid blocks once the detail body's HTML is in the DOM; re-runs
    // on theme toggle so diagrams repaint with the matching palette.
    useEffect(() => {
        if (selected) void renderMermaidBlocks(bodyRef.current, theme);
    }, [selected, theme]);

    const fmtDate = (iso?: string) => (iso ? new Date(iso).toLocaleString(language) : '');

    if (selected) {
        return (
            <div class="retro-pane">
                <div class="retro-detail-header">
                    <button class="retro-back-btn" onClick={() => setSelected(null)}>
                        {t('retro.back', language)}
                    </button>
                    <span class="retro-detail-time">{fmtDate(selected.updated || selected.created)}</span>
                </div>
                <div
                    ref={bodyRef}
                    class="markdown-body retro-detail-body"
                    dangerouslySetInnerHTML={{ __html: renderMarkdown(selected.body) }}
                />
            </div>
        );
    }

    return (
        <div class="retro-pane">
            <div class="retro-header">
                <h2 class="retro-title">{t('retro.title', language)}</h2>
            </div>
            <div class="retro-intro">{t('retro.intro', language)}</div>

            {error && <div class="retro-error">{error}</div>}

            {!loading && items.length === 0 && <div class="retro-empty">{t('retro.empty', language)}</div>}

            <div class="retro-grid bento-grid">
                {items.map(item => (
                    <button
                        key={item.slug}
                        type="button"
                        class="bento-card retro-card"
                        onClick={() => setSelected(item)}
                    >
                        <div class="bento-zone-header">
                            <div class="bento-card-icon retro-card-icon">
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                >
                                    <path d="M2 3h6a4 4 0 0 1 4 4v14a3 3 0 0 0-3-3H2z" />
                                    <path d="M22 3h-6a4 4 0 0 0-4 4v14a3 3 0 0 1 3-3h7z" />
                                </svg>
                            </div>
                        </div>
                        <div class="bento-zone-body">
                            <h3 class="bento-card-title">{item.title}</h3>
                            {item.summary && <p class="bento-card-desc">{item.summary}</p>}
                        </div>
                        <div class="bento-zone-footer retro-card-footer">
                            {item.tags && item.tags.length > 0 && (
                                <Fragment>
                                    {item.tags.map(tag => (
                                        <span key={tag} class="retro-tag">
                                            {tag}
                                        </span>
                                    ))}
                                </Fragment>
                            )}
                            <span class="retro-card-time">{fmtDate(item.updated || item.created)}</span>
                        </div>
                    </button>
                ))}
            </div>
        </div>
    );
}
