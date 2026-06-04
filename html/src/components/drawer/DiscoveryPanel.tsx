import { h } from 'preact';

interface LinkCard {
    title: string;
    description: string;
    url: string;
    badge?: string;
    iconColor?: string;
}

const QUICK_LINKS: LinkCard[] = [
    {
        title: 'NanoSkill.ai',
        description:
            '垂直于营销领域的 AI Agent 技能库与发布平台。提供开箱即用的专业 AI 技能，助力增长与自动化营销操作。',
        url: 'http://nanoskill.ai/',
        badge: '热门推荐',
        iconColor: '#4f46e5', // Royal Indigo
    },
];

interface DiscoveryPanelProps {
    onOpenBrowserTab?: (url: string) => void;
}

export function DiscoveryPanel({ onOpenBrowserTab }: DiscoveryPanelProps) {
    return (
        <div class="discovery-container">
            <div class="discovery-header-desc">
                精选各类实用的 AI 辅助工具与技能库，点击即可在内置浏览器中快速访问。
            </div>

            <div class="discovery-cards-list">
                {QUICK_LINKS.map((card, idx) => (
                    <a
                        key={idx}
                        href={card.url}
                        target="_blank"
                        rel="noopener noreferrer"
                        onClick={e => {
                            if (onOpenBrowserTab) {
                                e.preventDefault();
                                onOpenBrowserTab(card.url);
                            }
                        }}
                        class="discovery-card"
                    >
                        <div class="card-top">
                            <div
                                class="card-icon-wrapper"
                                style={`background-color: ${card.iconColor}15; color: ${card.iconColor};`}
                            >
                                <svg
                                    viewBox="0 0 24 24"
                                    fill="none"
                                    stroke="currentColor"
                                    stroke-width="2"
                                    stroke-linecap="round"
                                    stroke-linejoin="round"
                                    class="card-icon"
                                >
                                    <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                                </svg>
                            </div>
                            {card.badge && <span class="card-badge">{card.badge}</span>}
                        </div>

                        <div class="card-content">
                            <h3 class="card-title">{card.title}</h3>
                            <p class="card-description">{card.description}</p>
                        </div>

                        <div class="card-footer">
                            <span class="card-action-text">立即探索</span>
                            <svg
                                viewBox="0 0 24 24"
                                fill="none"
                                stroke="currentColor"
                                stroke-width="2.5"
                                stroke-linecap="round"
                                stroke-linejoin="round"
                                class="arrow-right-icon"
                            >
                                <line x1="5" y1="12" x2="19" y2="12" />
                                <polyline points="12 5 19 12 12 19" />
                            </svg>
                        </div>
                    </a>
                ))}
            </div>
        </div>
    );
}
