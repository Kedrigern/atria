const STATIC_CACHE = "atria-static-v10";
const STATIC_ASSETS = [
    "/static/water.css?v=2",
    "/static/style.css?v=5",
    "/static/htmx.min.js?v=2",
    "/static/ebook.js?v=2",
    "/static/app.js?v=1",
    "/static/plugins.js?v=1",
    "/static/mathjax-config.js?v=1",
    "/static/cryptogram.js?v=4",
    "/static/tabulator.min.css",
    "/static/tabulator.min.js",
    "/static/atria.png",
    "/static/atria-text.png",
    "/static/atria-text-dark.png",
    "/static/icon-192.png",
    "/static/icon-512.png",
    "/static/offline.html",
];

self.addEventListener("install", (event) => {
    event.waitUntil(caches.open(STATIC_CACHE).then((cache) => cache.addAll(STATIC_ASSETS)));
    self.skipWaiting();
});

self.addEventListener("activate", (event) => {
    event.waitUntil(
        caches
            .keys()
            .then((keys) => Promise.all(keys.filter((key) => key !== STATIC_CACHE).map((key) => caches.delete(key))))
            .then(() => self.clients.claim()),
    );
});

self.addEventListener("fetch", (event) => {
    const { request } = event;
    if (request.method !== "GET") return;

    const url = new URL(request.url);
    if (url.origin !== self.location.origin) return;

    if (url.pathname.startsWith("/static/")) {
        event.respondWith(
            caches.match(request).then((cached) => {
                if (cached) return cached;
                return fetch(request).then((response) => {
                    if (response.ok) {
                        const copy = response.clone();
                        caches.open(STATIC_CACHE).then((cache) => cache.put(request, copy));
                    }
                    return response;
                });
            }),
        );
        return;
    }

    if (request.mode === "navigate") {
        event.respondWith(fetch(request).catch(() => caches.match("/static/offline.html")));
    }
});
