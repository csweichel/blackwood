// Blackwood offline-first service worker.
// Cache-first for static assets, network-first with cache fallback for API calls.

const CACHE_NAME = "blackwood-v1";

// App shell files to pre-cache on install.
// Vite hashes JS/CSS filenames, so we cache index.html and let fetch events
// handle the hashed bundles on first load.
const APP_SHELL = ["/", "/index.html"];

// --- Install: pre-cache the app shell ---
self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(APP_SHELL))
  );
  // Activate immediately without waiting for old tabs to close.
  self.skipWaiting();
});

// --- Activate: clean up old caches ---
self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(
        keys
          .filter((key) => key !== CACHE_NAME)
          .map((key) => caches.delete(key))
      )
    )
  );
  // Take control of all open clients immediately.
  self.clients.claim();
});

// --- Fetch strategy ---
self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Only handle same-origin requests.
  if (url.origin !== self.location.origin) return;

  // API calls (Connect-RPC): network-first, cache fallback for GET-like reads.
  if (
    url.pathname.startsWith("/blackwood.v1.") ||
    url.pathname.startsWith("/api/")
  ) {
    event.respondWith(networkFirstApi(request));
    return;
  }

  // Static assets: cache-first.
  event.respondWith(cacheFirstStatic(request));
});

// Network-first for API calls. On success, cache the response for offline use.
// On failure (offline), return the cached version if available.
async function networkFirstApi(request) {
  const cache = await caches.open(CACHE_NAME);
  try {
    const response = await fetch(request);
    // Cache successful GET/POST reads so they're available offline.
    if (response.ok) {
      cache.put(request, response.clone());
    }
    return response;
  } catch {
    const cached = await cache.match(request);
    if (cached) return cached;
    // No cache available — return a generic offline error.
    return new Response(
      JSON.stringify({ code: "unavailable", message: "Offline" }),
      {
        status: 503,
        headers: { "Content-Type": "application/json" },
      }
    );
  }
}

// Cache-first for static assets. On cache miss, fetch from network and cache.
async function cacheFirstStatic(request) {
  const cache = await caches.open(CACHE_NAME);
  const cached = await cache.match(request);
  if (cached) {
    // Update cache in the background for next visit.
    fetch(request)
      .then((response) => {
        if (response.ok) cache.put(request, response);
      })
      .catch(() => {});
    return cached;
  }
  try {
    const response = await fetch(request);
    if (response.ok) {
      cache.put(request, response.clone());
    }
    return response;
  } catch {
    // For navigation requests, fall back to cached index.html (SPA).
    if (request.mode === "navigate") {
      const fallback = await cache.match("/index.html");
      if (fallback) return fallback;
    }
    return new Response("Offline", { status: 503 });
  }
}
