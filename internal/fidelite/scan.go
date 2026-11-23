// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"bytes"
	"encoding/json"
	"image"
	_ "image/jpeg" // photos du téléphone
	_ "image/png"
	"io"
	"net/http"

	"familycocon/internal/appli"
)

// Scanner une carte : le navigateur lit le code lui-même quand il le sait
// (BarcodeDetector, sur Android) ; sinon (iPhone…) il envoie une photo
// réduite (1 280 px au plus, JPEG) et le serveur la lit (Lire). Rien n'est
// gardé : la photo est oubliée dès la réponse.

// scanner : POST d'une photo (corps brut, image/jpeg ou image/png) →
// {"numero": "…", "format": "ean13"} ou {"erreur": "…"}.
func (m *Module) scanner(w http.ResponseWriter, r *http.Request, c *appli.Contexte) {
	repondre := func(statut int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statut)
		_ = json.NewEncoder(w).Encode(v)
	}
	corps, err := io.ReadAll(r.Body) // borné à 1 Mio par le serveur
	if err != nil || len(corps) == 0 {
		repondre(http.StatusRequestEntityTooLarge, map[string]string{"erreur": "Photo trop lourde."})
		return
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(corps))
	if err != nil || cfg.Width*cfg.Height > pixelsMax {
		repondre(http.StatusBadRequest, map[string]string{"erreur": "Photo illisible."})
		return
	}
	img, _, err := image.Decode(bytes.NewReader(corps))
	if err != nil {
		repondre(http.StatusBadRequest, map[string]string{"erreur": "Photo illisible."})
		return
	}
	numero, format, ok := Lire(img)
	if !ok {
		repondre(http.StatusUnprocessableEntity, map[string]string{"erreur": "Aucun code-barres lu : approchez la carte, à plat et bien éclairée, ou tapez le numéro."})
		return
	}
	repondre(http.StatusOK, map[string]string{"numero": numero, "format": format, "nom": NomFormat(format)})
}
