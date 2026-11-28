// SPDX-License-Identifier: GPL-3.0-or-later

package fidelite

import (
	"bytes"
	"encoding/json"
	"image"
	"io"
	"net/http"

	"familycocon/internal/appli"
	photos "familycocon/internal/photo"
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
	// Taille et mémoire vérifiées sur l'en-tête, avant tout décodage ; une
	// seule photo décodée à la fois (comme photos.Preparer).
	if err := photos.Controler(bytes.NewReader(corps), pixelsMax); err != nil {
		repondre(http.StatusBadRequest, map[string]string{"erreur": "Photo illisible ou trop grande."})
		return
	}
	var numero, format string
	var ok bool
	if err := photos.Decoder(corps, func(img image.Image) { numero, format, ok = Lire(img) }); err != nil {
		repondre(http.StatusBadRequest, map[string]string{"erreur": "Photo illisible."})
		return
	}
	if !ok {
		repondre(http.StatusUnprocessableEntity, map[string]string{"erreur": "Aucun code-barres lu : approchez la carte, à plat et bien éclairée, ou tapez le numéro."})
		return
	}
	repondre(http.StatusOK, map[string]string{"numero": numero, "format": format, "nom": NomFormat(format)})
}
