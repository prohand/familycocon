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
