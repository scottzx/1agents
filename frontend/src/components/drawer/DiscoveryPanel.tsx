import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import { t, type Lang } from '../i18n';
import * as appStore from '../../stores/appManifestStore';
import type { AppManifest } from '../../services/appManifestService';

type CategoryId = 'apps' | 'featured' | 'opensource';

interface LinkCard {
    title: string;
    descriptionKey: string;
    /** When set, used as description text instead of i18n key (manifest apps). */
    descriptionText?: string;
    badgeKey?: string;
    url: string;
    iconColor?: string;
    category: CategoryId;
    appId?: string;
    /** When false, card is shown but not launchable (disabled app). */
    enabled?: boolean;
}

const CATEGORIES: { id: CategoryId; titleKey: string }[] = [
    { id: 'apps', titleKey: 'discovery.catApps' },
    { id: 'featured', titleKey: 'discovery.catFeatured' },
    { id: 'opensource', titleKey: 'discovery.catOpensource' },
];

/**
 * Static discovery cards.
 * Agents 圆桌 is listed under「应用」so the path 更多 → 发现中心 → 应用 always
 * shows the card even before / while manifests load; launch still goes through
 * onOpenApp(appId) → openAppById (enable check via /api/apps).
 */
const STATIC_LINKS: LinkCard[] = [
    {
        title: 'Agents 圆桌',
        descriptionKey: 'discovery.roundtableDesc',
        badgeKey: 'discovery.appBadge',
        url: '#app/agents-roundtable',
        iconColor: '#2563eb',
        category: 'apps',
        appId: 'agents-roundtable',
        enabled: true,
    },
    {
        title: 'NanoSkill.ai',
        descriptionKey: 'discovery.nanoDesc',
        badgeKey: 'discovery.popular',
        url: 'http://nanoskill.ai/',
        iconColor: '#4f46e5',
        category: 'featured',
    },
    {
        title: '1gateway',
        descriptionKey: 'discovery.gatewayDesc',
        badgeKey: 'discovery.opensourceBadge',
        url: 'https://github.com/scottzx/1gateway',
        iconColor: '#16a34a',
        category: 'opensource',
    },
];

/** Known in-app descriptions (i18n keys) keyed by AppManifest.id. */
const APP_DESC_KEYS: Record<string, string> = {
    'agents-roundtable': 'discovery.roundtableDesc',
};

const APP_ICON_COLORS: Record<string, string> = {
    'agents-roundtable': '#2563eb',
};

function manifestToCard(app: AppManifest): LinkCard | null {
    const hasL1 = app.mountPoints.some(m => m.type === 'l1-page');
    if (!hasL1) return null;
    return {
        title: app.name,
        descriptionKey: APP_DESC_KEYS[app.id] || '',
        descriptionText: APP_DESC_KEYS[app.id] ? undefined : app.name,
        badgeKey: app.enabled ? 'discovery.appBadge' : 'discovery.appDisabledBadge',
        url: `#app/${app.id}`,
        iconColor: APP_ICON_COLORS[app.id] || 'var(--accent-color)',
        category: 'apps',
        appId: app.id,
        enabled: app.enabled,
    };
}

interface DiscoveryPanelProps {
    onOpenBrowserTab?: (url: string) => void;
    onOpenApp?: (appId: string) => void;
    language: Lang;
    /** When set, smoothly scroll the matching category section into view. */
    scrollToCategory?: string;
    /**
     * When set (desktop top-tab layout), render ONLY this category's section and
     * hide its title (the tab already labels it). Unset = the full scroll page
     * with all sections (mobile / legacy).
     */
    activeCategory?: string;
}

