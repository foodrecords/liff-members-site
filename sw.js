const CACHE_NAME = 'agaruke-v8e8da87';
const ASSETS = [
    '/',
    '/index.html',
    '/index.css',
    '/liff.js',
    '/config.js',
    '/foodrecords_logo.png',
    '/images/agaruke_icon.svg',
    '/images/agaruke_icon.png',
    '/images/ticket-a.png',
    '/images/ticket-b.png',
    '/images/ticket-c.png',
    '/images/ticket-d.png',
];

self.addEventListener('install', event => {
    event.waitUntil(
        caches.open(CACHE_NAME).then(cache => cache.addAll(ASSETS))
    );
    self.skipWaiting();
});

self.addEventListener('activate', event => {
    event.waitUntil(
        caches.keys().then(keys =>
            Promise.all(keys.filter(k => k !== CACHE_NAME).map(k => caches.delete(k)))
        ).then(() => self.clients.claim())
    );
});

self.addEventListener('fetch', event => {
    // LINE API・members-API へのリクエストはキャッシュしない
    const url = new URL(event.request.url);
    if (url.origin !== self.location.origin) return;

    event.respondWith(
        caches.match(event.request).then(cached => {
            if (cached) return cached;
            return fetch(event.request).then(response => {
                if (!response || response.status !== 200 || response.type === 'opaque') {
                    return response;
                }
                const clone = response.clone();
                caches.open(CACHE_NAME).then(cache => cache.put(event.request, clone));
                return response;
            });
        })
    );
});
