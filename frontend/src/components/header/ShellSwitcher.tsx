import { h, Fragment } from 'preact';
import { useSignal } from '@preact/signals';
import { t, type Lang } from '../../i18n';
import * as shellStore from '../../stores/productShellStore';
import * as stage from '../../stores/stageStore';
import { ShellIcon } from '../settings/ProductShellPanel';

interface ShellSwitcherProps {
    language: Lang;
}

/**
 * Global Product Shell switcher (#328) — the desktop header entry point.
 *
 * Lists the shells the tenant has enabled and switches the active one via
 * `stage.switchShell`, which is a pure navigation change: the Workspace,
 * running sessions and the right artifact column are preserved, only the
 * shell composition (which mount points render) recomputes.
 *
 * Graceful edges:
 *   - no shells loaded yet (legacy mode / API unreachable) → render nothing;
 *   - exactly one enabled shell (e.g. Personal as the only enabled shell) →
 *     a static pill, because there is nothing to switch to.
 */
export function ShellSwitcher({ language }: ShellSwitcherProps) {
    const open = useSignal(false);
    const shells = shellStore.enabledShells.value;
    const activeId = shellStore.activeShellId.value;
    const active = shells.find(s => s.id === activeId) ?? shells[0];

    // Nothing loaded yet → stay out of the way (legacy single-shell rendering).
    if (shells.length === 0 || !active) return null;

    const close = () => {
        open.value = false;
    };
    const pick = (id: string) => {
        close();
        if (id !== activeId) stage.switchShell(id);
    };

    const Chevron = (
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
    );

    // Single enabled shell → static pill, nothing to switch.
    if (shells.length === 1) {
        return (
            <div class="shell-switcher shell-switcher-single" title={active.description || active.name}>
                <span class="shell-switcher-icon">
                    <ShellIcon id={active.id} />
                </span>
                <span class="shell-switcher-name">{active.name}</span>
            </div>
        );
    }

    return (
        <div class="shell-switcher">
            <button
                type="button"
                class={`shell-switcher-btn${open.value ? ' open' : ''}`}
                onClick={() => (open.value = !open.value)}
                title={t('header.shellSwitcher', language)}
                aria-label={t('header.shellSwitcher', language)}
                aria-haspopup="menu"
                aria-expanded={open.value}
            >
                <span class="shell-switcher-icon">
                    <ShellIcon id={active.id} />
                </span>
                <span class="shell-switcher-name">{active.name}</span>
                {Chevron}
            </button>

            {open.value && (
                <Fragment>
                    <div class="shell-switcher-backdrop" onClick={close} />
                    <div class="shell-switcher-menu" role="menu" aria-label={t('header.shellSwitcher', language)}>
                        {shells.map(s => (
                            <button
                                type="button"
                                key={s.id}
                                role="menuitem"
                                class={`shell-switcher-item${s.id === activeId ? ' active' : ''}`}
                                onClick={() => pick(s.id)}
                                title={s.description || s.name}
                            >
                                <span class="shell-switcher-icon">
                                    <ShellIcon id={s.id} />
                                </span>
                                <span class="shell-switcher-item-name">{s.name}</span>
                                {s.id === activeId && (
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
