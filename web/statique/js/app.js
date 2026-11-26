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
      if (champ.type === "radio") {
        // Choix en boutons (carburant) : celui lu sur le ticket.
        var r = f.querySelector("[name=" + x[0] + "][value='" + x[1].replace(/'/g, "") + "']");
        if (!r) return;
        r.checked = true;
        r.closest("label").classList.add("lu");
      } else {
        champ.value = x[1];
        champ.classList.add("lu");
      }
      champ.dispatchEvent(new Event("input", { bubbles: true }));
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
      if (!e.isTrusted) return;
      if (e.target.classList && e.target.classList.contains("lu")) e.target.classList.remove("lu");
      var lbl = e.target.type === "radio" && e.target.form && e.target.form.querySelector("label.lu [name=" + e.target.name + "]");
      if (lbl) lbl.closest("label").classList.remove("lu");
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

// Carte de fidélité : « Scanner la carte ». La caméra filme et le code est
// lu en direct : par le lecteur du navigateur (BarcodeDetector, Android),
// sinon (iPhone…) par zxing-wasm dans un worker (/ocr/zxing316/lecteur.js :
// QR code, PDF417, Aztec, EAN, Code 128…). Caméra refusée : une photo, lue
// par zxing-wasm puis, en dernier recours, par le serveur (EAN, Code 128).
// Le numéro et le type de code sont remplis tout seuls ; rien n'est gardé.
(function () {
  "use strict";
  var types = { ean_13: "ean13", ean_8: "ean8", upc_a: "upca", code_128: "code128", code_39: "code39", code_93: "code93",
    codabar: "codabar", itf: "itf", qr_code: "qr", data_matrix: "datamatrix", pdf417: "pdf417", aztec: "aztec" };
  var typesZx = { EAN13: "ean13", EAN8: "ean8", UPCA: "upca", Code128: "code128", Code39: "code39", Code93: "code93",
    Codabar: "codabar", ITF: "itf", QRCode: "qr", DataMatrix: "datamatrix", PDF417: "pdf417", Aztec: "aztec" };
  var noms = { ean13: "EAN-13", ean8: "EAN-8", upca: "UPC-A", code128: "Code 128", code39: "Code 39", code93: "Code 93",
    codabar: "Codabar", itf: "ITF", qr: "QR code", datamatrix: "Data Matrix", pdf417: "PDF417", aztec: "Aztec" };
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  function remplir(f, numero, type) {
    // Codabar : sans les lettres de début et de fin (A à D), comme le numéro
    // imprimé ; le dessin les remet.
    if (type === "codabar" && /^[A-D].+[A-D]$/i.test(numero)) numero = numero.slice(1, -1);
    var champ = f.querySelector("[name=numero]");
    if (champ) champ.value = numero;
    var choix = f.querySelector("[name=format]");
    var t = choix && choix.querySelector("option[value='" + type + "']") ? type : "auto";
    if (choix) choix.value = t;
    var info = f.querySelector("[data-format-lu]");
    if (info) info.textContent = "Code lu : " + (noms[t] || "détecté tout seul");
    window.fcToast("Carte lue ✓");
  }

  // Lecteur zxing-wasm (worker unique, chargé au premier scan). lire(image)
  // → promesse de {texte, format} ou null.
  var zx = null;
  function zxing() {
    if (zx) return zx;
    if (!window.Worker || !window.WebAssembly) return null;
    var w = new Worker("/ocr/zxing316/lecteur.js");
    var attente = null;
    w.onmessage = function (e) {
      var r = attente; attente = null;
      if (!r) return;
      var c = e.data && e.data.codes && e.data.codes[0];
      r(c ? { texte: c.texte, format: typesZx[c.format] || "auto" } : null);
    };
    w.onerror = function () { var r = attente; attente = null; if (r) r(null); };
    zx = {
      lire: function (image) {
        return new Promise(function (ok) {
          if (attente) { ok(null); return; } // une lecture à la fois
          attente = ok;
          w.postMessage(image, [image.data.buffer]);
        });
      }
    };
    return zx;
  }
  // Une image (vidéo ou photo) en ImageData, 1 600 px au plus.
  function pixels(source, l, h) {
    var k = Math.min(1, 1600 / Math.max(l, h));
    var c = document.createElement("canvas");
    c.width = Math.round(l * k); c.height = Math.round(h * k);
    var x = c.getContext("2d", { willReadFrequently: true });
    x.drawImage(source, 0, 0, c.width, c.height);
    return x.getImageData(0, 0, c.width, c.height);
  }

  // Photo : zxing-wasm, sinon le serveur.
  function envoyer(f, fichier) {
    if (!window.createImageBitmap) { window.fcToast("Photo non prise en charge : tapez le numéro", true); return; }
    window.fcToast("Lecture de la photo…");
    var lecteur = zxing();
    var d = lecteur ? createImageBitmap(fichier).then(function (bmp) { return lecteur.lire(pixels(bmp, bmp.width, bmp.height)); }).catch(function () { return null; })
                    : Promise.resolve(null);
    d.then(function (code) {
      if (code) { remplir(f, code.texte, code.format); return; }
      return window.fcReduirePhoto(fichier, 1280).then(function (blob) {
        return fetch("/fidelite/scanner", { method: "POST", credentials: "same-origin",
          headers: { "X-Jeton": jeton(), "Content-Type": "image/jpeg" }, body: blob });
      }).then(function (rep) {
        return rep.json().then(function (j) { if (!rep.ok) throw new Error(j.erreur || "Code non lu"); return j; });
      }).then(function (j) { remplir(f, j.numero, j.format); });
    }).catch(function (e) { window.fcToast((e && e.message) || "Code non lu", true); });
  }

  // Caméra en direct : lecteur du navigateur, ou zxing-wasm image par image.
  function direct(f, photo) {
    var detecteur = null, lecteur = null;
    if ("BarcodeDetector" in window) { try { detecteur = new BarcodeDetector(); } catch (_) { detecteur = null; } }
    if (!detecteur) lecteur = zxing();
    if (!detecteur && !lecteur) { photo.click(); return; }
    navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment", width: { ideal: 1920 } } }).then(function (flux) {
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
      aide.textContent = "Visez le code de la carte (code-barres, QR code…)";
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
      function trouve(texte, type) { arreter(); remplir(f, texte, type); }
      (function chercher() {
        if (fini) return;
        if (detecteur) {
          detecteur.detect(v).then(function (codes) {
            if (fini) return;
            if (codes.length) { trouve(codes[0].rawValue, types[codes[0].format] || "auto"); return; }
            setTimeout(chercher, 250);
          }).catch(function () { setTimeout(chercher, 500); });
          return;
        }
        if (!v.videoWidth) { setTimeout(chercher, 300); return; }
        lecteur.lire(pixels(v, v.videoWidth, v.videoHeight)).then(function (code) {
          if (fini) return;
          if (code) { trouve(code.texte, code.format); return; }
          setTimeout(chercher, 200);
        });
      })();
    }).catch(function () { photo.click(); }); // caméra refusée : une photo
  }
  document.addEventListener("click", function (e) {
    var b = e.target.closest("[data-scanner]");
    if (!b) return;
    var f = b.closest("form");
    var photo = f && f.querySelector("[data-photo]");
    if (!photo) return;
    if (navigator.mediaDevices && navigator.mediaDevices.getUserMedia) direct(f, photo);
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
// Rayon fini (data-fini : tout est pris) : le serveur l'envoie replié, en
// bas. Rouvert à la main, il reste ouvert aux rechargements (un autre
// membre coche) tant qu'il est fini ; le replier ne touche pas à la mémoire
// de l'appareil (sinon il resterait replié une fois de nouveau à remplir).
(function () {
  "use strict";
  var CLE = "fc-rayons-replies";
  var finisOuverts = {}; // rayons finis rouverts à la main (cette visite)
  function lire() {
    try { return JSON.parse(localStorage.getItem(CLE) || "{}"); } catch (_) { return {}; }
  }
  function ecrire(replies) {
    try { localStorage.setItem(CLE, JSON.stringify(replies)); } catch (_) {}
  }
  function appliquer() {
    var replies = lire();
    document.querySelectorAll("details[data-rayon]").forEach(function (d) {
      var cle = d.getAttribute("data-rayon");
      if (d.hasAttribute("data-fini")) {
        if (finisOuverts[cle]) d.setAttribute("open", "");
        return;
      }
      delete finisOuverts[cle]; // plus fini : la prochaine fois, il se repliera
      if (replies[cle]) d.removeAttribute("open");
    });
  }
  document.addEventListener("toggle", function (e) {
    var d = e.target;
    if (!d.matches || !d.matches("details[data-rayon]")) return;
    var cle = d.getAttribute("data-rayon");
    var replies = lire();
    if (d.hasAttribute("data-fini")) {
      if (!d.open) { delete finisOuverts[cle]; return; }
      finisOuverts[cle] = 1;
      if (replies[cle]) { delete replies[cle]; ecrire(replies); } // ouvert exprès
      return;
    }
    if (d.open) delete replies[cle];
    else replies[cle] = 1;
    ecrire(replies);
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

// Fenêtres par-dessus la page (« Ajouter un plein », « Entretien ») :
// data-ouvrir="id" ouvre, data-fermer ferme ; data-auto : ouverte dès
// l'arrivée sur la page (raccourci « Ajouter un plein »). Dans la fenêtre du
// plein : prix au litre calculé, kilométrage comparé au dernier relevé,
// stations proposées en un toucher.
(function () {
  "use strict";
  function ouvrir(d) {
    if (!d || d.open) return;
    if (d.showModal) d.showModal(); else d.setAttribute("open", "");
  }
  document.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest("[data-ouvrir]");
    if (b) { e.preventDefault(); ouvrir(document.getElementById(b.getAttribute("data-ouvrir"))); return; }
    var f = e.target.closest && e.target.closest("[data-fermer]");
    if (f) { var d = f.closest("dialog"); if (d) d.close(); return; }
    // Toucher le fond (hors de la fenêtre) la ferme.
    if (e.target.tagName === "DIALOG" && e.target.classList.contains("fenetre")) {
      var r = e.target.getBoundingClientRect();
      if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) e.target.close();
    }
    var p = e.target.closest && e.target.closest("[data-remplir]");
    if (p) {
      var champ = p.closest("form").querySelector("[name=" + p.getAttribute("data-remplir") + "]");
      if (champ) { champ.value = p.getAttribute("data-valeur"); champ.dispatchEvent(new Event("input", { bubbles: true })); }
    }
  });
  function auto() {
    var d = document.querySelector("dialog.fenetre[data-auto]");
    if (!d) return;
    d.removeAttribute("data-auto");
    ouvrir(d);
    // L'adresse perd « ajout=… » : recharger la page ne rouvre pas la fenêtre.
    try {
      var u = new URL(location.href);
      u.searchParams.delete("ajout");
      history.replaceState(history.state, "", u.pathname + u.search + u.hash);
    } catch (_) { /* rien */ }
  }
  document.addEventListener("DOMContentLoaded", auto);
  document.addEventListener("htmx:afterSettle", auto);

  function nombre(v) { return parseFloat(String(v || "").replace(/\s/g, "").replace(",", ".")); }
  function milliers(n) { return String(Math.round(n)).replace(/\B(?=(\d{3})+(?!\d))/g, "\u202f"); }
  document.addEventListener("input", function (e) {
    var f = e.target.form;
    if (!f || !f.classList.contains("ajout-plein")) return;
    var l = nombre(f.litres && f.litres.value), m = nombre(f.montant && f.montant.value);
    var pl = f.querySelector("[data-prix-litre]");
    if (pl) pl.textContent = l > 0 && m > 0 ? "= " + (m / l).toFixed(3).replace(".", ",") + " le litre" : "";
    var etat = f.querySelector("[data-etat-km]"), dernier = parseInt(f.getAttribute("data-dernier-km"), 10) || 0;
    var km = parseInt(String(f.km.value).replace(/\D/g, ""), 10);
    if (!etat || !dernier) return;
    etat.classList.remove("attention");
    if (!km) { etat.textContent = "Dernier relevé : " + milliers(dernier) + " km"; return; }
    var d = km - dernier;
    if (d < 0) { etat.textContent = "⚠️ Moins que le dernier relevé (" + milliers(dernier) + " km) : vérifiez."; etat.classList.add("attention"); }
    else if (d > 3000) { etat.textContent = "⚠️ +" + milliers(d) + " km depuis le dernier relevé : vérifiez."; etat.classList.add("attention"); }
    else etat.textContent = "+" + milliers(d) + " km depuis le dernier relevé ✓";
  });

  // Chiffres : la période choisie, visible dans la rangée de boutons.
  function centrer() {
    document.querySelectorAll(".defile .active").forEach(function (a) {
      var n = a.parentNode;
      n.scrollLeft = a.offsetLeft - n.offsetLeft - (n.clientWidth - a.offsetWidth) / 2;
    });
  }
  document.addEventListener("DOMContentLoaded", centrer);
  document.addEventListener("htmx:afterSettle", centrer);
})();

// Notes : éditeur de texte (textarea[data-editeur] = numéro de la note,
// vide pour une nouvelle). Le serveur donne le contenu déjà mis en forme
// (<template data-editeur-contenu>) ; à chaque frappe, l'éditeur réécrit
// le texte simple de la note dans le textarea (caché), avec ses signes :
// « # titre », « **gras** », « - puce », « [ ] case », « [photo 12] »… (voir
// internal/notes/format.go). Sans JavaScript, le textarea s'affiche tel quel.
//   - barre d'outils fixe en haut : annuler, gras, italique, souligné,
//     barré, code, cases, puces, numéros, titres, citation, lien, photo,
//     séparation ;
//   - une image collée ou glissée est envoyée (réduite) et posée au
//     curseur ; un texte copié d'une page garde gras, listes et liens ;
//   - Entrée au bout d'une case : la suivante ; sur une case vide : fin ;
//   - un lien touché propose de l'ouvrir.
(function () {
  "use strict";
  function jeton() {
    try { return JSON.parse(document.body.getAttribute("hx-headers"))["X-Jeton"]; } catch (_) { return ""; }
  }
  var BLOCS = /^(DIV|P|H[1-6]|UL|OL|LI|BLOCKQUOTE|HR|FIGURE|PRE|SECTION|ARTICLE|HEADER|FOOTER|TABLE|TR)$/;

  // --- Du contenu de l'éditeur au texte de la note -------------------------

  // Le texte tel quel, ses signes précédés d'un « \ » (sauf dans les adresses).
  function echap(t) {
    return t.split(/(https?:\/\/[^\s<>"]+)/).map(function (x, i) {
      if (i % 2) return x;
      return x.replace(/\\/g, "\\\\").replace(/([*_`[])/g, "\\$1").replace(/~~/g, "\\~\\~").replace(/\+\+/g, "\\+\\+");
    }).join("");
  }
  // Une ligne de texte qui ressemblerait à un titre, une puce… : « \ » devant.
  function debut(l) { return /^\s*(#{1,6} |>|[-•] |\d+\. |-{3,}\s*$)/.test(l) ? "\\" + l.replace(/^\s+/, "") : l; }
  function entoure(signe, dedans) {
    var m = /^(\s*)([\s\S]*?)(\s*)$/.exec(dedans);
    return m[2] ? m[1] + signe + m[2] + signe + m[3] : dedans;
  }
  function enLigne(n) {
    var s = "";
    n.childNodes.forEach(function (c) { s += inl(c); });
    return s;
  }
  function inl(c) {
    if (c.nodeType === 3) return echap(c.nodeValue.replace(/[\r\n]+/g, " ").replace(/\u00a0/g, " "));
    if (c.nodeType !== 1) return "";
    var t = c.tagName;
    if (t === "BR") return " ";
    if (t === "INPUT" || c.classList.contains("coche-note") || c.classList.contains("bulle-lien")) return "";
    if (t === "B" || t === "STRONG") return entoure("**", enLigne(c));
    if (t === "I" || t === "EM") return entoure("_", enLigne(c));
    if (t === "U" || t === "INS") return entoure("++", enLigne(c));
    if (t === "S" || t === "STRIKE" || t === "DEL") return entoure("~~", enLigne(c));
    if (t === "CODE" || t === "KBD") { var x = c.textContent.replace(/`/g, "'").replace(/\s+/g, " "); return x.trim() ? "`" + x + "`" : x; }
    if (t === "A") {
      var h = c.getAttribute("href") || "", txt = enLigne(c);
      if (!/^(https?:\/\/|mailto:)[^\s"'<>]+$/.test(h)) return txt;
      return c.textContent.trim() === h ? h : "[" + txt + "](" + h + ")";
    }
    return enLigne(c);
  }
  // Les lignes d'un élément qui contient des blocs (l'éditeur, une div…).
  function blocs(racine, out) {
    var ligne = null;
    function finir() { if (ligne !== null) { out.push(debut(ligne)); ligne = null; } }
    racine.childNodes.forEach(function (n) {
      if (n.nodeType === 3) {
        if (n.nodeValue.replace(/[\r\n]/g, "") !== "") ligne = (ligne || "") + inl(n);
        return;
      }
      if (n.nodeType !== 1) return;
      var t = n.tagName;
      if (n.hasAttribute("data-photo")) { finir(); out.push("[photo " + n.getAttribute("data-photo") + "]"); return; }
      if (n.hasAttribute("data-case")) {
        finir();
        out.push((n.getAttribute("data-cochee") === "1" ? "[x] " : "[ ] ") + enLigne(n).trim());
        return;
      }
      if (t === "BR") { if (ligne === null) out.push(""); else finir(); return; }
      if (t === "HR") { finir(); out.push("---"); return; }
      if (/^H[1-6]$/.test(t)) { finir(); out.push("###".slice(0, Math.min(3, +t[1])) + " " + enLigne(n).trim()); return; }
      if (t === "UL" || t === "OL") {
        finir();
        var k = parseInt(n.getAttribute("start"), 10) || 1;
        Array.prototype.forEach.call(n.children, function (li) {
          out.push((t === "UL" ? "- " : (k++) + ". ") + enLigne(li).trim());
        });
        return;
      }
      if (t === "BLOCKQUOTE") {
        finir();
        var sous = [];
        blocs(n, sous);
        sous.forEach(function (l) { out.push(l ? "> " + l : ">"); });
        return;
      }
      if (BLOCS.test(t)) {
        finir();
        var d = [];
        blocs(n, d);
        if (!d.length) d.push("");
        d.forEach(function (l) { out.push(l); });
        return;
      }
      ligne = (ligne || "") + inl(n);
    });
    finir();
  }
  // Après un collage, chaque bloc (paragraphe, liste, titre…) redevient un
  // enfant direct de l'éditeur (le navigateur les imbrique dans la ligne).
  function aplatir(ed) {
    Array.prototype.slice.call(ed.children).forEach(function (n) {
      if (!/^(DIV|P)$/.test(n.tagName) || n.hasAttribute("data-case") || n.hasAttribute("data-photo")) return;
      if (!Array.prototype.some.call(n.children, function (c) { return BLOCS.test(c.tagName); })) return;
      var morceaux = [], ligne = null;
      Array.prototype.slice.call(n.childNodes).forEach(function (c) {
        if (c.nodeType === 1 && BLOCS.test(c.tagName)) { ligne = null; morceaux.push(c); return; }
        if (c.nodeType === 1 && c.tagName === "BR") { ligne = null; return; }
        if (!ligne) { ligne = document.createElement("div"); morceaux.push(ligne); }
        ligne.appendChild(c);
      });
      morceaux.forEach(function (m) { ed.insertBefore(m, n); });
      ed.removeChild(n);
      aplatir(ed);
    });
  }
  function texte(ed) {
    var l = [];
    blocs(ed, l);
    while (l.length && l[l.length - 1] === "") l.pop();
    return l.join("\n");
  }

  // --- Coller : seulement une mise en forme simple, sûre -------------------

  var GARDE = { B: "b", STRONG: "b", I: "i", EM: "i", U: "u", S: "s", STRIKE: "s", DEL: "s", CODE: "code",
    H1: "h1", H2: "h2", H3: "h3", H4: "h3", H5: "h3", H6: "h3", UL: "ul", OL: "ol", LI: "li",
    BLOCKQUOTE: "blockquote", P: "div", DIV: "div", BR: "br", HR: "hr", A: "a", PRE: "div", TR: "div", TABLE: "div" };
  // La mise en forme écrite en style (Google Docs, Word…) : gras, italique,
  // souligné, barré ; « font-weight: normal » annule un <b>.
  // (L'attribut est lu comme du texte : la CSP de la page empêche le
  // navigateur d'appliquer les styles collés.)
  function styles(n) {
    var a = (n.getAttribute("style") || "").toLowerCase(), l = [];
    var val = function (nom) { var m = new RegExp("(?:^|;)\\s*" + nom + "\\s*:\\s*([^;!]+)").exec(a); return m ? m[1].trim() : ""; };
    var g = val("font-weight");
    if (g === "bold" || g === "bolder" || +g >= 600) l.push("b");
    if (val("font-style") === "italic") l.push("i");
    var d = val("text-decoration") + " " + val("text-decoration-line");
    if (/underline/.test(d)) l.push("u");
    if (/line-through/.test(d)) l.push("s");
    return { ajout: l, sansGras: g === "normal" || g === "lighter" || (g !== "" && +g < 600) };
  }
  function nettoyer(src, dst) {
    src.childNodes.forEach(function (n) {
      if (n.nodeType === 3) { dst.appendChild(document.createTextNode(n.nodeValue)); return; }
      if (n.nodeType !== 1 || /^(SCRIPT|STYLE|META|LINK|TITLE|NOSCRIPT|TEMPLATE|IFRAME|OBJECT|SVG|BUTTON|INPUT|SELECT|TEXTAREA)$/.test(n.tagName)) return;
      var tag = GARDE[n.tagName], st = styles(n);
      if (tag === "b" && st.sansGras) tag = null; // Google Docs : <b style="font-weight:normal"> autour de tout
      // Gras, italique… donnés par le style : des balises autour du contenu.
      var ici = dst;
      st.ajout.forEach(function (t) { if (t !== tag) { var x = document.createElement(t); ici.appendChild(x); ici = x; } });
      if (!tag) { nettoyer(n, ici); return; }
      var e = document.createElement(tag);
      if (tag === "a") {
        var h = n.getAttribute("href") || "";
        if (!/^(https?:\/\/|mailto:)/.test(h)) { nettoyer(n, ici); return; }
        e.setAttribute("href", h);
      }
      ici.appendChild(e);
      nettoyer(n, e);
    });
  }
  // Les balises qui portent une mise en forme (pas seulement des lignes).
  var MISE_EN_FORME = "b, i, u, s, code, a, h1, h2, h3, ul, ol, blockquote, hr";
  // Un texte brut qui porte déjà des balises (« <b>…</b> ») ou des signes
  // de mise en forme (« **gras** », « # titre », « - [ ] case »…).
  var BALISES = /<\/?(b|strong|i|em|u|s|a|p|div|br|ul|ol|li|h[1-6]|blockquote|code|span)\b[^>]*>/i;
  var SIGNES = /\*\*[^*\n]+\*\*|~~[^~\n]+~~|\+\+[^+\n]+\+\+|(^|\n)\s*#{1,3} \S|(^|\n)\s*(?:[-*] )?\[[ xX]\] |\[[^\]\n]+\]\((?:https?:\/\/|mailto:)|(^|[\s(])_[^_\n]+_(?=$|[\s.,;:!?)])|(^|\n)\s*> \S|`[^`\n]+`/;

  // --- Petits outils de sélection --------------------------------------------

  function blocCurseur(ed) {
    var s = window.getSelection();
    if (!s.rangeCount) return null;
    var n = s.getRangeAt(0).startContainer;
    if (n === ed) return ed.childNodes[s.getRangeAt(0).startOffset] || ed.lastChild;
    while (n && n.parentNode !== ed) n = n.parentNode;
    return n;
  }
  function curseurAuBout(el) {
    var r = document.createRange();
    r.selectNodeContents(el);
    r.collapse(false);
    var s = window.getSelection();
    s.removeAllRanges();
    s.addRange(r);
  }
  function ligneVide() { var d = document.createElement("div"); d.appendChild(document.createElement("br")); return d; }
  // Pose un bloc après celui du curseur (à sa place s'il est vide), et
  // une ligne vide après, où l'on continue d'écrire.
  function poser(ed, el, apres) {
    var l = apres && apres.parentNode === ed ? apres : blocCurseur(ed);
    if (l && l.parentNode === ed && l.nodeType === 1 && !l.hasAttribute("data-photo") && l.tagName !== "HR" && l.textContent === "" && !l.querySelector("img")) {
      ed.replaceChild(el, l);
    } else if (l && l.parentNode === ed) {
      ed.insertBefore(el, l.nextSibling);
    } else {
      ed.appendChild(el);
    }
    var suite = el.nextSibling;
    if (!suite || suite.nodeType !== 1 || suite.hasAttribute("data-photo") || suite.tagName === "HR") {
      suite = ligneVide();
      ed.insertBefore(suite, el.nextSibling);
    }
    return suite;
  }
  function figure(id) {
    var f = document.createElement("figure");
    f.className = "photo-texte";
    f.contentEditable = "false";
    f.setAttribute("data-photo", id);
    f.setAttribute("draggable", "false");
    f.innerHTML = '<img alt="Photo de la note" draggable="false"><span class="outils-photo">' +
      '<button type="button" class="monter-photo" aria-label="Monter la photo">↑</button>' +
      '<button type="button" class="descendre-photo" aria-label="Descendre la photo">↓</button>' +
      '<button type="button" class="retirer-photo" aria-label="Retirer la photo du texte">✕</button></span>';
    f.querySelector("img").src = "/notes/photos/" + id;
    return f;
  }
  // Une photo ou une séparation n'est pas une ligne où l'on écrit : il en
  // faut toujours une avant (en tête) et après (à la fin, entre deux photos),
  // sinon le curseur ne peut pas y aller.
  function figee(n) { return n && n.nodeType === 1 && (n.hasAttribute("data-photo") || n.tagName === "HR"); }
  function garantirLignes(ed) {
    if (figee(ed.firstChild)) ed.insertBefore(ligneVide(), ed.firstChild);
    Array.prototype.slice.call(ed.children).forEach(function (n) {
      if (figee(n) && (!n.nextSibling || figee(n.nextSibling))) ed.insertBefore(ligneVide(), n.nextSibling);
    });
  }
  function caseVide(texte) {
    var d = document.createElement("div");
    d.className = "case-note";
    d.setAttribute("data-case", "");
    d.setAttribute("data-cochee", "0");
    if (texte) d.appendChild(document.createTextNode(texte)); else d.appendChild(document.createElement("br"));
    return d;
  }

  var OUTILS = [
    ["undo", "↶", "Annuler"], ["redo", "↷", "Refaire"], null,
    ["bold", "<b>G</b>", "Gras"], ["italic", "<i>I</i>", "Italique"], ["underline", "<u>S</u>", "Souligné"],
    ["strikeThrough", "<s>B</s>", "Barré"], ["code", "&lt;/&gt;", "Code"], null,
    ["case", "☑", "Case à cocher"], ["insertUnorderedList", "•≡", "Liste à puces"], ["insertOrderedList", "1≡", "Liste numérotée"], null,
    ["h1", "T1", "Titre"], ["h2", "T2", "Sous-titre"], ["h3", "T3", "Petit titre"], ["blockquote", "❝", "Citation"], null,
    ["lien", "🔗", "Lien"], ["photo", "📷", "Photo ici"], ["hr", "―", "Ligne de séparation"]
  ];

  function monter(t) {
    if (t.hasAttribute("data-editeur-pret")) return;
    // Le contenu mis en forme : juste après le textarea (ou dans le formulaire, pour une note).
    var modele = t.nextElementSibling;
    if (!modele || !modele.matches("template[data-editeur-contenu]")) modele = t.form && t.form.querySelector("template[data-editeur-contenu]");
    if (!modele) return;
    var avecPhotos = t.hasAttribute("data-editeur"); // les notes seulement
    t.setAttribute("data-editeur-pret", "");
    var lecture = t.disabled || !!t.closest("fieldset[disabled]");
    var id = t.getAttribute("data-editeur");
    var ed = document.createElement("div");
    ed.className = "editeur-note" + (avecPhotos ? "" : " court");
    ed.contentEditable = lecture ? "false" : "true";
    ed.setAttribute("role", "textbox");
    ed.setAttribute("aria-multiline", "true");
    ed.setAttribute("aria-label", "Texte de la note");
    ed.appendChild(modele.content.cloneNode(true));
    if (!ed.firstChild) ed.appendChild(ligneVide());
    garantirLignes(ed);
    t.hidden = true;
    // Hors du <label> du textarea (un clic dans un label irait au textarea caché).
    var lieu = t.closest("label") || t;
    lieu.parentNode.insertBefore(ed, lieu.nextSibling);
    if (lecture) {
      ed.querySelectorAll("button").forEach(function (x) { x.hidden = true; });
      return;
    }
    var barre = document.createElement("div");
    barre.className = "barre-note";
    barre.setAttribute("role", "toolbar");
    barre.setAttribute("aria-label", "Mise en forme");
    OUTILS.forEach(function (o) {
      if (o && o[0] === "photo" && !avecPhotos) return;
      if (!o) { var sep = document.createElement("span"); sep.className = "sep"; barre.appendChild(sep); return; }
      var b = document.createElement("button");
      b.type = "button";
      b.setAttribute("data-outil", o[0]);
      b.setAttribute("aria-label", o[2]);
      b.title = o[2];
      b.innerHTML = o[1];
      barre.appendChild(b);
    });
    var fichier = document.createElement("input");
    fichier.type = "file";
    fichier.accept = "image/*";
    fichier.multiple = true;
    fichier.hidden = true;
    barre.appendChild(fichier);
    lieu.parentNode.insertBefore(barre, ed);

    var maj = function () { t.value = texte(ed); };
    ed.addEventListener("input", function () { garantirLignes(ed); });
    var derniere = null; // bloc du curseur avant de toucher un bouton
    var memo = function () { var b = blocCurseur(ed); if (b) derniere = b; };
    ed.addEventListener("input", maj);
    ed.addEventListener("keyup", memo);
    ed.addEventListener("mouseup", memo);
    ed.addEventListener("blur", maj);
    t.form.addEventListener("submit", maj);
    try { document.execCommand("defaultParagraphSeparator", false, "div"); } catch (_) { /* rien */ }
    try { document.execCommand("styleWithCSS", false, false); } catch (_) { /* rien */ }

    // Coller : les images sont envoyées ; le reste, nettoyé.
    ed.addEventListener("paste", function (e) {
      var cd = e.clipboardData;
      if (!cd) return;
      var images = Array.prototype.filter.call(cd.files || [], function (f) { return /^image\//.test(f.type); });
      e.preventDefault();
      if (images.length) { memo(); images.forEach(function (f) { envoyer(f, blocCurseur(ed)); }); return; }
      var html = cd.getData("text/html"), brut = cd.getData("text/plain");
      // Texte qui porte des signes ou des balises écrits en clair (Méli-Mélo :
      // « **<u>Pneu :</u>** », « ![](attachment:680) ») : le serveur le met en forme.
      var aSignes = function (t) { return SIGNES.test(t) || BALISES.test(t) || /!\[[^\]\n]*\]\(/.test(t); };
      if (html) {
        var src = new DOMParser().parseFromString(html, "text/html").body;
        var propre = document.createElement("div");
        nettoyer(src, propre);
        if (!propre.querySelector(MISE_EN_FORME)) {
          var lignes = brut || propre.innerText || propre.textContent;
          if (aSignes(lignes)) { collerSignes(lignes); return; }
        }
        document.execCommand("insertHTML", false, propre.innerHTML);
        aplatir(ed);
        maj();
      } else if (aSignes(brut)) {
        collerSignes(brut);
      } else {
        document.execCommand("insertText", false, brut);
        maj();
      }
    });
    // Texte avec des signes (« **gras** »…) : le serveur le met en forme
    // comme une note enregistrée (même règles), puis il est inséré.
    function collerSignes(brut) {
      var r = window.getSelection().rangeCount ? window.getSelection().getRangeAt(0).cloneRange() : null;
      fetch("/riche/apercu", { method: "POST", credentials: "same-origin",
        headers: { "X-Jeton": jeton(), "Content-Type": "application/json" }, body: JSON.stringify({ texte: brut }) })
        .then(function (rep) { if (!rep.ok) throw new Error("refusé"); return rep.text(); })
        .then(function (h) {
          ed.focus();
          if (r) { var s = window.getSelection(); s.removeAllRanges(); s.addRange(r); }
          // Une seule ligne de texte : collée dans la ligne en cours.
          var une = /^<div>(?:(?!<div|<\/div>)[\s\S])*<\/div>$/.test(h) ? h.slice(5, -6) : h;
          document.execCommand("insertHTML", false, une);
          aplatir(ed);
          garantirLignes(ed);
          maj();
        })
        .catch(function () { document.execCommand("insertText", false, brut); maj(); });
    }
    // Une photo de la note ne se glisse pas (↑ ↓ la déplacent).
    ed.addEventListener("dragstart", function (e) { if (e.target.closest && e.target.closest("figure[data-photo]")) e.preventDefault(); });
    // Glisser une image dans la note.
    ed.addEventListener("dragover", function (e) { if (e.dataTransfer && Array.prototype.some.call(e.dataTransfer.types, function (x) { return x === "Files"; })) e.preventDefault(); });
    ed.addEventListener("drop", function (e) {
      var fs = e.dataTransfer && Array.prototype.filter.call(e.dataTransfer.files || [], function (f) { return /^image\//.test(f.type); });
      if (!fs || !fs.length) return;
      e.preventDefault();
      if (document.caretRangeFromPoint) {
        var r = document.caretRangeFromPoint(e.clientX, e.clientY);
        if (r) { var s = window.getSelection(); s.removeAllRanges(); s.addRange(r); memo(); }
      }
      fs.forEach(function (f) { envoyer(f, blocCurseur(ed)); });
    });
    // Cases cochées dans l'éditeur ; ✕ d'une photo ; lien touché.
    ed.addEventListener("change", maj);
    var bulle = null;
    function fermerBulle() { if (bulle) { bulle.remove(); bulle = null; } }
    ed.addEventListener("click", function (e) {
      fermerBulle();
      // Toucher la case (à gauche de la ligne) : cochée / décochée.
      var cs = e.target.closest && e.target.closest("[data-case]");
      if (cs && e.clientX - cs.getBoundingClientRect().left < 30) {
        e.preventDefault();
        cs.setAttribute("data-cochee", cs.getAttribute("data-cochee") === "1" ? "0" : "1");
        maj();
        return;
      }
      var mv = e.target.closest(".monter-photo, .descendre-photo");
      if (mv) {
        e.preventDefault();
        var fg = mv.closest("figure"), haut = mv.classList.contains("monter-photo");
        // Saute par-dessus le bloc voisin (une ligne vide comprise).
        var voisin = haut ? fg.previousElementSibling : fg.nextElementSibling;
        if (voisin) {
          if (haut) ed.insertBefore(fg, voisin); else ed.insertBefore(fg, voisin.nextSibling);
          garantirLignes(ed);
          maj();
          fg.scrollIntoView({ block: "nearest" });
        }
        return;
      }
      var x = e.target.closest(".retirer-photo");
      if (x) {
        e.preventDefault();
        var f = x.closest("figure");
        f.parentNode.removeChild(f);
        garantirLignes(ed);
        maj();
        window.fcToast("Photo retirée du texte : elle reste en bas de la note");
        return;
      }
      if (e.target.closest("figure[data-photo]")) { e.preventDefault(); return; }
      var a = e.target.closest("a[href]");
      if (a && !a.closest("figure")) {
        e.preventDefault();
        bulle = document.createElement("span");
        bulle.className = "bulle-lien";
        bulle.contentEditable = "false";
        var o = document.createElement("a");
        o.href = a.href;
        o.target = "_blank";
        o.rel = "noopener noreferrer";
        o.textContent = "↗ Ouvrir le lien";
        bulle.appendChild(o);
        a.parentNode.insertBefore(bulle, a.nextSibling);
      }
    });
    // Entrée au bout d'une case : la suivante ; sur une case vide : fin.
    ed.addEventListener("keydown", function (e) {
      if (e.key !== "Enter" || e.shiftKey || e.isComposing) return;
      var l = blocCurseur(ed);
      if (!l || l.nodeType !== 1 || !l.hasAttribute("data-case")) return;
      var s = window.getSelection(), r = document.createRange();
      r.selectNodeContents(l);
      r.setEnd(s.getRangeAt(0).startContainer, s.getRangeAt(0).startOffset);
      if (r.toString().length !== l.textContent.length) return; // pas au bout
      e.preventDefault();
      if (l.textContent.trim() === "") {
        var v = ligneVide();
        ed.replaceChild(v, l);
        curseurAuBout(v);
      } else {
        var n = caseVide("");
        ed.insertBefore(n, l.nextSibling);
        curseurAuBout(n);
      }
      maj();
    });

    // La barre : le curseur reste dans le texte.
    barre.addEventListener("mousedown", function (e) { if (e.target.closest("button")) e.preventDefault(); });
    barre.addEventListener("click", function (e) {
      var b = e.target.closest("button[data-outil]");
      if (!b) return;
      var o = b.getAttribute("data-outil");
      if (document.activeElement !== ed && o !== "photo") { ed.focus(); if (derniere && derniere.parentNode === ed) curseurAuBout(derniere); }
      var bl = blocCurseur(ed);
      switch (o) {
        case "h1": case "h2": case "h3": case "blockquote":
          var deja = bl && bl.nodeType === 1 && (bl.tagName === o.toUpperCase() || (o === "blockquote" && bl.closest && bl.closest("blockquote")));
          document.execCommand("formatBlock", false, deja ? "div" : o);
          break;
        case "case":
          if (bl && bl.nodeType === 1 && bl.hasAttribute("data-case")) {
            var p = document.createElement("div");
            enfants(bl, p);
            if (!p.firstChild) p.appendChild(document.createElement("br"));
            ed.replaceChild(p, bl);
            curseurAuBout(p);
          } else if (bl && bl.parentNode === ed && bl.nodeType === 1 && /^(DIV|P|H[1-6])$/.test(bl.tagName)) {
            var c = caseVide("");
            c.removeChild(c.lastChild); // la ligne garde son texte (ou son <br>)
            enfants(bl, c);
            if (!c.firstChild) c.appendChild(document.createElement("br"));
            ed.replaceChild(c, bl);
            curseurAuBout(c);
          } else {
            var c2 = caseVide("");
            poser(ed, c2, bl);
            curseurAuBout(c2);
          }
          break;
        case "code":
          var s = window.getSelection();
          if (!s.rangeCount || s.isCollapsed) { window.fcToast("Sélectionnez d'abord le texte"); return; }
          var dans = s.anchorNode && s.anchorNode.parentElement && s.anchorNode.parentElement.closest("code");
          if (dans && ed.contains(dans)) {
            dans.replaceWith(document.createTextNode(dans.textContent));
          } else {
            var code = document.createElement("code");
            code.textContent = s.toString();
            var r = s.getRangeAt(0);
            r.deleteContents();
            r.insertNode(code);
          }
          break;
        case "lien":
          var sel = window.getSelection().toString();
          var u = window.prompt("Adresse du lien (https://…)", /^https?:\/\//.test(sel) ? sel : "https://");
          if (!u || !/^(https?:\/\/|mailto:)\S+$/.test(u.trim())) return;
          if (sel) document.execCommand("createLink", false, u.trim());
          else document.execCommand("insertText", false, u.trim());
          break;
        case "hr":
          var hr = document.createElement("hr");
          hr.contentEditable = "false";
          curseurAuBout(poser(ed, hr, bl));
          break;
        case "photo":
          if (!id) {
            // Note pas encore enregistrée : elle l'est d'abord (la photo a besoin d'elle).
            maj();
            window.fcToast("Note enregistrée : touchez de nouveau 📷");
            var sv = t.form.querySelector("button[name=suite]");
            if (sv) sv.click(); else t.form.requestSubmit();
            return;
          }
          fichier.click();
          return;
        default:
          document.execCommand(o, false, null);
      }
      maj();
      memo();
    });
    function enfants(de, vers) {
      Array.prototype.slice.call(de.childNodes).forEach(function (x) {
        if (x.nodeType === 1 && x.classList.contains("coche-note")) return;
        vers.appendChild(x);
      });
    }
    fichier.addEventListener("change", function () {
      Array.prototype.forEach.call(fichier.files || [], envoyer);
      fichier.value = "";
    });
    function envoyer(f, ou) {
      if (!avecPhotos) { window.fcToast("Les photos se mettent dans les notes", true); return; }
      if (!id) { window.fcToast("Enregistrez d'abord la note (📷), puis collez la photo", true); return; }
      if (!window.createImageBitmap) { window.fcToast("Photo non prise en charge par ce navigateur", true); return; }
      var attente = document.createElement("div");
      attente.contentEditable = "false";
      attente.className = "photo-attente";
      attente.textContent = "⏳ Envoi de la photo…";
      derniere = poser(ed, attente, ou || derniere || blocCurseur(ed));
      window.fcReduirePhoto(f, 1600).then(function (blob) {
        return fetch("/notes/" + id + "/photos?place=1", { method: "POST", credentials: "same-origin",
          headers: { "X-Jeton": jeton(), "Content-Type": "image/jpeg" }, body: blob });
      }).then(function (rep) {
        if (!rep.ok) return rep.text().then(function () { throw new Error("refusée"); });
        return rep.json();
      }).then(function (r) {
        var fig = figure(String(r.id));
        ed.replaceChild(fig, attente);
        garantirLignes(ed);
        derniere = fig.nextSibling; // la ligne sous la photo : on y écrit
        ed.focus();
        curseurAuBout(derniere);
        maj();
        window.fcToast("Photo ajoutée : pensez à enregistrer la note");
      }).catch(function () {
        if (attente.parentNode) attente.parentNode.removeChild(attente);
        window.fcToast("Photo non envoyée", true);
      });
    }
    // Photo du bas de la note : « Dans le texte ».
    document.querySelectorAll("[data-placer]").forEach(function (b) {
      b.hidden = false;
      b.addEventListener("click", function () {
        var f = figure(b.getAttribute("data-placer"));
        derniere = poser(ed, f, derniere || ed.lastChild);
        garantirLignes(ed);
        var p = b.closest(".piece");
        if (p) p.parentNode.removeChild(p);
        maj();
        window.fcToast("Photo placée : pensez à enregistrer la note");
      });
    });
    maj();
  }
  function tout() { document.querySelectorAll("textarea[data-editeur], textarea[data-riche]").forEach(monter); }
  document.addEventListener("DOMContentLoaded", tout);
  document.addEventListener("htmx:afterSettle", tout);
})();

// Graphiques (svg.graphique, dessinés par le serveur) : un toucher ou la
// souris montre un repère et une bulle (date et valeur du point le plus
// proche, ou détail de la barre). Chaque point ou barre porte data-info.
(function () {
  "use strict";
  var NS = "http://www.w3.org/2000/svg";
  function cibles(svg) {
    var l = [];
    svg.querySelectorAll("circle[data-info]").forEach(function (c) {
      l.push({ x: +c.getAttribute("cx"), y: +c.getAttribute("cy"), info: c.getAttribute("data-info"), el: c });
    });
    svg.querySelectorAll("g[data-info]").forEach(function (g) {
      l.push({ x: +g.getAttribute("data-x"), y: null, info: g.getAttribute("data-info"), el: g });
    });
    return l;
  }
  function montrer(svg, e) {
    var l = svg._cibles || (svg._cibles = cibles(svg));
    if (!l.length) return;
    var r = svg.getBoundingClientRect(), vb = svg.viewBox.baseVal;
    var x = (e.clientX - r.left) * vb.width / r.width;
    var près = l[0];
    l.forEach(function (c) { if (Math.abs(c.x - x) < Math.abs(près.x - x)) près = c; });
    var h = vb.height;
    var rep = svg.querySelector(".g-repere");
    if (!rep) { rep = document.createElementNS(NS, "line"); rep.setAttribute("class", "g-repere"); svg.appendChild(rep); }
    rep.setAttribute("x1", près.x); rep.setAttribute("x2", près.x);
    rep.setAttribute("y1", 10); rep.setAttribute("y2", h - 22);
    var pt = svg.querySelector(".g-actif");
    if (près.y !== null) {
      if (!pt) { pt = document.createElementNS(NS, "circle"); pt.setAttribute("class", "g-actif"); pt.setAttribute("r", 5); svg.appendChild(pt); }
      pt.setAttribute("cx", près.x); pt.setAttribute("cy", près.y);
    } else if (pt) pt.remove();
    svg.querySelectorAll(".g-barre.choisie").forEach(function (g) { g.classList.remove("choisie"); });
    if (près.y === null) près.el.classList.add("choisie");
    var cadre = svg.parentNode, bulle = cadre.querySelector(".bulle-graphique");
    if (!bulle) { bulle = document.createElement("div"); bulle.className = "bulle-graphique"; bulle.setAttribute("aria-live", "polite"); cadre.appendChild(bulle); }
    bulle.textContent = près.info;
    bulle.hidden = false;
    var rc = cadre.getBoundingClientRect();
    var px = r.left - rc.left + près.x * r.width / vb.width;
    var w = bulle.offsetWidth, max = cadre.clientWidth - w - 4;
    bulle.style.left = Math.max(4, Math.min(max, px - w / 2)) + "px";
    bulle.style.top = Math.max(2, r.top - rc.top - bulle.offsetHeight + 4) + "px"; // juste au-dessus du dessin
  }
  function cacher(svg) {
    var x = svg.querySelector(".g-repere"); if (x) x.remove();
    var p = svg.querySelector(".g-actif"); if (p) p.remove();
    svg.querySelectorAll(".g-barre.choisie").forEach(function (g) { g.classList.remove("choisie"); });
    var b = svg.parentNode.querySelector(".bulle-graphique"); if (b) b.hidden = true;
  }
  document.addEventListener("pointermove", function (e) {
    var svg = e.target.closest && e.target.closest("svg.graphique");
    if (svg && e.pointerType === "mouse") montrer(svg, e);
  });
  document.addEventListener("pointerdown", function (e) {
    var svg = e.target.closest && e.target.closest("svg.graphique");
    document.querySelectorAll("svg.graphique").forEach(function (s) { if (s !== svg) cacher(s); });
    if (svg) montrer(svg, e);
  });
  document.addEventListener("pointerout", function (e) {
    var svg = e.target.closest && e.target.closest("svg.graphique");
    if (svg && e.pointerType === "mouse" && !svg.contains(e.relatedTarget)) cacher(svg);
  });
  // Glisser le doigt le long de la courbe.
  document.addEventListener("touchmove", function (e) {
    var svg = e.target.closest && e.target.closest("svg.graphique");
    if (svg && e.touches[0]) montrer(svg, e.touches[0]);
  }, { passive: true });
})();

// Choix de l'emoji d'une catégorie de notes : le bouton [data-choix-emoji]
// ouvre la grille (<template id="grille-emojis">) sous sa ligne
// ([data-lieu-emojis]) ; un toucher met l'emoji dans le champ caché voisin.
// « Autre » : un emoji écrit au clavier. data-choix-emoji="envoyer" :
// catégorie existante, enregistrée dès le choix.
(function () {
  "use strict";
  var ouverte = null; // { grille, bouton }

  function fermer() {
    if (!ouverte) return;
    ouverte.grille.remove();
    ouverte.bouton.setAttribute("aria-expanded", "false");
    ouverte = null;
  }
  function ouvrir(bouton) {
    var modele = document.getElementById("grille-emojis");
    var lieu = bouton.closest("[data-lieu-emojis]");
    if (!modele || !lieu) return;
    var grille = modele.content.firstElementChild.cloneNode(true);
    var valeur = bouton.parentNode.querySelector("input[name=emoji]").value;
    grille.querySelectorAll("[data-emoji]").forEach(function (b) {
      if (b.getAttribute("data-emoji") === valeur) b.classList.add("choisi");
    });
    lieu.after(grille);
    bouton.setAttribute("aria-expanded", "true");
    ouverte = { grille: grille, bouton: bouton };
  }
  // Met l'emoji dans le champ ; fin : on ferme (et on enregistre si demandé).
  function choisir(emoji, fin) {
    var b = ouverte.bouton;
    b.parentNode.querySelector("input[name=emoji]").value = emoji;
    b.textContent = emoji || "＋";
    b.classList.toggle("vide", !emoji);
    if (!fin) return;
    fermer();
    if (b.getAttribute("data-choix-emoji") === "envoyer" && b.form) b.form.requestSubmit();
    else b.focus();
  }

  document.addEventListener("click", function (e) {
    var b = e.target.closest && e.target.closest("[data-choix-emoji]");
    if (b) {
      var deja = ouverte && ouverte.bouton === b;
      fermer();
      if (!deja) ouvrir(b);
      return;
    }
    if (!ouverte) return;
    var x = e.target.closest("[data-emoji]");
    if (x && ouverte.grille.contains(x)) { choisir(x.getAttribute("data-emoji"), true); return; }
    // Toucher ailleurs ferme la grille.
    if (!ouverte.grille.contains(e.target)) fermer();
  });
  document.addEventListener("input", function (e) {
    if (ouverte && e.target.matches("[data-emoji-autre]")) choisir(e.target.value.trim(), false);
  });
  document.addEventListener("keydown", function (e) {
    if (!ouverte) return;
    if (e.key === "Escape") { var b = ouverte.bouton; fermer(); b.focus(); return; }
    // Entrée dans « Autre » : valider sans envoyer le formulaire d'ajout.
    if (e.key === "Enter" && e.target.matches("[data-emoji-autre]")) {
      e.preventDefault();
      choisir(e.target.value.trim(), true);
    }
  });
  // Page changée par htmx : la grille ouverte disparaît avec elle.
  document.addEventListener("htmx:beforeSwap", fermer);
})();

// Pièces jointes : le fichier choisi part aussitôt (le formulaire est
// envoyé par htmx, avec le fichier).
document.addEventListener("change", function (e) {
  var f = e.target;
  if (f.matches && f.matches("input[type=file][data-envoi-auto]") && f.files.length && f.form) f.form.requestSubmit();
});


// Notification touchée : si l'appli s'est ouverte ailleurs que sur la page
// visée (iPhone : souvent l'accueil), on y va. Le service worker garde la
// cible une minute et ne la donne qu'une fois.
(function () {
  "use strict";
  if (!navigator.serviceWorker || !navigator.serviceWorker.controller || !window.fetch) return;
  fetch("/__cible-notification", { credentials: "same-origin" }).then(function (r) {
    return r.status === 200 ? r.json() : null;
  }).then(function (j) {
    if (!j || !j.url || Date.now() - j.le > 60000) return;
    var cible = new URL(j.url, location.origin);
    if (cible.origin === location.origin && cible.pathname + cible.search !== location.pathname + location.search) {
      location.replace(cible.pathname + cible.search);
    }
  }).catch(function () {});
})();
