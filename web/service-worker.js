// Conflux service worker — network-first so updates (and admin branding
// changes in /manifest.json) are always picked up when online, with an
// offline fallback from cache. Bump CACHE_NAME to force-purge old caches.
const CACHE_NAME = 'conflux-v2';
const PRECACHE = [
  '/',
  '/index.html',
  '/css/style.css',
  '/js/api.js',
  '/js/app.js',
  '/js/ui.js',
];

// Install: pre-cache the shell, then activate immediately.
self.addEventListener('install', event => {
  event.waitUntil(
    caches.open(CACHE_NAME)
      .then(cache => cache.addAll(PRECACHE))
      .then(() => self.skipWaiting())
      .catch(() => self.skipWaiting())
  );
});

// Activate: drop any old caches and take control of open pages.
self.addEventListener('activate', event => {
  event.waitUntil(
    caches.keys()
      .then(names => Promise.all(
        names.filter(name => name !== CACHE_NAME).map(name => caches.delete(name))
      ))
      .then(() => self.clients.claim())
  );
});

// Fetch: network-first for same-origin GETs; never touch API calls.
self.addEventListener('fetch', event => {
  const req = event.request;
  if (req.method !== 'GET') return;

  const url = new URL(req.url);
  // Let API traffic go straight to the network (never cached).
  if (url.origin === self.location.origin && url.pathname.startsWith('/api/')) return;
  // Only manage same-origin assets/navigations.
  if (url.origin !== self.location.origin) return;

  event.respondWith(
    fetch(req)
      .then(res => {
        if (res && res.ok && res.type === 'basic') {
          const copy = res.clone();
          caches.open(CACHE_NAME).then(cache => cache.put(req, copy)).catch(() => {});
        }
        return res;
      })
      .catch(() => caches.match(req).then(hit => hit || caches.match('/index.html')))
  );
});
