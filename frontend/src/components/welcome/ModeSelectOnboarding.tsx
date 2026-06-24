import { h, Component } from 'preact';
import { t, type Lang } from '../i18n';

interface ModeSelectOnboardingProps {
    language: Lang;
    onSelect: (mode: 'beginner' | 'advanced') => void;
}

/**
 * First-launch mode picker. Shown whenever the persisted UI mode is unset
 * (`uiStore.uiMode === null`), independent of whether any workspace exists.
 * Two large side-by-side cards — beginner (blue / --accent) and advanced
 * (red / --danger) — each with a large image placeholder, title and a short
 * description. Picking one persists the mode via the host's onSelect.
 */
export class ModeSelectOnboarding extends Component<ModeSelectOnboardingProps> {
    render() {
        const { language, onSelect } = this.props;

        return (
            <div
                class="modeselect-container"
                style="display: flex; flex-direction: column; align-items: center; justify-content: center; min-height: 100vh; width: 100vw; background-color: var(--bg-page); color: var(--text-main); font-family: var(--font-sans); padding: 40px; box-sizing: border-box; text-align: center;"
            >
                <style>{`
                    .modeselect-card {
                        transition: transform 0.2s, box-shadow 0.2s, border-color 0.2s;
                        cursor: pointer;
                    }
                    .modeselect-card:hover {
                        transform: translateY(-4px);
                        box-shadow: var(--shadow-lg);
                    }
                    .modeselect-card.beginner:hover { border-color: var(--accent-color); }
                    .modeselect-card.advanced:hover { border-color: var(--danger-fg); }
                    .modeselect-grid {
                        display: grid;
                        grid-template-columns: 1fr 1fr;
                        gap: 24px;
                        width: 100%;
                        max-width: 760px;
                    }
                    @media (max-width: 640px) {
                        .modeselect-grid { grid-template-columns: 1fr; }
                    }
                `}</style>

                <h1 style="font-size: 28px; font-weight: 700; margin: 0 0 8px 0;">{t('modeselect.title', language)}</h1>
                <p style="font-size: 15px; color: var(--text-secondary); line-height: 1.6; margin: 0 0 36px 0; max-width: 480px;">
                    {t('modeselect.subtitle', language)}
                </p>

                <div class="modeselect-grid">
                    {/* Beginner — blue */}
                    <div
                        class="modeselect-card beginner"
                        onClick={() => onSelect('beginner')}
                        style="background-color: var(--bg-panel); border: 2px solid var(--border-color); border-radius: 16px; padding: 28px; box-sizing: border-box; box-shadow: var(--shadow-md); display: flex; flex-direction: column; align-items: center;"
                    >
                        <div
                            class="modeselect-art"
                            style="width: 100%; aspect-ratio: 1 / 1; border-radius: 12px; margin-bottom: 20px; background: var(--bg-page); display: flex; align-items: center; justify-content: center; overflow: hidden;"
                        >
                            <img
                                src="/mode-beginner.png"
                                alt={t('modeselect.beginnerTitle', language)}
                                style="width: 100%; height: 100%; object-fit: contain;"
                            />
                        </div>
                        <h2 style="font-size: 20px; font-weight: 700; margin: 0 0 8px 0; color: var(--accent-color);">
                            {t('modeselect.beginnerTitle', language)}
                        </h2>
                        <p style="font-size: 14px; color: var(--text-secondary); line-height: 1.5; margin: 0;">
                            {t('modeselect.beginnerDesc', language)}
                        </p>
                    </div>

                    {/* Advanced — red */}
                    <div
                        class="modeselect-card advanced"
                        onClick={() => onSelect('advanced')}
                        style="background-color: var(--bg-panel); border: 2px solid var(--border-color); border-radius: 16px; padding: 28px; box-sizing: border-box; box-shadow: var(--shadow-md); display: flex; flex-direction: column; align-items: center;"
                    >
                        <div
                            class="modeselect-art"
                            style="width: 100%; aspect-ratio: 1 / 1; border-radius: 12px; margin-bottom: 20px; background: var(--bg-page); display: flex; align-items: center; justify-content: center; overflow: hidden;"
                        >
                            <img
                                src="/mode-advanced.jpeg"
                                alt={t('modeselect.advancedTitle', language)}
                                style="width: 100%; height: 100%; object-fit: contain;"
                            />
                        </div>
                        <h2 style="font-size: 20px; font-weight: 700; margin: 0 0 8px 0; color: var(--danger-fg);">
                            {t('modeselect.advancedTitle', language)}
                        </h2>
                        <p style="font-size: 14px; color: var(--text-secondary); line-height: 1.5; margin: 0;">
                            {t('modeselect.advancedDesc', language)}
                        </p>
                    </div>
                </div>

                <p style="font-size: 13px; color: var(--text-muted); margin: 28px 0 0 0;">
                    {t('modeselect.switchHint', language)}
                </p>
            </div>
        );
    }
}
