// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"compress/gzip"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"familycocon/web"
)

// Lecture des tickets (scan d'un ticket de carburant) : Tesseract.js
// (Apache-2.0) tourne dans le navigateur ; ses fichiers (~1,8 Mo
// compressés) sont dans web/ocr/v7, gardés compressés dans le binaire et
// lus à la demande (jamais chargés en mémoire au démarrage, contrairement
// aux autres fichiers statiques). Le téléphone les télécharge une fois, au
// premier scan, puis les garde (adresse versionnée : v7).
//
// Même chose pour le lecteur de codes des cartes de fidélité (zxing-wasm,
// MIT, et zxing-cpp, Apache-2.0 : QR code, PDF417, Aztec, EAN, Code 128…),
// dans web/ocr/zxing316 (~420 Ko compressés), lancé dans le worker
// lecteur.js : un iPhone n'a pas de lecteur de codes dans son navigateur.
//
// Ces réponses autorisent WebAssembly ('wasm-unsafe-eval') : le lecteur
// est un programme WebAssembly, lancé dans un « worker » dont la politique
// de sécurité est celle de sa propre réponse. Le reste du site n'en a pas
// besoin et garde la politique stricte.

// cspOCR : la politique du site, plus WebAssembly.
var cspOCR = strings.Replace(csp, "script-src 'self'", "script-src 'self' 'wasm-unsafe-eval'", 1)

// servirOCR : GET /ocr/v7/<fichier>. Les fichiers .js et .wasm sont
// rangés en .gz : envoyés tels quels (Content-Encoding: gzip), ou
// décompressés pour un client qui ne lit pas le gzip.
func (a *App) servirOCR(w http.ResponseWriter, r *http.Request) {
	nom := path.Clean("/" + r.PathValue("fichier"))[1:]
	if !(strings.HasPrefix(nom, "v7/") || strings.HasPrefix(nom, "zxing316/")) || strings.Count(nom, "/") != 1 {
		http.NotFound(w, r)
		return
	}
	racine, _ := fs.Sub(web.Fichiers, "ocr")
	h := w.Header()
	h.Set("Content-Security-Policy", cspOCR)
	h.Set("Cache-Control", "public, max-age=31536000, immutable")
	// Le modèle de langue est déjà un .gz : le navigateur le décompresse lui-même.
	if strings.HasSuffix(nom, ".traineddata.gz") {
		f, err := racine.Open(nom)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		h.Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, f)
		return
	}
	f, err := racine.Open(nom + ".gz")
	if err != nil {
		// Petit script écrit ici (zxing316/lecteur.js) : gardé tel quel. Pas
		// les .gz eux-mêmes (« fichier.js.gz » demandé tel quel : 404).
		if strings.HasSuffix(nom, ".js") {
			if g, err := racine.Open(nom); err == nil {
				defer g.Close()
				h.Set("Content-Type", "text/javascript; charset=utf-8")
				_, _ = io.Copy(w, g)
				return
			}
		}
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	t := mime.TypeByExtension(path.Ext(nom))
	if path.Ext(nom) == ".js" {
		t = "text/javascript; charset=utf-8"
	}
	h.Set("Content-Type", t)
	h.Set("Vary", "Accept-Encoding")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		h.Set("Content-Encoding", "gzip")
		_, _ = io.Copy(w, f)
		return
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		http.Error(w, "fichier abîmé", http.StatusInternalServerError)
		return
	}
	_, _ = io.Copy(w, zr)
}