export function DiscoveryPanel({
    onOpenBrowserTab,
    onOpenApp,
    language,
    scrollToCategory,
    activeCategory,
}: DiscoveryPanelProps) {
    const containerRef = useRef<HTMLDivElement>(null);

    // Subscribe to signals during render so Preact re-renders on load.
    const manifests = appStore.appManifests.value;
    const loadingApps = appStore.appsLoading.value;

    // Ensure manifests are loaded when discovery opens (idempotent).
    useEffect(() => {
        if (manifests.length === 0 && !loadingApps) {
            void appStore.loadApps();
        }
    }, [manifests.length, loadingApps]);

    // Prefer enabled apps; still list disabled with badge so enable state is visible.
    // Dedupe by appId so static Agents 圆桌 card is not doubled when manifest loads.
    const fromManifest = manifests.map(manifestToCard).filter((c): c is LinkCard => c !== null);
    fromManifest.sort((a, b) => Number(b.enabled !== false) - Number(a.enabled !== false));
    const staticAppIds = new Set(STATIC_LINKS.map(c => c.appId).filter(Boolean));
    const cards = [
        ...STATIC_LINKS.map(card => {
            if (!card.appId) return card;
            const m = fromManifest.find(x => x.appId === card.appId);
            // Overlay enable state from live manifest when available.
            return m ? { ...card, enabled: m.enabled, title: m.title || card.title } : card;
        }),
        ...fromManifest.filter(c => !c.appId || !staticAppIds.has(c.appId)),
    ];

    useEffect(() => {
        if (!scrollToCategory || !containerRef.current) return;
        const section = containerRef.current.querySelector(`#discovery-section-${scrollToCategory}`);
        if (section) section.scrollIntoView({ behavior: 'smooth', block: 'start' });
    }, [scrollToCategory]);

    const renderCard = (card: LinkCard, idx: number) => {
        const desc =
            card.descriptionKey && !card.descriptionText
                ? t(card.descriptionKey, language)
                : card.descriptionText || '';
        const disabled = card.appId && card.enabled === false;
        return (
            <a
                key={`${card.appId || card.url}-${idx}`}
                href={card.url}
                target={card.appId ? undefined : '_blank'}
                rel="noopener noreferrer"
                class={`discovery-card${disabled ? ' is-disabled' : ''}`}
                aria-disabled={disabled ? 'true' : undefined}
                onClick={e => {
                    if (card.appId) {
                        e.preventDefault();
                        if (disabled) return;
                        if (onOpenApp) onOpenApp(card.appId);
                    } else if (onOpenBrowserTab) {
                        e.preventDefault();
                        onOpenBrowserTab(card.url);
                    }
                }}
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
                    <p class="bento-card-desc">{desc}</p>
                </div>

                <div class="bento-zone-footer">
                    <span class="card-action-text">
                        {disabled
                            ? t('discovery.appDisabled', language)
                            : card.appId
                              ? t('discovery.launchApp', language)
                              : t('discovery.exploreNow', language)}
                    </span>
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
    };

    return (
        <div class="discovery-container" ref={containerRef}>
            <div class="discovery-main">
                <div class="discovery-header-desc">{t('discovery.intro', language)}</div>

                {CATEGORIES.filter(cat => !activeCategory || cat.id === activeCategory).map(cat => {
                    const sectionCards = cards.filter(c => c.category === cat.id);
                    if (sectionCards.length === 0) {
                        // Always show apps section shell so empty/loading is visible.
                        if (cat.id !== 'apps') return null;
                        return (
                            <section class="discovery-section" id={`discovery-section-${cat.id}`} key={cat.id}>
                                {!activeCategory && (
                                    <h2 class="discovery-section-title">{t(cat.titleKey, language)}</h2>
                                )}
                                <div class="discovery-empty">
                                    {loadingApps
                                        ? t('discovery.appsLoading', language)
                                        : t('discovery.appsEmpty', language)}
                                </div>
                            </section>
                        );
                    }
                    return (
                        <section class="discovery-section" id={`discovery-section-${cat.id}`} key={cat.id}>
                            {!activeCategory && <h2 class="discovery-section-title">{t(cat.titleKey, language)}</h2>}
                            <div class="discovery-cards-list bento-grid">{sectionCards.map(renderCard)}</div>
                        </section>
                    );
                })}
            </div>
        </div>
    );
}
