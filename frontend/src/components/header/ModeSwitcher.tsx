import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { t, type Lang } from '../../i18n';
import { CHROME_MODE_LABELS, CHROME_MODE_OPTIONS, type ChromeMode } from '../../stores/chromeMode';
import { chromeMode } from '../../stores/chromeModeStore';
import { switchToChromeMode } from '../../stores/chromeModeActions';

interface ModeSwitcherProps {
    language: Lang;
}

/**
 * Chrome dropdown for 工作台 vs 聊天. Separate from Product Shell id and
 * from beginner/advanced uiMode.
 */
export function ModeSwitcher({ language }: ModeSwitcherProps) {
    const open = useSignal(false);
    const mode = chromeMode.value;
    const label = CHROME_MODE_LABELS[mode];
    const aria = t('header.modeSwitcher', language);

    const close = () => {
        open.value = false;
    };
    const pick = (next: ChromeMode) => {
        close();
        if (next !== mode) switchToChromeMode(next);
    };

    return (
        <div class="shell-switcher chrome-mode-switcher" data-testid="chrome-mode-switcher">
            <button
                type="button"
                class={`shell-switcher-btn${open.value ? ' open' : ''}`}
                onClick={() => (open.value = !open.value)}
                title={aria}
                aria-label={aria}
                aria-haspopup="menu"
                aria-expanded={open.value}
                data-chrome-mode={mode}
            >
                <span class="shell-switcher-name">{label}</span>
                <svg
                    class="shell-switcher-caret"
                    viewBox="0 0 24 24"
                    fill="none"
                    stroke="currentColor"
                    stroke-width="2.5"
                    stroke-linecap="round"
                    stroke-linejoin="round"
                >
                    <polyline points="6 9 12 15 18 9" />
                </svg>
            </button>

            {open.value && (
                <Fragment>
                    <div class="shell-switcher-backdrop" onClick={close} />
                    <div class="shell-switcher-menu" role="menu" aria-label={aria}>
                        {CHROME_MODE_OPTIONS.map(item => (
                            <button
                                type="button"
                                key={item.id}
                                role="menuitem"
                                class={`shell-switcher-item${item.id === mode ? ' active' : ''}`}
                                data-chrome-mode-option={item.id}
                                onClick={() => pick(item.id)}
                            >
                                <span class="shell-switcher-item-name">{item.name}</span>
                                {item.id === mode && (
                                    <span class="shell-switcher-check">
                                        <svg
                                            viewBox="0 0 24 24"
                                            fill="none"
                                            stroke="currentColor"
                                            stroke-width="2.5"
                                            stroke-linecap="round"
                                            stroke-linejoin="round"
                                        >
                                            <polyline points="20 6 9 17 4 12" />
                                        </svg>
                                    </span>
                                )}
                            </button>
                        ))}
                    </div>
                </Fragment>
            )}
        </div>
    );
}
