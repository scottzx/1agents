export type RoundtableBreadcrumbView = 'list' | 'create' | 'room' | 'session';

export interface RoundtableCrumb {
    label: string;
    onClick?: () => void;
}

interface RoundtableBreadcrumbOptions {
    view: RoundtableBreadcrumbView;
    roomTitle?: string;
    sessionTitle?: string;
    onList?: () => void;
    onRoom?: () => void;
}

/** Build only the path trail; the global header renders back as a separate icon. */
export function roundtableBreadcrumbs({
    view,
    roomTitle,
    sessionTitle,
    onList,
    onRoom,
}: RoundtableBreadcrumbOptions): RoundtableCrumb[] {
    if (view === 'list') {
        return [{ label: '圆桌列表' }];
    }

    const crumbs: RoundtableCrumb[] = [{ label: '圆桌列表', onClick: onList }];
    if (view === 'create') {
        crumbs.push({ label: '新建圆桌' });
        return crumbs;
    }

    const title = roomTitle?.trim() || '圆桌';
    if (view === 'room') {
        crumbs.push({ label: title });
        return crumbs;
    }

    crumbs.push({ label: title, onClick: onRoom });
    crumbs.push({ label: sessionTitle?.trim() || '会话' });
    return crumbs;
}
