// FamilyCocon : le seul JavaScript écrit à la main (le reste, c'est htmx).
// Notifications en bas de l'écran, bouton « Copier », temps réel.
(function () {
  "use strict";

  // Petite notification en bas de l'écran ; disparaît toute seule.
  function toast(texte, erreur, couleur, initiale) {
    var zone = document.getElementById("toasts");
    if (!zone || !texte) return;
    var t = document.createElement("div");
    t.className = "toast" + (erreur ? " erreur" : "");
    if (couleur) {
      var a = document.createElement("span");
      a.className = "avatar petit c" + couleur;
      a.textContent = initiale || "";
      t.appendChild(a);
    }
    t.appendChild(document.createTextNode(texte));
    zone.appendChild(t);
    setTimeout(function () { t.remove(); }, 5200);
    while (zone.children.length > 3) zone.firstChild.remove();
  }
  window.fcToast = toast;

  // Double clic : les boutons d'un formulaire sont désactivés pendant
  // l'envoi (pas de dépense ou de produit ajouté deux fois).
  function boutons(e) {
    var elt = e.detail && e.detail.elt;
    if (!elt || !elt.tagName) return [];
    if (elt.tagName === "FORM") return elt.querySelectorAll("button");
    return elt.tagName === "BUTTON" ? [elt] : [];
  }
  document.addEventListener("htmx:beforeRequest", function (e) {
    boutons(e).forEach(function (b) {
      if (!b.disabled) { b.disabled = true; b.setAttribute("data-envoi", "1"); }
    });
  });
  document.addEventListener("htmx:afterRequest", function (e) {
    boutons(e).forEach(function (b) {
      if (b.getAttribute("data-envoi")) { b.disabled = false; b.removeAttribute("data-envoi"); }
    });
  });

  // Notification demandée par le serveur (en-tête HX-Trigger : « fc-toast »).
  document.addEventListener("fc-toast", function (e) {
    var d = e.detail || {};
    toast(d.texte, d.erreur, d.couleur, d.initiale);
  });

  // Notification affichée au chargement (après une redirection) : retirée
  // après l'animation.
  function nettoyer() {
    document.querySelectorAll("#toasts .toast").forEach(function (t) {
      setTimeout(function () { t.remove(); }, 5200);
    });
  }
  document.addEventListener("DOMContentLoaded", nettoyer);
  document.addEventListener("htmx:afterSettle", nettoyer);

  // Bouton « Copier » : data-copier="#id-du-champ".
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-copier]");
    if (!b) return;
    var champ = document.querySelector(b.getAttribute("data-copier"));
    if (!champ) return;
    champ.select();
    var fait = function () { toast("Copié ✓"); };
    if (navigator.clipboard) {
      navigator.clipboard.writeText(champ.value).then(fait, function () { document.execCommand("copy"); fait(); });
    } else {
      document.execCommand("copy");
      fait();
    }
  });
})();

// Temps réel : une connexion /direct par page (EventSource se reconnecte
// tout seul). Message « fc » : { module, texte, auteur, couleur, initiale }.
//   - module : la page se met à jour (événement « maj-<module> » sur body,
//     écouté par les blocs htmx : hx-trigger="maj-courses from:body") ;
//   - texte : notification, sauf pour son auteur.
(function () {
  "use strict";
  var source = null;
  function connecter() {
    var url = document.body && document.body.getAttribute("data-direct");
    if (!url || source || !window.EventSource) return;
    source = new EventSource(url);
    source.addEventListener("fc", function (e) {
      var m;
      try { m = JSON.parse(e.data); } catch (_) { return; }
      var moi = document.body.getAttribute("data-membre");
      if (m.module && window.htmx) htmx.trigger(document.body, "maj-" + m.module, m);
      if (m.texte && String(m.auteur) !== moi) window.fcToast(m.texte, false, m.couleur, m.initiale);
    });
  }
  document.addEventListener("DOMContentLoaded", connecter);
  // Page mise en arrière-plan longtemps (téléphone) : on rafraîchit au retour.
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible" && source && source.readyState === 2) {
      source = null;
      connecter();
    }
  });
})();

// Formulaires d'ajout rapide (data-vider) : vidés après l'envoi, le curseur
// reste dans le champ pour enchaîner (« pain », « lait », « œufs »…).
document.addEventListener("htmx:afterRequest", function (e) {
  var f = e.detail.elt;
  if (!e.detail.successful || !f || !f.matches || !f.matches("form[data-vider]")) return;
  f.reset();
  var champ = f.querySelector("input:not([type=hidden])");
  if (champ) champ.focus();
});

