/* eslint-env serviceworker */
self.addEventListener('install', () => {
    self.skipWaiting();
});

self.addEventListener('activate', event => {
    event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', event => {
    if (event.request.method !== 'GET') return;
    if (
        event.request.url.includes('/ws') ||
        event.request.url.includes('/token') ||
        event.request.url.includes('/cc-connect') ||
        // HarnessKit embed bundle + custom element paths. Without these
        // excludes the SW intercepts the embed scripts, then re-serves
        // a stale cache entry after the hash changes — turning "iframe
        // load fails" into "module registration fails".
        event.request.url.includes('/extensions/') ||
        event.request.url.includes('/api/embed/')
    )
        return;

    event.respondWith(
        fetch(event.request).catch(async () => {
            const cached = await caches.match(event.request);
            // caches.match returns undefined on miss; respondWith requires a
            // Response or the SW throws "Failed to convert value to 'Response'".
            if (cached) return cached;
            return new Response('Network error', {
                status: 503,
                statusText: 'Service Unavailable',
                headers: { 'Content-Type': 'text/plain; charset=utf-8' },
            });
        })
    );
});
