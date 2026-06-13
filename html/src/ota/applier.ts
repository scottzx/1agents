// Over-the-air update applier.
//
// V1 strategy: hard reload with a cache-busting query string. The
// webpack contenthash in the asset filenames already invalidates JS
// chunks; we add the new version as ?v=... so the HTML itself is
// refetched by the browser.
//
// Future V2 can do chunk-level prefetch + soft-navigate without reload,
// but that requires a much larger change to the bootstrap flow.

import { dismiss } from './checker';
import type { RootManifest } from './checker';

/**
 * Apply an OTA update by reloading the page from the new entry URL.
 * Marks the version as dismissed before navigating so a reload mid-update
 * doesn't re-prompt the same release.
 */
export function apply(manifest: RootManifest): void {
    const entry = manifest.components?.frontend?.entry;
    const version = manifest.components?.frontend?.version;
    if (version) dismiss(version);

    if (entry) {
        // The "entry" is currently a tarball URL, not a navigable page.
        // For V1 we just reload the current URL with a version query so
        // the browser fetches the (versioned) HTML.
        const url = new URL(window.location.href);
        url.searchParams.set('v', version || Date.now().toString());
        window.location.replace(url.toString());
    } else {
        window.location.reload();
    }
}

/**
 * Best-effort prefetch: warm the browser cache for the versioned
 * assets referenced in the manifest. Currently a no-op until V2 lands,
 * kept as the public API surface that components can call.
 */
export async function prefetch(_manifest: RootManifest): Promise<void> {
    // Intentionally empty in V1 — the hard-reload strategy invalidates
    // caches on its own. Implement chunk-level prefetch here in V2.
    void _manifest;
    return;
}
