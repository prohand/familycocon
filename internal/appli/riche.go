// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"encoding/json"
	"net/http"

	"familycocon/internal/riche"
)

// apercuRiche : POST /riche/apercu : un texte collé qui contient déjà des
// signes de mise en forme (« **gras** », « # titre », « - [ ] case »…,
// copié d'une autre appli ou d'un texte brut) devient le HTML de l'éditeur,
// au lieu de garder ses signes à l'écran. Rien n'est enregistré.
func (a *App) apercuRiche(w http.ResponseWriter, r *http.Request, c *Contexte) {
	var corps struct {
		Texte string `json:"texte"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&corps); err != nil {
		http.Error(w, "texte illisible", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(riche.HTML(corps.Texte, riche.Options{Mode: riche.Edition})))
}
