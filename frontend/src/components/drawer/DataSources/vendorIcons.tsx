import { h } from 'preact';

// Line-art vendor marks (Feather-style: stroke=currentColor, no fill) — used
// instead of pictorial emoji so icon color/weight stays consistent with the
// rest of the app's icon system and adapts to theme via currentColor.

const PATHS: Record<string, string> = {
    icloud: '<path d="M12 6.2c-1.3 0-2.4.7-3.1 1.8-1.6-.2-3.1.6-3.7 2.1-1.6.3-2.7 1.8-2.5 3.4.2 1.6 1.6 2.7 3.2 2.5H16.6c1.7 0 3.1-1.3 3.2-3 .1-1.6-1-3-2.5-3.3-.3-1.9-2-3.3-3.9-3.3-.5 0-1 .1-1.4.3z"/><path d="M12.9 4.4c.3-.5.4-1.1.3-1.7-.6.1-1.1.4-1.5.9-.3.4-.5 1-.4 1.6.6 0 1.2-.3 1.6-.8z"/>',
    feishu: '<path d="M21 11.5a8.38 8.38 0 0 1-.9 3.8 8.5 8.5 0 0 1-7.6 4.7 8.38 8.38 0 0 1-3.8-.9L3 21l1.9-5.7a8.38 8.38 0 0 1-.9-3.8 8.5 8.5 0 0 1 4.7-7.6 8.38 8.38 0 0 1 3.8-.9h.5a8.48 8.48 0 0 1 8 8v.5z"/>',
    microsoft:
        '<rect x="3" y="3" width="7.5" height="7.5"/><rect x="13.5" y="3" width="7.5" height="7.5"/><rect x="3" y="13.5" width="7.5" height="7.5"/><rect x="13.5" y="13.5" width="7.5" height="7.5"/>',
    google: '<circle cx="12" cy="12" r="10"/><line x1="2" y1="12" x2="22" y2="12"/><path d="M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z"/>',
    agentmail: '<rect x="3" y="5" width="18" height="14" rx="2"/><path d="m3 7 9 6 9-6"/>',
    default:
        '<path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/>',
};

export function VendorIcon({ vendor, class: className }: { vendor: string; class?: string }) {
    const paths = PATHS[vendor] ?? PATHS.default;
    return (
        <svg
            class={className}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
            aria-hidden="true"
            dangerouslySetInnerHTML={{ __html: paths }}
        />
    );
}
