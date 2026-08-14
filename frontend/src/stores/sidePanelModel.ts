export const PINNED_OVERVIEW_TYPE = 'background' as const;

export function isPinnedSidePanelTab(tab: { type: string }): boolean {
    return tab.type === PINNED_OVERVIEW_TYPE;
}

/** Keep the pinned Overview tab first; drop any extra copies of the same type. */
export function withPinnedOverview<T extends { type: string }>(tabs: T[], overview: T): T[] {
    return [overview, ...tabs.filter(tab => !isPinnedSidePanelTab(tab))];
}

export function resolveSidePanelActiveId<T extends { id: string; type: string }>(
    tabs: T[],
    activeId: string | null
): string | null {
    if (activeId && tabs.some(tab => tab.id === activeId)) return activeId;
    return tabs.find(isPinnedSidePanelTab)?.id ?? tabs[0]?.id ?? null;
}
