const CACHE_NAME = 'terminal-v1';

self.addEventListener('install', () => {
    self.skipWaiting();
});

self.addEventListener('activate', event => {
    event.waitUntil(self.clients.claim());
});

self.addEventListener('fetch', event => {
    if (event.request.method !== 'GET') return;
    if (event.request.url.includes('/ws') || event.request.url.includes('/token')) return;

    event.respondWith(fetch(event.request).catch(() => caches.match(event.request)));
});
