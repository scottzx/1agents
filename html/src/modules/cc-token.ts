/**
 * Extract the management token and redirect path from a cc-connect URL.
 *
 * `1agents/backend` produces URLs of the form
 * `/cc-connect/login?token=<ManagementToken>&redirect=<path>&theme=...&lang=...`
 * via the `/api/cc-connect/url` endpoint. The `redirect` param tells
 * cc-connect which route to boot at — typically `/projects/<name>` for
 * workspace-scoped channels, or `/chat/<channel>` for a named channel.
 *
 * The custom element path needs both: the token to auto-login before the
 * React tree mounts, and the redirect path as the initial MemoryRouter entry.
 *
 * Pure functions — no DOM, no module state. Safe to call in render.
 */

export function extractCcToken(url: string | null | undefined): string {
    if (!url) return '';
    try {
        return new URL(url, window.location.origin).searchParams.get('token') || '';
    } catch {
        return '';
    }
}

export function extractCcRedirect(url: string | null | undefined, fallback = '/projects'): string {
    if (!url) return fallback;
    try {
        const redirect = new URL(url, window.location.origin).searchParams.get('redirect');
        return redirect || fallback;
    } catch {
        return fallback;
    }
}
