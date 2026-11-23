// Service worker de FamilyCocon (le plus petit possible) :
//   - fichiers statiques (/statique/…?v=…) : gardés en cache, ils ne
//     changent jamais à adresse égale ;
//   - pages : toujours le réseau (données à jour) ; sans réseau, une page
//     « hors ligne » ;
//   - notifications push : affichées, et un toucher ouvre la bonne page.
"use strict";
// Le serveur remplace ce nom par la version des fichiers : à chaque mise à
// jour, un nouveau cache est rempli et l'ancien est supprimé (« activate »).
const CACHE = "familycocon-1";

self.addEventListener("install", function (e) {
  e.waitUntil(caches.open(CACHE).then(function (c) {
    return c.addAll(["/hors-ligne", "/statique/app.css", "/statique/img/logo.svg", "/statique/fonts/nunito.woff2"]);
  }));
  self.skipWaiting();
});

self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (cles) {
    return Promise.all(cles.filter(function (k) { return k !== CACHE; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

self.addEventListener("fetch", function (e) {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;
  if (url.pathname.startsWith("/statique/")) {
    e.respondWith(caches.match(req).then(function (r) {
      return r || fetch(req).then(function (rep) {
        if (rep.ok && url.search) {
          const copie = rep.clone();
          caches.open(CACHE).then(function (c) { c.put(req, copie); });
        }
        return rep;
      });
    }));
    return;
  }
  if (req.mode === "navigate") {
    e.respondWith(fetch(req).catch(function () { return caches.match("/hors-ligne"); }));
  }
});

self.addEventListener("push", function (e) {
  let d = {};
  try { d = e.data ? e.data.json() : {}; } catch (_) { d = { texte: e.data && e.data.text() }; }
  e.waitUntil(self.registration.showNotification(d.titre || "FamilyCocon", {
    body: d.texte || "",
    icon: "/statique/img/icone-192.png",
    badge: "/statique/img/icone-192.png",
    tag: d.groupe || "familycocon",
    renotify: true,
    data: { url: d.url || "/" }
  }));
});

self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  const cible = new URL((e.notification.data && e.notification.data.url) || "/", location.origin).href;
  e.waitUntil(self.clients.matchAll({ type: "window", includeUncontrolled: true }).then(function (fenetres) {
    // Une fenêtre déjà sur la bonne page : on la montre.
    for (const f of fenetres) {
      if (f.url === cible && "focus" in f) return f.focus();
    }
    // Sinon une fenêtre de l'appli suivie par ce service worker change de
    // page (navigate() échoue sur une fenêtre qu'il ne suit pas) ; à défaut,
    // une nouvelle fenêtre s'ouvre.
    for (const f of fenetres) {
      if ("navigate" in f && "focus" in f) {
        return f.navigate(cible).then(function (g) { return (g || f).focus(); })
          .catch(function () { return self.clients.openWindow(cible); });
      }
    }
    return self.clients.openWindow(cible);
  }));
});
