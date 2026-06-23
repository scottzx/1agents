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
        // 1skills embed bundle + custom element paths. Without these
        // excludes the SW intercepts the embed scripts, then re-serves
        // a stale cache entry after the hash changes — turning "iframe
        // load fails" into "module registration fails".
        event.request.url.includes('/1skills') ||
        event.request.url.includes('/api/embed/')
    )
        return;

    event.respondWith(fetch(event.request).catch(() => caches.match(event.request)));
});
