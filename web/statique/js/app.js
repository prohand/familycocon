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
