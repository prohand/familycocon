// Service worker de FamilyCocon (le plus petit possible) :
//   - fichiers statiques (/statique/…?v=…) : gardés en cache, ils ne
//     changent jamais à adresse égale ;
//   - pages : toujours le réseau (données à jour) ; sans réseau, une page
//     « hors ligne » ;
//   - liste de courses : la dernière ouverte est gardée, pour la lire au
//     magasin sans réseau (cache vidé à la déconnexion, par app.js) ;
//   - notifications push : affichées, et un toucher ouvre la bonne page.
"use strict";
// Le serveur remplace ce nom par la version des fichiers : à chaque mise à
// jour, un nouveau cache est rempli et l'ancien est supprimé (« activate »).
const CACHE = "familycocon-1";
// Pages gardées pour une lecture sans réseau (la liste de courses).
const PAGES = "familycocon-pages";
const LISTE = /^\/courses\/liste\/\d+$/;
// Adresse (jamais demandée au serveur) où la page visée par une notification
// est gardée pour app.js.
const CIBLE = "/__cible-notification";

self.addEventListener("install", function (e) {
  e.waitUntil(caches.open(CACHE).then(function (c) {
    return c.addAll(["/hors-ligne", "/statique/app.css", "/statique/img/logo.svg", "/statique/fonts/nunito.woff2"]);
  }));
  self.skipWaiting();
});

self.addEventListener("activate", function (e) {
  e.waitUntil(caches.keys().then(function (cles) {
    return Promise.all(cles.filter(function (k) { return k !== CACHE && k !== PAGES; }).map(function (k) { return caches.delete(k); }));
  }).then(function () { return self.clients.claim(); }));
});

self.addEventListener("fetch", function (e) {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== location.origin) return;
  // La page visée par la dernière notification touchée (voir plus bas) :
  // lue une seule fois par app.js, puis oubliée.
  if (url.pathname === CIBLE) {
    e.respondWith(caches.open(PAGES).then(function (c) {
      return c.match(CIBLE).then(function (r) {
        c.delete(CIBLE);
        return r || new Response(null, { status: 204 });
      });
    }));
    return;
  }
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
    const courses = LISTE.test(url.pathname) || url.pathname === "/courses";
    e.respondWith(fetch(req).then(function (rep) {
      // Liste de courses ouverte : une copie est gardée (aussi sous /courses,
      // l'adresse de la barre du bas).
      if (rep.ok && !rep.redirected && LISTE.test(url.pathname)) {
        const copie = rep.clone(), copie2 = rep.clone();
        caches.open(PAGES).then(function (c) {
          c.put(url.pathname, copie);
          c.put("/courses", copie2);
        });
      }
      return rep;
    }).catch(function () {
      const secours = courses ? caches.open(PAGES).then(function (c) { return c.match(url.pathname); }) : Promise.resolve(null);
      return secours.then(function (r) { return r || caches.match("/hors-ligne"); });
    }));
  }
});

self.addEventListener("push", function (e) {
  let d = {};
  try { d = e.data ? e.data.json() : {}; } catch (_) { d = { texte: e.data && e.data.text() }; }
  e.waitUntil(self.registration.showNotification(d.titre || "FamilyCocon", {
    body: d.texte || "",
    icon: "/statique/img/icone-192.png",
    // Badge : silhouette blanche sur fond transparent (barre d'état Android).
    badge: "/statique/img/badge-96.png",
    tag: d.groupe || "familycocon",
    renotify: true,
    data: { url: d.url || "/" }
  }));
});

self.addEventListener("notificationclick", function (e) {
  e.notification.close();
  // Seulement une page de l'appli (jamais un autre site, même si le contenu
  // de la notification était faux).
  let cible = new URL((e.notification.data && e.notification.data.url) || "/", location.origin);
  if (cible.origin !== location.origin) cible = new URL("/", location.origin);
  cible = cible.href;
  // Sur iPhone, l'appli fermée s'ouvre souvent sur sa page de départ (l'accueil)
  // au lieu de la page demandée : la cible est gardée une minute, et app.js
  // y va dès que l'appli s'ouvre.
  const garder = caches.open(PAGES).then(function (c) {
    return c.put(CIBLE, new Response(JSON.stringify({ url: cible, le: Date.now() }), { headers: { "Content-Type": "application/json" } }));
  });
  e.waitUntil(garder.then(function () { return self.clients.matchAll({ type: "window", includeUncontrolled: true }); }).then(function (fenetres) {
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
