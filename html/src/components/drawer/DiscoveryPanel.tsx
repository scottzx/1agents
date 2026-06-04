import { h } from 'preact';
import { t, type Lang } from '../i18n';

interface LinkCard {
    title: string;
    descriptionKey: string;
    badgeKey?: string;
    url: string;
    iconColor?: string;
}

const QUICK_LINKS: LinkCard[] = [
    {
        title: 'NanoSkill.ai',
        descriptionKey: 'discovery.nanoDesc',
        badgeKey: 'discovery.popular',
        url: 'http://nanoskill.ai/',
        iconColor: '#4f46e5', // Royal Indigo
    },
];

interface DiscoveryPanelProps {
    onOpenBrowserTab?: (url: string) => void;
    language: Lang;
}

export function DiscoveryPanel({ onOpenBrowserTab, language }: DiscoveryPanelProps) {
    return (
        <div class="discovery-container">
            <div class="discovery-header-desc">{t('discovery.intro', language)}</div>

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
                            {card.badgeKey && <span class="card-badge">{t(card.badgeKey, language)}</span>}
                        </div>

                        <div class="card-content">
                            <h3 class="card-title">{card.title}</h3>
                            <p class="card-description">{t(card.descriptionKey, language)}</p>
                        </div>

                        <div class="card-footer">
                            <span class="card-action-text">{t('discovery.exploreNow', language)}</span>
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
