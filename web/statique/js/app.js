// FamilyCocon : le seul JavaScript écrit à la main (le reste, c'est htmx).
// Notifications en bas de l'écran, bouton « Copier », temps réel.
(function () {
  "use strict";

  // Petite notification en bas de l'écran ; disparaît toute seule.
  // annuler : adresse du bouton « Annuler » (suppression rattrapable).
  function toast(texte, erreur, couleur, initiale, annuler) {
    var zone = document.getElementById("toasts");
    if (!zone || !texte) return;
    var t = document.createElement("div");
    t.className = "toast" + (erreur ? " erreur" : "");
    // Une erreur est annoncée tout de suite aux lecteurs d'écran, et reste
    // plus longtemps à l'écran.
    if (erreur) t.setAttribute("role", "alert");
    if (couleur) {
      var a = document.createElement("span");
      a.className = "avatar petit c" + couleur;
      a.textContent = initiale || "";
      t.appendChild(a);
    }
    t.appendChild(document.createTextNode(texte));
    if (annuler) {
      var b = document.createElement("button");
      b.type = "button";
      b.className = "toast-action";
      b.textContent = "Annuler";
      b.addEventListener("click", function () {
        b.disabled = true;
        fetch(annuler, { method: "POST", credentials: "same-origin", headers: { "X-Jeton": jetonPage(), "HX-Request": "true" } })
          .then(function (r) {
            t.remove();
            if (r.ok) toast("Remis ✓");
            else toast("Trop tard pour annuler", true);
          }).catch(function () { toast("Pas de réseau : réessayez dans un instant", true); });
      });
      t.appendChild(b);
      t.classList.add("avec-action");
    }
    zone.appendChild(t);
    setTimeout(function () { t.remove(); }, erreur || annuler ? 8200 : 5200);
    while (zone.children.length > 3) zone.firstChild.remove();
  }
  window.fcToast = toast;
  function jetonPage() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }

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

  // Bloc « data-maj-prudent » (fiche avec des formulaires) : pas de
  // rechargement automatique pendant que quelqu'un y écrit ou a déplié un
  // formulaire, sa saisie serait perdue. Il sera à jour au prochain changement.
  document.addEventListener("htmx:beforeRequest", function (e) {
    var b = e.detail && e.detail.elt;
    if (!b || !b.hasAttribute || !b.hasAttribute("data-maj-prudent")) return;
    var actif = document.activeElement;
    var ecrit = actif && b.contains(actif) && /^(INPUT|TEXTAREA|SELECT)$/.test(actif.tagName);
    if (ecrit || b.querySelector("details[open]")) e.preventDefault();
  });

  // Réseau coupé ou serveur indisponible (proxy en 502…) : on le dit, au
  // lieu de ne rien faire. Un lien touché sans réseau ouvre la page « hors
  // ligne » du service worker (navigation classique).
  document.addEventListener("htmx:sendError", function (e) {
    var d = e.detail || {};
    var verbe = d.requestConfig && d.requestConfig.verb;
    if (verbe === "get" && d.elt && d.elt.tagName === "A" && d.elt.href && !d.elt.hasAttribute("hx-get")) {
      window.location.href = d.elt.href;
      return;
    }
    toast("Pas de réseau : réessayez dans un instant", true);
  });
  document.addEventListener("htmx:responseError", function (e) {
    var x = e.detail && e.detail.xhr;
    // Les erreurs du serveur FamilyCocon portent déjà leur message (fc-toast).
    if (x && x.getResponseHeader("HX-Trigger")) return;
    if (x && x.status >= 500) toast("Serveur indisponible : réessayez dans un instant", true);
  });

  // Notification demandée par le serveur (en-tête HX-Trigger : « fc-toast »).
  document.addEventListener("fc-toast", function (e) {
    var d = e.detail || {};
    toast(d.texte, d.erreur, d.couleur, d.initiale, d.annuler);
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
  function saisieEnCours() {
    var a = document.activeElement;
    return !!(a && /^(INPUT|TEXTAREA|SELECT)$/.test(a.tagName) && a.value);
  }
  function connecter() {
    var url = document.body && document.body.getAttribute("data-direct");
    if (!url || source || !window.EventSource) return;
    source = new EventSource(url);
    source.addEventListener("open", function () { document.dispatchEvent(new Event("fc-direct-ouvert")); });
    source.addEventListener("error", function () { document.dispatchEvent(new Event("fc-direct-erreur")); });
    source.addEventListener("fc", function (e) {
      var m;
      try { m = JSON.parse(e.data); } catch (_) { return; }
      var moi = document.body.getAttribute("data-membre");
      // Modules de la famille changés par un parent : tuiles, barre du bas et
      // pages changent ; la page est rechargée (sauf pendant une saisie).
      if (m.module === "famille" && String(m.auteur) !== moi) {
        if (!saisieEnCours()) window.location.reload();
        return;
      }
      if (m.module && window.htmx) {
        htmx.trigger(document.body, "maj-" + m.module, m);
        htmx.trigger(document.body, "maj", m); // n'importe quel module (accueil)
      }
      if (m.texte && String(m.auteur) !== moi) window.fcToast(m.texte, false, m.couleur, m.initiale);
    });
  }
  document.addEventListener("DOMContentLoaded", connecter);
  // Après une coupure (téléphone en veille, réseau perdu), les messages
  // envoyés entre-temps sont perdus : à la reconnexion, tous les blocs de la
  // page se rechargent (« maj-tout »).
  var perdu = false;
  function reprise() {
    if (!perdu) return;
    perdu = false;
    if (window.htmx) htmx.trigger(document.body, "maj-tout", {});
  }
  document.addEventListener("fc-direct-ouvert", reprise);
  document.addEventListener("fc-direct-erreur", function () { perdu = true; });
  // Page mise en arrière-plan longtemps (téléphone) : on rafraîchit au retour.
  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "hidden") {
      perdu = true; // le navigateur peut avoir gelé la connexion
      return;
    }
    if (source && source.readyState === 2) {
      source = null;
      connecter();
    } else if (source && source.readyState === 1) {
      reprise();
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

// Échéance choisie parmi les modèles (« Contrôle technique ») : sa
// fréquence (« Tous les 2 ans ») est choisie toute seule ; on peut la changer.
document.addEventListener("input", function (e) {
  var champ = e.target;
  if (!champ.matches || !champ.matches("input[data-modele]") || !champ.list || !champ.form) return;
  var choix = champ.form.querySelector("select[name=repetition_mois]");
  if (!choix) return;
  for (var i = 0; i < champ.list.options.length; i++) {
    var o = champ.list.options[i];
    if (o.value === champ.value && choix.querySelector("option[value='" + o.getAttribute("data-mois") + "']")) {
      choix.value = o.getAttribute("data-mois");
      return;
    }
  }
});

// Carte de fidélité à la caisse : l'écran reste allumé tant que la carte
// est affichée (Wake Lock, si le navigateur le permet).
(function () {
  "use strict";
  var verrou = null;
  function mettreAJour() {
    var voulu = document.visibilityState === "visible" && document.querySelector("[data-ecran-allume]");
    if (voulu && !verrou && navigator.wakeLock) {
      navigator.wakeLock.request("screen").then(function (v) {
        verrou = v;
        v.addEventListener("release", function () { verrou = null; });
      }).catch(function () {});
    } else if (!voulu && verrou) {
      verrou.release().catch(function () {});
      verrou = null;
    }
  }
  document.addEventListener("DOMContentLoaded", mettreAJour);
  document.addEventListener("htmx:afterSettle", mettreAJour);
  document.addEventListener("visibilitychange", mettreAJour);
})();

// Photo réduite dans le navigateur avant l'envoi (cote : plus grand côté),
// en JPEG : légère sur une connexion mobile, sous la limite d'envoi.
window.fcReduirePhoto = function (fichier, cote) {
  return createImageBitmap(fichier).then(function (img) {
    var r = Math.min(1, cote / Math.max(img.width, img.height));
    var c = document.createElement("canvas");
    c.width = Math.round(img.width * r);
    c.height = Math.round(img.height * r);
    c.getContext("2d").drawImage(img, 0, 0, c.width, c.height);
    return new Promise(function (ok) { c.toBlob(ok, "image/jpeg", 0.85); });
  });
};

// Carnet : photo d'un ticket ou d'une facture jointe à une ligne
// (input[data-piece] = adresse d'envoi). La page est rechargée ensuite.
document.addEventListener("change", function (e) {
  var p = e.target;
  if (!p.matches || !p.matches("input[data-piece]") || !p.files || !p.files[0]) return;
  if (!window.createImageBitmap) { window.fcToast("Photo non prise en charge par ce navigateur", true); return; }
  var jeton = "";
  try { jeton = JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) {}
  window.fcToast("Envoi de la photo…");
  window.fcReduirePhoto(p.files[0], 1600).then(function (blob) {
    return fetch(p.getAttribute("data-piece"), { method: "POST", credentials: "same-origin",
      headers: { "X-Jeton": jeton, "Content-Type": "image/jpeg" }, body: blob });
  }).then(function (rep) {
    if (!rep.ok) throw new Error("refusée");
    window.location.reload();
  }).catch(function () { window.fcToast("Photo non envoyée", true); });
  p.value = "";
});

// Notes : dans le texte (textarea[data-liste]), Entrée au bout d'une ligne
// « [ ] … », « [x] … » ou « - … » commence la ligne suivante pareil (une
// nouvelle case, une nouvelle puce). Entrée sur une case vide l'enlève et
// termine la liste.
document.addEventListener("keydown", function (e) {
  var t = e.target;
  if (e.key !== "Enter" || e.shiftKey || e.isComposing || !t.matches || !t.matches("textarea[data-liste]")) return;
  if (t.selectionStart !== t.selectionEnd) return;
  var v = t.value, pos = t.selectionStart;
  var debut = v.lastIndexOf("\n", pos - 1) + 1;
  var ligne = v.slice(debut, pos);
  var m = /^(\s*(?:[-*] )?)(\[[ xX]\] ?)?(.*)$/.exec(ligne);
  if (!m || (!m[2] && !/[-*] $/.test(m[1]))) return;
  e.preventDefault();
  var marque = m[1] + (m[2] ? "[ ] " : "");
  if (m[3].trim() === "") {
    // Ligne vide : fin de la liste.
    t.value = v.slice(0, debut) + v.slice(pos);
    t.selectionStart = t.selectionEnd = debut;
    return;
  }
  t.value = v.slice(0, pos) + "\n" + marque + v.slice(pos);
  t.selectionStart = t.selectionEnd = pos + 1 + marque.length;
});

// Véhicule : reprise d'une sauvegarde Fuelio (input[data-fuelio] = adresse
// d'envoi). Le fichier part tel quel (il peut peser des centaines de Mo,
// photos comprises) ; l'avancement s'affiche, puis la fiche est rechargée
// avec le bilan.
document.addEventListener("change", function (e) {
  var p = e.target;
  if (!p.matches || !p.matches("input[data-fuelio]") || !p.files || !p.files[0]) return;
  var jeton = "";
  try { jeton = JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) {}
  var x = new XMLHttpRequest();
  var palier = -1;
  x.open("POST", p.getAttribute("data-fuelio"));
  x.setRequestHeader("X-Jeton", jeton);
  x.setRequestHeader("HX-Request", "true");
  x.setRequestHeader("Content-Type", "application/octet-stream");
  x.upload.onprogress = function (ev) {
    if (!ev.lengthComputable) return;
    var pc = Math.floor(ev.loaded * 10 / ev.total) * 10;
    if (pc === palier) return;
    palier = pc;
    window.fcToast(pc < 100 ? "Envoi du fichier… " + pc + " %" : "Lecture des pleins et des photos… (patientez)");
  };
  x.onload = function () {
    if (x.status >= 200 && x.status < 300) { window.location.assign(x.getResponseHeader("HX-Location") || window.location.href); return; }
    var msg = "Reprise impossible";
    try { msg = JSON.parse(x.getResponseHeader("HX-Trigger"))["fc-toast"].texte || msg; } catch (_) {}
    window.fcToast(msg, true);
  };
  x.onerror = function () { window.fcToast("Envoi interrompu", true); };
  window.fcToast("Envoi du fichier…");
  x.send(p.files[0]);
  p.value = "";
});

// Pleins : « Scanner le ticket ». Le texte de la photo est lu dans le
// téléphone par Tesseract.js (chargé au premier scan seulement, depuis
// /ocr/v7, puis gardé par le navigateur), puis envoyé au serveur qui y
// trouve la date, les litres et les prix (/suivi/ticket). Les champs se
// remplissent et sont surlignés ; la photo réduite part avec le plein.
(function () {
  "use strict";
  var OCR = "/ocr/v7/";
  var script = null;
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  function chargerTesseract() {
    if (!script) script = new Promise(function (ok, ko) {
      var s = document.createElement("script");
      s.src = OCR + "tesseract.min.js";
      s.onload = ok;
      s.onerror = function () { script = null; ko(new Error("Lecteur indisponible")); };
      document.head.appendChild(s);
    });
    return script;
  }
  // L'image tournée (degrés) et mise à la taille voulue (plus grand côté).
  function dessiner(img, rot, cote) {
    var r = cote / Math.max(img.width, img.height);
    var w = Math.round(img.width * r), h = Math.round(img.height * r);
    var c = document.createElement("canvas");
    var quart = rot % 180 !== 0;
    c.width = quart ? h : w;
    c.height = quart ? w : h;
    var x = c.getContext("2d");
    x.translate(c.width / 2, c.height / 2);
    x.rotate(rot * Math.PI / 180);
    x.drawImage(img, -w / 2, -h / 2, w, h);
    return c;
  }
  function analyser(texte) {
    return fetch("/suivi/ticket", { method: "POST", credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-Jeton": jeton() }, body: JSON.stringify({ texte: texte }) })
      .then(function (rep) { if (!rep.ok) throw new Error("refusé"); return rep.json(); });
  }
  function trouves(t) { return [t.litres, t.montant, t.prixLitre].filter(Boolean).length; }
  async function lireTicket(fichier) {
    await chargerTesseract();
    var img = await createImageBitmap(fichier);
    var worker = await Tesseract.createWorker("fra", 1, { workerPath: OCR + "worker.min.js",
      corePath: OCR + "tesseract-core-simd-lstm.js", langPath: OCR, workerBlobURL: false });
    try {
      // Photo en largeur : le ticket est sans doute couché (un quart de tour).
      var sens = img.width > img.height ? [90, 270, 0] : [0, 180];
      var texte = "", res = null, rot = sens[0];
      for (var i = 0; i < sens.length; i++) {
        var t = (await worker.recognize(dessiner(img, sens[i], 1600))).data.text;
        var r = await analyser(t);
        if (!res || trouves(r) > trouves(res)) { res = r; texte = t; rot = sens[i]; }
        if (trouves(r) >= 2) break;
      }
      // Date ou nombres manquants : une seconde lecture, plus grande.
      if (trouves(res) >= 1 && (!res.complet || !res.jour)) {
        var t2 = (await worker.recognize(dessiner(img, rot, 2400))).data.text;
        res = await analyser(texte + "\n" + t2);
      }
      return res;
    } finally {
      worker.terminate();
    }
  }
  function remplir(f, res) {
    var n = 0;
    [["jour", res.jour], ["litres", res.litres], ["montant", res.montant], ["carburant", res.carburant], ["note", res.note]].forEach(function (x) {
      var champ = f.querySelector("[name=" + x[0] + "]");
      if (!champ || !x[1] || (x[0] === "note" && champ.value)) return;
      champ.value = x[1];
      champ.classList.add("lu");
      n++;
    });
    return n;
  }
  document.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest("[data-scanner-ticket]");
    if (!b) return;
    var photo = b.parentNode.querySelector("[data-photo-ticket]");
    if (photo) photo.click();
  });
  ["input", "change"].forEach(function (type) {
    document.addEventListener(type, function (e) {
      if (e.target.classList && e.target.classList.contains("lu")) e.target.classList.remove("lu");
    });
  });
  document.addEventListener("change", function (e) {
    var p = e.target;
    if (!p.matches || !p.matches("[data-photo-ticket]") || !p.files || !p.files[0]) return;
    var f = p.closest("form"), fichier = p.files[0];
    var etat = f.querySelector("[data-etat-ticket]"), bouton = f.querySelector("[data-scanner-ticket]");
    p.value = "";
    if (!window.createImageBitmap || !window.WebAssembly) { window.fcToast("Lecture impossible sur ce navigateur : remplissez à la main", true); return; }
    bouton.disabled = true;
    etat.textContent = "Lecture du ticket… (quelques secondes ; la première fois, le lecteur se télécharge : 2 Mo)";
    // La photo réduite, jointe au plein à l'enregistrement.
    window.fcReduirePhoto(fichier, 1200).then(function (blob) {
      var lecteur = new FileReader();
      lecteur.onload = function () {
        var url = String(lecteur.result);
        f.querySelector("[name=ticket]").value = url.slice(url.indexOf(",") + 1);
        var ap = f.querySelector("[data-apercu-ticket]");
        ap.src = url;
        ap.hidden = false;
      };
      lecteur.readAsDataURL(blob);
    });
    lireTicket(fichier).then(function (res) {
      var n = remplir(f, res);
      etat.textContent = n ? "Lu sur le ticket : vérifiez les champs surlignés, puis tapez le kilométrage. La photo sera jointe au plein."
        : "Ticket illisible : remplissez à la main (la photo sera jointe au plein).";
      window.fcToast(n ? "Ticket lu ✓" : "Ticket illisible", !n);
      var km = f.querySelector("[name=km]");
      if (km && n) km.focus();
    }).catch(function () {
      etat.textContent = "Lecture impossible sur ce téléphone : remplissez à la main (la photo sera jointe au plein).";
      window.fcToast("Lecture impossible", true);
    }).finally(function () { bouton.disabled = false; });
  });
})();

// Carte de fidélité : « Scanner la carte ». Sur Android (Chrome), le
// navigateur lit le code en direct avec la caméra (BarcodeDetector). Sinon
// (iPhone…), une photo est prise, réduite à 1 280 px et envoyée au serveur
// qui la lit (EAN-13, UPC-A, EAN-8, Code 128). Le numéro et le type de code
// sont remplis tout seuls ; rien n'est gardé.
(function () {
  "use strict";
  var types = { ean_13: "ean13", ean_8: "ean8", upc_a: "upca", code_128: "code128", qr_code: "qr" };
  var noms = { ean13: "EAN-13", ean8: "EAN-8", upca: "UPC-A", code128: "Code 128", qr: "QR code" };
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  function remplir(f, numero, type) {
    var champ = f.querySelector("[name=numero]");
    if (champ) champ.value = numero;
    var choix = f.querySelector("[name=format]");
    var t = choix && choix.querySelector("option[value='" + type + "']") ? type : "auto";
    if (choix) choix.value = t;
    var info = f.querySelector("[data-format-lu]");
    if (info) info.textContent = "Code lu : " + (noms[t] || "détecté tout seul");
    window.fcToast("Carte lue ✓");
  }
  // Photo (iPhone, ou caméra refusée) : lue par le serveur.
  function envoyer(f, fichier) {
    if (!window.createImageBitmap) { window.fcToast("Photo non prise en charge : tapez le numéro", true); return; }
    window.fcToast("Lecture de la photo…");
    window.fcReduirePhoto(fichier, 1280).then(function (blob) {
      return fetch("/fidelite/scanner", { method: "POST", credentials: "same-origin",
        headers: { "X-Jeton": jeton(), "Content-Type": "image/jpeg" }, body: blob });
    }).then(function (rep) {
      return rep.json().then(function (d) { if (!rep.ok) throw new Error(d.erreur || "Code non lu"); return d; });
    }).then(function (d) { remplir(f, d.numero, d.format); })
      .catch(function (e) { window.fcToast((e && e.message) || "Code non lu", true); });
  }
  // Caméra en direct avec le lecteur du navigateur.
  function direct(f, photo) {
    var lecteur;
    try { lecteur = new BarcodeDetector(); } catch (_) { photo.click(); return; }
    navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" } }).then(function (flux) {
      var d = document.createElement("dialog");
      d.className = "scanner";
      var v = document.createElement("video");
      v.setAttribute("playsinline", "");
      v.muted = true;
      v.srcObject = flux;
      var b = document.createElement("button");
      b.type = "button";
      b.className = "bouton secondaire";
      b.textContent = "Annuler";
      var aide = document.createElement("p");
      aide.textContent = "Visez le code-barres de la carte";
      d.appendChild(aide); d.appendChild(v); d.appendChild(b);
      document.body.appendChild(d);
      d.showModal();
      v.play().catch(function () {});
      var fini = false;
      function arreter() {
        if (fini) return;
        fini = true;
        flux.getTracks().forEach(function (t) { t.stop(); });
        d.close();
        d.remove();
      }
      b.addEventListener("click", arreter);
      d.addEventListener("cancel", arreter);
      (function chercher() {
        if (fini) return;
        lecteur.detect(v).then(function (codes) {
          if (fini) return;
          if (codes.length) { arreter(); remplir(f, codes[0].rawValue, types[codes[0].format] || "auto"); return; }
          setTimeout(chercher, 250);
        }).catch(function () { setTimeout(chercher, 500); });
      })();
    }).catch(function () { photo.click(); }); // caméra refusée : une photo
  }
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-scanner]");
    if (!b) return;
    var f = b.closest("form");
    var photo = f && f.querySelector("[data-photo]");
    if (!photo) return;
    if ("BarcodeDetector" in window && navigator.mediaDevices && navigator.mediaDevices.getUserMedia) direct(f, photo);
    else photo.click();
  });
  document.addEventListener("change", function (e) {
    var p = e.target;
    if (!p.matches || !p.matches("[data-photo]") || !p.files || !p.files[0]) return;
    envoyer(p.closest("form"), p.files[0]);
    p.value = "";
  });
})();

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
    if (!("serviceWorker" in navigator) || !("PushManager" in window)) {
      (window.caches ? caches.delete("familycocon-pages") : Promise.resolve()).catch(function () {}).finally(finir);
      return;
    }
    // « ready » ne répond jamais si le service worker n'a pas pu s'installer :
    // après 1,5 s, on déconnecte quand même.
    var delai = new Promise(function (ok) { setTimeout(ok, 1500); });
    var desabonner = navigator.serviceWorker.ready.then(function (reg) { return reg.pushManager.getSubscription(); }).then(function (ab) {
      if (!ab) return;
      return envoyer("/push/desabonnement", ab.toJSON()).finally(function () { return ab.unsubscribe(); });
    }).catch(function () {});
    // La liste de courses gardée pour le mode hors ligne part aussi.
    var oublier = window.caches ? caches.delete("familycocon-pages").catch(function () {}) : Promise.resolve();
    Promise.race([Promise.all([desabonner, oublier]), delai]).finally(function () {
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
