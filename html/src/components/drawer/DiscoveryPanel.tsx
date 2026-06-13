import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { t, type Lang } from '../i18n';

type CategoryId = 'featured' | 'opensource';

interface LinkCard {
    title: string;
    descriptionKey: string;
    badgeKey?: string;
    url: string;
    iconColor?: string;
    category: CategoryId;
    /** Featured cards occupy a double-width bento cell when space allows. */
    span2?: boolean;
}

const CATEGORIES: { id: CategoryId; titleKey: string }[] = [
    { id: 'featured', titleKey: 'discovery.catFeatured' },
    { id: 'opensource', titleKey: 'discovery.catOpensource' },
];

const QUICK_LINKS: LinkCard[] = [
    {
        title: 'NanoSkill.ai',
        descriptionKey: 'discovery.nanoDesc',
        badgeKey: 'discovery.popular',
        url: 'http://nanoskill.ai/',
        iconColor: '#4f46e5', // Royal Indigo
        category: 'featured',
        span2: true,
    },
    {
        title: '1gateway',
        descriptionKey: 'discovery.gatewayDesc',
        badgeKey: 'discovery.opensourceBadge',
        url: 'https://github.com/scottzx/1gateway',
        iconColor: '#16a34a', // Open-source Green
        category: 'opensource',
    },
];

interface DiscoveryPanelProps {
    onOpenBrowserTab?: (url: string) => void;
    language: Lang;
    /** When set, smoothly scroll the matching category section into view. */
    scrollToCategory?: string;
}

export function DiscoveryPanel({ onOpenBrowserTab, language, scrollToCategory }: DiscoveryPanelProps) {
    const containerRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (!scrollToCategory || !containerRef.current) return;
        const section = containerRef.current.querySelector(`#discovery-section-${scrollToCategory}`);
        if (section) section.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, [scrollToCategory]);

    const renderCard = (card: LinkCard, idx: number) => (
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
            class={`discovery-card${card.span2 ? ' bento-span-2' : ''}`}
        >
            <div class="bento-zone-header">
                <div
                    class="bento-card-icon"
                    style={`background-color: ${card.iconColor}15; color: ${card.iconColor};`}
                >
                    <svg
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
                    </svg>
                </div>
                {card.badgeKey && <span class="bento-card-badge">{t(card.badgeKey, language)}</span>}
            </div>

            <div class="bento-zone-body">
                <h3 class="bento-card-title">{card.title}</h3>
                <p class="bento-card-desc">{t(card.descriptionKey, language)}</p>
            </div>

            <div class="bento-zone-footer">
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
    );

    return (
        <div class="discovery-container" ref={containerRef}>
            <div class="discovery-header-desc">{t('discovery.intro', language)}</div>

            {CATEGORIES.map(cat => {
                const cards = QUICK_LINKS.filter(c => c.category === cat.id);
                if (cards.length === 0) return null;
                return (
                    <section class="discovery-section" id={`discovery-section-${cat.id}`} key={cat.id}>
                        <h2 class="discovery-section-title">{t(cat.titleKey, language)}</h2>
                        <div class="discovery-cards-list bento-grid">{cards.map(renderCard)}</div>
                    </section>
                );
            })}
        </div>
    );
}
