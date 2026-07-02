import { h } from 'preact';

import * as ui from '../../../stores/uiStore';
import { t } from '../../../i18n';

// SourceHome — the 数据源 landing page: one Bento card per data source (飞书 /
// Apple) plus an "添加数据源" card. Picking a card drills into that source (the
// breadcrumb gains a second level and the source's zones show as top-nav tabs).

interface SourceEntry {
    id: string;
    name: string;
    descKey: string;
    icon: string;
    color: string;
}

const SOURCES: SourceEntry[] = [
    { id: 'feishu', name: '飞书', descKey: 'datasource.home.feishuDesc', icon: '💬', color: '#3370ff' },
    { id: 'apple', name: 'Apple', descKey: 'datasource.home.appleDesc', icon: '', color: '#555' },
];

export function SourceHome({ onPick }: { onPick: (id: string) => void }) {
    const language = ui.language.value;
    return (
        <div class="source-home">
            <div class="bento-grid">
                {SOURCES.map(s => (
                    <button key={s.id} class="bento-card source-home-card" onClick={() => onPick(s.id)}>
                        <div class="bento-zone-header">
                            <div
                                class="bento-card-icon source-home-icon"
                                style={`background-color:${s.color}1a;color:${s.color};`}
                            >
                                {s.icon}
                            </div>
                        </div>
                        <div class="bento-zone-body">
                            <h3 class="bento-card-title">{s.name}</h3>
                            <p class="bento-card-desc">{t(s.descKey, language)}</p>
                        </div>
                        <div class="bento-zone-footer">
                            <span class="card-action-text">{t('datasource.home.manage', language)} →</span>
                        </div>
                    </button>
                ))}
                <button class="bento-card source-home-card source-home-add" onClick={() => onPick('add')}>
                    <div class="bento-zone-body source-home-add-body">
                        <span class="source-home-add-plus">+</span>
                        <h3 class="bento-card-title">{t('datasource.tab.add', language)}</h3>
                        <p class="bento-card-desc">{t('datasource.home.addDesc', language)}</p>
                    </div>
                </button>
            </div>
        </div>
    );
}
