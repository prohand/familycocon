// SPDX-License-Identifier: GPL-3.0-or-later

package appli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Annuler : après une suppression rapide (produit des courses, produits
// pris), la notification propose « Annuler » pendant quelques secondes.
// Ce qu'il faut pour remettre est gardé en mémoire (jamais en base) :
// un redémarrage du serveur l'oublie, ce n'est qu'un confort.

// dureeAnnuler : délai pour annuler.
const dureeAnnuler = 15 * time.Second

// annulationsMax : annulations gardées en même temps (mémoire bornée).
const annulationsMax = 200

type annulation struct {
	famille  int64
	module   string
	expire   time.Time
	remettre func(ctx context.Context) error
}

// Corbeille : les annulations possibles, par jeton.
type Corbeille struct {
	mu sync.Mutex
	l  map[string]annulation
}

func nouvelleCorbeille() *Corbeille { return &Corbeille{l: map[string]annulation{}} }

// deposer garde la fonction qui remet ce qui vient d'être supprimé ;
// renvoie le jeton à donner au navigateur.
func (cb *Corbeille) deposer(famille int64, module string, remettre func(ctx context.Context) error) string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	jeton := hex.EncodeToString(b)
	cb.mu.Lock()
	defer cb.mu.Unlock()
	maintenant := time.Now()
	for k, x := range cb.l {
		if maintenant.After(x.expire) {
			delete(cb.l, k)
		}
	}
	if len(cb.l) >= annulationsMax {
		return "" // trop d'annulations en attente : pas de bouton cette fois
	}
	cb.l[jeton] = annulation{famille: famille, module: module, expire: maintenant.Add(dureeAnnuler), remettre: remettre}
	return jeton
}

// prendre retire l'annulation (une seule fois) si elle est encore valable
// et appartient à cette famille.
func (cb *Corbeille) prendre(jeton string, famille int64) (annulation, bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	x, ok := cb.l[jeton]
	if !ok || x.famille != famille || time.Now().After(x.expire) {
		return annulation{}, false
	}
	delete(cb.l, jeton)
	return x, true
}

// NotifierAnnulable : notification avec un bouton « Annuler » qui appelle
// remettre (dans les 15 secondes). Sans place, une simple notification.
func (a *App) NotifierAnnulable(w http.ResponseWriter, c *Contexte, module, texte string, remettre func(ctx context.Context) error) {
	jeton := a.Corbeille.deposer(c.Famille.ID, module, remettre)
	toast := map[string]any{"texte": texte, "erreur": false}
	if jeton != "" {
		toast["annuler"] = "/annuler/" + jeton
	}
	b, _ := json.Marshal(map[string]any{"fc-toast": toast})
	w.Header().Set("HX-Trigger", jsonASCII(b))
}

// annuler : le bouton « Annuler » de la notification.
func (a *App) annuler(w http.ResponseWriter, r *http.Request, c *Contexte) {
	x, ok := a.Corbeille.prendre(r.PathValue("jeton"), c.Famille.ID)
	if !ok {
		a.Echec(w, r, http.StatusGone, "Trop tard pour annuler.")
		return
	}
	if err := x.remettre(r.Context()); err != nil {
		a.ErreurInterne(w, r, err)
		return
	}
	a.Publier(c, x.module, "")
	w.WriteHeader(http.StatusNoContent)
}
