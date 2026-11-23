# Composants tiers

| Composant | Version | Licence | Où |
|---|---|---|---|
| htmx | 2.0.11 | 0BSD (Zero-Clause BSD) | `web/statique/js/htmx.min.js` |
| Nunito (police) | variable, latin | SIL Open Font License 1.1 | `web/statique/fonts/` (`OFL.txt`) |
| modernc.org/sqlite | voir go.mod | BSD-3-Clause | dépendance Go |
| golang.org/x/crypto | voir go.mod | BSD-3-Clause | dépendance Go |
| rsc.io/qr | v0.2.0 | BSD-3-Clause | dépendance Go (QR code de la double authentification) |
| Tesseract.js | 7.0.0 | Apache-2.0 | `web/ocr/v7/tesseract.min.js.gz`, `worker.min.js.gz` (`web/ocr/LICENCE-tesseract.js.txt`, `LICENCE-composants.txt`) |
| tesseract.js-core (Tesseract, Leptonica…, en WebAssembly) | 7.0.0 | Apache-2.0 | `web/ocr/v7/tesseract-core-simd-lstm.*.gz` (`web/ocr/LICENCE-tesseract-core.txt`) |
| Modèle de langue français (tessdata, « best_int ») | @tesseract.js-data/fra 1.0.0 | Apache-2.0 (tessdata), paquet MIT | `web/ocr/v7/fra.traineddata.gz` |

Code repris de mirador-stack (même auteur, GPL-3.0) : indiqué en commentaire dans les fichiers concernés.