// Rayons repliés : gardés en mémoire sur cet appareil (la liste est
// rechargée à chaque changement, les rayons repliés le restent).
(function () {
  "use strict";
  var CLE = "fc-rayons-replies";
  function lire() {
    try { return JSON.parse(localStorage.getItem(CLE) || "{}"); } catch (_) { return {}; }
  }
  function appliquer() {
    var replies = lire();
    document.querySelectorAll("details[data-rayon]").forEach(function (d) {
      if (replies[d.getAttribute("data-rayon")]) d.removeAttribute("open");
    });
  }
  document.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d.matches || !d.matches("details[data-rayon]")) return;
    var replies = lire();
    if (d.open) delete replies[d.getAttribute("data-rayon")];
    else replies[d.getAttribute("data-rayon")] = 1;
    try { localStorage.setItem(CLE, JSON.stringify(replies)); } catch (_) {}
  }, true);
  document.addEventListener("DOMContentLoaded", appliquer);
  document.addEventListener("htmx:afterSettle", appliquer);
})();

// Application installable : service worker (cache des fichiers, page hors
// ligne, notifications push).
if ("serviceWorker" in navigator) {
  window.addEventListener("load", function () {
    navigator.serviceWorker.register("/sw.js").catch(function () {});
  });
}

// Notifications push : bouton « Activer sur cet appareil » (page Moi).
(function () {
  "use strict";
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  function cleBinaire(b64) {
    var s = atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/"));
    var out = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) out[i] = s.charCodeAt(i);
    return out;
  }
  function envoyer(url, abonnement) {
    return fetch(url, {
      method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Jeton": jeton() },
      body: JSON.stringify(abonnement)
    }).then(function (r) { if (!r.ok) throw new Error("refusé"); });
  }
  function etat(texte) {
    var e = document.getElementById("etat-push");
    if (e) e.textContent = texte;
  }
  // Appareil partagé : l'abonnement de ce navigateur suit la personne
  // connectée (une fois par session), pour qu'un enfant ne reçoive pas les
  // notifications d'un parent sur la tablette familiale.
  function rattacher() {
    var membre = document.body.getAttribute("data-membre");
    if (!membre || !("serviceWorker" in navigator) || !("PushManager" in window)) return;
    var cle = "fc-push-membre";
    try { if (sessionStorage.getItem(cle) === membre) return; } catch (_) { return; }
    navigator.serviceWorker.ready.then(function (reg) { return reg.pushManager.getSubscription(); }).then(function (ab) {
      if (!ab) return;
      return envoyer("/push/abonnement", ab.toJSON()).then(function () {
        try { sessionStorage.setItem(cle, membre); } catch (_) {}
      });
    }).catch(function () {});
  }
  document.addEventListener("DOMContentLoaded", rattacher);
  // Déconnexion : cet appareil arrête d'abord de recevoir les notifications.
  document.addEventListener("submit", function (e) {
    var f = e.target;
    if (!f.matches || !f.matches("form[data-deconnexion]") || f.getAttribute("data-fait")) return;
    e.preventDefault();
    var finir = function () { f.setAttribute("data-fait", "1"); f.submit(); };
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) { finir(); return; }
    // « ready » ne répond jamais si le service worker n'a pas pu s'installer :
    // après 1,5 s, on déconnecte quand même.
    var delai = new Promise(function (ok) { setTimeout(ok, 1500); });
    var desabonner = navigator.serviceWorker.ready.then(function (reg) { return reg.pushManager.getSubscription(); }).then(function (ab) {
      if (!ab) return;
      return envoyer("/push/desabonnement", ab.toJSON()).finally(function () { return ab.unsubscribe(); });
    }).catch(function () {});
    Promise.race([desabonner, delai]).finally(function () {
      try { sessionStorage.removeItem("fc-push-membre"); } catch (_) {}
      finir();
    });
  }, true);
  function verifier() {
    var b = document.querySelector("[data-push]");
    if (!b) return;
    if (!("serviceWorker" in navigator) || !("PushManager" in window) || !window.Notification) {
      etat("Ce navigateur ne gère pas les notifications. Sur iPhone : ajoutez d'abord FamilyCocon à l'écran d'accueil (Partager › Sur l'écran d'accueil).");
      b.hidden = true;
      return;
    }
    navigator.serviceWorker.ready.then(function (reg) { return reg.pushManager.getSubscription(); }).then(function (ab) {
      b.setAttribute("data-push", ab ? "desactiver" : "activer");
      b.textContent = ab ? "🔕 Couper sur cet appareil" : "🔔 Activer sur cet appareil";
      if (Notification.permission === "denied") etat("Notifications bloquées dans les réglages du navigateur.");
    });
  }
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-push]");
    if (!b) return;
    e.preventDefault();
    b.disabled = true;
    navigator.serviceWorker.ready.then(function (reg) {
      if (b.getAttribute("data-push") === "desactiver") {
        return reg.pushManager.getSubscription().then(function (ab) {
          if (!ab) return;
          return envoyer("/push/desabonnement", ab.toJSON()).finally(function () { return ab.unsubscribe(); });
        }).then(function () { window.fcToast("Notifications coupées sur cet appareil"); });
      }
      return Notification.requestPermission().then(function (p) {
        if (p !== "granted") throw new Error("permission");
        return fetch("/push/cle", { credentials: "same-origin" }).then(function (r) { return r.text(); });
      }).then(function (cle) {
        return reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: cleBinaire(cle) });
      }).then(function (ab) {
        return envoyer("/push/abonnement", ab.toJSON());
      }).then(function () { window.fcToast("Notifications activées 🔔"); });
    }).catch(function () {
      window.fcToast("Impossible d'activer les notifications", true);
    }).finally(function () { b.disabled = false; verifier(); });
  });
  document.addEventListener("DOMContentLoaded", verifier);
  document.addEventListener("htmx:afterSettle", verifier);
})();

