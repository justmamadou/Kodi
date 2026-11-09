// Service worker de Kodi. Rôle volontairement limité :
//  - l'interface reste disponible sans réseau (pages clés pré-chargées, ressources statiques en cache) ;
//  - l'API n'est JAMAIS mise en cache ici : les données hors connexion vivent dans IndexedDB,
//    avec leurs états d'envoi, et une décision financière exige toujours le réseau.
const VERSION = "v3";
const PAGES = `cf-pages-${VERSION}`;
const STATIC = `cf-static-${VERSION}`;
const OFFLINE_ROUTES = ["/", "/hors-ligne", "/ajouter", "/alertes", "/profil", "/materiaux", "/rapports/nouveau", "/depenses/nouvelle", "/synchro", "/chantiers"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    (async () => {
      const pages = await caches.open(PAGES);
      const statics = await caches.open(STATIC);
      await Promise.all(
        OFFLINE_ROUTES.map(async (route) => {
          // Une page manquante ne doit pas empêcher l'installation : elle sera mise en cache à la visite.
          try {
            const response = await fetch(route, { credentials: "same-origin" });
            if (!response.ok) return;
            await pages.put(route, response.clone());
            const html = await response.text();
            const assets = [...html.matchAll(/\/_next\/static\/[^"'\\\s)]+/g)].map((m) => m[0]);
            await Promise.allSettled([...new Set(assets)].map((asset) => statics.add(asset)));
          } catch {
            /* réseau instable pendant l'installation */
          }
        }),
      );
      await self.skipWaiting();
    })(),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const keep = [PAGES, STATIC];
      const names = await caches.keys();
      await Promise.all(names.filter((n) => n.startsWith("cf-") && !keep.includes(n)).map((n) => caches.delete(n)));
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const { request } = event;
  if (request.method !== "GET") return;
  const url = new URL(request.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;

  // Ressources versionnées par leur nom : le cache d'abord, elles ne changent jamais.
  if (url.pathname.startsWith("/_next/static/") || url.pathname.startsWith("/icons/")) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(request);
        if (cached) return cached;
        const response = await fetch(request);
        if (response.ok) (await caches.open(STATIC)).put(request, response.clone());
        return response;
      })(),
    );
    return;
  }

  // Pages et données de rendu : le réseau d'abord pour rester à jour, le cache sans réseau.
  if (request.mode === "navigate" || url.searchParams.has("_rsc")) {
    event.respondWith(
      (async () => {
        try {
          const response = await fetch(request);
          if (response.ok && request.mode === "navigate") (await caches.open(PAGES)).put(url.pathname, response.clone());
          return response;
        } catch {
          const pages = await caches.open(PAGES);
          const cached = (await pages.match(url.pathname)) ?? (await pages.match("/hors-ligne"));
          return cached ?? new Response("Hors connexion", { status: 503, headers: { "Content-Type": "text/plain; charset=utf-8" } });
        }
      })(),
    );
  }
});
