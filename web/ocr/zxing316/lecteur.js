// SPDX-License-Identifier: GPL-3.0-or-later
// Lecteur de codes (cartes de fidélité) : zxing-wasm 3.1.6 (MIT), qui
// emballe zxing-cpp (Apache-2.0) compilé en WebAssembly. Il tourne dans ce
// « worker » (sa politique de sécurité permet WebAssembly, pas la page).
// Reçoit une image (ImageData), répond {codes: [{texte, format}]}.
// Rien ne sort du téléphone.
/* global ZXingWASM */
"use strict";
importScripts("/ocr/zxing316/index.js");
ZXingWASM.prepareZXingModule({
  overrides: {
    locateFile: function (chemin, prefixe) {
      return chemin.endsWith(".wasm") ? "/ocr/zxing316/zxing_reader.wasm" : prefixe + chemin;
    }
  }
});
var formats = ["QRCode", "PDF417", "Aztec", "DataMatrix", "EAN13", "EAN8", "UPCA", "UPCE", "Code128", "Code39", "Code93", "ITF", "Codabar"];
self.onmessage = function (e) {
  ZXingWASM.readBarcodes(e.data, { formats: formats, tryHarder: true, tryRotate: true, tryInvert: true, tryDownscale: true, maxNumberOfSymbols: 1 })
    .then(function (res) {
      self.postMessage({ codes: res.filter(function (r) { return r.isValid; }).map(function (r) { return { texte: r.text, format: r.format }; }) });
    })
    .catch(function (err) { self.postMessage({ erreur: String(err) }); });
};
