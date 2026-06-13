/**
 * ModuleNav — renders a module's manifest inside the host's LeftSidebar.
 *
 * The host's existing `LeftSidebar` keeps its workspace tree and footer.
 * When a module-backed drawer tab is active, the host passes a `moduleNav`
 * prop down and we render its `topLinks` + `groups` between the workspace
 * section and the footer, sharing the same visual idiom (no parallel UI).
 */

import { h } from 'preact';
import { useSignal } from '@preact/signals';

import type { Lang } from '../i18n';
import { t } from '../i18n';
import { getModuleIconPath } from '../../modules/icon-registry';
import type { ModuleManifest, ModuleNavGroup, ModuleNavLink } from '../../modules/module-types';

export interface ModuleNavProps {
    manifest: ModuleManifest;
    activePath: string;
    language: Lang;
    onNavigate: (to: string) => void;
}

/**
 * Returns true if the active path matches the link. Handles both exact
 * matches ("/skills/use") and root matches ("/skills") so a parent group
 * can highlight when a child is active.
 */
function isPathActive(linkTo: string, activePath: string): boolean {
    if (!activePath) return false;
    if (activePath === linkTo) return true;
    // "/skills" should match "/skills/use" but NOT "/skills-other"
    if (linkTo !== '/' && activePath.startsWith(linkTo + '/')) return true;
    return false;
}

export function ModuleNav({ manifest, activePath, language, onNavigate }: ModuleNavProps) {
    return (
        <div class="sidebar-module-nav">
            {manifest.topLinks && manifest.topLinks.length > 0 && (
                <div class="module-nav-section">
                    {manifest.topLinks.map(link => (
                        <ModuleLink
                            key={link.key}
                            link={link}
                            active={isPathActive(link.to, activePath)}
                            onClick={() => onNavigate(link.to)}
                            language={language}
                        />
                    ))}
                </div>
            )}
            {manifest.groups.map(group => (
                <ModuleGroup
                    key={group.key}
                    group={group}
                    activePath={activePath}
                    defaultCollapsed={!!group.defaultCollapsed}
                    onNavigate={onNavigate}
                    language={language}
                />
            ))}
        </div>
    );
}

interface ModuleGroupProps {
    group: ModuleNavGroup;
    activePath: string;
    defaultCollapsed: boolean;
    onNavigate: (to: string) => void;
    language: Lang;
}

function ModuleGroup({ group, activePath, defaultCollapsed, onNavigate, language }: ModuleGroupProps) {
    const collapsed = useSignal(defaultCollapsed);
    const collapsible = group.collapsible !== false;
    const groupIcon = getModuleIconPath(group.iconKey);
    const groupActive = group.links.some(l => isPathActive(l.to, activePath));

    return (
        <div class={`module-nav-group${groupActive ? ' has-active' : ''}`} data-collapsed={collapsed.value}>
            <button
                type="button"
                class="module-nav-group__header"
                onClick={collapsible ? () => (collapsed.value = !collapsed.value) : undefined}
                aria-expanded={!collapsed.value}
            >
                {groupIcon && (
                    <svg
                        class="module-nav-icon"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        // The registry returns a pre-built inner-SVG string; we
                        // inject it via dangerouslySetInnerHTML below for the
                        // path data.
                        dangerouslySetInnerHTML={{ __html: groupIcon }}
                    />
                )}
                <span class="module-nav-group__label">{t(group.label, language)}</span>
                {group.count !== null && group.count !== undefined && (
                    <span class="module-nav-group__count">{group.count}</span>
                )}
                {collapsible && (
                    <svg
                        class="module-nav-chevron"
                        viewBox="0 0 24 24"
                        fill="none"
                        stroke="currentColor"
                        stroke-width="2.5"
                        stroke-linecap="round"
                        stroke-linejoin="round"
                    >
                        <polyline points="6 9 12 15 18 9" />
                    </svg>
                )}
            </button>
            {!collapsed.value && (
                <div class="module-nav-group__items">
                    {group.links.map(link => (
                        <ModuleLink
                            key={link.key}
                            link={link}
                            active={isPathActive(link.to, activePath)}
                            onClick={() => onNavigate(link.to)}
                            language={language}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}

interface ModuleLinkProps {
    link: ModuleNavLink;
    active: boolean;
    onClick: () => void;
    language: Lang;
}

function ModuleLink({ link, active, onClick, language }: ModuleLinkProps) {
    // Labels are i18n keys (e.g. 'module.skills.link.inUse'). The `t()` helper
    // falls back to the key string if missing — which still beats showing
    // nothing — and logs a console.warn so the missing key is visible.
    const display = t(link.label, language);
    return (
        <button type="button" class={`module-nav-link${active ? ' is-active' : ''}`} onClick={onClick} title={display}>
            <span class="module-nav-link__dot" aria-hidden="true" />
            <span class="module-nav-link__label">{display}</span>
            {link.count !== null && link.count !== undefined && (
                <span class="module-nav-link__count">{link.count}</span>
            )}
            {link.badge === 'review' && <span class="module-nav-link__badge">!</span>}
        </button>
    );
}