// Passkeys (WebAuthn) : connexion sans mot de passe, second facteur, ajout.
// Le serveur envoie les options en base64url ; le navigateur veut des octets.
(function () {
  "use strict";
  if (!window.PublicKeyCredential) return;
  function enOctets(b64) {
    var s = atob((b64 + "===".slice((b64.length + 3) % 4)).replace(/-/g, "+").replace(/_/g, "/"));
    var o = new Uint8Array(s.length);
    for (var i = 0; i < s.length; i++) o[i] = s.charCodeAt(i);
    return o.buffer;
  }
  function enB64(buf) {
    var o = new Uint8Array(buf), s = "";
    for (var i = 0; i < o.length; i++) s += String.fromCharCode(o[i]);
    return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  function lire(r) {
    return r.json().catch(function () { return {}; }).then(function (d) {
      if (!r.ok) throw new Error(d.erreur || "refusé");
      return d;
    });
  }
  function poster(url, corps) {
    return fetch(url, { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Jeton": jeton() }, body: JSON.stringify(corps) });
  }
  // Connexion : sans mot de passe (« connexion ») ou second facteur (« verification »).
  function seConnecter(mode) {
    var base = mode === "connexion" ? "/connexion/passkey" : "/connexion/verification";
    return fetch(base + (mode === "connexion" ? "/defi" : "/defi"), { credentials: "same-origin" }).then(lire).then(function (o) {
      o.challenge = enOctets(o.challenge);
      (o.allowCredentials || []).forEach(function (c) { c.id = enOctets(c.id); });
      return navigator.credentials.get({ publicKey: o });
    }).then(function (cred) {
      return poster(mode === "connexion" ? "/connexion/passkey" : "/connexion/verification/passkey", {
        id: cred.id,
        clientDataJSON: enB64(cred.response.clientDataJSON),
        authenticatorData: enB64(cred.response.authenticatorData),
        signature: enB64(cred.response.signature)
      });
    }).then(lire).then(function (d) { window.location.href = d.suite || "/"; });
  }
  function ajouter() {
    var nom = document.getElementById("nom-passkey");
    var mdp = document.getElementById("mdp-passkey");
    if (mdp && !mdp.value) return Promise.reject(new Error("Tapez votre mot de passe."));
    return fetch("/moi/securite/passkeys/defi", { credentials: "same-origin" }).then(lire).then(function (o) {
      o.challenge = enOctets(o.challenge);
      o.user.id = enOctets(o.user.id);
      (o.excludeCredentials || []).forEach(function (c) { c.id = enOctets(c.id); });
      return navigator.credentials.create({ publicKey: o });
    }).then(function (cred) {
      return poster("/moi/securite/passkeys", {
        clientDataJSON: enB64(cred.response.clientDataJSON),
        attestationObject: enB64(cred.response.attestationObject),
        nom: nom ? nom.value : "",
        mdp: mdp ? mdp.value : ""
      });
    }).then(function (r) {
      if (!r.ok) return lire(r);
      // Pas reload() : la page peut être à une adresse de formulaire
      // (après l'activation de la double authentification).
      window.location.href = "/moi/securite";
    });
  }
  function montrer() {
    document.querySelectorAll("[data-passkey]").forEach(function (b) { b.hidden = false; });
  }
  document.addEventListener("DOMContentLoaded", montrer);
  document.addEventListener("htmx:afterSettle", montrer);
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-passkey]");
    if (!b) return;
    e.preventDefault();
    b.disabled = true;
    var mode = b.getAttribute("data-passkey");
    var action = mode === "ajouter" ? ajouter() : seConnecter(mode);
    action.catch(function (err) {
      var msg = err && err.name === "NotAllowedError" ? "Annulé." : (err && err.message) || "Passkey refusée.";
      if (window.fcToast) window.fcToast(msg, true);
    }).finally(function () { b.disabled = false; });
  });
})();
